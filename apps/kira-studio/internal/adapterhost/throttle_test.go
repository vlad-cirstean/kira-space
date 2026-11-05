package adapterhost

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
)

func noopFn(ctx context.Context, op *adapters.OpCtx) (any, error) { return nil, nil }

// A high rate with a hand-installed burst of 1 (SetThrottle's own burst formula would give this
// rate a burst of 10, letting all five ops through immediately — this test is about RunOp's
// pacing, not the burst formula, so it installs the limiter directly). Five sequential reads at
// 50/s and burst 1 must take at least ~4/50s to let the last four through — a high rate
// deliberately, so this costs milliseconds and cannot flake on a slow machine the way a 1/s test
// would.
func TestThrottle_BurstThenPaces(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	const connID = "conn-throttle-pace"
	h.throttles.mu.Lock()
	h.throttles.limiters[connID] = rate.NewLimiter(rate.Limit(50), 1)
	h.throttles.mu.Unlock()

	start := time.Now()
	for i := 0; i < 5; i++ {
		_, _, err := h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)}, noopFn)
		if err != nil {
			t.Fatalf("op %d: unexpected error %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Errorf("elapsed = %v, want at least ~80ms for 4 paced ops at 50/s", elapsed)
	}
}

// No SetThrottle call at all — the connection is unthrottled, RunOp must not delay it by even a
// registry lookup's worth of hesitation, and no registry entry is ever created for it.
func TestThrottle_UnsetMeansNoDelayAndNoEntry(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	const connID = "conn-throttle-unset"

	start := time.Now()
	_, _, err := h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)}, noopFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Errorf("elapsed = %v, want near-zero for an unthrottled connection", elapsed)
	}
	if l := h.throttles.limiterFor(connID); l != nil {
		t.Error("expected no registry entry for a connection whose throttle was never set")
	}
}

// connect/disconnect/test must bypass the gate outright — even under a limiter so restrictive its
// burst is already exhausted, none of the three lifecycle kinds ever waits. A misconfigured
// throttle must never be able to lock a user out of connecting to, or disconnecting from, the
// connection it throttles.
func TestThrottle_LifecycleKindsBypassGate(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	const connID = "conn-throttle-lifecycle"
	limiter := rate.NewLimiter(rate.Limit(0.1), 1)
	limiter.Allow() // drain the one-token burst so any gated op would have to wait ~10s
	h.throttles.mu.Lock()
	h.throttles.limiters[connID] = limiter
	h.throttles.mu.Unlock()

	for _, kind := range []string{"connect", "disconnect"} {
		start := time.Now()
		_, _, err := h.RunOp(context.Background(), OpSpec{Kind: kind, ConnectionID: strp(connID)}, noopFn)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", kind, err)
		}
		if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
			t.Errorf("%s: elapsed = %v, want near-zero — lifecycle kinds must bypass the gate", kind, elapsed)
		}
	}

	// test never carries a connection id at all — the ConnectionID != nil check alone already
	// excludes it, verified here rather than assumed.
	start := time.Now()
	_, _, err := h.RunOp(context.Background(), OpSpec{Kind: "test"}, noopFn)
	if err != nil {
		t.Fatalf("test: unexpected error %v", err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Errorf("test: elapsed = %v, want near-zero", elapsed)
	}
}

// Cancelling a queued op must unblock it immediately with E_CANCELLED, and — the assertion that
// proves §5.5's no-half-row property — neither op:start nor op:end is ever emitted for it, since
// it never touched the database.
func TestThrottle_CancelledWhileQueued_NoEvents(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	const connID = "conn-throttle-cancel"
	limiter := rate.NewLimiter(rate.Limit(0.1), 1) // ~10s expected wait once the burst is drained
	limiter.Allow()
	h.throttles.mu.Lock()
	h.throttles.limiters[connID] = limiter
	h.throttles.mu.Unlock()

	events, unsubscribe := h.Subscribe()
	defer unsubscribe()

	const opID = "f68af556-603f-7570-88ee-42694eace76f"
	done := make(chan error, 1)
	go func() {
		_, _, err := h.RunOp(context.Background(), OpSpec{OpID: opID, Kind: "read", ConnectionID: strp(connID)}, noopFn)
		done <- err
	}()

	// RunOp registers the op in h.running before it ever reaches the throttle wait — poll for that
	// registration rather than a fixed sleep, so this can't flake on a slow machine.
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.Lock()
		_, registered := h.running[opID]
		h.mu.Unlock()
		if registered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("op never registered in h.running")
		}
		time.Sleep(time.Millisecond)
	}

	ok, err := h.CancelOp(context.Background(), opID)
	if err != nil || !ok {
		t.Fatalf("CancelOp = %v, %v, want true, nil", ok, err)
	}

	select {
	case err := <-done:
		code, has := adapters.CodeOf(err)
		if !has || code != adapters.CodeCancelled {
			t.Fatalf("RunOp error = %v, want an E_CANCELLED *adapters.Error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunOp never returned after CancelOp")
	}

	select {
	case e := <-events:
		t.Errorf("expected zero events for a cancelled-while-queued op, got %+v", e)
	case <-time.After(50 * time.Millisecond):
		// none observed, as expected
	}
}

// SetThrottle(id, 0) is Disconnect's own clear path — the limiter must actually be removed from
// the registry, not merely rendered ineffective, so a later SetThrottle(id, N>0) for a reused
// connection id starts from a fresh token bucket.
func TestThrottle_SetToZeroRemovesLimiter(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	const connID = "conn-throttle-clear"

	h.SetThrottle(connID, 5)
	if l := h.throttles.limiterFor(connID); l == nil {
		t.Fatal("expected a registry entry after SetThrottle(id, 5)")
	}

	h.SetThrottle(connID, 0)
	if l := h.throttles.limiterFor(connID); l != nil {
		t.Error("expected no registry entry after SetThrottle(id, 0)")
	}
}
