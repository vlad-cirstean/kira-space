package dbmcpflow

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-mask-echo.spec.ts (contract e2e-mask-echo): a server that echoes a
// kept or user value back in every spelling it takes on the wire shows the agent the reference only.

const (
	maskEchoKey = "e2e-mask-echo"
	// echoValue differs in four forms: space, slash, plus and equals escape differently in a query and a path. It holds
	// no character JSON escapes, which the echo route would escape a second time inside its own JSON.
	echoValue = "e2e mask/echo+Q9=z"
)

func TestE2EMaskEcho(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setAPI(t, true, "always")
	opts := f.e2eOpts(srv.URL)
	forms := srv.URL + "/forms"
	echoCall := func(value string, extra map[string]any) map[string]any {
		args := apiArgs(envID, "GET", forms, "X-Echo", value)
		for k, v := range extra {
			args[k] = v
		}
		return args
	}
	input := func(name string) map[string]any {
		return map[string]any{"inputs": []map[string]string{{"name": name, "label": name}}}
	}
	typed := func(t *testing.T, name, value string) {
		t.Helper()
		req := f.pending(t)
		o, _ := json.Marshal(map[string]any{"inputs": map[string]string{name: value}, "inputModes": map[string]string{name: "keep"}})
		f.answerWith(t, true, req.Item.ID, map[string]string{req.Item.ID: string(o)})
	}
	// echoed decodes the six forms the server answered, from an api_request result or a transform's result.
	echoed := func(t *testing.T, body string) map[string]string {
		t.Helper()
		var got map[string]string
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatalf("echo body %q: %v", body, err)
		}
		return got
	}
	bodyOf := func(t *testing.T, res string) string {
		t.Helper()
		var r struct{ Body string }
		if err := json.Unmarshal([]byte(res), &r); err != nil {
			t.Fatalf("result %q: %v", res, err)
		}
		return r.Body
	}
	var agent []string

	t.Run("a long user value is masked in all six forms, held and sent alike", func(t *testing.T) {
		f.setReviewApp(t, true)
		call := echoCall("{{kept.apikey}}", map[string]any{"script": map[string]any{"transform": "return response.json;"}})
		call["inputs"] = input("apikey")["inputs"]
		pending := callToolAsync(sess, "api_request", call)
		typed(t, "apikey", echoValue)
		testWait := f.startHeldFromQueue(t)
		var heldText struct{ Result map[string]string }
		if err := json.Unmarshal([]byte(testWait.held.Text), &heldText); err != nil || len(heldText.Result) != 6 {
			t.Fatalf("held text = %q (%v), want the six forms", testWait.held.Text, err)
		}
		for k, v := range heldText.Result {
			if v != "{{kept.apikey}}" {
				t.Errorf("held form %s = %q, want the reference", k, v)
			}
		}
		for _, form := range leakForms(t, echoValue) {
			if strings.Contains(testWait.held.Text, form) || strings.Contains(testWait.held.Raw, form) {
				t.Errorf("the held response holds the form %q", form)
			}
		}
		f.app.Contract(t, maskEchoKey, "ApprovalsService.List#forms", []approvals.Item{testWait.req.Item}, opts...)
		held := testWait.held
		held.Text, held.Raw = stable(held.Text), stable(held.Raw)
		f.app.Contract(t, maskEchoKey, "DbMcpService.PendingResponse#forms", held, opts...)
		f.answerItems(t, true, testWait.req.Item.ID)
		res := wait(t, pending)
		if res.IsError || text(res) != testWait.held.Text {
			t.Fatalf("result = %q, want exactly the held text %q", text(res), testWait.held.Text)
		}
		agent = append(agent, text(res))
		f.app.Contract(t, maskEchoKey, "result#forms", text(res))
		if got := f.app.W.Kept.List(externalScope); len(got) != 1 {
			t.Fatalf("kept = %v, want only the typed value", got)
		}
		f.setReviewApp(t, false)
	})

	t.Run("a short user value keeps its base64 form (documented rule)", func(t *testing.T) {
		const pin = "ab12cd"
		call := echoCall("{{kept.pin}}", input("pin"))
		pending := callToolAsync(sess, "api_request", call)
		typed(t, "pin", pin)
		res := wait(t, pending)
		if res.IsError {
			t.Fatalf("result = %q", text(res))
		}
		got := echoed(t, bodyOf(t, text(res)))
		b64 := base64.StdEncoding.EncodeToString([]byte(pin))
		for _, k := range []string{"raw", "json", "query", "path"} {
			if got[k] != "{{kept.pin}}" {
				t.Errorf("form %s = %q, want the reference", k, got[k])
			}
		}
		for _, k := range []string{"b64", "b64url"} {
			if got[k] != b64 {
				t.Errorf("form %s = %q, want %q: a value under 8 bytes keeps its encoded forms", k, got[k], b64)
			}
		}
		f.app.Contract(t, maskEchoKey, "result#short", stable(text(res)), opts...)
	})

	t.Run("a value derived from a secret is secret", func(t *testing.T) {
		if _, err := f.app.W.Kept.Put(keptvars.Put{Scope: externalScope, Name: "token2", Value: json.RawMessage(`"tok2-secret-value"`), Secret: true, Source: keptSource}); err != nil {
			t.Fatal(err)
		}
		args := scriptArgs(envID, "GET", srv.URL+"/echo", map[string]any{"transform": `kept.set("derived", kept.get("token2") + "-x"); return 1`})
		// The transform reads a secret, so its response is held for review.
		if res := f.finish(t, f.startHeldAfterCard(t, sess, args), true); res.IsError {
			t.Fatalf("derive = %q", text(res))
		}
		e, ok := f.keptValue(t, "derived")
		if !ok || !e.Secret {
			t.Fatalf("derived = %+v ok=%v, want a secret entry", e.Meta, ok)
		}
		pending := callToolAsync(sess, "api_request", echoCall("{{kept.derived}}", nil))
		f.answerItems(t, true, f.pending(t).Item.ID)
		res := wait(t, pending)
		got := echoed(t, bodyOf(t, text(res)))
		for k, v := range got {
			if v != "{{kept.derived}}" {
				t.Errorf("form %s = %q, want the reference", k, v)
			}
		}
		agent = append(agent, text(res))
		f.noLeak(t, "tok2-secret-value-x", agent...)
	})

	t.Run("a plain kept value is not masked (Known open item)", func(t *testing.T) {
		f.keep(t, externalScope, "plainid", `"p-123456"`)
		pending := callToolAsync(sess, "api_request", echoCall("{{kept.plainid}}", nil))
		f.answerItems(t, true, f.pending(t).Item.ID)
		res := wait(t, pending)
		if got := echoed(t, bodyOf(t, text(res))); got["raw"] != "p-123456" {
			t.Fatalf("raw = %q, want the plain value shown: only a secret is masked", got["raw"])
		}
	})

	t.Run("an API secret variable is masked in all forms", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", echoCall("{{token}}", nil))
		f.answerItems(t, true, f.pending(t).Item.ID)
		res := wait(t, pending)
		for k, v := range echoed(t, bodyOf(t, text(res))) {
			if v != "{{token}}" {
				t.Errorf("form %s = %q, want {{token}}", k, v)
			}
		}
		agent = append(agent, text(res))
		f.noLeak(t, apiSecret, agent...)
	})

	t.Run("the typed value is nowhere", func(t *testing.T) {
		f.noLeak(t, echoValue, agent...)
	})
}
