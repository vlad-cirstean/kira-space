package dbflow

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
)

func opUpdates(app *flowharness.App, mark int) []model.OpRecord {
	var out []model.OpRecord
	for _, ev := range app.Events.Since(mark, bridge.ChannelOpUpdate) {
		if rec, ok := ev.Data.(model.OpRecord); ok {
			out = append(out, rec)
		}
	}
	return out
}

// waitOp blocks until an op update of kind and status arrives after mark; the op log consumes
// op events asynchronously, so they can land after Connect returns.
func waitOp(t *testing.T, app *flowharness.App, mark int, kind, status string) {
	t.Helper()
	app.Events.WaitAfter(t, mark, bridge.ChannelOpUpdate, func(ev flowharness.Event) bool {
		rec, ok := ev.Data.(model.OpRecord)
		return ok && rec.Kind == kind && rec.Status == status
	}, 5*time.Second)
}

func indexOf(recs []model.OpRecord, kind, status string) int {
	for i, r := range recs {
		if r.Kind == kind && r.Status == status {
			return i
		}
	}
	return -1
}

func TestSQLitePreconnectOp(t *testing.T) {
	t.Run("script shows as an op that ends before connect starts", func(t *testing.T) {
		app := flowharness.New(t)
		in := sqliteInput("tunnel", newSQLiteFile(t))
		in.Preconnect = ptr("sleep 0.3")
		conn := createConn(t, app, in)

		mark := app.Events.Mark()
		if st := connect(t, app, conn.ID); st.Status != "connected" {
			t.Fatalf("Connect = %+v, want connected", st)
		}

		waitOp(t, app, mark, "preconnect", "ok")
		waitOp(t, app, mark, "connect", "ok")
		recs := opUpdates(app, mark)
		running, done, connectRunning := indexOf(recs, "preconnect", "running"), indexOf(recs, "preconnect", "ok"), indexOf(recs, "connect", "running")
		if running < 0 || done < 0 || connectRunning < 0 {
			t.Fatalf("op updates = %+v, want preconnect running+ok and connect running", recs)
		}
		if done > connectRunning {
			t.Errorf("preconnect finished at update %d, after connect started at %d", done, connectRunning)
		}
		pre := recs[done]
		if pre.ConnectionID == nil || *pre.ConnectionID != conn.ID {
			t.Errorf("preconnect connectionId = %v, want %s", pre.ConnectionID, conn.ID)
		}
		if pre.Command == nil || *pre.Command != "sleep 0.3" {
			t.Errorf("preconnect command = %v, want the script text", pre.Command)
		}
		if pre.DurationMs == nil || *pre.DurationMs < 300 {
			t.Errorf("preconnect durationMs = %v, want >= 300", pre.DurationMs)
		}

		mask := flowharness.Mask("id", "durationMs")
		app.Contract(t, "preconnect-op", "event:preconnect-running", recs[running], mask)
		app.Contract(t, "preconnect-op", "event:preconnect-ok", pre, mask)
	})

	t.Run("failing script errors the connection with no connect op", func(t *testing.T) {
		app := flowharness.New(t)
		in := sqliteInput("broken", newSQLiteFile(t))
		in.Preconnect = ptr("echo boom >&2; exit 3")
		conn := createConn(t, app, in)

		mark := app.Events.Mark()
		st, _ := app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: conn.ID})
		if st.Status != "error" {
			t.Fatalf("Connect state = %+v, want error", st)
		}

		waitOp(t, app, mark, "preconnect", "error")
		recs := opUpdates(app, mark)
		pre := recs[indexOf(recs, "preconnect", "error")]
		if pre.Error == nil || !strings.Contains(*pre.Error, "boom") {
			t.Errorf("preconnect error = %v, want it to carry the script's stderr", pre.Error)
		}
		for _, r := range recs {
			if r.Kind == "connect" {
				t.Errorf("connect op %+v ran after a failed pre-connect script", r)
			}
		}
	})
}

func TestSQLitePreconnectDelay(t *testing.T) {
	t.Run("delay holds connect and is counted in the op", func(t *testing.T) {
		app := flowharness.New(t)
		in := sqliteInput("slow-tunnel", newSQLiteFile(t))
		in.Preconnect = ptr("true")
		in.PreconnectDelaySec = 1
		conn := createConn(t, app, in)

		list, err := app.W.Connections.List()
		if err != nil || len(list) != 1 || list[0].PreconnectDelaySec != 1 {
			t.Fatalf("List = %+v (%v), want one connection with preconnectDelaySec 1", list, err)
		}
		dup, err := app.W.Connections.Duplicate(bridge.ConnectionsIDArgs{ID: conn.ID})
		if err != nil || dup.PreconnectDelaySec != 1 {
			t.Fatalf("Duplicate = %+v (%v), want preconnectDelaySec 1", dup, err)
		}

		mark := app.Events.Mark()
		if st := connect(t, app, conn.ID); st.Status != "connected" {
			t.Fatalf("Connect = %+v, want connected", st)
		}

		waitOp(t, app, mark, "preconnect", "ok")
		waitOp(t, app, mark, "connect", "running")
		recs := opUpdates(app, mark)
		running, done, connectRunning := indexOf(recs, "preconnect", "running"), indexOf(recs, "preconnect", "ok"), indexOf(recs, "connect", "running")
		if running < 0 || done < 0 || connectRunning < 0 || done > connectRunning {
			t.Fatalf("op updates = %+v, want preconnect running, preconnect ok, then connect running", recs)
		}
		if d := recs[running].DelayMs; d == nil || *d != 1000 {
			t.Errorf("running preconnect delayMs = %v, want 1000", d)
		}
		if d := recs[done].DurationMs; d == nil || *d < 1000 {
			t.Errorf("preconnect durationMs = %v, want >= 1000", d)
		}
		app.Contract(t, "preconnect-delay", "event:preconnect-delay-running", recs[running], flowharness.Mask("id", "durationMs"))
	})

	t.Run("cancel during the delay", func(t *testing.T) {
		app := flowharness.New(t)
		in := sqliteInput("cancelled-tunnel", newSQLiteFile(t))
		in.Preconnect = ptr("exec sleep 30")
		in.PreconnectDelaySec = 30
		conn := createConn(t, app, in)

		mark := app.Events.Mark()
		type result struct {
			st  model.ConnectionState
			err error
		}
		done := make(chan result, 1)
		go func() {
			st, err := app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: conn.ID})
			done <- result{st, err}
		}()

		waitOp(t, app, mark, "preconnect", "running")
		recs := opUpdates(app, mark)
		opID := recs[indexOf(recs, "preconnect", "running")].ID
		if err := app.W.Ops.Cancel(bridge.OpsCancelArgs{OpID: opID}); err != nil {
			t.Fatalf("Ops.Cancel: %v", err)
		}

		select {
		case r := <-done:
			if r.st.Status != "disconnected" || r.st.Error != nil {
				t.Errorf("Connect = %+v (%v), want disconnected without an error", r.st, r.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Connect still waiting 5s after cancelling the delayed preconnect op")
		}
		waitOp(t, app, mark, "preconnect", "cancelled")
		states, err := app.W.Connections.States()
		if err != nil {
			t.Fatalf("States: %v", err)
		}
		for _, st := range states {
			if st.ConnectionID == conn.ID && st.Status != "disconnected" {
				t.Errorf("state after cancel = %+v, want disconnected", st)
			}
		}
		for _, r := range opUpdates(app, mark) {
			if r.Kind == "connect" {
				t.Errorf("connect op %+v ran after the preconnect was cancelled", r)
			}
		}
	})
}

// Quit mid-delay of a dialog Test must kill its script: the process is not tracked by StopAll yet.
func TestSQLiteTestPreconnectKilledOnQuit(t *testing.T) {
	app := flowharness.New(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	in := sqliteInput("dialog-test", newSQLiteFile(t))
	in.Preconnect = ptr("echo $$ > " + pidFile + "; exec sleep 30")
	in.PreconnectDelaySec = 30

	conn := createConn(t, app, in)

	mark := app.Events.Mark()
	done := make(chan connections.TestResult, 1)
	go func() { done <- app.W.Connections.Test(bridge.ConnectionsIDArgs{ID: conn.ID}) }()
	waitOp(t, app, mark, "preconnect", "running")

	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && pid == 0 {
		if b, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("script never wrote its pid")
	}

	app.Quit(t)

	select {
	case r := <-done:
		if r.OK {
			t.Errorf("Test = %+v, want failure after quit", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Test still waiting 5s after quit")
	}
	for i := 0; i < 50 && syscall.Kill(pid, 0) == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("pre-connect script pid %d survived quit", pid)
	}
}
