package dbmcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
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
	return maskset{Set: rules, withheld: w.Names, known: w.Known}, nil, nil
}

// unknownResultColumn reports whether a result column name is outside the last column check: an alias, a
// computed column, or a column added since that check.
func unknownResultColumn(known map[string]bool, columns []page.ColumnDescriptor) bool {
	for _, c := range columns {
		if !known[strings.ToLower(c.Name)] {
			return true
		}
	}
	return false
}

// settleResult makes the withheld set cover every column of a result. A name the last check never saw
// forces one refresh with columns:true; the name is withheld only if that refresh finds it pending, so
// aliases and computed columns still pass. At most one refresh per call. A non-nil result refuses the
// call because the refresh could not complete.
func (s *Server) settleResult(ctx context.Context, summary model.ConnectionSummary, set maskset, columns []page.ColumnDescriptor) (maskset, *mcp.CallToolResult) {
	if set.known == nil || !unknownResultColumn(set.known, columns) {
		return set, nil
	}
	w := s.cfg.ColumnReview.Recheck(ctx, summary)
	if w.Incomplete != "" {
		return set, reviewRefusal(summary, w.Incomplete)
	}
	set.withheld, set.known = w.Names, w.Known
	return set, nil
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
