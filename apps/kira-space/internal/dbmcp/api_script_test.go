package dbmcp

import (
	"encoding/json"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/jsengine"
)

func TestApplyPrepared(t *testing.T) {
	agent := agentapi.Input{
		Method: "POST", URL: "https://x.test/a", Body: "agent", ContentType: "text/plain",
		Headers: []agentapi.NameValue{{Name: "X-A", Value: "1"}},
	}
	obj := func(js string) jsengine.Result { return jsengine.Result{JSON: json.RawMessage(js), Type: "object"} }
	cases := []struct {
		name    string
		in      agentapi.Input
		out     jsengine.Result
		want    func(t *testing.T, got agentapi.Input)
		changed []string
		err     string
	}{
		{
			name: "partial keeps the rest", in: agent, out: obj(`{"body":"mine"}`), changed: []string{"body"},
			want: func(t *testing.T, g agentapi.Input) {
				if g.Body != "mine" || g.URL != agent.URL || g.ContentType != "text/plain" || len(g.Headers) != 1 {
					t.Fatalf("got %+v", g)
				}
			},
		},
		{
			name: "object body is compact json with application/json", in: agentapi.Input{Method: "POST", URL: "u"},
			out: obj(`{"body":{"items":[{"id":1}]}}`), changed: []string{"body", "contentType"},
			want: func(t *testing.T, g agentapi.Input) {
				if g.Body != `{"items":[{"id":1}]}` || g.ContentType != "application/json" {
					t.Fatalf("got %+v", g)
				}
			},
		},
		{
			name: "object body keeps the agent content type", in: agentapi.Input{Method: "POST", URL: "u", ContentType: "application/vnd.x+json"},
			out: obj(`{"body":{"a":1}}`), changed: []string{"body"},
			want: func(t *testing.T, g agentapi.Input) {
				if g.ContentType != "application/vnd.x+json" {
					t.Fatalf("got %+v", g)
				}
			},
		},
		{
			name: "big integer stays exact", in: agentapi.Input{Method: "POST", URL: "u"},
			out: obj(`{"body":{"id":9007199254740993}}`), changed: []string{"body", "contentType"},
			want: func(t *testing.T, g agentapi.Input) {
				if g.Body != `{"id":9007199254740993}` {
					t.Fatalf("got %q", g.Body)
				}
			},
		},
		{name: "string body with agent form", in: agentapi.Input{Method: "POST", URL: "u", Form: []agentapi.NameValue{{Name: "a", Value: "b"}}}, out: obj(`{"body":"x"}`), err: "body and form cannot both be set"},
		{name: "form with agent body", in: agent, out: obj(`{"form":[{"name":"a","value":"b"}]}`), err: "body and form cannot both be set"},
		{name: "unknown key", in: agent, out: obj(`{"timeoutSeconds":5}`), err: `unknown key "timeoutSeconds"; allowed: method, url, headers, query, body, contentType, form`},
		{name: "member type with path", in: agent, out: obj(`{"headers":[{"name":"a","value":"b"},{"name":"c","value":"d"},{"name":"e","value":3}]}`), err: "headers[2].value must be a string"},
		{name: "method type", in: agent, out: obj(`{"method":5}`), err: "method must be a string"},
		{name: "body type", in: agent, out: obj(`{"body":5}`), err: "body must be a string, object or array"},
		{name: "not an object", in: agent, out: jsengine.Result{JSON: json.RawMessage(`42`), Type: "number"}, err: "the script must return an object, not number"},
		{
			name: "changed lists exactly the returned keys", in: agent,
			out: obj(`{"url":"https://y.test","query":[{"name":"q","value":"1"}],"method":"PUT"}`), changed: []string{"method", "url", "query"},
			want: func(t *testing.T, g agentapi.Input) {
				if g.Method != "PUT" || g.URL != "https://y.test" || len(g.Query) != 1 {
					t.Fatalf("got %+v", g)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed, err := applyPrepared(c.in, c.out)
			if c.err != "" {
				if err == nil || err.Error() != c.err {
					t.Fatalf("err = %v, want %q", err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(changed) != len(c.changed) {
				t.Fatalf("changed = %v, want %v", changed, c.changed)
			}
			for i := range changed {
				if changed[i] != c.changed[i] {
					t.Fatalf("changed = %v, want %v", changed, c.changed)
				}
			}
			c.want(t, got)
		})
	}
}
