package dbmcpflow

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/servers"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/jsengine"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/approvals-api-response.spec.ts (contract approvals-api-response): the
// held API response, its raw and transformed views, and the settings that hold it.

const reviewKey = "approvals-api-response"

var volatile = regexp.MustCompile(`"elapsedMs":\d+|"value":"[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4}[^"]*GMT"`)

// stable pins what changes between runs inside a response text: timing and the Date header.
func stable(s string) string {
	return volatile.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, `"elapsedMs"`) {
			return `"elapsedMs":0`
		}
		return `"value":"<date>"`
	})
}

type responseItem struct {
	Method      string
	URL         string
	Host        string
	Environment *struct{ ID, Name, Color string }
	Status      int
	StatusText  string
	IsError     bool
	Scripted    bool
	Bytes       int
	RawBytes    int
	Class       string
	Reason      string
}

func (f *fixture) responseItem(t *testing.T, it approvals.Item) responseItem {
	t.Helper()
	if it.Kind != approvals.KindAPIResponse {
		t.Fatalf("item kind = %s, want api.response", it.Kind)
	}
	var p responseItem
	if err := json.Unmarshal(it.Payload, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *fixture) pendingResponse(t *testing.T, requestID string) bridge.DbMcpPendingResponse {
	t.Helper()
	held, err := f.app.W.DbMcp.PendingResponse(bridge.DbMcpApprovalArgs{RequestID: requestID})
	if err != nil {
		t.Fatal(err)
	}
	return held
}

// reviewEnv is apiEnv with extra top-level keys in the Dev environment file.
func (f *fixture) reviewEnv(t *testing.T, extra string) string {
	t.Helper()
	text := "name: Dev\ncolor: green\n" + extra + "variables:\n" +
		"  - name: base_url\n    value: " + f.base + "\n    agent: true\n" +
		"  - name: token\n    secret: ${secret:dev.token}\n    agent: true\n    agentHosts: [127.0.0.1]\n"
	f.app.WriteAPIFile(t, devFile, []byte(text))
	if _, err := f.app.W.ApiConfig.SetSecret(bridge.ApiSecretArgs{Name: "dev.token", Value: apiSecret}); err != nil {
		t.Fatal(err)
	}
	f.setAPI(t, true, "never")
	envs, err := f.app.W.ApiFiles.EnvironmentList()
	if err != nil || len(envs) != 1 {
		t.Fatalf("EnvironmentList = %+v, %v", envs, err)
	}
	return envs[0].ID
}

func (f *fixture) setReviewApp(t *testing.T, on bool) {
	t.Helper()
	if _, err := f.app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{APIReviewResponses: &on}}}); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) reviewSession(t *testing.T, extra string) (*servers.HTTPServer, *mcp.ClientSession, string) {
	t.Helper()
	srv := flowharness.HTTP(t)
	f.base = srv.URL
	envID := f.reviewEnv(t, extra)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return srv, sess, envID
}

// recordReview records the held item, its pending response and the answer under key, and returns the item.
func (f *fixture) recordReview(t *testing.T, key string, srvURL string, req pendingReq, held bridge.DbMcpPendingResponse) {
	t.Helper()
	opts := []flowharness.ContractOption{flowharness.Mask("createdAt", "expiresAt", "cwd", "environment.id"), flowharness.Replace(srvURL, "http://127.0.0.1:<port>")}
	f.app.Contract(t, reviewKey, "ApprovalsService.List#"+key, []approvals.Item{req.Item}, opts...)
	held.Text, held.Raw = stable(held.Text), stable(held.Raw)
	f.app.Contract(t, reviewKey, "DbMcpService.PendingResponse#"+key, held, opts...)
	f.app.Contract(t, reviewKey, "args:ApprovalsService.Answer#"+key, approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
}

// TestApiResponseReview: a held response raises one api.response item without its body; Send returns the
// exact text the user saw.
func TestApiResponseReview(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setReviewApp(t, true)

	pending := callToolAsync(sess, "api_request", apiArgs(envID, "GET", usersURL(srv.URL)))
	req := f.pending(t)
	p := f.responseItem(t, req.Item)
	if p.Method != "GET" || p.Host != "127.0.0.1" || p.Status != 200 || p.Scripted || p.IsError || p.Class != "read" || p.Reason != "setting" ||
		p.Environment == nil || p.Environment.Name != "Dev" || p.RawBytes != 0 {
		t.Fatalf("payload = %s, want a plain read held by setting", req.Item.Payload)
	}
	held := f.pendingResponse(t, req.RequestID)
	if p.Bytes != len(held.Text) || held.Raw != "" || held.Script != nil || held.IsError {
		t.Fatalf("pending = %+v, payload bytes %d, want text only", held, p.Bytes)
	}
	var shown apiResult
	if err := json.Unmarshal([]byte(held.Text), &shown); err != nil || shown.Status != 200 || !strings.Contains(shown.Body, "a@x.test") {
		t.Fatalf("held text = %s (%v), want the masked result", held.Text, err)
	}
	listJSON, _ := json.Marshal(f.list(t))
	for _, leak := range []string{"a@x.test", "Ann", apiSecret} {
		if strings.Contains(string(listJSON), leak) {
			t.Errorf("the approvals list carries %q", leak)
		}
	}
	f.recordReview(t, "plain", srv.URL, req, held)

	f.answer(t, true, req.RequestID)
	res := wait(t, pending)
	if res.IsError || text(res) != held.Text {
		t.Fatalf("result = %s, want exactly the held text", text(res))
	}
	f.app.Contract(t, reviewKey, "result#plain", stable(text(res)), flowharness.Replace(srv.URL, "http://127.0.0.1:<port>"))
	noSecret(t, "the tool result", text(res))
}

// TestApiResponseReviewScripted: with a transform the user sees the raw response, the transformed result
// and the script; a failing transform is held too.
func TestApiResponseReviewScripted(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setReviewApp(t, true)

	run := func(key, method, transform string) (responseItem, bridge.DbMcpPendingResponse, *mcp.CallToolResult) {
		t.Helper()
		args := scriptArgs(envID, method, usersURL(srv.URL), map[string]any{"transform": transform})
		pending := callToolAsync(sess, "api_request", args)
		req := f.pending(t)
		p := f.responseItem(t, req.Item)
		held := f.pendingResponse(t, req.RequestID)
		f.recordReview(t, key, srv.URL, req, held)
		f.answer(t, true, req.RequestID)
		return p, held, wait(t, pending)
	}

	p, held, res := run("scripted", "GET", transformSrc)
	const want = `{"output":[{"id":9007199254740993,"domain":"x.test"},{"id":2,"domain":"y.test"}]}`
	if !p.Scripted || p.IsError || p.Bytes != len(want) || p.RawBytes != len(held.Raw) || p.RawBytes == 0 || p.Class != "read" {
		t.Fatalf("payload = %+v, want scripted with raw bytes", p)
	}
	if held.Text != want || !strings.Contains(held.Raw, "a@x.test") || !strings.Contains(held.Raw, `"status":200`) || strings.Contains(held.Text, "a@x.test") {
		t.Fatalf("pending text %s raw %s, want the transform result and the raw response", held.Text, held.Raw)
	}
	if held.Script == nil || held.Script.Source != transformSrc || held.Script.Hash != jsengine.SourceHash(transformSrc) {
		t.Fatalf("pending script = %+v, want the transform and its hash", held.Script)
	}
	if res.IsError || text(res) != held.Text {
		t.Fatalf("result = %s, want exactly the held text", text(res))
	}

	if p, _, _ := run("scriptedWrite", "POST", transformSrc); p.Class != "write" || !p.Scripted {
		t.Fatalf("POST payload = %+v, want a scripted write", p)
	}

	const failing = "throw new Error('boom')"
	p, held, res = run("scriptFailed", "GET", failing)
	if !p.IsError || !p.Scripted || !strings.Contains(held.Text, "transform script failed:") || !strings.Contains(held.Text, "was sent (HTTP 200)") || !strings.Contains(held.Raw, "a@x.test") {
		t.Fatalf("failed payload %+v pending %+v, want the failure text with the raw response", p, held)
	}
	if !res.IsError || text(res) != held.Text {
		t.Fatalf("failed result = %s, want exactly the held failure text", text(res))
	}
}

// TestApiResponseWithhold: every way the response does not reach the agent gives app text only; the
// request was sent exactly once.
func TestApiResponseWithhold(t *testing.T) {
	f := newFixture(t, flowharness.WithApprovalHeldCaps(1<<10, 1000))
	srv, sess, envID := f.reviewSession(t, "")
	f.setReviewApp(t, true)
	denied := func(method string) string {
		t.Helper()
		pending := callToolAsync(sess, "api_request", apiArgs(envID, method, usersURL(srv.URL)))
		f.answer(t, false, f.pending(t).RequestID)
		res := wait(t, pending)
		if !res.IsError {
			t.Fatalf("%s withheld = %q, want an error", method, text(res))
		}
		return text(res)
	}

	const base = "the user withheld the response to GET 127.0.0.1; the request was sent, but you get no response. Do not resend it to get the response another way."
	if got := denied("GET"); got != base {
		t.Fatalf("GET deny = %q, want %q", got, base)
	}
	if got := denied("POST"); got != strings.Replace(base, "GET", "POST", 1)+" Its effects stay." {
		t.Fatalf("POST deny = %q, want the text with the effects note", got)
	}
	if got := len(srv.Requests()); got != 2 {
		t.Fatalf("server saw %d requests, want one per call", got)
	}
	for _, leak := range []string{"200", "a@x.test", "application/json"} {
		if strings.Contains(base, leak) {
			t.Errorf("deny text carries response data %q", leak)
		}
	}

	// Caps: an over-cap response is withheld unseen, and a deny releases its bytes.
	big := "/raw?ct=application%2Fjson&hex=" + fmt.Sprintf("%x", `"`+strings.Repeat("x", 2000)+`"`)
	res := callTool(t, sess, "api_request", apiArgs(envID, "GET", srv.URL+big))
	if !res.IsError || !strings.Contains(text(res), "over the 8 MB review limit") || !strings.Contains(text(res), "withheld unseen") ||
		!strings.Contains(text(res), "Ask for less") {
		t.Fatalf("over the cap = %q, want the too-large refusal", text(res))
	}
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("a too-large response raised %d items", n)
	}
	mid := "/raw?ct=application%2Fjson&hex=" + fmt.Sprintf("%x", `"`+strings.Repeat("y", 20)+`"`)
	for i := 0; i < 2; i++ {
		pending := callToolAsync(sess, "api_request", apiArgs(envID, "GET", srv.URL+mid))
		rq := f.pending(t)
		f.answer(t, false, rq.RequestID)
		if r := wait(t, pending); !r.IsError || !strings.Contains(text(r), "withheld the response") {
			t.Fatalf("round %d = %q, want a deny after bytes were released", i, text(r))
		}
	}
	// Total cap: two held responses fit, the third is refused while they wait.
	first := callToolAsync(sess, "api_request", apiArgs(envID, "GET", srv.URL+mid))
	_ = f.items(t, 1)
	second := callToolAsync(sess, "api_request", apiArgs(envID, "GET", srv.URL+mid))
	_ = f.items(t, 2)
	third := callTool(t, sess, "api_request", apiArgs(envID, "GET", srv.URL+mid))
	if !third.IsError || !strings.Contains(text(third), "too many results are waiting") || !strings.Contains(text(third), "Do not resend it.") {
		t.Fatalf("third = %q, want the too-many-waiting refusal", text(third))
	}
	for _, it := range f.list(t) {
		f.answerItems(t, true, it.ID)
	}
	wait(t, first)
	wait(t, second)

	// Abandon: a client that cancels takes its held response with it.
	abandon, cancel := context.WithCancel(ctx)
	go func() {
		_, _ = sess.CallTool(abandon, &mcp.CallToolParams{Name: "api_request", Arguments: apiArgs(envID, "GET", usersURL(srv.URL))})
	}()
	req := f.pending(t)
	cancel()
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
	if _, err := f.app.W.DbMcp.PendingResponse(bridge.DbMcpApprovalArgs{RequestID: req.RequestID}); err == nil {
		t.Fatal("PendingResponse still answers after the client left")
	}

	// Turned off while the response waits.
	pending := callToolAsync(sess, "api_request", apiArgs(envID, "POST", usersURL(srv.URL)))
	req = f.pending(t)
	f.setAPI(t, false, "never")
	f.answer(t, true, req.RequestID)
	res = wait(t, pending)
	const off = "the POST 127.0.0.1 request was sent, but API requests are turned off in Kira Space's Studio MCP settings while its response was in review, so it was withheld. Its effects stay. Do not resend it."
	if !res.IsError || text(res) != off {
		t.Fatalf("turned off = %q, want %q", text(res), off)
	}
}

// TestApiResponseReviewSettings: the app leaf and the environment's agentReview decide per class.
func TestApiResponseReviewSettings(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "agentReview:\n  read: send\n  write: review\n")
	f.setReviewApp(t, true)

	held := func(method string, body bool) bool {
		t.Helper()
		args := apiArgs(envID, method, usersURL(srv.URL))
		if body {
			args["body"] = "x"
		}
		pending := callToolAsync(sess, "api_request", args)
		var hold bool
		select {
		case <-time.After(1500 * time.Millisecond):
			hold = f.queueLen(t) == 1
		case out := <-pending:
			pending <- out
		}
		if hold {
			f.answerItems(t, true, f.list(t)[0].ID)
		}
		if res := wait(t, pending); res.IsError {
			t.Fatalf("%s = %q", method, text(res))
		}
		return hold
	}
	listed := func() map[string]map[string]bool {
		t.Helper()
		res := callTool(t, sess, "list", map[string]any{"what": []string{"environments"}})
		var out struct {
			Environments []struct {
				Name   string
				Review []string
			}
		}
		if err := json.Unmarshal([]byte(text(res)), &out); err != nil || len(out.Environments) != 1 {
			t.Fatalf("listing = %s (%v)", text(res), err)
		}
		noSecret(t, "the listing", text(res))
		held := map[string]bool{}
		for _, mode := range out.Environments[0].Review {
			held[mode] = true
		}
		return map[string]map[string]bool{out.Environments[0].Name: held}
	}
	if got := listed()["Dev"]; got["read"] || !got["write"] {
		t.Fatalf("review = %v, want read sent, write held", got)
	}
	if held("GET", false) || !held("POST", false) {
		t.Fatal("env read send / write review: want GET sent and POST held")
	}
	// A GET with a body is a write.
	if !held("GET", true) {
		t.Fatal("GET with a body: want it held as a write")
	}

	f.reviewEnv(t, "")
	f.setReviewApp(t, false)
	if got := listed()["Dev"]; got["read"] || got["write"] {
		t.Fatalf("reviewResults = %v, want none with inherit and the app off", got)
	}
	if held("GET", false) || held("POST", false) {
		t.Fatal("inherit with the app off: want nothing held")
	}
	f.reviewEnv(t, "agentReview:\n  read: review\n")
	if got := listed()["Dev"]; !got["read"] || got["write"] {
		t.Fatalf("reviewResults = %v, want read held only", got)
	}
	if !held("GET", false) || held("POST", false) {
		t.Fatal("env read review, app off: want GET held and POST sent")
	}

	// No environment: the app leaf decides.
	run := func() bool {
		pending := callToolAsync(sess, "api_request", map[string]any{"method": "GET", "url": usersURL(srv.URL)})
		select {
		case <-time.After(1500 * time.Millisecond):
			hold := f.queueLen(t) == 1
			if hold {
				f.answerItems(t, true, f.list(t)[0].ID)
			}
			wait(t, pending)
			return hold
		case out := <-pending:
			if out.res != nil && out.res.IsError {
				t.Fatalf("no environment = %q", text(out.res))
			}
			return false
		}
	}
	if run() {
		t.Fatal("no environment with the app off: want it sent")
	}
	f.setReviewApp(t, true)
	if !run() {
		t.Fatal("no environment with the app on: want it held")
	}
}

// TestApiResponseReviewOnRequest: the approval card offers the review; ticking it holds this response.
func TestApiResponseReviewOnRequest(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setAPI(t, true, "always")
	f.setReviewApp(t, false)
	const sc = "approvals-api"

	type offer struct{ ReviewOffered, ReviewResult bool }
	payloadOffer := func(it approvals.Item) offer {
		var o offer
		if err := json.Unmarshal(it.Payload, &o); err != nil {
			t.Fatal(err)
		}
		return o
	}

	pending := callToolAsync(sess, "api_request", apiArgs(envID, "GET", usersURL(srv.URL)))
	req := f.pending(t)
	if o := payloadOffer(req.Item); !o.ReviewOffered || o.ReviewResult {
		t.Fatalf("offer = %+v, want review offered and not on", o)
	}
	opts := []flowharness.ContractOption{flowharness.Mask("createdAt", "expiresAt", "cwd", "environment.id"), flowharness.Replace(srv.URL, "http://127.0.0.1:<port>")}
	f.app.Contract(t, sc, "ApprovalsService.List#reviewOffered", []approvals.Item{req.Item}, opts...)
	args := approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true, Options: map[string]json.RawMessage{req.Item.ID: json.RawMessage(`{"reviewResult":true}`)}}
	f.app.Contract(t, sc, "args:ApprovalsService.Answer#reviewOffered", args)
	bad := approvals.AnswerArgs{IDs: args.IDs, Approve: true, Options: map[string]json.RawMessage{req.Item.ID: json.RawMessage(`{"reviewResult":true,"extra":1}`)}}
	if res, err := f.app.W.ApprovalsSvc.Answer(bad); err != nil || len(res) != 1 || res[0].Error == "" {
		t.Fatalf("answer with an unknown option = %+v (%v), want a contract error", res, err)
	}
	if res, err := f.app.W.ApprovalsSvc.Answer(args); err != nil || res[0].Error != "" {
		t.Fatalf("answer = %+v (%v)", res, err)
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
	resp := f.pending(t)
	if p := f.responseItem(t, resp.Item); p.Reason != "requested" {
		t.Fatalf("payload = %+v, want reason requested", p)
	}
	f.answer(t, true, resp.RequestID)
	if res := wait(t, pending); res.IsError {
		t.Fatalf("sent = %q", text(res))
	}

	// Without the tick the response goes straight back.
	pending = callToolAsync(sess, "api_request", apiArgs(envID, "GET", usersURL(srv.URL)))
	f.answerItems(t, true, f.pending(t).Item.ID)
	if res := wait(t, pending); res.IsError || f.queueLen(t) != 0 {
		t.Fatalf("unticked = %q, queue %d, want no response item", text(res), f.queueLen(t))
	}

	// Already on by settings: shown on and locked, and a response item follows by setting.
	f.setReviewApp(t, true)
	pending = callToolAsync(sess, "api_request", apiArgs(envID, "GET", usersURL(srv.URL)))
	req = f.pending(t)
	if o := payloadOffer(req.Item); !o.ReviewOffered || !o.ReviewResult {
		t.Fatalf("offer = %+v, want review offered and already on", o)
	}
	f.app.Contract(t, sc, "ApprovalsService.List#reviewOn", []approvals.Item{req.Item}, opts...)
	f.answerItems(t, true, req.Item.ID)
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
	resp = f.pending(t)
	if p := f.responseItem(t, resp.Item); p.Reason != "setting" {
		t.Fatalf("payload = %+v, want reason setting", p)
	}

	// A response item takes no options.
	opt := approvals.AnswerArgs{IDs: []string{resp.Item.ID}, Approve: true, Options: map[string]json.RawMessage{resp.Item.ID: json.RawMessage(`{"reviewResult":true}`)}}
	if res, err := f.app.W.ApprovalsSvc.Answer(opt); err != nil || res[0].Error == "" {
		t.Fatalf("options on a response item = %+v (%v), want a contract error", res, err)
	}
	f.answer(t, false, resp.RequestID)
	wait(t, pending)
}
