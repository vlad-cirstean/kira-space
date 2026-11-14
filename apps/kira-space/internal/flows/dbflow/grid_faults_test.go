package dbflow

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// J-GRID-038 (OQ-GRID-5 safe part): the connection dies after the commit reached the server and
// before the acknowledgement. Mutate must fail, the server holds the truth, and a retry of the same
// plan never inserts twice.
func TestGridLostAck(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartPostgres(t)
	pgExec(t, fx.URI, `drop table if exists app.lost_ack`, `create table app.lost_ack (id int primary key, v text)`)
	proxy := testsupport.StartPausableProxy(t, fx.Config)
	app := flowharness.New(t)
	conn := createConn(t, app, pgInputFor(proxy.Config()))
	connect(t, app, conn.ID)
	tp := tableNodePath(t, app, conn.ID, "lost_ack")
	ds := app.DataStream(t)

	ops := []model.MutationRowOp{ins(vals("id", "10", "v", "once"))}
	proxy.PauseWhenSent("COMMIT")
	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	opID := ids.NewID()
	go func() { n, err := ds.MutateOp(opID, conn.ID, tp, ops); done <- result{n, err} }()

	// The statement and its commit reach the server while the acknowledgement is withheld.
	testx.WaitUntil(t, 20*time.Second, func() bool { return len(pgRows(t, fx.URI, `select id from app.lost_ack`)) > 0 })
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

	// Read after reconnect returns server truth.
	p, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tp})
	if err != nil {
		t.Fatalf("read after reconnect: %v", err)
	}
	wantRows(t, "read after reconnect", p.Rows, [][]any{{"10", "once"}})

	// Retry of the same plan: refused by the key, never a second row.
	if _, err := ds.Mutate(conn.ID, tp, ops); err == nil {
		t.Fatal("retry of an applied insert succeeded")
	}
	wantRows(t, "second client after retry", pgRows(t, fx.URI, `select id, v from app.lost_ack`), [][]any{{"10", "once"}})
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

// J-GRID-040 (OQ-GRID-6 safe part): a row staged in the app changes or disappears on the server. An
// update or delete that matches no row fails and names the count; it is never reported as success.
// A second window reads the committed value after the first window's commit invalidated the cache.
// Contract studio-grid-stale.
func TestGridStaleRow(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "stale.db")
	sqliteExec(t, path,
		`create table items (id integer primary key, name text)`,
		`insert into items values (1,'a'),(2,'b'),(3,'c')`)
	conn := createConn(t, app, sqliteInput("stale", path))
	connect(t, app, conn.ID)
	tp := tableNodePath(t, app, conn.ID, "items")
	w1, w2 := app.DataStream(t), app.DataStream(t)

	// Both windows load the page; window 2 now holds a cached copy.
	wantRows(t, "w1 load", readRows(t, w1, conn.ID, tp), [][]any{{"1", "a"}, {"2", "b"}, {"3", "c"}})
	if _, src, err := w2.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tp}); err != nil || src != "cache" {
		t.Fatalf("w2 first read source = %q, %v, want the shared cache", src, err)
	}

	// A second client deletes row 2 and changes row 3's key while the app has both staged.
	sqliteExec(t, path, `delete from items where id = 2`, `update items set id = 30 where id = 3`)
	staged := []model.MutationRowOp{
		upd(vals("id", "2"), vals("name", "b2")),
		del(vals("id", "3")),
	}
	app.Contract(t, "studio-grid-stale", "mutate:ops", flowharness.MutateWire(conn.ID, tp, staged), contractMask)
	_, err := w1.Mutate(conn.ID, tp, staged)
	var derr *flowharness.DataError
	if !errors.As(err, &derr) || !strings.Contains(derr.Message, "affected 0") {
		t.Fatalf("update of a vanished row = %v, want a failure naming the affected count", err)
	}
	app.Contract(t, "studio-grid-stale", "mutate:error", derr, flowharness.Mask("message"))
	wantRows(t, "second client after refused plan", sqliteRows(t, path, `select id, name from items order by id`), [][]any{{"1", "a"}, {"30", "c"}})

	// Window 1 commits a change; window 2's next read sees it, not its cached page.
	if _, err := w1.Mutate(conn.ID, tp, []model.MutationRowOp{upd(vals("id", "1"), vals("name", "a1"))}); err != nil {
		t.Fatal(err)
	}
	rows := readRows(t, w2, conn.ID, tp)
	wantRows(t, "w2 after w1 commit", rows, [][]any{{"1", "a1"}, {"30", "c"}})
}
