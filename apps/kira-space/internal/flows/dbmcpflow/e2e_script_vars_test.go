package dbmcpflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Backend half of tests/ui/studio/e2e-script-vars.spec.ts (contract e2e-script-vars): scripts read and write saved
// variables through the globals plain and secret. A prepare script writes values its own request uses, a transform
// writes and deletes, a kube prepare saves a value, and a refusal table pins every rule that stops a script.

const (
	scriptVarsKey = "e2e-script-vars"
	varsPrepare   = "const region = plain.region;\nplain.ts = 't-' + region;\nsecret.sig = 'sig-' + region;\nreturn {headers: [{name: 'X-Ts', value: '{{plain.ts}}'}], query: [{name: 'sig', value: '{{secret.sig}}'}]};"
	varsTransform = "plain.next = response.json.users[0].id;\nsecret.token = 'tok-' + response.json.users[0].id;\ndelete plain.old;\nreturn {count: response.json.total};"
)

type scriptVarsCard struct {
	Script struct {
		Prepare *struct {
			Source string
			Writes []struct {
				Name, Preview string
				Secret        bool
			}
		}
	}
}

func TestE2EScriptVars(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setAPI(t, true, "always")
	opts := f.e2eOpts(srv.URL)

	t.Run("prepare writes feed its own request and are saved after it", func(t *testing.T) {
		f.setReviewApp(t, false)
		f.keep(t, externalScope, "region", `"eu"`)
		pending := callToolAsync(sess, "api_request", scriptArgs(envID, "GET", srv.URL+"/echo", map[string]any{"prepare": varsPrepare}))
		req := f.pending(t)
		var card scriptVarsCard
		if err := json.Unmarshal(req.Item.Payload, &card); err != nil {
			t.Fatal(err)
		}
		w := card.Script.Prepare.Writes
		if len(w) != 2 || w[0].Name != "ts" || w[0].Secret || w[0].Preview != `"t-eu"` || w[1].Name != "sig" || !w[1].Secret || w[1].Preview != "" {
			t.Fatalf("prepare writes = %+v, want ts with a preview and sig as a secret without one", w)
		}
		if names := f.keptNames(externalScope); len(names) != 1 {
			t.Fatalf("store = %v before Send, want region only", names)
		}
		ro := append([]flowharness.ContractOption{flowharness.Replace(f.keptID(t, externalScope, "region"), "kv_region")}, opts...)
		f.app.Contract(t, scriptVarsKey, "ApprovalsService.List#prepare", []approvals.Item{req.Item}, ro...)
		f.app.Contract(t, scriptVarsKey, "args:ApprovalsService.Answer#prepare", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		f.answerItems(t, true, req.Item.ID)
		res := wait(t, pending)
		if res.IsError {
			t.Fatalf("result = %q", text(res))
		}
		seen := srv.Requests()
		last := seen[len(seen)-1]
		if last.Header.Get("X-Ts") != "t-eu" || !strings.Contains(last.Query, "sig=sig-eu") {
			t.Fatalf("server saw %+v, want the staged values in the header and the query", last)
		}
		if strings.Contains(text(res), "sig-eu") || !strings.Contains(text(res), `"tempSet":[{"name":"ts"`) {
			t.Fatalf("result = %q, want a saved summary and no secret", text(res))
		}
		if e, ok := f.keptValue(t, "sig"); !ok || !e.Secret || string(e.Value) != `"sig-eu"` {
			t.Fatalf("sig = %+v ok=%v, want a saved secret", e.Meta, ok)
		}
		if e, ok := f.keptValue(t, "ts"); !ok || e.Secret || string(e.Value) != `"t-eu"` {
			t.Fatalf("ts = %+v ok=%v, want a saved plain value", e.Meta, ok)
		}
	})

	t.Run("transform writes and deletes after the review", func(t *testing.T) {
		f.setReviewApp(t, true)
		f.keep(t, externalScope, "old", `"x"`)
		pending := callToolAsync(sess, "api_request", scriptArgs(envID, "GET", usersURL(srv.URL), map[string]any{"transform": varsTransform}))
		card := f.pending(t)
		f.app.Contract(t, scriptVarsKey, "ApprovalsService.List#transform", []approvals.Item{card.Item},
			append([]flowharness.ContractOption{flowharness.Replace(f.keptID(t, externalScope, "old"), "kv_old")}, opts...)...)
		f.answerItems(t, true, card.Item.ID)
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
		req := f.pending(t)
		held := f.pendingResponse(t, req.RequestID)
		got := map[string]bool{}
		for _, k := range held.Keep {
			got[k.Name] = k.Deleted
		}
		if len(held.Keep) != 3 || got["next"] || got["token"] || !got["old"] {
			t.Fatalf("held keep = %+v, want next and token saved and old deleted", held.Keep)
		}
		if _, ok := f.keptValue(t, "old"); !ok {
			t.Fatal("old deleted before Send")
		}
		pinned := held
		pinned.Text, pinned.Raw = stable(held.Text), stable(held.Raw)
		f.app.Contract(t, scriptVarsKey, "ApprovalsService.List#review", []approvals.Item{req.Item}, opts...)
		f.app.Contract(t, scriptVarsKey, "DbMcpService.PendingResponse#review", pinned, opts...)
		f.answerItems(t, true, req.Item.ID)
		res := wait(t, pending)
		out := text(res)
		if res.IsError || !strings.Contains(out, `"tempSet":[`) || !strings.Contains(out, `"tempDeleted":["old"]`) || strings.Contains(out, "tok-") {
			t.Fatalf("result = %q, want saved and deleted lists and no token", out)
		}
		if _, ok := f.keptValue(t, "old"); ok {
			t.Error("old still saved after Send")
		}
		if e, ok := f.keptValue(t, "token"); !ok || !e.Secret {
			t.Errorf("token = %+v ok=%v, want a saved secret", e.Meta, ok)
		}
		if _, ok := f.keptValue(t, "next"); !ok {
			t.Error("next not saved")
		}
	})

	t.Run("refusals", func(t *testing.T) {
		f.keep(t, externalScope, "plainone", `1`)
		if _, err := f.app.W.Kept.Put(keptvars.Put{Scope: externalScope, Name: "sec", Value: json.RawMessage(`"s3cr3t"`), Secret: true, Source: keptSource}); err != nil {
			t.Fatal(err)
		}
		before := f.keptNames(externalScope)
		many := ""
		for i := 0; i <= 16; i++ {
			many += fmt.Sprintf("plain.v%d = %d;\n", i, i)
		}
		cases := []struct{ name, script, want string }{
			{"bothKinds", `plain.x = 1; secret.x = 2; return 1;`, "both plain.x and secret.x"},
			{"kindClashPlain", `secret.plainone = 1; return 1;`, `"plainone" is a plain temporary variable`},
			{"kindClashSecret", `plain.sec = 1; return 1;`, `"sec" is`},
			{"invalidName", `plain["1x"] = 1; return 1;`, "not a valid name"},
			{"functionValue", `plain.f = function () {}; return 1;`, "is a function"},
			{"localPlain", `const plain = {}; plain.x = 1; return 1;`, "plain"},
			{"localSecretParam", `secret.y = 1; function f(secret) { return secret; } return f(1);`, "secret"},
			{"dynamicName", `plain.a = 1; const k = "dyn"; plain[k] = 2; return 1;`, "not written literally"},
			{"secretRead", `return plain.sec;`, "is secret"},
			{"tooMany", many + `return 1;`, "16"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got := f.runRefused(t, sess, scriptArgs(envID, "GET", usersURL(srv.URL), map[string]any{"transform": c.script}))
				if !strings.Contains(got, c.want) || strings.Contains(got, "s3cr3t") {
					t.Fatalf("result = %q, want a refusal naming %q", got, c.want)
				}
			})
		}
		t.Run("sqlSecretRead", func(t *testing.T) {
			res := callTool(t, f.session(t), "run_query", f.queryScriptArgs(scriptStmt, "read", `return plain.sec;`))
			if !res.IsError || !strings.Contains(text(res), "is secret") || strings.Contains(text(res), "s3cr3t") {
				t.Fatalf("result = %q, want the secret refusal", text(res))
			}
		})
		if after := f.keptNames(externalScope); len(after) != len(before) {
			t.Fatalf("store = %v after the refusals, want %v: nothing staged", after, before)
		}
	})
}

// runRefused runs a scripted api_request, answering whatever card or review it raises, and returns the agent text.
func (f *fixture) runRefused(t *testing.T, sess *mcp.ClientSession, args map[string]any) string {
	t.Helper()
	pending := callToolAsync(sess, "api_request", args)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case out := <-pending:
			if out.err != nil {
				t.Fatal(out.err)
			}
			return text(out.res)
		default:
		}
		if items := f.list(t); len(items) > 0 {
			f.answerItems(t, true, items[0].ID)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("scripted call did not finish")
	return ""
}

// TestE2EScriptVarsKube: a kube prepare script saves a value; it reaches the store once the read has run.
func TestE2EScriptVarsKube(t *testing.T) {
	k := newKubeFixture(t)
	k.setMcp(t, "kind-dev", on("prompt"))

	pending := k.callAsync("kube.logs", k.logsScript(map[string]any{"prepare": `plain.pod2 = "web-2"; return {name: "web-1", tail: 2};`}))
	req := k.pending(t)
	var card struct {
		Script struct {
			Prepare *struct {
				Writes []struct {
					Name   string
					Secret bool
				}
			}
		}
	}
	if err := json.Unmarshal(req.Item.Payload, &card); err != nil {
		t.Fatal(err)
	}
	if w := card.Script.Prepare.Writes; len(w) != 1 || w[0].Name != "pod2" || w[0].Secret {
		t.Fatalf("prepare writes = %+v, want pod2 as a plain value", w)
	}
	if _, ok := k.keptValue(t, "pod2"); ok {
		t.Fatal("pod2 saved before the read ran")
	}
	k.answerItems(t, true, req.Item.ID)
	out := <-pending
	if out.err != nil || out.res.IsError {
		t.Fatalf("logs = %q (%v)", text(out.res), out.err)
	}
	if e, ok := k.keptValue(t, "pod2"); !ok || string(e.Value) != `"web-2"` || e.Source.MCP != "kube" {
		t.Fatalf("pod2 = %+v ok=%v, want a plain kube-sourced value after the read", e.Meta, ok)
	}
}
