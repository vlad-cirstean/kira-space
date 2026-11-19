package dbmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/jsengine"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	apiReviewBySetting   = "setting"
	apiReviewByRequested = "requested"
	// apiReviewByUserValue: a script read a value the user supplied; a transform could hide it from the mask.
	apiReviewByUserValue = "userValue"
)

// apiReview is the review decision for one request.
type apiReview struct {
	On bool
	// Reason is why the response is held: "setting", "requested" or "userValue".
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

// apiReviewFor is the one place that decides whether a response is held. A script reading a user-supplied
// value forces it whatever the settings say; setting wins over requested for the reason.
func (s *Server) apiReviewFor(plan agentapi.Planned, sc *apiScripts, requested bool) apiReview {
	switch {
	case sc != nil && sc.kept.readsUserValue():
		return apiReview{On: true, Reason: apiReviewByUserValue}
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
	if sc.hasTransform() && o.Failure == "" {
		held.Keep = sc.kept.stagedViews()
	}
	if sc.hasTransform() {
		raw, merr := json.Marshal(o.Raw)
		if merr != nil {
			return apiSentSuffix(fmt.Sprintf("the %s request was sent, but its response could not be prepared for review and was withheld.", apiReq(plan)), read)
		}
		held.Raw = string(raw)
		held.Script = &HeldScript{Source: sc.Transform, Hash: jsengine.SourceHash(sc.Transform)}
		held.Bytes += len(held.Raw) + len(held.Script.Source)
	}
	outcome := s.cfg.Approvals.Request(ctx, ApprovalRequest{
		ConnectionName: plan.Host, Kind: "http", Class: adapters.ClassUnknown, Statement: plan.Method + " " + plan.URL,
		Reason: ApprovalReasonPermission, Origin: originFrom(ctx), Subject: approvals.KindAPIResponse,
		Stage: ApprovalStageResponse, API: &plan, Response: &held,
		APIReview: &apiReviewMeta{Status: o.Raw.Status, StatusText: o.Raw.StatusText, Read: read, Reason: rv.Reason},
	})
	if outcome == ApprovalApproved {
		return s.recheckAPIHeld(plan, sc, read)
	}
	return apiHeldRefusal(outcome, plan, read, held.Bytes)
}

// recheckAPIHeld returns why an approved response must still be withheld, "" when it may go. The held text
// was masked by the request's own masker at send time, and no user setting changes what that masker covers,
// so only the tools switch and, for a scripted request, the scripts switch are re-read.
func (s *Server) recheckAPIHeld(plan agentapi.Planned, sc *apiScripts, read bool) string {
	switch {
	case !s.apiEnabled():
		return apiSentSuffix(fmt.Sprintf("the %s request was sent, but %s while its response was in review, so it was withheld.", apiReq(plan), apiRequestsOff), read)
	case sc != nil && !s.scriptsOn():
		return apiSentSuffix(fmt.Sprintf("the %s request was sent, but %s while its response was in review, so it was withheld.", apiReq(plan), apiScriptsOff), read)
	}
	return ""
}

func apiReq(p agentapi.Planned) string { return p.Method + " " + p.Host }

// apiHeldRefusal is the app-authored text of a response the user did not send. It never carries response
// data, not even the status.
func apiHeldRefusal(outcome ApprovalOutcome, plan agentapi.Planned, read bool, size int) string {
	req := apiReq(plan)
	switch outcome {
	case ApprovalDenied:
		return apiSentSuffix(fmt.Sprintf("the user withheld the response to %s; the request was sent, but you get no response. Do not resend it to get the response another way.", req), read)
	case ApprovalTooLarge:
		msg := fmt.Sprintf("the %s request was sent, but its response is %.1f MB, over the %d MB review limit, so it was withheld unseen.", req, float64(size)/(1<<20), maxHeldResultBytes>>20)
		if read {
			return msg + " Ask for less: narrow the request or reduce the response with a transform script."
		}
		return apiSentSuffix(msg, read)
	case ApprovalHeldFull:
		return apiSentSuffix(fmt.Sprintf("the %s request was sent, but too many results are waiting for the user's review, so its response was withheld. Do not resend it.", req), read)
	case ApprovalTimedOut:
		return apiSentSuffix(fmt.Sprintf("the %s request was sent, but its response got no review within %s and was withheld.", req, approvalWindow()), read)
	case ApprovalAbandoned:
		return apiSentSuffix(fmt.Sprintf("the Studio MCP server stopped before the response to %s was reviewed; the request was sent and its response was withheld.", req), read)
	case ApprovalQueueFull:
		return apiSentSuffix(fmt.Sprintf("the %s request was sent, but too many requests are waiting for approval, so its response was withheld. Do not resend it.", req), read)
	}
	return apiSentSuffix(fmt.Sprintf("the %s request was sent, but its response was not approved.", req), read)
}

// apiSentSuffix tells the agent a write's effects stay and it must not resend, so it does not repeat a
// committed write. A read gets neither.
func apiSentSuffix(msg string, read bool) string {
	if read {
		return msg
	}
	msg += " Its effects stay."
	if !strings.Contains(msg, "not resend") {
		msg += " Do not resend it."
	}
	return msg
}
