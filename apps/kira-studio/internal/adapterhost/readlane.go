package adapterhost

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// laneIdleTimeout closes a read lane that served no op for this long.
const laneIdleTimeout = 10 * time.Minute

// laneKinds are the kinds whose read statements run on a second adapter. The rest have no write
// path (kibana) or no console (kafka, s3, sqs): they run on the main adapter.
var laneKinds = map[string]bool{
	"postgres": true, "mysql": true, "mariadb": true, "sqlite": true,
	"clickhouse": true, "mongodb": true, "redis": true,
}

// readLane is one connection's second adapter: same credentials, read-only session.
type readLane struct {
	adapter  adapters.Adapter
	primary  adapters.Adapter // the live adapter this lane was opened beside
	inflight int
	idle     *time.Timer
}

// readLanes is the Router's lane registry plus the last resolved config per connection.
type readLanes struct {
	mu   sync.Mutex
	byID map[string]*readLane
	cfgs map[string]model.ResolvedConnectionConfig
	// open serialises lane opens per connection so concurrent first calls open one lane.
	open      *keyedMutex
	idleAfter time.Duration
}

func newReadLanes() *readLanes {
	return &readLanes{
		byID: make(map[string]*readLane), cfgs: make(map[string]model.ResolvedConnectionConfig),
		open: newKeyedMutex(), idleAfter: laneIdleTimeout,
	}
}

// laneConfig is cfg with the read-only switch on and the pool cap marker: same credentials.
func laneConfig(cfg model.ResolvedConnectionConfig) model.ResolvedConnectionConfig {
	lane := cfg
	lane.ReadOnly = true
	lane.Options = maps.Clone(cfg.Options)
	if lane.Options == nil {
		lane.Options = make(map[string]any, 1)
	}
	lane.Options[adapters.LaneOption] = true
	return lane
}

func (l *readLanes) setConfig(cfg model.ResolvedConnectionConfig) {
	l.mu.Lock()
	l.cfgs[cfg.ID] = cfg
	l.mu.Unlock()
}

func (l *readLanes) forgetConfig(connectionID string) {
	l.mu.Lock()
	delete(l.cfgs, connectionID)
	l.mu.Unlock()
}

func (l *readLanes) config(connectionID string) (model.ResolvedConnectionConfig, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cfg, ok := l.cfgs[connectionID]
	return cfg, ok
}

// checkout returns the open lane beside primary with its in-flight count raised. A lane opened
// beside an older primary is removed and returned as stale for the caller to close.
func (l *readLanes) checkout(connectionID string, primary adapters.Adapter) (lane, stale *readLane) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cur := l.byID[connectionID]
	if cur == nil {
		return nil, nil
	}
	if cur.primary != primary {
		delete(l.byID, connectionID)
		return nil, cur
	}
	cur.inflight++
	if cur.idle != nil {
		cur.idle.Stop()
	}
	return cur, nil
}

// release drops one in-flight use and arms the idle timer once the lane is unused.
func (l *readLanes) release(connectionID string, lane *readLane, closeIdle func(string, *readLane)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lane.inflight--
	if lane.inflight > 0 || l.byID[connectionID] != lane {
		return
	}
	fire := func() {
		l.mu.Lock()
		if l.byID[connectionID] != lane || lane.inflight > 0 {
			l.mu.Unlock()
			return
		}
		delete(l.byID, connectionID)
		l.mu.Unlock()
		closeIdle(connectionID, lane)
	}
	if lane.idle == nil {
		lane.idle = time.AfterFunc(l.idleAfter, fire)
		return
	}
	lane.idle.Reset(l.idleAfter)
}

// take removes and returns connectionID's lane, if any.
func (l *readLanes) take(connectionID string) *readLane {
	l.mu.Lock()
	defer l.mu.Unlock()
	lane := l.byID[connectionID]
	delete(l.byID, connectionID)
	return lane
}

func (l *readLanes) takeAll() map[string]*readLane {
	l.mu.Lock()
	defer l.mu.Unlock()
	all := l.byID
	l.byID = make(map[string]*readLane)
	return all
}

// ExecuteRead runs req on connectionID's read lane: a second adapter with the same credentials
// opened in the engine's read-only session mode. It never falls back to the main adapter when the
// lane cannot open. Kinds without a lane run on the main adapter (they have no write path).
func (r *Router) ExecuteRead(ctx context.Context, req ExecuteRequestWire) (ExecuteResponse, error) {
	primary, err := requireLiveAdapter(req.ConnectionID)
	if err != nil {
		return ExecuteResponse{}, err
	}
	req.ReadOnly = true
	cfg, ok := r.lanes.config(req.ConnectionID)
	if !ok {
		return ExecuteResponse{}, adapters.New(adapters.CodeEngineDown, "connection "+req.ConnectionID+" is not connected", nil)
	}
	if !laneKinds[cfg.Kind] {
		return r.dispatcher.executeOn(ctx, primary, nil, req)
	}
	lane, err := r.acquireLane(ctx, req.ConnectionID, primary, cfg)
	if err != nil {
		return ExecuteResponse{}, err
	}
	defer r.lanes.release(req.ConnectionID, lane, r.closeIdleLane)
	return r.dispatcher.executeOn(ctx, lane.adapter, lane.adapter, req)
}

// acquireLane returns connectionID's lane with its use counted, opening it on first use.
func (r *Router) acquireLane(ctx context.Context, connectionID string, primary adapters.Adapter, cfg model.ResolvedConnectionConfig) (*readLane, error) {
	unlock := r.lanes.open.lock(connectionID)
	defer unlock()
	lane, stale := r.lanes.checkout(connectionID, primary)
	if stale != nil {
		r.teardownLane(ctx, connectionID, stale)
	}
	if lane != nil {
		return lane, nil
	}
	return r.openLane(ctx, connectionID, primary, cfg)
}

// openLane connects a fresh adapter with laneConfig(cfg) and registers it, counted as in use.
func (r *Router) openLane(ctx context.Context, connectionID string, primary adapters.Adapter, cfg model.ResolvedConnectionConfig) (*readLane, error) {
	adapter, err := adapters.CreateAdapter(cfg.Kind, r.deps)
	if err != nil {
		return nil, err
	}
	id := connectionID
	if _, _, err := r.host.RunOp(ctx, OpSpec{ConnectionID: &id, Kind: "connect", Adapter: adapter},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			return adapter.Connect(ctx, laneConfig(cfg), op)
		}); err != nil {
		boundedCleanup, cancel := context.WithTimeout(context.Background(), disconnectTimeout)
		_ = adapter.Disconnect(boundedCleanup)
		cancel()
		return nil, err
	}
	lane := &readLane{adapter: adapter, primary: primary, inflight: 1}
	r.lanes.mu.Lock()
	r.lanes.byID[connectionID] = lane
	r.lanes.mu.Unlock()
	// The primary can be torn down between the caller's lookup and the insert above. Teardown
	// takes the primary before it closes lanes, so a primary still live here means the close is
	// still to come and will see this lane.
	if cur, ok := adapters.GetLiveAdapter(connectionID); !ok || cur != primary {
		if r.lanes.discard(connectionID, lane) {
			r.teardownLane(ctx, connectionID, lane)
		}
		return nil, adapters.New(adapters.CodeEngineDown, "the connection changed while its read connection opened; retry", nil)
	}
	return lane, nil
}

func (l *readLanes) stopIdle(lane *readLane) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lane.idle != nil {
		lane.idle.Stop()
	}
}

// discard removes lane from the registry if it is still the registered one.
func (l *readLanes) discard(connectionID string, lane *readLane) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.byID[connectionID] != lane {
		return false
	}
	delete(l.byID, connectionID)
	return true
}

// teardownLane cancels the lane's ops and disconnects it, bounded by disconnectTimeout.
func (r *Router) teardownLane(ctx context.Context, connectionID string, lane *readLane) {
	r.lanes.stopIdle(lane)
	r.host.CancelOpsOnAdapter(lane.adapter)
	r.disconnectAdapter(ctx, connectionID, lane.adapter)
}

func (r *Router) closeIdleLane(connectionID string, lane *readLane) {
	r.teardownLane(context.Background(), connectionID, lane)
}

// CloseReadLane tears connectionID's read lane down (reconnect, disconnect, exposure off).
func (r *Router) CloseReadLane(ctx context.Context, connectionID string) {
	if lane := r.lanes.take(connectionID); lane != nil {
		r.teardownLane(ctx, connectionID, lane)
	}
}

// CloseReadLanes tears every read lane down (DB MCP server stop, app quit).
func (r *Router) CloseReadLanes(ctx context.Context) {
	for id, lane := range r.lanes.takeAll() {
		r.teardownLane(ctx, id, lane)
	}
}

// ReadLaneOpen reports whether connectionID has an open read lane.
func (r *Router) ReadLaneOpen(connectionID string) bool {
	r.lanes.mu.Lock()
	defer r.lanes.mu.Unlock()
	return r.lanes.byID[connectionID] != nil
}
