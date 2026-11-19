package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-db-params.spec.ts (contract e2e-db-params): kept lists and text
// bound as parameters on sqlite, and the kinds that take none.

const (
	dbParamsKey = "e2e-db-params"
	// thousand renders ids 1 to 1000 from a recursive CTE.
	thousandCTE = "with recursive c(x) as (select 1 union all select x+1 from c) select x from c limit 1000"
	// evilWord has a quote, unicode and a second statement.
	evilWord = "Zoë'); drop table notes; --"
)

func TestE2EDbParams(t *testing.T) {
	f := newFixture(t)
	captureLog(t)
	sess := f.session(t)

	t.Run("a script keeps 1000 ids and the next read binds them", func(t *testing.T) {
		// The script reads the rendered result, 200 rows unless maxRows asks for more.
		args := f.queryScriptArgs(thousandCTE, "read", `kept.set("big", result.rows.map(r => Number(r[0]))); return 1;`)
		args["maxRows"] = 1000
		keep := callTool(t, sess, "run_query", args)
		if keep.IsError || !strings.Contains(text(keep), `"name":"big","type":"array","items":1000`) {
			t.Fatalf("keep = %q, want big kept with 1000 items", text(keep))
		}
		if strings.Contains(text(keep), "999,1000") {
			t.Fatalf("keep = %q, want no value in the agent text", text(keep))
		}
		const count = "select count(*) as n from (with recursive c(x) as (select 1 union all select x+1 from c) select x from c limit 2000) where x in (select value from json_each(?))"
		res := callTool(t, sess, "run_query", f.paramsArgs(count, "read", "big"))
		if res.IsError || !strings.Contains(text(res), `["1000"]`) {
			t.Fatalf("count by the 1000 ids = %q, want 1000", text(res))
		}
		f.app.Contract(t, dbParamsKey, "result#count", text(res))
	})

	t.Run("text with a quote, unicode and a statement binds as text", func(t *testing.T) {
		f.mcpSet(t, map[string]string{"write": "prompt"})
		quoted, err := json.Marshal(evilWord)
		if err != nil {
			t.Fatal(err)
		}
		word := f.keep(t, externalScope, "word", string(quoted))
		pending := callToolAsync(sess, "run_query", f.paramsArgs("insert into notes (body) values (?)", "write", "word"))
		req := f.pending(t)
		pl := decodeKept(t, req.Item)
		if len(pl.Params) != 1 || pl.Params[0] != "word" || len(pl.Placeholders) != 1 || pl.Placeholders[0] != "? 1" ||
			len(pl.Kept) != 1 || pl.Kept[0].ID != word.ID || pl.Kept[0].Preview != string(quoted) {
			t.Fatalf("payload = %+v, want word at ? 1 with its preview", pl)
		}
		opts := []flowharness.ContractOption{flowharness.Mask("createdAt", "expiresAt", "cwd"), flowharness.Replace(word.ID, "kv_word"), flowharness.Replace(f.dbPath, "/data/mcp.db")}
		f.app.Contract(t, dbParamsKey, "ApprovalsService.List#write", []approvals.Item{req.Item}, opts...)
		f.app.Contract(t, dbParamsKey, "args:ApprovalsService.Answer#write", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		f.answer(t, true, req.RequestID)
		if res := wait(t, pending); res.IsError {
			t.Fatalf("write = %q, want it to run", text(res))
		}
		if got := f.notesBodies(t); len(got) != 3 || got[2] != evilWord {
			t.Fatalf("notes = %q, want the value stored as text and the table intact", got)
		}
		f.mcpSet(t, map[string]string{"write": "deny"})
	})

	t.Run("kinds without params refuse before connecting", func(t *testing.T) {
		for _, kind := range []string{"mongodb", "redis", "kibana"} {
			// Unreachable on purpose: the refusal comes before any connect.
			conn, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
				Name: "e2e " + kind, Kind: kind, Color: "teal", Mode: "fields", Host: ptr("127.0.0.1"), Port: ptr(1), Options: map[string]any{},
				McpEnabled: true, McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny",
			}})
			if err != nil {
				t.Fatalf("create %s connection: %v", kind, err)
			}
			id := conn.ID
			res := callTool(t, sess, "run_query", map[string]any{"connectionId": id, "sql": "x", "path": "database:none", "mode": "read", "params": []string{"word"}})
			if !res.IsError || !strings.Contains(text(res), "takes no params") || !strings.Contains(text(res), kind) {
				t.Errorf("%s params = %q, want a takes-no-params refusal naming the kind", kind, text(res))
			}
		}
		if n := f.queueLen(t); n != 0 {
			t.Fatalf("queue %d, want none", n)
		}
	})

	t.Run("a value the user typed is refused as a param", func(t *testing.T) {
		const typed = "e2e-typed-secret-7Q9"
		if _, err := f.app.W.Kept.Put(keptvars.Put{
			Scope: externalScope, Name: "apikey", Value: json.RawMessage(`"` + typed + `"`), Secret: true,
			Source: keptvars.Source{Kind: keptvars.SourceUser, MCP: "api", Tool: "api_request", Request: "input apikey", Client: "claude-code"},
		}); err != nil {
			t.Fatal(err)
		}
		res := callTool(t, sess, "run_query", f.paramsArgs("select ? as x", "read", "apikey"))
		if !res.IsError || !strings.Contains(text(res), `kept variable "apikey" is secret`) {
			t.Fatalf("typed value as param = %q, want an is-secret refusal", text(res))
		}
		f.noLeak(t, typed, text(res))
		if got := f.notesBodies(t); len(got) != 3 {
			t.Fatalf("notes = %q, want no change", got)
		}
	})
}
