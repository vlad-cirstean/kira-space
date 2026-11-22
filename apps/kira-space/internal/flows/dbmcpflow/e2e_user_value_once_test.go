package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/approvals-api-input-once.spec.ts (contract e2e-user-value-once): a value the
// user types for one request only reaches that request and is stored nowhere.

const (
	onceKey    = "e2e-user-value-once"
	onceTyped  = "once-typed-otp-4F8"
	keptTyped  = "kept-typed-key-9K2"
	otherTyped = "once-replaced-key-6M1"
	deriveText = "once-derive-code-3B7"
)

type oncePayload struct {
	Inputs []struct {
		Name, Label, State, Mode string
		KeepMinutes              int
	}
}

func answerModes(itemID string, values, modes map[string]string) approvals.AnswerArgs {
	opts, _ := json.Marshal(map[string]any{"inputs": values, "inputModes": modes})
	return approvals.AnswerArgs{IDs: []string{itemID}, Approve: true, Options: map[string]json.RawMessage{itemID: opts}}
}

func TestE2EUserValueOnce(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	opts := f.e2eOpts(srv.URL)
	var agent []string
	remember := func(res string) { agent = append(agent, res) }
	otpArgs := func(path string) map[string]any {
		args := apiArgs(envID, "GET", "{{base_url}}"+path+"?code={{kept.otp}}", "X-Otp", "{{kept.otp}}")
		args["inputs"] = []map[string]any{{"name": "otp", "label": "One-time code"}}
		return args
	}
	mixedArgs := func(path string) map[string]any {
		args := apiArgs(envID, "GET", "{{base_url}}"+path+"?code={{kept.otp}}", "X-Key", "{{kept.partner_key}}")
		args["inputs"] = []map[string]any{
			{"name": "partner_key", "label": "Partner key", "reuse": true},
			{"name": "otp", "label": "One-time code"},
		}
		return args
	}
	payloadOf := func(t *testing.T, it approvals.Item) oncePayload {
		t.Helper()
		var p oncePayload
		if err := json.Unmarshal(it.Payload, &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	answer := func(t *testing.T, id string, values, modes map[string]string) {
		t.Helper()
		res, err := f.app.W.ApprovalsSvc.Answer(answerModes(id, values, modes))
		if err != nil || len(res) != 1 || res[0].Error != "" {
			t.Fatalf("answer = %+v (%v)", res, err)
		}
	}
	type outcome struct {
		Body   string
		Inputs []struct {
			Name, Status     string
			ExpiresInSeconds int
		}
		KeptNotSaved []struct{ Name, Error string }
	}
	decode := func(t *testing.T, text string) outcome {
		t.Helper()
		var o outcome
		if err := json.Unmarshal([]byte(text), &o); err != nil {
			t.Fatalf("result %q: %v", text, err)
		}
		return o
	}

	t.Run("once: sent, stored nowhere", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", otpArgs("/echo"))
		req := f.pending(t)
		p := payloadOf(t, req.Item)
		if len(p.Inputs) != 1 || p.Inputs[0].State != "needed" || p.Inputs[0].Mode != "once" || p.Inputs[0].KeepMinutes != 60 {
			t.Fatalf("inputs = %+v, want one needed input, mode once, keep 60 min", p.Inputs)
		}
		f.app.Contract(t, onceKey, "ApprovalsService.List#once", []approvals.Item{req.Item}, opts...)
		f.app.Contract(t, onceKey, "args:ApprovalsService.Answer#once", answerModes(req.Item.ID, map[string]string{"otp": onceTyped}, map[string]string{"otp": "once"}))
		answer(t, req.Item.ID, map[string]string{"otp": "  " + onceTyped + " "}, map[string]string{"otp": "once"})
		out := wait(t, pending)
		remember(text(out))
		got := decode(t, text(out))
		if out.IsError || len(got.Inputs) != 1 || got.Inputs[0].Status != "once" || got.Inputs[0].ExpiresInSeconds != 0 || strings.Count(got.Body, "{{kept.otp}}") < 2 {
			t.Fatalf("result = %q, want status once and the echo masked as the reference", text(out))
		}
		if seen := srv.Requests(); len(seen) != 1 || seen[0].Header.Get("X-Otp") != onceTyped || !strings.Contains(seen[0].Query, "code="+onceTyped) {
			t.Fatalf("server saw %+v, want the trimmed value in header and query", seen)
		}
		if n := f.keptNames(externalScope); len(n) != 0 {
			t.Fatalf("kept store = %v, want empty", n)
		}
		list, err := f.app.W.KeptVars.List()
		if err != nil || len(list.Entries) != 0 {
			t.Fatalf("KeptVarsService.List = %+v (%v), want no entry", list, err)
		}
		if listing := text(callTool(t, sess, "list_kept_variables", map[string]any{})); strings.Contains(listing, "otp") {
			t.Fatalf("list_kept_variables = %s, want no otp", listing)
		}
	})

	t.Run("asked again", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", otpArgs("/echo/second"))
		req := f.pending(t)
		if p := payloadOf(t, req.Item); len(p.Inputs) != 1 || p.Inputs[0].State != "needed" {
			t.Fatalf("inputs = %+v, want needed again", p.Inputs)
		}
		f.app.Contract(t, onceKey, "ApprovalsService.List#again", []approvals.Item{req.Item}, opts...)
		f.answerItems(t, false, req.Item.ID)
		remember(text(wait(t, pending)))
	})

	t.Run("mixed: keep one, use one once", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", mixedArgs("/echo/mixed"))
		req := f.pending(t)
		p := payloadOf(t, req.Item)
		if len(p.Inputs) != 2 || p.Inputs[0].Mode != "keep" || p.Inputs[1].Mode != "once" {
			t.Fatalf("inputs = %+v, want partner_key suggested keep and otp once", p.Inputs)
		}
		f.app.Contract(t, onceKey, "ApprovalsService.List#mixed", []approvals.Item{req.Item}, opts...)
		answerArgs := answerModes(req.Item.ID, map[string]string{"partner_key": keptTyped, "otp": onceTyped}, map[string]string{"partner_key": "keep", "otp": "once"})
		f.app.Contract(t, onceKey, "args:ApprovalsService.Answer#mixed", answerArgs)
		if res, err := f.app.W.ApprovalsSvc.Answer(answerArgs); err != nil || res[0].Error != "" {
			t.Fatalf("answer = %+v (%v)", res, err)
		}
		out := wait(t, pending)
		remember(text(out))
		if out.IsError {
			t.Fatalf("result = %q", text(out))
		}
		if names := f.keptNames(externalScope); len(names) != 1 || names[0] != "partner_key" {
			t.Fatalf("kept store = %v, want only partner_key", names)
		}
	})

	t.Run("later request: kept reused, one-time asked again", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", mixedArgs("/echo/later"))
		req := f.pending(t)
		p := payloadOf(t, req.Item)
		if len(p.Inputs) != 2 || p.Inputs[0].State != "stored" || p.Inputs[1].State != "needed" {
			t.Fatalf("inputs = %+v, want partner_key stored and otp needed", p.Inputs)
		}
		f.app.Contract(t, onceKey, "ApprovalsService.List#later", []approvals.Item{req.Item},
			append(opts, flowharness.Replace(f.keptID(t, externalScope, "partner_key"), "kv_partner_key"))...)
		f.app.Contract(t, onceKey, "args:ApprovalsService.Answer#later", answerModes(req.Item.ID, map[string]string{"otp": onceTyped}, map[string]string{"otp": "once"}))
		answer(t, req.Item.ID, map[string]string{"otp": onceTyped}, map[string]string{"otp": "once"})
		out := wait(t, pending)
		remember(text(out))
		got := decode(t, text(out))
		if out.IsError || len(got.Inputs) != 2 || got.Inputs[0].Status != "reused" || got.Inputs[1].Status != "once" {
			t.Fatalf("result = %q, want partner_key reused and otp once", text(out))
		}
	})

	t.Run("a transform reading it keeps nothing derived", func(t *testing.T) {
		args := otpArgs("/echo/derive")
		args["script"] = map[string]any{"transform": `kira.storeValue("derived", kira.getValue("otp") + "-x"); return "done"`}
		pending := callToolAsync(sess, "api_request", args)
		req := f.pending(t)
		answer(t, req.Item.ID, map[string]string{"otp": deriveText}, map[string]string{"otp": "once"})
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
		held := f.pending(t)
		if p := f.responseItem(t, held.Item); p.Reason != "userValue" {
			t.Fatalf("held reason = %q, want userValue", p.Reason)
		}
		pr := f.pendingResponse(t, held.RequestID)
		if len(pr.Keep) != 1 || pr.Keep[0].Name != "derived" || !pr.Keep[0].Blocked {
			t.Fatalf("held keep = %+v, want derived marked blocked", pr.Keep)
		}
		f.app.Contract(t, onceKey, "ApprovalsService.List#derived", []approvals.Item{held.Item}, opts...)
		pr.Text, pr.Raw = stable(pr.Text), stable(pr.Raw)
		f.app.Contract(t, onceKey, "DbMcpService.PendingResponse#derived", pr, opts...)
		f.answerItems(t, true, held.Item.ID)
		out := wait(t, pending)
		remember(text(out))
		got := decode(t, text(out))
		if out.IsError || len(got.KeptNotSaved) != 1 || got.KeptNotSaved[0].Name != "derived" || !strings.Contains(got.KeptNotSaved[0].Error, "this request only") {
			t.Fatalf("result = %q, want derived refused", text(out))
		}
		for _, n := range f.keptNames(externalScope) {
			if n == "derived" || n == "otp" {
				t.Fatalf("kept store holds %q", n)
			}
		}
	})

	t.Run("replace a stored value with a one-time one", func(t *testing.T) {
		args := apiArgs(envID, "GET", "{{base_url}}/echo/replace", "X-Key", "{{kept.partner_key}}")
		args["inputs"] = []map[string]any{{"name": "partner_key", "label": "Partner key"}}
		pending := callToolAsync(sess, "api_request", args)
		req := f.pending(t)
		if p := payloadOf(t, req.Item); len(p.Inputs) != 1 || p.Inputs[0].State != "stored" {
			t.Fatalf("inputs = %+v, want partner_key stored", p.Inputs)
		}
		f.app.Contract(t, onceKey, "ApprovalsService.List#replace", []approvals.Item{req.Item},
			append(opts, flowharness.Replace(f.keptID(t, externalScope, "partner_key"), "kv_partner_key"))...)
		f.app.Contract(t, onceKey, "args:ApprovalsService.Answer#replace", answerModes(req.Item.ID, map[string]string{"partner_key": otherTyped}, map[string]string{"partner_key": "once"}))
		answer(t, req.Item.ID, map[string]string{"partner_key": otherTyped}, map[string]string{"partner_key": "once"})
		out := wait(t, pending)
		remember(text(out))
		got := decode(t, text(out))
		if out.IsError || len(got.Inputs) != 1 || got.Inputs[0].Status != "once" {
			t.Fatalf("result = %q, want once", text(out))
		}
		if _, ok := f.keptValue(t, "partner_key"); ok {
			t.Fatal("the replaced stored value is still held")
		}
		seen := srv.Requests()
		if last := seen[len(seen)-1]; last.Header.Get("X-Key") != otherTyped {
			t.Fatalf("server saw %q, want the one-time value", last.Header.Get("X-Key"))
		}
	})

	t.Run("answers the card refuses", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", otpArgs("/echo/refuse"))
		req := f.pending(t)
		for name, a := range map[string]approvals.AnswerArgs{
			"mode for an input not asked": answerModes(req.Item.ID, map[string]string{"otp": onceTyped}, map[string]string{"otp": "once", "ghost": "once"}),
			"mode without a value":        answerModes(req.Item.ID, map[string]string{"otp": onceTyped}, map[string]string{"otp": "once", "partner_key": "keep"}),
			"unknown mode":                answerModes(req.Item.ID, map[string]string{"otp": onceTyped}, map[string]string{"otp": "forever"}),
		} {
			res, err := f.app.W.ApprovalsSvc.Answer(a)
			if err != nil || len(res) != 1 || res[0].Error == "" {
				t.Fatalf("%s = %+v (%v), want a refusal", name, res, err)
			}
		}
		if f.queueLen(t) != 1 {
			t.Fatalf("queue = %d, want the item to stay pending", f.queueLen(t))
		}
		f.answerItems(t, false, req.Item.ID)
		remember(text(wait(t, pending)))
	})

	t.Run("no typed value is anywhere", func(t *testing.T) {
		for _, v := range []string{onceTyped, otherTyped, deriveText} {
			f.noLeak(t, v, agent...)
		}
	})
}
