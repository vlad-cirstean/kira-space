package httpflow

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
)

const secretToken = "tok-7f3a9c1e2b"

func TestSendResolvesSecretsOnlyOnTheWire(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)

	col, err := app.W.Collections.CreateCollection(bridge.CollectionsCreateCollectionArgs{Name: "api"})
	if err != nil {
		t.Fatal(err)
	}
	env := app.SeedEnvironment(t, "dev", studioflowharness.APIVar{Name: "token", Value: secretToken, Secret: true})
	if err := app.W.Variables.SetActiveEnvironment(bridge.VariablesEnvironmentIDArgs{ID: env.ID}); err != nil {
		t.Fatal(err)
	}

	args := get(srv.URL + "/status?code=200&t={{token}}")
	args.CollectionID, args.EnvironmentID = col.ID, env.ID
	args.Headers = []httpclient.Header{{Name: "Authorization", Value: "Bearer {{token}}"}}
	// Contract api-http, read by tests/ui/http-variables.spec.ts "a secret reference stays literal
	// in the send args and the history list".
	const sc = "api-http"
	opts := studioflowharness.Replace(srv.URL, "<http>")
	envs, err := app.W.Variables.ListEnvironments()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "VariablesService.ListEnvironments", envs)
	vars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeEnvironment, OwnerID: env.ID})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "VariablesService.List", vars)
	app.Contract(t, sc, "args:HttpService.Send", args, studioflowharness.Mask("opId", "tabId"), opts)
	res := send(t, app, args)
	app.Contract(t, sc, "HttpService.Send", res, opts, studioflowharness.Mask("elapsedMs", "bodyBytes"), studioflowharness.Omit("body", "headers", "timeline", "wire"))

	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(reqs))
	}
	if got := reqs[0].Header.Get("Authorization"); got != "Bearer "+secretToken {
		t.Errorf("server Authorization = %q, want the plaintext token", got)
	}
	if !strings.Contains(reqs[0].Query, secretToken) {
		t.Errorf("server query = %q, want the plaintext token", reqs[0].Query)
	}

	if res.Wire == nil {
		t.Fatal("no wire exchange")
	}
	if !strings.Contains(res.Wire.Request, "{{token}}") {
		t.Errorf("wire request does not show the placeholder:\n%s", res.Wire.Request)
	}
	mustNotContain(t, "wire request", res.Wire.Request, secretToken)
	mustNotContain(t, "final url", res.FinalURL, secretToken)
	for _, h := range res.Timeline.Hops {
		mustNotContain(t, "timeline hop url", h.URL, secretToken)
	}

	entries, err := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{TabID: "tab-1"})
	if err != nil || len(entries) != 1 {
		t.Fatalf("history entries = %d, err %v; want 1", len(entries), err)
	}
	app.Contract(t, sc, "ResponseHistoryService.List", entries, opts, studioflowharness.Mask("elapsedMs", "bodyBytes", "storedBytes"))
	snap, err := app.W.ResponseHistory.Get(bridge.ResponseHistoryIDArgs{ID: entries[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Request.Headers[0].Value != "Bearer {{token}}" {
		t.Errorf("history request header = %q, want the placeholder", snap.Request.Headers[0].Value)
	}
	mustNotContain(t, "history url", snap.Request.URL, secretToken)

	op := waitOp(t, app, args.OpID)
	if op.Command == nil || !strings.Contains(*op.Command, "{{token}}") {
		t.Errorf("op command = %v, want the placeholder", op.Command)
	}
	if op.Command != nil {
		mustNotContain(t, "op command", *op.Command, secretToken)
	}
	if op.Status != "ok" {
		t.Errorf("op status = %q, want ok", op.Status)
	}

	db := string(dbBytes(t, app))
	if !strings.Contains(db, "/status?code=200&t={{token}}") {
		t.Error("database bytes lack the stored history URL; the scan reads the wrong files")
	}
	if strings.Contains(db, secretToken) {
		t.Error("database bytes contain the plaintext token")
	}
}

func TestRedirects(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)
	payload := "payload-body"

	for _, tc := range []struct {
		code       string
		wantMethod string
		wantBody   bool
	}{
		{"301", "GET", false}, {"302", "GET", false}, {"303", "GET", false},
		{"307", "POST", true}, {"308", "POST", true},
	} {
		t.Run(tc.code, func(t *testing.T) {
			before := len(srv.Requests())
			args := get(srv.URL + "/redirect?n=1&code=" + tc.code)
			args.Method = "POST"
			args.Body = httpclient.Body{Mode: "raw", Raw: payload}
			res := send(t, app, args)
			if res.Status != 200 {
				t.Fatalf("status = %d, want 200", res.Status)
			}
			reqs := srv.Requests()[before:]
			if len(reqs) != 3 {
				t.Fatalf("server saw %d requests, want 3 (2 redirects + /echo)", len(reqs))
			}
			final := reqs[2]
			if final.Path != "/echo" || final.Method != tc.wantMethod {
				t.Errorf("final request = %s %s, want %s /echo", final.Method, final.Path, tc.wantMethod)
			}
			if got := final.BodyLen == len(payload); got != tc.wantBody {
				t.Errorf("final body length %d, body kept = %v, want %v", final.BodyLen, got, tc.wantBody)
			}
			if len(res.Redirects) != 2 || len(res.Timeline.Hops) != 3 {
				t.Errorf("redirects = %d, hops = %d; want 2 and 3", len(res.Redirects), len(res.Timeline.Hops))
			}
			if res.Redirects[0].Status != mustAtoi(t, tc.code) {
				t.Errorf("first redirect status = %d, want %s", res.Redirects[0].Status, tc.code)
			}
		})
	}

	t.Run("cross host drops user headers", func(t *testing.T) {
		before := len(srv.Requests())
		args := get(srv.URL + "/redirect?n=0&code=302&to=" + url.QueryEscape(srv.AltURL+"/echo"))
		args.Headers = []httpclient.Header{
			{Name: "Authorization", Value: "Bearer abc"}, {Name: "X-Custom", Value: "keep-me"},
		}
		send(t, app, args)
		reqs := srv.Requests()[before:]
		if len(reqs) != 2 {
			t.Fatalf("server saw %d requests, want 2", len(reqs))
		}
		if reqs[0].Header.Get("X-Custom") != "keep-me" {
			t.Error("first hop lost X-Custom")
		}
		if !strings.HasPrefix(reqs[1].Host, "localhost:") {
			t.Fatalf("second hop host = %q, want localhost", reqs[1].Host)
		}
		for _, h := range []string{"Authorization", "X-Custom"} {
			if v := reqs[1].Header.Get(h); v != "" {
				t.Errorf("cross-host hop kept %s = %q", h, v)
			}
		}
	})

	t.Run("same host keeps user headers", func(t *testing.T) {
		before := len(srv.Requests())
		args := get(srv.URL + "/redirect?n=0&code=302")
		args.Headers = []httpclient.Header{{Name: "X-Custom", Value: "keep-me"}}
		send(t, app, args)
		reqs := srv.Requests()[before:]
		if reqs[len(reqs)-1].Header.Get("X-Custom") != "keep-me" {
			t.Error("same-host hop dropped X-Custom")
		}
	})

	t.Run("follow off returns the 302", func(t *testing.T) {
		before := len(srv.Requests())
		args := get(srv.URL + "/redirect?n=0&code=302")
		args.Options.FollowRedirects = ptr(false)
		res := send(t, app, args)
		if res.Status != 302 || len(srv.Requests())-before != 1 {
			t.Errorf("status = %d, requests = %d; want 302 and 1", res.Status, len(srv.Requests())-before)
		}
	})

	t.Run("max redirects", func(t *testing.T) {
		args := get(srv.URL + "/redirect?n=3&code=302")
		args.Options.MaxRedirects = ptr(2)
		e := sendErr(t, app, args)
		if e.Code != string(httpclient.CodeTransport) || !strings.Contains(e.Message, "redirect") {
			t.Errorf("error = %s %q, want E_TRANSPORT naming redirects", e.Code, e.Message)
		}
		if tl := errTimeline(t, e); len(tl.Hops) < 2 {
			t.Errorf("failed timeline hops = %d, want the chain walked so far (>= 2)", len(tl.Hops))
		}
	})
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestCookieJar(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)

	cookieEcho := func(base string) map[string]string {
		t.Helper()
		res := send(t, app, get(base+"/cookie/echo"))
		out := map[string]string{}
		for _, line := range strings.Split(strings.Trim(res.Body, "{}\n"), ",") {
			if k, v, ok := strings.Cut(line, ":"); ok {
				out[strings.Trim(k, `"`)] = strings.Trim(v, `"`)
			}
		}
		return out
	}

	t.Run("default settings keep no jar", func(t *testing.T) {
		send(t, app, get(srv.URL+"/cookie/set?name=a&value=1"))
		if got := cookieEcho(srv.URL); len(got) != 0 {
			t.Errorf("default send replayed cookies %v, want none (api.disableCookieJar defaults to true)", got)
		}
		list, err := app.W.Http.Cookies(bridge.HttpCookiesArgs{URL: srv.URL})
		if err != nil || len(list) != 0 {
			t.Errorf("jar = %v, err %v; want empty", list, err)
		}
	})

	setApi(t, app, model.ApiPatch{DisableCookieJar: ptr(false)})

	t.Run("jar on sets and replays", func(t *testing.T) {
		res := send(t, app, get(srv.URL+"/cookie/set?name=a&value=1"))
		if len(res.ReceivedCookies) != 1 || res.ReceivedCookies[0].Name != "a" {
			t.Errorf("received cookies = %+v, want a", res.ReceivedCookies)
		}
		if got := cookieEcho(srv.URL); got["a"] != "1" {
			t.Errorf("replayed cookies = %v, want a=1", got)
		}
	})

	t.Run("scoping", func(t *testing.T) {
		send(t, app, get(srv.URL+"/cookie/set?name=p&value=2&path=/cookie/echo"))
		before := len(srv.Requests())
		send(t, app, get(srv.URL+"/echo"))
		if c := srv.Requests()[before].Header.Get("Cookie"); strings.Contains(c, "p=2") {
			t.Errorf("path-scoped cookie sent to /echo: %q", c)
		}
		if got := cookieEcho(srv.URL); got["p"] != "2" {
			t.Errorf("path-scoped cookie not replayed on its path: %v", got)
		}
		if got := cookieEcho(srv.AltURL); len(got) != 0 {
			t.Errorf("cookies for 127.0.0.1 reached localhost: %v", got)
		}
	})

	t.Run("list delete clear", func(t *testing.T) {
		list, err := app.W.Http.Cookies(bridge.HttpCookiesArgs{URL: srv.URL + "/cookie/echo"})
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]httpclient.Cookie{}
		for _, c := range list {
			names[c.Name] = c
		}
		if names["a"].Name == "" || names["p"].Name == "" {
			t.Fatalf("jar = %+v, want a and p", list)
		}
		a := names["a"]
		after, err := app.W.Http.DeleteCookie(bridge.HttpCookieDeleteArgs{URL: srv.URL + "/cookie/echo", Name: a.Name, Domain: a.Domain, Path: a.Path})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range after {
			if c.Name == "a" {
				t.Error("a survived DeleteCookie")
			}
		}
		if err := app.W.Http.ClearCookies(); err != nil {
			t.Fatal(err)
		}
		if got := cookieEcho(srv.URL); len(got) != 0 {
			t.Errorf("cookies after ClearCookies: %v", got)
		}
	})

	t.Run("incognito cookie never reaches the shared jar", func(t *testing.T) {
		args := get(srv.URL + "/cookie/set?name=inc&value=9")
		args.Incognito = true
		send(t, app, args)
		list, err := app.W.Http.Cookies(bridge.HttpCookiesArgs{URL: srv.URL})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range list {
			if c.Name == "inc" {
				t.Error("incognito cookie landed in the shared jar")
			}
		}
	})
}

func TestTLSAndHTTPVersion(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTPS(t)

	e := sendErr(t, app, get(srv.URL+"/echo"))
	if e.Code != string(httpclient.CodeTransport) || !strings.Contains(strings.ToLower(e.Message), "certificate") {
		t.Errorf("default verify error = %s %q, want E_TRANSPORT naming the certificate", e.Code, e.Message)
	}

	skip := get(srv.URL + "/echo")
	skip.Options.SSLVerify = ptr(false)
	res := send(t, app, skip)
	if res.Status != 200 {
		t.Fatalf("unverified status = %d, want 200", res.Status)
	}
	if res.Proto != "HTTP/2.0" {
		t.Errorf("default response proto = %s, want HTTP/2.0 (api.httpVersion defaults to 2)", res.Proto)
	}

	h1 := get(srv.URL + "/echo")
	h1.Options.SSLVerify = ptr(false)
	h1.Options.HTTPVersion = ptr("1.1")
	send(t, app, h1)

	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("server saw %d requests, want 2 (the verify failure sends none)", len(reqs))
	}
	if reqs[0].Proto != "HTTP/2.0" || reqs[1].Proto != "HTTP/1.1" {
		t.Errorf("server protos = %s, %s; want HTTP/2.0, HTTP/1.1", reqs[0].Proto, reqs[1].Proto)
	}
	if !reqs[0].TLS || !reqs[1].TLS {
		t.Error("requests did not arrive over TLS")
	}

	// A global sslVerify=false applies without a per-request override.
	setApi(t, app, model.ApiPatch{SSLVerify: ptr(false)})
	if got := send(t, app, get(srv.URL+"/echo")); got.Status != 200 {
		t.Errorf("global sslVerify=false status = %d, want 200", got.Status)
	}
}

func TestConcurrentSendsSharedJar(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)
	setApi(t, app, model.ApiPatch{DisableCookieJar: ptr(false)})

	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name, value := fmt.Sprintf("c%d", i), fmt.Sprintf("v%d", i)
			if _, err := app.W.Http.Send(ctx, get(srv.URL+"/cookie/set?name="+name+"&value="+value)); err != nil {
				t.Errorf("set %s: %v", name, err)
				return
			}
			res, err := app.W.Http.Send(ctx, get(srv.URL+"/cookie/echo"))
			if err != nil {
				t.Errorf("echo %s: %v", name, err)
				return
			}
			if !strings.Contains(res.Body, `"`+name+`":"`+value+`"`) {
				t.Errorf("send %d did not replay its own cookie: %s", i, res.Body)
			}
		}()
	}
	wg.Wait()

	list, err := app.W.Http.Cookies(bridge.HttpCookiesArgs{URL: srv.URL + "/cookie/echo"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range list {
		got[c.Name] = c.Value
	}
	if len(got) != n {
		t.Fatalf("jar holds %d cookies, want %d: %v", len(got), n, got)
	}
	for i := range n {
		if got[fmt.Sprintf("c%d", i)] != fmt.Sprintf("v%d", i) {
			t.Fatalf("cookie c%d = %q, want v%d", i, got[fmt.Sprintf("c%d", i)], i)
		}
	}
	res := send(t, app, get(srv.URL+"/cookie/echo"))
	for i := range n {
		if !strings.Contains(res.Body, fmt.Sprintf(`"c%d":"v%d"`, i, i)) {
			t.Fatalf("final echo is missing c%d: %s", i, res.Body)
		}
	}
}
