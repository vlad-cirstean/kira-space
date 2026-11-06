package dbmcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/queryplan"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxExemptionReasonRunes = 300

// claimedExemption is an exemption id the agent presented and the store accepted: the limits the user
// is asked to lift for this one call and the agent's reason.
type claimedExemption struct {
	hits   []LimitHit
	reason string
}

// covers reports whether the user is asked to lift limit name for this call. Nil-safe: no exemption
// covers nothing.
func (e *claimedExemption) covers(name string) bool {
	return e != nil && hasLimit(e.hits, name)
}

// claimExemption validates the exemption field and claims its id. The id is spent on first
// presentation whatever the outcome, so a refused retry cannot be replayed or used to probe. It runs
// after resolveVerdict: a gate that refuses never reaches it and leaves the id unclaimed.
func (s *Server) claimExemption(args runQueryArgs, rv queryResolution) (ex *claimedExemption, res *mcp.CallToolResult, out any, err error, done bool) {
	if args.Exemption == nil {
		return nil, nil, nil, nil, false
	}
	refuse := func(text string) (*claimedExemption, *mcp.CallToolResult, any, error, bool) {
		res, out, err := errResult(text)
		return nil, res, out, err, true
	}
	id := args.Exemption.ID
	if !ids.Is(ids.Exemption, id) {
		return refuse("exemption.id is not an id from a limit refusal")
	}
	reason := strings.Join(strings.Fields(args.Exemption.Reason), " ")
	if n := utf8.RuneCountInString(reason); n == 0 || n > maxExemptionReasonRunes {
		return refuse(fmt.Sprintf("exemption.reason needs one sentence, at most %d characters", maxExemptionReasonRunes))
	}
	rec, ok := s.exemptions.claim(id)
	if !ok {
		return refuse(fmt.Sprintf("exemption %s is unknown, used or expired. Run the statement without exemption; a limit refusal gives a new id.", id))
	}
	if rec.connectionID != args.ConnectionID || rec.mode != rv.mode || rec.path != args.Path || rec.sql != args.SQL {
		if args.Mode == modeMasked {
			return refuse(fmt.Sprintf("exemption %s was issued for a different call. Repeat the refused call exactly: same connectionId, path, columns, filters, order, limit and offset.", id))
		}
		return refuse(fmt.Sprintf("exemption %s was issued for a different call. Repeat the refused call exactly: same connectionId, sql, path and mode.", id))
	}
	return &claimedExemption{hits: rec.hits, reason: reason}, nil, nil, nil, false
}

func (s *Server) limitCall(args runQueryArgs, rv queryResolution, exempt bool) LimitCall {
	return LimitCall{
		Summary: rv.summary, Mode: rv.mode, Class: rv.class, SQL: args.SQL, Path: args.Path, Exempt: exempt,
		Plan: rv.plan, PlanErr: rv.planErr, Threshold: rv.threshold,
	}
}

// checkLimits runs every pre-limit the exemption does not cover. A hit refuses before any approval is
// spent.
func (s *Server) checkLimits(ctx context.Context, args runQueryArgs, rv queryResolution, ex *claimedExemption) (res *mcp.CallToolResult, out any, err error, done bool) {
	var hits []LimitHit
	for _, l := range s.limits {
		pl, ok := l.(PreLimit)
		if !ok || ex.covers(l.Name()) {
			continue
		}
		hit, cerr := pl.Check(ctx, s.limitCall(args, rv, false))
		if cerr != nil {
			res, out, err = s.limitFailure(args.ConnectionID, cerr)
			return res, out, err, true
		}
		if hit != nil {
			hits = append(hits, *hit)
		}
	}
	if len(hits) == 0 {
		return nil, nil, nil, false
	}
	res, out, err = s.refuseByLimits(rv.summary, ex, hits, args)
	return res, out, err, true
}

// limitFailure fails the call for a limit's own error, withholding the text on a masked connection like
// any driver error.
func (s *Server) limitFailure(connectionID string, cerr error) (*mcp.CallToolResult, any, error) {
	var pce *planCheckError
	if errors.As(cerr, &pce) {
		return errResult(pce.Error())
	}
	if s.maskedConnection(connectionID) {
		return maskedToolError(cerr)
	}
	return toolError(cerr)
}

// execHits asks each exec limit the exemption does not cover whether err is its trip.
func (s *Server) execHits(args runQueryArgs, rv queryResolution, ex *claimedExemption, err error, elapsed time.Duration) []LimitHit {
	var hits []LimitHit
	for _, l := range s.limits {
		el, ok := l.(ExecLimit)
		if !ok || ex.covers(l.Name()) {
			continue
		}
		if hit := el.Hit(s.limitCall(args, rv, false), err, elapsed); hit != nil {
			hits = append(hits, *hit)
		}
	}
	return hits
}

// refuseByLimits mints a fresh id covering the claimed exemption's limits plus the new hits, so the
// next dialog lists everything the statement needs. The refusal is app-authored text with no data,
// so callers return it without a result review.
func (s *Server) refuseByLimits(summary model.ConnectionSummary, ex *claimedExemption, hits []LimitHit, args runQueryArgs) (*mcp.CallToolResult, any, error) {
	if ex != nil {
		hits = mergeHits(ex.hits, hits)
	}
	id := s.exemptions.mint(exemptionRecord{
		connectionID: args.ConnectionID, mode: args.Mode, path: args.Path, sql: args.SQL, hits: hits,
	})
	return errResult((&LimitError{ConnectionName: summary.Name, Hits: hits, ExemptionID: id, Browse: args.Mode == modeMasked}).Error())
}

// awaitExemption raises the one db.exemption approval of an exempted call. It never reads the mode's
// verdict or any review setting: an exempted call always asks.
func (s *Server) awaitExemption(ctx context.Context, args runQueryArgs, rv *queryResolution, ex *claimedExemption, plan *queryplan.Plan) (res *mcp.CallToolResult, out any, err error, done bool) {
	outcome, choice := s.cfg.Approvals.RequestChoice(ctx, ApprovalRequest{
		Subject: approvals.KindDbExemption, ConnectionID: args.ConnectionID, ConnectionName: rv.summary.Name, Kind: rv.summary.Kind,
		Class: rv.class, Mode: rv.mode, Enforcement: enforcementFor(rv.summary, rv.mode),
		Statement: args.SQL, Reason: ApprovalReasonExemption,
		Target:    targetOf(rv.summary, args.Path),
		Plan:      approvalPlanFrom(plan, rv.threshold),
		Exemption: &ApprovalExemption{Reason: ex.reason, Hits: ex.hits}, Origin: originFrom(ctx),
		OfferReview: true, ReviewOn: s.reviewFor(rv.summary, rv.mode, rv.class),
	})
	switch outcome {
	case ApprovalApproved:
		rv.reviewRequested = choice.ReviewResult
		return s.recheckAfterApproval(ctx, args, rv)
	case ApprovalDenied:
		names := make([]string, len(ex.hits))
		for i, h := range ex.hits {
			names[i] = h.Limit
		}
		res, out, err = errResult(fmt.Sprintf("the user kept limit %s for this statement on %q; it did not run. Improve the statement or stop.", strings.Join(names, ", "), rv.summary.Name))
		return res, out, err, true
	default:
		res, out, err = approvalOutcomeError(outcome, fmt.Sprintf("exemption request for query against %q", rv.summary.Name))
		return res, out, err, true
	}
}
