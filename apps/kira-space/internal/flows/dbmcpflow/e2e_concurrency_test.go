package dbmcpflow

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/e2e-concurrency.spec.ts (contract e2e-concurrency): cards raised by three
// clients queue in the order they arrive and each answer reaches only its own caller. The UI half shows the
// same five cards one at a time and sends the same Deny all. The pressure steps (script slots, a commit race,
// a mixed Answer) have no UI surface and are backend only. Run with -race.

const (
	concurrencyKey = "e2e-concurrency"
	e13Insert      = "insert into notes (body) select 'm2' from json_each(?)"
)

// e13Rig is the three clients and the second connection every step shares.
type e13Rig struct {
	f          *fixture
	a, b, s    *mcp.ClientSession
	envID      string
	echo       string
	promptConn string
}

// raiseFive starts one call of each kind and waits until all five cards are queued, oldest first: an API card
// that reads a kept list (A), a write that binds it (B), a held scripted result (A), an API card that asks for a
// typed value (S) and a scripted read on a prompting connection (S).
func (r e13Rig) raiseFive(t *testing.T) ([]chan callOutcome, []approvals.Item) {
	t.Helper()
	f := r.f
	f.app.W.Kept.ClearAll()
	f.keep(t, externalScope, "ids", "[1,2]")
	input := apiArgs(r.envID, "GET", "{{base_url}}/echo?m=m4&k={{secret.apikey}}")
	input["inputs"] = []string{"apikey"}
	prompted := withMode(r.promptConn, scriptStmt, "read")
	prompted["script"] = `return "m5";`
	calls := []chan callOutcome{callToolAsync(r.a, "api_request", jsonPost(r.envID, r.echo, `{"m": "m1", "ids": {{plain.ids}}}`))}
	f.items(t, 1)
	calls = append(calls, callToolAsync(r.b, "run_query", f.paramsArgs(e13Insert, "write", "ids")))
	f.items(t, 2)
	calls = append(calls, callToolAsync(r.a, "run_query", f.queryScriptArgs(scriptStmt, "read", `return "m3";`)))
	f.items(t, 3)
	calls = append(calls, callToolAsync(r.s, "api_request", input))
	f.items(t, 4)
	calls = append(calls, callToolAsync(r.s, "run_query", prompted))
	items := f.items(t, 5)
	want := []string{approvals.KindAPIRequest, approvals.KindDbQuery, approvals.KindDbResponse, approvals.KindAPIRequest, approvals.KindDbQuery}
	var got []string
	for _, it := range items {
		got = append(got, it.Kind)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("queue kinds = %v, want enqueue order %v", got, want)
	}
	return calls, items
}

func TestE2EConcurrency(t *testing.T) {
	captureLog(t)
	f := newFixture(t, flowharness.WithDbMcpLimits(fakeRowsLimit{}, fakeTimeLimit{}))
	srv, a, envID := f.apiSetup(t, "127.0.0.1", false)
	s, _ := f.studioSession(t, "e13-s")
	r := e13Rig{f: f, a: a, b: f.externalSession(t), s: s, envID: envID, echo: srv.URL + "/echo",
		promptConn: f.addConn(t, "prompt db", "sqlite", "prompt", "prompt", "deny", false)}
	opts := f.e2eOpts(srv.URL)
	f.mcpSet(t, map[string]string{"review.read": "review"})
	idle := func(t *testing.T) {
		t.Helper()
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
	}
	var agent []string
	want := func(t *testing.T, ch chan callOutcome, parts ...string) {
		t.Helper()
		res := wait(t, ch)
		agent = append(agent, text(res))
		if res.IsError {
			t.Fatalf("result = %q, want %v", text(res), parts)
		}
		for _, p := range parts {
			if !strings.Contains(text(res), p) {
				t.Fatalf("result = %q, want it to hold its own marker %q", text(res), p)
			}
		}
	}

	t.Run("five cards queue in order and are answered in reverse", func(t *testing.T) {
		calls, items := r.raiseFive(t)
		ro := append(slices.Clip(opts), flowharness.Replace(f.keptID(t, externalScope, "ids"), "kv_ids"))
		f.app.Contract(t, concurrencyKey, "ApprovalsService.List#five", items, ro...)
		held := f.pendingResponse(t, strings.TrimPrefix(items[2].ID, items[2].Kind+":"))
		f.app.Contract(t, concurrencyKey, "DbMcpService.PendingResponse#five", held, ro...)
		f.answerItems(t, true, items[4].ID)
		want(t, calls[4], `"output":"m5"`)
		if res, err := f.app.W.ApprovalsSvc.Answer(inputAnswer(items[3].ID, e2eTyped)); err != nil || res[0].Error != "" {
			t.Fatalf("typed answer = %+v (%v)", res, err)
		}
		want(t, calls[3], "m4")
		f.answerItems(t, true, items[2].ID)
		want(t, calls[2], `"output":"m3"`)
		f.answerItems(t, true, items[1].ID)
		if res := wait(t, calls[1]); res.IsError {
			t.Fatalf("write = %q", text(res))
		}
		f.answerItems(t, true, items[0].ID)
		want(t, calls[0], "m1")
		if got := f.notesBodies(t); !slices.Equal(got, []string{"one", "two", "m2", "m2"}) {
			t.Fatalf("notes = %q, want the write of B alone", got)
		}
		idle(t)
		f.noLeak(t, e2eTyped, agent...)
	})

	t.Run("Deny all reaches every caller and runs nothing", func(t *testing.T) {
		notes, sent := f.notesBodies(t), len(srv.Requests())
		calls, items := r.raiseFive(t)
		ids := make([]string, len(items))
		for i, it := range items {
			ids[i] = it.ID
		}
		ro := append(slices.Clip(opts), flowharness.Replace(f.keptID(t, externalScope, "ids"), "kv_ids"))
		f.app.Contract(t, concurrencyKey, "ApprovalsService.List#all", items, ro...)
		held := f.pendingResponse(t, strings.TrimPrefix(items[2].ID, items[2].Kind+":"))
		f.app.Contract(t, concurrencyKey, "DbMcpService.PendingResponse#all", held, ro...)
		f.app.Contract(t, concurrencyKey, "args:ApprovalsService.Answer#all", approvals.AnswerArgs{IDs: ids, Approve: false})
		f.answerItems(t, false, ids...)
		for i, c := range calls {
			res := wait(t, c)
			wantText := "denied by the user"
			if i == 2 {
				wantText = "the user withheld the result"
			}
			if !res.IsError || !strings.Contains(text(res), wantText) {
				t.Fatalf("call %d = %q, want %q", i+1, text(res), wantText)
			}
		}
		idle(t)
		if got := f.notesBodies(t); !slices.Equal(got, notes) {
			t.Fatalf("notes = %q, want %q", got, notes)
		}
		if got := len(srv.Requests()); got != sent {
			t.Fatalf("server saw %d requests, want %d", got, sent)
		}
	})

	t.Run("two commits to one name: the last delivered wins and a pinned card is withdrawn", func(t *testing.T) {
		f.app.W.Kept.ClearAll()
		f.keep(t, externalScope, "ids", "[9]")
		notes := f.notesBodies(t)
		first := callToolAsync(r.a, "run_query", f.queryScriptArgs("select id, body from notes order by id", "read", `plain.ids = [1]; return 1;`))
		f.items(t, 1)
		second := callToolAsync(r.a, "run_query", f.queryScriptArgs("select id, body from notes order by id desc", "read", `plain.ids = [2]; return 2;`))
		f.items(t, 2)
		pinned := callToolAsync(r.b, "run_query", f.paramsArgs(e13Insert, "write", "ids"))
		items := f.items(t, 3)
		if items[0].Kind != approvals.KindDbResponse || items[1].Kind != approvals.KindDbResponse || items[2].Kind != approvals.KindDbQuery {
			t.Fatalf("kinds = %s %s %s, want two held results and the pinned write", items[0].Kind, items[1].Kind, items[2].Kind)
		}
		f.answerItems(t, true, items[1].ID)
		want(t, second, `"tempSet":[{"name":"ids"`)
		out := wait(t, pinned)
		if !out.IsError || !strings.HasPrefix(text(out), `temporary variable "ids" was replaced while waiting for approval; nothing ran.`) {
			t.Fatalf("pinned = %q, want it withdrawn as replaced", text(out))
		}
		f.answerItems(t, true, items[0].ID)
		want(t, first, `"tempSet":[{"name":"ids"`)
		e, ok := f.keptValue(t, "ids")
		if !ok || string(e.Value) != "[1]" || !strings.Contains(e.Source.Request, "order by id") || strings.Contains(e.Source.Request, "desc") {
			t.Fatalf("ids = %s from %q ok=%v, want [1] from the statement delivered last", e.Value, e.Source.Request, ok)
		}
		idle(t)
		if got := f.notesBodies(t); !slices.Equal(got, notes) {
			t.Fatalf("notes = %q, want the withdrawn write not to have run", got)
		}
	})

	t.Run("script slots under pressure: eight finish, two time out, the next call runs", func(t *testing.T) {
		f.mcpSet(t, map[string]string{"review.read": "send"})
		clients := []*mcp.ClientSession{r.a, r.b, r.s}
		calls := make([]chan callOutcome, 10)
		var start sync.WaitGroup
		start.Add(1)
		for i := range calls {
			script := fmt.Sprintf(`return "r%d";`, i)
			if i%5 == 4 {
				script = `while (true) {}`
			}
			ch := make(chan callOutcome, 1)
			calls[i] = ch
			go func(c *mcp.ClientSession) {
				start.Wait()
				ch <- <-callToolAsync(c, "run_query", f.queryScriptArgs(scriptStmt, "read", script))
			}(clients[i%len(clients)])
		}
		start.Done()
		var done, timedOut int
		for i, ch := range calls {
			res := wait(t, ch)
			switch {
			case i%5 == 4 && res.IsError && strings.Contains(text(res), "timeout"):
				timedOut++
			case i%5 != 4 && !res.IsError && strings.Contains(text(res), fmt.Sprintf(`"output":"r%d"`, i)):
				done++
			default:
				t.Errorf("call %d = %q (error %v)", i, text(res), res.IsError)
			}
		}
		if done != 8 || timedOut != 2 {
			t.Fatalf("%d finished and %d timed out, want 8 and 2", done, timedOut)
		}
		if res := callTool(t, r.a, "run_query", f.queryScriptArgs(scriptStmt, "read", `return "after";`)); res.IsError || !strings.Contains(text(res), `"output":"after"`) {
			t.Fatalf("next call = %q, want a free slot", text(res))
		}
	})

	t.Run("an answer for everything approves what can be and leaves the card that needs a value", func(t *testing.T) {
		f.app.W.Kept.ClearAll()
		f.keep(t, externalScope, "ids", "[1,2]")
		exempt := f.queryScriptArgs(e12Exempt, "read", `return result.rows.length;`, "ids")
		id := exemptionID(t, callTool(t, r.a, "run_query", exempt))
		exempt["exemption"] = map[string]any{"id": id, "reason": "needs the full run"}
		input := apiArgs(envID, "GET", "{{base_url}}/echo?m=m9&k={{secret.apikey}}")
		input["inputs"] = []string{"apikey"}
		notes := f.notesBodies(t)
		calls := []chan callOutcome{callToolAsync(r.a, "api_request", jsonPost(envID, r.echo, `{"m": "m6", "ids": {{plain.ids}}}`))}
		f.items(t, 1)
		calls = append(calls, callToolAsync(r.b, "run_query", f.paramsArgs(e13Insert, "write", "ids")))
		f.items(t, 2)
		calls = append(calls, callToolAsync(r.a, "run_query", exempt))
		f.items(t, 3)
		blocked := callToolAsync(r.s, "api_request", input)
		items := f.items(t, 4)
		ids := make([]string, len(items))
		for i, it := range items {
			ids[i] = it.ID
		}
		res, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: ids, Approve: true})
		if err != nil || len(res) != 4 {
			t.Fatalf("answer = %+v (%v)", res, err)
		}
		for i, r := range res {
			if (i == 3) != (r.Error != "") {
				t.Fatalf("results = %+v, want only the input card refused", res)
			}
		}
		want(t, calls[0], "m6")
		if out := wait(t, calls[1]); out.IsError {
			t.Fatalf("write = %q", text(out))
		}
		want(t, calls[2], `"output":2`)
		if left := f.items(t, 1); left[0].ID != items[3].ID {
			t.Fatalf("left = %s, want the input card", left[0].ID)
		}
		f.answerItems(t, false, items[3].ID)
		if out := wait(t, blocked); !out.IsError || !strings.Contains(text(out), "denied by the user") {
			t.Fatalf("input call = %q, want a denial", text(out))
		}
		if got := f.notesBodies(t); len(got) != len(notes)+2 {
			t.Fatalf("notes = %q, want only the approved write added", got)
		}
	})
}
