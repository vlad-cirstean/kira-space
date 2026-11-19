package dbmcpflow

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-cross-mcp.spec.ts (contract e2e-cross-mcp): a value kept by one MCP is
// read by the other (API list into SQL params, SQL rows into an API body) with its source named on the card;
// a secret or typed value never crosses into SQL.

const (
	crossKey   = "e2e-cross-mcp"
	crossWrite = "update notes set body = 'hit' where id in (select value from json_each(?))"
	crossKeep  = `kept.set("bodies", result.rows.map(r => r[1])); return result.rows.length;`
)

func TestE2ECrossMcp(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	opts := f.e2eOpts(srv.URL)
	echo := srv.URL + "/echo"
	var agent []string
	// A big id proves the exact digits survive both MCPs.
	f.mcpSet(t, map[string]string{"write": "allow"})
	if res := runQuery(t, sess, f.connID, "insert into notes (id, body) values ("+bigID+", 'big')"); res.IsError {
		t.Fatalf("insert = %q", text(res))
	}
	f.mcpSet(t, map[string]string{"write": "prompt"})
	nothingRan := func(t *testing.T, before []string) {
		t.Helper()
		if n := f.queueLen(t); n != 0 {
			t.Fatalf("queue %d, want a refusal before any card", n)
		}
		if got := f.notesBodies(t); !slices.Equal(got, before) {
			t.Fatalf("notes = %q, want %q", got, before)
		}
	}

	t.Run("API to SQL: a kept id list binds into an update", func(t *testing.T) {
		keep := callToolAsync(sess, "api_request", scriptArgs(envID, "GET", usersURL(srv.URL), map[string]any{"transform": chainKeep}))
		f.answerItems(t, true, f.pending(t).Item.ID)
		if res := wait(t, keep); res.IsError {
			t.Fatalf("keep = %q", text(res))
		}
		pending := callToolAsync(sess, "run_query", f.paramsArgs(crossWrite, "write", "ids"))
		req := f.pending(t)
		pl := decodeKept(t, req.Item)
		var src struct {
			Kept []struct {
				Source struct{ MCP, Tool string }
			}
		}
		if err := json.Unmarshal(req.Item.Payload, &src); err != nil || len(src.Kept) != 1 ||
			src.Kept[0].Source.MCP != "api" || src.Kept[0].Source.Tool != "api_request" || pl.Kept[0].Preview != keptIDs {
			t.Fatalf("kept = %+v (%v), want ids from the API MCP", src.Kept, err)
		}
		ro := append(slices.Clip(opts), flowharness.Replace(f.keptID(t, externalScope, "ids"), "kv_ids"))
		f.app.Contract(t, crossKey, "ApprovalsService.List#apiToSql", []approvals.Item{req.Item}, ro...)
		f.answer(t, true, req.RequestID)
		if res := wait(t, pending); res.IsError {
			t.Fatalf("update = %q", text(res))
		}
		if got := f.notesBodies(t); !slices.Equal(got, []string{"one", "hit", "hit"}) {
			t.Fatalf("notes = %q, want the big id and id 2 updated exactly", got)
		}
	})

	t.Run("SQL to API: kept rows fill a request body", func(t *testing.T) {
		keep := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", crossKeep))
		if keep.IsError || !strings.Contains(text(keep), `"kept":[{"name":"bodies"`) {
			t.Fatalf("keep = %q", text(keep))
		}
		pending := callToolAsync(sess, "api_request", jsonPost(envID, echo, `{"b": {{kept.bodies}}}`))
		req := f.pending(t)
		var pl struct {
			Kept []struct {
				Name   string
				Source struct{ MCP, Tool, Request string }
			}
		}
		if err := json.Unmarshal(req.Item.Payload, &pl); err != nil || len(pl.Kept) != 1 || pl.Kept[0].Name != "bodies" ||
			pl.Kept[0].Source.MCP != "db" || pl.Kept[0].Source.Tool != "run_query" || !strings.HasPrefix(pl.Kept[0].Source.Request, "notes db: select") {
			t.Fatalf("kept = %+v (%v), want bodies from run_query on notes db", pl.Kept, err)
		}
		ro := append(slices.Clip(opts), flowharness.Replace(f.keptID(t, externalScope, "bodies"), "kv_bodies"))
		f.app.Contract(t, crossKey, "ApprovalsService.List#sqlToApi", []approvals.Item{req.Item}, ro...)
		f.answer(t, true, req.RequestID)
		if res := wait(t, pending); res.IsError {
			t.Fatalf("request = %q", text(res))
		}
		seen := srv.Requests()
		if last := seen[len(seen)-1]; string(last.Body) != `{"b": ["one","hit","hit"]}` {
			t.Fatalf("server saw %q, want the rows as a list", last.Body)
		}
	})

	t.Run("a secret derived by an API script stays out of SQL", func(t *testing.T) {
		if _, err := f.app.W.Kept.Put(keptvars.Put{Scope: externalScope, Name: "token2", Value: json.RawMessage(`"tok2-cross-secret"`), Secret: true, Source: keptSource}); err != nil {
			t.Fatal(err)
		}
		derive := callToolAsync(sess, "api_request", scriptArgs(envID, "GET", echo, map[string]any{"transform": `kept.set("tainted", kept.get("token2") + "-y"); return 1`}))
		f.answerItems(t, true, f.pending(t).Item.ID)
		if res := wait(t, derive); res.IsError {
			t.Fatalf("derive = %q", text(res))
		}
		if e, ok := f.keptValue(t, "tainted"); !ok || !e.Secret {
			t.Fatalf("tainted = %+v ok=%v, want a secret entry", e.Meta, ok)
		}
		before := f.notesBodies(t)
		params := callTool(t, sess, "run_query", f.paramsArgs(crossWrite, "write", "tainted"))
		script := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `return kept.get("tainted");`))
		if !params.IsError || !strings.Contains(text(params), `kept variable "tainted" is secret`) {
			t.Fatalf("params = %q, want the secret refusal", text(params))
		}
		if !script.IsError || !strings.Contains(text(script), `kept variable "tainted" is secret or was typed by the user`) {
			t.Fatalf("script = %q, want the secret-or-typed refusal", text(script))
		}
		nothingRan(t, before)
		agent = append(agent, text(params), text(script))
		f.noLeak(t, "tok2-cross-secret", agent...)
		f.noLeak(t, "tok2-cross-secret-y", agent...)
	})

	t.Run("a typed value never reaches SQL", func(t *testing.T) {
		user, err := f.app.W.Kept.Put(keptvars.Put{
			Scope: externalScope, Name: "apikey", Value: json.RawMessage(`"` + e2eTyped + `"`), Secret: true,
			Origin: srv.URL, Source: keptvars.Source{Kind: keptvars.SourceUser, MCP: "api", Tool: "api_request", Request: "input apikey", Client: "claude-code"},
		})
		if err != nil {
			t.Fatal(err)
		}
		before := f.notesBodies(t)
		params := callTool(t, sess, "run_query", f.paramsArgs("select ? as x", "read", "apikey"))
		script := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `return kept.get("apikey");`))
		if !params.IsError || !strings.Contains(text(params), `kept variable "apikey" is secret`) {
			t.Fatalf("params = %q, want the secret refusal", text(params))
		}
		if !script.IsError || !strings.Contains(text(script), `kept variable "apikey" is secret or was typed by the user`) {
			t.Fatalf("script = %q, want the typed-value refusal", text(script))
		}
		nothingRan(t, before)
		agent = append(agent, text(params), text(script))
		f.noLeak(t, e2eTyped, agent...)

		keep := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `kept.set("apikey", 1); return 1;`))
		if keep.IsError || !strings.Contains(text(keep), `"keptNotSaved":[{"name":"apikey","error":"`+keptvars.ErrNameTaken.Error()+`"}]`) {
			t.Fatalf("keep = %q, want keptNotSaved naming the user entry", text(keep))
		}
		if e, ok := f.keptValue(t, "apikey"); !ok || e.ID != user.ID || string(e.Value) != `"`+e2eTyped+`"` {
			t.Fatalf("apikey = %+v ok=%v, want the user entry untouched", e.Meta, ok)
		}
		agent = append(agent, text(keep))
		f.noLeak(t, e2eTyped, agent...)
	})
}
