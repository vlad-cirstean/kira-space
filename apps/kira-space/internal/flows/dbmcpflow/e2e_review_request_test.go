package dbmcpflow

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-review-request.spec.ts (contract e2e-review-request): a held scripted
// result of an API call, a database read and a kube list carries the request the prompt showed. The UI half
// opens the Request section of each review and compares it to the card.

const reviewRequestKey = "e2e-review-request"

// fields decodes the named top-level keys of an item payload.
func fields(t *testing.T, it approvals.Item, keys ...string) map[string]any {
	t.Helper()
	var all map[string]any
	if err := json.Unmarshal(it.Payload, &all); err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for _, k := range keys {
		out[k] = all[k]
	}
	return out
}

func sameFields(t *testing.T, what string, card, held map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(card, held) {
		t.Fatalf("%s: held request %v, want the prompt's %v", what, held, card)
	}
}

func TestE2EReviewRequestAPI(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setAPI(t, true, "always")
	f.setReviewApp(t, true)
	opts := f.e2eOpts(srv.URL)

	args := scriptArgs(envID, "POST", loginURL(srv.URL), map[string]any{"transform": `return {account: response.json.account};`})
	args["headers"] = []map[string]string{{"name": "X-Trace", "value": "t-1"}}
	pending := callToolAsync(sess, "api_request", args)
	card := f.pending(t)
	f.app.Contract(t, reviewRequestKey, "ApprovalsService.List#api-card", []approvals.Item{card.Item}, opts...)
	f.answerItems(t, true, card.Item.ID)
	h := f.startHeldFromQueue(t)
	h.pending = pending

	sameFields(t, "api method and url", fields(t, card.Item, "method", "url"), fields(t, h.req.Item, "method", "url"))
	held := fields(t, h.req.Item, "request")["request"].(map[string]any)
	sameFields(t, "api headers, body and variables", fields(t, card.Item, "headers", "body", "variables"),
		map[string]any{"headers": held["headers"], "body": held["body"], "variables": held["variables"]})

	sized := append(slices.Clip(opts), flowharness.Mask("rawBytes", "bytes"))
	f.app.Contract(t, reviewRequestKey, "ApprovalsService.List#api", []approvals.Item{h.req.Item}, sized...)
	f.app.Contract(t, reviewRequestKey, "DbMcpService.PendingResponse#api", stableHeld(h.held), opts...)
	f.finish(t, h, false)
}

func TestE2EReviewRequestDB(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"read": "prompt", "write": "prompt", "ddl": "deny", "review.read": "review"})
	opts := scriptChainOpts(f, flowharness.Mask("rawBytes", "bytes"))

	pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", keepIDsScript))
	card := f.pending(t)
	if card.Stage != "query" {
		t.Fatalf("pending = %+v, want the query card first", card)
	}
	f.app.Contract(t, reviewRequestKey, "ApprovalsService.List#db-card", []approvals.Item{card.Item}, opts...)
	f.answer(t, true, card.RequestID)
	h := f.startHeldFromQueue(t)

	sameFields(t, "db statement",
		fields(t, card.Item, "statement", "params", "placeholders", "kept"),
		fields(t, h.req.Item, "statement", "params", "placeholders", "kept"))
	f.app.Contract(t, reviewRequestKey, "ApprovalsService.List#db", []approvals.Item{h.req.Item}, opts...)
	f.app.Contract(t, reviewRequestKey, "DbMcpService.PendingResponse#db", stableHeld(h.held), opts...)
	f.answer(t, false, h.req.RequestID)
	wait(t, pending)
}

func TestE2EReviewRequestKube(t *testing.T) {
	captureLog(t)
	k := newKubeFixture(t)
	opts := []flowharness.ContractOption{
		flowharness.Replace(k.fake.URL, "<kube>"), flowharness.Replace(k.cfg, "<kubeconfig>"),
		flowharness.Mask("id", "clusterId", "requestId", "connectionId", "expiresAtMs", "expiresAt", "createdAt", "startedAt", "durationMs", "cwd", "hash", "rawBytes", "bytes"),
	}
	args := k.listPods("default", map[string]any{"script": map[string]any{
		"prepare":   `return {labelSelector: "app=web", maxRows: 5};`,
		"transform": `return {first: result.items[0].metadata.name};`,
	}})

	k.setMcp(t, "kind-dev", on("prompt"))
	pending := k.callAsync("kube_list_resources", args)
	card := k.pending(t)
	k.app.Contract(t, reviewRequestKey, "ApprovalsService.List#kube-card", []approvals.Item{card.Item}, opts...)
	k.answerItems(t, false, card.Item.ID)
	<-pending

	k.setMcp(t, "kind-dev", review("review"))
	pending, req, held := k.held(t, "kube_list_resources", args)
	sameFields(t, "kube request",
		fields(t, card.Item, "tool", "request", "changed"),
		fields(t, req.Item, "tool", "request", "changed"))
	k.app.Contract(t, reviewRequestKey, "ApprovalsService.List#kube", []approvals.Item{req.Item}, opts...)
	k.app.Contract(t, reviewRequestKey, "DbMcpService.PendingResponse#kube", stableHeld(held), opts...)
	k.answerItems(t, false, req.Item.ID)
	<-pending
}
