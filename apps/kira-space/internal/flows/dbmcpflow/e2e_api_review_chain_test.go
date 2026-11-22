package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/e2e-api-review-chain.spec.ts (contract e2e-api-review-chain): prepare,
// approval, transform and review in one chain. The user sees the raw and the transformed response before
// anything is kept, and the kept value lands only on Send.

const (
	reviewChainKey = "e2e-api-review-chain"
	chainDomains   = `plain.domains = response.json.users.map(u => u.email.split("@")[1]); return response.json.users.length;`
)

func TestE2EApiReviewChain(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setAPI(t, true, "always")
	f.setReviewApp(t, true)
	opts := f.e2eOpts(srv.URL)
	args := func(transform string) map[string]any {
		return scriptArgs(envID, "POST", usersURL(srv.URL), map[string]any{
			"prepare": prepareSource, "input": map[string]any{"ids": []int{1, 2}}, "transform": transform,
		})
	}
	// start runs the call through its approval card and returns it with the response held.
	start := func(t *testing.T, transform string, record bool) heldCall {
		t.Helper()
		pending := callToolAsync(sess, "api_request", args(transform))
		req := f.pending(t)
		if req.Item.Kind != approvals.KindAPIRequest {
			t.Fatalf("item kind = %s, want api.request", req.Item.Kind)
		}
		if record {
			var pl scriptItem
			if err := json.Unmarshal(req.Item.Payload, &pl); err != nil {
				t.Fatal(err)
			}
			p := pl.Script.Prepare
			if p == nil || p.Source != prepareSource || p.Input != `{"ids":[1,2]}` || strings.Join(p.Changed, ",") != "body,contentType" || pl.Script.Transform == nil {
				t.Fatalf("card script = %+v, want prepare with input and a transform", pl.Script)
			}
			f.app.Contract(t, reviewChainKey, "ApprovalsService.List#card", []approvals.Item{req.Item}, opts...)
			f.app.Contract(t, reviewChainKey, "args:ApprovalsService.Answer#card", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		}
		f.answerItems(t, true, req.Item.ID)
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
		held := f.pending(t)
		return heldCall{pending: pending, req: held, held: f.pendingResponse(t, held.RequestID)}
	}
	sentBody := func(t *testing.T, n int) {
		t.Helper()
		seen := srv.Requests()
		if len(seen) != n || string(seen[n-1].Body) != `{"items":[{"id":1},{"id":2}]}` {
			t.Fatalf("server saw %d requests, last body %q, want the prepared body", len(seen), seen[len(seen)-1].Body)
		}
	}
	stored := func() bool { _, ok := f.keptValue(t, "domains"); return ok }

	t.Run("the held response shows raw and result and keeps nothing yet", func(t *testing.T) {
		h := start(t, chainDomains, true)
		sentBody(t, 1)
		held := h.held
		if !strings.Contains(held.Raw, `"status":200`) || !strings.Contains(held.Raw, "a@x.test") || strings.Contains(held.Raw, apiSecret) ||
			held.Text != `{"result":2}` || len(held.Keep) != 1 || held.Keep[0].Name != "domains" || held.Keep[0].Preview != `["x.test","y.test"]` {
			t.Fatalf("pending = %+v, want raw 200, text {\"result\":2} and domains in Will keep", held)
		}
		if stored() {
			t.Fatal("domains stored before Send")
		}
		if list := text(callTool(t, sess, "list_saved_variables", map[string]any{})); strings.Contains(list, "domains") {
			t.Fatalf("agent listing = %s, want no domains while the review waits", list)
		}
		f.app.Contract(t, reviewChainKey, "ApprovalsService.List#review", []approvals.Item{h.req.Item}, opts...)
		held.Text, held.Raw = stable(held.Text), stable(held.Raw)
		f.app.Contract(t, reviewChainKey, "DbMcpService.PendingResponse#review", held, opts...)

		f.app.Contract(t, reviewChainKey, "args:ApprovalsService.Answer#withhold", approvals.AnswerArgs{IDs: []string{h.req.Item.ID}, Approve: false})
		res := f.finish(t, h, false)
		if !res.IsError || !strings.Contains(text(res), "the user withheld the response") || strings.Contains(text(res), "result") || strings.Contains(text(res), "x.test") {
			t.Fatalf("result = %q, want the withheld text and no data", text(res))
		}
		if stored() {
			t.Fatal("domains stored after Withhold")
		}
		if got := len(srv.Requests()); got != 1 {
			t.Fatalf("server saw %d requests, want 1", got)
		}
	})

	var domainsID string
	t.Run("Send delivers the held result and keeps the value", func(t *testing.T) {
		h := start(t, chainDomains, false)
		f.app.Contract(t, reviewChainKey, "args:ApprovalsService.Answer#send", approvals.AnswerArgs{IDs: []string{h.req.Item.ID}, Approve: true})
		heldText := h.held.Text
		res := f.finish(t, h, true)
		var agent, shown struct{ Result int }
		if err := json.Unmarshal([]byte(text(res)), &agent); err != nil || res.IsError {
			t.Fatalf("result = %q (%v)", text(res), err)
		}
		_ = json.Unmarshal([]byte(heldText), &shown)
		if agent.Result != shown.Result || agent.Result != 2 || !strings.Contains(text(res), `"saved":[{"name":"domains"`) {
			t.Fatalf("result = %q, held %q, want the held result plus a kept summary", text(res), heldText)
		}
		f.app.Contract(t, reviewChainKey, "result#send", expiresIn.ReplaceAllString(stable(text(res)), `"expiresInSeconds":0`), opts...)
		e, ok := f.keptValue(t, "domains")
		if !ok || string(e.Value) != `["x.test","y.test"]` || e.Source.Tool != "api_request" || !strings.HasPrefix(e.Source.Request, "POST http://127.0.0.1:") {
			t.Fatalf("stored = %+v ok=%v, want the domains with this request as source", e.Meta, ok)
		}
		domainsID = e.ID
		sentBody(t, 2)
	})

	t.Run("a failed transform is held with its raw response and keeps nothing", func(t *testing.T) {
		h := start(t, `plain.domains = 1; return null.x`, false)
		if !h.held.IsError || len(h.held.Keep) != 0 || !strings.Contains(h.held.Raw, "a@x.test") || !strings.Contains(h.held.Text, "transform script failed:") {
			t.Fatalf("pending = %+v, want an error item with the raw response and no staged values", h.held)
		}
		var p responseItem = f.responseItem(t, h.req.Item)
		if !p.IsError || !p.Scripted {
			t.Fatalf("payload = %+v, want a scripted error item", p)
		}
		f.app.Contract(t, reviewChainKey, "ApprovalsService.List#failed", []approvals.Item{h.req.Item}, opts...)
		held := h.held
		held.Text, held.Raw = stable(held.Text), stable(held.Raw)
		f.app.Contract(t, reviewChainKey, "DbMcpService.PendingResponse#failed", held, opts...)
		res := f.finish(t, h, true)
		if !res.IsError || !strings.Contains(text(res), "transform script failed:") || !strings.Contains(text(res), "was sent (HTTP 200)") {
			t.Fatalf("result = %q, want the failure text", text(res))
		}
		if e, ok := f.keptValue(t, "domains"); !ok || e.ID != domainsID || string(e.Value) != `["x.test","y.test"]` {
			t.Fatalf("domains = %+v ok=%v, want the step 4 entry unchanged", e.Meta, ok)
		}
	})
}
