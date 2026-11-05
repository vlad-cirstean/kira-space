package adapterhost

import (
	"context"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// Router is the concrete type that satisfies connections.Backend, tree.Backend and
// bridge.Canceller structurally — one type, three small consumer-declared interfaces. It also owns
// the data-plane routing (dataframe.go) and the per-session write queue (session.go).
//
// Every kind has been served in-process (ten of ten), so the Node engine child this Router used to
// forward non-native kinds to is gone — there is no more routing decision to make, only the native
// path.
type Router struct {
	deps       adapters.Deps
	host       *Host
	dispatcher *Dispatcher
	cache      *enginecache.Cache

	// teardown serializes the check-and-swap step (see takeLiveAdapterForTeardown) per
	// connection id, so a reconnect's own old-adapter teardown and an explicit Disconnect racing
	// for the same id can never both "win" tearing down the same adapter instance, or race each
	// other into observing a stale GetLiveAdapter result between the read and the compare-and-
	// delete. Held only across that read-then-CAS step, never across the adapter's own Disconnect
	// call or RunOp — those can run for up to disconnectTimeout, and a second caller only needs to
	// learn "there is nothing left for me to tear down here", not wait that out.
	teardown *keyedMutex

	// connects tracks per-id Connect activity so a slow Disconnect can tell a newer connection
	// started while it ran (F2, P168 Part 5), guarded by teardown.lock.
	connects map[string]*connectState

	// lanes holds the DB MCP read lanes and the resolved config each was derived from (readlane.go).
	lanes *readLanes
}

// connectState: epoch bumps when a Connect begins and ends; active counts Connects in flight.
type connectState struct {
	epoch  uint64
	active int
}

// NewRouter constructs a Router. deps.Log is normalised (withDefaultLog, P21 round 3 finding 8)
// before either Router or Host stores its own copy, so both always see a callable Log.
func NewRouter(deps adapters.Deps, cache *enginecache.Cache) *Router {
	deps = withDefaultLog(deps)
	host := NewHost(deps, cache)
	return &Router{deps: deps, host: host, dispatcher: NewDispatcher(host, cache), cache: cache, teardown: newKeyedMutex(), connects: make(map[string]*connectState), lanes: newReadLanes()}
}

// keyedMutex is a per-key mutex, used by Router to serialize the check-and-swap step per
// connection id. Its own map only ever grows (one entry per connection id ever seen), the same
// trade-off throttleRegistry already makes — bounded by how many connections this app's user has,
// never unboundedly many.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newKeyedMutex() *keyedMutex { return &keyedMutex{locks: make(map[string]*sync.Mutex)} }

// lock acquires key's own mutex, minting one on first use, and returns the matching unlock func.
func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	l, ok := k.locks[key]
	if !ok {
		l = &sync.Mutex{}
		k.locks[key] = l
	}
	k.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// disconnectTimeout bounds how long an old adapter's own Disconnect may run, on top of whatever
// ctx the caller already provided. QueryTracker.Drain and ConnSet.CloseAll have
// no bound of their own under context.Background (Router.Disconnect's own callers all pass it, by
// contract — connections/service.go), so without this a dead-network TCP retransmit timeout, or an
// in-flight op still holding CloseAll's own entry mutex, can hang a reconnect or an explicit
// Disconnect for minutes. CancelOpsForConnection (called first, at every call site below) is what
// unblocks the common case quickly; this is the backstop for whatever it cannot reach.
const disconnectTimeout = 10 * time.Second

// takeLiveAdapterForTeardown wins the exclusive right to tear connectionID's currently-registered
// adapter down, if one is registered right now, and removes it from the registry as part of
// winning that right — F2 (P108 Part 6). Serialized (teardown.lock) so a concurrent reconnect and
// Disconnect for the same id can never both capture the same adapter instance and race each other
// to delete it: whichever caller's compare-and-delete runs second inside the lock simply finds the
// first has already removed it and reports ok=false, the same outcome as if nothing had ever been
// live. Never returns an adapter a caller does not now exclusively own tearing down.
func (r *Router) takeLiveAdapterForTeardown(connectionID string) (adapters.Adapter, bool) {
	adapter, _, ok := r.takeLiveAdapterWithEpoch(connectionID)
	return adapter, ok
}

// takeLiveAdapterWithEpoch is takeLiveAdapterForTeardown plus the connect epoch at the moment of
// winning, for Disconnect's newer-connection check.
func (r *Router) takeLiveAdapterWithEpoch(connectionID string) (adapters.Adapter, uint64, bool) {
	unlock := r.teardown.lock(connectionID)
	defer unlock()
	existing, ok := adapters.GetLiveAdapter(connectionID)
	if !ok {
		return nil, 0, false
	}
	if !adapters.DeleteLiveAdapterIf(connectionID, existing) {
		// Raced: another caller already won (and removed) this exact adapter between the Get above
		// and here, or has since installed a different one — not ours to tear down either way.
		return nil, 0, false
	}
	return existing, r.connectStateLocked(connectionID).epoch, true
}

func (r *Router) connectStateLocked(connectionID string) *connectState {
	st, ok := r.connects[connectionID]
	if !ok {
		st = &connectState{}
		r.connects[connectionID] = st
	}
	return st
}

// beginConnect marks a Connect in flight and returns its end func.
func (r *Router) beginConnect(connectionID string) func() {
	unlock := r.teardown.lock(connectionID)
	st := r.connectStateLocked(connectionID)
	st.epoch++
	st.active++
	unlock()
	return func() {
		unlock := r.teardown.lock(connectionID)
		defer unlock()
		st.epoch++
		st.active--
	}
}

// supersededSince reports whether a Connect began, ran or ended after epoch was read, or is
// running now: the old session's cleanup must not touch the newer one's state.
func (r *Router) supersededSince(connectionID string, epoch uint64) bool {
	unlock := r.teardown.lock(connectionID)
	defer unlock()
	st := r.connectStateLocked(connectionID)
	return st.epoch != epoch || st.active > 0
}

// disconnectAdapter runs adapter's own Disconnect inside RunOp (kind "disconnect") — so it is
// both op-logged (visible, and locally cancellable, from the Operations panel) and bounded to ctx
// plus disconnectTimeout, rather than the unbounded context.Background() every call site used to
// pass. The caller must already own exclusive teardown of this exact adapter instance
// (takeLiveAdapterForTeardown) — there is nothing left to undo on an error here besides logging it.
func (r *Router) disconnectAdapter(ctx context.Context, connectionID string, adapter adapters.Adapter) {
	bounded, cancel := context.WithTimeout(ctx, disconnectTimeout)
	defer cancel()
	id := connectionID
	if _, _, err := r.host.RunOp(bounded, OpSpec{ConnectionID: &id, Kind: "disconnect"},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			return nil, adapter.Disconnect(ctx)
		}); err != nil {
		r.deps.Log("warn", "disconnecting the old adapter for "+connectionID+" failed: "+err.Error())
	}
}

// Host returns the router's own scheduler, for callers that need to Subscribe to op:start/op:end
// (oplog.New, main.go — P58f D9) or push cache config to the same Cache the router's Backend
// methods use.
func (r *Router) Host() *Host { return r.host }

// SetThrottle delegates to the host's own registry — P28 §5.5's connections.Backend method,
// called on connect and on a live edit (connections/service.go), and cleared in Disconnect below.
func (r *Router) SetThrottle(connectionID string, perSec float64) {
	r.host.SetThrottle(connectionID, perSec)
}

// PushCacheConfig pushes engine-relevant settings (today: the L2 cache byte budget) into the
// Go-native cache this router's Dispatcher reads (§4.9).
func (r *Router) PushCacheConfig(settings model.Settings) {
	r.cache.Configure(settings.Cache.L2BudgetMb * 1024 * 1024)
}

// ---- connections.Backend ----

// Connect is the Go analogue of control.ts's handleConnect.
func (r *Router) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig) (connections.ConnectResult, error) {
	// A reconnect is a disconnect + connect, never two live clients for the same connection.
	// Cancel every op still running against whatever is currently live for this id first — its own
	// driver ctx watcher unblocks the query goroutine, so the old adapter's Disconnect below (if
	// this call wins tearing it down) does not have to wait out QueryTracker.Drain/ConnSet.CloseAll
	// with no bound of their own. Safe to call even when nothing is live for this id (a fresh
	// connect, not a reconnect) — it finds nothing to cancel.
	r.host.CancelOpsForConnection(cfg.ID, "")
	r.CloseReadLane(ctx, cfg.ID)
	defer r.beginConnect(cfg.ID)()
	// takeLiveAdapterForTeardown is the compare-and-delete that replaces the old bare
	// GetLiveAdapter+DeleteLiveAdapter pair — a concurrent Disconnect racing this same reconnect
	// (Router.Disconnect is not serialized against Connect, by connections.Service's own contract)
	// can no longer have its own stale removal delete whatever new adapter this call goes on to
	// install, nor can this call's own teardown ever delete an adapter Disconnect has since
	// replaced this one with (it cannot: only one of them will ever be live at a time under the
	// same id, and only one CAS can win it).
	if existing, ok := r.takeLiveAdapterForTeardown(cfg.ID); ok {
		// F3 (P168 Part 5): drop before the slow teardown so cache hits stop serving a dying session.
		r.cache.DropConnection(cfg.ID)
		r.lanes.forgetConfig(cfg.ID)
		r.CloseReadLane(ctx, cfg.ID)
		r.disconnectAdapter(ctx, cfg.ID, existing)
		// Mirrors Disconnect's own DropConnection call below — a reconnect that races
		// ahead of onPreconnectExit's own async Disconnect (connections/service.go) lands here,
		// in this branch, rather than through Disconnect at all, so without this the old
		// connectionId's L2 pages and L3 counts survive the reconnect and can be served back as
		// stale data/row-count results against the newly connected adapter. Gated on this call
		// having won teardown above: if a concurrent Disconnect won it instead, its
		// own DropConnection already ran, and dropping again here would be redundant, not wrong,
		// but the gate keeps the two paths from ever disagreeing about whose job this was.
		r.cache.DropConnection(cfg.ID)
	}
	adapter, err := adapters.CreateAdapter(cfg.Kind, r.deps)
	if err != nil {
		return connections.ConnectResult{}, err
	}

	id := cfg.ID
	_, value, err := r.host.RunOp(ctx, OpSpec{ConnectionID: &id, Kind: "connect"},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			return adapter.Connect(ctx, cfg, op)
		})
	if err != nil {
		// The engine created this adapter, so it disconnects it on every path, including
		// a failed probe or an aborted connect — an adapter left un-disconnected here can leak
		// whatever its driver already opened. This adapter was never registered live (that
		// happens only below, on success), so there is nothing to race a reconnect/Disconnect for
		// — the bound still applies (a fixed upper bound, not the unbounded Background() this
		// used to pass, on top of ctx so a caller-driven cancel still short-circuits it).
		boundedCleanup, cancelCleanup := context.WithTimeout(context.Background(), disconnectTimeout)
		_ = adapter.Disconnect(boundedCleanup)
		cancelCleanup()
		return connections.ConnectResult{}, err
	}
	r.lanes.setConfig(cfg)
	adapters.SetLiveAdapter(cfg.ID, adapter)
	info := value.(adapters.ConnectInfo)
	return connections.ConnectResult{ServerVersion: info.ServerVersion, Caps: adapter.Caps()}, nil
}

// Test is control.ts's handleTest: a throwaway adapter, never registered live, connected and
// unconditionally disconnected.
func (r *Router) Test(ctx context.Context, cfg model.ResolvedConnectionConfig) (string, error) {
	adapter, err := adapters.CreateAdapter(cfg.Kind, r.deps)
	if err != nil {
		return "", err
	}
	// Unconditional, so a failed probe is cleaned up the same as a successful one. The same fixed
	// upper bound as Connect's own failed-probe cleanup — this adapter was never registered live
	// either, so context.Background() is still the right base (a cancelled caller ctx must not skip
	// real cleanup of a probe connection), just no longer unbounded.
	defer func() {
		boundedCleanup, cancelCleanup := context.WithTimeout(context.Background(), disconnectTimeout)
		defer cancelCleanup()
		_ = adapter.Disconnect(boundedCleanup)
	}()

	_, value, err := r.host.RunOp(ctx, OpSpec{Kind: "test"},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			return adapter.Connect(ctx, cfg, op)
		})
	if err != nil {
		return "", err
	}
	return value.(adapters.ConnectInfo).ServerVersion, nil
}

// RunPreconnect is connections.Backend's pre-connect hook: run (the script's startup wait) executes
// as a `preconnect` op whose command is the script text, so the Operations panel lists it with a
// timer and can cancel it.
func (r *Router) RunPreconnect(ctx context.Context, connectionID *string, command string, delay time.Duration, run func(context.Context) error) error {
	_, _, err := r.host.RunOp(ctx, OpSpec{ConnectionID: connectionID, Kind: "preconnect", DelayMs: int(delay.Milliseconds())},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			op.SetCommand(command)
			return nil, run(ctx)
		})
	return err
}

// Disconnect covers all three fire-and-forget call sites (onPreconnectExit, Remove, Disconnect —
// the count settled on three connections.Backend methods, not four). Every one of them
// passes context.Background() by contract (connections/service.go), so disconnectAdapter's own
// bound is what keeps this from hanging on a dead adapter regardless of what ctx arrives here.
func (r *Router) Disconnect(ctx context.Context, connectionID string) error {
	// F1: see Connect's reconnect branch for why this runs first.
	r.host.CancelOpsForConnection(connectionID, "")
	r.CloseReadLane(ctx, connectionID)
	// takeLiveAdapterForTeardown, not a bare GetLiveAdapter+DeleteLiveAdapter — a concurrent
	// reconnect (Router.Connect) racing this same id can otherwise install a newer adapter that
	// this call's own delete-by-id would then remove out from under it (the adapter leaks, never
	// itself disconnected, while the UI still reports "connected" and every op on it starts
	// failing with E_ENGINE_DOWN). ok is false both when nothing was ever live and when a racing
	// caller already won teardown of it — either way, fire-and-forget Disconnect has nothing left
	// to do.
	adapter, epoch, ok := r.takeLiveAdapterWithEpoch(connectionID)
	if !ok {
		return nil
	}
	// F3 (P168 Part 5): drop before the slow teardown so cache hits stop serving a dying session.
	r.cache.DropConnection(connectionID)
	r.CloseReadLane(ctx, connectionID)
	// Always attempted below, even on disconnectAdapter's own internal error (it only logs) —
	// this call already exclusively owns tearing this adapter down by the time we reach here, so
	// there is no error path left that could skip the cache/throttle cleanup a partial-then-return
	// used to risk.
	r.disconnectAdapter(ctx, connectionID, adapter)
	// F2 (P168 Part 5): a Connect that ran meanwhile owns the throttle and cache now.
	if r.supersededSince(connectionID, epoch) {
		return nil
	}
	r.lanes.forgetConfig(connectionID)
	// Disconnecting releases the connection's driver state; its pages already dropped above.
	// A limiter's lifetime matches the live adapter's — cleared alongside it, covering
	// every disconnect path (Remove, onPreconnectExit, an explicit Disconnect) with no second call
	// site needed in the service.
	r.host.SetThrottle(connectionID, 0)
	return nil
}

// ---- tree.Backend ----

func (r *Router) Children(ctx context.Context, connectionID string, path model.NodePath) (adapters.TreeChildren, error) {
	adapter, err := requireLiveAdapter(connectionID)
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	children, err := runOp(ctx, r.host, connectionID, "children", "", nil, adapter,
		func(ctx context.Context, adapter adapters.Adapter, op *adapters.OpCtx) (adapters.TreeChildren, error) {
			result, err := adapter.Children(ctx, path, op)
			if err != nil {
				return adapters.TreeChildren{}, err
			}
			op.SetRows(len(result.Nodes))
			return result, nil
		})
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	// Same nil-slice-over-the-wire hazard Describe/Definition already guard against: a native
	// adapter's `var nodes []model.TreeNode` left empty marshals as
	// `null`, not `[]`, and Adapter rule 5 requires Children() to answer a leaf with an empty
	// list, never null -- project/state/tree.ts's `if (treeState.children[k])` and
	// filterTree.ts's `Object.entries` both treat null as "not loaded" or crash outright.
	if children.Nodes == nil {
		children.Nodes = []model.TreeNode{}
	}
	return children, nil
}

func (r *Router) Describe(ctx context.Context, connectionID string, path model.NodePath, tabID *string) (model.ObjectMeta, error) {
	adapter, err := requireLiveAdapter(connectionID)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	return runOp(ctx, r.host, connectionID, "describe", "", tabID, adapter,
		func(ctx context.Context, adapter adapters.Adapter, op *adapters.OpCtx) (model.ObjectMeta, error) {
			meta, err := adapter.Describe(ctx, path, op)
			if err != nil {
				return model.ObjectMeta{}, err
			}
			// Native adapters build their list fields (e.g. ReferencedBy) as `var x []T` and leave
			// them nil when empty, which json.Marshal renders as `null` — a cached result gets this
			// normalization for free from tree/service.go's own JSON round trip through the cache,
			// but a live Describe result never passes through there, so it needs it here.
			model.ValidateObjectMeta(&meta)
			op.SetRows(len(meta.Columns))
			return meta, nil
		})
}

func (r *Router) Definition(ctx context.Context, connectionID string, path model.NodePath, tabID *string) (model.ObjectDefinition, error) {
	adapter, err := requireLiveAdapter(connectionID)
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	return runOp(ctx, r.host, connectionID, "definition", "", tabID, adapter,
		func(ctx context.Context, adapter adapters.Adapter, op *adapters.OpCtx) (model.ObjectDefinition, error) {
			def, err := adapter.Definition(ctx, path, op)
			if err != nil {
				return model.ObjectDefinition{}, err
			}
			// Same nil-slice-over-the-wire hazard as Describe above, for Notes/Constraints/
			// Sections.
			model.ValidateObjectDefinition(&def)
			op.SetRows(len(def.Statements))
			return def, nil
		})
}

// SchemaColumns is P22c D1/D2's schema-wide sibling of Describe.
func (r *Router) SchemaColumns(ctx context.Context, connectionID string, path model.NodePath) ([]model.RelationColumns, error) {
	adapter, err := requireLiveAdapter(connectionID)
	if err != nil {
		return nil, err
	}
	relations, err := runOp(ctx, r.host, connectionID, "schemaColumns", "", nil, adapter,
		func(ctx context.Context, adapter adapters.Adapter, op *adapters.OpCtx) ([]model.RelationColumns, error) {
			relations, err := adapter.SchemaColumns(ctx, path, op)
			if err != nil {
				return nil, err
			}
			total := 0
			for i := range relations {
				// Same nil-slice-over-the-wire hazard Describe/Definition already guard against.
				if relations[i].Columns == nil {
					relations[i].Columns = []model.ColumnMeta{}
				}
				total += len(relations[i].Columns)
			}
			op.SetRows(total)
			return relations, nil
		})
	if err != nil {
		return nil, err
	}
	if relations == nil {
		relations = []model.RelationColumns{}
	}
	return relations, nil
}

// KeyTypes is P63 §4.3's tree.Backend method — mirrors SchemaColumns' shape, uncached (tree/
// service.go's own KeyTypes never reads/writes the metadata cache, see its doc comment).
func (r *Router) KeyTypes(ctx context.Context, connectionID string, paths []model.NodePath) ([]string, error) {
	adapter, err := requireLiveAdapter(connectionID)
	if err != nil {
		return nil, err
	}
	types, err := runOp(ctx, r.host, connectionID, "keyTypes", "", nil, adapter,
		func(ctx context.Context, adapter adapters.Adapter, op *adapters.OpCtx) ([]string, error) {
			types, err := adapter.KeyTypes(ctx, paths, op)
			if err != nil {
				return nil, err
			}
			op.SetRows(len(types))
			return types, nil
		})
	if err != nil {
		return nil, err
	}
	if types == nil {
		types = []string{}
	}
	return types, nil
}

// ---- dbmcp.QueryRunner ----

// Execute forwards to the dispatcher's own Execute — the in-process peer of dataframe.go's
// "data:execute" case, which the console's own run() calls through. internal/dbmcp's run_query
// tool is this seam's other caller: same throttling, op-logging, cancellation,
// panic recovery and each adapter's own read-only wrap, nothing re-implemented.
func (r *Router) Execute(ctx context.Context, req ExecuteRequestWire) (ExecuteResponse, error) {
	return r.dispatcher.Execute(ctx, req)
}

// ClassifyStatement answers what one console statement would do on connectionID's live adapter —
// dbmcp's own permission gate. Deliberately outside RunOp: it issues no server work of its
// own (redis's COMMAND table is fetched once per connection set and cached), and the Execute it
// gates is the op that belongs in the op-log.
func (r *Router) ClassifyStatement(ctx context.Context, connectionID, statement string) (adapters.OpClass, error) {
	adapter, err := requireLiveAdapter(connectionID)
	if err != nil {
		return adapters.ClassUnknown, err
	}
	classifier, ok := adapter.(adapters.StatementClassifier)
	if !ok {
		return adapters.ClassUnknown, nil
	}
	return classifier.ClassifyStatement(ctx, statement)
}

// ---- bridge.Canceller ----

// Cancel asks the in-process scheduler — the only place an op can be running now that P58f's
// Phase 4 deleted the Node engine child it used to fall back to.
func (r *Router) Cancel(ctx context.Context, opID string) (bool, error) {
	return r.host.CancelOp(ctx, opID)
}

// RunningOps lists started ops (see Host.RunningOps).
func (r *Router) RunningOps() []RunningOp { return r.host.RunningOps() }

// CancelRunning cancels a started op without early-cancel memory (see Host.CancelRunning).
func (r *Router) CancelRunning(ctx context.Context, opID string) (bool, error) {
	return r.host.CancelRunning(ctx, opID)
}
