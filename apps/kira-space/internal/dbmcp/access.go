package dbmcp

import (
	"context"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// resolveEnabled looks up connectionID among every connection this app knows and returns it only
// if McpEnabled is true — deny by default.
// A connection that exists but is not exposed
// reports the exact same "no such connection" text an unknown id would: its existence is not an
// AI client's business (the rule for list_connections, applied here to a client that names
// one directly instead of discovering it).
func (s *Server) resolveEnabled(connectionID string) (model.ConnectionSummary, error) {
	conns, err := s.cfg.Conns.List()
	if err != nil {
		return model.ConnectionSummary{}, err
	}
	for _, c := range conns {
		if c.ID != connectionID {
			continue
		}
		if !c.McpEnabled {
			break
		}
		return c, nil
	}
	return model.ConnectionSummary{}, fmt.Errorf("no connection %q exposed to MCP", connectionID)
}

// capsOf extracts the live adapter's own Caps from a connection's current state. Caps are only
// knowable once connected — adapterhost.Router.Connect stores adapter.Caps() on ConnectResult,
// carried through to ConnectionState.Caps — so a never-connected or currently-disconnected
// connection reports ok=false rather than a guess.
func capsOf(state model.ConnectionState) (adapters.Caps, bool) {
	if state.Status != "connected" {
		return adapters.Caps{}, false
	}
	caps, ok := state.Caps.(adapters.Caps)
	return caps, ok
}

// connectForQuery connects connectionID on demand — run_query's own rule. Exposing a
// connection to MCP (resolveEnabled's own gate) is the human's explicit, per-connection consent;
// Connect is the same deduplicated path the UI uses, so the connection visibly comes up in the app
// rather than opening invisibly. Already-connected is left alone: Connect's own semantics tear a
// live connection down and rebuild it unconditionally, so calling it on every run_query would churn
// the adapter, drop its cached metadata and re-run the pre-connect script on every already-open
// connection. A failure is returned as-is, not retried — its own error state text is what the
// caller sees (connectStateError below).
func (s *Server) connectForQuery(ctx context.Context, connectionID string) (model.ConnectionState, error) {
	if state := s.cfg.Conns.StateOf(connectionID); state.Status == "connected" {
		return state, nil
	}
	return s.cfg.Conns.ConnectContext(ctx, connectionID)
}

// connectStateError formats a non-connected ConnectionState as run_query's connect-failure text:
// doConnect encodes a failed connect as a state with err == nil, so the caller must read
// the state's own Status/Error rather than falling through to Execute and getting a generic
// dispatcher error instead of the connection's real failure reason.
func connectStateError(state model.ConnectionState) string {
	if state.Error != nil && *state.Error != "" {
		return fmt.Sprintf("connect failed (%s): %s", state.Status, *state.Error)
	}
	return fmt.Sprintf("connect failed: %s", state.Status)
}
