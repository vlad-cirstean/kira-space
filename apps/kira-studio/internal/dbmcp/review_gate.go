package dbmcp

import (
	"context"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// reviewRefusal is the refusal when column review cannot vouch for the connection's columns.
func reviewRefusal(summary model.ConnectionSummary, reason string) *mcp.CallToolResult {
	res, _, _ := errResult(fmt.Sprintf(
		"connection %q has column review on, and Kira could not check its columns (%s). Ask the user to narrow the table allow list or turn column review off.",
		summary.Name, reason))
	return res
}

// maskStateFor is the masking a call applies: the connection's rules plus the columns withheld until reviewed
// (P534). A non-nil result refuses the call because the columns could not be checked; err is an internal fault.
func (s *Server) maskStateFor(ctx context.Context, summary model.ConnectionSummary) (maskset, *mcp.CallToolResult, error) {
	rules, err := s.cfg.MaskRules.MaskSetFor(summary.ID)
	if err != nil {
		return maskset{}, nil, err
	}
	w := s.cfg.ColumnReview.For(ctx, summary)
	if w.Incomplete != "" {
		return maskset{Set: rules}, reviewRefusal(summary, w.Incomplete), nil
	}
	return maskset{Set: rules, withheld: w.Names}, nil, nil
}

// checkColumnReview refuses a call on a connection whose columns cannot be checked. It is the last
// step of resolveVerdict and is not a P528 limit: no exemption applies.
func (s *Server) checkColumnReview(ctx context.Context, summary model.ConnectionSummary) (*mcp.CallToolResult, any, error) {
	if _, refusal, err := s.maskStateFor(ctx, summary); refusal != nil || err != nil {
		return refusal, nil, err
	}
	return nil, nil, nil
}

// maskedConnection reports whether driver errors of the connection stay value-free: it has mask rules or
// withheld columns. A read failure counts as masked. No op: withheld columns come from the cached state.
func (s *Server) maskedConnection(connectionID string) bool {
	set, err := s.cfg.MaskRules.MaskSetFor(connectionID)
	if err != nil || !set.Empty() {
		return true
	}
	w, ok := s.cfg.ColumnReview.Cached(connectionID)
	return ok && len(w.Names) > 0
}
