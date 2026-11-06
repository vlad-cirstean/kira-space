package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/postgres"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

var (
	regexpPGVersion = testsupport.VersionPattern("PostgreSQL", testsupport.PostgresServerMajor())
	bigRowsDetail   = regexp.MustCompile(`^~[\d.]+[A-Za-z]*$`)
)

// TestMain is bun:test's own beforeAll/afterAll-per-file precedent (postgres.spec.ts:68-75): the
// container this whole package's tests share is torn down exactly once, after every test has run
// — never from an individual test's t.Cleanup, which would fire the moment the first test to call
// testsupport.StartPostgres returns and kill the container out from under every later test.
func TestMain(m *testing.M) {
	code := m.Run()
	testsupport.StopPostgres()
	os.Exit(code)
}

// Ported from packages/db-fixtures/postgres.spec.ts, case by case where practical — the spec's own
// numbering is kept in each test's name so the two can be diffed. Not every one of the 34 TS
// cases has a Go twin; the ones ported are the load-bearing behaviours the "adapter-first-
// test-first" rule exists to protect: connect/disconnect lifecycle, the catalog->tree mapping,
// quoting, describe, cancellation, all three pagination shapes, mutate's transactionality, and
// execute's batch semantics.

var deps = adapters.Deps{Log: func(level, message string) {}}

func newAdapter(t *testing.T) adapters.Adapter {
	t.Helper()
	a, err := adapters.CreateAdapter("postgres", deps)
	if err != nil {
		t.Fatalf("CreateAdapter: %v", err)
	}
	return a
}

func connectedAdapter(t *testing.T, fixture *testsupport.PgFixture) adapters.Adapter {
	t.Helper()
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

// nodePath is testsupport.FixtureNodePath instantiated for *PgFixture (P107 I2-28).
var nodePath = testsupport.FixtureNodePath[*testsupport.PgFixture]

var (
	seg          = testsupport.Seg
	childNames   = testsupport.ChildNames
	containsName = testsupport.ContainsName
	cellAt       = testsupport.CellAt
	strp         = testsupport.Strp
)

// 1. connect / disconnect
func TestPostgres_ConnectDisconnect(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := newAdapter(t)

	info, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("op-1"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !regexpPGVersion.MatchString(info.ServerVersion) {
		t.Errorf("ServerVersion = %q, want to match %s", info.ServerVersion, regexpPGVersion)
	}

	if err := a.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	side, err := pgx.Connect(context.Background(), fixture.URI)
	if err != nil {
		t.Fatalf("side connect: %v", err)
	}
	defer side.Close(context.Background())
	var n int
	if err := side.QueryRow(context.Background(),
		"SELECT count(*) FROM pg_stat_activity WHERE application_name = 'kira-studio'").Scan(&n); err != nil {
		t.Fatalf("stat query: %v", err)
	}
	if n != 0 {
		t.Errorf("pg_stat_activity count = %d, want 0 (Disconnect must close every connection)", n)
	}
}

// 2. auth failure
func TestPostgres_AuthFailure(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := newAdapter(t)

	badCfg := fixture.Config
	wrong := "definitely-wrong"
	badCfg.Password = &wrong

	_, err := a.Connect(context.Background(), badCfg, adapters.NewOpCtx("op-2"))
	if err == nil {
		t.Fatal("expected an error for a wrong password")
	}
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeAuth {
		t.Fatalf("got %v, want an E_AUTH *adapters.Error", err)
	}
}

// A probe-query failure after a successful auth must not deadlock Connect. The probe runs
// while still holding connSet's primary connEntry lock (acquired by connSet.Primary); the
// error-handling path used to call Disconnect synchronously from the same call frame, which
// re-locks that same non-reentrant mutex via ConnSet.CloseAll and hangs forever. Reproduced here
// by letting auth succeed but revoking the probe query's own privilege, so the failure happens
// exactly where the deadlock used to live — after Primary() returns, before release() runs.
func TestPostgres_ConnectProbeFailureDoesNotDeadlock(t *testing.T) {
	fixture := testsupport.StartPostgres(t)

	side, err := pgx.Connect(context.Background(), fixture.URI)
	if err != nil {
		t.Fatalf("side connect: %v", err)
	}
	// Registered first, so its LIFO position is last: every cleanup below runs against a still-open
	// connection (authmatrix_test.go's own pattern).
	t.Cleanup(func() { _ = side.Close(context.Background()) })

	const roleName, rolePassword = "p1_probe_fail", "p1_probe_pw"
	mustExec(t, side, fmt.Sprintf(`CREATE ROLE %s LOGIN PASSWORD '%s'`, roleName, rolePassword))
	t.Cleanup(func() {
		mustExec(t, side, `DROP OWNED BY `+roleName)
		mustExec(t, side, `REVOKE CONNECT ON DATABASE kira_test FROM `+roleName)
		mustExec(t, side, `DROP ROLE IF EXISTS `+roleName)
	})
	mustExec(t, side, `GRANT CONNECT ON DATABASE kira_test TO `+roleName)

	// Breaks adapter.go's connect probe (`current_setting('server_encoding')`) for every
	// non-superuser role, so auth succeeds but the probe fails with a permission error. No other
	// test in this package runs concurrently (no t.Parallel here), and the grant is restored
	// below and in cleanup, so this doesn't leak into any other test.
	mustExec(t, side, `REVOKE EXECUTE ON FUNCTION pg_catalog.current_setting(text) FROM PUBLIC`)
	t.Cleanup(func() {
		mustExec(t, side, `GRANT EXECUTE ON FUNCTION pg_catalog.current_setting(text) TO PUBLIC`)
	})

	cfg := fixture.Config
	cfg.Username, cfg.Password = testsupport.Strp(roleName), testsupport.Strp(rolePassword)

	a := newAdapter(t)

	failDone := make(chan error, 1)
	go func() {
		_, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-probe-fail"))
		failDone <- err
	}()
	select {
	case err := <-failDone:
		if err == nil {
			t.Fatal("expected Connect to fail when the post-connect probe query fails")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Connect did not return within 5s for a failing probe — probable self-deadlock")
	}

	// Restore the probe's privilege and retry on the very same *Adapter: the earlier failure
	// must not have left the primary connEntry's lock permanently held.
	mustExec(t, side, `GRANT EXECUTE ON FUNCTION pg_catalog.current_setting(text) TO PUBLIC`)

	retryDone := make(chan error, 1)
	go func() {
		_, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-probe-fail-retry"))
		retryDone <- err
	}()
	select {
	case err := <-retryDone:
		if err != nil {
			t.Fatalf("retry Connect after a probe failure: %v", err)
		}
		t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	case <-time.After(5 * time.Second):
		t.Fatal("retry Connect did not return within 5s — entry left locked after the probe failure")
	}
}

// A fields-mode connection with no `database` (Validate, input.go, never requires one for
// a non-file kind) must still connect — before client.go's buildConfig defaulted the primary
// connection's database, an empty `database` left the Postgres wire protocol's own "database"
// startup parameter unset, which the server defaults to the connecting *user* name (not something
// this app ever chose) — a real least-privilege role's own name essentially never matches an
// existing database, so this failed every such connection with FATAL 3D000 "database \"<user>\"
// does not exist", surfacing as a plain E_QUERY that read exactly like the user-reported "fails to
// authenticate".
func TestPostgres_ConnectWithNoDatabaseDefaultsToMaintenanceDB(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := newAdapter(t)

	side, err := pgx.Connect(context.Background(), fixture.URI)
	if err != nil {
		t.Fatalf("side connect: %v", err)
	}
	// Registered first, so its LIFO position is last: the DROP ROLE cleanup below must run
	// against a still-open connection — a plain `defer side.Close(...)` would close it before
	// t.Cleanup callbacks even start (authmatrix_test.go's own pattern, mustExec included).
	t.Cleanup(func() { _ = side.Close(context.Background()) })
	mustExec(t, side, `CREATE ROLE app_user LOGIN PASSWORD 'app_pw'`)
	t.Cleanup(func() {
		// DROP ROLE fails while any privilege is still granted to it (here, just the
		// database-level CONNECT below) — GRANT CONNECT leaves a pg_shdepend entry, so a
		// swallowed failure here left the role behind for good on a rerun (`go test -count=2`),
		// since CREATE ROLE above has no IF NOT EXISTS (authmatrix_test.go's own pattern).
		mustExec(t, side, `DROP OWNED BY app_user`)
		mustExec(t, side, `REVOKE CONNECT ON DATABASE kira_test FROM app_user`)
		mustExec(t, side, `DROP ROLE IF EXISTS app_user`)
	})
	mustExec(t, side, `GRANT CONNECT ON DATABASE kira_test TO app_user`)

	cfg := fixture.Config
	cfg.Database = nil // exactly what an empty "Database" field in the dialog sends
	user, pw := "app_user", "app_pw"
	cfg.Username, cfg.Password = &user, &pw

	info, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-2a"))
	if err != nil {
		t.Fatalf("Connect with no database: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	if info.Details["database"] != "postgres" {
		t.Errorf("connected database = %q, want the maintenance db \"postgres\"", info.Details["database"])
	}
}

// 3. tree enumeration
func TestPostgres_TreeEnumeration(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	dbs, err := a.Children(ctx, nodePath(fixture), adapters.NewOpCtx("op-3a"))
	if err != nil {
		t.Fatalf("Children(root): %v", err)
	}
	dbNames := childNames(t, dbs)
	if !containsName(dbNames, "kira_test") {
		t.Errorf("databases = %v, want kira_test", dbNames)
	}
	for _, n := range dbs.Nodes {
		if n.Name == "kira_test" && (n.Detail == nil || *n.Detail != "connected") {
			t.Errorf("kira_test detail = %v, want \"connected\"", n.Detail)
		}
	}

	schemas, err := a.Children(ctx, nodePath(fixture, seg("database", "kira_test")), adapters.NewOpCtx("op-3b"))
	if err != nil {
		t.Fatalf("Children(database): %v", err)
	}
	schemaNames := childNames(t, schemas)
	if !containsName(schemaNames, "app") || !containsName(schemaNames, "analytics") {
		t.Errorf("schemas = %v, want app and analytics (and no pg_catalog/information_schema/public)", schemaNames)
	}
	for _, forbidden := range []string{"pg_catalog", "information_schema", "public"} {
		if containsName(schemaNames, forbidden) {
			t.Errorf("schemas = %v, must not include %q", schemaNames, forbidden)
		}
	}

	objects, err := a.Children(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "app")), adapters.NewOpCtx("op-3c"))
	if err != nil {
		t.Fatalf("Children(schema): %v", err)
	}
	kindOf := map[string]string{}
	for _, n := range objects.Nodes {
		kindOf[n.Name] = n.Kind
	}
	wantKinds := map[string]string{
		"wide_table": "table", "order_summary": "view", "customer_totals": "matview",
		"invoice_number_seq": "sequence", "full_name": "function",
	}
	for name, wantKind := range wantKinds {
		if kindOf[name] != wantKind {
			t.Errorf("object %q kind = %q, want %q", name, kindOf[name], wantKind)
		}
	}
	for _, n := range objects.Nodes {
		if n.HasChildren {
			t.Errorf("object %q HasChildren = true, want false (P19 D5: every relation is a leaf)", n.Name)
		}
	}
}

// 4. quoting — a table and a column whose names need quoting/escaping.
func TestPostgres_Quoting(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	p, err := a.Read(ctx, adapters.ReadRequest{
		Path:     nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", `weird"name`)),
		PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-4"))
	if err != nil {
		t.Fatalf("Read(weird\"name): %v", err)
	}
	tp := p.(page.TabularPage)
	if tp.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1", tp.RowCount)
	}
}

// 5. describe
func TestPostgres_Describe(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	meta, err := a.Describe(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "order_items")), adapters.NewOpCtx("op-5"))
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(meta.Indexes) < 2 {
		t.Errorf("Indexes = %d, want >= 2 (the PK plus order_items_order_product_idx)", len(meta.Indexes))
	}
	if len(meta.PrimaryKey) == 0 {
		t.Error("expected a primary key")
	}
}

// P22c §4.1: SchemaColumns is Describe's schema-wide sibling — every relation in one container
// together with its columns, in one round trip, byte-identical to what Describe reports.
func TestPostgres_SchemaColumns(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	relations, err := a.SchemaColumns(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "app")), adapters.NewOpCtx("op-schema-columns-1"))
	if err != nil {
		t.Fatalf("SchemaColumns: %v", err)
	}
	byName := make(map[string]model.RelationColumns, len(relations))
	for _, rc := range relations {
		byName[rc.Name] = rc
	}
	orderItems, ok := byName["order_items"]
	if !ok || orderItems.Kind != "table" {
		t.Fatalf("relations = %v, want to find order_items as a table", relations)
	}
	summary, ok := byName["order_summary"]
	if !ok || summary.Kind != "view" {
		t.Fatalf("relations = %v, want to find order_summary as a view", relations)
	}
	if len(summary.Columns) == 0 {
		t.Error("order_summary (a view) has no columns")
	}
	for i, col := range orderItems.Columns {
		if col.Position != i+1 {
			t.Errorf("order_items.Columns[%d].Position = %d, want ordinal %d", i, col.Position, i+1)
		}
	}
	var sawPK bool
	for _, col := range orderItems.Columns {
		if col.IsPrimaryKey {
			sawPK = true
		}
	}
	if !sawPK {
		t.Error("order_items has no column reported as a primary key")
	}

	// Every column's DataType/IsPrimaryKey must be byte-identical to what Describe reports for the
	// same column — the property D6's shared supply depends on.
	meta, err := a.Describe(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "order_items")), adapters.NewOpCtx("op-schema-columns-2"))
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	describeByName := make(map[string]model.ColumnMeta, len(meta.Columns))
	for _, c := range meta.Columns {
		describeByName[c.Name] = c
	}
	for _, c := range orderItems.Columns {
		want, ok := describeByName[c.Name]
		if !ok {
			t.Errorf("SchemaColumns column %q not found in Describe's own column list", c.Name)
			continue
		}
		if c.DataType != want.DataType {
			t.Errorf("column %q: SchemaColumns DataType = %q, Describe DataType = %q", c.Name, c.DataType, want.DataType)
		}
		if c.IsPrimaryKey != want.IsPrimaryKey {
			t.Errorf("column %q: SchemaColumns IsPrimaryKey = %v, Describe IsPrimaryKey = %v", c.Name, c.IsPrimaryKey, want.IsPrimaryKey)
		}
	}

	// An empty container (public — nothing is ever created there by the seed) returns an empty
	// slice, not an error.
	empty, err := a.SchemaColumns(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "public")), adapters.NewOpCtx("op-schema-columns-3"))
	if err != nil {
		t.Fatalf("SchemaColumns(public): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("SchemaColumns(public) = %v, want an empty slice", empty)
	}
}

// 6. row estimate: never-analysed relations surface rowEstimate: nil, never the raw -1.
func TestPostgres_RowEstimateNeverAnalysed(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	meta, err := a.Describe(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "order_items")), adapters.NewOpCtx("op-6"))
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if meta.RowEstimate != nil {
		t.Errorf("RowEstimate = %v, want nil for a never-analysed table", *meta.RowEstimate)
	}

	objects, err := a.Children(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "app")), adapters.NewOpCtx("op-6b"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	for _, n := range objects.Nodes {
		if n.Name == "order_items" && n.Detail != nil {
			t.Errorf("order_items detail = %q, want nil (never analysed)", *n.Detail)
		}
		if n.Name == "big_rows" && (n.Detail == nil || !bigRowsDetail.MatchString(*n.Detail)) {
			t.Errorf("big_rows detail = %v, want a \"~N\" estimate", n.Detail)
		}
	}
}

// P328: a declarative partition lists under its partitioned table, not at schema level.
func TestPostgres_PartitionsNestUnderParent(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	side, err := pgx.Connect(ctx, fixture.URI)
	if err != nil {
		t.Fatalf("side connect: %v", err)
	}
	defer side.Close(context.Background())
	// TestPostgres_TreeEnumeration enumerates the schema set; a leaked schema must not break it.
	t.Cleanup(func() { _, _ = side.Exec(context.Background(), "DROP SCHEMA IF EXISTS p328 CASCADE") })
	for _, stmt := range []string{
		"CREATE SCHEMA p328",
		"CREATE TABLE p328.events (id int, at date) PARTITION BY RANGE (at)",
		"CREATE TABLE p328.events_2024 PARTITION OF p328.events FOR VALUES FROM ('2024-01-01') TO ('2025-01-01')",
		"CREATE TABLE p328.events_2025 PARTITION OF p328.events FOR VALUES FROM ('2025-01-01') TO ('2026-01-01')",
		"CREATE TABLE p328.plain (id int)",
	} {
		if _, err := side.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	schema, err := a.Children(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "p328")), adapters.NewOpCtx("op-p328-1"))
	if err != nil {
		t.Fatalf("Children(schema): %v", err)
	}
	if got := childNames(t, schema); !reflect.DeepEqual(got, []string{"events", "plain"}) {
		t.Fatalf("schema children = %v, want [events plain] (no partitions)", got)
	}
	for _, n := range schema.Nodes {
		if want := n.Name == "events"; n.HasChildren != want {
			t.Errorf("%s HasChildren = %v, want %v", n.Name, n.HasChildren, want)
		}
	}

	parts, err := a.Children(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "p328"), seg("table", "events")), adapters.NewOpCtx("op-p328-2"))
	if err != nil {
		t.Fatalf("Children(events): %v", err)
	}
	if got := childNames(t, parts); !reflect.DeepEqual(got, []string{"events_2024", "events_2025"}) {
		t.Fatalf("events children = %v, want both partitions in name order", got)
	}
	for _, n := range parts.Nodes {
		if want := "database:kira_test/schema:p328/table:" + n.Name; n.Path != want {
			t.Errorf("partition path = %q, want %q", n.Path, want)
		}
	}

	plain, err := a.Children(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "p328"), seg("table", "plain")), adapters.NewOpCtx("op-p328-3"))
	if err != nil {
		t.Fatalf("Children(plain): %v", err)
	}
	if len(plain.Nodes) != 0 {
		t.Errorf("plain table children = %v, want []", childNames(t, plain))
	}
}

// 7. cancel, asserted server-side.
func TestPostgres_Cancel(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)

	ctx, cancel := context.WithCancel(context.Background())
	op := adapters.NewOpCtx("op-cancel")

	errCh := make(chan error, 1)
	go func() {
		_, err := a.Execute(ctx, model.ConsoleRequest{
			Path: nodePath(fixture, seg("database", "kira_test")), Statements: []string{"SELECT pg_sleep(30)"},
		}, op)
		errCh <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if op.Command() != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel() // unblocks the local wait

	ok, err := a.Cancel(context.Background(), "op-cancel")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !ok {
		t.Error("Cancel reported false, want true (a real running query)")
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected Execute to fail once cancelled")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Execute never returned after Cancel")
	}

	// A second cancel of the same (now-finished) op is a no-op.
	ok2, err2 := a.Cancel(context.Background(), "op-cancel")
	if err2 != nil || ok2 {
		t.Errorf("second Cancel = %v, %v, want false, nil", ok2, err2)
	}
}

// 8. cap honesty
func TestPostgres_CapHonesty(t *testing.T) {
	a := newAdapter(t)
	c := a.Caps()
	if !c.Tabular || c.Documents || c.KeyValue || c.Stream {
		t.Errorf("caps = %+v, want tabular-only", c)
	}
	if c.Pagination != adapters.PaginationKeyset {
		t.Errorf("Pagination = %q, want keyset", c.Pagination)
	}
	if !c.Writable || !c.Transactions || !c.Cancel {
		t.Error("expected Writable, Transactions and Cancel all true")
	}
	if c.FileTransfer {
		t.Error("expected FileTransfer false")
	}
	if !c.SchemaColumns {
		t.Error("expected SchemaColumns true")
	}
}

// 10. read: first page
func TestPostgres_ReadFirstPage(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path:     nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers")),
		PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-10"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	tp := p.(page.TabularPage)
	if tp.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2", tp.RowCount)
	}
	if tp.Position.HasMore {
		t.Error("HasMore = true, want false (only 2 rows, page size 10)")
	}
}

// 11. read: deep page by offset
func TestPostgres_ReadOffsetPage(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path:     nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "big_rows")),
		Sort:     &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}},
		PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 500_000},
	}, adapters.NewOpCtx("op-11"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	tp := p.(page.TabularPage)
	if tp.RowCount != 10 {
		t.Fatalf("RowCount = %d, want 10", tp.RowCount)
	}
	first := cellAt(t, tp, 0, 0)
	if first == nil || *first != "500001" {
		t.Errorf("first id = %v, want 500001", first)
	}
}

// 12. read: keyset forward and backward
func TestPostgres_ReadKeysetForwardBackward(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()
	tablePath := nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "big_rows"))
	sort := &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}}

	first, err := a.Read(ctx, adapters.ReadRequest{
		Path: tablePath, Sort: sort, PageSize: 5, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-12a"))
	if err != nil {
		t.Fatalf("Read(first): %v", err)
	}
	firstPage := first.(page.TabularPage)
	if firstPage.Position.NextToken == nil {
		t.Fatal("expected a NextToken on the first page")
	}

	second, err := a.Read(ctx, adapters.ReadRequest{
		Path: tablePath, Sort: sort, PageSize: 5, Cursor: model.PageCursor{Mode: "after", Token: *firstPage.Position.NextToken},
	}, adapters.NewOpCtx("op-12b"))
	if err != nil {
		t.Fatalf("Read(after): %v", err)
	}
	secondPage := second.(page.TabularPage)
	secondFirstID := cellAt(t, secondPage, 0, 0)
	if secondFirstID == nil || *secondFirstID != "6" {
		t.Errorf("second page's first id = %v, want 6", secondFirstID)
	}
	if secondPage.Position.PrevToken == nil {
		t.Fatal("expected a PrevToken on the second page")
	}

	back, err := a.Read(ctx, adapters.ReadRequest{
		Path: tablePath, Sort: sort, PageSize: 5, Cursor: model.PageCursor{Mode: "before", Token: *secondPage.Position.PrevToken},
	}, adapters.NewOpCtx("op-12c"))
	if err != nil {
		t.Fatalf("Read(before): %v", err)
	}
	backPage := back.(page.TabularPage)
	backFirstID := cellAt(t, backPage, 0, 0)
	if backFirstID == nil || *backFirstID != "1" {
		t.Errorf("page-before's first id = %v, want 1 (display order preserved)", backFirstID)
	}

	// A "before" page is fetched in descending (fetch) order and reversed to
	// ascending (display) order — the streaming readPage must swap its tracked first/last raw rows
	// along with builder.Reverse(), or this page's own NextToken (built from
	// CellAt(rowCount-1,...), the last *displayed* row) would be built from the wrong row's keyset
	// value entirely.
	if backPage.Position.NextToken == nil {
		t.Fatal("expected a NextToken on the page-before result")
	}
	forwardAgain, err := a.Read(ctx, adapters.ReadRequest{
		Path: tablePath, Sort: sort, PageSize: 5, Cursor: model.PageCursor{Mode: "after", Token: *backPage.Position.NextToken},
	}, adapters.NewOpCtx("op-12d"))
	if err != nil {
		t.Fatalf("Read(after, from page-before's NextToken): %v", err)
	}
	forwardAgainPage := forwardAgain.(page.TabularPage)
	forwardAgainFirstID := cellAt(t, forwardAgainPage, 0, 0)
	if forwardAgainFirstID == nil || *forwardAgainFirstID != "6" {
		t.Errorf("page after page-before's NextToken: first id = %v, want 6", forwardAgainFirstID)
	}
}

// P168 Part 3 F10: an identity column's own sequence is recreated by the identity clause, so the
// definition must not also emit a standalone CREATE SEQUENCE for it (a serial column still does).
func TestPostgres_DefinitionIdentityColumnHasNoStraySequence(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()
	dbPath := nodePath(fixture, seg("database", "kira_test"))
	if _, err := a.Execute(ctx, model.ConsoleRequest{Path: dbPath, Statements: []string{
		"DROP TABLE IF EXISTS app.p168_ident",
		"DROP TABLE IF EXISTS app.p168_serial",
		"CREATE TABLE app.p168_ident (id int GENERATED ALWAYS AS IDENTITY PRIMARY KEY, v int)",
		"CREATE TABLE app.p168_serial (id serial PRIMARY KEY, v int)",
	}}, adapters.NewOpCtx("op-ident-setup")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer a.Execute(ctx, model.ConsoleRequest{Path: dbPath, Statements: []string{"DROP TABLE app.p168_ident", "DROP TABLE app.p168_serial"}}, adapters.NewOpCtx("op-ident-cleanup")) //nolint:errcheck

	definition := func(table string) string {
		def, err := a.Definition(ctx, nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", table)), adapters.NewOpCtx("op-def-"+table))
		if err != nil {
			t.Fatalf("Definition(%s): %v", table, err)
		}
		return strings.Join(def.Statements, "\n")
	}
	if got := definition("p168_ident"); strings.Contains(got, "CREATE SEQUENCE") || strings.Contains(got, "OWNED BY") {
		t.Errorf("identity table definition has a stray sequence:\n%s", got)
	}
	if got := definition("p168_serial"); !strings.Contains(got, "CREATE SEQUENCE") {
		t.Errorf("serial table definition lost its CREATE SEQUENCE:\n%s", got)
	}
}

func TestPostgres_ConnectionLifecycle(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	testsupport.ConnectionLifecycleScenarios(t, fixture.Config, newAdapter)
}

// P168 Part 3 F4: a binary key's token must bind as raw bytes, not its 0x<hex> display text.
func TestPostgres_ReadKeysetBinaryPrimaryKey(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()
	dbPath := nodePath(fixture, seg("database", "kira_test"))
	if _, err := a.Execute(ctx, model.ConsoleRequest{
		Path: dbPath,
		Statements: []string{
			"DROP TABLE IF EXISTS app.p168_kbin",
			"CREATE TABLE app.p168_kbin (id bytea PRIMARY KEY)",
			`INSERT INTO app.p168_kbin VALUES ('\x01'), ('\x0A'), ('\x30'), ('\x7F'), ('\x80'), ('\x81FF'), ('\xFF'), ('\xFFFF')`,
		},
	}, adapters.NewOpCtx("op-kbin-setup")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer a.Execute(ctx, model.ConsoleRequest{Path: dbPath, Statements: []string{"DROP TABLE app.p168_kbin"}}, adapters.NewOpCtx("op-kbin-cleanup")) //nolint:errcheck
	sort := &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}}
	got := testsupport.KeysetForwardIDs(t, a, nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "p168_kbin")), sort, 2, 8)
	want := []string{"0x01", "0x0a", "0x30", "0x7f", "0x80", "0x81ff", "0xff", "0xffff"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
}

// Sorting by a nullable column must never be granted keyset pagination, even
// when a non-nullable tiebreaker (id) is available — customers.region_id has no NOT NULL
// constraint (packages/db-fixtures/fixtures/0001_seed.sql), so a naive grant here would risk either
// dropping any future NULL-region customer from every keyset page forever, or hard-failing a page
// whose boundary row has a NULL region_id. Falling back to offset pagination sidesteps both failure
// modes; RowCount must still see every customer, not just the ones with a non-NULL region_id.
func TestPostgres_ReadSortByNullableColumnFallsBackToOffset(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	tablePath := nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers"))
	sort := &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "region_id", Direction: "asc"}}}

	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: tablePath, Sort: sort, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-13-nullable-sort"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	tp := p.(page.TabularPage)
	if tp.Position.Strategy != "offset" {
		t.Errorf("Position.Strategy = %q, want offset (region_id is nullable)", tp.Position.Strategy)
	}
	if tp.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2 (no row dropped by a bad keyset comparison)", tp.RowCount)
	}
}

// 14. read: projection
func TestPostgres_ReadProjection(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path:       nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers")),
		Projection: []string{"name"},
		PageSize:   10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-14"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	tp := p.(page.TabularPage)
	if len(tp.Columns) != 1 || tp.Columns[0].Name != "name" {
		t.Errorf("Columns = %+v, want exactly [name]", tp.Columns)
	}
}

// 15. read: filter and sort
func TestPostgres_ReadFilterAndSort(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	filter := "region_id = 1"
	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path:   nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers")),
		Filter: &filter, Sort: &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "name", Direction: "asc"}}},
		PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-15"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	tp := p.(page.TabularPage)
	if tp.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1 (only Acme Co is in region 1)", tp.RowCount)
	}
}

// 17. count
func TestPostgres_Count(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	result, err := a.Count(context.Background(), adapters.CountRequest{
		Path: nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers")),
	}, adapters.NewOpCtx("op-17"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if result.Value != 2 || !result.Exact {
		t.Errorf("Count = %+v, want {2, true}", result)
	}
}

// 18. read cannot write — Preview/Mutate on a read-only connection is E_UNSUPPORTED.
func TestPostgres_ReadOnlyConnectionCannotWrite(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	roCfg := fixture.Config
	roCfg.ReadOnly = true
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), roCfg, adapters.NewOpCtx("op-18-connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer a.Disconnect(context.Background())

	plan := model.MutationPlan{
		Path: nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers")),
		Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "id", Value: strp("1")}}}},
	}
	_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-18"))
	if err == nil {
		t.Fatal("expected an error mutating a read-only connection")
	}
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
		t.Fatalf("got %v, want E_UNSUPPORTED", err)
	}
}

// client.go's own SET default_transaction_read_only=on only governs the *next*
// transaction, not the current one — a console statement can flip it back (or, on Postgres
// specifically, use SET TRANSACTION READ WRITE to flip the current transaction's own mode) and
// have a later statement in the same batch run writable. Confirmed against a real server before
// fixing this: this exact statement sequence deleted every row. execute() now wraps the whole
// batch in BEGIN READ ONLY and rejects any statement containing "READ WRITE" outright.
func TestPostgres_ReadOnlyConnectionExecuteCannotEscapeReadOnlyTransaction(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	roCfg := fixture.Config
	roCfg.ReadOnly = true
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), roCfg, adapters.NewOpCtx("op-ro-connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer a.Disconnect(context.Background())

	// Targets order_items rather than customers: customers is referenced by orders' own FK, so a
	// DELETE against it fails with a foreign-key error regardless of read-only enforcement — a
	// false pass that doesn't exercise the guard. order_items has no dependents, so a
	// DELETE against it only fails if the read-only guard itself is doing its job.
	before, err := a.Count(context.Background(), adapters.CountRequest{
		Path: nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "order_items")),
	}, adapters.NewOpCtx("op-ro-count-before"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	attempts := [][]string{
		{"SET default_transaction_read_only = off", "DELETE FROM app.order_items"},
		{"SET TRANSACTION READ WRITE", "DELETE FROM app.order_items"},
		{"COMMIT", "BEGIN", "DELETE FROM app.order_items"},
		// P26 §3.4(3): a DDL statement takes a different server-side path than DELETE — untested
		// before this.
		{"CREATE TABLE app.p26_ro_escape (id int)"},
		// Review finding, case 1: naming the GUC directly (no "READ WRITE" phrase anywhere) —
		// confirmed against a real server before the fix that this deleted every row.
		{"SET transaction_read_only = off", "DELETE FROM app.order_items"},
		// The batch's own COMMIT ends the wrapping read-only transaction,
		// after which SET SESSION took effect immediately for the plain-autocommit write that
		// followed — confirmed not to escape on postgres specifically (see errors.go's
		// endsTransaction doc comment), but rejected unconditionally regardless of engine.
		{"COMMIT", "SET SESSION transaction_read_only = OFF", "DELETE FROM app.order_items"},
		// Review finding, case 3: a nested block comment — confirmed against a real server that
		// `READ /* /* */ x */ WRITE` parses identically to `READ WRITE`, which the old non-nesting
		// regexp missed.
		{"SET TRANSACTION READ /* /* */ x */ WRITE", "DELETE FROM app.order_items"},
		// P168 Part 3 F1: spellings the text screen cannot name — boolean prefixes, quoted names,
		// ABORT then RESET, set_config() — are caught by the per-statement server-state check.
		{"SET transaction_read_only = no", "DELETE FROM app.order_items"},
		{`SET "transaction_read_only" = off`, "DELETE FROM app.order_items"},
		{"ABORT", "RESET default_transaction_read_only", "DELETE FROM app.order_items"},
		{"SELECT set_config('transaction_read_only', 'off', false)", "DELETE FROM app.order_items"},
		// Resets to the startup default (writable) without any falsy spelling for a screen to match.
		{"RESET default_transaction_read_only", "SELECT 1"},
		{"RESET ALL", "SELECT 1"},
		{"SET default_transaction_read_only TO DEFAULT", "SELECT 1"},
		{"SELECT set_config('default_transaction_read_only', 'off', false)", "DELETE FROM app.order_items"},
	}
	for _, statements := range attempts {
		_, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path:       nodePath(fixture, seg("database", "kira_test")),
			Statements: statements,
		}, adapters.NewOpCtx("op-ro-escape"))
		if err == nil {
			t.Fatalf("Execute(%v) succeeded on a read-only connection, want an error", statements)
		}
	}

	after, err := a.Count(context.Background(), adapters.CountRequest{
		Path: nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "order_items")),
	}, adapters.NewOpCtx("op-ro-count-after"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if after.Value != before.Value {
		t.Fatalf("order_items row count = %d after read-only Execute attempts, want unchanged %d", after.Value, before.Value)
	}

	side, err := pgx.Connect(context.Background(), fixture.URI)
	if err != nil {
		t.Fatalf("side connect: %v", err)
	}
	defer side.Close(context.Background())
	var exists bool
	if err := side.QueryRow(context.Background(),
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'app' AND table_name = 'p26_ro_escape')").Scan(&exists); err != nil {
		t.Fatalf("information_schema check: %v", err)
	}
	if exists {
		t.Error("app.p26_ro_escape exists after a read-only Execute attempt, want it never created")
	}

	// The wrapping transaction must actually end after each attempt — a genuine, ordinary
	// statement on the same connection right after must still work.
	if _, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path:       nodePath(fixture, seg("database", "kira_test")),
		Statements: []string{"SELECT 1"},
	}, adapters.NewOpCtx("op-ro-still-usable")); err != nil {
		t.Fatalf("Execute(SELECT 1) after failed escape attempts: %v", err)
	}

	// The session default must be read-only again after every attempt (F1: ABORT + RESET and
	// set_config() otherwise left the pinned connection writable for every later op).
	pages, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path:       nodePath(fixture, seg("database", "kira_test")),
		Statements: []string{"SHOW default_transaction_read_only"},
	}, adapters.NewOpCtx("op-ro-default"))
	if err != nil {
		t.Fatalf("Execute(SHOW): %v", err)
	}
	if got := page.CellText(pages[0].(page.TabularPage).Chunks[0], 0); got != "on" {
		t.Fatalf("default_transaction_read_only = %q after escape attempts, want on", got)
	}
}

// 21. preview: exact text, never executes.
func TestPostgres_PreviewNeverExecutes(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	plan := model.MutationPlan{
		Path: nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "composite_pk")),
		Ops: []model.MutationRowOp{{
			Kind: "insert", Values: model.RowValues{
				{Name: "tenant_id", Value: strp("3")}, {Name: "entity_id", Value: strp("1")}, {Name: "name", Value: strp("new tenant")},
			},
		}},
	}
	statements, err := a.Preview(plan)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(statements) != 1 {
		t.Fatalf("statements = %v, want exactly 1", statements)
	}
	want := `INSERT INTO "app"."composite_pk" ("tenant_id", "entity_id", "name") VALUES ('3', '1', 'new tenant')`
	if statements[0] != want {
		t.Errorf("got %q, want %q", statements[0], want)
	}

	count, err := a.Count(context.Background(), adapters.CountRequest{Path: plan.Path}, adapters.NewOpCtx("op-21-count"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count.Value != 3 {
		t.Errorf("Count after Preview = %d, want unchanged at 3 (preview must never execute)", count.Value)
	}
}

// 22. mutate: update lands correctly (affectedRows).
func TestPostgres_MutateUpdate(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	plan := model.MutationPlan{
		Path: nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers")),
		Ops: []model.MutationRowOp{{
			Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}},
			Changes: model.RowValues{{Name: "name", Value: strp("Acme Corp")}},
		}},
	}
	result, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-22"))
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if result.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
	}
}

// P26 §3.4(1): the phase's flagship test for this adapter — does an object DDL created become
// visible to Children/Describe on the same connection, the one Postgres ✗ that isn't a write.
func TestPostgres_ExecuteDDLRoundTrip(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()
	schemaPath := nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"))
	tablePath := nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "p26_scratch"))

	side, err := pgx.Connect(ctx, fixture.URI)
	if err != nil {
		t.Fatalf("side connect: %v", err)
	}
	defer side.Close(context.Background())
	// TestPostgres_TreeEnumeration (:175) enumerates app's own child set — a leaked scratch table
	// from a failed run must not break it.
	t.Cleanup(func() { _, _ = side.Exec(context.Background(), "DROP TABLE IF EXISTS app.p26_scratch") })

	if _, err := a.Execute(ctx, model.ConsoleRequest{
		Path:       schemaPath,
		Statements: []string{"CREATE TABLE app.p26_scratch (id serial PRIMARY KEY, name text, region_id int)"},
	}, adapters.NewOpCtx("op-ddl-1")); err != nil {
		t.Fatalf("Execute(CREATE TABLE): %v", err)
	}

	children, err := a.Children(ctx, schemaPath, adapters.NewOpCtx("op-ddl-2"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if !containsName(childNames(t, children), "p26_scratch") {
		t.Fatalf("Children(app) = %v, want p26_scratch present", childNames(t, children))
	}

	meta, err := a.Describe(ctx, tablePath, adapters.NewOpCtx("op-ddl-3"))
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(meta.Columns) != 3 {
		t.Fatalf("Columns = %+v, want exactly 3 (id, name, region_id)", meta.Columns)
	}
	if len(meta.PrimaryKey) != 1 || meta.PrimaryKey[0] != "id" {
		t.Errorf("PrimaryKey = %v, want [id]", meta.PrimaryKey)
	}

	if _, err := a.Execute(ctx, model.ConsoleRequest{
		Path:       schemaPath,
		Statements: []string{"ALTER TABLE app.p26_scratch ADD COLUMN note text"},
	}, adapters.NewOpCtx("op-ddl-4")); err != nil {
		t.Fatalf("Execute(ALTER TABLE): %v", err)
	}
	meta2, err := a.Describe(ctx, tablePath, adapters.NewOpCtx("op-ddl-5"))
	if err != nil {
		t.Fatalf("Describe after ALTER: %v", err)
	}
	if len(meta2.Columns) != 4 {
		t.Fatalf("Columns after ALTER = %+v, want exactly 4", meta2.Columns)
	}

	if _, err := a.Execute(ctx, model.ConsoleRequest{
		Path:       schemaPath,
		Statements: []string{"DROP TABLE app.p26_scratch"},
	}, adapters.NewOpCtx("op-ddl-6")); err != nil {
		t.Fatalf("Execute(DROP TABLE): %v", err)
	}
	childrenAfter, err := a.Children(ctx, schemaPath, adapters.NewOpCtx("op-ddl-7"))
	if err != nil {
		t.Fatalf("Children after DROP: %v", err)
	}
	if containsName(childNames(t, childrenAfter), "p26_scratch") {
		t.Errorf("Children(app) after DROP = %v, must not contain p26_scratch", childNames(t, childrenAfter))
	}
}

// P26 §3.4(2): the only ✗ in Postgres's own §1.3 row — a delete that actually lands.
func TestPostgres_MutateDeleteRemovesTheRow(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()
	tablePath := nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "p26_delete"))

	side, err := pgx.Connect(ctx, fixture.URI)
	if err != nil {
		t.Fatalf("side connect: %v", err)
	}
	// Registered first, so its LIFO position is last: the DROP TABLE cleanup below must run
	// against a still-open connection — a plain `defer side.Close(...)` would close it before
	// t.Cleanup callbacks even start (authmatrix_test.go's own pattern, mustExec included).
	t.Cleanup(func() { _ = side.Close(context.Background()) })
	if _, err := side.Exec(ctx, "CREATE TABLE IF NOT EXISTS app.p26_delete (id int PRIMARY KEY, name text)"); err != nil {
		t.Fatalf("create scratch table: %v", err)
	}
	t.Cleanup(func() { _, _ = side.Exec(context.Background(), "DROP TABLE IF EXISTS app.p26_delete") })

	insertPlan := model.MutationPlan{
		Path: tablePath,
		Ops: []model.MutationRowOp{
			{Kind: "insert", Values: model.RowValues{{Name: "id", Value: strp("1")}, {Name: "name", Value: strp("a")}}},
			{Kind: "insert", Values: model.RowValues{{Name: "id", Value: strp("2")}, {Name: "name", Value: strp("b")}}},
			{Kind: "insert", Values: model.RowValues{{Name: "id", Value: strp("3")}, {Name: "name", Value: strp("c")}}},
		},
	}
	if _, err := a.Mutate(ctx, insertPlan, adapters.NewOpCtx("op-del-1")); err != nil {
		t.Fatalf("Mutate(insert): %v", err)
	}
	before, err := a.Count(ctx, adapters.CountRequest{Path: tablePath}, adapters.NewOpCtx("op-del-2"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if before.Value != 3 {
		t.Fatalf("Count after insert = %d, want 3", before.Value)
	}

	deletePlan := model.MutationPlan{
		Path: tablePath,
		Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "id", Value: strp("2")}}}},
	}
	result, err := a.Mutate(ctx, deletePlan, adapters.NewOpCtx("op-del-3"))
	if err != nil {
		t.Fatalf("Mutate(delete): %v", err)
	}
	if result.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
	}

	after, err := a.Count(ctx, adapters.CountRequest{Path: tablePath}, adapters.NewOpCtx("op-del-4"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if after.Value != 2 {
		t.Fatalf("Count after delete = %d, want 2", after.Value)
	}

	read, err := a.Read(ctx, adapters.ReadRequest{
		Path: tablePath, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-del-5"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	readPage := read.(page.TabularPage)
	if readPage.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2", readPage.RowCount)
	}
	for row := 0; row < readPage.RowCount; row++ {
		if id := cellAt(t, readPage, 0, row); id != nil && *id == "2" {
			t.Error("row with id=2 is still present after delete")
		}
	}
}

// pgx.Conn is not safe for concurrent use, but nothing above the adapter serializes ops
// against the same connection — adapterhost dispatches every inbound frame on its own goroutine.
// Two goroutines hammering one Adapter with real Reads and Mutates is exactly the scenario
// ConnSet.Acquire's own per-connection lock exists to serialize; `go test -race` (this repo's own
// bar for concurrency fixes, not plain `go test`) is what proves it — a passing functional
// assertion alone wouldn't catch a data race that merely got lucky this run.
func TestPostgres_ConcurrentOpsOnOneConnectionAreSerialized(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	tablePath := nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers"))
	const goroutines = 8
	const rounds = 20

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*rounds)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				opID := "op-concurrent-" + strconv.Itoa(g) + "-" + strconv.Itoa(r)
				if r%2 == 0 {
					plan := model.MutationPlan{
						Path: tablePath,
						Ops: []model.MutationRowOp{{
							Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("2")}},
							Changes: model.RowValues{{Name: "name", Value: strp("Globex " + strconv.Itoa(g) + "-" + strconv.Itoa(r))}},
						}},
					}
					result, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx(opID))
					if err != nil {
						errs <- err
						continue
					}
					if result.AffectedRows != 1 {
						errs <- errors.New("mutate: AffectedRows = " + strconv.Itoa(result.AffectedRows) + ", want 1")
					}
					continue
				}
				p, err := a.Read(context.Background(), adapters.ReadRequest{
					Path: tablePath, Projection: []string{"id", "name"}, PageSize: 10,
					Filter: strp("id = 1"), Cursor: model.PageCursor{Mode: "offset", Offset: 0},
				}, adapters.NewOpCtx(opID))
				if err != nil {
					errs <- err
					continue
				}
				tp := p.(page.TabularPage)
				if tp.RowCount != 1 {
					errs <- errors.New("read: RowCount = " + strconv.Itoa(tp.RowCount) + ", want 1")
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent op failed: %v", err)
	}
}

// 23. mutate: unknown column is E_NOT_FOUND
func TestPostgres_MutateUnknownColumn(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	plan := model.MutationPlan{
		Path: nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers")),
		Ops: []model.MutationRowOp{{
			Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}},
			Changes: model.RowValues{{Name: "no_such_column", Value: strp("x")}},
		}},
	}
	_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-23"))
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeNotFound {
		t.Fatalf("got %v, want E_NOT_FOUND", err)
	}
}

// 25. mutate: a row-count conflict rolls back the whole batch.
func TestPostgres_MutateRowCountConflictRollsBack(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	tablePath := nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers"))
	plan := model.MutationPlan{
		Path: tablePath,
		Ops: []model.MutationRowOp{
			{Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}}, Changes: model.RowValues{{Name: "name", Value: strp("changed")}}},
			// A key matching zero rows — AssertAffectedExactlyOne must fail and roll back both.
			{Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("999999")}}, Changes: model.RowValues{{Name: "name", Value: strp("changed")}}},
		},
	}
	_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-25"))
	if err == nil {
		t.Fatal("expected an error for the zero-row update")
	}

	// The first update must have rolled back too.
	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: tablePath, Projection: []string{"name"}, PageSize: 10,
		Filter: strp("id = 1"), Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-25-verify"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	tp := p.(page.TabularPage)
	got := cellAt(t, tp, 0, 0)
	if got == nil || *got == "changed" {
		t.Errorf("name = %v, want the pre-mutation value (rollback failed)", got)
	}
}

// flippingCtx reports ctx.Err() as nil for the first `after` calls, then as context.Canceled for
// every call after that — Done() always returns nil (never fires), so RunWithAbortRace's own
// select (abort.go) never treats a call through this ctx as cancelled mid-flight; only the
// synchronous CheckNotStarted-style pre-checks (query.go's runCommand, catalog.go's execFor) ever
// observe the flip. This lets a test force a cancellation to land at an exact, deterministic point
// in mutate()'s own sequence of CheckNotStarted-gated calls (the catalog lookup inside
// getReadTarget, then BEGIN, then each compiled row op, then COMMIT) without any sleep or goroutine
// race — which of those calls is "BEGIN" isn't part of this package's exported surface, so a test
// sweeps every plausible index instead of hardcoding one.
type flippingCtx struct {
	context.Context
	calls *int32
	after int32
}

func (c flippingCtx) Err() error {
	if atomic.AddInt32(c.calls, 1) > c.after {
		return context.Canceled
	}
	return nil
}
func (c flippingCtx) Done() <-chan struct{} { return nil }

// A cancellation landing between BEGIN and COMMIT left neither COMMIT nor the compensating
// ROLLBACK ever reaching the server — both are gated by the same CheckNotStarted check COMMIT
// itself is, so an already-observed-as-cancelled ctx short-circuits ROLLBACK exactly like it does
// COMMIT. Because this adapter's *pgx.Conn is pinned for its whole lifetime (not per-op), the
// server session was then left "idle in transaction" indefinitely: the next Mutate's own BEGIN is a
// no-op inside that still-open transaction, so its COMMIT silently committed the earlier cancelled
// write too — worse than a hang. This sweeps every plausible cancellation point and, for whichever
// ones trigger (an error from Mutate), proves the transaction was rolled back: a
// clean follow-up Mutate on a different row must succeed quickly and must not also commit the
// earlier row's cancelled change.
func TestPostgres_MutateCancelledMidTransactionDoesNotLeakOpenTransaction(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	tablePath := nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "order_items"))

	readQuantity := func(id string) string {
		t.Helper()
		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path: tablePath, Projection: []string{"quantity"}, PageSize: 10,
			Filter: strp("id = " + id), Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-26-verify-"+id))
		if err != nil {
			t.Fatalf("Read(id=%s): %v", id, err)
		}
		got := cellAt(t, p.(page.TabularPage), 0, 0)
		if got == nil {
			t.Fatalf("Read(id=%s): no row", id)
		}
		return *got
	}
	baseline := readQuantity("1")

	for after := int32(1); after <= 12; after++ {
		plan := model.MutationPlan{
			Path: tablePath,
			Ops: []model.MutationRowOp{
				{Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}}, Changes: model.RowValues{{Name: "quantity", Value: strp("77")}}},
			},
		}
		var calls int32
		ctx := flippingCtx{Context: context.Background(), calls: &calls, after: after}
		_, err := a.Mutate(ctx, plan, adapters.NewOpCtx("op-26-cancel"))
		if err == nil {
			// This `after` fell past every CheckNotStarted check mutate() makes (COMMIT included) —
			// the mutate ran to completion normally. Reset row 1 and move on: nothing to prove here.
			if got := readQuantity("1"); got != "77" {
				t.Fatalf("after=%d: Mutate reported success but quantity = %s, want 77", after, got)
			}
			if _, err := a.Mutate(context.Background(), model.MutationPlan{
				Path: tablePath,
				Ops: []model.MutationRowOp{
					{Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}}, Changes: model.RowValues{{Name: "quantity", Value: strp(baseline)}}},
				},
			}, adapters.NewOpCtx("op-26-reset")); err != nil {
				t.Fatalf("after=%d: reset Mutate: %v", after, err)
			}
			continue
		}

		// A cancellation landed somewhere at or before COMMIT. The load-bearing assertion: a
		// completely unrelated follow-up Mutate, on a real ctx, must complete quickly rather than
		// hang behind a stale open transaction on the pinned connection...
		verifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err = a.Mutate(verifyCtx, model.MutationPlan{
			Path: tablePath,
			Ops: []model.MutationRowOp{
				{Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("2")}}, Changes: model.RowValues{{Name: "quantity", Value: strp("55")}}},
			},
		}, adapters.NewOpCtx("op-26-followup"))
		cancel()
		if err != nil {
			t.Fatalf("after=%d: follow-up Mutate on a different row: %v (the earlier cancellation left a stale open transaction)", after, err)
		}
		// ...and it must not silently commit row 1's own cancelled write along with it — proof the
		// cancelled transaction was actually rolled back, not left dangling for this commit to close.
		if got := readQuantity("1"); got != baseline {
			t.Fatalf("after=%d: row 1 quantity = %s after a cancelled Mutate + unrelated follow-up, want unchanged %s (the cancelled write leaked into the follow-up's commit)", after, got, baseline)
		}
		// Restore row 2 for the next iteration.
		if _, err := a.Mutate(context.Background(), model.MutationPlan{
			Path: tablePath,
			Ops: []model.MutationRowOp{
				{Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("2")}}, Changes: model.RowValues{{Name: "quantity", Value: strp("1")}}},
			},
		}, adapters.NewOpCtx("op-26-reset-2")); err != nil {
			t.Fatalf("after=%d: reset row 2: %v", after, err)
		}
	}
}

// 27. mutate: no primary key is E_UNSUPPORTED.
func TestPostgres_MutateNoPrimaryKey(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	probe, err := pgx.Connect(ctx, fixture.URI)
	if err != nil {
		t.Fatalf("probe connect: %v", err)
	}
	defer probe.Close(context.Background())
	if _, err := probe.Exec(ctx, "CREATE TABLE IF NOT EXISTS app.no_pk_probe (col text)"); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	defer probe.Exec(context.Background(), "DROP TABLE IF EXISTS app.no_pk_probe")

	plan := model.MutationPlan{
		Path: nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "no_pk_probe")),
		Ops: []model.MutationRowOp{{
			Kind: "update", Key: model.RowValues{{Name: "col", Value: strp("x")}},
			Changes: model.RowValues{{Name: "col", Value: strp("y")}},
		}},
	}
	_, err = a.Mutate(ctx, plan, adapters.NewOpCtx("op-27"))
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
		t.Fatalf("got %v, want E_UNSUPPORTED (no_pk_probe has no primary key)", err)
	}
}

// A bytea column's edited value arrives from the grid still spelled in the app's own
// "0x<hex>" display convention (read.go's own normalizeCellText) — Mutate must decode it back
// into raw bytes before binding it as a parameter, not hand pgx that ASCII text as if it were the
// new column content (byteain would then store it verbatim in Postgres's own escape format).
func TestPostgres_MutateBinaryColumnRoundTrips(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	probe, err := pgx.Connect(ctx, fixture.URI)
	if err != nil {
		t.Fatalf("probe connect: %v", err)
	}
	defer probe.Close(context.Background())
	if _, err := probe.Exec(ctx, "CREATE TABLE IF NOT EXISTS app.blob_rw (id int PRIMARY KEY, data bytea)"); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	defer probe.Exec(context.Background(), "DROP TABLE IF EXISTS app.blob_rw")
	if _, err := probe.Exec(ctx, "INSERT INTO app.blob_rw (id, data) VALUES (1, $1)", []byte{0x01, 0x02, 0x03}); err != nil {
		t.Fatalf("seed probe table: %v", err)
	}

	plan := model.MutationPlan{
		Path: nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "blob_rw")),
		Ops: []model.MutationRowOp{{
			Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}},
			Changes: model.RowValues{{Name: "data", Value: strp("0x0405")}},
		}},
	}
	result, err := a.Mutate(ctx, plan, adapters.NewOpCtx("op-blob-rw"))
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if result.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
	}

	var stored []byte
	if err := probe.QueryRow(ctx, "SELECT data FROM app.blob_rw WHERE id = 1").Scan(&stored); err != nil {
		t.Fatalf("probe read back: %v", err)
	}
	want := []byte{0x04, 0x05}
	if !bytes.Equal(stored, want) {
		t.Errorf("stored bytes = %#v, want %#v (got the display text instead of decoded bytes?)", stored, want)
	}

	// readPage's per-cell loop reuses the scanned *string directly for every
	// non-TypeClassBinary column and only allocates a fresh one for TypeClassBinary — get that
	// column-typeclass check backwards and a bytea cell would render as pgx's raw `\x0405` text
	// instead of the app's own `0x0405` convention.
	read, err := a.Read(ctx, adapters.ReadRequest{
		Path:   nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "blob_rw")),
		Cursor: model.PageCursor{Mode: "offset", Offset: 0}, PageSize: 10,
	}, adapters.NewOpCtx("op-blob-rw-read"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	readPage := read.(page.TabularPage)
	dataCell := cellAt(t, readPage, 1, 0)
	if dataCell == nil || *dataCell != "0x0405" {
		t.Errorf("data cell = %v, want 0x0405", dataCell)
	}
}

// The fake-data generator's own batch scale — this belongs under CLAUDE.md's conformance-suite
// exemption, not the general unit-test bar, because nothing else exercises a multi-row insert plan
// at anything beyond TestPostgres_MutateRowCountConflictRollsBack's two-op scale.
func TestPostgres_MutateBulkInsertPlanCommitsAtomically(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	tablePath := nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "customers"))

	// The container is memoized across this whole file (TestMain), so the 200 rows this test
	// inserts must not outlive it — TestPostgres_ExecuteOnePagePerStatement's own RowCount == 2
	// assertion depends on `customers` being back at its seeded size.
	t.Cleanup(func() {
		probe, err := pgx.Connect(context.Background(), fixture.URI)
		if err != nil {
			t.Logf("cleanup probe connect: %v", err)
			return
		}
		defer probe.Close(context.Background())
		if _, err := probe.Exec(context.Background(),
			"DELETE FROM app.customers WHERE name LIKE 'Fixture Customer %' OR name LIKE 'Should Roll Back %'",
		); err != nil {
			t.Logf("cleanup delete: %v", err)
		}
	})

	countCustomers := func(label string) int64 {
		t.Helper()
		result, err := a.Count(context.Background(), adapters.CountRequest{Path: tablePath}, adapters.NewOpCtx("op-29-count-"+label))
		if err != nil {
			t.Fatalf("Count(%s): %v", label, err)
		}
		return result.Value
	}
	before := countCustomers("before")

	// A 200-op insert plan omitting `id` (a serial PK) entirely — the proof that D4 rule 2's "skip
	// the serial PK" produces a plan a real server actually accepts, in one transaction
	// (Caps.Transactions == true for postgres, F2/F3).
	const rowCount = 200
	ops := make([]model.MutationRowOp, rowCount)
	for i := 0; i < rowCount; i++ {
		ops[i] = model.MutationRowOp{
			Kind:   "insert",
			Values: model.RowValues{{Name: "name", Value: strp("Fixture Customer " + strconv.Itoa(i))}},
		}
	}
	result, err := a.Mutate(context.Background(), model.MutationPlan{Path: tablePath, Ops: ops}, adapters.NewOpCtx("op-29-insert"))
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if result.AffectedRows != rowCount {
		t.Errorf("AffectedRows = %d, want %d", result.AffectedRows, rowCount)
	}
	afterInsert := countCustomers("after-insert")
	if afterInsert != before+int64(rowCount) {
		t.Errorf("Count after insert = %d, want %d", afterInsert, before+int64(rowCount))
	}

	// A second 200-op plan whose 150th op names an unknown column — F3's transactional half, at
	// batch scale: the whole plan must be rejected, leaving the row count exactly as it was.
	badOps := make([]model.MutationRowOp, rowCount)
	for i := 0; i < rowCount; i++ {
		if i == 149 {
			badOps[i] = model.MutationRowOp{
				Kind:   "insert",
				Values: model.RowValues{{Name: "no_such_column", Value: strp("x")}},
			}
			continue
		}
		badOps[i] = model.MutationRowOp{
			Kind:   "insert",
			Values: model.RowValues{{Name: "name", Value: strp("Should Roll Back " + strconv.Itoa(i))}},
		}
	}
	if _, err := a.Mutate(context.Background(), model.MutationPlan{Path: tablePath, Ops: badOps}, adapters.NewOpCtx("op-29-insert-bad")); err == nil {
		t.Fatal("expected an error for the unknown column")
	}
	afterFailure := countCustomers("after-failure")
	if afterFailure != afterInsert {
		t.Errorf("Count after failed batch = %d, want unchanged at %d (rollback failed)", afterFailure, afterInsert)
	}
}

// 28. execute: one page per statement, including a non-row-returning one.
func TestPostgres_ExecuteOnePagePerStatement(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	pages, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path: nodePath(fixture, seg("database", "kira_test")),
		Statements: []string{
			"SELECT id, name FROM app.customers ORDER BY id",
			"CREATE TEMP TABLE t_exec_28 (x int)",
		},
	}, adapters.NewOpCtx("op-28"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(pages))
	}
	first := pages[0].(page.TabularPage)
	if first.RowCount != 2 {
		t.Errorf("first page RowCount = %d, want 2", first.RowCount)
	}
	second := pages[1].(page.TabularPage)
	if len(second.Columns) != 1 || second.Columns[0].Name != "status" {
		t.Errorf("second page columns = %+v, want a single status column", second.Columns)
	}
}

// P174: the console stops at the result cap; cell-count and truncation flag are exact.
func TestPostgres_ExecuteStopsAtResultCap(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	cfg := fixture.Config
	testsupport.RunScenarios(t, connectedAdapter(t, fixture), cfg,
		testsupport.ConsoleResultCap(nodePath(fixture, seg("database", "kira_test")), func(n int) string {
			return fmt.Sprintf("SELECT generate_series(1, %d)", n)
		}),
	)
}

// Structured conditions bind as parameters and filter on real Postgres.
func TestPostgres_ReadConditions(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	testsupport.RunScenarios(t, connectedAdapter(t, fixture), fixture.Config,
		testsupport.ReadConditions(testsupport.ConditionsCase{
			Name: "postgres", Database: nodePath(fixture, seg("database", "kira_test")),
			Table: nodePath(fixture, seg("database", "kira_test"), seg("schema", "app"), seg("table", "p535_cond")), Binary: true,
			Setup: []string{
				"CREATE TABLE app.p535_cond (id integer PRIMARY KEY, name text, note text, bin bytea)",
				`INSERT INTO app.p535_cond VALUES (1,'Alice','x','\x0102'),(2,'O''Brien',NULL,'\x0304'),(3,'Bob','y','\x0506'),(4,'alina',NULL,NULL)`,
			},
			Teardown: []string{"DROP TABLE app.p535_cond"},
		}),
	)
}

// MCP read verdicts run read-only even on a writable connection.
func TestPostgres_ExecuteReadOnlyRequestRefusesWrite(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	testsupport.RunScenarios(t, connectedAdapter(t, fixture), fixture.Config,
		testsupport.ReadOnlyRequestRefusesWrite(nodePath(fixture, seg("database", "kira_test")),
			"CREATE TABLE kira_ro_probe (id int)", "SELECT 1"),
		testsupport.ReadOnlyRequestKeepsOpenTransaction(nodePath(fixture, seg("database", "kira_test")),
			"CREATE TEMP TABLE kira_ro_tx (x int)", "BEGIN", "INSERT INTO kira_ro_tx VALUES (1)", "SELECT count(*) FROM kira_ro_tx"),
		testsupport.ReadOnlyRequestAllowsBackslashRead(nodePath(fixture, seg("database", "kira_test")),
			`SELECT 'a1' ~ '\d+'`, `SELECT 'a_b' LIKE 'a\_b'`, `SELECT E'\n'`, `SELECT 'read write'`),
	)
}

// 29. execute: a failing statement rejects the whole call — earlier statements already landed.
func TestPostgres_ExecuteFailingStatementRejectsBatch(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	a := connectedAdapter(t, fixture)
	_, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path:       nodePath(fixture, seg("database", "kira_test")),
		Statements: []string{"CREATE TEMP TABLE t_exec_29 (x int)", "SELECT * FROM no_such_table_at_all"},
	}, adapters.NewOpCtx("op-29"))
	if err == nil {
		t.Fatal("expected the second (invalid) statement to fail the whole call")
	}
}

// DB MCP read lane: the engine refuses writes, including one hidden in a function, and cancel
// reaches the lane's own backend.
func TestPostgres_ReadLane(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	path := nodePath(fixture, seg("database", "kira_test"))
	a := connectedAdapter(t, fixture)
	unchanged := func(t *testing.T, main adapters.Adapter) {
		t.Helper()
		if got := testsupport.FirstCell(t, main, path, "SELECT count(*) FROM app.kira_lane_log"); got != "0" {
			t.Errorf("kira_lane_log rows = %s, want 0", got)
		}
	}
	setup := []string{
		"CREATE TABLE app.kira_lane_log (id int)",
		"CREATE FUNCTION app.kira_write_fn() RETURNS int LANGUAGE sql AS 'INSERT INTO app.kira_lane_log VALUES (1) RETURNING id'",
	}
	teardown := []string{"DROP FUNCTION app.kira_write_fn()", "DROP TABLE app.kira_lane_log"}
	testsupport.RunScenarios(t, a, fixture.Config,
		testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
			Path: path, Setup: setup, Write: "SELECT app.kira_write_fn()", WantErr: "25006", Unchanged: unchanged, Teardown: teardown,
		}),
		testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
			Path: path, Setup: setup, Write: "INSERT INTO app.kira_lane_log VALUES (2)", WantErr: "25006", Unchanged: unchanged, Teardown: teardown,
		}),
		testsupport.LaneCancel(path, "SELECT pg_sleep(30)"),
	)
}

// DB MCP statement timeout: the engine stops the statement on the lane and on the main connection,
// and the setting does not outlive the request.
func TestPostgres_StatementTimeout(t *testing.T) {
	fixture := testsupport.StartPostgres(t)
	path := nodePath(fixture, seg("database", "kira_test"))
	a := connectedAdapter(t, fixture)
	unchanged := func(t *testing.T, main adapters.Adapter) {
		t.Helper()
		if got := testsupport.FirstCell(t, main, path, "SELECT count(*) FROM app.kira_to_log"); got != "0" {
			t.Errorf("kira_to_log rows = %s, want 0", got)
		}
	}
	setup := []string{"CREATE TABLE app.kira_to_log (id int)"}
	teardown := []string{"DROP TABLE app.kira_to_log"}
	testsupport.RunScenarios(t, a, fixture.Config,
		testsupport.StatementTimeout(testsupport.StatementTimeoutCase{
			Name: "read lane", Path: path, Lane: true, Slow: "SELECT pg_sleep(5)", WantCode: "57014",
			Baseline: "SHOW statement_timeout", BaselineWant: "0", Lifted: "SELECT pg_sleep(2)",
		}),
		testsupport.StatementTimeout(testsupport.StatementTimeoutCase{
			Name: "main write", Path: path, Setup: setup, Teardown: teardown,
			Slow: "INSERT INTO app.kira_to_log SELECT 1 FROM pg_sleep(5)", WantCode: "57014", Unchanged: unchanged,
			Baseline: "SHOW statement_timeout", BaselineWant: "0", Lifted: "SELECT pg_sleep(2)",
		}),
		testsupport.StatementTimeout(testsupport.StatementTimeoutCase{
			Name: "main write inside a user transaction", Path: path,
			Setup: []string{"BEGIN"}, After: []string{"ROLLBACK"}, Teardown: []string{"ROLLBACK"},
			Slow: "SELECT pg_sleep(5)", WantCode: "57014",
			Baseline: "SHOW statement_timeout", BaselineWant: "0", Lifted: "SELECT pg_sleep(2)",
		}),
	)
}
