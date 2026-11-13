package ade

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/leases"
	"github.com/kirathecat/kira-studio/internal/harness"
)

// A delivery that finds the session busy re-queues its messages; the idle flush that fires in
// between must wait for it and then send them, not see an empty queue and leave them stranded.
func TestFlushLease_idleFlushWaitsForInFlightDelivery(t *testing.T) {
	tr, _, live, clock := newTestTracker(t)
	res := composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: "/repo"})
	sessions, _ := listSessions(tr)
	recordID := sessions[0].ID
	clock.advance(time.Second)
	tr.HandleEvent(harness.AgentEvent{TerminalID: res.TerminalID, Event: harness.EventTurnEnd})

	b := &TaskBoard{}
	b.deps.Tracker = tr
	b.lease = &leaseState{pending: map[leases.Key][]string{}, runs: map[leases.Key]struct{}{}, flush: map[leases.Key]*leaseFlush{}}
	key := leases.Key(recordID)
	b.lease.pending[key] = []string{"you now hold lease q"}

	// The first send attempt (flush A) reads a draft and gives up; the idle flush (C) starts then.
	var calls atomic.Int32
	cDone := make(chan struct{})
	tr.deps.HasDraft = func(string) bool {
		if calls.Add(1) > 1 {
			return false
		}
		go func() { b.flushLease(recordID); close(cDone) }()
		time.Sleep(100 * time.Millisecond) // C is now at the lock, or past it
		return true
	}
	b.flushLease(recordID)
	select {
	case <-cDone:
	case <-time.After(5 * time.Second):
		t.Fatal("idle flush never finished")
	}
	live.mu.Lock()
	written := strings.Join(live.logs, "")
	live.mu.Unlock()
	if !strings.Contains(written, "you now hold lease q") {
		t.Fatalf("message stranded: terminal got %q, pending %v", written, b.lease.pending[key])
	}
	if n := len(b.lease.pending[key]); n != 0 {
		t.Fatalf("pending after delivery: %d", n)
	}
}

// A session that ends while a delivery is in flight must not get its messages re-queued.
func TestFlushLease_noRequeueAfterEnd(t *testing.T) {
	tr, _, live, clock := newTestTracker(t)
	res := composeAndSpawn(t, tr, live, PrepareArgs{TaskID: "t1", Cwd: "/repo"})
	sessions, _ := listSessions(tr)
	recordID := sessions[0].ID
	clock.advance(time.Second)
	tr.HandleEvent(harness.AgentEvent{TerminalID: res.TerminalID, Event: harness.EventTurnEnd})

	b := &TaskBoard{}
	b.deps.Tracker = tr
	b.lease = &leaseState{pending: map[leases.Key][]string{}, runs: map[leases.Key]struct{}{}, flush: map[leases.Key]*leaseFlush{}}
	b.lease.book = leases.New(leases.Options{})
	key := leases.Key(recordID)
	b.lease.pending[key] = []string{"m"}

	tr.deps.HasDraft = func(string) bool {
		b.endLeases(key) // the session ends while A is between take and re-queue
		return true
	}
	b.flushLease(recordID)
	if n := len(b.lease.pending[key]); n != 0 {
		t.Fatalf("messages re-queued for an ended key: %d", n)
	}
}
