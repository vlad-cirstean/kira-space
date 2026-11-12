package ade

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// stalledBuilds counts builds and parks the first one after its store read until release.
type stalledBuilds struct {
	builds  atomic.Int32
	loaded  chan struct{}
	release chan struct{}
}

func stallFirstBuild(h *boardHarness) *stalledBuilds {
	s := &stalledBuilds{loaded: make(chan struct{}), release: make(chan struct{})}
	h.board.bcache.afterLoad = func() {
		if s.builds.Add(1) == 1 {
			close(s.loaded)
			<-s.release
		}
	}
	return s
}

func TestBoardCache_concurrentColdReadersShareOneBuild(t *testing.T) {
	h := newBoardHarness(t)
	var builds atomic.Int32
	h.board.bcache.afterLoad = func() { builds.Add(1) }
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := h.board.Board(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := builds.Load(); got != 1 {
		t.Fatalf("%d builds for 20 concurrent readers", got)
	}
	if _, err := h.board.Board(context.Background()); err != nil || builds.Load() != 1 {
		t.Fatalf("a fresh snapshot rebuilt: builds %d err %v", builds.Load(), err)
	}
}

func TestBoardCache_readerAfterInvalidationNeverGetsOlderBuild(t *testing.T) {
	h := newBoardHarness(t)
	h.addTask("T1", "task")
	s := stallFirstBuild(h)
	type result struct {
		tasks int
		err   error
	}
	first := make(chan result, 1)
	go func() {
		b, err := h.board.Board(context.Background())
		first <- result{len(b.Tasks), err}
	}()
	<-s.loaded
	h.addTask("T2", "task")
	h.board.invalidateBoard()
	b, err := h.board.Board(context.Background())
	if err != nil || len(b.Tasks) != 2 {
		t.Fatalf("reader after the bump: %d tasks, err %v", len(b.Tasks), err)
	}
	close(s.release)
	if r := <-first; r.err != nil || r.tasks != 1 {
		t.Fatalf("reader that joined before the bump: %+v", r)
	}
	// the older build finishing last must not replace the newer snapshot
	if b, err := h.board.Board(context.Background()); err != nil || len(b.Tasks) != 2 || s.builds.Load() != 2 {
		t.Fatalf("snapshot after both builds: %d tasks, %d builds, err %v", len(b.Tasks), s.builds.Load(), err)
	}
}

func TestBoardCache_callerCancelLeavesBuildRunning(t *testing.T) {
	h := newBoardHarness(t)
	s := stallFirstBuild(h)
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan error, 1)
	go func() {
		_, err := h.board.Board(ctx)
		got <- err
	}()
	<-s.loaded
	cancel()
	if err := <-got; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled caller got %v", err)
	}
	close(s.release)
	if _, err := h.board.Board(context.Background()); err != nil || s.builds.Load() != 1 {
		t.Fatalf("build was not kept: builds %d err %v", s.builds.Load(), err)
	}
}

func TestBoardPush_coalescesAndMerges(t *testing.T) {
	h := newBoardHarness(t)
	for i := range 50 {
		h.board.notifyBoardWith(taskChanged([]string{"a", "b", "c", "d", "e"}[i%5]))
	}
	waitUntil(t, "coalesced push", func() bool { return len(h.pushed()) >= 1 })
	// notifyBoardNow settles the stream: any further timer push would show before it.
	h.board.notifyBoardNow(taskChanged("z"))
	got := h.pushed()
	want := []adewire.BoardChangedEvent{{TaskIDs: []string{"a", "b", "c", "d", "e"}}, {TaskIDs: []string{"z"}}}
	if !slices.EqualFunc(got, want, eqBoardEvent) {
		t.Fatalf("pushes %+v, want %+v", got, want)
	}

	var c boardChanges
	c.merge(taskChanged("a"))
	c.merge(adewire.BoardChangedEvent{Refs: true, TaskIDs: []string{"b"}})
	if ev := c.event(); !eqBoardEvent(ev, adewire.BoardChangedEvent{Refs: true, TaskIDs: []string{"a", "b"}}) {
		t.Fatalf("refs OR, ids union: %+v", ev)
	}
	c.merge(allChanged)
	if ev := c.event(); !ev.Refs || ev.TaskIDs != nil {
		t.Fatalf("unknown tasks win: %+v", ev)
	}
}

func TestBoardPush_nowEmitsBeforeReturnAndLeavesNoTimerPush(t *testing.T) {
	h := newBoardHarness(t)
	h.board.notifyBoardWith(taskChanged("a"))
	h.board.notifyBoardNow(taskChanged("b"))
	if got := h.pushed(); len(got) != 1 || !eqBoardEvent(got[0], adewire.BoardChangedEvent{TaskIDs: []string{"a", "b"}}) {
		t.Fatalf("notifyBoardNow did not carry the pending change synchronously: %+v", got)
	}
	h.board.bcache.pushMu.Lock()
	armed := h.board.bcache.pushTimer != nil
	h.board.bcache.pushMu.Unlock()
	if armed {
		t.Fatal("push timer still armed after notifyBoardNow")
	}
	h.board.notifyBoardWith(taskChanged("c"))
	waitUntil(t, "later push", func() bool { return len(h.pushed()) == 2 })
	if got := h.pushed(); !eqBoardEvent(got[1], adewire.BoardChangedEvent{TaskIDs: []string{"c"}}) {
		t.Fatalf("stray timer push: %+v", got)
	}
}

func eqBoardEvent(a, b adewire.BoardChangedEvent) bool {
	return a.Refs == b.Refs && (a.TaskIDs == nil) == (b.TaskIDs == nil) && slices.Equal(a.TaskIDs, b.TaskIDs)
}
