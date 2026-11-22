package dbmcpflow

import (
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apiaccess"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const (
	usersJSON     = `{"users":[{"id":9007199254740993,"email":"a@x.test","name":"Ann"},{"id":2,"email":"b@y.test","name":"Bob"}],"total":2}`
	prepareSource = `return {body: {items: input.ids.map(id => ({id}))}};`
	transformSrc  = `return response.json.users.map(u => ({id: u.id, domain: u.email.split('@')[1]}));`
)

// usersURL is a /raw route answering usersJSON as application/json, whatever the method.
func usersURL(base string) string {
	return base + "/raw?ct=" + url.QueryEscape("application/json") + "&hex=" + hex.EncodeToString([]byte(usersJSON))
}

func scriptArgs(envID, method, u string, script map[string]any) map[string]any {
	args := apiArgs(envID, method, u)
	args["script"] = script
	return args
}

// opRow waits for the finished op row of kind whose command is command.
func (f *fixture) scriptOpRow(t *testing.T, kind, command string) oplog.Record {
	t.Helper()
	var row oplog.Record
	testx.WaitUntil(t, 10*time.Second, func() bool {
		recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range recent {
			if r.Kind == kind && r.Status != "running" && r.Command != nil && *r.Command == command {
				row = r
				return true
			}
		}
		return false
	})
	return row
}

func (f *fixture) scriptOpKinds(t *testing.T) []string {
	t.Helper()
	recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, r := range recent {
		kinds = append(kinds, r.Kind)
	}
	return kinds
}

func hasKind(kinds []string, kind string) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

type scriptItem struct {
	Body   struct{ Text, ContentType string }
	Script struct {
		Prepare *struct {
			Source, Hash   string
			Changed        []string
			Input          string
			InputTruncated bool
		}
		Transform *struct{ Source, Hash string }
	}
}

// TestApiRequestScripted: prepare builds the request the user approves, transform reduces the response,
// and the agent gets only {"output": ...}. UI halves: tests/ui/studio/approvals-api-script.spec.ts and
// tests/ui/operations-api-script.spec.ts.
func TestApiRequestScripted(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)

	args := scriptArgs(envID, "POST", usersURL(srv.URL), map[string]any{
		"prepare": prepareSource, "input": map[string]any{"ids": []int{1, 2}}, "transform": transformSrc,
	})
	pending := callToolAsync(sess, "api_request", args)
	req := f.pending(t)
	it := req.Item
	var pl scriptItem
	if err := json.Unmarshal(it.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	p := pl.Script.Prepare
	if p == nil || p.Source != prepareSource || len(p.Hash) != 64 || strings.Join(p.Changed, ",") != "body,contentType" ||
		p.Input != `{"ids":[1,2]}` || p.InputTruncated {
		t.Fatalf("payload prepare = %+v", p)
	}
	if pl.Script.Transform == nil || pl.Script.Transform.Source != transformSrc || len(pl.Script.Transform.Hash) != 64 {
		t.Fatalf("payload transform = %+v", pl.Script.Transform)
	}
	if pl.Body.Text != `{"items":[{"id":1},{"id":2}]}` || pl.Body.ContentType != "application/json" {
		t.Fatalf("item body = %+v, want the prepared JSON", pl.Body)
	}
	f.app.Contract(t, "approvals-api-script", "ApprovalsService.List", []approvals.Item{it},
		flowharness.Mask("createdAt", "expiresAt", "cwd", "environment.id"), flowharness.Replace(srv.URL, "http://127.0.0.1:<port>"))
	f.app.Contract(t, "approvals-api-script", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{it.ID}, Approve: true})

	f.answer(t, true, req.RequestID)
	res := wait(t, pending)
	const want = `{"output":[{"id":9007199254740993,"domain":"x.test"},{"id":2,"domain":"y.test"}]}`
	if res.IsError || text(res) != want {
		t.Fatalf("result = %s, want %s", text(res), want)
	}
	for _, leak := range []string{"a@x.test", "Ann", "total", "status"} {
		if strings.Contains(text(res), leak) {
			t.Errorf("result leaks %q", leak)
		}
	}
	got := srv.Requests()
	if len(got) != 1 || string(got[0].Body) != `{"items":[{"id":1},{"id":2}]}` || got[0].Header.Get("Content-Type") != "application/json" {
		t.Fatalf("server saw %+v, want the prepared JSON body", got)
	}

	prep := f.scriptOpRow(t, "jsPrepare", prepareSource)
	trans := f.scriptOpRow(t, "jsTransform", transformSrc)
	var httpRow oplog.Record
	testx.WaitUntil(t, 10*time.Second, func() bool {
		recent, _ := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 100})
		for _, r := range recent {
			if r.Kind == "http" && r.Status != "running" {
				httpRow = r
			}
		}
		return httpRow.ID != ""
	})
	for _, r := range []oplog.Record{prep, httpRow, trans} {
		if r.Status != "ok" || r.Source != "mcp" {
			t.Fatalf("op row = %+v, want ok from mcp", r)
		}
	}
	mask := flowharness.Mask("id", "startedAt", "durationMs", "connectionId")
	f.app.Contract(t, "api-script-ops", "OpsService.Recent", []oplog.Record{prep, httpRow, trans}, mask, flowharness.Replace(srv.URL, "http://127.0.0.1:<port>"))
	f.app.Contract(t, "api-script-ops", "result", text(res))
}

// TestApiScriptFailures: every failure gives the agent and the op row the same text.
func TestApiScriptFailures(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.setAPI(t, true, "never")
	u := usersURL(srv.URL)

	type tc struct {
		key, stage, src string
		script          map[string]any
		sent            int
		contains        []string
	}
	cases := []tc{
		{"transformSyntax", "jsTransform", "return (", map[string]any{"transform": "return ("}, 0, []string{"transform script: script syntax error at line", "Nothing was sent."}},
		{"prepareThrow", "jsPrepare", "throw new Error('bad')", map[string]any{"prepare": "throw new Error('bad')"}, 0, []string{"prepare script failed:", "bad", "Nothing was sent."}},
		{"prepareInvalid", "jsPrepare", "return 42", map[string]any{"prepare": "return 42"}, 0, []string{"prepare script returned an invalid request: the script must return an object, not number. Nothing was sent."}},
		{"transformRuntime", "jsTransform", "return null.x", map[string]any{"transform": "return null.x"}, 1, []string{"transform script failed:", "was sent (HTTP 200)"}},
		{"transformTimeout", "jsTransform", "while(true){}", map[string]any{"transform": "while(true){}"}, 1, []string{"transform script failed:", "timeout", "was sent (HTTP 200)"}},
		{"transformOutput", "jsTransform", "return 'x'.repeat(100000)", map[string]any{"transform": "return 'x'.repeat(100000)"}, 1, []string{"transform script failed:", "Return less, or raise maxResponseBytes (max 524288)."}},
	}
	sent := 0
	for _, c := range cases {
		res := callTool(t, sess, "api_request", scriptArgs(envID, "GET", u, c.script))
		sent += c.sent
		msg := text(res)
		if !res.IsError {
			t.Fatalf("%s: result %q, want an error", c.key, msg)
		}
		for _, w := range c.contains {
			if !strings.Contains(msg, w) {
				t.Errorf("%s: text %q lacks %q", c.key, msg, w)
			}
		}
		if got := len(srv.Requests()); got != sent {
			t.Errorf("%s: server saw %d requests, want %d", c.key, got, sent)
		}
		row := f.scriptOpRow(t, c.stage, c.src)
		if row.Status != "error" || row.Error == nil || *row.Error != msg {
			t.Errorf("%s: op row error = %v, want the tool text %q", c.key, row.Error, msg)
		}
		f.app.Contract(t, "api-script-ops", "failure:"+c.key, map[string]any{"kind": c.stage, "command": c.src, "text": msg}, flowharness.Mask("text"))
		f.app.Contract(t, "api-script-ops", "failureText:"+c.key, msg)
	}
	if f.queueLen(t) != 0 {
		t.Fatalf("queue = %d, want no items", f.queueLen(t))
	}

	before := len(f.scriptOpKinds(t))
	res := callTool(t, sess, "api_request", scriptArgs(envID, "GET", u, map[string]any{"input": map[string]any{"a": 1}, "transform": "return 1"}))
	if !res.IsError || text(res) != apiScriptNoPrepText || len(f.scriptOpKinds(t)) != before {
		t.Fatalf("input without prepare = %q, want the refusal and no row", text(res))
	}
}

const apiScriptNoPrepText = "script.input needs script.prepare"

// TestApiScriptApprovalClass: the approval decision reads the prepared request, never the script.
func TestApiScriptApprovalClass(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	rules := []apiaccess.Rule{{Effect: "allow", Hosts: []string{"127.0.0.1"}, Methods: []string{"GET"}}}
	if _, err := f.app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{APIRules: &rules}}}); err != nil {
		t.Fatal(err)
	}
	u := srv.URL + "/echo"

	// A GET the rule allows, turned into a POST by prepare: the item appears.
	pending := callToolAsync(sess, "api_request", scriptArgs(envID, "GET", u, map[string]any{"prepare": `return {method: "POST"}`}))
	req := f.pending(t)
	if req.Item.Kind != approvals.KindAPIRequest {
		t.Fatalf("item = %+v", req.Item)
	}
	f.answer(t, true, req.RequestID)
	if res := wait(t, pending); res.IsError {
		t.Fatalf("approved send = %q", text(res))
	}

	// A POST made a GET by prepare: sent without an item.
	res := callTool(t, sess, "api_request", scriptArgs(envID, "POST", u, map[string]any{"prepare": `return {method: "GET"}`}))
	if res.IsError || f.queueLen(t) != 0 {
		t.Fatalf("GET after prepare = %q, queue %d, want a send with no item", text(res), f.queueLen(t))
	}
	if !hasKind(f.scriptOpKinds(t), "jsPrepare") {
		t.Fatal("no jsPrepare row for a skipped approval")
	}

	// apiApproval never: no item, both rows.
	f.setAPI(t, true, "never")
	post := scriptArgs(envID, "POST", u, map[string]any{"prepare": `return {body: "x"}`, "transform": `return response.status`})
	res = callTool(t, sess, "api_request", post)
	if text(res) != `{"output":200}` || f.queueLen(t) != 0 || !hasKind(f.scriptOpKinds(t), "jsTransform") {
		t.Fatalf("never = %q, queue %d", text(res), f.queueLen(t))
	}

	// Denied: nothing sent, no transform row.
	f.setAPI(t, true, "always")
	sentBefore := len(srv.Requests())
	denyScript := `return response.headers.length`
	pending = callToolAsync(sess, "api_request", scriptArgs(envID, "POST", u, map[string]any{"transform": denyScript}))
	req = f.pending(t)
	f.answer(t, false, req.RequestID)
	if res := wait(t, pending); !res.IsError || !strings.Contains(text(res), "denied") {
		t.Fatalf("denied = %q", text(res))
	}
	if len(srv.Requests()) != sentBefore {
		t.Fatal("a denied request was sent")
	}
	recent, _ := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 100})
	for _, r := range recent {
		if r.Kind == "jsTransform" && r.Command != nil && *r.Command == denyScript {
			t.Fatal("transform ran for a denied request")
		}
	}
}

// TestApiScriptMasking: a secret in the response never reaches the script, its output or its error.
func TestApiScriptMasking(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.setAPI(t, true, "never")
	hdr := func(script map[string]any) map[string]any {
		a := apiArgs(envID, "GET", srv.URL+"/echo", "Authorization", "Bearer {{token}}")
		a["script"] = script
		return a
	}
	ok := callTool(t, sess, "api_request", hdr(map[string]any{"transform": `return response.body`}))
	if ok.IsError || !strings.Contains(text(ok), "{{token}}") {
		t.Fatalf("result = %q, want the placeholder", text(ok))
	}
	bad := callTool(t, sess, "api_request", hdr(map[string]any{"transform": `throw new Error(response.body)`}))
	if !bad.IsError || !strings.Contains(text(bad), "{{token}}") {
		t.Fatalf("error = %q, want the placeholder", text(bad))
	}
	var rows []byte
	testx.WaitUntil(t, 10*time.Second, func() bool {
		recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		rows, _ = json.Marshal(recent)
		for _, r := range recent {
			if r.Status == "running" {
				return false
			}
		}
		return len(recent) >= 4
	})
	list, _ := json.Marshal(f.list(t))
	noSecret(t, "a tool result, the item list or the op log", text(ok), text(bad), string(rows), string(list))
}

// TestApiScriptsToggle: apiScripts off removes script from the schema and refuses it; on again needs no
// restart; the schema then refuses script itself (apiScriptsOff is the backstop). Contract dbmcp-api-settings: tests/ui/studio/dbmcp-api-settings.spec.ts.
func TestApiScriptsToggle(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.setAPI(t, true, "never")
	described := func() (schema bool, suffix bool) {
		list, err := sess.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range list.Tools {
			if tool.Name == "api_request" {
				b, _ := json.Marshal(tool.InputSchema)
				return strings.Contains(string(b), `"script"`), strings.Contains(string(b), `"prepare"`)
			}
		}
		t.Fatal("api_request not listed")
		return
	}
	if s, d := described(); !s || !d {
		t.Fatalf("scripts on: schema %v, description %v", s, d)
	}
	f.patchAPI(t, "scriptsOff", &model.DbMcpPatch{APIScripts: ptr(false)})
	if s, d := described(); s || d {
		t.Fatalf("scripts off: schema %v, description %v", s, d)
	}
	res := callTool(t, sess, "api_request", scriptArgs(envID, "GET", srv.URL+"/echo", map[string]any{"transform": "return 1"}))
	if !res.IsError || !strings.Contains(text(res), "unexpected additional properties") {
		t.Fatalf("scripts off call = %q", text(res))
	}
	f.patchAPI(t, "scriptsOn", &model.DbMcpPatch{APIScripts: ptr(true)})
	if s, d := described(); !s || !d {
		t.Fatalf("scripts back on: schema %v, description %v", s, d)
	}
	res = callTool(t, sess, "api_request", scriptArgs(envID, "GET", srv.URL+"/echo", map[string]any{"transform": "return 1"}))
	if text(res) != `{"output":1}` {
		t.Fatalf("scripts on call = %q", text(res))
	}
}
