package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-user-value.spec.ts (contract e2e-user-value): a value the user types
// into a card is reused by later requests and scripts, masked in every answer and shown nowhere afterwards.

const (
	userValueKey = "e2e-user-value"
	e2eTyped     = "e2e-typed-secret-7Q9"
)

func TestE2EUserValue(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	opts := f.e2eOpts(srv.URL)
	var agent []string
	remember := func(res string) { agent = append(agent, res) }
	withInput := func(path string) map[string]any {
		args := apiArgs(envID, "GET", "{{base_url}}"+path+"?k={{kept.apikey}}", "X-Api-Key", "{{kept.apikey}}")
		args["inputs"] = []map[string]string{{"name": "apikey", "label": "API key"}}
		return args
	}
	type inputState struct {
		Inputs []struct{ Name, State string }
	}
	stateOf := func(t *testing.T, it approvals.Item) string {
		t.Helper()
		var p inputState
		if err := json.Unmarshal(it.Payload, &p); err != nil || len(p.Inputs) != 1 {
			t.Fatalf("payload inputs = %s (%v)", it.Payload, err)
		}
		return p.Inputs[0].State
	}

	t.Run("typed once, sent in header and query, masked in the answer", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", withInput("/echo"))
		req := f.pending(t)
		if s := stateOf(t, req.Item); s != "needed" {
			t.Fatalf("input state = %q, want needed", s)
		}
		f.app.Contract(t, userValueKey, "ApprovalsService.List#needed", []approvals.Item{req.Item}, opts...)
		f.app.Contract(t, userValueKey, "args:ApprovalsService.Answer#typed", inputAnswer(req.Item.ID, e2eTyped))
		res, err := f.app.W.ApprovalsSvc.Answer(inputAnswer(req.Item.ID, "  "+e2eTyped+" "))
		if err != nil || len(res) != 1 || res[0].Error != "" {
			t.Fatalf("answer = %+v (%v)", res, err)
		}
		out := wait(t, pending)
		remember(text(out))
		var got struct {
			Body   string
			Inputs []struct{ Name, Status string }
		}
		if err := json.Unmarshal([]byte(text(out)), &got); err != nil || out.IsError {
			t.Fatalf("result = %q (%v)", text(out), err)
		}
		if len(got.Inputs) != 1 || got.Inputs[0].Status != "supplied" || strings.Count(got.Body, "{{kept.apikey}}") < 2 {
			t.Fatalf("result = %q, want supplied and the echoed header and query masked as the reference", text(out))
		}
		seen := srv.Requests()
		if len(seen) != 1 || seen[0].Header.Get("X-Api-Key") != e2eTyped || !strings.Contains(seen[0].Query, "k="+e2eTyped) {
			t.Fatalf("server saw %+v, want the trimmed value in the header and the query", seen)
		}
	})

	t.Run("reused without typing", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", withInput("/echo/second"))
		req := f.pending(t)
		if s := stateOf(t, req.Item); s != "stored" {
			t.Fatalf("input state = %q, want stored", s)
		}
		f.app.Contract(t, userValueKey, "ApprovalsService.List#stored", []approvals.Item{req.Item},
			append(opts, flowharness.Replace(f.keptID(t, externalScope, "apikey"), "kv_apikey"))...)
		f.answerItems(t, true, req.Item.ID)
		out := wait(t, pending)
		remember(text(out))
		if out.IsError || !strings.Contains(text(out), `"status":"reused"`) {
			t.Fatalf("result = %q, want the input reused", text(out))
		}
	})

	t.Run("a transform reading it forces the review", func(t *testing.T) {
		args := apiArgs(envID, "GET", "{{base_url}}/echo")
		args["script"] = map[string]any{"transform": `return kira.getValue("apikey") + "|" + kira.getValue("apikey").split("").reverse().join("")`}
		h := f.startHeldAfterCard(t, sess, args)
		if p := f.responseItem(t, h.req.Item); p.Reason != "userValue" {
			t.Fatalf("held reason = %q, want userValue though the review settings are off", p.Reason)
		}
		reversed := reverse(e2eTyped)
		if !strings.Contains(h.held.Text, "{{kept.apikey}}|"+reversed) || strings.Contains(h.held.Text, e2eTyped) {
			t.Fatalf("held text = %q, want the value masked and its reversal visible to the user (Known open item)", h.held.Text)
		}
		f.app.Contract(t, userValueKey, "ApprovalsService.List#forced", []approvals.Item{h.req.Item}, opts...)
		held := h.held
		held.Text, held.Raw = stable(held.Text), stable(held.Raw)
		f.app.Contract(t, userValueKey, "DbMcpService.PendingResponse#forced", held, opts...)
		f.app.Contract(t, userValueKey, "args:ApprovalsService.Answer#withhold", approvals.AnswerArgs{IDs: []string{h.req.Item.ID}, Approve: false})
		list, err := f.app.W.KeptVars.List()
		if err != nil {
			t.Fatal(err)
		}
		f.app.Contract(t, userValueKey, "KeptVarsService.List#held", list, append(opts, flowharness.Replace(f.keptID(t, externalScope, "apikey"), "kv_apikey"))...)

		res := f.finish(t, h, false)
		remember(text(res))
		if !res.IsError || strings.Contains(text(res), reversed) || strings.Contains(text(res), "result") {
			t.Fatalf("result = %q, want the withheld text and no data", text(res))
		}
	})

	t.Run("prepare cannot read it", func(t *testing.T) {
		sent := len(srv.Requests())
		res := callTool(t, sess, "api_request", scriptArgs(envID, "GET", srv.URL+"/echo", map[string]any{
			"prepare": `return {headers: [{name: "X-K", value: kira.getValue("apikey")}]};`,
		}))
		remember(text(res))
		if !res.IsError || !strings.Contains(text(res), "is secret") || !strings.HasSuffix(text(res), "Nothing was sent.") || f.queueLen(t) != 0 || len(srv.Requests()) != sent {
			t.Fatalf("result = %q, queue %d, want a refusal before any card", text(res), f.queueLen(t))
		}
	})

	t.Run("another origin is refused", func(t *testing.T) {
		other := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
		sent := len(srv.Requests())
		res := callTool(t, sess, "api_request", apiArgs(envID, "GET", other+"/echo?k={{kept.apikey}}"))
		remember(text(res))
		if !res.IsError || !strings.Contains(text(res), "apikey") || f.queueLen(t) != 0 || len(srv.Requests()) != sent {
			t.Fatalf("result = %q, queue %d, want a refusal before any card", text(res), f.queueLen(t))
		}
		f.app.Contract(t, userValueKey, "result#otherOrigin", text(res), append(opts, flowharness.Replace(other, "http://localhost:<port>"))...)
	})

	t.Run("cleared, asked again, denied", func(t *testing.T) {
		id := f.keptID(t, externalScope, "apikey")
		clear := bridge.KeptClearArgs{IDs: []string{id}}
		f.app.Contract(t, userValueKey, "args:KeptVarsService.Clear#cleared", clear, flowharness.Replace(id, "kv_apikey"))
		if _, err := f.app.W.KeptVars.Clear(clear); err != nil {
			t.Fatal(err)
		}
		sent := len(srv.Requests())
		pending := callToolAsync(sess, "api_request", withInput("/echo"))
		req := f.pending(t)
		if s := stateOf(t, req.Item); s != "needed" {
			t.Fatalf("input state = %q, want needed again", s)
		}
		f.app.Contract(t, userValueKey, "ApprovalsService.List#again", []approvals.Item{req.Item}, opts...)
		f.app.Contract(t, userValueKey, "args:ApprovalsService.Answer#deny", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: false})
		f.answerItems(t, false, req.Item.ID)
		out := wait(t, pending)
		remember(text(out))
		if !out.IsError {
			t.Fatalf("result = %q, want a denial", text(out))
		}
		if _, ok := f.keptValue(t, "apikey"); ok || len(srv.Requests()) != sent {
			t.Fatalf("kept %v, sent %d, want nothing stored or sent", ok, len(srv.Requests())-sent)
		}
	})

	t.Run("the value is nowhere", func(t *testing.T) {
		f.noLeak(t, e2eTyped, agent...)
	})
}

func reverse(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}
