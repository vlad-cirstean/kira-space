// Package mysqlfamily_test is the Go analogue of packages/db-fixtures/{mariadb,mysql}.spec.ts: the two specs
// are ~90% the same file, so this is one shared suite function (runFamilySuite) plus two thin
// drivers (TestMariaDB/TestMySQL), in one package so one TestMain stops both containers
// (docs/HISTORY.md (v1 P58b)). Not every one of the two specs' ~30
// scenarios has a Go twin — the ones ported are the load-bearing behaviours P58 D12's own
// "adapter-first-test-first" rule exists to protect, matching P58a's own postgres_test.go
// precedent.
package mysqlfamily_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/mariadb"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/mysql"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

func TestMain(m *testing.M) {
	testsupport.Prewarm("mariadb", "mysql")
	code := m.Run()
	testsupport.StopConcurrently(testsupport.StopMariadb, testsupport.StopMysql)
	os.Exit(code)
}

var deps = adapters.Deps{Log: func(level, message string) {}}

var (
	seg          = testsupport.Seg
	childNames   = testsupport.ChildNames
	containsName = testsupport.ContainsName
	cellAt       = testsupport.CellAt
	strp         = testsupport.Strp
)

// flippingCtx reports ctx.Err() as nil for the first `after` calls, then as context.Canceled for
// every call after that — Done() always returns nil (never fires), so RunWithAbortRace's own
// select (abort.go) never treats a call through this ctx as cancelled mid-flight; only the
// synchronous CheckNotStarted-style pre-checks (query.go's runCommand) ever observe the flip. This
// lets a test force a cancellation to land at an exact, deterministic point in mutate()'s own
// sequence of CheckNotStarted-gated calls (the catalog lookup inside getReadTarget, then START
// TRANSACTION, then each compiled row op, then COMMIT) without any sleep or goroutine race — which
// of those calls is "START TRANSACTION" isn't part of this package's exported surface, so a test
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

// nodePath is testsupport.NodePath (P107 I2-28).
var nodePath = testsupport.NodePath

// sideDSN builds a plain go-sql-driver DSN from cfg's own credentials — a side connection for
// test-only DDL (e.g. a throwaway no-PK probe table), not the adapter's own connection.
func sideDSN(cfg model.ResolvedConnectionConfig) string {
	mc := mysql.NewConfig()
	mc.Net = "tcp"
	host := ""
	if cfg.Host != nil {
		host = *cfg.Host
	}
	port := 3306
	if cfg.Port != nil {
		port = *cfg.Port
	}
	mc.Addr = fmt.Sprintf("%s:%d", host, port)
	if cfg.Username != nil {
		mc.User = *cfg.Username
	}
	if cfg.Password != nil {
		mc.Passwd = *cfg.Password
	}
	if cfg.Database != nil {
		mc.DBName = *cfg.Database
	}
	return mc.FormatDSN()
}

func newAdapter(t *testing.T, kind string) adapters.Adapter {
	t.Helper()
	a, err := adapters.CreateAdapter(kind, deps)
	if err != nil {
		t.Fatalf("CreateAdapter(%s): %v", kind, err)
	}
	return a
}

func connectedAdapter(t *testing.T, kind string, cfg model.ResolvedConnectionConfig) adapters.Adapter {
	t.Helper()
	a := newAdapter(t, kind)
	if _, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

func TestMariaDB(t *testing.T) {
	fixture := testsupport.StartMariadb(t)
	runFamilySuite(t, "mariadb", fixture.Config, testsupport.VersionPattern("MariaDB", testsupport.MariaServerMajor()))
}

func TestMySQL(t *testing.T) {
	fixture := testsupport.StartMysql(t)
	runFamilySuite(t, "mysql", fixture.Config, testsupport.VersionPattern("MySQL", testsupport.MysqlServerMajor()))
}

var bigRowsDetail = regexp.MustCompile(`^~[\d.]+[A-Za-z]*$`)

// runFamilySuite is the ~27 scenarios packages/db-fixtures/mariadb.spec.ts and packages/db-fixtures/mysql.spec.ts share.
func runFamilySuite(t *testing.T, kind string, cfg model.ResolvedConnectionConfig, versionRE *regexp.Regexp) {
	t.Run("pinned connection lifecycle", func(t *testing.T) {
		testsupport.ConnectionLifecycleScenarios(t, cfg, func(t *testing.T) adapters.Adapter { return newAdapter(t, kind) })
	})

	t.Run("connect/disconnect, real server version", func(t *testing.T) {
		a := newAdapter(t, kind)
		info, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-1"))
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		if !versionRE.MatchString(info.ServerVersion) {
			t.Errorf("ServerVersion = %q, want to match %s", info.ServerVersion, versionRE)
		}
		if err := a.Disconnect(context.Background()); err != nil {
			t.Fatalf("Disconnect: %v", err)
		}
	})

	t.Run("auth failure", func(t *testing.T) {
		a := newAdapter(t, kind)
		bad := cfg
		wrong := "definitely-wrong"
		bad.Password = &wrong
		_, err := a.Connect(context.Background(), bad, adapters.NewOpCtx("op-2"))
		if err == nil {
			t.Fatal("expected an error for a wrong password")
		}
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeAuth {
			t.Fatalf("got %v, want an E_AUTH *adapters.Error", err)
		}
	})

	// A fields-mode connection with no `database` (input.go's Validate never requires one for
	// this kind) must still connect. Before adapter.go's Connect scanned DATABASE() into a plain
	// Go string, an unset default schema (client.go's BuildConfig only sets mc.DBName when
	// cfg.Database is non-empty) made DATABASE() return SQL NULL, and database/sql's Scan into a
	// non-nullable string failed the whole probe with "converting NULL to string is unsupported" —
	// 100% reproducible for every such connection, surfacing as a plain E_QUERY the instant a real
	// user tried a database-less connection, which read exactly like the reported "fails to
	// authenticate".
	t.Run("connect with no database", func(t *testing.T) {
		a := newAdapter(t, kind)
		noDB := cfg
		noDB.Database = nil
		info, err := a.Connect(context.Background(), noDB, adapters.NewOpCtx("op-2a"))
		if err != nil {
			t.Fatalf("Connect with no database: %v", err)
		}
		t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
		if info.Details["database"] != "" {
			t.Errorf("connected database = %q, want \"\" (no default schema was selected)", info.Details["database"])
		}
	})

	t.Run("tree enumeration", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		ctx := context.Background()

		dbs, err := a.Children(ctx, nodePath(cfg.ID), adapters.NewOpCtx("op-3a"))
		if err != nil {
			t.Fatalf("Children(root): %v", err)
		}
		dbNames := childNames(t, dbs)
		if !containsName(dbNames, "kira_test") || !containsName(dbNames, "kira_analytics") {
			t.Errorf("databases = %v, want kira_test and kira_analytics", dbNames)
		}
		for _, n := range dbs.Nodes {
			if n.Name == "kira_test" && (n.Detail == nil || *n.Detail != "connected") {
				t.Errorf("kira_test detail = %v, want \"connected\"", n.Detail)
			}
		}

		objects, err := a.Children(ctx, nodePath(cfg.ID, seg("database", "kira_test")), adapters.NewOpCtx("op-3b"))
		if err != nil {
			t.Fatalf("Children(database): %v", err)
		}
		kindOf := map[string]string{}
		for _, n := range objects.Nodes {
			kindOf[n.Name] = n.Kind
		}
		if kindOf["orders"] != "table" || kindOf["order_summary"] != "view" || kindOf["full_name"] != "function" {
			t.Errorf("object kinds = %+v, want orders=table order_summary=view full_name=function", kindOf)
		}
		for _, n := range objects.Nodes {
			if n.HasChildren {
				t.Errorf("object %q HasChildren = true, want false (P19 D5: every relation is a leaf)", n.Name)
			}
		}
	})

	t.Run("describe", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		meta, err := a.Describe(context.Background(), nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "order_items")), adapters.NewOpCtx("op-5"))
		if err != nil {
			t.Fatalf("Describe: %v", err)
		}
		if len(meta.PrimaryKey) == 0 {
			t.Error("expected a primary key")
		}
		if len(meta.ForeignKeys) == 0 {
			t.Error("expected at least one foreign key (order_items references orders/products)")
		}
	})

	// P22c §4.1: SchemaColumns is Describe's schema-wide sibling — every relation in one database
	// together with its columns, in one round trip, byte-identical to what Describe reports.
	t.Run("SchemaColumns: relations with their columns, byte-identical to Describe", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		ctx := context.Background()

		relations, err := a.SchemaColumns(ctx, nodePath(cfg.ID, seg("database", "kira_test")), adapters.NewOpCtx("op-schema-columns-1"))
		if err != nil {
			t.Fatalf("SchemaColumns: %v", err)
		}
		if !a.Caps().SchemaColumns {
			t.Error("Caps().SchemaColumns = false, want true")
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
		if _, ok := byName["full_name"]; ok {
			t.Error("full_name (a function) must not appear — SchemaColumns lists relations, not routines")
		}

		meta, err := a.Describe(ctx, nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "order_items")), adapters.NewOpCtx("op-schema-columns-2"))
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
	})

	// A MySQL/MariaDB constraint name only has to be unique within its own schema — two
	// unrelated tables in two different databases legitimately reusing the same constraint name
	// (a very plausible thing to do) must not be folded into one merged, corrupted entry. The
	// grouping logic itself (groupForeignKeys) has its own unconditional, engine-independent
	// white-box coverage in catalog_internal_test.go; this is the live, end-to-end reproduction.
	t.Run("describe: referencedBy from two databases with a colliding constraint name are not merged", func(t *testing.T) {
		// P26: scoped to this one subtest, not the rest of runFamilySuite — moved in from a bare
		// t.Skip() that used to sit directly in this function's own body just above this t.Run, which
		// (a pre-existing bug this phase's own ×2-engine coverage goal surfaced) skipped every later
		// t.Run in the whole suite for MariaDB, this phase's own new tests included.
		if kind == "mariadb" {
			t.Skip("MariaDB's information_schema.REFERENTIAL_CONSTRAINTS doesn't surface a row for a " +
				"schema the connected user only holds a db-level wildcard GRANT SELECT on (confirmed via " +
				"a standalone probe: KEY_COLUMN_USAGE sees kira_analytics fine, REFERENTIAL_CONSTRAINTS " +
				"returns zero rows for it) — a separate, pre-existing MariaDB limitation unrelated to this " +
				"fix, not reproducible under this fixture's own deliberately-restricted `kira` user")
		}
		a := connectedAdapter(t, kind, cfg)
		ctx := context.Background()

		// A root connection, not sideDSN's own `kira` user: kira only ever holds GRANT SELECT on
		// kira_analytics (seedMysqlExtras/seedMariadbExtras), deliberately no DDL rights there — the
		// very scenario this bug lives in is an app user with read-only cross-database visibility.
		host := ""
		if cfg.Host != nil {
			host = *cfg.Host
		}
		port := 3306
		if cfg.Port != nil {
			port = *cfg.Port
		}
		probeDB, err := sql.Open("mysql", testsupport.RootMysqlDSN(host, port, ""))
		if err != nil {
			t.Fatalf("probe connect: %v", err)
		}
		defer probeDB.Close()

		if _, err := probeDB.ExecContext(ctx,
			"CREATE TABLE IF NOT EXISTS kira_test.fk_collision_a (id INT PRIMARY KEY, customer_id INT, "+
				"CONSTRAINT fk_dup FOREIGN KEY (customer_id) REFERENCES kira_test.customers (id))",
		); err != nil {
			t.Fatalf("create fk_collision_a: %v", err)
		}
		defer probeDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS kira_test.fk_collision_a")

		if _, err := probeDB.ExecContext(ctx,
			"CREATE TABLE IF NOT EXISTS kira_analytics.fk_collision_b (id INT PRIMARY KEY, customer_id INT, "+
				"CONSTRAINT fk_dup FOREIGN KEY (customer_id) REFERENCES kira_test.customers (id))",
		); err != nil {
			t.Fatalf("create fk_collision_b: %v", err)
		}
		defer probeDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS kira_analytics.fk_collision_b")

		meta, err := a.Describe(ctx, nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")), adapters.NewOpCtx("op-91-referencedby"))
		if err != nil {
			t.Fatalf("Describe: %v", err)
		}

		aPath := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "kira_test"}, {Kind: "table", Name: "fk_collision_a"}})
		bPath := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "kira_analytics"}, {Kind: "table", Name: "fk_collision_b"}})
		var fromA, fromB *model.ForeignKeyMeta
		for i := range meta.ReferencedBy {
			fk := &meta.ReferencedBy[i]
			if fk.Name != "fk_dup" {
				continue
			}
			switch fk.ReferencedPath {
			case aPath:
				fromA = fk
			case bPath:
				fromB = fk
			}
		}
		if fromA == nil || fromB == nil {
			t.Fatalf("expected two separate fk_dup entries, one per database, got %+v", meta.ReferencedBy)
		}
		if len(fromA.ReferencedColumns) != 1 || fromA.ReferencedColumns[0] != "customer_id" {
			t.Errorf("fk_collision_a entry ReferencedColumns = %v, want [customer_id] (not merged with fk_collision_b's)", fromA.ReferencedColumns)
		}
		if len(fromB.ReferencedColumns) != 1 || fromB.ReferencedColumns[0] != "customer_id" {
			t.Errorf("fk_collision_b entry ReferencedColumns = %v, want [customer_id] (not merged with fk_collision_a's)", fromB.ReferencedColumns)
		}
	})

	t.Run("row estimate on big_rows", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		objects, err := a.Children(context.Background(), nodePath(cfg.ID, seg("database", "kira_test")), adapters.NewOpCtx("op-6"))
		if err != nil {
			t.Fatalf("Children: %v", err)
		}
		for _, n := range objects.Nodes {
			if n.Name == "big_rows" && (n.Detail == nil || !bigRowsDetail.MatchString(*n.Detail)) {
				t.Errorf("big_rows detail = %v, want a \"~N\" estimate", n.Detail)
			}
		}
	})

	t.Run("definition renders SHOW CREATE TABLE", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		def, err := a.Definition(context.Background(), nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")), adapters.NewOpCtx("op-7"))
		if err != nil {
			t.Fatalf("Definition: %v", err)
		}
		if len(def.Statements) != 1 || def.Statements[0] == "" {
			t.Fatalf("Statements = %v, want exactly one non-empty CREATE TABLE", def.Statements)
		}
	})

	t.Run("quoting: a table whose name needs backtick-doubling", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "weird`name")),
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-8"))
		if err != nil {
			t.Fatalf("Read(weird`name): %v", err)
		}
		tp := p.(page.TabularPage)
		if tp.RowCount != 1 {
			t.Fatalf("RowCount = %d, want 1", tp.RowCount)
		}
	})

	t.Run("read: first page", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-9"))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		tp := p.(page.TabularPage)
		if tp.RowCount != 2 {
			t.Errorf("RowCount = %d, want 2", tp.RowCount)
		}
	})

	// P168 Part 3 F4: a binary key's token must bind as raw bytes, not its 0x<hex> display text.
	t.Run("read: keyset over a binary primary key terminates and covers every row", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		ctx := context.Background()
		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path: nodePath(cfg.ID, seg("database", "kira_test")),
			Statements: []string{
				"DROP TABLE IF EXISTS p168_kbin",
				"CREATE TABLE p168_kbin (id VARBINARY(4) PRIMARY KEY)",
				"INSERT INTO p168_kbin VALUES (0x01), (0x0A), (0x30), (0x7F), (0x80), (0x81FF), (0xFF), (0xFFFF)",
			},
		}, adapters.NewOpCtx("op-kbin-setup")); err != nil {
			t.Fatalf("setup: %v", err)
		}
		defer a.Execute(ctx, model.ConsoleRequest{Path: nodePath(cfg.ID, seg("database", "kira_test")), Statements: []string{"DROP TABLE p168_kbin"}}, adapters.NewOpCtx("op-kbin-cleanup")) //nolint:errcheck
		sort := &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}}
		got := testsupport.KeysetForwardIDs(t, a, nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "p168_kbin")), sort, 2, 8)
		want := []string{"0x01", "0x0a", "0x30", "0x7f", "0x80", "0x81ff", "0xff", "0xffff"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	})

	t.Run("read: keyset forward and back over big_rows", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		ctx := context.Background()
		tablePath := nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "big_rows"))
		sort := &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}}

		first, err := a.Read(ctx, adapters.ReadRequest{
			Path: tablePath, Sort: sort, PageSize: 5, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-10a"))
		if err != nil {
			t.Fatalf("Read(first): %v", err)
		}
		firstPage := first.(page.TabularPage)
		if firstPage.Position.NextToken == nil {
			t.Fatal("expected a NextToken on the first page")
		}
		if firstPage.Position.Strategy != "keyset" {
			t.Errorf("Position.Strategy = %q, want keyset", firstPage.Position.Strategy)
		}

		second, err := a.Read(ctx, adapters.ReadRequest{
			Path: tablePath, Sort: sort, PageSize: 5, Cursor: model.PageCursor{Mode: "after", Token: *firstPage.Position.NextToken},
		}, adapters.NewOpCtx("op-10b"))
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
		}, adapters.NewOpCtx("op-10c"))
		if err != nil {
			t.Fatalf("Read(before): %v", err)
		}
		backPage := back.(page.TabularPage)
		backFirstID := cellAt(t, backPage, 0, 0)
		if backFirstID == nil || *backFirstID != "1" {
			t.Errorf("page-before's first id = %v, want 1 (display order preserved)", backFirstID)
		}

		// Regression: a "before" page is fetched in descending (fetch) order and reversed to
		// ascending (display) order — the streaming readPage must swap its tracked first/last raw
		// rows along with builder.Reverse(), or this page's own NextToken (built from
		// CellAt(rowCount-1,...), the last *displayed* row) would be built from the wrong row's
		// keyset value entirely.
		if backPage.Position.NextToken == nil {
			t.Fatal("expected a NextToken on the page-before result")
		}
		forwardAgain, err := a.Read(ctx, adapters.ReadRequest{
			Path: tablePath, Sort: sort, PageSize: 5, Cursor: model.PageCursor{Mode: "after", Token: *backPage.Position.NextToken},
		}, adapters.NewOpCtx("op-10d"))
		if err != nil {
			t.Fatalf("Read(after, from page-before's NextToken): %v", err)
		}
		forwardAgainPage := forwardAgain.(page.TabularPage)
		forwardAgainFirstID := cellAt(t, forwardAgainPage, 0, 0)
		if forwardAgainFirstID == nil || *forwardAgainFirstID != "6" {
			t.Errorf("page after page-before's NextToken: first id = %v, want 6", forwardAgainFirstID)
		}
	})

	// P26 §3.2(1)/(2): mysql/caps.go and mariadb/caps.go both declare Projection/ServerFilter true;
	// neither had a dedicated real-container test before this.
	t.Run("read: filter and sort", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		// finding 9: "region_id = 1" narrowed to Acme Co alone, so the requested sort order was
		// never actually observable — the test would pass even with Sort silently dropped.
		// "region_id IS NOT NULL" keeps both seeded customers (Acme Co, Globex both have a
		// region), so descending-name order is a real, checkable assertion.
		//
		// round-2 finding 3: this must not assert the seeded rows' exact literal names — two later
		// subtests in this same file ("mutate: update lands, affectedRows" and "mutate: update
		// setting a column to its own current value still reports affectedRows=1") permanently
		// rename these same two rows and never restore them, so a pinned-literal assertion here only
		// passed on source-order coincidence within one process/container. Assert relative order
		// (descending: first sorts after second alphabetically) instead, which holds regardless of
		// what the rows are actually named by the time this runs.
		filter := "region_id IS NOT NULL"
		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path:   nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			Filter: &filter, Sort: &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "name", Direction: "desc"}}},
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-10e"))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		tp := p.(page.TabularPage)
		if tp.RowCount != 2 {
			t.Fatalf("RowCount = %d, want 2 (both customers have a region)", tp.RowCount)
		}
		nameCol := -1
		for i, c := range tp.Columns {
			if c.Name == "name" {
				nameCol = i
				break
			}
		}
		if nameCol < 0 {
			t.Fatalf("no name column in %+v", tp.Columns)
		}
		first, second := cellAt(t, tp, nameCol, 0), cellAt(t, tp, nameCol, 1)
		deref := func(s *string) string {
			if s == nil {
				return "<nil>"
			}
			return *s
		}
		if first == nil || second == nil {
			t.Fatalf("sorted names = [%s, %s], want two non-nil names", deref(first), deref(second))
		}
		if *first == *second || *first < *second {
			t.Errorf("sorted names = [%s, %s], want first > second (name desc)", *first, *second)
		}
	})

	// Wired through scenarios.go's shared constructor (P26 §2.2/P26 review F3) rather than
	// hand-rolled, so this scenario also backs Tier 2 via RunMatrix's own Then slices.
	testsupport.RunScenarios(t, connectedAdapter(t, kind, cfg), cfg,
		testsupport.ProjectionLimitsColumns(
			nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			[]string{"name"},
		),
		testsupport.ReadOnlyRequestRefusesWrite(nodePath(cfg.ID, seg("database", "kira_test")),
			"INSERT INTO customers (name) VALUES ('ro-probe')", "SELECT 1"),
		testsupport.ReadOnlyRequestKeepsOpenTransaction(nodePath(cfg.ID, seg("database", "kira_test")),
			"CREATE TEMPORARY TABLE kira_ro_tx (x int) ENGINE=InnoDB", "START TRANSACTION", "INSERT INTO kira_ro_tx VALUES (1)", "SELECT count(*) FROM kira_ro_tx"),
		testsupport.ReadOnlyRequestAllowsBackslashRead(nodePath(cfg.ID, seg("database", "kira_test")),
			`SELECT 'a1' REGEXP '\\d+'`, `SELECT 'a_b' LIKE 'a\\_b'`, `SELECT 'read write'`),
		testsupport.ConsoleResultCap(nodePath(cfg.ID, seg("database", "kira_test")), func(n int) string {
			return fmt.Sprintf("WITH RECURSIVE s(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM s WHERE n < %d) SELECT n FROM s", n)
		}),
	)

	lanePath := nodePath(cfg.ID, seg("database", "kira_test"))
	laneUnchanged := func(t *testing.T, main adapters.Adapter) {
		t.Helper()
		if got := testsupport.FirstCell(t, main, lanePath, "SELECT count(*) FROM kira_lane_log"); got != "0" {
			t.Errorf("kira_lane_log rows = %s, want 0", got)
		}
	}
	laneSetup := []string{
		"CREATE TABLE kira_lane_log (id int)",
		"CREATE FUNCTION kira_write_fn() RETURNS int MODIFIES SQL DATA BEGIN INSERT INTO kira_lane_log VALUES (1); RETURN 1; END",
	}
	laneTeardown := []string{"DROP FUNCTION IF EXISTS kira_write_fn", "DROP TABLE IF EXISTS kira_lane_log"}
	testsupport.RunScenarios(t, connectedAdapter(t, kind, cfg), cfg,
		testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
			Path: lanePath, Setup: laneSetup, Write: "SELECT kira_write_fn()", WantErr: "read only transaction", Unchanged: laneUnchanged, Teardown: laneTeardown,
		}),
		testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
			Path: lanePath, Setup: laneSetup, Write: "INSERT INTO kira_lane_log VALUES (2)", WantErr: "read only transaction", Unchanged: laneUnchanged, Teardown: laneTeardown,
		}),
		testsupport.LaneCancel(lanePath, "SELECT SLEEP(30)"),
	)

	t.Run("count", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		result, err := a.Count(context.Background(), adapters.CountRequest{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
		}, adapters.NewOpCtx("op-11"))
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if result.Value != 2 || !result.Exact {
			t.Errorf("Count = %+v, want {2, true}", result)
		}
	})

	t.Run("preview never executes", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			Ops: []model.MutationRowOp{{
				Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}},
				Changes: model.RowValues{{Name: "name", Value: strp("should not land")}},
			}},
		}
		statements, err := a.Preview(plan)
		if err != nil {
			t.Fatalf("Preview: %v", err)
		}
		if len(statements) != 1 {
			t.Fatalf("statements = %v, want exactly 1", statements)
		}
		count, err := a.Count(context.Background(), adapters.CountRequest{Path: plan.Path}, adapters.NewOpCtx("op-12-count"))
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if count.Value != 2 {
			t.Errorf("Count after Preview = %d, want unchanged at 2 (preview must never execute)", count.Value)
		}
	})

	t.Run("preview escapes backslash in literal values", func(t *testing.T) {
		// MySQL/MariaDB treat \ as a string-literal escape character (unlike
		// Postgres/SQLite), so an unescaped backslash in a preview literal would mis-render what
		// the dialect parses — e.g. a trailing \' would escape the closing quote instead
		// of ending the string. preview() never executes, so this only checks the rendered text.
		a := connectedAdapter(t, kind, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			Ops: []model.MutationRowOp{{
				Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}},
				Changes: model.RowValues{{Name: "name", Value: strp(`C:\temp\'; DROP TABLE customers; --`)}},
			}},
		}
		statements, err := a.Preview(plan)
		if err != nil {
			t.Fatalf("Preview: %v", err)
		}
		if len(statements) != 1 {
			t.Fatalf("statements = %v, want exactly 1", statements)
		}
		want := `'C:\\temp\\''; DROP TABLE customers; --'`
		if !strings.Contains(statements[0], want) {
			t.Errorf("statement = %q, want it to contain %q (backslash doubled before quote escaping)", statements[0], want)
		}
	})

	t.Run("mutate: update lands, affectedRows", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			Ops: []model.MutationRowOp{{
				Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}},
				Changes: model.RowValues{{Name: "name", Value: strp("Acme Corp")}},
			}},
		}
		result, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-13"))
		if err != nil {
			t.Fatalf("Mutate: %v", err)
		}
		if result.AffectedRows != 1 {
			t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
		}
	})

	// P2 R2: without CLIENT_FOUND_ROWS, MySQL/MariaDB report an UPDATE's "rows changed" count, not
	// "rows matched" — setting a column back to the value it already holds affects 0 rows on the
	// wire, which AssertAffectedExactlyOne then rejected as a failed update. Two mutates to the same
	// value, back to back: the second must land exactly like the first, not roll back as if the row
	// were never found.
	t.Run("mutate: update setting a column to its own current value still reports affectedRows=1", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			Ops: []model.MutationRowOp{{
				Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("2")}},
				Changes: model.RowValues{{Name: "name", Value: strp("Globex Noop")}},
			}},
		}
		if _, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-13-noop-setup")); err != nil {
			t.Fatalf("Mutate (setup): %v", err)
		}
		result, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-13-noop"))
		if err != nil {
			t.Fatalf("Mutate (no-op update): %v", err)
		}
		if result.AffectedRows != 1 {
			t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
		}
	})

	t.Run("mutate: unknown column is E_NOT_FOUND", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			Ops: []model.MutationRowOp{{
				Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}},
				Changes: model.RowValues{{Name: "no_such_column", Value: strp("x")}},
			}},
		}
		_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-14"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeNotFound {
			t.Fatalf("got %v, want E_NOT_FOUND", err)
		}
	})

	// A cancellation landing between START TRANSACTION and COMMIT left neither COMMIT nor
	// the compensating ROLLBACK ever reaching the server — both are gated by the same
	// CheckNotStarted check COMMIT itself is, so an already-observed-as-cancelled ctx short-
	// circuits ROLLBACK exactly like it does COMMIT. Because this adapter's *sql.Conn is pinned
	// for its whole lifetime (SetMaxOpenConns(1)), the server session was then left inside that
	// still-open transaction indefinitely: the next Mutate's own START TRANSACTION landed inside
	// it, so its COMMIT silently committed the earlier cancelled write too — worse than a hang.
	// flippingCtx sweeps every plausible cancellation point and, for whichever ones
	// trigger (an error from Mutate), proves the transaction was rolled back: a clean
	// follow-up Mutate on a different row must succeed quickly and must not also commit the
	// earlier row's cancelled change.
	t.Run("mutate: a cancellation between START TRANSACTION and COMMIT does not leak an open transaction", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		tablePath := nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "order_items"))

		readQuantity := func(id string) string {
			t.Helper()
			p, err := a.Read(context.Background(), adapters.ReadRequest{
				Path: tablePath, Projection: []string{"quantity"}, PageSize: 10,
				Filter: strp("id = " + id), Cursor: model.PageCursor{Mode: "offset", Offset: 0},
			}, adapters.NewOpCtx("op-15-verify-"+id))
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
			_, err := a.Mutate(ctx, plan, adapters.NewOpCtx("op-15-cancel"))
			if err == nil {
				// This `after` fell past every CheckNotStarted check mutate() makes (COMMIT
				// included) — the mutate ran to completion normally. Reset row 1 and move on.
				if got := readQuantity("1"); got != "77" {
					t.Fatalf("after=%d: Mutate reported success but quantity = %s, want 77", after, got)
				}
				if _, err := a.Mutate(context.Background(), model.MutationPlan{
					Path: tablePath,
					Ops: []model.MutationRowOp{
						{Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}}, Changes: model.RowValues{{Name: "quantity", Value: strp(baseline)}}},
					},
				}, adapters.NewOpCtx("op-15-reset")); err != nil {
					t.Fatalf("after=%d: reset Mutate: %v", after, err)
				}
				continue
			}

			// A cancellation landed somewhere at or before COMMIT. The load-bearing assertion: a
			// completely unrelated follow-up Mutate, on a real ctx, must complete quickly rather
			// than hang behind a stale open transaction on the pinned connection...
			verifyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err = a.Mutate(verifyCtx, model.MutationPlan{
				Path: tablePath,
				Ops: []model.MutationRowOp{
					{Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("2")}}, Changes: model.RowValues{{Name: "quantity", Value: strp("55")}}},
				},
			}, adapters.NewOpCtx("op-15-followup"))
			cancel()
			if err != nil {
				t.Fatalf("after=%d: follow-up Mutate on a different row: %v (the earlier cancellation left a stale open transaction)", after, err)
			}
			// ...and it must not silently commit row 1's own cancelled write along with it — proof
			// the cancelled transaction was actually rolled back, not left dangling for this
			// commit to close.
			if got := readQuantity("1"); got != baseline {
				t.Fatalf("after=%d: row 1 quantity = %s after a cancelled Mutate + unrelated follow-up, want unchanged %s (the cancelled write leaked into the follow-up's commit)", after, got, baseline)
			}
			// Restore row 2 for the next iteration.
			if _, err := a.Mutate(context.Background(), model.MutationPlan{
				Path: tablePath,
				Ops: []model.MutationRowOp{
					{Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("2")}}, Changes: model.RowValues{{Name: "quantity", Value: strp("1")}}},
				},
			}, adapters.NewOpCtx("op-15-reset-2")); err != nil {
				t.Fatalf("after=%d: reset row 2: %v", after, err)
			}
		}
	})

	t.Run("mutate: no primary key is E_UNSUPPORTED", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		ctx := context.Background()

		probeDB, err := sql.Open("mysql", sideDSN(cfg))
		if err != nil {
			t.Fatalf("probe connect: %v", err)
		}
		defer probeDB.Close()
		if _, err := probeDB.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS no_pk_probe (col VARCHAR(255))"); err != nil {
			t.Fatalf("create probe table: %v", err)
		}
		defer probeDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS no_pk_probe")

		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "no_pk_probe")),
			Ops: []model.MutationRowOp{{
				Kind: "update", Key: model.RowValues{{Name: "col", Value: strp("x")}},
				Changes: model.RowValues{{Name: "col", Value: strp("y")}},
			}},
		}
		_, err = a.Mutate(ctx, plan, adapters.NewOpCtx("op-15"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
			t.Fatalf("got %v, want E_UNSUPPORTED (no_pk_probe has no primary key)", err)
		}
	})

	// A VARBINARY column's edited value arrives from the grid still spelled in the app's
	// own "0x<hex>" display convention (read.go's own cellText) — Mutate must decode it back into
	// raw bytes before binding it as a parameter, not hand the driver that ASCII text as if it
	// were the new column content.
	t.Run("mutate: VARBINARY column edit round-trips as bytes, not its own hex text", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		ctx := context.Background()

		probeDB, err := sql.Open("mysql", sideDSN(cfg))
		if err != nil {
			t.Fatalf("probe connect: %v", err)
		}
		defer probeDB.Close()
		if _, err := probeDB.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS blob_rw (id INT PRIMARY KEY, data VARBINARY(255))"); err != nil {
			t.Fatalf("create probe table: %v", err)
		}
		defer probeDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS blob_rw")
		if _, err := probeDB.ExecContext(ctx, "INSERT INTO blob_rw (id, data) VALUES (1, ?)", []byte{0x01, 0x02, 0x03}); err != nil {
			t.Fatalf("seed probe table: %v", err)
		}

		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "blob_rw")),
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
		if err := probeDB.QueryRowContext(ctx, "SELECT data FROM blob_rw WHERE id = 1").Scan(&stored); err != nil {
			t.Fatalf("probe read back: %v", err)
		}
		want := []byte{0x04, 0x05}
		if !bytes.Equal(stored, want) {
			t.Errorf("stored bytes = %#v, want %#v (got the display text instead of decoded bytes?)", stored, want)
		}
	})

	// P26 §3.2(3): a DDL round trip through Execute, visible to Children/Describe/Definition on the
	// same connection — closes the ✗ mark neither engine had, ×2 via runFamilySuite.
	t.Run("execute: a DDL round trip is visible to the catalog", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		ctx := context.Background()
		databasePath := nodePath(cfg.ID, seg("database", "kira_test"))
		tablePath := nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "p26_scratch"))

		probeDB, err := sql.Open("mysql", sideDSN(cfg))
		if err != nil {
			t.Fatalf("probe connect: %v", err)
		}
		// Registered first, so its LIFO position is last: the DROP TABLE cleanup below must run
		// against a still-open connection — a plain `defer probeDB.Close()` would close it before
		// t.Cleanup callbacks even start (postgres_test.go's own pattern).
		t.Cleanup(func() { _ = probeDB.Close() })
		probeDB.ExecContext(ctx, "DROP TABLE IF EXISTS kira_test.p26_scratch")
		t.Cleanup(func() { probeDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS kira_test.p26_scratch") })

		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path:       databasePath,
			Statements: []string{"CREATE TABLE p26_scratch (id INT PRIMARY KEY, name VARCHAR(64))"},
		}, adapters.NewOpCtx("op-ddl-1")); err != nil {
			t.Fatalf("Execute(CREATE TABLE): %v", err)
		}

		children, err := a.Children(ctx, databasePath, adapters.NewOpCtx("op-ddl-2"))
		if err != nil {
			t.Fatalf("Children: %v", err)
		}
		if !containsName(childNames(t, children), "p26_scratch") {
			t.Fatalf("Children(kira_test) = %v, want p26_scratch present", childNames(t, children))
		}

		meta, err := a.Describe(ctx, tablePath, adapters.NewOpCtx("op-ddl-3"))
		if err != nil {
			t.Fatalf("Describe: %v", err)
		}
		if len(meta.Columns) != 2 {
			t.Fatalf("Columns = %+v, want exactly 2 (id, name)", meta.Columns)
		}
		if len(meta.PrimaryKey) != 1 || meta.PrimaryKey[0] != "id" {
			t.Errorf("PrimaryKey = %v, want [id]", meta.PrimaryKey)
		}

		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path:       databasePath,
			Statements: []string{"ALTER TABLE p26_scratch ADD COLUMN note VARCHAR(64)"},
		}, adapters.NewOpCtx("op-ddl-4")); err != nil {
			t.Fatalf("Execute(ALTER TABLE): %v", err)
		}
		meta2, err := a.Describe(ctx, tablePath, adapters.NewOpCtx("op-ddl-5"))
		if err != nil {
			t.Fatalf("Describe after ALTER: %v", err)
		}
		if len(meta2.Columns) != 3 {
			t.Fatalf("Columns after ALTER = %+v, want exactly 3", meta2.Columns)
		}

		// MySQL and MariaDB render SHOW CREATE TABLE differently — this is the one place the two
		// engines' own Definition output is compared against DDL this test itself wrote, rather than
		// only against the seeded `customers` table ("definition renders SHOW CREATE TABLE" above).
		def, err := a.Definition(ctx, tablePath, adapters.NewOpCtx("op-ddl-6"))
		if err != nil {
			t.Fatalf("Definition: %v", err)
		}
		if len(def.Statements) != 1 || def.Statements[0] == "" {
			t.Fatalf("Statements = %v, want exactly one non-empty CREATE TABLE", def.Statements)
		}

		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path:       databasePath,
			Statements: []string{"DROP TABLE p26_scratch"},
		}, adapters.NewOpCtx("op-ddl-7")); err != nil {
			t.Fatalf("Execute(DROP TABLE): %v", err)
		}
		childrenAfter, err := a.Children(ctx, databasePath, adapters.NewOpCtx("op-ddl-8"))
		if err != nil {
			t.Fatalf("Children after DROP: %v", err)
		}
		if containsName(childNames(t, childrenAfter), "p26_scratch") {
			t.Errorf("Children(kira_test) after DROP = %v, must not contain p26_scratch", childNames(t, childrenAfter))
		}
	})

	// P26 §3.2(4): closes both the insert and the delete ✗ marks in one test, against a scratch
	// table — no seeded table is touched (§2.4).
	t.Run("mutate: insert then delete round-trips", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		ctx := context.Background()
		tablePath := nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "p26_insdel"))

		probeDB, err := sql.Open("mysql", sideDSN(cfg))
		if err != nil {
			t.Fatalf("probe connect: %v", err)
		}
		// Registered first, so its LIFO position is last: the DROP TABLE cleanup below must run
		// against a still-open connection — a plain `defer probeDB.Close()` would close it before
		// t.Cleanup callbacks even start (postgres_test.go's own pattern).
		t.Cleanup(func() { _ = probeDB.Close() })
		if _, err := probeDB.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS p26_insdel (id INT PRIMARY KEY, name VARCHAR(64))"); err != nil {
			t.Fatalf("create scratch table: %v", err)
		}
		t.Cleanup(func() { probeDB.ExecContext(context.Background(), "DROP TABLE IF EXISTS p26_insdel") })

		testsupport.InsertDeleteRoundTrip(t, a, tablePath)
	})

	t.Run("read-only connection cannot write", func(t *testing.T) {
		ro := cfg
		ro.ReadOnly = true
		a := newAdapter(t, kind)
		if _, err := a.Connect(context.Background(), ro, adapters.NewOpCtx("op-16-connect")); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer a.Disconnect(context.Background())
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "id", Value: strp("1")}}}},
		}
		_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-16"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
			t.Fatalf("got %v, want E_UNSUPPORTED", err)
		}
	})

	// P2 R2: client.go's own SET SESSION TRANSACTION READ ONLY only governs transactions not yet
	// started, so a console statement could flip it back for whatever ran after it in the same
	// batch. execute() now wraps the whole batch in START TRANSACTION READ ONLY, which — confirmed
	// against a real server — MariaDB/MySQL refuse to let any statement lift mid-transaction
	// ("Transaction characteristics can't be changed while a transaction is in progress").
	//
	// Targets order_items rather than customers: customers is referenced by orders' own FK, so a
	// DELETE against it fails with a foreign-key error regardless of read-only enforcement — a
	// false pass that doesn't actually exercise the guard. order_items has no dependents, so a
	// DELETE against it only fails if the read-only guard itself is doing its job.
	t.Run("read-only connection execute cannot escape read-only transaction", func(t *testing.T) {
		ro := cfg
		ro.ReadOnly = true
		a := newAdapter(t, kind)
		if _, err := a.Connect(context.Background(), ro, adapters.NewOpCtx("op-ro-connect")); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer a.Disconnect(context.Background())

		before, err := a.Count(context.Background(), adapters.CountRequest{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "order_items")),
		}, adapters.NewOpCtx("op-ro-count-before"))
		if err != nil {
			t.Fatalf("Count: %v", err)
		}

		// tx_read_only is mariadb's own spelling; transaction_read_only is mysql's (and postgres's).
		roVarName := "transaction_read_only"
		if kind == "mariadb" {
			roVarName = "tx_read_only"
		}
		attempts := [][]string{
			{"SET SESSION TRANSACTION READ WRITE", "DELETE FROM order_items"},
			{"SET TRANSACTION READ WRITE", "DELETE FROM order_items"},
			{"COMMIT", "START TRANSACTION", "DELETE FROM order_items"},
			// P26 §3.2(5): a DDL statement takes a different server-side path than DELETE — untested
			// before this.
			{"CREATE TABLE p26_ro_escape (id INT)"},
			// The batch's own COMMIT ends the wrapping read-only transaction,
			// after which SET SESSION took effect immediately for the plain-autocommit write that
			// followed — confirmed against a real server before the fix that this created the table
			// on both mariadb (tx_read_only) and mysql (transaction_read_only).
			{"COMMIT", "SET SESSION " + roVarName + " = OFF", "DELETE FROM order_items"},
			// P168 Part 3 F2: comment-lexer gaps (# comments, `--` arithmetic, non-nesting blocks)
			// no longer hide the COMMIT or the session flip.
			{"# x\nCOMMIT", "SET @a = 1--1, SESSION " + roVarName + " = OFF", "DELETE FROM order_items"},
			{"/* /* */ COMMIT -- */", "SET @a = 1--1, SESSION " + roVarName + " = OFF", "DELETE FROM order_items"},
			// An implicit commit the text screen cannot name; the per-statement state check
			// catches it before the session flip, hidden behind a variable, runs.
			{"LOCK TABLES order_items READ", "SET @v = 0", "SET SESSION " + roVarName + " = @v", "UNLOCK TABLES", "DELETE FROM order_items"},
			// Flipping only the session default (the wrap itself stays open) must not leave the
			// pinned connection writable once the wrap ends.
			{"SET @v = 0", "SET SESSION " + roVarName + " = @v"},
		}
		for _, statements := range attempts {
			_, err := a.Execute(context.Background(), model.ConsoleRequest{
				Path:       nodePath(cfg.ID, seg("database", "kira_test")),
				Statements: statements,
			}, adapters.NewOpCtx("op-ro-escape"))
			if err == nil {
				t.Fatalf("Execute(%v) succeeded on a read-only connection, want an error", statements)
			}
		}

		after, err := a.Count(context.Background(), adapters.CountRequest{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "order_items")),
		}, adapters.NewOpCtx("op-ro-count-after"))
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if after.Value != before.Value {
			t.Fatalf("order_items row count = %d after read-only Execute attempts, want unchanged %d", after.Value, before.Value)
		}

		probeDB, err := sql.Open("mysql", sideDSN(cfg))
		if err != nil {
			t.Fatalf("probe connect: %v", err)
		}
		defer probeDB.Close()
		var exists int
		if err := probeDB.QueryRowContext(context.Background(),
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'kira_test' AND table_name = 'p26_ro_escape'").Scan(&exists); err != nil {
			t.Fatalf("information_schema check: %v", err)
		}
		if exists != 0 {
			t.Errorf("p26_ro_escape exists after a read-only Execute attempt, want it never created")
		}

		if _, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path:       nodePath(cfg.ID, seg("database", "kira_test")),
			Statements: []string{"SELECT 1"},
		}, adapters.NewOpCtx("op-ro-still-usable")); err != nil {
			t.Fatalf("Execute(SELECT 1) after failed escape attempts: %v", err)
		}
	})

	t.Run("execute: one page per statement", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		pages, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path: nodePath(cfg.ID, seg("database", "kira_test")),
			Statements: []string{
				"SELECT id, name FROM customers ORDER BY id",
				"CREATE TEMPORARY TABLE t_exec_17 (x INT)",
			},
		}, adapters.NewOpCtx("op-17"))
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
			t.Errorf("second page columns = %+v, want a single status column (a non-row-returning statement's generic OK page)", second.Columns)
		}
	})

	t.Run("execute: a failing statement rejects the whole call", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		_, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path:       nodePath(cfg.ID, seg("database", "kira_test")),
			Statements: []string{"CREATE TEMPORARY TABLE t_exec_18 (x INT)", "SELECT * FROM no_such_table_at_all"},
		}, adapters.NewOpCtx("op-18"))
		if err == nil {
			t.Fatal("expected the second (invalid) statement to fail the whole call")
		}
	})

	// "The scenario that must not be softened" (§5.3): a real SELECT SLEEP, KILL QUERY from a real
	// side connection, and — the assertion that actually matters — SHOW PROCESSLIST confirms the
	// query is gone afterward, not merely that the local call returned.
	t.Run("cancel, asserted server-side", func(t *testing.T) {
		a := connectedAdapter(t, kind, cfg)
		side := connectedAdapter(t, kind, cfg)

		ctx, cancel := context.WithCancel(context.Background())
		op := adapters.NewOpCtx("op-cancel")

		errCh := make(chan error, 1)
		go func() {
			_, err := a.Execute(ctx, model.ConsoleRequest{
				Path:       nodePath(cfg.ID, seg("database", "kira_test")),
				Statements: []string{"SELECT SLEEP(30)"},
			}, op)
			errCh <- err
		}()

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if op.Command() != "" {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(200 * time.Millisecond) // let the statement actually reach the server
		cancel()                           // unblocks the local wait only — see query.go's own doc comment

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

		waitForProcesslistClean(t, side, 2*time.Second)
	})
}

// waitForProcesslistClean polls SHOW PROCESSLIST over a separate, already-connected adapter until
// no SLEEP(30) command is running — the real proof KILL QUERY reached the server, not just that
// the local call returned.
func waitForProcesslistClean(t *testing.T, side adapters.Adapter, timeout time.Duration) {
	t.Helper()
	// Executed through Execute() so it goes over the same adapter path everything else in this
	// suite uses — no direct driver access needed here.
	deadline := time.Now().Add(timeout)
	for {
		pages, err := side.Execute(context.Background(), model.ConsoleRequest{
			Statements: []string{"SELECT COUNT(*) AS n FROM information_schema.PROCESSLIST WHERE INFO LIKE 'SELECT SLEEP%'"},
		}, adapters.NewOpCtx("op-processlist-check"))
		if err != nil {
			t.Fatalf("SHOW PROCESSLIST check: %v", err)
		}
		tp := pages[0].(page.TabularPage)
		n := cellAt(t, tp, 0, 0)
		if n != nil && *n == "0" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pg_sleep still present in PROCESSLIST after %s", timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
