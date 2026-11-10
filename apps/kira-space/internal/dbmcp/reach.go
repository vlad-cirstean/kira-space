package dbmcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// reachProber is the optional QueryRunner half that probes a connection (*adapterhost.Router).
type reachProber interface {
	Probe(ctx context.Context, connectionID string, lane bool) error
}

// lostMarker is the optional ConnectionsReader half that drops a connection found unreachable
// (*connections.Service).
type lostMarker interface {
	MarkLost(connectionID, detail string)
}

// ensureReachable probes the connection a statement will run on, right before an approval is
// raised: an unreachable connection returns its error and raises nothing. Read and masked
// statements run on the read lane, so the probe covers it; everything else runs on the main adapter.
// A failed main-adapter probe also drops the stale connected state so the next call reconnects.
func (s *Server) ensureReachable(ctx context.Context, summary model.ConnectionSummary, mode string) (res *mcp.CallToolResult, out any, err error, done bool) {
	prober, ok := s.cfg.Query.(reachProber)
	if !ok {
		return nil, nil, nil, false
	}
	perr := prober.Probe(ctx, summary.ID, mode == modeRead || mode == modeMasked)
	if perr == nil {
		return nil, nil, nil, false
	}
	msg := probeMessage(perr)
	var pe *adapterhost.ProbeError
	primary := errors.As(perr, &pe) && pe.Primary
	if primary && ctx.Err() == nil && adapterhost.IsConnectionClass(perr) {
		if marker, ok := s.cfg.Conns.(lostMarker); ok {
			marker.MarkLost(summary.ID, msg)
		}
	}
	if primary {
		res, out, err = errResult(fmt.Sprintf("connection %q is not reachable: %s. Nothing ran and no approval was asked.", summary.Name, msg))
	} else {
		res, out, err = errResult(fmt.Sprintf("connection %q could not open its read-only session: %s. Nothing ran and no approval was asked.", summary.Name, msg))
	}
	return res, out, err, true
}

// probeMessage is the adapter's own message when there is one.
func probeMessage(err error) string {
	var ae *adapters.Error
	if errors.As(err, &ae) {
		return ae.Message
	}
	return err.Error()
}
