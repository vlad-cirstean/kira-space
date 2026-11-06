package adapters

import (
	"context"
	"sync"
)

// QueryTracker is index.ts's opRuntime's own trackerFor/inFlight bookkeeping, generalized over the
// per-engine "running query" identity Q (mysqlfamily/postgres's own RunningQuery struct; a bare
// query_id string for clickhouse) — postgres/mysqlfamily/clickhouse's own adapter.go each carried
// this verbatim.
type QueryTracker[Q comparable] struct {
	mu sync.Mutex
	// inFlight counts query.go/console.go's own adapters.RunWithAbortRace background goroutines
	// still touching a connection. RunWithAbortRace can return to its caller (on ctx.Done()) well
	// before the goroutine it spawned stops using the connection — by design, so a local
	// op abort does not itself kill the query (query.ts:77-80). Disconnect must not close a
	// connection out from under one of these goroutines.
	inFlight sync.WaitGroup
	// draining is set for the duration of Drain's own inFlight.Wait(): sync.WaitGroup
	// requires every Add(1) that could race a Wait() to happen-before it, which TrackerFor's own
	// Add (previously issued with no lock held, after unlocking mu) could not guarantee against a
	// concurrent Drain. Add now runs under the same lock this flag is checked under, and a
	// registration attempt made while draining is refused outright (a no-op release) rather than
	// risk that misuse — Disconnect already means no new op should be starting on this connection
	// anyway.
	draining    bool
	runningByOp map[string]Q
}

// TrackerFor registers q as opID's own running query and hands back its release, called once the
// statement settles. The identity check in the release closure is what makes a multi-statement op
// (mutate's insert, console's "Run all") correct: a later query for the same opID must not have its
// own registration erased by an earlier one's stale release. Returns a no-op release if this
// tracker is currently draining — see draining's own doc comment.
func (t *QueryTracker[Q]) TrackerFor(opID string) func(q Q) (release func()) {
	return func(q Q) func() {
		t.mu.Lock()
		if t.draining {
			t.mu.Unlock()
			return func() {}
		}
		if t.runningByOp == nil {
			t.runningByOp = make(map[string]Q)
		}
		t.runningByOp[opID] = q
		t.inFlight.Add(1)
		t.mu.Unlock()
		return func() {
			defer t.inFlight.Done()
			t.mu.Lock()
			if t.runningByOp[opID] == q {
				delete(t.runningByOp, opID)
			}
			t.mu.Unlock()
		}
	}
}

// PopRunning is Cancel's own lookup: the running query registered for opID, if any, removed from
// the map in the same locked step so a racing release can never re-delete a different (later)
// query's own registration for the same opID.
func (t *QueryTracker[Q]) PopRunning(opID string) (q Q, ok bool) {
	return t.PopRunningWith(opID, nil)
}

// PopRunningWith is PopRunning that also runs onPop, under the tracker lock, for a popped query.
// A query's release takes the same lock, so whatever onPop registers (a pending cancel against
// the query's connection) is visible before the query's op can release that connection.
func (t *QueryTracker[Q]) PopRunningWith(opID string, onPop func(Q)) (q Q, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	q, ok = t.runningByOp[opID]
	delete(t.runningByOp, opID)
	if ok && onPop != nil {
		onPop(q)
	}
	return q, ok
}

// Snapshot returns a copy of every opID currently tracked as running, without removing any of them
// — Disconnect's own first step, cancelling each server-side (its existing Cancel path) before
// Drain, rather than relying on Drain's own inFlight.Wait() to eventually unblock on its own once
// a query happens to finish.
func (t *QueryTracker[Q]) Snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	opIDs := make([]string, 0, len(t.runningByOp))
	for opID := range t.runningByOp {
		opIDs = append(opIDs, opID)
	}
	return opIDs
}

// Drain is Disconnect's own cleanup chain: refuse any new registration (draining), wait for
// every still-in-flight query's own release to run (see inFlight's own doc comment), then clear the
// map and reopen the tracker for a future reconnect on the same adapter instance.
//
// Bounded by ctx: nothing cancels a running query on its own — Disconnect's own
// Snapshot-then-cancel step above is what makes inFlight.Wait() return promptly in
// practice. Without a bound here, Disconnect (and a reconnect's own call to it) blocked the whole
// bridge call for as long as the longest still-running query, even though the UI already reported
// "disconnected". The background goroutine below keeps running past ctx's own deadline so
// runningByOp/draining are still correctly cleared once the real work finishes, even
// though this call itself already returned.
func (t *QueryTracker[Q]) Drain(ctx context.Context) {
	t.mu.Lock()
	t.draining = true
	t.mu.Unlock()

	done := make(chan struct{})
	go func() {
		t.inFlight.Wait()
		t.mu.Lock()
		t.runningByOp = nil
		t.draining = false
		t.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}
