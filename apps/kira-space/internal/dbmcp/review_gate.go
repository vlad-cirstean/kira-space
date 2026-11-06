package dbmcp

import (
	"context"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
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

// stmtTables is the relations a statement reads; parsed is false when they could not be named.
type stmtTables struct {
	refs   []tableref.Ref
	parsed bool
}

// statementRefs names the statement's relations for settleResult. A statement the engine cannot parse yields
// parsed=false.
func (s *Server) statementRefs(ctx context.Context, summary model.ConnectionSummary, path, sql string) *stmtTables {
	tabler, ok := s.cfg.Query.(statementTabler)
	if !ok {
		return &stmtTables{}
	}
	tr, err := tabler.StatementTables(ctx, summary.ID, path, sql)
	if err != nil || tr.Refusal != "" {
		return &stmtTables{}
	}
	return &stmtTables{refs: tr.Refs, parsed: true}
}

// ranReviewRefusal is reviewRefusal for a statement that already ran.
func ranReviewRefusal(rv queryResolution, reason string) *mcp.CallToolResult {
	res, _, _ := errResult(ranSuffix(fmt.Sprintf(
		"query against %q ran, but Kira could not check its columns (%s), so its result was withheld. Ask the user to narrow the table allow list or turn column review off.",
		rv.summary.Name, reason), rv))
	return res
}

// settleResult makes the withheld set cover every column of a result. A result column that is no column of
// the statement's relations (an alias, a computed column, or a column added since the last check) forces one
// refresh of those relations' containers; the name is withheld only if that refresh finds it pending, so
// aliases and computed columns still pass. At most one refresh per call. A non-nil result refuses the call
// because the refresh could not complete; the statement has already run.
func (s *Server) settleResult(ctx context.Context, rv queryResolution, args runQueryArgs, set maskset, columns []page.ColumnDescriptor) (maskset, *mcp.CallToolResult) {
	if set.known == nil {
		return set, nil
	}
	if set.stmt == nil {
		set.stmt = s.statementRefs(ctx, rv.summary, args.Path, args.SQL)
	}
	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = c.Name
	}
	unsettled := set.known.Unsettled(set.stmt.refs, set.stmt.parsed, names)
	if len(unsettled) == 0 {
		return set, nil
	}
	w := s.cfg.ColumnReview.Recheck(ctx, rv.summary, maskreview.Rescan{Tables: set.stmt.refs, Parsed: set.stmt.parsed, Names: unsettled})
	if w.Incomplete != "" {
		return set, ranReviewRefusal(rv, w.Incomplete)
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
