package adapterhost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/internal/ids"
)

func readFrame(t *testing.T, id int, connID, opID string) []byte {
	t.Helper()
	return mustFrame(t, map[string]any{
		"kind": "req", "id": id, "op": "data:read",
		"payload": map[string]any{
			"opId": opID, "tabId": nil, "connectionId": connID, "path": "database:app/table:t",
			"projection": nil, "filter": nil, "sort": nil, "pageSize": 10,
			"cursor": map[string]any{"mode": "offset", "offset": 0},
		},
	})
}

// P2 R1: handleDataOp used to derive every op's context from context.Background() — an op had no
// way to be bounded by anything but an explicit ops:cancel RPC, so it (and the driver call inside
// it) outlived its own session indefinitely once the renderer that started it was already gone
// (page reload/close). The op's context now comes from the session instead, so closing the session
// actually cancels whatever is still running against it.
func TestHandleDataFrame_SessionClose_CancelsInFlightOp(t *testing.T) {
	r := newTestRouter()

	started := make(chan struct{})
	cancelled := make(chan error, 1)
	fake := &dataFakeAdapter{readCtxFn: func(ctx context.Context) (page.Page, error) {
		close(started)
		<-ctx.Done()
		cancelled <- ctx.Err()
		return page.TabularPage{}, ctx.Err()
	}}
	adapters.SetLiveAdapter("conn-cancel", fake)
	defer adapters.DeleteLiveAdapter("conn-cancel")

	conn := newFakeConn()
	session, detach := r.AttachStream(conn)

	go r.HandleDataFrame(session, readFrame(t, 1, "conn-cancel", "3a2f8037-8838-7c1b-8cc6-619da3f0bb5b"))

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("adapter.Read never started")
	}

	detach()

	select {
	case err := <-cancelled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ctx.Err() = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the op's context was never cancelled when its session closed")
	}
}

// ServeEngineStream used to spawn `go router.HandleDataFrame(...)` per inbound frame with no
// cap — a burst of frames could grow goroutines (and the driver work each one holds open) without
// bound. HandleDataFrameAsync now blocks acquiring one of a fixed number of concurrency slots
// before spawning, so a session pinned at the cap makes the next frame wait rather than spawn.
func TestHandleDataFrameAsync_BoundsConcurrentOps(t *testing.T) {
	r := newTestRouter()

	entered := make(chan struct{}, sessionMaxInFlightOps+1)
	release := make(chan struct{})
	fake := &dataFakeAdapter{readCtxFn: func(ctx context.Context) (page.Page, error) {
		entered <- struct{}{}
		<-release
		return page.TabularPage{}, nil
	}}
	adapters.SetLiveAdapter("conn-bound", fake)
	defer adapters.DeleteLiveAdapter("conn-bound")

	conn := newFakeConn()
	// fakeConn.sent has a small fixed buffer (session_test.go) and this test drives far more
	// responses through it than that — drain it in the background so writeLoop's own conn.Send
	// never blocks and stalls the queue behind it.
	go func() {
		for range conn.sent {
		}
	}()
	session, detach := r.AttachStream(conn)
	defer detach()

	// Saturate every concurrency slot. Each call acquires its slot synchronously before returning
	// (acquireSlot has capacity to spare for all of these), so this loop itself never blocks.
	for i := 0; i < sessionMaxInFlightOps; i++ {
		r.HandleDataFrameAsync(session, readFrame(t, i, "conn-bound", ids.NewID()))
	}
	for i := 0; i < sessionMaxInFlightOps; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d/%d ops had started after 2s", i, sessionMaxInFlightOps)
		}
	}

	// One more frame: acquireSlot must now block (every slot is held by a still-running op), so
	// HandleDataFrameAsync itself must not yet have returned.
	extraDone := make(chan struct{})
	go func() {
		r.HandleDataFrameAsync(session, readFrame(t, sessionMaxInFlightOps, "conn-bound", "60a74230-8ce5-70eb-8ba4-908258f84e1f"))
		close(extraDone)
	}()

	select {
	case <-extraDone:
		t.Fatal("HandleDataFrameAsync returned (spawned an op) before any concurrency slot was free")
	case <-time.After(100 * time.Millisecond):
	}

	// Free exactly one op; its slot becomes available and the pending call should now go through.
	release <- struct{}{}

	select {
	case <-extraDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the extra op's slot was never granted after one in-flight op finished")
	}

	// Unblock the remaining sessionMaxInFlightOps-1 ops so the test doesn't leak goroutines.
	go func() {
		for i := 0; i < sessionMaxInFlightOps; i++ {
			release <- struct{}{}
		}
	}()
}
