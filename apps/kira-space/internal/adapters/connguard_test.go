package adapters

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func tryLock(g *ConnGuard, within time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()
	if err := g.Lock(ctx); err != nil {
		return false
	}
	g.Unlock()
	return true
}

func TestConnGuardLockQueuedOpIsCancellable(t *testing.T) {
	var g ConnGuard
	if err := g.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- g.Lock(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if code, _ := CodeOf(err); code != CodeCancelled {
			t.Fatalf("Lock = %v, want E_CANCELLED", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued Lock ignored its ctx")
	}
	g.Unlock()
	if !tryLock(&g, time.Second) {
		t.Fatal("connection not free after Unlock")
	}
}

func TestConnGuardReleaseHoldsConnectionUntilPendingWorkEnds(t *testing.T) {
	var g ConnGuard
	_ = g.Lock(context.Background())
	done := g.Track()
	g.BeginCancel()

	returned := make(chan struct{})
	go func() { g.Release(); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Release blocked the caller on pending work")
	}
	if tryLock(&g, 30*time.Millisecond) {
		t.Fatal("connection handed out while a query and a cancel were pending")
	}
	done()
	if tryLock(&g, 30*time.Millisecond) {
		t.Fatal("connection handed out while a cancel was still pending")
	}
	g.EndCancel()
	if !tryLock(&g, time.Second) {
		t.Fatal("connection still held after all pending work ended")
	}
}

func TestConnGuardCloseWithin(t *testing.T) {
	t.Run("graceful once free", func(t *testing.T) {
		var g ConnGuard
		var graceful, forced atomic.Bool
		g.CloseWithin(context.Background(), time.Second, func() { graceful.Store(true) }, func() { forced.Store(true) })
		if !graceful.Load() || forced.Load() {
			t.Fatalf("graceful=%v forced=%v, want graceful only", graceful.Load(), forced.Load())
		}
	})
	t.Run("force after grace while held", func(t *testing.T) {
		var g ConnGuard
		_ = g.Lock(context.Background())
		var graceful, forced atomic.Bool
		g.CloseWithin(context.Background(), 20*time.Millisecond, func() { graceful.Store(true) }, func() { forced.Store(true) })
		if graceful.Load() || !forced.Load() {
			t.Fatalf("graceful=%v forced=%v, want forced only", graceful.Load(), forced.Load())
		}
	})
	t.Run("force when ctx ends while held", func(t *testing.T) {
		var g ConnGuard
		_ = g.Lock(context.Background())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var forced atomic.Bool
		g.CloseWithin(ctx, time.Minute, func() {}, func() { forced.Store(true) })
		if !forced.Load() {
			t.Fatal("not forced on a done ctx")
		}
	})
}

// A cancel popped from the tracker must keep the connection held past the cancelled query's own
// release, however the two race: that is the whole point of registering it under the tracker lock.
func TestConnGuardCancelPopBlocksReleaseOfSameQuery(t *testing.T) {
	type q struct{ g *ConnGuard }
	for i := 0; i < 300; i++ {
		var g ConnGuard
		var tr QueryTracker[q]
		_ = g.Lock(context.Background())
		query := q{g: &g}
		release := tr.TrackerFor("op")(query)

		var popped atomic.Bool
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, ok := tr.PopRunningWith("op", func(q q) { q.g.BeginCancel() })
			popped.Store(ok)
		}()
		go func() {
			defer wg.Done()
			release()
			g.Release()
		}()
		wg.Wait()

		if popped.Load() {
			if tryLock(&g, 5*time.Millisecond) {
				t.Fatal("connection free while a popped cancel was still pending")
			}
			g.EndCancel()
		}
		if !tryLock(&g, time.Second) {
			t.Fatal("connection never freed")
		}
	}
}
