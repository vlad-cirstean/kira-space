package dbmcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/jsengine"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	apiReviewBySetting   = "setting"
	apiReviewByRequested = "requested"
)

// apiReview is the review decision for one request.
type apiReview struct {
	On bool
	// Reason is why the response is held: "setting" or "requested".
	Reason string
}

// apiReadClass is the class a review setting applies to: a GET, HEAD or OPTIONS without a body reads,
// anything else writes. Same rule as agentAutoApproveReads; a GET with a body is a write.
func apiReadClass(p agentapi.Planned) bool { return isReadMethod(p.Method) && !p.HasBody() }

// apiReviewSetting is the environment's value for the class; inherit (or no environment) reads the app leaf.
func (s *Server) apiReviewSetting(env agentapi.EnvInfo, read bool) bool {
	v := env.Review.Write
	if read {
		v = env.Review.Read
	}
	switch v {
	case "review":
		return true
	case "send":
		return false
	}
	return s.apiSettings().ReviewResponses
}

// apiReviewFor is the one place that decides whether a response is held; setting wins over requested for
// the reason.
func (s *Server) apiReviewFor(plan agentapi.Planned, sc *apiScripts, requested bool) apiReview {
	switch {
	case s.apiReviewSetting(plan.Env, apiReadClass(plan)):
		return apiReview{On: true, Reason: apiReviewBySetting}
	case requested:
		return apiReview{On: true, Reason: apiReviewByRequested}
	}
	return apiReview{}
}

// apiReviewMeta is what the review item shows beside the held response; never the response data.
type apiReviewMeta struct {
	Status     int
	StatusText string
	Read       bool
	Reason     string
}

// reviewAPI holds the response of a request that was already sent until the user sends or withholds it.
// A non-empty string is the refusal text; "" means the user sent it and res goes out untouched.
//
// The text lives only in the broker entry and this goroutine: no log, op record, snapshot or
// notification carries it. Both end on every exit (answer, timeout, client cancel, server stop, quit).
func (s *Server) reviewAPI(ctx context.Context, plan agentapi.Planned, sc *apiScripts, o apiOutcome, rv apiReview, res *mcp.CallToolResult, err error) string {
	read := apiReadClass(plan)
	// go-sdk toolForErr: a bare handler error reaches the client as an IsError text result carrying err.Error().
	held := ApprovalResponse{IsError: true, Requested: rv.Reason == apiReviewByRequested}
	if res != nil {
		held.IsError = res.IsError
		held.Text = resultText(res)
	} else if err != nil {
		held.Text = err.Error()
	}
	held.Bytes = len(held.Text)
	if sc != nil && sc.kept != nil && o.Failure == "" {
		held.Keep = sc.kept.stagedViews()
		held.staged = sc.kept
	}
	if sc.hasTransform() {
		raw, merr := json.Marshal(o.Raw)
		if merr != nil {
			return apiHeldCall(plan).suffix(fmt.Sprintf("the %s request was sent, but its response could not be prepared for review and was withheld.", apiReq(plan)))
		}
		held.Raw = string(raw)
		held.Script = &HeldScript{Source: sc.Transform, Hash: jsengine.SourceHash(sc.Transform)}
		held.Bytes += len(held.Raw) + len(held.Script.Source)
	}
	return s.holdResult(ctx, ApprovalRequest{
		ConnectionName: plan.Host, Kind: "http", Class: adapters.ClassUnknown, Statement: plan.Method + " " + plan.URL,
		Reason: ApprovalReasonPermission, Origin: originFrom(ctx), Subject: approvals.KindAPIResponse,
		API: &plan, APIReview: &apiReviewMeta{Status: o.Raw.Status, StatusText: o.Raw.StatusText, Read: read, Reason: rv.Reason},
	}, held, apiHeldCall(plan), func() string { return s.recheckAPIHeld(plan, sc) })
}

// recheckAPIHeld returns why an approved response must still be withheld, "" when it may go. The held text
// was masked by the request's own masker at send time, and no user setting changes what that masker covers,
// so only the tools switch and, for a scripted request, the scripts switch are re-read.
func (s *Server) recheckAPIHeld(plan agentapi.Planned, sc *apiScripts) string {
	switch {
	case !s.apiEnabled():
		return apiHeldCall(plan).suffix(fmt.Sprintf("the %s request was sent, but %s while its response was in review, so it was withheld.", apiReq(plan), apiRequestsOff))
	case sc != nil && !s.scriptsOn():
		return apiHeldCall(plan).suffix(fmt.Sprintf("the %s request was sent, but %s while its response was in review, so it was withheld.", apiReq(plan), apiScriptsOff))
	}
	return ""
}

func apiReq(p agentapi.Planned) string { return p.Method + " " + p.Host }
