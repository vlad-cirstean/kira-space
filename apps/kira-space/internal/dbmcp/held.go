package dbmcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
)

// heldCall names a call whose result is held for review, so every tool family shares one refusal
// template. Phrases are whole fragments: a family words its own nouns, the template owns the structure.
type heldCall struct {
	// noun is what the user withholds: "result" or "response".
	noun string
	// withheld and abandoned finish "the user withheld the <noun> ..." and "before the <noun> ... was reviewed".
	withheld, abandoned string
	// ran is the call as a sentence subject with its verb; bigRan is the same for the too-large text.
	ran, bigRan string
	// deniedRan and abandonedRan say that the call already happened.
	deniedRan, abandonedRan string
	// none is what the agent does not get: "no rows", "no response", "no data".
	none string
	// redo is the verb the agent must not repeat: "rerun" or "resend".
	redo string
	// waiting names the queue that is full: "queries", "requests", "reads".
	waiting string
	// narrow is the too-large hint for a read.
	narrow string
	// stays says what a write leaves behind: "changes" or "effects".
	stays string
	// read marks a call with no lasting effect: it gets no "stays" suffix.
	read bool
}

// suffix tells the agent a write's effects stay and it must not repeat the call, so it does not redo a
// committed write. A read gets neither.
func (c heldCall) suffix(msg string) string {
	if c.read {
		return msg
	}
	msg += fmt.Sprintf(" Its %s stay.", c.stays)
	if !strings.Contains(msg, "not "+c.redo) {
		msg += fmt.Sprintf(" Do not %s it.", c.redo)
	}
	return msg
}

// holdResult raises the response-stage item for held and waits for the answer. A non-empty string is the
// refusal text; "" means the user sent the result. recheck runs after an approval, since the review can
// take up to ApprovalTimeout: it returns why the result must still be withheld, "" when it may go.
//
// The text lives only in the broker entry and this goroutine: no log, op record, snapshot or notification
// carries it. Both end on every exit (answer, timeout, client cancel, server stop, quit). A result over
// the broker's per-result cap, or one that would push the total over its cap, is withheld unseen.
func (s *Server) holdResult(ctx context.Context, req ApprovalRequest, held ApprovalResponse, c heldCall, recheck func() string) string {
	req.Stage = ApprovalStageResponse
	req.Response = &held
	outcome := s.cfg.Approvals.Request(ctx, req)
	if outcome == ApprovalApproved {
		if recheck == nil {
			return ""
		}
		return recheck()
	}
	return heldRefusalText(outcome, c, held.Bytes)
}

// heldRefusalText is the app-authored text of a result the user did not send. It never carries data.
// The call has already happened, so every text says so.
func heldRefusalText(outcome ApprovalOutcome, c heldCall, size int) string {
	switch outcome {
	case ApprovalDenied:
		return c.suffix(fmt.Sprintf("the user withheld the %s %s; %s, but you get %s. Do not %s it to get the %s another way.", c.noun, c.withheld, c.deniedRan, c.none, c.redo, c.noun))
	case ApprovalTooLarge:
		msg := fmt.Sprintf("%s, but its %s is %.1f MB, over the %d MB review limit, so it was withheld unseen.", c.bigRan, c.noun, float64(size)/(1<<20), maxHeldResultBytes>>20)
		if c.read {
			return msg + " " + c.narrow
		}
		return c.suffix(msg)
	case ApprovalHeldFull:
		return c.suffix(fmt.Sprintf("%s, but too many results are waiting for the user's review, so its %s was withheld. Do not %s it.", c.ran, c.noun, c.redo))
	case ApprovalTimedOut:
		return c.suffix(fmt.Sprintf("%s, but its %s got no review within %s and was withheld.", c.ran, c.noun, approvalWindow()))
	case ApprovalAbandoned:
		return c.suffix(fmt.Sprintf("the Studio MCP server stopped before the %s %s was reviewed; %s and its %s was withheld.", c.noun, c.abandoned, c.abandonedRan, c.noun))
	case ApprovalQueueFull:
		return c.suffix(fmt.Sprintf("%s, but too many %s are waiting for approval, so its %s was withheld. Do not %s it.", c.ran, c.waiting, c.noun, c.redo))
	}
	return c.suffix(fmt.Sprintf("%s, but its %s was not approved.", c.ran, c.noun))
}

// dbHeldCall words a held run_query or browse_table result.
func dbHeldCall(rv queryResolution) heldCall {
	name := rv.summary.Name
	return heldCall{
		noun:     "result",
		withheld: fmt.Sprintf("of this query on %q", name), abandoned: fmt.Sprintf("of the query against %q", name),
		ran: fmt.Sprintf("query against %q ran", name), bigRan: fmt.Sprintf("the query on %q ran", name),
		deniedRan: "it ran", abandonedRan: "the query ran",
		none: "no rows", redo: "rerun", waiting: "queries",
		narrow: "Ask for fewer rows or columns.", stays: "changes",
		read: rv.class == adapters.ClassRead,
	}
}

// apiHeldCall words a held api_request response.
func apiHeldCall(p agentapi.Planned) heldCall {
	req := apiReq(p)
	return heldCall{
		noun:     "response",
		withheld: "to " + req, abandoned: "to " + req,
		ran: fmt.Sprintf("the %s request was sent", req), bigRan: fmt.Sprintf("the %s request was sent", req),
		deniedRan: "the request was sent", abandonedRan: "the request was sent",
		none: "no response", redo: "resend", waiting: "requests",
		narrow: "Ask for less: narrow the request or reduce the response with a transform script.", stays: "effects",
		read: apiReadClass(p),
	}
}

// kubeHeldCall words a held kube_list_resources, kube_describe or kube_pod_logs result. A cluster read
// changes nothing, so the call is always a read.
func kubeHeldCall(cluster, op string) heldCall {
	what := fmt.Sprintf("Kubernetes %s on %q", op, cluster)
	return heldCall{
		noun:     "result",
		withheld: "of this " + what, abandoned: "of the " + what,
		ran: "the " + what + " ran", bigRan: "the " + what + " ran",
		deniedRan: "it ran", abandonedRan: "the read ran",
		none: "no data", redo: "rerun", waiting: "reads",
		narrow: "Ask for fewer rows, a narrower selector or a shorter tail.", stays: "changes",
		read: true,
	}
}
