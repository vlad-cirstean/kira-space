package dbmcpflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness/servers"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const (
	apiSecret = "tok-9f2c1d7a-plain"
	devFile   = "dev.environment.yaml"
)

type apiResult struct {
	Status    int    `json:"status"`
	Body      string `json:"body"`
	Binary    bool   `json:"binary"`
	BodyBytes int    `json:"bodyBytes"`
	Truncated bool   `json:"truncated"`
	FinalURL  string `json:"finalUrl"`
}

// apiEnv writes the "Dev" environment (token limited to hosts) and the secrets, and turns the tools on.
func (f *fixture) apiEnv(t *testing.T, hosts string, autoReads bool) (envID string) {
	t.Helper()
	f.writeDev(t, hosts, autoReads)
	for _, name := range []string{"dev.token", "dev.other"} {
		if _, err := f.app.W.ApiConfig.SetSecret(bridge.ApiSecretArgs{Name: name, Value: apiSecret}); err != nil {
			t.Fatal(err)
		}
	}
	f.setAPI(t, true, "always")
	envs, err := f.app.W.ApiFiles.EnvironmentList()
	if err != nil || len(envs) != 1 {
		t.Fatalf("EnvironmentList = %+v, %v", envs, err)
	}
	return envs[0].ID
}

func (f *fixture) writeDev(t *testing.T, hosts string, autoReads bool) {
	t.Helper()
	text := "name: Dev\ncolor: green\nagentAutoApproveReads: " + fmt.Sprint(autoReads) + "\nvariables:\n" +
		"  - name: base_url\n    value: BASE\n    agent: true\n" +
		"  - name: token\n    secret: ${secret:dev.token}\n    agent: true\n    agentHosts: [" + hosts + "]\n" +
		"  - name: other\n    secret: ${secret:dev.other}\n"
	f.app.WriteAPIFile(t, devFile, []byte(strings.Replace(text, "BASE", f.base, 1)))
}

// patchAPI applies one API setting alone, as the settings pane does, and records the call as the
// dbmcp-api-settings contract tests/ui/dbmcp-api-settings.spec.ts replays.
func (f *fixture) patchAPI(t *testing.T, key string, patch *model.DbMcpPatch) {
	t.Helper()
	args := bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: patch}}
	if _, err := f.app.W.Settings.Set(args); err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, "dbmcp-api-settings", "args:SettingsService.Set#"+key, args)
}

func (f *fixture) setAPI(t *testing.T, enabled bool, approval string) {
	t.Helper()
	patch := &model.DbMcpPatch{APIRequests: &enabled, APIApproval: &approval}
	if _, err := f.app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: patch}}); err != nil {
		t.Fatal(err)
	}
}

func apiArgs(envID, method, url string, headers ...string) map[string]any {
	args := map[string]any{"environmentId": envID, "method": method, "url": url}
	var hs []map[string]string
	for i := 0; i+1 < len(headers); i += 2 {
		hs = append(hs, map[string]string{"name": headers[i], "value": headers[i+1]})
	}
	if hs != nil {
		args["headers"] = hs
	}
	return args
}

func (f *fixture) apiSetup(t *testing.T, hosts string, autoReads bool) (*servers.HTTPServer, *mcp.ClientSession, string) {
	t.Helper()
	srv := flowharness.HTTP(t)
	f.base = srv.URL
	envID := f.apiEnv(t, hosts, autoReads)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return srv, sess, envID
}

func noSecret(t *testing.T, what string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(p, apiSecret) {
			t.Errorf("%s holds the secret value", what)
		}
	}
}

// TestApiRequestApproved: an api_request waits as an api.request item, sends with the credential, and
// shows the agent only the placeholder. UI half: tests/ui/approvals-api.spec.ts.
func TestApiRequestApproved(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)

	args := apiArgs(envID, "GET", "{{base_url}}/echo", "Authorization", "Bearer {{token}}")
	pending := callToolAsync(sess, "api_request", args)
	req := f.pending(t)
	it := req.Item
	if it.Kind != approvals.KindAPIRequest || it.Target.Name != "127.0.0.1" || it.Operation.Tone != "neutral" {
		t.Fatalf("pending = %+v, want a neutral api.request item for 127.0.0.1", it)
	}
	var pl struct {
		Headers     []struct{ Name, Value string }
		Credentials []struct{ Name string }
	}
	if err := json.Unmarshal(it.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	if len(pl.Headers) != 1 || pl.Headers[0].Value != "Bearer {{token}}" || len(pl.Credentials) != 1 || pl.Credentials[0].Name != "token" {
		t.Fatalf("payload = %s, want the placeholder header and the token credential", it.Payload)
	}
	listJSON, _ := json.Marshal(f.list(t))
	f.app.Contract(t, "approvals-api", "ApprovalsService.List", []approvals.Item{it}, flowharness.Mask("createdAt", "expiresAt", "cwd", "environment.id"), flowharness.Replace(srv.URL, "http://127.0.0.1:<port>"))
	f.app.Contract(t, "approvals-api", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{it.ID}, Approve: true})

	f.answer(t, true, req.RequestID)
	res := wait(t, pending)
	out := decodeResult[apiResult](t, res)
	if out.Status != 200 || !strings.Contains(out.Body, "Bearer {{token}}") {
		t.Fatalf("result = %s, want 200 echoing the placeholder", text(res))
	}
	if got := srv.Requests(); len(got) != 1 || got[0].Header.Get("Authorization") != "Bearer "+apiSecret {
		t.Fatalf("server saw %+v, want one request with the real credential", got)
	}

	// The op log is written by an async consumer: wait for the row to leave "running".
	var row *model.OpRecord
	var rowsJSON []byte
	testx.WaitUntil(t, 10*time.Second, func() bool {
		recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		rowsJSON, _ = json.Marshal(recent)
		row = nil
		for i := range recent {
			if recent[i].Kind == "http" && recent[i].Status != "running" {
				row = &recent[i]
			}
		}
		return row != nil
	})
	if row == nil || row.Source != "mcp" || row.Command == nil || !strings.HasPrefix(*row.Command, "GET "+srv.URL+"/echo") {
		t.Fatalf("op rows = %s, want one mcp http row", rowsJSON)
	}
	var history int
	if err := f.app.DB().QueryRow("SELECT COUNT(*) FROM api_response_history").Scan(&history); err != nil || history != 0 {
		t.Fatalf("response history rows = %d, %v, want 0", history, err)
	}
	noSecret(t, "the tool result, item list or op log", text(res), string(listJSON), string(rowsJSON))
}

func TestApiRequestBlockedHost(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)

	res := callTool(t, sess, "api_request", apiArgs(envID, "GET", srv.AltURL+"/echo", "Authorization", "Bearer {{token}}"))
	if !res.IsError || !strings.Contains(text(res), `"token"`) || !strings.Contains(text(res), "localhost") {
		t.Fatalf("blocked host = %q, want an error naming token and localhost", text(res))
	}
	if f.queueLen(t) != 0 || len(srv.Requests()) != 0 {
		t.Fatalf("queue %d, requests %d, want none", f.queueLen(t), len(srv.Requests()))
	}
}

func TestApiRequestRedirectBlocked(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.setAPI(t, true, "never")

	url := srv.URL + "/redirect?to=" + srv.AltURL + "/echo"
	res := callTool(t, sess, "api_request", apiArgs(envID, "GET", url, "Authorization", "Bearer {{token}}"))
	if !res.IsError || !strings.Contains(text(res), "redirect to localhost") || !strings.Contains(text(res), "blocked") {
		t.Fatalf("redirect = %q, want a blocked redirect to localhost", text(res))
	}
	if got := srv.Requests(); len(got) != 1 {
		t.Fatalf("server saw %d requests, want the first hop only", len(got))
	}
	noSecret(t, "the error", text(res))
}

func TestApiRequestRedirectNoCredential(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.setAPI(t, true, "never")

	res := callTool(t, sess, "api_request", apiArgs(envID, "GET", srv.URL+"/redirect?to="+srv.AltURL+"/echo"))
	if out := decodeResult[apiResult](t, res); out.Status != 200 || len(srv.Requests()) != 2 {
		t.Fatalf("result = %s, requests %d, want 200 after the redirect", text(res), len(srv.Requests()))
	}
}

func TestApiRequestUnavailable(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)

	other := callTool(t, sess, "api_request", apiArgs(envID, "GET", "{{base_url}}/echo", "X-A", "{{other}}"))
	missing := callTool(t, sess, "api_request", apiArgs(envID, "GET", "{{base_url}}/echo", "X-A", "{{missing}}"))
	if !other.IsError || !missing.IsError || !strings.Contains(text(other), "not available to agents") {
		t.Fatalf("other = %q, missing = %q, want the not-available error", text(other), text(missing))
	}
	if strings.ReplaceAll(text(other), "other", "x") != strings.ReplaceAll(text(missing), "missing", "x") {
		t.Errorf("an unavailable and a nonexistent variable differ: %q vs %q", text(other), text(missing))
	}
	dyn := callTool(t, sess, "api_request", apiArgs(envID, "GET", "{{base_url}}/echo", "X-A", "{{$uuid}}"))
	if !dyn.IsError || !strings.Contains(text(dyn), "dynamic values are not supported") {
		t.Fatalf("dynamic = %q, want the dynamic refusal", text(dyn))
	}
	if f.queueLen(t) != 0 || len(srv.Requests()) != 0 {
		t.Fatal("a refused request raised an item or reached the server")
	}
}

func TestApiRequestDeniedAndReplanned(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	args := apiArgs(envID, "GET", "{{base_url}}/echo", "Authorization", "Bearer {{token}}")

	pending := callToolAsync(sess, "api_request", args)
	f.answer(t, false, f.pending(t).RequestID)
	if out := wait(t, pending); !out.IsError || !strings.Contains(text(out), "denied by the user") {
		t.Fatalf("denied = %q, want a denial", text(out))
	}

	pending = callToolAsync(sess, "api_request", args)
	req := f.pending(t)
	f.writeDev(t, "example.com", false)
	f.answer(t, true, req.RequestID)
	out := wait(t, pending)
	if !out.IsError || !strings.Contains(text(out), "cannot be sent") {
		t.Fatalf("after the host edit = %q, want a blocked error", text(out))
	}
	if len(srv.Requests()) != 0 {
		t.Fatalf("server saw %d requests, want none", len(srv.Requests()))
	}
}

// TestApiRequestRefusedWhenVariableChanges: a variable edited while the request waits stops it.
func TestApiRequestRefusedWhenVariableChanges(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	pending := callToolAsync(sess, "api_request", apiArgs(envID, "GET", "{{base_url}}/echo"))
	req := f.pending(t)
	f.base = srv.URL + "/moved"
	f.writeDev(t, "127.0.0.1", false)
	f.answer(t, true, req.RequestID)
	if out := wait(t, pending); !out.IsError || !strings.Contains(text(out), "environment changed") {
		t.Fatalf("after the variable edit = %q, want an environment-changed error", text(out))
	}
	if len(srv.Requests()) != 0 {
		t.Fatalf("server saw %d requests, want none", len(srv.Requests()))
	}
}

func TestApiApprovalModes(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", true)

	get := apiArgs(envID, "GET", "{{base_url}}/echo")
	if out := decodeResult[apiResult](t, callTool(t, sess, "api_request", get)); out.Status != 200 || f.queueLen(t) != 0 {
		t.Fatalf("auto-approved GET = %+v, queue %d", out, f.queueLen(t))
	}
	post := apiArgs(envID, "POST", "{{base_url}}/echo")
	post["body"] = "x"
	pending := callToolAsync(sess, "api_request", post)
	f.answer(t, true, f.pending(t).RequestID)
	if out := decodeResult[apiResult](t, wait(t, pending)); out.Status != 200 {
		t.Fatalf("POST = %+v", out)
	}

	f.writeDev(t, "127.0.0.1", false)
	f.patchAPI(t, "approval", &model.DbMcpPatch{APIApproval: ptr("never")})
	if out := decodeResult[apiResult](t, callTool(t, sess, "api_request", post)); out.Status != 200 || f.queueLen(t) != 0 {
		t.Fatalf("approval never = %+v, queue %d, want a send with no item", out, f.queueLen(t))
	}
	if len(srv.Requests()) != 3 {
		t.Fatalf("server saw %d requests, want 3", len(srv.Requests()))
	}
}

func TestApiToolsToggle(t *testing.T) {
	f := newFixture(t)
	_, sess, _ := f.apiSetup(t, "127.0.0.1", false)
	has := func() bool {
		list, err := sess.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range list.Tools {
			if strings.HasPrefix(tool.Name, "api_") {
				return true
			}
		}
		return false
	}
	if !has() {
		t.Fatal("api tools missing with apiRequests on")
	}
	f.patchAPI(t, "off", &model.DbMcpPatch{APIRequests: ptr(false)})
	if has() {
		t.Fatal("api tools listed with apiRequests off")
	}
	if res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "api_request", Arguments: apiArgs("", "GET", "http://127.0.0.1/")}); err == nil && !res.IsError {
		t.Fatalf("api_request with the tools off = %q, want an error", text(res))
	}
	f.patchAPI(t, "on", &model.DbMcpPatch{APIRequests: ptr(true)})
	if !has() {
		t.Fatal("api tools missing after turning them back on")
	}
}

func TestApiListEnvironments(t *testing.T) {
	f := newFixture(t)
	_, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.app.WriteAPIFile(t, "plain.environment.yaml", []byte("name: Plain\nvariables:\n  - name: a\n    value: b\n"))

	res := callTool(t, sess, "api_list_environments", map[string]any{})
	var out struct {
		Environments []struct {
			ID        string
			Name      string
			Variables []struct {
				Name   string
				Secret bool
				Hosts  []string
			}
		}
	}
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Environments) != 1 || out.Environments[0].ID != envID || out.Environments[0].Name != "Dev" {
		t.Fatalf("environments = %s, want only Dev", text(res))
	}
	var names []string
	for _, v := range out.Environments[0].Variables {
		names = append(names, fmt.Sprintf("%s:%t:%v", v.Name, v.Secret, v.Hosts))
	}
	if got := strings.Join(names, ","); got != "base_url:false:[],token:true:[127.0.0.1]" {
		t.Fatalf("variables = %s", got)
	}
	noSecret(t, "the listing", text(res))
}

func TestApiResponseCaps(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	f.setAPI(t, true, "never")

	big := decodeResult[apiResult](t, callTool(t, sess, "api_request", apiArgs(envID, "GET", srv.URL+"/bytes?n=200000")))
	if big.Binary || !big.Truncated || big.BodyBytes != 200000 || len(big.Body) != 64<<10 {
		t.Fatalf("large body = %d bytes shown, %+v, want 64 KiB of 200000, truncated", len(big.Body), big.Truncated)
	}
	slow := apiArgs(envID, "GET", srv.URL+"/slow?ms=5000")
	slow["timeoutSeconds"] = 1
	if res := callTool(t, sess, "api_request", slow); !res.IsError {
		t.Fatalf("slow = %q, want a timeout error", text(res))
	}
}

// TestApiRequestBodies: the item carries the body as sent (JSON text, form fields), and a DELETE reads
// as danger. Contract approvals-api, read by tests/ui/approvals-api.spec.ts.
func TestApiRequestBodies(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)

	record := func(key string, args map[string]any) approvals.Item {
		t.Helper()
		pending := callToolAsync(sess, "api_request", args)
		req := f.pending(t)
		f.app.Contract(t, "approvals-api", "item#"+key, req.Item, flowharness.Mask("createdAt", "expiresAt", "cwd", "environment.id"), flowharness.Replace(srv.URL, "http://127.0.0.1:<port>"))
		f.answer(t, false, req.RequestID)
		if out := wait(t, pending); !out.IsError {
			t.Fatalf("denied %s = %q, want an error", key, text(out))
		}
		return req.Item
	}

	jsonArgs := apiArgs(envID, "POST", "{{base_url}}/echo", "Authorization", "Bearer {{token}}")
	jsonArgs["body"] = `{"name":"kira","tags":["a","b"]}`
	jsonArgs["contentType"] = "application/json"
	record("json", jsonArgs)

	formArgs := apiArgs(envID, "POST", "{{base_url}}/echo")
	formArgs["form"] = []map[string]string{{"name": "grant_type", "value": "client_credentials"}, {"name": "scope", "value": "read"}}
	record("form", formArgs)

	del := record("delete", apiArgs(envID, "DELETE", "{{base_url}}/echo?id=7"))
	if del.Operation.Tone != "danger" {
		t.Fatalf("DELETE tone = %q, want danger", del.Operation.Tone)
	}
	if len(srv.Requests()) != 0 {
		t.Fatalf("server saw %d requests, want none", len(srv.Requests()))
	}
}
