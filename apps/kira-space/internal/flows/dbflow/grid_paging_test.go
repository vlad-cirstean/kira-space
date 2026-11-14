package dbflow

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// J-GRID-034 / J-BC-020 / J-BC-021: paging while a second client writes, and the tokens' limits.

type pagingEngine struct {
	name string
	// setup returns a connected app, connection id, table path and a second-client exec.
	setup func(t *testing.T) (app *flowharness.App, connID, path string, exec func(stmts ...string))
}

func pagingEngines() []pagingEngine {
	return []pagingEngine{
		{"sqlite", func(t *testing.T) (*flowharness.App, string, string, func(...string)) {
			app := flowharness.New(t)
			file := filepath.Join(t.TempDir(), "paging.db")
			exec := func(s ...string) { sqliteExec(t, file, s...) }
			exec(`create table items (id integer primary key, label text)`)
			for i := 1; i <= 35; i++ {
				exec(fmt.Sprintf(`insert into items values (%d, 'item %d')`, i, i))
			}
			conn := createConn(t, app, sqliteInput("paging", file))
			connect(t, app, conn.ID)
			return app, conn.ID, tableNodePath(t, app, conn.ID, "items"), exec
		}},
		{"postgres", func(t *testing.T) (*flowharness.App, string, string, func(...string)) {
			flowharness.RequireDocker(t)
			fx := testsupport.StartPostgres(t)
			exec := func(s ...string) { pgExec(t, fx.URI, s...) }
			exec(`drop table if exists app.items`, `create table app.items (id int primary key, label text)`,
				`insert into app.items select g, 'item ' || g from generate_series(1, 35) g`)
			app := flowharness.New(t)
			conn := pgFixtureConn(t, app, fx)
			return app, conn.ID, tableNodePath(t, app, conn.ID, "items"), exec
		}},
	}
}

func pageIDs(t *testing.T, p flowharness.DataPage) []int {
	t.Helper()
	var ids []int
	for _, r := range p.Rows {
		n, err := strconv.Atoi(r[0].(string))
		if err != nil {
			t.Fatalf("id %v: %v", r[0], err)
		}
		ids = append(ids, n)
	}
	return ids
}

func readPage(t *testing.T, ds *flowharness.DataStream, connID, path string, size int, cur model.PageCursor) flowharness.DataPage {
	t.Helper()
	p, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: connID, Path: path, PageSize: size, Cursor: cur})
	if err != nil {
		t.Fatalf("read %+v: %v", cur, err)
	}
	return p
}

// Keyset paging forward and back never repeats or skips a primary key while a second client
// inserts ahead of and behind the reader and deletes rows. A text sort cannot page by key, so that
// page says "offset" and the same writes make it repeat a row.
func TestGridPagingStableUnderSecondClientWrites(t *testing.T) {
	for _, eng := range pagingEngines() {
		t.Run(eng.name, func(t *testing.T) {
			app, connID, path, exec := eng.setup(t)
			ds := app.DataStream(t)
			tbl := qual(eng.name) + "items"

			textSort := &model.SortSpec{Kind: "text", Text: "id"}
			readText := func(cur model.PageCursor) flowharness.DataPage {
				p, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: connID, Path: path, PageSize: 10, Sort: textSort, Cursor: cur})
				if err != nil {
					t.Fatalf("text-sort read %+v: %v", cur, err)
				}
				return p
			}
			o1 := readText(model.PageCursor{Mode: "offset"})
			if o1.Strategy != "offset" {
				t.Fatalf("text-sort strategy = %q, want offset", o1.Strategy)
			}
			exec(`insert into ` + tbl + ` values (0, 'front')`)
			o2 := readText(model.PageCursor{Mode: "offset", Offset: 10})
			if got, last := pageIDs(t, o2)[0], pageIDs(t, o1)[9]; got != last {
				t.Fatalf("offset page 2 starts at %d, want a repeat of page 1's last row %d", got, last)
			}
			exec(`delete from ` + tbl + ` where id = 0`)

			first := readPage(t, ds, connID, path, 10, model.PageCursor{Mode: "offset"})
			if first.Strategy != "keyset" || !first.HasMore || first.NextToken == "" {
				t.Fatalf("first page = strategy %q more %v next %q, want a keyset page with a next token", first.Strategy, first.HasMore, first.NextToken)
			}
			exec(`insert into `+tbl+` values (0, 'behind')`, `insert into `+tbl+` values (100, 'ahead')`,
				`delete from `+tbl+` where id in (15, 16)`)

			var forward []int
			forward = append(forward, pageIDs(t, first)...)
			cur := first
			for cur.HasMore {
				cur = readPage(t, ds, connID, path, 10, model.PageCursor{Mode: "after", Token: cur.NextToken})
				forward = append(forward, pageIDs(t, cur)...)
			}
			var want []int
			for i := 1; i <= 35; i++ {
				if i != 15 && i != 16 {
					want = append(want, i)
				}
			}
			want = append(want, 100)
			if !equalInts(forward, want) {
				t.Fatalf("forward ids = %v, want %v (no repeat, no skip, the row behind absent, the row ahead present)", forward, want)
			}

			// The count refreshes on demand and matches the table, the row behind included.
			if n, exact, err := ds.Count(connID, path, nil, true); err != nil || !exact || n != int64(len(want)+1) {
				t.Fatalf("count = %d exact %v (%v), want %d", n, exact, err, len(want)+1)
			}

			// Back from the last page: every earlier row once, in order.
			var back []int
			for p := cur; p.PrevToken != ""; {
				p = readPage(t, ds, connID, path, 10, model.PageCursor{Mode: "before", Token: p.PrevToken})
				back = append(pageIDs(t, p), back...)
				if len(p.Rows) == 0 {
					break
				}
			}
			lastPage := pageIDs(t, cur)
			wantBack := []int{0}
			for _, id := range want {
				if id < lastPage[0] {
					wantBack = append(wantBack, id)
				}
			}
			if !equalInts(back, wantBack) {
				t.Fatalf("back ids = %v, want %v", back, wantBack)
			}
		})
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func qual(engine string) string {
	if engine == "postgres" {
		return "app."
	}
	return ""
}

// A page token belongs to the request that made it. Another page size, filter, sort or projection
// gets a clear E_QUERY instead of rows from the wrong position, and so does a table whose key
// changed under the token.
func TestGridPageTokenFingerprint(t *testing.T) {
	for _, eng := range pagingEngines() {
		t.Run(eng.name, func(t *testing.T) {
			app, connID, path, exec := eng.setup(t)
			ds := app.DataStream(t)
			first := readPage(t, ds, connID, path, 10, model.PageCursor{Mode: "offset"})
			token := model.PageCursor{Mode: "after", Token: first.NextToken}
			ptr := func(s string) *string { return &s }
			other := map[string]flowharness.ReadArgs{
				"page size":  {PageSize: 100},
				"filter":     {PageSize: 10, Filter: ptr("id > 3")},
				"sort":       {PageSize: 10, Sort: &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}}},
				"projection": {PageSize: 10, Projection: []string{"id"}},
			}
			for name, args := range other {
				args.ConnectionID, args.Path, args.Cursor = connID, path, token
				_, _, err := ds.Read(args)
				var derr *flowharness.DataError
				if !errors.As(err, &derr) || derr.Code != "E_QUERY" || !strings.Contains(derr.Message, "page") {
					t.Fatalf("%s changed: err = %v, want E_QUERY naming the page", name, err)
				}
			}
			// The same request still pages.
			readPage(t, ds, connID, path, 10, token)

			// Garbage is refused as malformed, never decoded into a position.
			_, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: connID, Path: path, PageSize: 10, Cursor: model.PageCursor{Mode: "after", Token: "not-a-token"}})
			var derr *flowharness.DataError
			if !errors.As(err, &derr) || derr.Code != "E_CONTRACT" {
				t.Fatalf("garbage token: err = %v, want E_CONTRACT", err)
			}

			// The key changed under the token: the table is rebuilt keyed on another column.
			rebuilt := qual(eng.name) + "items"
			exec(`alter table `+rebuilt+` rename to items_old`,
				`create table `+rebuilt+` (label text primary key, id int)`,
				`insert into `+rebuilt+` select label, id from `+qual(eng.name)+`items_old`)
			if _, err := app.W.Connections.Disconnect(bridge.ConnectionsIDArgs{ID: connID}); err != nil {
				t.Fatal(err)
			}
			connect(t, app, connID)
			path = tableNodePath(t, app, connID, "items")
			_, _, err = ds.Read(flowharness.ReadArgs{ConnectionID: connID, Path: path, PageSize: 10, Cursor: token})
			if !errors.As(err, &derr) || derr.Code != "E_QUERY" || !strings.Contains(derr.Message, "table key") {
				t.Fatalf("token after the key changed: err = %v, want E_QUERY", err)
			}
		})
	}
}

// J-BC-020: a page click that is replaced before its read finishes is cancelled, the engine runs
// no leftover query for it, and the last request still returns its page.
func TestGridRapidPageClicksCancelSupersededReads(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartPostgres(t)
	pgExec(t, fx.URI, `drop table if exists app.items`, `create table app.items (id int primary key, label text)`,
		`insert into app.items select g, 'item ' || g from generate_series(1, 35) g`)
	app := flowharness.New(t)
	conn := pgFixtureConn(t, app, fx)
	path := tableNodePath(t, app, conn.ID, "items")
	ds := app.DataStream(t)

	slow := "pg_sleep(60) is not null"
	var waits []func() (flowharness.DataPage, error)
	var opIDs []string
	for i := 0; i < 3; i++ {
		id, wait := ds.ReadFire(flowharness.ReadArgs{ConnectionID: conn.ID, Path: path, PageSize: 10, Filter: &slow, Cursor: model.PageCursor{Mode: "offset", Offset: i * 10}})
		opIDs, waits = append(opIDs, id), append(waits, wait)
	}
	const running = `select 1 from pg_stat_activity where state = 'active' and query like '%pg_sleep(60)%' and query not like '%pg_stat_activity%'`
	testx.WaitUntil(t, 20*time.Second, func() bool { return len(pgRows(t, fx.URI, running)) > 0 })
	for _, id := range opIDs {
		if ok, err := app.W.Router.Cancel(ctx, id); err != nil || !ok {
			t.Fatalf("Cancel(%s) = %v, %v", id, ok, err)
		}
	}
	for i, wait := range waits {
		done := make(chan error, 1)
		go func() { _, err := wait(); done <- err }()
		select {
		case err := <-done:
			var derr *flowharness.DataError
			if !errors.As(err, &derr) || derr.Code != "E_CANCELLED" {
				t.Fatalf("cancelled read %d: err = %v, want E_CANCELLED", i, err)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("cancelled read %d never returned", i)
		}
	}
	testx.WaitUntil(t, 15*time.Second, func() bool { return len(pgRows(t, fx.URI, running)) == 0 })
	last := readPage(t, ds, conn.ID, path, 10, model.PageCursor{Mode: "offset", Offset: 20})
	if ids := pageIDs(t, last); len(ids) != 10 || ids[0] != 21 {
		t.Fatalf("last requested page ids = %v, want 21..30", ids)
	}
}
