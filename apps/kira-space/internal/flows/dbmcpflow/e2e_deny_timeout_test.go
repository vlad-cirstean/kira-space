package dbmcpflow

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/e2e-deny-timeout.spec.ts (contract e2e-deny-timeout): every card kind the
// MCP servers raise is denied once and left to time out once; either way nothing is sent or kept, no typed
// value is stored and the queue empties. The UI half sends the same denial from each card.

const (
	denyTimeoutKey = "e2e-deny-timeout"
	e12Stage       = `kira.storeValue("w", [1,2]); return 1;`
	e12Exempt      = "select body from notes where id in (select value from json_each(?)) /* big */"
)

// e12Kind is one card kind: start raises it and returns the waiting call with the card on the queue.
type e12Kind struct {
	key, name string
	start     func(t *testing.T) chan callOutcome
	// deny and expire are the pieces the agent's text holds for each outcome.
	deny, expire string
	// sent is how many requests reach the server even though the user refuses (a held API response).
	sent int
	// scripts is how many script rows the call leaves: a held result ran its script before the review.
	scripts int
}

func TestE2EDenyTimeout(t *testing.T) {
	f := newFixture(t, flowharness.WithApprovalTimeout(2*time.Second), flowharness.WithDbMcpLimits(fakeRowsLimit{}, fakeTimeLimit{}))
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	opts := f.e2eOpts(srv.URL)
	f.setReviewApp(t, true)
	f.mcpSet(t, map[string]string{"review.read": "review"})
	f.keep(t, externalScope, "ids", "[1,2]")

	req := apiArgs(envID, "POST", "{{base_url}}/echo", "X-Api-Key", "{{kept.apikey}}")
	req["body"], req["contentType"] = `{"ids": {{kept.ids}}}`, "application/json"
	req["inputs"] = []map[string]string{{"name": "apikey", "label": "API key"}}
	scripted := scriptArgs(envID, "GET", usersURL(srv.URL), map[string]any{"transform": e12Stage})
	raise := func(name string, args map[string]any) func(*testing.T) chan callOutcome {
		return func(t *testing.T) chan callOutcome {
			t.Helper()
			ch := callToolAsync(sess, name, args)
			f.items(t, 1)
			return ch
		}
	}
	kinds := []e12Kind{
		{key: "api-request", name: approvals.KindAPIRequest, start: raise("api_request", req),
			deny: "denied by the user", expire: "got no answer within"},
		{key: "api-response", name: approvals.KindAPIResponse, sent: 1, scripts: 1, deny: "the user withheld the response", expire: "got no review within",
			start: func(t *testing.T) chan callOutcome {
				t.Helper()
				ch := callToolAsync(sess, "api_request", scripted)
				f.answerItems(t, true, f.pending(t).Item.ID)
				testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
				return ch
			}},
		{key: "db-query", name: approvals.KindDbQuery, deny: "denied by the user", expire: "got no answer within",
			start: raise("run_query", f.queryScriptArgs("insert into notes (body) select 'x' from json_each(?)", "write", e12Stage, "ids"))},
		{key: "db-response", name: approvals.KindDbResponse, scripts: 1, deny: "the user withheld the result", expire: "got no review within",
			start: raise("run_query", f.queryScriptArgs(scriptStmt, "read", e12Stage))},
		{key: "db-exemption", name: approvals.KindDbExemption, deny: "the user kept limit expensive_query", expire: "got no answer within",
			start: func(t *testing.T) chan callOutcome {
				t.Helper()
				first := f.queryScriptArgs(e12Exempt, "read", e12Stage, "ids")
				id := exemptionID(t, callTool(t, sess, "run_query", first))
				again := f.queryScriptArgs(e12Exempt, "read", e12Stage, "ids")
				again["exemption"] = map[string]any{"id": id, "reason": "needs the full run"}
				ch := callToolAsync(sess, "run_query", again)
				f.items(t, 1)
				return ch
			}},
	}

	// outcome checks what a refused card left behind.
	outcome := func(t *testing.T, k e12Kind, res *mcp.CallToolResult, want string, sent int, notes []string) {
		t.Helper()
		if !res.IsError || !strings.Contains(text(res), want) {
			t.Fatalf("%s result = %q, want an error holding %q", k.key, text(res), want)
		}
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
		if got := len(srv.Requests()); got != sent+k.sent {
			t.Fatalf("%s: server saw %d requests, want %d", k.key, got, sent+k.sent)
		}
		if got := f.notesBodies(t); !slices.Equal(got, notes) {
			t.Fatalf("%s: notes = %q, want %q", k.key, got, notes)
		}
		if names := f.keptNames(externalScope); !slices.Equal(names, []string{"ids"}) {
			t.Fatalf("%s: kept = %v, want ids only: no staged write, no typed value", k.key, names)
		}
	}

	for _, k := range kinds {
		t.Run(k.key+" is denied", func(t *testing.T) {
			sent, notes, rows := len(srv.Requests()), f.notesBodies(t), f.scriptRows(t)
			ch := k.start(t)
			item := f.items(t, 1)[0]
			if item.Kind != k.name {
				t.Fatalf("item kind = %s, want %s", item.Kind, k.name)
			}
			ro := append(slices.Clip(opts), flowharness.Replace(f.keptID(t, externalScope, "ids"), "kv_ids"))
			f.app.Contract(t, denyTimeoutKey, "ApprovalsService.List#"+k.key, []approvals.Item{item}, ro...)
			if item.Kind == approvals.KindAPIResponse || item.Kind == approvals.KindDbResponse {
				held := f.pendingResponse(t, strings.TrimPrefix(item.ID, item.Kind+":"))
				held.Text, held.Raw = stable(held.Text), stable(held.Raw)
				f.app.Contract(t, denyTimeoutKey, "DbMcpService.PendingResponse#"+k.key, held, ro...)
			}
			f.app.Contract(t, denyTimeoutKey, "args:ApprovalsService.Answer#deny-"+k.key, approvals.AnswerArgs{IDs: []string{item.ID}, Approve: false})
			f.answerItems(t, false, item.ID)
			outcome(t, k, wait(t, ch), k.deny, sent, notes)
			if got := f.scriptRows(t); got != rows+k.scripts {
				t.Fatalf("%s: %d script rows, want %d", k.key, got-rows, k.scripts)
			}
		})
		t.Run(k.key+" times out", func(t *testing.T) {
			sent, notes, rows := len(srv.Requests()), f.notesBodies(t), f.scriptRows(t)
			ch := k.start(t)
			outcome(t, k, wait(t, ch), k.expire, sent, notes)
			if got := f.scriptRows(t); got != rows+k.scripts {
				t.Fatalf("%s: %d script rows, want %d", k.key, got-rows, k.scripts)
			}
		})
	}
}

// scriptRows counts the finished script op rows of both stages.
func (f *fixture) scriptRows(t *testing.T) int {
	t.Helper()
	n := 0
	for _, r := range f.opsRecent(t) {
		if r.Kind == "jsPrepare" || r.Kind == "jsTransform" {
			n++
		}
	}
	return n
}
