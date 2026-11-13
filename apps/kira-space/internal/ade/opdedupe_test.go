package ade

import (
	"context"
	"strings"
	"testing"
	"time"
)

func testOpBoard(t *testing.T) *TaskBoard {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	b := &TaskBoard{ctx: ctx, cancel: cancel}
	t.Cleanup(cancel)
	return b
}

// The leader closing its window must not cancel the work a joined caller waits for, and a panic in
// the work comes back as an error instead of crashing the process.
func TestJoinOp_leaderCancelAndPanic(t *testing.T) {
	b := testOpBoard(t)
	release := make(chan struct{})
	started := make(chan struct{})
	var workErr error
	fn := func(ctx context.Context) (int, error) {
		close(started)
		<-release
		workErr = ctx.Err()
		return 7, nil
	}
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() { _, err := joinOp(leaderCtx, b, "k", fn); leaderDone <- err }()
	<-started
	type out struct {
		v   int
		err error
	}
	joined := make(chan out, 1)
	go func() {
		v, err := joinOp(context.Background(), b, "k", func(context.Context) (int, error) { t.Error("joiner ran its own work"); return 0, nil })
		joined <- out{v, err}
	}()
	time.Sleep(50 * time.Millisecond) // joiner attached
	cancelLeader()
	if err := <-leaderDone; err != context.Canceled {
		t.Fatalf("leader: %v", err)
	}
	close(release)
	if o := <-joined; o.err != nil || o.v != 7 || workErr != nil {
		t.Fatalf("joiner got %+v, work ctx err %v", o, workErr)
	}

	_, err := joinOp(context.Background(), b, "p", func(context.Context) (int, error) { panic("boom") })
	if err == nil || !strings.Contains(err.Error(), "p failed unexpectedly") {
		t.Fatalf("panic must come back as an error, got %v", err)
	}
}

// Close waits for work a caller already left.
func TestJoinOp_closeWaitsForWork(t *testing.T) {
	b := testOpBoard(t)
	release := make(chan struct{})
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_, _ = joinOp(ctx, b, "k", func(context.Context) (int, error) { close(started); <-release; return 0, nil })
	}()
	<-started
	cancel()
	waited := make(chan struct{})
	go func() { b.wg.Wait(); close(waited) }()
	select {
	case <-waited:
		t.Fatal("wait returned while the work ran")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-waited
}
