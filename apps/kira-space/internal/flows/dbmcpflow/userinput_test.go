package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// typedSecret is what the user types in the card; nothing the agent, a log or the op log holds may carry it.
const typedSecret = "typed-secret-value-42"

func keyArgs(envID, url string) map[string]any {
	args := apiArgs(envID, "GET", url, "X-Api-Key", "{{secret.apikey}}")
	args["inputs"] = []string{"apikey"}
	return args
}

func inputAnswer(itemID, value string) approvals.AnswerArgs {
	opts, _ := json.Marshal(map[string]any{"inputs": map[string]string{"apikey": value}})
	return approvals.AnswerArgs{IDs: []string{itemID}, Approve: true, Options: map[string]json.RawMessage{itemID: opts}}
}

type inputPayload struct {
	Inputs []struct{ Name, State, Origin, KeptID string }
	Kept   []struct {
		ID, Name string
		Pending  bool
		Secret   bool
	}
}

// TestUserInputSupplied: the agent declares an input, the card asks for it, the typed value is kept as a
// secret bound to the request's origin, sent in the header, masked in the result and absent from every log.
// A second request reuses the held value without asking again.
// UI half: tests/ui/studio/approvals-api-input.spec.ts.
func TestUserInputSupplied(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)

	pending := callToolAsync(sess, "api_request", keyArgs(envID, "{{base_url}}/echo"))
	req := f.pending(t)
	var pl inputPayload
	if err := json.Unmarshal(req.Item.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	if len(pl.Inputs) != 1 || pl.Inputs[0].State != "needed" || pl.Inputs[0].Origin != srv.URL {
		t.Fatalf("inputs = %+v, want one needed input bound to %s", pl.Inputs, srv.URL)
	}
	if len(pl.Kept) != 1 || !pl.Kept[0].Pending || pl.Kept[0].ID != "" || !pl.Kept[0].Secret {
		t.Fatalf("kept = %+v, want one pending secret use with no id", pl.Kept)
	}
	opts := []flowharness.ContractOption{
		flowharness.Mask("createdAt", "expiresAt", "cwd", "environment.id"), flowharness.Replace(srv.URL, "http://127.0.0.1:<port>"),
	}
	f.app.Contract(t, "approvals-api-input", "ApprovalsService.List", []approvals.Item{req.Item}, opts...)
	f.app.Contract(t, "approvals-api-input", "args:ApprovalsService.Answer", inputAnswer(req.Item.ID, typedSecret))

	// Approve without the value is refused and the item stays.
	bare, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
	if err != nil || len(bare) != 1 || bare[0].Error == "" {
		t.Fatalf("approve without a value = %+v, %v, want a refusal", bare, err)
	}
	if f.queueLen(t) != 1 {
		t.Fatalf("queue = %d, want the item to stay", f.queueLen(t))
	}

	res, err := f.app.W.ApprovalsSvc.Answer(inputAnswer(req.Item.ID, "  "+typedSecret+" "))
	if err != nil || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("answer = %+v, %v", res, err)
	}
	out := wait(t, pending)
	if out.IsError {
		t.Fatalf("result = %s", text(out))
	}
	var got struct {
		Status int
		Body   string
		Inputs []struct {
			Name, Status     string
			ExpiresInSeconds int
		}
	}
	if err := json.Unmarshal([]byte(text(out)), &got); err != nil || got.Status != 200 {
		t.Fatalf("result = %s (%v)", text(out), err)
	}
	if len(got.Inputs) != 1 || got.Inputs[0].Name != "apikey" || got.Inputs[0].Status != "supplied" || got.Inputs[0].ExpiresInSeconds < 3500 {
		t.Fatalf("inputs = %+v, want apikey supplied", got.Inputs)
	}
	if strings.Contains(text(out), typedSecret) || !strings.Contains(got.Body, "{{secret.apikey}}") {
		t.Fatalf("result = %s, want the echoed value masked as its reference", text(out))
	}
	if seen := srv.Requests(); len(seen) != 1 || seen[0].Header.Get("X-Api-Key") != typedSecret {
		t.Fatalf("server saw %+v, want the trimmed typed value", seen)
	}

	e, ok := f.keptValue(t, "apikey")
	if !ok || !e.Secret || e.Source.Kind != keptvars.SourceUser || e.Origin != srv.URL || string(e.Value) != `"`+typedSecret+`"` {
		t.Fatalf("stored = %+v ok=%v, want a secret user entry bound to %s", e, ok, srv.URL)
	}
	list := text(callTool(t, sess, "list", map[string]any{"what": []string{"temp"}}))
	if !strings.Contains(list, `"apikey"`) || !strings.Contains(list, `"secret":true`) || strings.Contains(list, typedSecret) {
		t.Fatalf("listing = %s, want the name and secret flag, no value", list)
	}
	f.noValueAnywhere(t, typedSecret)

	// Held value: the card shows it stored, and approving without typing sends it again.
	again := callToolAsync(sess, "api_request", keyArgs(envID, "{{base_url}}/echo"))
	req2 := f.pending(t)
	var pl2 inputPayload
	if err := json.Unmarshal(req2.Item.Payload, &pl2); err != nil {
		t.Fatal(err)
	}
	if len(pl2.Inputs) != 1 || pl2.Inputs[0].State != "stored" || pl2.Inputs[0].KeptID != e.ID || len(pl2.Kept) != 1 || pl2.Kept[0].Pending {
		t.Fatalf("second payload = %+v, want a stored input with its entry", pl2)
	}
	f.app.Contract(t, "approvals-api-input-stored", "ApprovalsService.List", []approvals.Item{req2.Item},
		append(opts, flowharness.Replace(e.ID, "kv_apikey"))...)
	f.answer(t, true, req2.RequestID)
	if r := wait(t, again); r.IsError || !strings.Contains(text(r), `"reused"`) || strings.Contains(text(r), typedSecret) {
		t.Fatalf("second result = %s, want reused and no value", text(r))
	}
	if seen := srv.Requests(); len(seen) != 2 || seen[1].Header.Get("X-Api-Key") != typedSecret {
		t.Fatalf("server saw %d requests, want the held value sent again", len(seen))
	}
}

// TestUserInputDenied: a deny stores nothing and sends nothing.
func TestUserInputDenied(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)

	pending := callToolAsync(sess, "api_request", keyArgs(envID, "{{base_url}}/echo"))
	req := f.pending(t)
	opts, _ := json.Marshal(map[string]any{"inputs": map[string]string{"apikey": typedSecret}})
	if _, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{
		IDs: []string{req.Item.ID}, Approve: false, Options: map[string]json.RawMessage{req.Item.ID: opts},
	}); err != nil {
		t.Fatal(err)
	}
	if out := wait(t, pending); !out.IsError || strings.Contains(text(out), typedSecret) {
		t.Fatalf("result = %s, want a denial", text(out))
	}
	if _, ok := f.keptValue(t, "apikey"); ok || len(srv.Requests()) != 0 {
		t.Fatalf("kept %v, requests %d, want nothing stored or sent", ok, len(srv.Requests()))
	}
}

// TestUserInputRefusals: an input that cannot work is refused before any card appears.
func TestUserInputRefusals(t *testing.T) {
	f := newFixture(t)
	_, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.keep(t, externalScope, "apikey", `"script-made"`)

	// A name a script kept is not the user's to replace.
	res := callTool(t, sess, "api_request", keyArgs(envID, "{{base_url}}/echo"))
	if !res.IsError || !strings.Contains(text(res), "apikey") {
		t.Fatalf("declared over a script's entry = %q, want a refusal naming it", text(res))
	}
	// A declared input must be used.
	args := apiArgs(envID, "GET", "{{base_url}}/echo")
	args["inputs"] = []string{"unused"}
	if res := callTool(t, sess, "api_request", args); !res.IsError {
		t.Fatalf("unused input = %q, want a refusal", text(res))
	}
	if f.queueLen(t) != 0 {
		t.Fatalf("queue = %d, want no card", f.queueLen(t))
	}
}

// TestKeptVarsUserReveal: Settings lists a user entry with its origin and no value; Reveal asks the gate
// and shows it once confirmed. UI half: tests/ui/studio/kept-variables-user.spec.ts.
func TestKeptVarsUserReveal(t *testing.T) {
	f := newFixture(t)
	f.app.W.Kept.ClearAll()
	m, err := f.app.W.Kept.Put(keptvars.Put{
		Scope: externalScope, Name: "apikey", Value: json.RawMessage(`"` + typedSecret + `"`), Secret: true,
		Origin: "https://api.example.test",
		Source: keptvars.Source{Kind: keptvars.SourceUser, MCP: "api", Tool: "api_request", Request: "GET https://api.example.test/users", Client: "claude-code"},
	})
	if err != nil {
		t.Fatal(err)
	}
	opts := []flowharness.ContractOption{flowharness.Replace(m.ID, "kv_apikey"), flowharness.Mask("createdAt", "expiresAt")}

	list, err := f.app.W.KeptVars.List()
	if err != nil || len(list.Entries) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if e := list.Entries[0]; !e.Secret || e.Preview != "" || e.Origin != "https://api.example.test" || e.Source.Kind != "user" {
		t.Fatalf("entry = %+v, want a secret user entry with its origin and no preview", e)
	}
	if strings.Contains(mustJSON(t, list), typedSecret) {
		t.Fatal("the listing holds the value")
	}
	f.app.Contract(t, "kept-vars-user", "KeptVarsService.List", list, opts...)

	f.app.Contract(t, "kept-vars-user", "args:KeptVarsService.Reveal", bridge.KeptRevealArgs{ID: m.ID}, opts...)
	first, err := f.app.W.KeptVars.Reveal(bridge.KeptRevealArgs{ID: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != "confirmation-required" || first.Value != nil {
		t.Fatalf("first reveal = %+v, want confirmation-required with no value", first)
	}
	f.app.Contract(t, "kept-vars-user", "KeptVarsService.Reveal#confirmation", first, opts...)
	shown, err := f.app.W.KeptVars.Reveal(bridge.KeptRevealArgs{ID: m.ID, Confirmed: true})
	if err != nil || shown.Outcome != "revealed" || shown.Value == nil || *shown.Value != typedSecret {
		t.Fatalf("confirmed reveal = %+v, %v, want the value", shown, err)
	}
	f.app.Contract(t, "kept-vars-user", "KeptVarsService.Reveal", bridge.KeptReveal{Value: shown.Value, Outcome: shown.Outcome}, opts...)
	if _, err := f.app.W.KeptVars.Reveal(bridge.KeptRevealArgs{ID: "kv_missing", Confirmed: true}); err == nil {
		t.Fatal("reveal of a missing id succeeded")
	}
}

// noValueAnywhere fails when the op log holds value.
func (f *fixture) noValueAnywhere(t *testing.T, value string) {
	t.Helper()
	recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mustJSON(t, recent), value) {
		t.Errorf("the op log holds the typed value")
	}
}
