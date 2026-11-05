// Package sqlite_test is the Go analogue of packages/db-fixtures/sqlite.spec.ts. Not every one of
// the spec's 43 scenarios has a Go twin — the ones ported are the load-bearing behaviours the
// "adapter-first-test-first" rule exists to protect, plus the four new/changed cases (the
// value-follows-the-value codec, the real modernc.org/sqlite decltype coercion, the real
// cancellation, and the mattn-regression-class "cancel then reuse" case).
package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/sqlite"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testsupport.StopSqlite()
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
	a, err := adapters.CreateAdapter("sqlite", deps)
	if err != nil {
		t.Fatalf("CreateAdapter(sqlite): %v", err)
	}
	return a
}

func connectedAdapter(t *testing.T, cfg model.ResolvedConnectionConfig) adapters.Adapter {
	t.Helper()
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

var versionRE = regexp.MustCompile(`^SQLite 3\.`)

func TestSqlite(t *testing.T) {
	fixture := testsupport.StartSqlite(t)
	cfg := fixture.Config

	// P168 Part 3 F9: Connect honours its ctx (Part 2's abort of an in-flight Connect depends on it).
	t.Run("connect: a cancelled ctx aborts with E_CANCELLED and leaves nothing connected", func(t *testing.T) {
		a := newAdapter(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := a.Connect(ctx, cfg, adapters.NewOpCtx("op-connect-cancelled"))
		if code, _ := adapters.CodeOf(err); code != adapters.CodeCancelled {
			t.Fatalf("Connect with a cancelled ctx: err = %v, want E_CANCELLED", err)
		}
		_, err = a.Count(context.Background(), adapters.CountRequest{
			Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
		}, adapters.NewOpCtx("op-after-cancelled-connect"))
		if code, _ := adapters.CodeOf(err); code != adapters.CodeConnect {
			t.Fatalf("op after a cancelled Connect: err = %v, want E_CONNECT", err)
		}
	})

	// P168 Part 3 F8: ops racing Disconnect either run or are refused, never Add to a WaitGroup a
	// Wait is already running on (a panic) or run on a database whose close is pending. Run with -race.
	t.Run("disconnect racing new ops neither panics nor leaks work onto a closing database", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			a := newAdapter(t)
			if _, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-race-connect")); err != nil {
				t.Fatalf("Connect: %v", err)
			}
			var wg sync.WaitGroup
			for j := 0; j < 8; j++ {
				wg.Add(1)
				go func(j int) {
					defer wg.Done()
					_, err := a.Count(context.Background(), adapters.CountRequest{
						Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
					}, adapters.NewOpCtx("op-race-"+strconv.Itoa(j)))
					if err == nil {
						return
					}
					if code, _ := adapters.CodeOf(err); code != adapters.CodeConnect && code != adapters.CodeCancelled && code != adapters.CodeQuery {
						t.Errorf("Count racing Disconnect: err = %v", err)
					}
				}(j)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = a.Disconnect(context.Background())
			}()
			wg.Wait()
		}
	})

	t.Run("connect/disconnect, real server version", func(t *testing.T) {
		a := newAdapter(t)
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

	t.Run("connect against a missing file is E_NOT_FOUND", func(t *testing.T) {
		a := newAdapter(t)
		bad := cfg
		missing := fixture.Dir + "/does-not-exist.sqlite"
		bad.Database = &missing
		_, err := a.Connect(context.Background(), bad, adapters.NewOpCtx("op-2"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeNotFound {
			t.Fatalf("got %v, want E_NOT_FOUND", err)
		}
	})

	t.Run("connect against a garbage (non-database) file is E_CONNECT, and creates no sidecar", func(t *testing.T) {
		garbagePath := fixture.Dir + "/garbage.sqlite"
		if err := os.WriteFile(garbagePath, []byte("this is not a sqlite database"), 0o600); err != nil {
			t.Fatalf("write garbage file: %v", err)
		}
		a := newAdapter(t)
		bad := cfg
		bad.Database = &garbagePath
		_, err := a.Connect(context.Background(), bad, adapters.NewOpCtx("op-3"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeConnect {
			t.Fatalf("got %v, want E_CONNECT", err)
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			if _, statErr := os.Stat(garbagePath + suffix); statErr == nil {
				t.Errorf("a failed connect left a %s sidecar behind", suffix)
			}
		}
	})

	t.Run("tree enumeration: databases, table/view kinds, FTS5 shadow tables hidden", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()

		dbs, err := a.Children(ctx, nodePath(cfg.ID), adapters.NewOpCtx("op-4a"))
		if err != nil {
			t.Fatalf("Children(root): %v", err)
		}
		if !containsName(childNames(t, dbs), "main") {
			t.Errorf("databases = %v, want main", childNames(t, dbs))
		}

		objects, err := a.Children(ctx, nodePath(cfg.ID, seg("database", "main")), adapters.NewOpCtx("op-4b"))
		if err != nil {
			t.Fatalf("Children(database): %v", err)
		}
		kindOf := make(map[string]string)
		for _, n := range objects.Nodes {
			kindOf[n.Name] = n.Kind
		}
		if kindOf["orders"] != "table" || kindOf["order_summary"] != "view" {
			t.Errorf("object kinds = %+v, want orders=table order_summary=view", kindOf)
		}
		// F17/F24: fts_docs is a real table (shown), but its own shadow bookkeeping tables and
		// every sqlite_-prefixed name must never appear.
		if kindOf["fts_docs"] != "table" {
			t.Errorf("fts_docs kind = %q, want table", kindOf["fts_docs"])
		}
		for name := range kindOf {
			if name == "fts_docs_data" || name == "fts_docs_idx" || len(name) >= 7 && name[:7] == "sqlite_" {
				t.Errorf("shadow/internal table %q must not appear in the tree", name)
			}
		}
		for _, n := range objects.Nodes {
			if n.HasChildren {
				t.Errorf("object %q HasChildren = true, want false (P19 D5: every relation is a leaf)", n.Name)
			}
		}
	})

	t.Run("describe", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		meta, err := a.Describe(context.Background(), nodePath(cfg.ID, seg("database", "main"), seg("table", "order_items")), adapters.NewOpCtx("op-6"))
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

	// P22c §4.1: SchemaColumns is Describe's schema-wide sibling — every relation in one container
	// together with its columns, in one query, never one per relation.
	t.Run("SchemaColumns: relations with their columns, byte-identical to Describe", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()

		relations, err := a.SchemaColumns(ctx, nodePath(cfg.ID, seg("database", "main")), adapters.NewOpCtx("op-schema-columns-1"))
		if err != nil {
			t.Fatalf("SchemaColumns: %v", err)
		}
		byName := make(map[string]model.RelationColumns, len(relations))
		for _, rc := range relations {
			byName[rc.Name] = rc
		}
		orders, ok := byName["orders"]
		if !ok || orders.Kind != "table" {
			t.Fatalf("relations = %v, want to find orders as a table", relations)
		}
		orderItems, ok := byName["order_items"]
		if !ok || orderItems.Kind != "table" {
			t.Fatalf("relations = %v, want to find order_items as a table", relations)
		}
		summary, ok := byName["order_summary"]
		if !ok || summary.Kind != "view" {
			t.Fatalf("relations = %v, want to find order_summary as a view", relations)
		}
		if len(orderItems.Columns) == 0 {
			t.Fatal("order_items has no columns")
		}
		// SQLite's own Position is table_xinfo's 0-based `cid`, not a 1-based ordinal (listColumns'
		// own convention, unchanged here).
		for i, col := range orderItems.Columns {
			if col.Position != i {
				t.Errorf("order_items.Columns[%d].Position = %d, want cid %d", i, col.Position, i)
			}
		}

		// Every column's DataType (and IsPrimaryKey) must be byte-identical to what Describe
		// reports for the same column — the property D6's shared supply depends on.
		meta, err := a.Describe(ctx, nodePath(cfg.ID, seg("database", "main"), seg("table", "order_items")), adapters.NewOpCtx("op-schema-columns-2"))
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

		// A view's own columns are still reported — no adapter should special-case it away.
		if len(summary.Columns) == 0 {
			t.Error("order_summary (a view) has no columns")
		}
	})

	t.Run("row estimate: big_rows has one, wide_table (never ANALYZEd) has none", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		objects, err := a.Children(context.Background(), nodePath(cfg.ID, seg("database", "main")), adapters.NewOpCtx("op-7"))
		if err != nil {
			t.Fatalf("Children: %v", err)
		}
		bigRowsDetail := regexp.MustCompile(`^~[\d.]+[A-Za-z]*$`)
		for _, n := range objects.Nodes {
			switch n.Name {
			case "big_rows":
				if n.Detail == nil || !bigRowsDetail.MatchString(*n.Detail) {
					t.Errorf("big_rows detail = %v, want a \"~N\" estimate", n.Detail)
				}
			case "wide_table":
				if n.Detail != nil {
					t.Errorf("wide_table detail = %v, want nil (never ANALYZEd — F20)", *n.Detail)
				}
			}
		}
	})

	t.Run("definition renders sqlite_master's own CREATE TABLE verbatim", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		def, err := a.Definition(context.Background(), nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")), adapters.NewOpCtx("op-8"))
		if err != nil {
			t.Fatalf("Definition: %v", err)
		}
		if len(def.Statements) != 1 || def.Statements[0] == "" {
			t.Fatalf("Statements = %v, want exactly one non-empty CREATE TABLE", def.Statements)
		}
	})

	t.Run("quoting: a doubled-double-quote table name and a space in an identifier", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()

		p1, err := a.Read(ctx, adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "main"), seg("table", `weird"name`)),
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-9a"))
		if err != nil {
			t.Fatalf(`Read(weird"name): %v`, err)
		}
		if tp := p1.(page.TabularPage); tp.RowCount != 1 {
			t.Errorf(`weird"name RowCount = %d, want 1`, tp.RowCount)
		}

		p2, err := a.Read(ctx, adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "main"), seg("table", "Order Items")),
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-9b"))
		if err != nil {
			t.Fatalf("Read(Order Items): %v", err)
		}
		if tp := p2.(page.TabularPage); tp.RowCount != 1 {
			t.Errorf("Order Items RowCount = %d, want 1", tp.RowCount)
		}
	})

	t.Run("read: first page", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-10"))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if tp := p.(page.TabularPage); tp.RowCount != 2 {
			t.Errorf("RowCount = %d, want 2", tp.RowCount)
		}
	})

	// P168 Part 3 F4: a binary key's token must bind as raw bytes, not its 0x<hex> display text.
	t.Run("read: keyset over a blob primary key terminates and covers every row", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()
		dbPath := nodePath(cfg.ID, seg("database", "main"))
		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path: dbPath,
			Statements: []string{
				"DROP TABLE IF EXISTS p168_kbin",
				"CREATE TABLE p168_kbin (id BLOB PRIMARY KEY)",
				"INSERT INTO p168_kbin VALUES (x'01'), (x'0A'), (x'30'), (x'7F'), (x'80'), (x'81FF'), (x'FF'), (x'FFFF')",
			},
		}, adapters.NewOpCtx("op-kbin-setup")); err != nil {
			t.Fatalf("setup: %v", err)
		}
		defer a.Execute(ctx, model.ConsoleRequest{Path: dbPath, Statements: []string{"DROP TABLE p168_kbin"}}, adapters.NewOpCtx("op-kbin-cleanup")) //nolint:errcheck
		sort := &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}}
		got := testsupport.KeysetForwardIDs(t, a, nodePath(cfg.ID, seg("database", "main"), seg("table", "p168_kbin")), sort, 2, 8)
		want := []string{"0x01", "0x0a", "0x30", "0x7f", "0x80", "0x81ff", "0xff", "0xffff"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	})

	t.Run("read: keyset forward and back over big_rows", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()
		tablePath := nodePath(cfg.ID, seg("database", "main"), seg("table", "big_rows"))
		sort := &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}}

		first, err := a.Read(ctx, adapters.ReadRequest{
			Path: tablePath, Sort: sort, PageSize: 5, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-11a"))
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
		}, adapters.NewOpCtx("op-11b"))
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
		}, adapters.NewOpCtx("op-11c"))
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
		}, adapters.NewOpCtx("op-11d"))
		if err != nil {
			t.Fatalf("Read(after, from page-before's NextToken): %v", err)
		}
		forwardAgainPage := forwardAgain.(page.TabularPage)
		forwardAgainFirstID := cellAt(t, forwardAgainPage, 0, 0)
		if forwardAgainFirstID == nil || *forwardAgainFirstID != "6" {
			t.Errorf("page after page-before's NextToken: first id = %v, want 6", forwardAgainFirstID)
		}
	})

	t.Run("read: keyset paging by the implicit rowid (F23/D22/D23)", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		target := nodePath(cfg.ID, seg("database", "main"), seg("table", "no_pk_rowid"))
		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path: target, PageSize: 2, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-12"))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		tp := p.(page.TabularPage)
		if tp.Position.Strategy != "keyset" {
			t.Errorf("Position.Strategy = %q, want keyset (a rowid table has an implicit tiebreaker)", tp.Position.Strategy)
		}
		for _, c := range tp.Columns {
			if c.Name == "rowid" {
				t.Error("rowid must never appear as a page column (D23: it is not mutation identity)")
			}
		}
		if tp.Position.NextToken == nil {
			t.Fatal("expected a NextToken")
		}
	})

	// caps.go declares ServerFilter/Projection both true; there was no Filter test at all before
	// this, and Projection appeared only inside a value-codec test. Wired through scenarios.go's
	// shared constructors rather than hand-rolled, so this scenario also backs Tier 2 via
	// RunMatrix's own Then slices for the same capability.
	testsupport.RunScenarios(t, connectedAdapter(t, cfg), cfg,
		testsupport.FilterNarrowsResult(
			nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
			"region_id = 1", 1, // only Acme Co is in region 1
		),
		testsupport.ProjectionLimitsColumns(
			nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
			[]string{"name"},
		),
		testsupport.ConsoleResultCap(nodePath(cfg.ID, seg("database", "main")), func(n int) string {
			return fmt.Sprintf("WITH RECURSIVE s(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM s WHERE n < %d) SELECT n FROM s", n)
		}),
	)

	t.Run("execute: console result stops at the byte cap", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		pages, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path:       nodePath(cfg.ID, seg("database", "main")),
			Statements: []string{"WITH RECURSIVE s(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM s WHERE n < 4) SELECT hex(zeroblob(1000)) FROM s"},
			Cap:        page.ResultCap{Bytes: 3000},
		}, adapters.NewOpCtx("op-byte-cap"))
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		p := pages[0].(page.TabularPage)
		// 2000-byte cells: row 2 starts under the cap (2000 < 3000), row 3 starts over it (4000).
		if p.RowCount != 2 || !p.Position.Truncated {
			t.Fatalf("RowCount = %d, Truncated = %v, want 2, true", p.RowCount, p.Position.Truncated)
		}
	})

	t.Run("count", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		result, err := a.Count(context.Background(), adapters.CountRequest{
			Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
		}, adapters.NewOpCtx("op-13"))
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if result.Value != 2 || !result.Exact {
			t.Errorf("Count = %+v, want {2, true}", result)
		}
	})

	t.Run("preview never executes", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
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
		count, err := a.Count(context.Background(), adapters.CountRequest{Path: plan.Path}, adapters.NewOpCtx("op-13-count"))
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if count.Value != 2 {
			t.Errorf("Count after Preview = %d, want unchanged at 2 (preview must never execute)", count.Value)
		}
	})

	t.Run("mutate: update lands, affectedRows", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
			Ops: []model.MutationRowOp{{
				Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}},
				Changes: model.RowValues{{Name: "name", Value: strp("Acme Corp")}},
			}},
		}
		result, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-14"))
		if err != nil {
			t.Fatalf("Mutate: %v", err)
		}
		if result.AffectedRows != 1 {
			t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
		}
	})

	t.Run("mutate: unknown column is E_NOT_FOUND", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
			Ops: []model.MutationRowOp{{
				Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}},
				Changes: model.RowValues{{Name: "no_such_column", Value: strp("x")}},
			}},
		}
		_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-15"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeNotFound {
			t.Fatalf("got %v, want E_NOT_FOUND", err)
		}
	})

	// The "cancellation between BEGIN IMMEDIATE and COMMIT" regression test lives in
	// mutate_internal_test.go (package sqlite), not here: runOnConn deliberately never lets
	// Adapter.Mutate's own outer ctx reach mutate()'s internals — its work runs on an adapter-owned
	// driverCtx, cancelled only by Adapter.Cancel or the goroutine's own completion (abort.go's
	// RunWithAbortRace doc comment calls this sqlite's "opposite polarity" versus postgres/mysql).
	// A black-box flippingCtx passed to a.Mutate here only races runOnConn's own "caller gave up"
	// check — it never lands inside mutate()'s BEGIN IMMEDIATE/COMMIT sequence, so it cannot
	// exercise the fix. Only a direct, same-package call to mutate() can.

	// no_pk_rowid is a genuine no-PK rowid table already in the seed data — used directly rather
	// than creating a throwaway probe table, unlike M6.2's own postgres/mysql-family suites (both
	// of which had to build one because their own seeded "events"-shaped table had a real PK).
	t.Run("mutate: no primary key is E_UNSUPPORTED", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "no_pk_rowid")),
			Ops: []model.MutationRowOp{{
				Kind: "update", Key: model.RowValues{{Name: "label", Value: strp("alpha")}},
				Changes: model.RowValues{{Name: "value", Value: strp("99")}},
			}},
		}
		_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-16"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
			t.Fatalf("got %v, want E_UNSUPPORTED (no_pk_rowid has no primary key)", err)
		}
	})

	// P26 §3.3(3), including the CREATE INDEX step (worth taking here and nowhere else: SQLite is
	// the engine where Describe's index list is cheapest to assert). One implementer trap from this
	// package's own suite (query.go rejects a smuggled second statement in one string, ":494"): each
	// DDL statement here is its own Statements element, never one semicolon-joined string.
	t.Run("execute: a DDL round trip is visible to the catalog", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()
		databasePath := nodePath(cfg.ID, seg("database", "main"))
		tablePath := nodePath(cfg.ID, seg("database", "main"), seg("table", "p26_scratch"))
		t.Cleanup(func() {
			_, _ = a.Execute(context.Background(), model.ConsoleRequest{
				Path: databasePath, Statements: []string{"DROP TABLE IF EXISTS p26_scratch"},
			}, adapters.NewOpCtx("op-ddl-cleanup"))
		})

		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path:       databasePath,
			Statements: []string{"CREATE TABLE p26_scratch (id INTEGER PRIMARY KEY, name TEXT)"},
		}, adapters.NewOpCtx("op-ddl-1")); err != nil {
			t.Fatalf("Execute(CREATE TABLE): %v", err)
		}

		children, err := a.Children(ctx, databasePath, adapters.NewOpCtx("op-ddl-2"))
		if err != nil {
			t.Fatalf("Children: %v", err)
		}
		if !containsName(childNames(t, children), "p26_scratch") {
			t.Fatalf("Children(main) = %v, want p26_scratch present", childNames(t, children))
		}

		meta, err := a.Describe(ctx, tablePath, adapters.NewOpCtx("op-ddl-3"))
		if err != nil {
			t.Fatalf("Describe: %v", err)
		}
		if len(meta.Columns) != 2 {
			t.Fatalf("Columns = %+v, want exactly 2 (id, name)", meta.Columns)
		}

		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path:       databasePath,
			Statements: []string{"ALTER TABLE p26_scratch ADD COLUMN note TEXT"},
		}, adapters.NewOpCtx("op-ddl-4")); err != nil {
			t.Fatalf("Execute(ALTER TABLE): %v", err)
		}
		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path:       databasePath,
			Statements: []string{"CREATE INDEX p26_scratch_name ON p26_scratch(name)"},
		}, adapters.NewOpCtx("op-ddl-5")); err != nil {
			t.Fatalf("Execute(CREATE INDEX): %v", err)
		}

		meta2, err := a.Describe(ctx, tablePath, adapters.NewOpCtx("op-ddl-6"))
		if err != nil {
			t.Fatalf("Describe after ALTER/CREATE INDEX: %v", err)
		}
		if len(meta2.Columns) != 3 {
			t.Fatalf("Columns after ALTER = %+v, want exactly 3", meta2.Columns)
		}
		foundIndex := false
		for _, idx := range meta2.Indexes {
			if idx.Name == "p26_scratch_name" {
				foundIndex = true
			}
		}
		if !foundIndex {
			t.Errorf("Indexes = %+v, want p26_scratch_name present", meta2.Indexes)
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
			t.Errorf("Children(main) after DROP = %v, must not contain p26_scratch", childNames(t, childrenAfter))
		}
	})

	// P26 §3.3(4): a scratch table, so no seeded table is touched (§2.4).
	t.Run("mutate: insert then delete round-trips", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		ctx := context.Background()
		tablePath := nodePath(cfg.ID, seg("database", "main"), seg("table", "p26_insdel"))
		t.Cleanup(func() {
			_, _ = a.Execute(context.Background(), model.ConsoleRequest{
				Path: nodePath(cfg.ID, seg("database", "main")), Statements: []string{"DROP TABLE IF EXISTS p26_insdel"},
			}, adapters.NewOpCtx("op-insdel-cleanup"))
		})

		if _, err := a.Execute(ctx, model.ConsoleRequest{
			Path:       nodePath(cfg.ID, seg("database", "main")),
			Statements: []string{"CREATE TABLE p26_insdel (id INTEGER PRIMARY KEY, name TEXT)"},
		}, adapters.NewOpCtx("op-insdel-setup")); err != nil {
			t.Fatalf("Execute(CREATE TABLE): %v", err)
		}

		testsupport.InsertDeleteRoundTrip(t, a, tablePath)
	})

	t.Run("read-only connection cannot write", func(t *testing.T) {
		ro := cfg
		ro.ReadOnly = true
		a := newAdapter(t)
		if _, err := a.Connect(context.Background(), ro, adapters.NewOpCtx("op-17-connect")); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer a.Disconnect(context.Background())
		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
			Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "id", Value: strp("2")}}}},
		}
		_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-17"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
			t.Fatalf("got %v, want E_UNSUPPORTED", err)
		}
	})

	// P26 §3.3(5): a read-only connection's DSN carries mode=ro (client.go) — the honest
	// SQLite-specific property of a refused write, extending the file-level assertion ":742" already
	// makes for reads to a refused write: no -wal/-shm sidecar appears either.
	t.Run("read-only connection cannot run DDL, and creates no sidecar", func(t *testing.T) {
		ro := cfg
		ro.ReadOnly = true
		a := newAdapter(t)
		if _, err := a.Connect(context.Background(), ro, adapters.NewOpCtx("op-ro-ddl-connect")); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer a.Disconnect(context.Background())

		before, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("read before: %v", err)
		}

		_, err = a.Execute(context.Background(), model.ConsoleRequest{
			Path:       nodePath(cfg.ID, seg("database", "main")),
			Statements: []string{"CREATE TABLE p26_ro_ddl (id INTEGER PRIMARY KEY)"},
		}, adapters.NewOpCtx("op-ro-ddl"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
			t.Fatalf("got %v, want E_UNSUPPORTED (sqlite/errors.go:44-45: primary code 8, readOnly)", err)
		}

		after, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("read after: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Error("a refused read-only DDL attempt modified the database file")
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			if _, statErr := os.Stat(fixture.Path + suffix); statErr == nil {
				t.Errorf("a refused read-only DDL attempt left a %s sidecar behind", suffix)
			}
		}

		side, err := sql.Open("sqlite", "file:"+fixture.Path+"?mode=ro")
		if err != nil {
			t.Fatalf("side connect: %v", err)
		}
		defer side.Close()
		var exists int
		if err := side.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'p26_ro_ddl'").Scan(&exists); err != nil {
			t.Fatalf("sqlite_master check: %v", err)
		}
		if exists != 0 {
			t.Errorf("p26_ro_ddl exists after a read-only Execute attempt, want it never created")
		}
	})

	t.Run("execute: one page per statement", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		pages, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path: nodePath(cfg.ID, seg("database", "main")),
			Statements: []string{
				"SELECT id, name FROM customers ORDER BY id",
				"CREATE TEMPORARY TABLE t_exec_18 (x INT)",
			},
		}, adapters.NewOpCtx("op-18"))
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
		a := connectedAdapter(t, cfg)
		_, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path:       nodePath(cfg.ID, seg("database", "main")),
			Statements: []string{"CREATE TEMPORARY TABLE t_exec_19 (x INT)", "SELECT * FROM no_such_table_at_all"},
		}, adapters.NewOpCtx("op-19"))
		if err == nil {
			t.Fatal("expected the second (invalid) statement to fail the whole call")
		}
	})

	// B9/SQ-1: modernc.org/sqlite executes a smuggled second statement in full rather than
	// silently dropping it the way node:sqlite's own prepare() did — the adapter must reject it
	// itself, before the driver ever sees it.
	t.Run("execute: a smuggled second statement in one string is rejected, not run", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		_, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path:       nodePath(cfg.ID, seg("database", "main")),
			Statements: []string{"CREATE TEMPORARY TABLE t_exec_20 (x INT); DROP TABLE t_exec_20"},
		}, adapters.NewOpCtx("op-20"))
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeQuery {
			t.Fatalf("got %v, want E_QUERY (multiple statements are not supported)", err)
		}
	})

	// B7's whole argument, and spec 35's own table: the codec follows the value's own Go type,
	// never the column's declared type.
	t.Run("the value codec follows the value, not the declared type", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		probe, err := sql.Open("sqlite", "file:"+fixture.Path)
		if err != nil {
			t.Fatalf("probe connect: %v", err)
		}
		defer probe.Close()
		if _, err := probe.Exec("CREATE TABLE IF NOT EXISTS dyn_probe (a TEXT, b INTEGER)"); err != nil {
			t.Fatalf("create probe table: %v", err)
		}
		defer probe.Exec("DROP TABLE IF EXISTS dyn_probe")
		if _, err := probe.Exec("INSERT INTO dyn_probe (a, b) VALUES (?, ?)", []byte{0xde, 0xad, 0xbe, 0xef}, "not a number"); err != nil {
			t.Fatalf("seed probe table: %v", err)
		}

		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path:     nodePath(cfg.ID, seg("database", "main"), seg("table", "dyn_probe")),
			PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-21"))
		if err != nil {
			t.Fatalf("Read(dyn_probe): %v", err)
		}
		tp := p.(page.TabularPage)
		aVal, bVal := cellAt(t, tp, 0, 0), cellAt(t, tp, 1, 0)
		if aVal == nil || !regexp.MustCompile(`^0x[0-9a-f]+$`).MatchString(*aVal) {
			t.Errorf("a (BLOB in a TEXT column) = %v, want 0x<hex>", aVal)
		}
		if bVal == nil || *bVal != "not a number" {
			t.Errorf("b (TEXT in an INTEGER column) = %v, want \"not a number\" verbatim", bVal)
		}
	})

	// A BLOB column's edited value arrives from the grid still spelled in the app's own
	// "0x<hex>" display convention (read.go's cellText above) — Mutate must decode it back into
	// raw bytes before binding it as a parameter, not hand the driver that 6-character ASCII
	// string as if it were the new column content.
	t.Run("mutate: BLOB column edit round-trips as bytes, not its own hex text", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		probe, err := sql.Open("sqlite", "file:"+fixture.Path)
		if err != nil {
			t.Fatalf("probe connect: %v", err)
		}
		defer probe.Close()
		if _, err := probe.Exec("CREATE TABLE IF NOT EXISTS blob_rw (id INTEGER PRIMARY KEY, data BLOB)"); err != nil {
			t.Fatalf("create probe table: %v", err)
		}
		defer probe.Exec("DROP TABLE IF EXISTS blob_rw")
		if _, err := probe.Exec("INSERT INTO blob_rw (id, data) VALUES (1, ?)", []byte{0x01, 0x02, 0x03}); err != nil {
			t.Fatalf("seed probe table: %v", err)
		}

		plan := model.MutationPlan{
			Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "blob_rw")),
			Ops: []model.MutationRowOp{{
				Kind: "update", Key: model.RowValues{{Name: "id", Value: strp("1")}},
				Changes: model.RowValues{{Name: "data", Value: strp("0x0405")}},
			}},
		}
		result, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-blob-rw"))
		if err != nil {
			t.Fatalf("Mutate: %v", err)
		}
		if result.AffectedRows != 1 {
			t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
		}

		var stored []byte
		if err := probe.QueryRow("SELECT data FROM blob_rw WHERE id = 1").Scan(&stored); err != nil {
			t.Fatalf("probe read back: %v", err)
		}
		want := []byte{0x04, 0x05}
		if !bytes.Equal(stored, want) {
			t.Errorf("stored bytes = %#v, want %#v (got the display text instead of decoded bytes?)", stored, want)
		}
	})

	// Unlike node:sqlite, this driver silently re-parses a TEXT value into a Go time.Time whenever
	// the column's declared type is DATE/DATETIME/TIMESTAMP and the text happens to look like a
	// date — read.go's selectExpr routes around it. This test would fail immediately without that
	// fix (every datetime_a/date_a cell would render the Go default time.Time string instead of the
	// server's own stored text).
	t.Run("BOOLEAN renders 0/1, DATETIME/DATE render their stored text unmodified", func(t *testing.T) {
		a := connectedAdapter(t, cfg)
		p, err := a.Read(context.Background(), adapters.ReadRequest{
			Path:       nodePath(cfg.ID, seg("database", "main"), seg("table", "wide_table")),
			Projection: []string{"bool_a", "date_a", "datetime_a"},
			PageSize:   10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-22"))
		if err != nil {
			t.Fatalf("Read(wide_table): %v", err)
		}
		tp := p.(page.TabularPage)
		boolVal := cellAt(t, tp, 0, 0)
		if boolVal == nil || (*boolVal != "0" && *boolVal != "1") {
			t.Errorf("bool_a = %v, want \"0\" or \"1\"", boolVal)
		}
		dateVal := cellAt(t, tp, 1, 0)
		if dateVal == nil || !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(*dateVal) {
			t.Errorf("date_a = %v, want a plain YYYY-MM-DD string (not a Go time.Time default format)", dateVal)
		}
		datetimeVal := cellAt(t, tp, 2, 0)
		if datetimeVal == nil || !regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`).MatchString(*datetimeVal) {
			t.Errorf("datetime_a = %v, want a plain \"YYYY-MM-DD HH:MM:SS\" string (not a Go time.Time default format)", datetimeVal)
		}
	})

	// B8/P58 D8: cancel actually interrupts a running statement, and — the mattn-regression-class
	// proof — a cancel is scoped to the op's own dedicated *sql.Conn, so an unrelated query on the
	// same adapter right afterward succeeds rather than hanging on the pool's sole connection.
	t.Run("cancel actually interrupts, and the adapter is immediately reusable afterward", func(t *testing.T) {
		a := connectedAdapter(t, cfg)

		ctx, cancelLocal := context.WithCancel(context.Background())
		op := adapters.NewOpCtx("op-cancel")

		errCh := make(chan error, 1)
		go func() {
			_, err := a.Execute(ctx, model.ConsoleRequest{
				Path: nodePath(cfg.ID, seg("database", "main")),
				Statements: []string{
					`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 900000000) SELECT count(*) FROM seq`,
				},
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
		time.Sleep(200 * time.Millisecond) // let the statement actually start stepping
		cancelLocal()                      // unblocks the local wait only — see adapter.go's runOnConn

		ok, err := a.Cancel(context.Background(), "op-cancel")
		if err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		if !ok {
			t.Error("Cancel reported false, want true (a real running query)")
		}

		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Fatal("Execute never returned after cancelLocal()")
		}

		// The real proof: SetMaxOpenConns(1) means this only succeeds quickly if sqlite3_interrupt
		// actually freed the sole pooled connection rather than leaving the runaway statement
		// running forever in the abandoned background goroutine.
		reuseCtx, reuseCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer reuseCancel()
		result, err := a.Count(reuseCtx, adapters.CountRequest{
			Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
		}, adapters.NewOpCtx("op-cancel-reuse"))
		if err != nil {
			t.Fatalf("Count after cancel: %v (the connection was not freed in time)", err)
		}
		if result.Value != 2 {
			t.Errorf("Count after cancel = %d, want 2", result.Value)
		}
	})

	// SetMaxOpenConns(1) means a second op can genuinely queue inside a.db.Conn(ctx) waiting
	// for the sole connection, rather than ever reaching a statement at all — cancelling that
	// queued op's own local ctx must still report E_CANCELLED, not the generic E_QUERY runOnConn's
	// mapError fallback used to produce for db.Conn's own ctx.Err() return.
	t.Run("cancel while queued behind the pool limit reports E_CANCELLED, not E_QUERY", func(t *testing.T) {
		a := connectedAdapter(t, cfg)

		opA := adapters.NewOpCtx("op-pool-a")
		errChA := make(chan error, 1)
		go func() {
			_, err := a.Execute(context.Background(), model.ConsoleRequest{
				Path: nodePath(cfg.ID, seg("database", "main")),
				Statements: []string{
					`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 900000000) SELECT count(*) FROM seq`,
				},
			}, opA)
			errChA <- err
		}()

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if opA.Command() != "" {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(200 * time.Millisecond) // let op A actually hold the sole pooled connection

		// Registered as a cleanup, not run inline after the assertions below: a t.Fatalf there
		// (the very thing this test exists to catch) would otherwise skip this and leave op A's
		// runaway statement abandoned, hanging connectedAdapter's own Disconnect cleanup forever
		// on a.inFlight.Wait().
		t.Cleanup(func() {
			_, _ = a.Cancel(context.Background(), "op-pool-a")
			select {
			case <-errChA:
			case <-time.After(5 * time.Second):
				t.Error("op A's Execute never returned after Cancel")
			}
		})

		ctxB, cancelB := context.WithCancel(context.Background())
		errChB := make(chan error, 1)
		go func() {
			_, err := a.Count(ctxB, adapters.CountRequest{
				Path: nodePath(cfg.ID, seg("database", "main"), seg("table", "customers")),
			}, adapters.NewOpCtx("op-pool-b"))
			errChB <- err
		}()

		time.Sleep(100 * time.Millisecond) // let op B actually reach the blocking db.Conn(ctxB) call
		cancelB()

		select {
		case err := <-errChB:
			if err == nil {
				t.Fatal("Count while queued behind the pool limit succeeded, want it cancelled")
			}
			var ae *adapters.Error
			if !errors.As(err, &ae) || ae.Code != adapters.CodeCancelled {
				t.Fatalf("Count error = %v, want an E_CANCELLED *adapters.Error", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Count never returned after cancelling its own local ctx — cancel is a no-op while queued behind the pool limit")
		}
	})

	t.Run("the file is not modified by a read session, and no -wal/-shm sidecar appears", func(t *testing.T) {
		a := newAdapter(t)
		if _, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-23-connect")); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer a.Disconnect(context.Background())

		before, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("read before: %v", err)
		}
		ctx := context.Background()
		if _, err := a.Children(ctx, nodePath(cfg.ID), adapters.NewOpCtx("op-23a")); err != nil {
			t.Fatalf("Children(root): %v", err)
		}
		if _, err := a.Children(ctx, nodePath(cfg.ID, seg("database", "main")), adapters.NewOpCtx("op-23b")); err != nil {
			t.Fatalf("Children(database): %v", err)
		}
		employeesPath := nodePath(cfg.ID, seg("database", "main"), seg("table", "employees"))
		if _, err := a.Describe(ctx, employeesPath, adapters.NewOpCtx("op-23c")); err != nil {
			t.Fatalf("Describe: %v", err)
		}
		if _, err := a.Read(ctx, adapters.ReadRequest{
			Path: employeesPath, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx("op-23d")); err != nil {
			t.Fatalf("Read: %v", err)
		}
		if _, err := a.Definition(ctx, employeesPath, adapters.NewOpCtx("op-23e")); err != nil {
			t.Fatalf("Definition: %v", err)
		}
		after, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("read after: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Error("a read-only session modified the database file")
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			if _, statErr := os.Stat(fixture.Path + suffix); statErr == nil {
				t.Errorf("a read session left a %s sidecar behind", suffix)
			}
		}
	})
}

// DB MCP read lane: the file is opened read-only, so the engine refuses the write.
func TestSqlite_ReadLane(t *testing.T) {
	fixture := testsupport.StartSqlite(t)
	cfg := fixture.Config
	path := nodePath(cfg.ID, seg("database", "main"))
	unchanged := func(t *testing.T, main adapters.Adapter) {
		t.Helper()
		if got := testsupport.FirstCell(t, main, path, "SELECT count(*) FROM kira_lane_log"); got != "0" {
			t.Errorf("kira_lane_log rows = %s, want 0", got)
		}
	}
	testsupport.RunScenarios(t, connectedAdapter(t, cfg), cfg,
		testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
			Path: path, Setup: []string{"CREATE TABLE kira_lane_log (id int)"}, Write: "INSERT INTO kira_lane_log VALUES (1)",
			WantErr: "readonly", Unchanged: unchanged, Teardown: []string{"DROP TABLE kira_lane_log"},
		}),
		testsupport.LaneCancel(path, "WITH RECURSIVE s(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM s WHERE n < 2000000000) SELECT count(*) FROM s"),
	)
}
