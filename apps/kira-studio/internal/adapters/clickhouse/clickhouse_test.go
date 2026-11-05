// Package clickhouse_test is the Go analogue of packages/db-fixtures/clickhouse.spec.ts. Not every
// one of the spec's 47 scenarios has a Go twin — the ones ported are the load-bearing behaviours
// the "adapter-first-test-first" rule exists to protect, plus the three carrying the most weight
// (wide types, big integers, NULL-vs-"null"-vs-NaN).
package clickhouse_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/clickhouse"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testsupport.StopClickHouse()
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

// nodePath is testsupport.NodePath (P107 I2-28).
var nodePath = testsupport.NodePath

func newAdapter(t *testing.T) adapters.Adapter {
	t.Helper()
	a, err := adapters.CreateAdapter("clickhouse", deps)
	if err != nil {
		t.Fatalf("CreateAdapter(clickhouse): %v", err)
	}
	return a
}

var (
	connectCounter atomic.Int64
	opNonce        = strconv.FormatInt(time.Now().UnixNano(), 36)
	opCounter      atomic.Int64
)

// opCtx returns an OpCtx whose id is unique per call and per process run: ClickHouse rejects a
// query id still registered from an earlier run (QUERY_WITH_SAME_ID_IS_ALREADY_RUNNING).
func opCtx(t *testing.T, name string) *adapters.OpCtx {
	t.Helper()
	return adapters.NewOpCtx(name + "-" + opNonce + "-" + strconv.FormatInt(opCounter.Add(1), 10))
}

func connectedAdapter(t *testing.T, cfg model.ResolvedConnectionConfig) adapters.Adapter {
	t.Helper()
	a := newAdapter(t)
	// The op's own OpID feeds adapter.go's own nextQueryID ("kira-<opID>-<seq>") — it must be
	// unique per Connect() call, not a shared literal: a fixed "connect" here produced a genuine
	// QUERY_WITH_SAME_ID_IS_ALREADY_RUNNING collision against the real server when two connects'
	// probes landed close enough in wall-clock time that ClickHouse had not yet cleared the
	// previous one's query_id from its own registry (found running the whole shell module's test
	// suite, not this package alone — a real per-connection uniqueness requirement this test
	// file's own convenience helper was violating, not a production adapter bug: a real op.OpID is
	// always unique per operation). t.Name() alone is not enough either: a subtest connecting twice
	// (the cancel test's own `a` and `side`) shares one t.Name() for both calls.
	opID := "connect-" + t.Name() + "-" + opNonce + "-" + strconv.FormatInt(connectCounter.Add(1), 10)
	if _, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx(opID)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

var versionRE = testsupport.VersionPattern("ClickHouse", testsupport.ClickHouseServerMajor())

func derefStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestClickHouse(t *testing.T) {
	fixture := testsupport.StartClickHouse(t)
	testsupport.ClickHouseRun(t, fixture)
	cfg := fixture.Config

	t.Run("connect/disconnect, real server version", func(t *testing.T) {
		a := newAdapter(t)
		info, err := a.Connect(context.Background(), cfg, opCtx(t, "op-1"))
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
		a := newAdapter(t)
		bad := cfg
		wrong := "definitely-wrong"
		bad.Password = &wrong
		_, err := a.Connect(context.Background(), bad, opCtx(t, "op-2"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeAuth {
			t.Fatalf("got %v, want an E_AUTH *adapters.Error", err)
		}
	})

	t.Run("tree enumeration", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()

		dbs, err := a.Children(ctx, nodePath(cfg.ID), opCtx(t, "op-3a"))
		if err != nil {
			t.Fatalf("Children(root): %v", err)
		}
		dbNames := childNames(t, dbs)
		// D15: system is genuinely browsable and kept, unlike mysql-family's own SYSTEM_SCHEMAS hiding.
		if !containsName(dbNames, "kira_test") || !containsName(dbNames, "default") || !containsName(dbNames, "system") {
			t.Errorf("databases = %v, want kira_test, default and system all present", dbNames)
		}

		objects, err := a.Children(ctx, nodePath(cfg.ID, seg("database", "kira_test")), opCtx(t, "op-3b"))
		if err != nil {
			t.Fatalf("Children(database): %v", err)
		}
		kindOf := make(map[string]string)
		for _, n := range objects.Nodes {
			kindOf[n.Name] = n.Kind
		}
		if kindOf["orders"] != "table" || kindOf["order_summary"] != "view" || kindOf["order_summary_mv"] != "matview" {
			t.Errorf("object kinds = %+v, want orders=table order_summary=view order_summary_mv=matview", kindOf)
		}
		for _, n := range objects.Nodes {
			if n.HasChildren {
				t.Errorf("object %q HasChildren = true, want false (P19 D5: every relation is a leaf)", n.Name)
			}
		}
	})

	t.Run("describe: no primary key, a sparse primary index, no foreign keys", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		meta, err := a.Describe(context.Background(), nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "order_items")), opCtx(t, "op-5"))
		if err != nil {
			t.Fatalf("Describe: %v", err)
		}
		// D18: a MergeTree PRIMARY KEY is a sparse index, not a uniqueness constraint — never
		// claimed as ObjectMeta.PrimaryKey.
		if meta.PrimaryKey != nil {
			t.Errorf("PrimaryKey = %v, want nil (D18)", meta.PrimaryKey)
		}
		if len(meta.ForeignKeys) != 0 {
			t.Errorf("ForeignKeys = %v, want none (F17: ClickHouse has no such concept)", meta.ForeignKeys)
		}
		foundPrimary := false
		for _, idx := range meta.Indexes {
			if idx.Primary {
				foundPrimary = true
			}
		}
		if !foundPrimary {
			t.Error("expected a synthesized primary sparse-index entry")
		}
	})

	// P22c §4.1: SchemaColumns is Describe's schema-wide sibling — every relation in one database
	// together with its columns, in one round trip, byte-identical to what Describe reports.
	t.Run("SchemaColumns: relations with their columns, byte-identical to Describe", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()

		if !a.Caps().SchemaColumns {
			t.Error("Caps().SchemaColumns = false, want true")
		}
		relations, err := a.SchemaColumns(ctx, nodePath(cfg.ID, seg("database", "kira_test")), opCtx(t, "op-schema-columns-1"))
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
		summaryMv, ok := byName["order_summary_mv"]
		if !ok || summaryMv.Kind != "matview" {
			t.Fatalf("relations = %v, want to find order_summary_mv as a matview", relations)
		}
		if len(summary.Columns) == 0 {
			t.Error("order_summary (a view) has no columns")
		}
		for _, c := range orderItems.Columns {
			// D18/D23: IsPrimaryKey is always false — a MergeTree PRIMARY KEY is a sparse index,
			// not a uniqueness constraint, exactly like toColumnMeta's own per-object rule.
			if c.IsPrimaryKey {
				t.Errorf("column %q: IsPrimaryKey = true, want false (D18/D23)", c.Name)
			}
		}

		meta, err := a.Describe(ctx, nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "order_items")), opCtx(t, "op-schema-columns-2"))
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
		}

		// An empty container ("default" — nothing is ever created there by the seed) returns an
		// empty slice, not an error.
		empty, err := a.SchemaColumns(ctx, nodePath(cfg.ID, seg("database", "default")), opCtx(t, "op-schema-columns-3"))
		if err != nil {
			t.Fatalf("SchemaColumns(default): %v", err)
		}
		if len(empty) != 0 {
			t.Errorf("SchemaColumns(default) = %v, want an empty slice", empty)
		}
	})

	t.Run("row estimate on big_rows", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		objects, err := a.Children(context.Background(), nodePath(cfg.ID, seg("database", "kira_test")), opCtx(t, "op-6"))
		if err != nil {
			t.Fatalf("Children: %v", err)
		}
		bigRowsDetail := regexp.MustCompile(`^~[\d.]+[A-Za-z]*$`)
		for _, n := range objects.Nodes {
			if n.Name == "big_rows" && (n.Detail == nil || !bigRowsDetail.MatchString(*n.Detail)) {
				t.Errorf("big_rows detail = %v, want a \"~N\" estimate", n.Detail)
			}
		}
	})

	t.Run("definition renders create_table_query verbatim, plus Table properties", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		def, err := a.Definition(context.Background(), nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")), opCtx(t, "op-7"))
		if err != nil {
			t.Fatalf("Definition: %v", err)
		}
		if len(def.Statements) != 1 || def.Statements[0] == "" {
			t.Fatalf("Statements = %v, want exactly one non-empty CREATE TABLE", def.Statements)
		}
		if len(def.Sections) != 1 || def.Sections[0].Title != "Table properties" {
			t.Fatalf("Sections = %+v, want a single \"Table properties\" section", def.Sections)
		}
	})

	t.Run("quoting: a doubled-backtick table name and a space in an identifier", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()

		p1, err := a.Read(ctx, adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "weird`name")),
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, opCtx(t, "op-8a"))
		if err != nil {
			t.Fatalf("Read(weird`name): %v", err)
		}
		if tp := p1.(page.TabularPage); tp.RowCount != 1 {
			t.Errorf("weird`name RowCount = %d, want 1", tp.RowCount)
		}

		p2, err := a.Read(ctx, adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "Order Items")),
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, opCtx(t, "op-8b"))
		if err != nil {
			t.Fatalf("Read(Order Items): %v", err)
		}
		if tp := p2.(page.TabularPage); tp.RowCount != 1 {
			t.Errorf("Order Items RowCount = %d, want 1", tp.RowCount)
		}
	})

	// Unlike MySQL/MariaDB, ClickHouse's identifier lexer processes backslash escapes the
	// same as a string literal — quoteIdent must double a raw backslash, not just a backtick, or
	// the generated query never reaches this table (a trailing "\`" is read as an escaped
	// literal backtick, not the identifier's closing quote).
	t.Run("quoting: a table name containing a raw backslash", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "kira_test"), seg("table", `trail\`)),
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, opCtx(t, "op-8c"))
		if err != nil {
			t.Fatalf(`Read(trail\): %v`, err)
		}
		if tp := p.(page.TabularPage); tp.RowCount != 1 {
			t.Errorf(`trail\ RowCount = %d, want 1`, tp.RowCount)
		}
	})

	t.Run("read: first page", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, opCtx(t, "op-9"))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if tp := p.(page.TabularPage); tp.RowCount != 2 || tp.Position.Strategy != "offset" {
			t.Errorf("RowCount/Strategy = %d/%s, want 2/offset", tp.RowCount, tp.Position.Strategy)
		}
	})

	t.Run("a keyset cursor is refused with E_UNSUPPORTED, not silently downgraded", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		_, err := a.Read(context.Background(), adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			PageSize: 10, Cursor: model.PageCursor{Mode: "after", Token: "whatever"},
		}, opCtx(t, "op-10"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
			t.Fatalf("got %v, want E_UNSUPPORTED (D20: no total order to build a keyset cursor on)", err)
		}
	})

	t.Run("count", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		result, err := a.Count(context.Background(), adapters.CountRequest{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
		}, opCtx(t, "op-11"))
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if result.Value != 2 || !result.Exact {
			t.Errorf("Count = %+v, want {2, true}", result)
		}
	})

	// Projection/ServerFilter are both declared true in caps.go.
	// Wired through scenarios.go's shared constructors rather than hand-rolled, so this scenario
	// also backs Tier 2 via RunMatrix's own Then slices for the same capability.
	testsupport.RunScenarios(t, connectedAdapter(t, cfg), cfg,
		testsupport.FilterNarrowsResult(
			nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			"region_id = 1", 1, // only Acme Co is in region 1
		),
		testsupport.ProjectionLimitsColumns(
			nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "customers")),
			[]string{"name"},
		),
		testsupport.ConsoleResultCap(nodePath(cfg.ID, seg("database", "kira_test")), func(n int) string {
			return fmt.Sprintf("SELECT number FROM numbers(%d)", n)
		}),
	)

	// P26 §3.1(3), the phase's flagship test for this adapter: catalog.go's own parser is exercised
	// only against strings this repo wrote (catalog_test.go); this is the one place it meets
	// ClickHouse's own re-rendered create_table_query output for DDL this test itself issued.
	t.Run("execute: a DDL round trip is visible to the catalog", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()
		databasePath := nodePath(cfg.ID, seg("database", "kira_test"))
		tablePath := nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "p26_scratch"))

		testsupport.AdminStatements(t, fixture, "DROP TABLE IF EXISTS kira_test.p26_scratch")
		t.Cleanup(func() { testsupport.AdminStatements(t, fixture, "DROP TABLE IF EXISTS kira_test.p26_scratch") })

		pages, err := a.Execute(ctx, model.ConsoleRequest{
			Path:       databasePath,
			Statements: []string{"CREATE TABLE kira_test.p26_scratch (id UInt32, name String) ENGINE = MergeTree ORDER BY id"},
		}, opCtx(t, "op-ddl-1"))
		if err != nil {
			t.Fatalf("Execute(CREATE TABLE): %v", err)
		}
		created := pages[0].(page.TabularPage)
		if len(created.Columns) != 1 || created.Columns[0].Name != "status" {
			t.Errorf("CREATE TABLE page = %+v, want a status page, not a row page", created.Columns)
		}

		children, err := a.Children(ctx, databasePath, opCtx(t, "op-ddl-2"))
		if err != nil {
			t.Fatalf("Children: %v", err)
		}
		if !containsName(childNames(t, children), "p26_scratch") {
			t.Fatalf("Children(kira_test) = %v, want p26_scratch present", childNames(t, children))
		}

		meta, err := a.Describe(ctx, tablePath, opCtx(t, "op-ddl-3"))
		if err != nil {
			t.Fatalf("Describe: %v", err)
		}
		if len(meta.Columns) != 2 {
			t.Fatalf("Columns = %+v, want exactly 2 (id, name)", meta.Columns)
		}
		foundPrimary := false
		for _, idx := range meta.Indexes {
			if idx.Primary {
				foundPrimary = true
			}
		}
		if !foundPrimary {
			t.Error("expected a synthesized primary sparse-index entry (D18), same as a seeded table's own")
		}

		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path:       databasePath,
			Statements: []string{"ALTER TABLE kira_test.p26_scratch ADD COLUMN note String"},
		}, opCtx(t, "op-ddl-4")); err != nil {
			t.Fatalf("Execute(ALTER TABLE): %v", err)
		}
		meta2, err := a.Describe(ctx, tablePath, opCtx(t, "op-ddl-5"))
		if err != nil {
			t.Fatalf("Describe after ALTER: %v", err)
		}
		if len(meta2.Columns) != 3 {
			t.Fatalf("Columns after ALTER = %+v, want exactly 3", meta2.Columns)
		}

		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path:       databasePath,
			Statements: []string{"DROP TABLE kira_test.p26_scratch"},
		}, opCtx(t, "op-ddl-6")); err != nil {
			t.Fatalf("Execute(DROP TABLE): %v", err)
		}
		childrenAfter, err := a.Children(ctx, databasePath, opCtx(t, "op-ddl-7"))
		if err != nil {
			t.Fatalf("Children after DROP: %v", err)
		}
		if containsName(childNames(t, childrenAfter), "p26_scratch") {
			t.Errorf("Children(kira_test) after DROP = %v, must not contain p26_scratch", childNames(t, childrenAfter))
		}
	})

	t.Run("preview never executes", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "regions")),
			Ops: []model.MutationRowOp{{
				Kind: "insert", Values: model.RowValues{{Name: "id", Value: strp("99")}, {Name: "name", Value: strp("should not land")}},
			}},
		}
		statements, err := a.Preview(plan)
		if err != nil {
			t.Fatalf("Preview: %v", err)
		}
		if len(statements) != 1 {
			t.Fatalf("statements = %v, want exactly 1", statements)
		}
		count, err := a.Count(context.Background(), adapters.CountRequest{Path: plan.Path}, opCtx(t, "op-12-count"))
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if count.Value != 2 {
			t.Errorf("Count after Preview = %d, want unchanged at 2 (preview must never execute)", count.Value)
		}
	})

	t.Run("mutate: insert lands, affectedRows", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "regions")),
			Ops: []model.MutationRowOp{{
				Kind: "insert", Values: model.RowValues{{Name: "id", Value: strp("50")}, {Name: "name", Value: strp("LATAM")}},
			}},
		}
		result, err := a.Mutate(context.Background(), plan, opCtx(t, "op-13"))
		if err != nil {
			t.Fatalf("Mutate: %v", err)
		}
		if result.AffectedRows != 1 {
			t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
		}
	})

	t.Run("mutate: update/delete are E_UNSUPPORTED (D24/D25: no addressable row)", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "regions")),
			Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "id", Value: strp("1")}}}},
		}
		_, err := a.Mutate(context.Background(), plan, opCtx(t, "op-14"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
			t.Fatalf("got %v, want E_UNSUPPORTED", err)
		}
	})

	t.Run("mutate: unknown column is E_NOT_FOUND", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "regions")),
			Ops: []model.MutationRowOp{{
				Kind: "insert", Values: model.RowValues{{Name: "no_such_column", Value: strp("x")}},
			}},
		}
		_, err := a.Mutate(context.Background(), plan, opCtx(t, "op-15"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeNotFound {
			t.Fatalf("got %v, want E_NOT_FOUND", err)
		}
	})

	t.Run("read-only connection cannot write", func(t *testing.T) {
		a := connectedAdapter(t, fixture.ReadOnlyConfig)
		plan := model.MutationPlan{
			Path: nodePath(fixture.ReadOnlyConfig.ID, seg("database", "kira_test"), seg("table", "regions")),
			Ops: []model.MutationRowOp{{
				Kind: "insert", Values: model.RowValues{{Name: "id", Value: strp("60")}, {Name: "name", Value: strp("nope")}},
			}},
		}
		_, err := a.Mutate(context.Background(), plan, opCtx(t, "op-16"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
			t.Fatalf("got %v, want E_UNSUPPORTED", err)
		}
	})

	t.Run("execute: one page per statement, row-returning vs command classification", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		pages, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path: nodePath(cfg.ID, seg("database", "kira_test")),
			Statements: []string{
				"SELECT id, name FROM customers ORDER BY id",
				"INSERT INTO regions (id, name) VALUES (70, 'console-insert')",
			},
		}, opCtx(t, "op-17"))
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
	})

	// P168 Part 3 F11: the *Strings formats render NULL as a sentinel a real string can equal.
	t.Run("NULL sentinel text: a real string is a value, a real NULL is NULL", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()
		dbPath := nodePath(cfg.ID, seg("database", "kira_test"))
		if _, err := a.Execute(ctx, model.ConsoleRequest{Path: dbPath, Statements: []string{
			"DROP TABLE IF EXISTS p168_sentinel",
			"CREATE TABLE p168_sentinel (id UInt8, n Nullable(String), s String) ENGINE = Memory",
			"INSERT INTO p168_sentinel VALUES (1, NULL, 'ᴺᵁᴸᴸ'), (2, 'ᴺᵁᴸᴸ', 'x')",
		}}, opCtx(t, "op-sentinel-setup")); err != nil {
			t.Fatalf("setup: %v", err)
		}
		defer a.Execute(ctx, model.ConsoleRequest{Path: dbPath, Statements: []string{"DROP TABLE p168_sentinel"}}, opCtx(t, "op-sentinel-cleanup")) //nolint:errcheck

		p, err := a.Read(ctx, adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "p168_sentinel")),
			Sort:     &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}},
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, opCtx(t, "op-sentinel-read"))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		tp := p.(page.TabularPage)
		if n := cellAt(t, tp, 1, 0); n != nil {
			t.Errorf("row 1 n = %v, want a real NULL", derefStr(n))
		}
		if s := cellAt(t, tp, 2, 0); s == nil || *s != "ᴺᵁᴸᴸ" {
			t.Errorf("row 1 s = %v, want the literal sentinel text", derefStr(s))
		}
		if n := cellAt(t, tp, 1, 1); n == nil || *n != "ᴺᵁᴸᴸ" {
			t.Errorf("row 2 n = %v, want the literal sentinel text, not NULL", derefStr(n))
		}
		if len(tp.Columns) != 3 {
			t.Errorf("columns = %d, want 3 (the isNull flag is not a visible column)", len(tp.Columns))
		}

		pages, err := a.Execute(ctx, model.ConsoleRequest{Path: dbPath, Statements: []string{"SELECT s FROM p168_sentinel ORDER BY id"}}, opCtx(t, "op-sentinel-console"))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if s := cellAt(t, pages[0].(page.TabularPage), 0, 0); s == nil || *s != "ᴺᵁᴸᴸ" {
			t.Errorf("console s row 1 = %v, want the literal sentinel text from a non-Nullable column", derefStr(s))
		}
	})

	// P168 Part 3 F12: a leading # comment, a parenthesised SELECT and a FROM-first query all
	// return rows, like the --/block-comment forms.
	t.Run("execute: # comment, parenthesised and FROM-first queries return rows", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		pages, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path:       nodePath(cfg.ID, seg("database", "kira_test")),
			Statements: []string{"# note\nSELECT 42", "(SELECT 43)", "FROM system.one SELECT 44"},
		}, opCtx(t, "op-rowreturning"))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		for i, want := range []string{"42", "43", "44"} {
			tp := pages[i].(page.TabularPage)
			if got := cellAt(t, tp, 0, 0); tp.RowCount != 1 || got == nil || *got != want {
				t.Errorf("statement %d = %d rows, first cell %v, want 1 row of %q", i, tp.RowCount, derefStr(got), want)
			}
		}
	})

	// P168 Part 3 F13: a tab or newline in an identifier travels in a catalog query parameter.
	t.Run("quoting: a table name containing a tab and a newline", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()
		dbPath := nodePath(cfg.ID, seg("database", "kira_test"))
		const table = "p168_tab\tnew\nline"
		if _, err := a.Execute(ctx, model.ConsoleRequest{Path: dbPath, Statements: []string{
			"DROP TABLE IF EXISTS `p168_tab\\tnew\\nline`",
			"CREATE TABLE `p168_tab\\tnew\\nline` (x UInt8) ENGINE = Memory",
			"INSERT INTO `p168_tab\\tnew\\nline` VALUES (7)",
		}}, opCtx(t, "op-tabname-setup")); err != nil {
			t.Fatalf("setup: %v", err)
		}
		defer a.Execute(ctx, model.ConsoleRequest{Path: dbPath, Statements: []string{"DROP TABLE `p168_tab\\tnew\\nline`"}}, opCtx(t, "op-tabname-cleanup")) //nolint:errcheck
		path := nodePath(cfg.ID, seg("database", "kira_test"), seg("table", table))
		if _, err := a.Describe(ctx, path, opCtx(t, "op-tabname-describe")); err != nil {
			t.Fatalf("Describe: %v", err)
		}
		count, err := a.Count(ctx, adapters.CountRequest{Path: path}, opCtx(t, "op-tabname-count"))
		if err != nil || count.Value != 1 {
			t.Fatalf("Count = %+v, %v, want 1", count, err)
		}
	})

	t.Run("execute: a failing statement rejects the whole call", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		_, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path:       nodePath(cfg.ID, seg("database", "kira_test")),
			Statements: []string{"SELECT 1", "SELECT * FROM no_such_table_at_all"},
		}, opCtx(t, "op-18"))
		if err == nil {
			t.Fatal("expected the second (invalid) statement to fail the whole call")
		}
	})

	// §5.5's own three highest-weight scenarios for judging B11, combined into one read.
	t.Run("wide types: exact cell values, big integers keep every digit, NULL is not \"null\", NaN is not NULL", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "kira_test"), seg("table", "wide_types")),
			Sort:     &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}},
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, opCtx(t, "op-19"))
		if err != nil {
			t.Fatalf("Read(wide_types): %v", err)
		}
		tp := p.(page.TabularPage)
		colIdx := make(map[string]int, len(tp.Columns))
		for i, c := range tp.Columns {
			colIdx[c.Name] = i
		}

		en := cellAt(t, tp, colIdx["en"], 0)
		if en == nil || *en != "green" {
			t.Errorf("row0 en = %v, want \"green\"", derefStr(en))
		}
		uid := cellAt(t, tp, colIdx["uid"], 0)
		if uid == nil || *uid != "61f0c404-5cb3-11e7-907b-a6006ad3dba0" {
			t.Errorf("row0 uid = %v, want the exact UUID text", derefStr(uid))
		}
		// Decimal128(20) holds 20 fractional digits; the seed's own literal
		// ('123456789012345678.12345678901234567890') has a trailing zero beyond the type's own
		// precision, which ClickHouse itself drops on storage — this is the server's own real
		// output, confirmed against a live container, not a value this adapter reformats.
		dec := cellAt(t, tp, colIdx["dec"], 0)
		if dec == nil || *dec != "123456789012345678.1234567890123456789" {
			t.Errorf("row0 dec = %v, want every Decimal128 digit preserved", derefStr(dec))
		}
		bigUint := cellAt(t, tp, colIdx["big_uint"], 0)
		if bigUint == nil || *bigUint != "18446744073709551615" {
			t.Errorf("row0 big_uint = %v, want the exact max-UInt64 text", derefStr(bigUint))
		}

		// row 1 (id=2): nullable_val is a real NULL; row 0's own nullable_val is also a real NULL.
		nullableRow0 := cellAt(t, tp, colIdx["nullable_val"], 0)
		if nullableRow0 != nil {
			t.Errorf("row0 nullable_val = %v, want a real NULL", derefStr(nullableRow0))
		}
		nullableRow1 := cellAt(t, tp, colIdx["nullable_val"], 1)
		if nullableRow1 == nil || *nullableRow1 != "null" {
			t.Errorf(`row1 nullable_val = %v, want the four-character string "null", not NULL`, derefStr(nullableRow1))
		}
		floatRow1 := cellAt(t, tp, colIdx["float_val"], 1)
		if floatRow1 == nil || *floatRow1 != "nan" {
			t.Errorf(`row1 float_val = %v, want the three-character string "nan", not NULL`, derefStr(floatRow1))
		}
		typeClassOfNullable := tp.Columns[colIdx["nullable_val"]]
		if !typeClassOfNullable.Nullable {
			t.Error("nullable_val column must report Nullable = true")
		}
	})

	// The scenario that must not be softened: a real slow
	// query, a real KILL QUERY from a separate side connection's own HTTP request, and — the
	// assertion that matters — system.processes confirms the query is gone afterward, not
	// merely that the local call returned.
	t.Run("cancel, asserted server-side", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		side := connectedAdapter(t, cfg)

		ctx, cancel := context.WithCancel(context.Background())
		op := opCtx(t, "op-cancel")

		errCh := make(chan error, 1)
		go func() {
			_, err := a.Execute(ctx, model.ConsoleRequest{
				Path:       nodePath(cfg.ID, seg("database", "kira_test")),
				Statements: []string{"SELECT sleep(3) FROM numbers(20)"},
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
		time.Sleep(1500 * time.Millisecond) // let the statement actually reach the server
		cancel()                            // unblocks the local wait only — see query.go's own doc comment

		ok, err := a.Cancel(context.Background(), op.OpID)
		if err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		if !ok {
			t.Error("Cancel reported false, want true (a real running query)")
		}

		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Fatal("Execute never returned after cancel()")
		}

		// adapter.go's own nextQueryID scheme: "kira-<opID>-<seq>", seq 0 for this op's one
		// statement — checked by query_id, not by matching the query's own SQL text: a LIKE '%...%'
		// filter on `query` would match this very checking statement's own text (its WHERE clause
		// literally contains the substring being searched for), producing a permanent false
		// positive that never clears.
		waitForProcessesClean(t, side, "kira-"+op.OpID+"-0", 5*time.Second)
	})
}

// waitForProcessesClean polls system.processes over a separate, already-connected adapter until the
// given query_id is gone — the real proof KILL QUERY reached the server, not just that the local
// call returned.
func waitForProcessesClean(t *testing.T, side adapters.Adapter, queryID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		pages, err := side.Execute(context.Background(), model.ConsoleRequest{
			Statements: []string{"SELECT count() AS n FROM system.processes WHERE query_id = '" + queryID + "'"},
		}, opCtx(t, "op-processes-check"))
		if err != nil {
			t.Fatalf("system.processes check: %v", err)
		}
		tp := pages[0].(page.TabularPage)
		n := cellAt(t, tp, 0, 0)
		if n != nil && *n == "0" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sleep(3) still present in system.processes after %s", timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// DB MCP read lane: readonly=2 on every request makes the server refuse the write.
func TestClickHouse_ReadLane(t *testing.T) {
	fixture := testsupport.StartClickHouse(t)
	testsupport.ClickHouseRun(t, fixture)
	cfg := fixture.Config
	path := nodePath(cfg.ID)
	unchanged := func(t *testing.T, main adapters.Adapter) {
		t.Helper()
		if got := testsupport.FirstCell(t, main, path, "SELECT count() FROM kira_lane_log"); got != "0" {
			t.Errorf("kira_lane_log rows = %s, want 0", got)
		}
	}
	testsupport.RunScenarios(t, connectedAdapter(t, cfg), cfg,
		testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
			Path: path, Setup: []string{"CREATE TABLE kira_lane_log (id UInt8) ENGINE = Memory"}, Write: "INSERT INTO kira_lane_log VALUES (1)",
			WantErr: "readonly mode", Unchanged: unchanged, Teardown: []string{"DROP TABLE kira_lane_log"},
		}),
		testsupport.LaneCancel(path, "SELECT sleepEachRow(1) FROM numbers(30) SETTINGS max_block_size = 1"),
	)
}
