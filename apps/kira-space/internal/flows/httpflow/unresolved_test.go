package httpflow

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
)

// J-HTTP-048 (OQ-API-2): a {{name}} no variable defines blocks the send before any request leaves.
// The error text is a contract: tests/ui/studio/http-variables.spec.ts shows the same text when the
// renderer refuses the send, and asserts no HttpService.Send call.
func TestUnresolvedVariableBlocksSend(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	const sc = "api-http-unresolved"
	opts := flowharness.Replace(srv.URL, "<http>")

	args := get(srv.URL + "/echo/{{missing}}")
	args.Method = "POST"
	args.Headers = []httpclient.Header{{Name: "X-A", Value: "{{nope}}"}}
	args.Body = httpclient.Body{Mode: "raw", Raw: `{"a":"{{gone}}"}`}
	app.Contract(t, sc, "args:HttpService.Send", args, flowharness.Mask("opId", "tabId"), opts)

	e := sendErr(t, app, args)
	for _, name := range []string{"missing", "nope", "gone"} {
		if !strings.Contains(e.Message, name) {
			t.Errorf("error %q does not list %s", e.Message, name)
		}
	}
	app.Contract(t, sc, "error:HttpService.Send", e.Message)
	if n := len(srv.Requests()); n != 0 {
		t.Fatalf("recorder saw %d requests, want 0", n)
	}

	t.Run("each field alone blocks", func(t *testing.T) {
		for name, mut := range map[string]func(*bridge.HttpSendArgs){
			"url":    func(a *bridge.HttpSendArgs) { a.URL = srv.URL + "/echo?q={{only}}" },
			"header": func(a *bridge.HttpSendArgs) { a.Headers = []httpclient.Header{{Name: "X-A", Value: "{{only}}"}} },
			"body":   func(a *bridge.HttpSendArgs) { a.Body = httpclient.Body{Mode: "raw", Raw: "{{only}}"} },
			"urlencoded": func(a *bridge.HttpSendArgs) {
				a.Body = httpclient.Body{Mode: "urlencoded", URLEncoded: []httpclient.Field{{Name: "k", Value: "{{only}}"}}}
			},
		} {
			a := get(srv.URL + "/echo")
			a.Method = "POST"
			mut(&a)
			if e := sendErr(t, app, a); !strings.Contains(e.Message, "only") {
				t.Errorf("%s: error %q does not list only", name, e.Message)
			}
		}
		if n := len(srv.Requests()); n != 0 {
			t.Fatalf("recorder saw %d requests, want 0", n)
		}
	})

	t.Run("defined secret resolves and the rest still block", func(t *testing.T) {
		col, env := secretVar(t, app, "token", "tok-1")
		a := get(srv.URL + "/echo")
		a.CollectionID, a.EnvironmentID = col, env
		a.Headers = []httpclient.Header{{Name: "Authorization", Value: "Bearer {{token}}"}}
		if res := send(t, app, a); res.Status != 200 {
			t.Fatalf("status = %d, want 200", res.Status)
		}
		before := len(srv.Requests())
		a.Headers = append(a.Headers, httpclient.Header{Name: "X-B", Value: "{{missing}}"})
		e := sendErr(t, app, a)
		if !strings.Contains(e.Message, "missing") || strings.Contains(e.Message, "token") || len(srv.Requests()) != before {
			t.Errorf("error %q, requests %d -> %d; want only missing named and nothing sent", e.Message, before, len(srv.Requests()))
		}
	})

	t.Run("defining the variable lets the send through", func(t *testing.T) {
		col, env := secretVar(t, app, "missing", "now-defined")
		a := get(srv.URL + "/echo")
		a.CollectionID, a.EnvironmentID = col, env
		a.Headers = []httpclient.Header{{Name: "X-A", Value: "{{missing}}"}}
		send(t, app, a)
		reqs := srv.Requests()
		if got := reqs[len(reqs)-1].Header.Get("X-A"); got != "now-defined" {
			t.Errorf("X-A on the wire = %q, want now-defined", got)
		}
	})
}
