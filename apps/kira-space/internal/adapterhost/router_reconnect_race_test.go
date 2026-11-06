package adapterhost

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// hangingDisconnectAdapter mirrors QueryTracker.Drain's own shape (Part 4/5): Disconnect waits for
// whatever ops were in flight when it started to actually finish touching the connection, via a
// plain WaitGroup rather than a real driver's ctx-watched one.
type hangingDisconnectAdapter struct {
	adapters.Adapter
	inFlight sync.WaitGroup
}

func (a *hangingDisconnectAdapter) Connect(context.Context, model.ResolvedConnectionConfig, *adapters.OpCtx) (adapters.ConnectInfo, error) {
	return adapters.ConnectInfo{}, nil
}
func (a *hangingDisconnectAdapter) Disconnect(context.Context) error {
	a.inFlight.Wait()
	return nil
}
func (a *hangingDisconnectAdapter) Caps() adapters.Caps { return adapters.Caps{} }

// A reconnect's own old-adapter Disconnect used to run outside RunOp, with no
// local cancellation of whatever ops were still running against that old adapter first — so a
// Disconnect shaped like QueryTracker.Drain's real inFlight.Wait() (no bound under
// context.Background) hung the whole reconnect for as long as the longest still-running query.
// This confirms Connect's own reconnect branch now cancels those ops' local ctx first, unblocking
// the driver's own ctx watcher (simulated here directly) before the old adapter's Disconnect ever
// has to wait on them.
func TestRouter_Reconnect_CancelsInFlightOpsBeforeOldDisconnectWaits(t *testing.T) {
	const kind = "test-f1-reconnect-hang"
	adapter := &hangingDisconnectAdapter{}
	adapters.Register(kind, func(adapters.Deps) (adapters.Adapter, error) { return adapter, nil })

	const connID = "conn-f1-reconnect-hang"
	adapters.DeleteLiveAdapter(connID)
	r := NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
	adapters.SetLiveAdapter(connID, adapter)

	adapter.inFlight.Add(1)
	opStarted := make(chan struct{})
	opDone := make(chan struct{})
	go func() {
		_, _, _ = r.Host().RunOp(context.Background(), OpSpec{ConnectionID: strp(connID), Kind: "read"},
			func(ctx context.Context, op *adapters.OpCtx) (any, error) {
				close(opStarted)
				<-ctx.Done() // a driver's own ctx watcher aborting the still-running query
				adapter.inFlight.Done()
				return nil, ctx.Err()
			})
		close(opDone)
	}()
	<-opStarted

	cfg := model.ResolvedConnectionConfig{ID: connID, Kind: kind}
	connectDone := make(chan error, 1)
	go func() {
		_, err := r.Connect(context.Background(), cfg)
		connectDone <- err
	}()

	select {
	case err := <-connectDone:
		if err != nil {
			t.Fatalf("reconnect Connect: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect hung — the in-flight op's local ctx was never cancelled before the old adapter's Disconnect ran")
	}
	<-opDone
}

// takeLiveAdapterForTeardown is the compare-and-delete that replaces a bare
// GetLiveAdapter+DeleteLiveAdapter pair. Exactly one of several concurrent callers racing to tear
// down the same registered adapter instance must win (and remove it); every other must
// see there is nothing left for it to do, never delete a *different* adapter that has since
// replaced it. New mechanism, no pre-fix equivalent — failing without the fix is definitional.
func TestRouter_TakeLiveAdapterForTeardown_ExactlyOneCallerWins(t *testing.T) {
	const connID = "conn-f2-cas-race"
	adapters.DeleteLiveAdapter(connID)
	adapter := &reconnectFakeAdapter{}
	adapters.SetLiveAdapter(connID, adapter)

	r := NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))

	const n = 25
	results := make(chan bool, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, ok := r.takeLiveAdapterForTeardown(connID)
			results <- ok
		}()
	}
	wg.Wait()
	close(results)

	wins := 0
	for ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("winners = %d, want exactly 1 — every other concurrent caller must see it already lost", wins)
	}
	if _, stillLive := adapters.GetLiveAdapter(connID); stillLive {
		t.Fatal("the winning caller must have removed the adapter from the live registry")
	}
}

// gatedDisconnectAdapter signals disconnectStarted the first time Disconnect is entered, then
// blocks until the test closes release — the controlled window a concurrent reconnect races into.
type gatedDisconnectAdapter struct {
	adapters.Adapter
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *gatedDisconnectAdapter) Disconnect(context.Context) error {
	a.once.Do(func() { close(a.started) })
	<-a.release
	return nil
}

// reconnectStubAdapter is a pointer-identity stand-in for "the reconnect's newer adapter" — a
// distinct type/instance from reconnectFakeAdapter (a value type elsewhere in this package) so a
// pointer-identity comparison genuinely distinguishes "still registered" from "silently replaced".
type reconnectStubAdapter struct{ adapters.Adapter }

func (*reconnectStubAdapter) Connect(context.Context, model.ResolvedConnectionConfig, *adapters.OpCtx) (adapters.ConnectInfo, error) {
	return adapters.ConnectInfo{}, nil
}
func (*reconnectStubAdapter) Disconnect(context.Context) error { return nil }
func (*reconnectStubAdapter) Caps() adapters.Caps              { return adapters.Caps{} }

// End to end: Router.Disconnect racing a concurrent reconnect for the same
// connection id must never have its own (possibly slow) teardown of the old adapter delete
// whatever newer adapter the reconnect has since installed. Pre-fix, Disconnect captured the old
// adapter, waited on its Disconnect, and only then deleted-by-bare-id — so a reconnect that
// installed a new adapter *while that wait was still in flight* had it deleted out from under it
// the moment the old wait finally returned (confirmed: this test hangs, then fails, against the
// pre-fix Connect/Disconnect via a scoped `git stash` on router.go/live.go, since pre-fix Connect's
// own reconnect branch also redundantly calls the still-registered old adapter's own Disconnect,
// which blocks on the same gate this test only releases after the reconnect has already
// returned — a real deadlock, not just a wrong answer).
func TestRouter_ConcurrentDisconnectAndReconnect_NeverLosesTheNewerAdapter(t *testing.T) {
	const kindB = "test-f2-e2e-b"
	slowA := &gatedDisconnectAdapter{started: make(chan struct{}), release: make(chan struct{})}
	adapterB := &reconnectStubAdapter{}
	adapters.Register(kindB, func(adapters.Deps) (adapters.Adapter, error) { return adapterB, nil })

	const connID = "conn-f2-e2e"
	adapters.DeleteLiveAdapter(connID)
	r := NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
	adapters.SetLiveAdapter(connID, slowA)

	disconnectDone := make(chan struct{})
	go func() {
		_ = r.Disconnect(context.Background(), connID)
		close(disconnectDone)
	}()
	<-slowA.started // Disconnect's own call into the old adapter is now in flight, gated on release

	cfg := model.ResolvedConnectionConfig{ID: connID, Kind: kindB}
	if _, err := r.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("reconnect Connect: %v", err)
	}

	close(slowA.release) // let the original Disconnect's own slow call finally return
	select {
	case <-disconnectDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Disconnect never finished after its old adapter's Disconnect call returned")
	}

	live, ok := adapters.GetLiveAdapter(connID)
	if !ok || live != adapters.Adapter(adapterB) {
		t.Fatalf("live adapter after the race = (%v, %v), want the reconnect's own adapterB, true — "+
			"Disconnect's own stale teardown must not delete the reconnect's newer adapter", live, ok)
	}
}

// F2 (P168 Part 5): a slow Disconnect finishing after a newer Connect must leave that session's
// throttle in place.
func TestRouter_SlowDisconnect_KeepsNewerConnectionThrottle(t *testing.T) {
	const kindB = "test-p168-f2-throttle"
	slowA := &gatedDisconnectAdapter{started: make(chan struct{}), release: make(chan struct{})}
	adapters.Register(kindB, func(adapters.Deps) (adapters.Adapter, error) { return &reconnectStubAdapter{}, nil })

	const connID = "conn-p168-f2-throttle"
	adapters.DeleteLiveAdapter(connID)
	r := NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
	adapters.SetLiveAdapter(connID, slowA)

	done := make(chan struct{})
	go func() {
		_ = r.Disconnect(context.Background(), connID)
		close(done)
	}()
	<-slowA.started

	r.SetThrottle(connID, 5)
	if _, err := r.Connect(context.Background(), model.ResolvedConnectionConfig{ID: connID, Kind: kindB}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	close(slowA.release)
	<-done

	if r.host.throttles.gateFor(connID) == nil {
		t.Fatal("the older Disconnect cleared the newer connection's throttle")
	}
}
