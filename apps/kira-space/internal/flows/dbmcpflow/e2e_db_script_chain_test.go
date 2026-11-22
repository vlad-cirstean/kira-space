package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-db-script-chain.spec.ts (contract e2e-db-script-chain): a result
// script on run_query with review, the class taken from the statement, and an exemption bound to the
// script.

const scriptChainKey = "e2e-db-script-chain"

func scriptChainOpts(f *fixture, extra ...flowharness.ContractOption) []flowharness.ContractOption {
	return append([]flowharness.ContractOption{flowharness.Mask("createdAt", "expiresAt", "cwd"), flowharness.Replace(f.dbPath, "/data/mcp.db")}, extra...)
}

func TestE2EDbScriptChain(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"read": "allow", "write": "prompt", "ddl": "deny", "review.read": "review"})
	opts := scriptChainOpts(f)

	t.Run("a scripted read is held, nothing is kept before Send", func(t *testing.T) {
		pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", keepIDsScript))
		req := f.pending(t)
		if req.Stage != "response" || req.Class != "read" {
			t.Fatalf("pending = %+v, want a held read result and no query card", req)
		}
		var item struct {
			Scripted bool `json:"scripted"`
			RawBytes int  `json:"rawBytes"`
		}
		if err := json.Unmarshal(req.Item.Payload, &item); err != nil || !item.Scripted || item.RawBytes == 0 {
			t.Fatalf("payload = %s (%v), want a scripted item with rawBytes", req.Item.Payload, err)
		}
		held := f.pendingResponse(t, req.RequestID)
		const envelope = `{"kind":"script","output":{"count":2},"input":{"kind":"tabular","rowCount":2,"truncated":false}}`
		if held.Text != envelope || !strings.Contains(held.Raw, `"rows":[["1","one"],["2","two"]]`) {
			t.Fatalf("held text %s raw %s, want the envelope and the rows", held.Text, held.Raw)
		}
		if len(held.Keep) != 1 || held.Keep[0].Name != "ids" || held.Keep[0].Preview != "[1,2]" {
			t.Fatalf("keep = %+v, want the ids preview", held.Keep)
		}
		if _, ok := f.keptValue(t, "ids"); ok {
			t.Fatal("ids stored while the result was held")
		}
		f.app.Contract(t, scriptChainKey, "ApprovalsService.List#review", []approvals.Item{req.Item}, opts...)
		f.app.Contract(t, scriptChainKey, "DbMcpService.PendingResponse#review", held, opts...)
		f.app.Contract(t, scriptChainKey, "args:ApprovalsService.Answer#review", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		f.answer(t, true, req.RequestID)
		res := wait(t, pending)
		if res.IsError || !strings.Contains(text(res), `"kept":[{"name":"ids","type":"array","items":2,`) {
			t.Fatalf("sent = %q, want the kept summary", text(res))
		}
		if e, ok := f.keptValue(t, "ids"); !ok || string(e.Value) != "[1,2]" {
			t.Fatalf("stored = %s ok=%v, want ids after Send", e.Value, ok)
		}
	})

	t.Run("a script that names a delete does not make the call a write", func(t *testing.T) {
		const script = `const sql = "DELETE FROM notes"; kira.storeValue("x", 1); return sql.length;`
		pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", script))
		req := f.pending(t)
		if req.Stage != "response" || req.Class != "read" {
			t.Fatalf("pending = %+v, want a read-class held result and no write card", req)
		}
		f.answer(t, true, req.RequestID)
		if res := wait(t, pending); res.IsError || !strings.Contains(text(res), `"output":17`) {
			t.Fatalf("result = %q, want the script ran as a read", text(res))
		}
		if got := f.notesBodies(t); len(got) != 2 {
			t.Fatalf("notes = %q, want both rows", got)
		}
	})

	t.Run("a write with a script asks as a write and a denial runs nothing", func(t *testing.T) {
		before := len(f.transformOps(t))
		pending := callToolAsync(sess, "run_query", f.queryScriptArgs("insert into notes (body) values ('three')", "write", "return 1"))
		req := f.pending(t)
		if req.Stage != "query" || req.Class != "write" || decodeScriptPayload(t, req.Item).Script == nil {
			t.Fatalf("pending = %+v, want a write card carrying the script", req)
		}
		f.app.Contract(t, scriptChainKey, "ApprovalsService.List#write", []approvals.Item{req.Item}, opts...)
		f.app.Contract(t, scriptChainKey, "args:ApprovalsService.Answer#deny", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: false})
		f.answer(t, false, req.RequestID)
		res := wait(t, pending)
		if !res.IsError || !strings.Contains(text(res), "denied") {
			t.Fatalf("denied write = %q, want a denial", text(res))
		}
		if got := f.notesBodies(t); len(got) != 2 {
			t.Fatalf("notes = %q, want the insert not to have run", got)
		}
		if after := len(f.transformOps(t)); after != before {
			t.Fatalf("script ops %d -> %d, want the script not to have run", before, after)
		}
	})

	t.Run("ddl with a script is refused by the mode before the script runs", func(t *testing.T) {
		before := len(f.transformOps(t))
		res := callTool(t, sess, "run_query", f.queryScriptArgs("create table scratch (id integer)", "ddl", "return 1"))
		if !res.IsError || !strings.Contains(text(res), "does not allow ddl") {
			t.Fatalf("ddl = %q, want the mode refusal", text(res))
		}
		if after := len(f.transformOps(t)); after != before {
			t.Fatalf("script ops %d -> %d, want none", before, after)
		}
		if n := f.queueLen(t); n != 0 {
			t.Fatalf("queue %d, want none", n)
		}
	})

	t.Run("a masked column reaches the script and the review redacted", func(t *testing.T) {
		f.maskColumn(t, "body")
		pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", "return result.rows;"))
		req := f.pending(t)
		held := f.pendingResponse(t, req.RequestID)
		if !strings.Contains(held.Raw, "[redacted]") || strings.Contains(held.Raw, "one") || strings.Contains(held.Text, "one") {
			t.Fatalf("held raw %s text %s, want the masked render", held.Raw, held.Text)
		}
		f.app.Contract(t, scriptChainKey, "DbMcpService.PendingResponse#masked", held, opts...)
		f.answer(t, true, req.RequestID)
		res := wait(t, pending)
		if res.IsError || !strings.Contains(text(res), "[redacted]") || strings.Contains(text(res), "one") || strings.Contains(text(res), "two") {
			t.Fatalf("sent = %q, want redacted cells only", text(res))
		}
	})
}

// TestE2EDbScriptChainExemption: the exemption binds statement, script and kept params together.
func TestE2EDbScriptChainExemption(t *testing.T) {
	f, sess := newExemptionFixture(t)
	ids := f.keep(t, externalScope, "ids", "[1,2]")
	const stmt = "select body from notes where id in (select value from json_each(?)) /* big */"
	const scriptA = `return result.rows.map(r => r[0]);`
	const scriptB = `return result.rows.length;`
	withExemption := func(script, id string) map[string]any {
		args := f.queryScriptArgs(stmt, "read", script, "ids")
		args["exemption"] = map[string]any{"id": id, "reason": "needs the full run"}
		return args
	}

	id := exemptionID(t, callTool(t, sess, "run_query", f.queryScriptArgs(stmt, "read", scriptA, "ids")))
	if res := callTool(t, sess, "run_query", withExemption(scriptB, id)); !res.IsError || !strings.Contains(text(res), "different call") {
		t.Fatalf("other script = %q, want a different-call refusal", text(res))
	}

	id = exemptionID(t, callTool(t, sess, "run_query", f.queryScriptArgs(stmt, "read", scriptA, "ids")))
	pending := callToolAsync(sess, "run_query", withExemption(scriptA, id))
	items := f.items(t, 1)
	pl := decodeScriptPayload(t, items[0])
	kp := decodeKept(t, items[0])
	if items[0].Kind != approvals.KindDbExemption || pl.Script == nil || pl.Script.Source != scriptA || len(kp.Params) != 1 || kp.Params[0] != "ids" {
		t.Fatalf("item = %s %s, want a db.exemption with the script and params ids", items[0].Kind, items[0].Payload)
	}
	f.app.Contract(t, scriptChainKey, "ApprovalsService.List#exemption", items, scriptChainOpts(f, flowharness.Replace(ids.ID, "kv_ids"))...)
	f.app.Contract(t, scriptChainKey, "args:ApprovalsService.Answer#exemption", approvals.AnswerArgs{IDs: []string{items[0].ID}, Approve: true})
	f.answerItems(t, true, items[0].ID)
	res := wait(t, pending)
	if res.IsError || !strings.Contains(text(res), `"output":["one","two"]`) {
		t.Fatalf("exempted call = %q, want the script output", text(res))
	}

	// The approved id is spent: the same triple needs a new refusal and a new id.
	if res := callTool(t, sess, "run_query", withExemption(scriptA, id)); !res.IsError || !strings.Contains(text(res), "unknown, used or expired") {
		t.Fatalf("spent id = %q, want an unknown-or-used refusal", text(res))
	}
}
