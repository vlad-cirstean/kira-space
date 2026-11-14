package dbflow

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// J-GRID-038 (decision 2): the connection dies after the commit reached the server and before the
// acknowledgement. Mutate fails with the unknown-outcome error (details.outcomeUnknown tells the
// grid to reconcile), the server holds the truth, and a read after the reconnect shows the row
// exactly once. The server has no memory of the plan: a blind resend of an insert into a serial-key
// or keyless table lands a second row, which is why the grid reloads and settles its staged set
// before Commit can run again (UI half tests/ui/studio/grid-lost-ack.spec.ts). Only an explicit
// key refuses the resend.
func TestGridLostAck(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartPostgres(t)
	cases := []struct {
		name, table, ddl string
		ops              []model.MutationRowOp
		count, rows      string
		want             [][]any
		resendRefused    bool
	}{
		{"explicit key", "lost_ack", `create table app.lost_ack (id int primary key, v text)`,
			[]model.MutationRowOp{ins(vals("id", "10", "v", "once"))},
			`select id from app.lost_ack`, `select id, v from app.lost_ack`, [][]any{{"10", "once"}}, true},
		{"serial key", "lost_ack_serial", `create table app.lost_ack_serial (id serial primary key, v text)`,
			[]model.MutationRowOp{ins(vals("v", "once"))},
			`select id from app.lost_ack_serial`, `select id, v from app.lost_ack_serial order by id`, [][]any{{"1", "once"}}, false},
		{"no key", "lost_ack_nokey", `create table app.lost_ack_nokey (v text)`,
			[]model.MutationRowOp{ins(vals("v", "once"))},
			`select v from app.lost_ack_nokey`, `select v from app.lost_ack_nokey`, [][]any{{"once"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pgExec(t, fx.URI, `drop table if exists app.`+c.table, c.ddl)
			proxy := testsupport.StartPausableProxy(t, fx.Config)
			app := flowharness.New(t)
			conn := createConn(t, app, pgInputFor(proxy.Config()))
			connect(t, app, conn.ID)
			tp := tableNodePath(t, app, conn.ID, c.table)
			ds := app.DataStream(t)

			proxy.PauseWhenSent("COMMIT")
			type result struct {
				n   int
				err error
			}
			done := make(chan result, 1)
			opID := ids.NewID()
			go func() { n, err := ds.MutateOp(opID, conn.ID, tp, c.ops); done <- result{n, err} }()

			// The statement and its commit reach the server while the acknowledgement is withheld.
			testx.WaitUntil(t, 20*time.Second, func() bool { return len(pgRows(t, fx.URI, c.count)) > 0 })
			proxy.Kill()
			proxy.Resume()
			var r result
			select {
			case r = <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("mutate did not return after the connection died")
			}
			var derr *flowharness.DataError
			if r.err == nil || !errors.As(r.err, &derr) {
				t.Fatalf("mutate after a lost ack = %d, %v, want a wire error", r.n, r.err)
			}
			if !strings.Contains(derr.Message, "may or may not be saved") {
				t.Fatalf("lost-ack error %q does not name the unknown outcome", derr.Message)
			}
			if derr.Details != `{"outcomeUnknown":true}` {
				t.Fatalf("lost-ack details = %q, want the outcomeUnknown flag the grid reconciles on", derr.Details)
			}

			// Decision 2, UI half tests/ui/studio/grid-lost-ack.spec.ts: the grid shows this text and
			// settles its staged set against a reload. The driver's cause varies by run, so the
			// contract holds a placeholder.
			if c.name == "explicit key" {
				app.Contract(t, "studio-grid-lost-ack", "mutate:lostAck", map[string]string{
					"Code": derr.Code, "Details": derr.Details,
					"Message": regexp.MustCompile(`\([^)]*\)`).ReplaceAllString(derr.Message, "(<cause>)"),
				})
			}

			// Read after reconnect returns server truth: the row, once.
			p, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tp})
			if err != nil {
				t.Fatalf("read after reconnect: %v", err)
			}
			wantRows(t, "read after reconnect", p.Rows, c.want)

			// A resend of the same plan: refused by an explicit key, a second row otherwise.
			_, err = ds.Mutate(conn.ID, tp, c.ops)
			if c.resendRefused != (err != nil) {
				t.Fatalf("resend of an applied insert: err = %v, refused = %v", err, c.resendRefused)
			}
			if c.resendRefused {
				wantRows(t, "second client after resend", pgRows(t, fx.URI, c.rows), c.want)
			} else if got := pgRows(t, fx.URI, c.rows); len(got) != 2 {
				t.Fatalf("second client after resend = %v, want the duplicate row a blind retry lands", got)
			}
		})
	}
}

// J-GRID-038: cancelling a commit on a transactional engine leaves nothing applied.
func TestGridCancelCommit(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartPostgres(t)
	pgExec(t, fx.URI, `drop table if exists app.cancel_commit`, `create table app.cancel_commit (id int primary key, v text)`)
	proxy := testsupport.StartPausableProxy(t, fx.Config)
	app := flowharness.New(t)
	conn := createConn(t, app, pgInputFor(proxy.Config()))
	connect(t, app, conn.ID)
	tp := tableNodePath(t, app, conn.ID, "cancel_commit")
	ds := app.DataStream(t)

	proxy.Pause()
	opID := ids.NewID()
	done := make(chan error, 1)
	go func() {
		_, err := ds.MutateOp(opID, conn.ID, tp, []model.MutationRowOp{ins(vals("id", "1", "v", "a")), ins(vals("id", "2", "v", "b"))})
		done <- err
	}()
	// Wait for the op to be running, then cancel it by id.
	testx.WaitUntil(t, 20*time.Second, func() bool {
		for _, o := range app.W.Router.RunningOps() {
			if o.OpID == opID {
				return true
			}
		}
		return false
	})
	if ok, err := app.W.Router.Cancel(ctx, opID); err != nil || !ok {
		t.Fatalf("Cancel = %v, %v", ok, err)
	}
	proxy.Resume()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled mutate reported success")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("cancelled mutate did not return")
	}
	wantRows(t, "second client after cancel", pgRows(t, fx.URI, `select id from app.cancel_commit`), nil)
}

// J-GRID-040 (OQ-GRID-6) + decision 3: a row staged in the app changes or disappears on the server.
// An update or delete that matches no row fails and names the count; it is never reported as
// success. A guarded op (the grid sends the loaded values) fails with E_CONFLICT naming the row, and
// the same plan without the guard, sent after the user confirms the overwrite, lands. A second window
// reads the committed value after the first window's commit invalidated the cache. The table has the
// shape of the UI fixture app.composite_pk so grid-stale-row.spec.ts replays the contract ops.
// Contract studio-grid-stale.
func TestGridStaleRow(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "stale.db")
	sqliteExec(t, path,
		`create table composite_pk (tenant_id integer not null, entity_id integer not null, name text, primary key (tenant_id, entity_id))`,
		`insert into composite_pk values (1,1,'tenant 1 / entity 1'),(1,2,'tenant 1 / entity 2'),(2,1,'tenant 2 / entity 1')`)
	conn := createConn(t, app, sqliteInput("stale", path))
	connect(t, app, conn.ID)
	tp := tableNodePath(t, app, conn.ID, "composite_pk")
	w1, w2 := app.DataStream(t), app.DataStream(t)
	dump := `select tenant_id, entity_id, name from composite_pk order by 1, 2`

	// Both windows load the page; window 2 now holds a cached copy.
	wantRows(t, "w1 load", readRows(t, w1, conn.ID, tp), [][]any{{"1", "1", "tenant 1 / entity 1"}, {"1", "2", "tenant 1 / entity 2"}, {"2", "1", "tenant 2 / entity 1"}})
	if _, src, err := w2.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tp}); err != nil || src != "cache" {
		t.Fatalf("w2 first read source = %q, %v, want the shared cache", src, err)
	}

	// A second client deletes (1,2) and changes (2,1)'s key while the app has both staged, unguarded.
	sqliteExec(t, path, `delete from composite_pk where tenant_id = 1 and entity_id = 2`, `update composite_pk set entity_id = 30 where tenant_id = 2`)
	staged := []model.MutationRowOp{
		upd(vals("tenant_id", "1", "entity_id", "2"), vals("name", "b2")),
		del(vals("tenant_id", "2", "entity_id", "1")),
	}
	app.Contract(t, "studio-grid-stale", "mutate:ops", flowharness.MutateWire(conn.ID, tp, staged), contractMask)
	_, err := w1.Mutate(conn.ID, tp, staged)
	var derr *flowharness.DataError
	if !errors.As(err, &derr) || !strings.Contains(derr.Message, "affected 0") {
		t.Fatalf("update of a vanished row = %v, want a failure naming the affected count", err)
	}
	app.Contract(t, "studio-grid-stale", "mutate:error", derr, flowharness.Mask("message"))
	wantRows(t, "second client after refused plan", sqliteRows(t, path, dump), [][]any{{"1", "1", "tenant 1 / entity 1"}, {"2", "30", "tenant 2 / entity 1"}})

	// Another client edits (1,1) after the grid loaded it. The guarded update names the row and
	// changes nothing; the unguarded plan lands.
	sqliteExec(t, path, `update composite_pk set name = 'theirs' where tenant_id = 1 and entity_id = 1`)
	key := vals("tenant_id", "1", "entity_id", "1")
	guardedOps := []model.MutationRowOp{guarded(upd(key, vals("name", "mine")), vals("name", "tenant 1 / entity 1"))}
	app.Contract(t, "studio-grid-stale", "mutate:guarded-ops", flowharness.MutateWire(conn.ID, tp, guardedOps), contractMask)
	_, err = w1.Mutate(conn.ID, tp, guardedOps)
	if !errors.As(err, &derr) || derr.Code != "E_CONFLICT" || !strings.Contains(derr.Message, "entity_id = 1") {
		t.Fatalf("guarded update of a changed row = %v, want E_CONFLICT naming the row", err)
	}
	app.Contract(t, "studio-grid-stale", "mutate:conflict", derr, flowharness.Mask("message"))
	wantRows(t, "second client after conflict", sqliteRows(t, path, `select name from composite_pk where tenant_id = 1`), [][]any{{"theirs"}})
	forced := []model.MutationRowOp{upd(key, vals("name", "mine"))}
	app.Contract(t, "studio-grid-stale", "mutate:overwrite-ops", flowharness.MutateWire(conn.ID, tp, forced), contractMask)
	if _, err := w1.Mutate(conn.ID, tp, forced); err != nil {
		t.Fatal(err)
	}
	wantRows(t, "second client after overwrite", sqliteRows(t, path, `select name from composite_pk where tenant_id = 1`), [][]any{{"mine"}})

	// An unchanged row passes the guard; a delete guard refuses a row whose columns moved.
	if _, err := w1.Mutate(conn.ID, tp, []model.MutationRowOp{guarded(upd(key, vals("name", "mine2")), vals("name", "mine"))}); err != nil {
		t.Fatalf("guarded update of an unchanged row: %v", err)
	}
	_, err = w1.Mutate(conn.ID, tp, []model.MutationRowOp{guarded(del(key), vals("tenant_id", "1", "entity_id", "1", "name", "mine"))})
	if !errors.As(err, &derr) || derr.Code != "E_CONFLICT" {
		t.Fatalf("guarded delete of a changed row = %v, want E_CONFLICT", err)
	}
	wantRows(t, "row survives the refused delete", sqliteRows(t, path, `select name from composite_pk where tenant_id = 1`), [][]any{{"mine2"}})

	// Window 1's last commit; window 2's next read sees it, not its cached page.
	if _, err := w1.Mutate(conn.ID, tp, []model.MutationRowOp{upd(key, vals("name", "a1"))}); err != nil {
		t.Fatal(err)
	}
	wantRows(t, "w2 after w1 commit", readRows(t, w2, conn.ID, tp), [][]any{{"1", "1", "a1"}, {"2", "30", "tenant 2 / entity 1"}})
}

// Decision 3 on Postgres: the guard compares the text the grid displays, so every guarded type must
// match its own displayed value (no false conflict) and still refuse a changed row. The matrix holds
// types with no `=` operator (xml, point, polygon, path), a padded char(n), an enum, and the usual
// numeric, temporal and text types. Per column: a guard rebuilt from a fresh read passes, the column
// then changes on the server, and the same guard fails with E_CONFLICT; the control with only that
// column refreshed passes, so the conflict is the guard of that column and no other.
func TestGridGuardTypesPostgres(t *testing.T) {
	fx := testsupport.StartPostgres(t)
	app := flowharness.New(t)
	conn := pgFixtureConn(t, app, fx)
	pgExec(t, fx.URI, `drop table if exists app.guard_types`, `drop type if exists app.guard_mood`,
		`create type app.guard_mood as enum ('happy', 'sad')`,
		`create table app.guard_types (id integer primary key, num numeric(38,10), big bigint, dbl double precision,
			tstz timestamptz, d date, flag boolean, txt text, uni text, x xml, pt point, poly polygon, pth path,
			ch char(5), mood app.guard_mood, uid uuid, ip inet)`,
		`insert into app.guard_types values (1, 12345678901234567890123456.1234567891, 9223372036854775807, 0.1,
			'2024-03-10 12:34:56.789012+05:30', '2024-03-10', true, E'it''s \\ "q"', 'héllo 日本語',
			'<a b="1">x</a>', '(1,2)', '((0,0),(2,2),(4,0))', '[(0,0),(3,3)]', 'ab', 'happy',
			'11111111-1111-1111-1111-111111111111', '192.168.1.5/24'),
			(2, 1, 1, 1.5, '2024-01-01 00:00:00+00', '2024-01-01', false, 't', 'u',
			'<b/>', '(5,6)', '((0,0),(1,1),(2,0))', '((0,0),(1,1))', 'cd', 'sad',
			'22222222-2222-2222-2222-222222222222', '10.0.0.1')`)
	defer pgExec(t, fx.URI, `drop table app.guard_types`, `drop type app.guard_mood`)
	ds := app.DataStream(t)
	tp := tableNodePath(t, app, conn.ID, "guard_types")

	// loaded is every non-key column of row id as the grid displays it.
	loaded := func(id string) model.RowValues {
		page, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tp})
		if err != nil {
			t.Fatal(err)
		}
		var expect model.RowValues
		for r := range page.Rows {
			if page.Rows[r][0].(string) != id {
				continue
			}
			for i, c := range page.Columns {
				if c.Name != "id" {
					v := page.Rows[r][i].(string)
					expect = append(expect, model.RowValue{Name: c.Name, Value: &v})
				}
			}
		}
		if len(expect) != len(page.Columns)-1 {
			t.Fatalf("row %s not read (%d guard values)", id, len(expect))
		}
		return expect
	}
	update := func(expect model.RowValues, txt string) error {
		_, err := ds.Mutate(conn.ID, tp, []model.MutationRowOp{guarded(upd(vals("id", "1"), vals("uni", txt)), expect)})
		return err
	}
	refreshed := func(expect, fresh model.RowValues, column string) model.RowValues {
		out := append(model.RowValues{}, expect...)
		for i, kv := range out {
			if kv.Name == column {
				for _, f := range fresh {
					if f.Name == column {
						out[i].Value = f.Value
					}
				}
			}
		}
		return out
	}

	// Every column changes to a different value, one at a time.
	changes := []struct{ column, set string }{
		{"num", `num + 1`}, {"big", `big - 1`}, {"dbl", `0.2`}, {"tstz", `tstz + interval '1 hour'`},
		{"d", `d + 1`}, {"flag", `not flag`}, {"txt", `'other'`}, {"uni", `uni || '!'`},
		{"x", `'<a>2</a>'::xml`}, {"pt", `'(3,4)'::point`}, {"poly", `'((0,0),(3,3),(6,0))'::polygon`},
		{"pth", `'[(1,1),(2,2)]'::path`}, {"ch", `'zz'`}, {"mood", `'sad'`},
		{"uid", `'33333333-3333-3333-3333-333333333333'`}, {"ip", `'10.9.9.9'`},
	}
	for i, c := range changes {
		expect := loaded("1")
		if err := update(expect, fmt.Sprintf("edit %d", i)); err != nil {
			t.Fatalf("%s: guarded update with every loaded value: %v", c.column, err)
		}
		expect = loaded("1")
		pgExec(t, fx.URI, fmt.Sprintf(`update app.guard_types set %s = %s where id = 1`, c.column, c.set))
		var derr *flowharness.DataError
		if err := update(expect, "refused"); !errors.As(err, &derr) || derr.Code != "E_CONFLICT" || derr.Details != `{"changed":true}` {
			t.Fatalf("%s: guarded update after another client changed it = %v, want E_CONFLICT with details.changed", c.column, err)
		}
		if err := update(refreshed(expect, loaded("1"), c.column), "control"); err != nil {
			t.Fatalf("%s: guard refreshed for only that column: %v", c.column, err)
		}
	}

	// A delete guarded by every loaded value passes on an untouched row of the same table.
	if _, err := ds.Mutate(conn.ID, tp, []model.MutationRowOp{guarded(del(vals("id", "2")), loaded("2"))}); err != nil {
		t.Fatalf("guarded delete of an untouched row: %v", err)
	}
	wantRows(t, "rows after delete", pgRows(t, fx.URI, `select id from app.guard_types`), [][]any{{"1"}})

	// An expect naming no column is a contract error, not raw SQL text.
	bad := model.RowValues{{Name: "nope", Value: ptr("1")}}
	var derr *flowharness.DataError
	if err := update(bad, "x"); !errors.As(err, &derr) || !strings.Contains(derr.Message, "unknown column") {
		t.Fatalf("expect naming an unknown column = %v, want unknown column", err)
	}
}
