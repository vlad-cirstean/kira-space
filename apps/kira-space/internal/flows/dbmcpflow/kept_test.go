package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// The test client connects through the external server, whose one kept scope is "external".
const externalScope = "external"

var keptSource = keptvars.Source{
	Kind: keptvars.SourceScript, MCP: "api", Tool: "api_request", Request: "GET {{base_url}}/users", Client: "claude-code",
}

// keep stores a value as an earlier request would have.
func (f *fixture) keep(t *testing.T, scope, name, value string) keptvars.Meta {
	t.Helper()
	m, err := f.app.W.Kept.Put(keptvars.Put{Scope: scope, Name: name, Value: json.RawMessage(value), Source: keptSource})
	if err != nil {
		t.Fatalf("keep %s: %v", name, err)
	}
	return m
}

func jsonPost(envID, url, body string) map[string]any {
	args := apiArgs(envID, "POST", url)
	args["body"] = body
	args["contentType"] = "application/json"
	return args
}

// TestKeptVarsChainedRequest: a request names a kept list in its JSON body; the card lists the variable
// and where it is read, and the send inserts the value. A server that echoes the body back shows the
// value to the agent: only the request itself is template-only (see Known open items).
// UI half: tests/ui/studio/approvals-api-kept.spec.ts.
func TestKeptVarsChainedRequest(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	meta := f.keep(t, externalScope, "ids", "[11,12,13]")

	pending := callToolAsync(sess, "api_request", jsonPost(envID, "{{base_url}}/echo", `{"ids": {{plain.ids}}, "n": 3}`))
	req := f.pending(t)
	var pl struct {
		Body struct{ Text string }
		Kept []struct {
			ID, Name, Type, Preview string
			Items                   int
			Sites                   []struct {
				Field    string
				From, To int
			}
		}
	}
	if err := json.Unmarshal(req.Item.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	if len(pl.Kept) != 1 || pl.Kept[0].ID != meta.ID || pl.Kept[0].Type != "array" || pl.Kept[0].Items != 3 || pl.Kept[0].Preview != "[11,12,13]" {
		t.Fatalf("kept = %+v, want the ids list", pl.Kept)
	}
	site := pl.Kept[0].Sites
	if len(site) != 1 || site[0].Field != "body" || pl.Body.Text[site[0].From:site[0].To] != "{{plain.ids}}" {
		t.Fatalf("sites = %+v in %q, want the body span", site, pl.Body.Text)
	}
	f.app.Contract(t, "approvals-api-kept", "ApprovalsService.List", []approvals.Item{req.Item},
		flowharness.Mask("createdAt", "expiresAt", "cwd", "environment.id"),
		flowharness.Replace(meta.ID, "kv_ids"), flowharness.Replace(srv.URL, "http://127.0.0.1:<port>"))
	f.app.Contract(t, "approvals-api-kept", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})

	f.answer(t, true, req.RequestID)
	res := wait(t, pending)
	if out := decodeResult[apiResult](t, res); out.Status != 200 {
		t.Fatalf("result = %s, want 200", text(res))
	}
	got := srv.Requests()
	if len(got) != 1 || string(got[0].Body) != `{"ids": [11,12,13], "n": 3}` {
		t.Fatalf("server saw %+v, want the list inserted as JSON", got)
	}
	var row string
	testx.WaitUntil(t, 10*time.Second, func() bool {
		recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range recent {
			if r.Kind == "http" && r.Status != "running" && r.Command != nil {
				row = *r.Command
			}
		}
		return row != ""
	})
	if strings.Contains(row, "11,12,13") || strings.Contains(row, "kept") {
		t.Errorf("op log command = %q, want no kept value", row)
	}
}

// TestKeptVarsGoneWhileWaiting: clearing a variable while its request waits withdraws the card and stops
// the send.
func TestKeptVarsGoneWhileWaiting(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	meta := f.keep(t, externalScope, "ids", "[1]")

	pending := callToolAsync(sess, "api_request", jsonPost(envID, "{{base_url}}/echo", `{"ids": {{plain.ids}}}`))
	f.items(t, 1)
	if _, err := f.app.W.KeptVars.Clear(bridge.KeptClearArgs{IDs: []string{meta.ID}}); err != nil {
		t.Fatal(err)
	}
	out := wait(t, pending)
	if !out.IsError || !strings.Contains(text(out), `"ids"`) || !strings.Contains(text(out), "cleared by the user") {
		t.Fatalf("result = %q, want a cleared-while-waiting error naming ids", text(out))
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
	if len(srv.Requests()) != 0 {
		t.Fatalf("server saw %d requests, want none", len(srv.Requests()))
	}
}

// TestKeptVarsRefusals: references that cannot be used are refused before any card appears.
func TestKeptVarsRefusals(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.keep(t, externalScope, "ids", "[1,2]")
	f.keep(t, externalScope, "word", `"a"`)
	f.keep(t, externalScope, "multi", `"a\nb"`)

	post := func(url, body string, headers ...string) map[string]any {
		args := jsonPost(envID, url, body)
		if len(headers) > 0 {
			args["headers"] = []map[string]string{{"name": headers[0], "value": headers[1]}}
		}
		return args
	}
	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"unknown":       {post("{{base_url}}/echo", `{"a": {{plain.nope}}}`), `no saved variable "nope"`},
		"host":          {apiArgs(envID, "GET", "http://{{plain.word}}/echo"), "scheme, host or port"},
		"list in query": {apiArgs(envID, "GET", "{{base_url}}/echo?x={{plain.ids}}"), "is a list"},
		"quoted":        {post("{{base_url}}/echo", `{"a": "{{plain.word}}"}`), "without quotes"},
		"line break":    {post("{{base_url}}/echo", `{}`, "X-A", "{{plain.multi}}"), "line break"},
		"pipeline":      {apiArgs(envID, "GET", "{{base_url}}/echo?x={{plain.word | base64}}"), "no transforms"},
	} {
		res := callTool(t, sess, "api_request", c.args)
		if !res.IsError || !strings.Contains(text(res), c.want) {
			t.Errorf("%s = %q, want an error containing %q", name, text(res), c.want)
		}
	}
	if f.queueLen(t) != 0 || len(srv.Requests()) != 0 {
		t.Fatalf("queue %d, requests %d, want none", f.queueLen(t), len(srv.Requests()))
	}
}

// TestKeptVarsScope: a variable of another scope is not found, and the listing shows only the caller's.
func TestKeptVarsScope(t *testing.T) {
	f := newFixture(t)
	_, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.keep(t, "session:other", "secretive", `"x"`)
	f.keep(t, externalScope, "mine", `"y"`)

	res := callTool(t, sess, "api_request", apiArgs(envID, "GET", "{{base_url}}/echo?x={{secret.secretive}}"))
	if !res.IsError || !strings.Contains(text(res), `no saved variable "secretive"`) {
		t.Fatalf("other scope = %q, want not found", text(res))
	}
	list := text(callTool(t, sess, "list_saved_variables", map[string]any{}))
	if !strings.Contains(list, `"mine"`) || strings.Contains(list, "secretive") {
		t.Fatalf("list = %s, want only the caller's variable", list)
	}
}

// TestKeptVarsListTool: the listing names, types and remaining time, never a value or the request.
func TestKeptVarsListTool(t *testing.T) {
	f := newFixture(t)
	_, sess, _ := f.apiSetup(t, "127.0.0.1", false)
	f.keep(t, externalScope, "ids", "[11,12,13]")

	res := callTool(t, sess, "list_saved_variables", map[string]any{})
	var out struct {
		Variables []struct {
			Name             string
			Type             string
			Items            int
			ExpiresInSeconds int
			Source           struct{ MCP, Tool string }
		}
	}
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Variables) != 1 || out.Variables[0].Name != "ids" || out.Variables[0].Type != "array" || out.Variables[0].Items != 3 ||
		out.Variables[0].ExpiresInSeconds < 3500 || out.Variables[0].Source.MCP != "api" {
		t.Fatalf("variables = %+v", out.Variables)
	}
	for _, leak := range []string{"11,12,13", "base_url", "claude-code"} {
		if strings.Contains(text(res), leak) {
			t.Errorf("the listing holds %q: %s", leak, text(res))
		}
	}
}

// TestKeptVarsAllowRule: kept use does not change the approval decision; auto-approved reads still skip the card.
func TestKeptVarsAllowRule(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", true)
	f.keep(t, externalScope, "page", "2")

	res := callTool(t, sess, "api_request", apiArgs(envID, "GET", "{{base_url}}/echo?page={{plain.page}}"))
	if out := decodeResult[apiResult](t, res); out.Status != 200 || f.queueLen(t) != 0 {
		t.Fatalf("auto-approved read = %s, queue %d, want a send with no card", text(res), f.queueLen(t))
	}
	if got := srv.Requests(); len(got) != 1 || got[0].Query != "page=2" {
		t.Fatalf("server saw %+v, want page=2", got)
	}
}

// TestKeptVarsService: the Settings view lists with previews, reads a full value, refuses a secret and
// clears. UI half: tests/ui/studio/kept-variables.spec.ts.
func TestKeptVarsService(t *testing.T) {
	f := newFixture(t)
	f.app.W.Kept.ClearAll()
	long := make([]int, 800)
	for i := range long {
		long[i] = 100000 + i
	}
	longJSON, _ := json.Marshal(long)
	ids := f.keep(t, externalScope, "ids", string(longJSON))
	word := f.keep(t, "session:abc", "word", `"hello"`)
	sec, err := f.app.W.Kept.Put(keptvars.Put{
		Scope: externalScope, Name: "token", Value: json.RawMessage(`"tok-plain"`), Secret: true,
		Source: keptvars.Source{Kind: keptvars.SourceUser, MCP: "api", Tool: "api_request", Request: "input token", Client: "claude-code"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids2 := []keptvars.Meta{ids, word, sec}
	opts := []flowharness.ContractOption{flowharness.Replace(ids.ID, "kv_ids"), flowharness.Replace(word.ID, "kv_word"), flowharness.Replace(sec.ID, "kv_token")}

	list, err := f.app.W.KeptVars.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Entries) != len(ids2) || list.RetentionMinutes != 60 {
		t.Fatalf("list = %+v, want three entries and 60 minutes", list)
	}
	var sawLong, sawSecret bool
	for _, e := range list.Entries {
		switch e.Name {
		case "ids":
			sawLong = e.PreviewTruncated && e.Items == 800 && e.ScopeLabel == "External clients"
		case "token":
			sawSecret = e.Secret && e.Preview == ""
		}
	}
	if !sawLong || !sawSecret {
		t.Fatalf("entries = %+v, want a truncated list preview and a secret with none", list.Entries)
	}
	if strings.Contains(mustJSON(t, list), "tok-plain") {
		t.Fatal("the listing holds a secret value")
	}
	f.app.Contract(t, "kept-vars", "KeptVarsService.List", list, opts...)

	full, err := f.app.W.KeptVars.Value(bridge.KeptValueArgs{ID: ids.ID})
	if err != nil || full.JSON != string(longJSON) || full.Type != "array" {
		t.Fatalf("Value = %+v, %v, want the whole list", full.Type, err)
	}
	f.app.Contract(t, "kept-vars", "KeptVarsService.Value", bridge.KeptValue{JSON: `["a"]`, Type: "array"})
	if _, err := f.app.W.KeptVars.Value(bridge.KeptValueArgs{ID: sec.ID}); err == nil {
		t.Fatal("Value of a secret succeeded")
	}
	if _, err := f.app.W.KeptVars.Value(bridge.KeptValueArgs{ID: "kv_missing"}); err == nil {
		t.Fatal("Value of a missing id succeeded")
	}

	f.app.Contract(t, "kept-vars", "args:KeptVarsService.Clear", bridge.KeptClearArgs{IDs: []string{word.ID}}, opts...)
	if n, err := f.app.W.KeptVars.Clear(bridge.KeptClearArgs{IDs: []string{word.ID}}); err != nil || n != 1 {
		t.Fatalf("Clear = %d, %v, want 1", n, err)
	}
	if n, err := f.app.W.KeptVars.ClearAll(); err != nil || n != 2 {
		t.Fatalf("ClearAll = %d, %v, want 2", n, err)
	}
	if left, _ := f.app.W.KeptVars.List(); len(left.Entries) != 0 {
		t.Fatalf("entries after ClearAll = %+v", left.Entries)
	}
}

// TestKeptVarsRetentionSetting: the retention leaf is read per Put and validated.
func TestKeptVarsRetentionSetting(t *testing.T) {
	f := newFixture(t)
	set := func(m int) error {
		_, err := f.app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{KeptRetentionMinutes: &m}}})
		return err
	}
	if err := set(4); err == nil {
		t.Fatal("retention 4 accepted, want a range error")
	}
	if err := set(241); err == nil {
		t.Fatal("retention 241 accepted, want a range error")
	}
	if err := set(10); err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, "kept-vars", "args:SettingsService.Set#retention", bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{KeptRetentionMinutes: ptr(10)}}})
	m := f.keep(t, externalScope, "short", "1")
	if got := m.ExpiresAt.Sub(m.CreatedAt); got != 10*time.Minute {
		t.Fatalf("ttl = %v, want 10m", got)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
