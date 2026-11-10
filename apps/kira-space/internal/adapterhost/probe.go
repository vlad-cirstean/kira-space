package adapterhost

import (
	"context"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const (
	// probeTimeout matches postgres's connect timeout.
	probeTimeout = 10 * time.Second
	// confirmTimeout bounds the primary check that follows a lane failure.
	confirmTimeout = 5 * time.Second
)

// ProbeError is a failed Router.Probe. Primary is true when the connection's main adapter itself
// failed, so its connected state is false.
type ProbeError struct {
	Primary bool
	Err     error
}

func (e *ProbeError) Error() string { return e.Err.Error() }
func (e *ProbeError) Unwrap() error { return e.Err }

// IsConnectionClass reports whether err says the server is unreachable or refused the session, as
// opposed to a statement-level failure.
func IsConnectionClass(err error) bool {
	code, ok := adapters.CodeOf(err)
	if !ok {
		return false
	}
	switch code {
	case adapters.CodeConnect, adapters.CodeEngineDown, adapters.CodeTimeout, adapters.CodeAuth:
		return true
	}
	return false
}

// Probe checks, right before an approval is raised, that the connection a statement will run on
// answers. lane selects the read lane for kinds that have one: opened here when absent, so the open
// is the probe. A lane failure that says the target is gone (a connect-class code, or not-found, as
// for a vanished SQLite file) is confirmed on the main adapter. Otherwise, and for writes, the main
// adapter is pinged. It never resolves secrets and writes no op-log row beyond the lane's own
// connect op. Kinds without a Pinger have no evidence beyond the connection state, so they pass.
// The error is a *ProbeError.
func (r *Router) Probe(ctx context.Context, connectionID string, lane bool) error {
	primary, err := requireLiveAdapter(connectionID)
	if err != nil {
		return &ProbeError{Primary: true, Err: err}
	}
	if cfg, ok := r.lanes.config(connectionID); lane && ok && laneKinds[cfg.Kind] {
		laneErr := r.probeLane(ctx, connectionID, primary, cfg)
		if laneErr == nil {
			return nil
		}
		if code, _ := adapters.CodeOf(laneErr); IsConnectionClass(laneErr) || code == adapters.CodeNotFound {
			if perr := r.probePrimary(ctx, connectionID, primary, confirmTimeout); perr != nil {
				return &ProbeError{Primary: true, Err: perr}
			}
		}
		return &ProbeError{Err: laneErr}
	}
	if err := r.probePrimary(ctx, connectionID, primary, probeTimeout); err != nil {
		return &ProbeError{Primary: true, Err: err}
	}
	return nil
}

// probePrimary pings the main adapter, twice at most: pg and mysql redial a socket the first ping
// found dead. A busy primary passes: the statement would queue behind the running op and a ping
// would wait on the pinned connection's lock.
func (r *Router) probePrimary(ctx context.Context, connectionID string, primary adapters.Adapter, timeout time.Duration) error {
	pinger, ok := primary.(adapters.Pinger)
	if !ok || r.host.busyOnPrimary(connectionID) {
		return nil
	}
	var err error
	for range 2 {
		pingCtx, cancel := context.WithTimeout(ctx, timeout)
		err = pinger.Ping(pingCtx)
		cancel()
		if err == nil || ctx.Err() != nil {
			return err
		}
	}
	return err
}

// probeLane opens the lane when absent (that open is the probe) or pings the idle one. A lane whose
// ping fails is torn down and reopened once, which heals a socket that died on a server restart.
func (r *Router) probeLane(ctx context.Context, connectionID string, primary adapters.Adapter, cfg model.ResolvedConnectionConfig) error {
	unlock := r.lanes.open.lock(connectionID)
	defer unlock()
	lane, stale := r.lanes.checkout(connectionID, primary)
	if stale != nil {
		r.teardownLane(ctx, connectionID, stale)
	}
	if lane == nil {
		return r.openProbeLane(ctx, connectionID, primary, cfg)
	}
	if r.lanes.busy(lane) {
		r.lanes.release(connectionID, lane, r.closeIdleLane)
		return nil
	}
	pinger, ok := lane.adapter.(adapters.Pinger)
	if !ok {
		r.lanes.release(connectionID, lane, r.closeIdleLane)
		return nil
	}
	pingCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	err := pinger.Ping(pingCtx)
	cancel()
	if err == nil || ctx.Err() != nil {
		r.lanes.release(connectionID, lane, r.closeIdleLane)
		return err
	}
	if r.lanes.discard(connectionID, lane) {
		r.teardownLane(ctx, connectionID, lane)
	}
	return r.openProbeLane(ctx, connectionID, primary, cfg)
}

// openProbeLane opens the lane for a probe and hands it back to the idle timer.
func (r *Router) openProbeLane(ctx context.Context, connectionID string, primary adapters.Adapter, cfg model.ResolvedConnectionConfig) error {
	openCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	lane, err := r.openLane(openCtx, connectionID, primary, cfg)
	if err != nil {
		return err
	}
	r.lanes.release(connectionID, lane, r.closeIdleLane)
	return nil
}
