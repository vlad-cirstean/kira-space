package adapterhost

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// probeFake is a primary or lane adapter whose Connect and Ping outcomes a test controls.
type probeFake struct {
	adapters.Adapter
	mu          sync.Mutex
	connects    int
	pings       int
	disconnects int
	connectErr  error
	pingErr     error
}

func (a *probeFake) Caps() adapters.Caps { return adapters.Caps{} }
func (a *probeFake) Connect(context.Context, model.ResolvedConnectionConfig, *adapters.OpCtx) (adapters.ConnectInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.connects++
	return adapters.ConnectInfo{}, a.connectErr
}
func (a *probeFake) Disconnect(context.Context) error {
	a.mu.Lock()
	a.disconnects++
	a.mu.Unlock()
	return nil
}
func (a *probeFake) Ping(context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pings++
	return a.pingErr
}
func (a *probeFake) counts() (connects, pings, disconnects int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connects, a.pings, a.disconnects
}
func (a *probeFake) set(connectErr, pingErr error) {
	a.mu.Lock()
	a.connectErr, a.pingErr = connectErr, pingErr
	a.mu.Unlock()
}

type probeEnv struct {
	r       *Router
	primary *probeFake
	connID  string
	mu      sync.Mutex
	lanes   []*probeFake
	next    func() *probeFake
}

func (e *probeEnv) lane(i int) *probeFake {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lanes[i]
}

func (e *probeEnv) laneCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.lanes)
}

func newProbeEnv(t *testing.T) *probeEnv {
	t.Helper()
	kind := "test-probe-" + t.Name()
	e := &probeEnv{primary: &probeFake{}, connID: "conn-" + t.Name()}
	adapters.Register(kind, func(adapters.Deps) (adapters.Adapter, error) {
		lane := &probeFake{}
		if e.next != nil {
			lane = e.next()
		}
		e.mu.Lock()
		e.lanes = append(e.lanes, lane)
		e.mu.Unlock()
		return lane, nil
	})
	laneKinds[kind] = true
	t.Cleanup(func() { delete(laneKinds, kind); adapters.DeleteLiveAdapter(e.connID) })
	e.r = NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
	adapters.SetLiveAdapter(e.connID, e.primary)
	e.r.lanes.setConfig(model.ResolvedConnectionConfig{ID: e.connID, Kind: kind})
	return e
}

func probeErr(t *testing.T, err error) *ProbeError {
	t.Helper()
	var pe *ProbeError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want *ProbeError", err)
	}
	return pe
}

func TestRouter_Probe(t *testing.T) {
	down := adapters.New(adapters.CodeEngineDown, "server gone", nil)

	t.Run("a fresh lane open is the probe: no ping", func(t *testing.T) {
		e := newProbeEnv(t)
		if err := e.r.Probe(context.Background(), e.connID, true); err != nil {
			t.Fatalf("Probe: %v", err)
		}
		connects, pings, _ := e.lane(0).counts()
		if connects != 1 || pings != 0 || !e.r.ReadLaneOpen(e.connID) {
			t.Errorf("lane connects/pings = %d/%d open=%v, want 1/0 true", connects, pings, e.r.ReadLaneOpen(e.connID))
		}
	})

	t.Run("an idle lane is pinged, not reopened", func(t *testing.T) {
		e := newProbeEnv(t)
		_ = e.r.Probe(context.Background(), e.connID, true)
		if err := e.r.Probe(context.Background(), e.connID, true); err != nil {
			t.Fatalf("Probe: %v", err)
		}
		connects, pings, _ := e.lane(0).counts()
		if connects != 1 || pings != 1 {
			t.Errorf("lane connects/pings = %d/%d, want 1/1", connects, pings)
		}
	})

	t.Run("a busy lane skips the ping", func(t *testing.T) {
		e := newProbeEnv(t)
		_ = e.r.Probe(context.Background(), e.connID, true)
		busy, _ := e.r.lanes.checkout(e.connID, e.primary)
		defer e.r.lanes.release(e.connID, busy, e.r.closeIdleLane)
		if err := e.r.Probe(context.Background(), e.connID, true); err != nil {
			t.Fatalf("Probe: %v", err)
		}
		if _, pings, _ := e.lane(0).counts(); pings != 0 {
			t.Errorf("lane pings = %d, want 0", pings)
		}
	})

	t.Run("a stale lane is torn down and reopened once", func(t *testing.T) {
		e := newProbeEnv(t)
		_ = e.r.Probe(context.Background(), e.connID, true)
		e.lane(0).set(nil, down)
		if err := e.r.Probe(context.Background(), e.connID, true); err != nil {
			t.Fatalf("Probe: %v", err)
		}
		if _, _, disconnects := e.lane(0).counts(); disconnects != 1 {
			t.Errorf("old lane disconnects = %d, want 1", disconnects)
		}
		if e.laneCount() != 2 {
			t.Fatalf("lanes opened = %d, want 2", e.laneCount())
		}
		if connects, _, _ := e.lane(1).counts(); connects != 1 {
			t.Errorf("new lane connects = %d, want 1", connects)
		}
	})

	t.Run("a lane that cannot open and a dead primary report Primary", func(t *testing.T) {
		e := newProbeEnv(t)
		e.next = func() *probeFake { return &probeFake{connectErr: down} }
		e.primary.set(nil, down)
		pe := probeErr(t, e.r.Probe(context.Background(), e.connID, true))
		if !pe.Primary {
			t.Error("Primary = false, want true")
		}
		if e.r.ReadLaneOpen(e.connID) {
			t.Error("a failed lane open left a lane registered")
		}
	})

	t.Run("a lane that cannot open on a live primary stays a lane failure", func(t *testing.T) {
		e := newProbeEnv(t)
		e.next = func() *probeFake { return &probeFake{connectErr: down} }
		pe := probeErr(t, e.r.Probe(context.Background(), e.connID, true))
		if pe.Primary {
			t.Error("Primary = true, want false")
		}
		if _, pings, _ := e.primary.counts(); pings != 1 {
			t.Errorf("primary confirm pings = %d, want 1", pings)
		}
	})

	t.Run("a busy primary skips the ping", func(t *testing.T) {
		e := newProbeEnv(t)
		e.primary.set(nil, down)
		started, release := make(chan struct{}), make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _, _ = e.r.Host().RunOp(context.Background(), OpSpec{ConnectionID: strp(e.connID), Kind: "read"},
				func(context.Context, *adapters.OpCtx) (any, error) { close(started); <-release; return nil, nil })
		}()
		<-started
		err := e.r.Probe(context.Background(), e.connID, false)
		close(release)
		<-done
		if err != nil {
			t.Errorf("Probe on a busy primary = %v, want nil", err)
		}
		if _, pings, _ := e.primary.counts(); pings != 0 {
			t.Errorf("primary pings = %d, want 0", pings)
		}
	})

	t.Run("a primary that fails both pings reports Primary", func(t *testing.T) {
		e := newProbeEnv(t)
		e.primary.set(nil, down)
		pe := probeErr(t, e.r.Probe(context.Background(), e.connID, false))
		if !pe.Primary {
			t.Error("Primary = false, want true")
		}
		if _, pings, _ := e.primary.counts(); pings != 2 {
			t.Errorf("primary pings = %d, want 2 (one retry)", pings)
		}
	})
}
