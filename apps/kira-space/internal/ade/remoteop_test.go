package ade

import (
	"context"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// A remote op waits for the repo's slot instead of failing with OperationInProgress, and its ctx
// ends the wait.
func TestRunRemote_queuesBehindHolder(t *testing.T) {
	skipWithoutGitQueue(t)
	h := newBoardHarness(t)
	_, dir := initQueueRepo(t)
	h.addRepoFromPath("r", dir)
	entry, err := h.board.openRepo(context.Background(), "r")
	if err != nil {
		t.Fatal(err)
	}
	params := gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"}

	slot := h.board.remoteSlot("r")
	slot <- struct{}{} // another ADE remote op holds the repo
	type out struct {
		res gitsession.RemoteOpResult
		err error
	}
	done := make(chan out, 1)
	go func() {
		res, err := h.board.runRemote(context.Background(), "r", entry, params)
		done <- out{res, err}
	}()
	select {
	case <-done:
		t.Fatal("remote op ran while the slot was held")
	case <-time.After(150 * time.Millisecond):
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.board.runRemote(ctx, "r", entry, params); err != context.Canceled {
		t.Fatalf("cancelled wait: %v", err)
	}

	<-slot
	select {
	case o := <-done:
		if o.err != nil || !o.res.OK {
			t.Fatalf("queued op: ok=%v err=%v res=%+v", o.res.OK, o.err, o.res.Error)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("queued op never ran")
	}
}
