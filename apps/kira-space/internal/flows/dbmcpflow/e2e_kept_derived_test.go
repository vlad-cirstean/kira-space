package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
)

// Backend half of the derived-value rule (no UI change beyond the card text): a value a script derives from a
// user value keeps its origin and its forced review, and a transform reading any secret is always reviewed.

const derivedSecret = "derived-user-secret-4Z2"

func TestKeptDerivedFromUserValue(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	var agent []string
	put := func(name string, kind keptvars.SourceKind, origin string) keptvars.Meta {
		t.Helper()
		m, err := f.app.W.Kept.Put(keptvars.Put{
			Scope: externalScope, Name: name, Value: json.RawMessage(`"` + derivedSecret + `"`), Secret: true, Origin: origin,
			Source: keptvars.Source{Kind: kind, MCP: "api", Tool: "api_request", Request: "GET " + srv.URL, Client: "claude-code"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	put("pw", keptvars.SourceUser, srv.URL)
	echo := srv.URL + "/echo"

	t.Run("copy keeps the origin and the user taint", func(t *testing.T) {
		args := apiArgs(envID, "GET", echo)
		args["script"] = map[string]any{"transform": `kept.set("d", kept.get("pw")); return 1;`}
		h := f.startHeldAfterCard(t, sess, args)
		if p := f.responseItem(t, h.req.Item); p.Reason != "userValue" {
			t.Fatalf("held reason = %q, want userValue", p.Reason)
		}
		res := f.finish(t, h, true)
		agent = append(agent, text(res))
		d, ok := f.keptValue(t, "d")
		if !ok || res.IsError || d.Origin != srv.URL || !d.Secret || !d.Bound() || d.Source.Kind != keptvars.SourceScript {
			t.Fatalf("d = %+v ok=%v result %q, want a secret script entry bound to %s", d.Meta, ok, text(res), srv.URL)
		}
	})

	t.Run("send to another host is refused", func(t *testing.T) {
		other := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
		sent := len(srv.Requests())
		res := callTool(t, sess, "api_request", apiArgs(envID, "GET", other+"/echo?x={{kept.d}}"))
		agent = append(agent, text(res))
		if !res.IsError || !strings.Contains(text(res), "d") || f.queueLen(t) != 0 || len(srv.Requests()) != sent {
			t.Fatalf("result = %q, queue %d, want a refusal before any card", text(res), f.queueLen(t))
		}
	})

	t.Run("transform reshaping the derived value is reviewed", func(t *testing.T) {
		args := apiArgs(envID, "GET", echo)
		args["script"] = map[string]any{"transform": `return kept.get("d").split("").reverse().join("")`}
		h := f.startHeldAfterCard(t, sess, args)
		if p := f.responseItem(t, h.req.Item); p.Reason != "userValue" {
			t.Fatalf("held reason = %q, want userValue though the review settings are off", p.Reason)
		}
		if strings.Contains(h.held.Text, derivedSecret) {
			t.Fatalf("held text = %q, want no plain value", h.held.Text)
		}
		res := f.finish(t, h, false)
		agent = append(agent, text(res))
		if !res.IsError || strings.Contains(text(res), reverse(derivedSecret)) {
			t.Fatalf("result = %q, want the withheld text and no data", text(res))
		}
	})

	t.Run("any secret read in a transform is reviewed", func(t *testing.T) {
		put("tok", keptvars.SourceScript, "")
		args := apiArgs(envID, "GET", echo)
		args["script"] = map[string]any{"transform": `return kept.get("tok").split("").reverse().join("")`}
		h := f.startHeldAfterCard(t, sess, args)
		if p := f.responseItem(t, h.req.Item); p.Reason != "userValue" {
			t.Fatalf("held reason = %q, want userValue for a secret script value", p.Reason)
		}
		res := f.finish(t, h, false)
		agent = append(agent, text(res))
		if !res.IsError || strings.Contains(text(res), reverse(derivedSecret)) {
			t.Fatalf("result = %q, want the withheld text and no data", text(res))
		}
	})

	t.Run("values of two origins cannot be combined", func(t *testing.T) {
		put("pw2", keptvars.SourceUser, "http://localhost:1")
		args := apiArgs(envID, "GET", echo)
		args["script"] = map[string]any{"transform": `kept.set("mix", kept.get("pw") + kept.get("pw2")); return 1;`}
		h := f.startHeldAfterCard(t, sess, args)
		res := f.finish(t, h, true)
		agent = append(agent, text(res))
		if res.IsError || !strings.Contains(text(res), `"keptNotSaved":[{"name":"mix"`) {
			t.Fatalf("result = %q, want keptNotSaved naming mix", text(res))
		}
		if _, ok := f.keptValue(t, "mix"); ok {
			t.Fatal("mix was stored, want it refused")
		}
	})

	t.Run("the value is nowhere", func(t *testing.T) {
		f.noLeak(t, derivedSecret, agent...)
	})
}
