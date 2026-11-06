package adapterhost

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

func testGate(l *rate.Limiter) *connGate {
	return &connGate{limiter: l, slot: semaphore.NewWeighted(1)}
}

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
	h.throttles.byID[connID] = testGate(rate.NewLimiter(rate.Limit(50), 1))
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
	if l := h.throttles.gateFor(connID); l != nil {
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
	h.throttles.byID[connID] = testGate(limiter)
	h.throttles.mu.Unlock()

	for _, kind := range []string{"connect", "disconnect", "preconnect"} {
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
	h.throttles.byID[connID] = testGate(limiter)
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
	if l := h.throttles.gateFor(connID); l == nil {
		t.Fatal("expected a registry entry after SetThrottle(id, 5)")
	}

	h.SetThrottle(connID, 0)
	if l := h.throttles.gateFor(connID); l != nil {
		t.Error("expected no registry entry after SetThrottle(id, 0)")
	}
}

// holdOp starts a read on connID whose fn blocks until release is closed; it returns once fn runs.
func holdOp(t *testing.T, h *Host, connID string) (release chan struct{}, done chan error) {
	t.Helper()
	release, done = make(chan struct{}), make(chan error, 1)
	started := make(chan struct{})
	go func() {
		_, _, err := h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)},
			func(ctx context.Context, op *adapters.OpCtx) (any, error) {
				close(started)
				<-release
				return nil, nil
			})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("holder never started")
	}
	return release, done
}

func waitRegistered(t *testing.T, h *Host, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.Lock()
		got := len(h.running)
		h.mu.Unlock()
		if got >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d ops registered, want %d", got, n)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestThrottle_SerialPerConnection(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	h.SetThrottle("conn-serial", 1000)
	release, holderDone := holdOp(t, h, "conn-serial")

	second := make(chan struct{})
	go func() {
		_, _, _ = h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp("conn-serial")},
			func(context.Context, *adapters.OpCtx) (any, error) { close(second); return nil, nil })
	}()
	select {
	case <-second:
		t.Fatal("second op started while the first held the slot")
	case <-time.After(100 * time.Millisecond):
	}

	// Another connection is unaffected.
	if _, _, err := h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp("conn-other")}, noopFn); err != nil {
		t.Fatal(err)
	}

	close(release)
	<-holderDone
	select {
	case <-second:
	case <-time.After(2 * time.Second):
		t.Fatal("second op never started after the first returned")
	}
}

func TestThrottle_FIFOOrder(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	const connID = "conn-fifo"
	h.SetThrottle(connID, 1000)
	release, holderDone := holdOp(t, h, connID)

	var mu sync.Mutex
	var order []int
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)},
				func(context.Context, *adapters.OpCtx) (any, error) {
					mu.Lock()
					order = append(order, i)
					mu.Unlock()
					return nil, nil
				})
		}()
		waitRegistered(t, h, i+2) // holder plus i+1 queued; fixes arrival order
		time.Sleep(20 * time.Millisecond)
	}
	close(release)
	<-holderDone
	wg.Wait()
	if len(order) != 3 || order[0] != 0 || order[1] != 1 || order[2] != 2 {
		t.Errorf("start order = %v, want [0 1 2]", order)
	}
}

func TestThrottle_CancelWhileWaitingForSlot(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	const connID = "conn-slot-cancel"
	h.SetThrottle(connID, 1000)
	release, holderDone := holdOp(t, h, connID)
	events, unsubscribe := h.Subscribe()
	defer unsubscribe()

	const opID = "f68af556-603f-7570-88ee-42694eace76f"
	done := make(chan error, 1)
	go func() {
		_, _, err := h.RunOp(context.Background(), OpSpec{OpID: opID, Kind: "read", ConnectionID: strp(connID)}, noopFn)
		done <- err
	}()
	waitRegistered(t, h, 2)
	if ok, err := h.CancelOp(context.Background(), opID); err != nil || !ok {
		t.Fatalf("CancelOp = %v, %v", ok, err)
	}
	if code, _ := adapters.CodeOf(<-done); code != adapters.CodeCancelled {
		t.Fatalf("code = %v, want E_CANCELLED", code)
	}
	select {
	case e := <-events:
		t.Errorf("unexpected event for a cancelled queued op: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	<-holderDone
	// Slot not leaked: the next op runs.
	if _, _, err := h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)}, noopFn); err != nil {
		t.Fatalf("op after cancelled waiter: %v", err)
	}
}

func TestThrottle_BusyTimeout(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	h.throttleWait = 50 * time.Millisecond
	const connID = "conn-busy"
	h.SetThrottle(connID, 1000)
	release, holderDone := holdOp(t, h, connID)
	defer func() { close(release); <-holderDone }()

	_, _, err := h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)}, noopFn)
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeTimeout || !strings.Contains(ae.Message, "another operation") {
		t.Fatalf("err = %v, want E_TIMEOUT throttleBusyTimeout", err)
	}
}

func TestThrottle_RateTimeout(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	h.throttleWait = 50 * time.Millisecond
	const connID = "conn-rate-timeout"
	h.throttles.byID[connID] = testGate(rate.NewLimiter(rate.Limit(0.1), 1))
	h.throttles.byID[connID].limiter.Allow()

	_, _, err := h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)}, noopFn)
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeTimeout || !strings.Contains(ae.Message, "ops per second") {
		t.Fatalf("err = %v, want E_TIMEOUT throttleRateTimeout", err)
	}
	// The slot taken before the failed token wait is freed.
	h.throttles.byID[connID].limiter.SetLimit(1000)
	h.throttles.byID[connID].limiter.SetBurst(10)
	if _, _, err := h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)}, noopFn); err != nil {
		t.Fatalf("slot leaked after rate timeout: %v", err)
	}
}

func TestThrottle_LiveRateChangeKeepsSlot(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	const connID = "conn-live"
	h.SetThrottle(connID, 5)
	before := h.throttles.gateFor(connID)
	release, holderDone := holdOp(t, h, connID)

	h.SetThrottle(connID, 100)
	g := h.throttles.gateFor(connID)
	if g != before || float64(g.limiter.Limit()) != 100 || g.limiter.Burst() != 10 {
		t.Fatalf("gate replaced or rate not applied: same=%v limit=%v burst=%d", g == before, g.limiter.Limit(), g.limiter.Burst())
	}

	second := make(chan struct{})
	go func() {
		_, _, _ = h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)},
			func(context.Context, *adapters.OpCtx) (any, error) { close(second); return nil, nil })
	}()
	select {
	case <-second:
		t.Fatal("rate change let a second op past the held slot")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-holderDone
	<-second
}

func TestThrottle_ExemptKindsRunWhileSlotHeld_UnlistedKindGated(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	h.throttleWait = 50 * time.Millisecond
	const connID = "conn-exempt"
	h.SetThrottle(connID, 1000)
	release, holderDone := holdOp(t, h, connID)
	defer func() { close(release); <-holderDone }()

	for kind := range throttleExemptKinds {
		if _, _, err := h.RunOp(context.Background(), OpSpec{Kind: kind, ConnectionID: strp(connID)}, noopFn); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}
	_, _, err := h.RunOp(context.Background(), OpSpec{Kind: "someFutureKind", ConnectionID: strp(connID)}, noopFn)
	if code, _ := adapters.CodeOf(err); code != adapters.CodeTimeout {
		t.Errorf("unlisted kind err = %v, want gated (E_TIMEOUT)", err)
	}
}

func TestThrottle_ReentrantAdmitDoesNotDeadlock(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	h.throttleWait = 200 * time.Millisecond
	const connID = "conn-reentrant"
	h.SetThrottle(connID, 1000)

	_, _, err := h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			_, rel, err := h.admit(ctx, connID)
			if err == nil {
				rel()
			}
			return nil, err
		})
	if err != nil {
		t.Fatalf("nested admit: %v", err)
	}
	// Outer release freed the slot exactly once.
	if _, _, err := h.RunOp(context.Background(), OpSpec{Kind: "read", ConnectionID: strp(connID)}, noopFn); err != nil {
		t.Fatalf("op after nested admit: %v", err)
	}
}

type pureClassifierAdapter struct{ adapters.Adapter }

func (pureClassifierAdapter) ClassifyStatement(context.Context, string) (adapters.OpClass, error) {
	return adapters.ClassRead, nil
}

type remoteClassifierAdapter struct{ pureClassifierAdapter }

func (remoteClassifierAdapter) ClassifiesRemotely() bool { return true }

func TestThrottle_ClassifyStatementTakesTurnOnlyForRemoteClassifier(t *testing.T) {
	r := NewRouter(adapters.Deps{}, nil)
	const connID = "conn-classify-throttle"
	r.host.throttleWait = 50 * time.Millisecond
	r.SetThrottle(connID, 1000)
	release, holderDone := holdOp(t, r.host, connID)
	defer func() { close(release); <-holderDone }()
	defer adapters.DeleteLiveAdapter(connID)

	adapters.SetLiveAdapter(connID, pureClassifierAdapter{})
	if class, err := r.ClassifyStatement(context.Background(), connID, "x"); err != nil || class != adapters.ClassRead {
		t.Fatalf("pure classifier = %v, %v, want read, nil", class, err)
	}

	adapters.SetLiveAdapter(connID, remoteClassifierAdapter{})
	_, err := r.ClassifyStatement(context.Background(), connID, "x")
	if code, _ := adapters.CodeOf(err); code != adapters.CodeTimeout {
		t.Fatalf("remote classifier err = %v, want E_TIMEOUT while the slot is held", err)
	}
}
