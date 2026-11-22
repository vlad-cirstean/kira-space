package dbmcpflow

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/approvals-api-kept-script.spec.ts (contract approvals-api-kept-script) and
// of the "Will keep" block of the review (contract approvals-api-kept-review).

const (
	keepPrepare   = `const region = plain.region; return {headers: [{name: "X-Region", value: region}]};`
	keepTransform = `plain.ids = response.json.users.map(u => u.id); return {count: response.json.total};`
	// keptIDs is what keepTransform keeps: the first id is above 2^53, so it proves the value stays exact.
	keptIDs = `[9007199254740993,2]`
)

var expiresIn = regexp.MustCompile(`"expiresInSeconds":\d+`)

type keptScriptItem struct {
	Kept []struct {
		ID, Name, Type, Preview string
		Sites                   []struct {
			Field, Part string
			From, To    int
		}
	}
	Script struct {
		Prepare   struct{ Source string }
		Transform struct {
			Source  string
			MayKeep []struct {
				Name  string
				Sites []struct {
					Field, Part string
					From, To    int
				}
			}
		}
	}
}

func (f *fixture) keptValue(t *testing.T, name string) (keptvars.Entry, bool) {
	t.Helper()
	return f.app.W.Kept.Get(externalScope, name)
}

// TestKeptVarsFromScript: a script reads a kept value and keeps another; the card marks both calls, the
// agent gets a summary without the value, and the next request uses it.
func TestKeptVarsFromScript(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	region := f.keep(t, externalScope, "region", `"eu"`)

	args := scriptArgs(envID, "GET", usersURL(srv.URL), map[string]any{"prepare": keepPrepare, "transform": keepTransform})
	pending := callToolAsync(sess, "api_request", args)
	req := f.pending(t)
	var pl keptScriptItem
	if err := json.Unmarshal(req.Item.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	if len(pl.Kept) != 1 || pl.Kept[0].ID != region.ID || pl.Kept[0].Name != "region" || pl.Kept[0].Preview != `"eu"` {
		t.Fatalf("kept = %+v, want the region read", pl.Kept)
	}
	if s := pl.Kept[0].Sites; len(s) != 1 || s[0].Field != "script" || s[0].Part != "prepare" || keepPrepare[s[0].From:s[0].To] != `plain.region` {
		t.Fatalf("read sites = %+v, want the prepare call", s)
	}
	mk := pl.Script.Transform.MayKeep
	if len(mk) != 1 || mk[0].Name != "ids" || len(mk[0].Sites) != 1 || mk[0].Sites[0].Part != "transform" ||
		keepTransform[mk[0].Sites[0].From:mk[0].Sites[0].To] != `plain.ids` {
		t.Fatalf("mayKeep = %+v, want ids with its transform site", mk)
	}
	if _, ok := f.keptValue(t, "ids"); ok {
		t.Fatal("ids stored before the response was final")
	}
	opts := []flowharness.ContractOption{
		flowharness.Mask("createdAt", "expiresAt", "cwd", "environment.id"),
		flowharness.Replace(region.ID, "kv_region"), flowharness.Replace(srv.URL, "http://127.0.0.1:<port>"),
	}
	f.app.Contract(t, "approvals-api-kept-script", "ApprovalsService.List", []approvals.Item{req.Item}, opts...)
	f.app.Contract(t, "approvals-api-kept-script", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})

	f.answer(t, true, req.RequestID)
	res := wait(t, pending)
	var got struct {
		Output struct{ Count int }
		Saved  []struct {
			Name, Type       string
			Items            int
			ExpiresInSeconds int
		}
	}
	if err := json.Unmarshal([]byte(text(res)), &got); err != nil || res.IsError {
		t.Fatalf("result = %s (%v)", text(res), err)
	}
	if got.Output.Count != 2 || len(got.Saved) != 1 || got.Saved[0].Name != "ids" || got.Saved[0].Type != "array" || got.Saved[0].Items != 2 || got.Saved[0].ExpiresInSeconds < 3590 {
		t.Fatalf("result = %s, want the count and a summary of ids", text(res))
	}
	for _, leak := range []string{"9007199254740993", "a@x.test"} {
		if strings.Contains(text(res), leak) {
			t.Errorf("result leaks %q", leak)
		}
	}
	f.app.Contract(t, "approvals-api-kept-script", "result", expiresIn.ReplaceAllString(text(res), `"expiresInSeconds":0`))
	e, ok := f.keptValue(t, "ids")
	if !ok || string(e.Value) != keptIDs || e.Source.Tool != "api_request" || e.Source.MCP != "api" || !strings.HasPrefix(e.Source.Request, "GET ") {
		t.Fatalf("stored = %+v ok=%v, want the exact ids with this request as source", e, ok)
	}
	if first := srv.Requests(); len(first) != 1 || first[0].Header.Get("X-Region") != "eu" {
		t.Fatalf("server saw %+v, want X-Region eu from plain.region", first)
	}

	next := callToolAsync(sess, "api_request", jsonPost(envID, "{{base_url}}/echo", `{"ids": {{plain.ids}}}`))
	f.answer(t, true, f.pending(t).RequestID)
	if r := wait(t, next); r.IsError {
		t.Fatalf("chained request = %s", text(r))
	}
	if all := srv.Requests(); len(all) != 2 || string(all[1].Body) != `{"ids": `+keptIDs+`}` {
		t.Fatalf("server saw %+v, want the kept ids inserted", all)
	}
}

// TestKeptScriptUndeclared: a name the source does not write as a literal is refused, however the object is
// reached. Prepare may write too: its values interpolate into the same request.
func TestKeptScriptUndeclared(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.keep(t, externalScope, "region", `"eu"`)
	f.keep(t, externalScope, "other", `"x"`)

	run := func(script map[string]any) string {
		t.Helper()
		pending := callToolAsync(sess, "api_request", scriptArgs(envID, "GET", usersURL(srv.URL), script))
		it := f.pending(t)
		f.answer(t, true, it.RequestID)
		return text(wait(t, pending))
	}
	if got := run(map[string]any{"transform": `plain.region; const k = plain; const n = "oth" + "er"; return k[n];`}); got != `{"output":null}` {
		t.Fatalf("aliased get = %q, want null: a name the source does not write is not loaded", got)
	}
	if got := run(map[string]any{"transform": `plain.a = 1; const k = plain; k["b"] = 2; const n = "c"; k[n] = 3; return 1;`}); !strings.Contains(got, "not written literally") {
		t.Fatalf("aliased set = %q, want the not-literal refusal", got)
	}
	if _, ok := f.keptValue(t, "a"); ok {
		t.Error("a failed transform saved a value")
	}
}

// TestKeptScriptNoSmuggledReference: a kept value that holds {{token}} is not turned into a reference by
// the prepare script that copies it into the request.
func TestKeptScriptNoSmuggledReference(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.keep(t, externalScope, "evil", `"Bearer {{token}}"`)

	res := callTool(t, sess, "api_request", scriptArgs(envID, "GET", usersURL(srv.URL),
		map[string]any{"prepare": `return {headers: [{name: "Authorization", value: plain.evil}]};`}))
	if !res.IsError || !strings.Contains(text(res), "references built from saved data are not resolved") || strings.Contains(text(res), "token") {
		t.Fatalf("result = %q, want a refusal that does not quote the text", text(res))
	}
	if f.queueLen(t) != 0 || len(srv.Requests()) != 0 {
		t.Fatalf("queue %d, server %d requests, want nothing", f.queueLen(t), len(srv.Requests()))
	}
}

// TestKeptScriptSecret: a script cannot read a saved secret, in prepare or transform; what it writes as
// secret.<name> is saved as a secret and never shown.
func TestKeptScriptSecret(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	const secret = "s3cr3t-kept-value"
	if _, err := f.app.W.Kept.Put(keptvars.Put{Scope: externalScope, Name: "tok", Value: json.RawMessage(`"` + secret + `"`), Secret: true, Source: keptSource}); err != nil {
		t.Fatal(err)
	}

	for name, script := range map[string]map[string]any{
		"prepare":   {"prepare": `return {headers: [{name: "X-T", value: plain.tok}]};`},
		"transform": {"transform": `plain.copy = plain.tok; return 1;`},
	} {
		res := callTool(t, sess, "api_request", scriptArgs(envID, "GET", usersURL(srv.URL), script))
		if !res.IsError || !strings.Contains(text(res), "is secret") || strings.Contains(text(res), secret) || f.queueLen(t) != 0 {
			t.Fatalf("%s read = %q, want a refusal without the value", name, text(res))
		}
	}

	pending := callToolAsync(sess, "api_request", scriptArgs(envID, "GET", usersURL(srv.URL),
		map[string]any{"transform": `secret.copy = response.json.users[0].email; return {count: response.json.total};`}))
	f.answer(t, true, f.pending(t).RequestID)
	out := wait(t, pending)
	if out.IsError || strings.Contains(text(out), "a@x.test") {
		t.Fatalf("result = %q, want the secret value absent", text(out))
	}
	e, ok := f.keptValue(t, "copy")
	if !ok || !e.Secret {
		t.Fatalf("copy = %+v ok=%v, want a secret entry", e.Meta, ok)
	}
}

// TestKeptScriptReadGoneWhileWaiting: a variable a script reads is watched like a request's own.
func TestKeptScriptReadGoneWhileWaiting(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	region := f.keep(t, externalScope, "region", `"eu"`)

	pending := callToolAsync(sess, "api_request", scriptArgs(envID, "GET", usersURL(srv.URL), map[string]any{"prepare": keepPrepare}))
	f.items(t, 1)
	if _, err := f.app.W.KeptVars.Clear(bridge.KeptClearArgs{IDs: []string{region.ID}}); err != nil {
		t.Fatal(err)
	}
	out := wait(t, pending)
	if !out.IsError || !strings.Contains(text(out), `"region"`) || !strings.Contains(text(out), "cleared by the user") {
		t.Fatalf("result = %q, want a cleared-while-waiting error naming region", text(out))
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
	if len(srv.Requests()) != 0 {
		t.Fatalf("server saw %d requests, want none", len(srv.Requests()))
	}
}

// TestKeptScriptReviewHeld: with the response held, the review lists what will be kept and nothing is
// stored until the user sends; a withheld response keeps nothing.
func TestKeptScriptReviewHeld(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setReviewApp(t, true)
	f.keep(t, externalScope, "region", `"eu"`)
	opts := []flowharness.ContractOption{flowharness.Mask("createdAt", "expiresAt", "cwd", "environment.id"), flowharness.Replace(srv.URL, "http://127.0.0.1:<port>")}
	args := func() map[string]any {
		return scriptArgs(envID, "GET", usersURL(srv.URL), map[string]any{"prepare": keepPrepare, "transform": keepTransform})
	}

	denied := callToolAsync(sess, "api_request", args())
	rq := f.pending(t)
	if held := f.pendingResponse(t, rq.RequestID); len(held.Keep) != 1 || held.Keep[0].Name != "ids" || held.Keep[0].Preview != keptIDs ||
		held.Keep[0].Type != "array" || held.Keep[0].Items != 2 || held.Keep[0].Secret {
		t.Fatalf("held keep = %+v, want the ids preview", held.Keep)
	}
	f.answer(t, false, rq.RequestID)
	if out := wait(t, denied); !out.IsError || strings.Contains(text(out), "saved") {
		t.Fatalf("withheld result = %q, want the withhold text only", text(out))
	}
	if _, ok := f.keptValue(t, "ids"); ok {
		t.Fatal("a withheld response kept a value")
	}

	sent := callToolAsync(sess, "api_request", args())
	rq = f.pending(t)
	held := f.pendingResponse(t, rq.RequestID)
	f.app.Contract(t, "approvals-api-kept-review", "ApprovalsService.List", []approvals.Item{rq.Item}, opts...)
	held.Text, held.Raw = stable(held.Text), stable(held.Raw)
	f.app.Contract(t, "approvals-api-kept-review", "DbMcpService.PendingResponse", held, opts...)
	f.app.Contract(t, "approvals-api-kept-review", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{rq.Item.ID}, Approve: true})
	if _, ok := f.keptValue(t, "ids"); ok {
		t.Fatal("ids stored while the response was held")
	}
	f.answer(t, true, rq.RequestID)
	out := wait(t, sent)
	if out.IsError || !strings.Contains(text(out), `"saved":[{"name":"ids"`) || strings.Contains(text(out), "9007199254740993") {
		t.Fatalf("result = %q, want the count and the kept summary", text(out))
	}
	if e, ok := f.keptValue(t, "ids"); !ok || string(e.Value) != keptIDs {
		t.Fatalf("stored = %s ok=%v, want the ids after the send", e.Value, ok)
	}
}
