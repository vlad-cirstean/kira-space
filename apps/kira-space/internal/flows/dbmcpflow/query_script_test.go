package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/jsengine"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/approvals-db-script.spec.ts (contracts dbmcp-query-script and
// dbmcp-query-script-kept) and approvals-db-script-review.spec.ts (contract dbmcp-query-script-review): the
// result transform of run_query, its review and the values it keeps.

const (
	scriptStmt = "select id, body from notes order by id"
	// pickBodies is the script of the plain scenario: only the bodies reach the agent.
	pickBodies = `return result.rows.map(r => r[1]);`
	// bodiesEnvelope is what the agent receives for pickBodies on the two seeded notes.
	bodiesEnvelope = `{"kind":"script","output":["one","two"],"input":{"kind":"tabular","rowCount":2,"truncated":false}}`
	keepIDsScript  = `kept.set("ids", result.rows.map(r => Number(r[0]))); return {count: result.rows.length};`
	chainStmt      = "select body from notes where id in (select value from json_each(?)) order by id"
)

// queryScriptArgs is a run_query call with a result script.
func (f *fixture) queryScriptArgs(sqlText, mode, script string, params ...string) map[string]any {
	args := withMode(f.connID, sqlText, mode)
	args["script"] = script
	if len(params) > 0 {
		args["params"] = params
	}
	return args
}

type queryScriptPayload struct {
	Statement string `json:"statement"`
	Script    *struct {
		Source  string `json:"source"`
		Hash    string `json:"hash"`
		MayKeep []struct {
			Name  string `json:"name"`
			Sites []struct {
				Field, Part string
				From, To    int
			} `json:"sites"`
		} `json:"mayKeep"`
	} `json:"script"`
	Kept []struct {
		ID, Name string
		Sites    []struct {
			Field, Part string
			From, To    int
		}
	} `json:"kept"`
}

func decodeScriptPayload(t *testing.T, it approvals.Item) queryScriptPayload {
	t.Helper()
	var pl queryScriptPayload
	if err := json.Unmarshal(it.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	return pl
}

// scriptOps lists the finished op commands of kind jsTransform.
func (f *fixture) transformOps(t *testing.T) []string {
	t.Helper()
	recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range recent {
		if r.Kind == "jsTransform" && r.Status != "running" && r.Command != nil {
			out = append(out, *r.Command)
		}
	}
	return out
}

// TestDbMcpQueryScript: the card shows the script next to the statement; the agent gets the script's output
// and input metadata only, never the rows, and the op log records the script.
func TestDbMcpQueryScript(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"read": "prompt"})

	pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", pickBodies))
	req := f.pending(t)
	pl := decodeScriptPayload(t, req.Item)
	if pl.Script == nil || pl.Script.Source != pickBodies || pl.Script.Hash != jsengine.SourceHash(pickBodies) || len(pl.Script.MayKeep) != 0 {
		t.Fatalf("payload script = %+v, want the source and its hash", pl.Script)
	}
	f.app.Contract(t, "dbmcp-query-script", "ApprovalsService.List", []approvals.Item{req.Item},
		flowharness.Mask("createdAt", "expiresAt", "cwd"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.app.Contract(t, "dbmcp-query-script", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})

	f.answer(t, true, req.RequestID)
	res := wait(t, pending)
	if res.IsError || text(res) != bodiesEnvelope {
		t.Fatalf("result = %s, want %s", text(res), bodiesEnvelope)
	}
	if strings.Contains(text(res), `"rows"`) || strings.Contains(text(res), `"columns"`) {
		t.Fatalf("result = %s, want no rows or columns", text(res))
	}
	testx.WaitUntil(t, 10*time.Second, func() bool {
		for _, c := range f.transformOps(t) {
			if c == pickBodies {
				return true
			}
		}
		return false
	})
}

// TestDbMcpQueryScriptReview: a held scripted result has two views and the script; Send returns the
// envelope, Withhold returns no data and keeps nothing, and both texts count against the held caps.
func TestDbMcpQueryScriptReview(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"review.read": "review"})
	opts := []flowharness.ContractOption{flowharness.Mask("createdAt", "expiresAt", "cwd"), flowharness.Replace(f.dbPath, "/data/mcp.db")}

	pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", keepIDsScript))
	req := f.pending(t)
	var item struct {
		Scripted bool `json:"scripted"`
		Bytes    int  `json:"bytes"`
		RawBytes int  `json:"rawBytes"`
	}
	if err := json.Unmarshal(req.Item.Payload, &item); err != nil {
		t.Fatal(err)
	}
	held := f.pendingResponse(t, req.RequestID)
	const envelope = `{"kind":"script","output":{"count":2},"input":{"kind":"tabular","rowCount":2,"truncated":false}}`
	if !item.Scripted || item.Bytes != len(envelope) || item.RawBytes != len(held.Raw) || item.RawBytes == 0 {
		t.Fatalf("payload = %+v, want a scripted item sized by both texts", item)
	}
	if held.Text != envelope || !strings.Contains(held.Raw, `"rows":[["1","one"],["2","two"]]`) || strings.Contains(held.Text, "one") {
		t.Fatalf("pending text %s raw %s, want the envelope and the rendered rows", held.Text, held.Raw)
	}
	if held.Script == nil || held.Script.Source != keepIDsScript || held.Script.Hash != jsengine.SourceHash(keepIDsScript) {
		t.Fatalf("pending script = %+v, want the script and its hash", held.Script)
	}
	if len(held.Keep) != 1 || held.Keep[0].Name != "ids" || held.Keep[0].Preview != "[1,2]" || held.Keep[0].Items != 2 {
		t.Fatalf("pending keep = %+v, want the ids preview", held.Keep)
	}
	listJSON, _ := json.Marshal(f.list(t))
	if strings.Contains(string(listJSON), `"one"`) {
		t.Error("the approvals list carries a cell value")
	}
	if _, ok := f.keptValue(t, "ids"); ok {
		t.Fatal("ids stored while the result was held")
	}
	f.app.Contract(t, "dbmcp-query-script-review", "ApprovalsService.List", []approvals.Item{req.Item}, opts...)
	f.app.Contract(t, "dbmcp-query-script-review", "DbMcpService.PendingResponse", held, opts...)
	f.app.Contract(t, "dbmcp-query-script-review", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})

	f.answer(t, true, req.RequestID)
	res := wait(t, pending)
	if res.IsError || !strings.HasPrefix(text(res), strings.TrimSuffix(envelope, "}")) || !strings.Contains(text(res), `"kept":[{"name":"ids","type":"array","items":2,`) {
		t.Fatalf("sent = %s, want the envelope and the kept summary", text(res))
	}
	if e, ok := f.keptValue(t, "ids"); !ok || string(e.Value) != "[1,2]" {
		t.Fatalf("stored = %s ok=%v, want ids after the send", e.Value, ok)
	}
	f.app.Contract(t, "dbmcp-query-script-review", "result", text(res), flowharness.Mask("expiresInSeconds"))

	stored, _ := f.keptValue(t, "ids")
	if _, err := f.app.W.KeptVars.Clear(bridge.KeptClearArgs{IDs: []string{stored.ID}}); err != nil {
		t.Fatal(err)
	}
	pending = callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", keepIDsScript))
	f.answer(t, false, f.pending(t).RequestID)
	if out := wait(t, pending); !out.IsError || text(out) != withheldR {
		t.Fatalf("withheld = %q, want %q", text(out), withheldR)
	}
	if _, ok := f.keptValue(t, "ids"); ok {
		t.Fatal("a withheld result kept a value")
	}
}

// TestDbMcpQueryScriptReviewCaps: the held-result cap counts the script, the raw result and the text, so a
// result that fits unscripted is withheld unseen once a script adds its two.
func TestDbMcpQueryScriptReviewCaps(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"review.read": "review"})

	pending := callToolAsync(sess, "run_query", withMode(f.connID, scriptStmt, "read"))
	req := f.pending(t)
	plain := req.Bytes
	f.answer(t, true, req.RequestID)
	wait(t, pending)

	f.app.W.DbMcp.Approvals.SetHeldCaps(plain, plain)
	pending = callToolAsync(sess, "run_query", withMode(f.connID, scriptStmt, "read"))
	req = f.pending(t)
	f.answer(t, true, req.RequestID)
	if out := wait(t, pending); out.IsError {
		t.Fatalf("unscripted at the cap = %q, want it held and sent", text(out))
	}

	out := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", pickBodies))
	if !out.IsError || !strings.Contains(text(out), "review limit, so it was withheld unseen") || strings.Contains(text(out), "one") {
		t.Fatalf("scripted over the cap = %q, want a too-large withhold with no data", text(out))
	}
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("queue %d, want none for an unseen withhold", n)
	}
}

// TestDbMcpQueryScriptKeep: a script keeps ids from one query, the agent learns the name only, and the next
// query binds them as params. The source request is the redacted statement. A script reads a kept value
// and the card marks the call.
func TestDbMcpQueryScriptKeep(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)

	res := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", keepIDsScript))
	if res.IsError || !strings.Contains(text(res), `"output":{"count":2}`) || !strings.Contains(text(res), `"kept":[{"name":"ids","type":"array","items":2,"expiresInSeconds":`) {
		t.Fatalf("result = %s, want the count and the kept summary", text(res))
	}
	if strings.Contains(text(res), "[1,2]") || strings.Contains(text(res), "one") {
		t.Fatalf("result = %s, want no value or row", text(res))
	}
	e, ok := f.keptValue(t, "ids")
	if !ok || string(e.Value) != "[1,2]" || e.Source.MCP != "db" || e.Source.Tool != "run_query" || e.Source.Kind != keptvars.SourceScript ||
		e.Source.Request != "notes db: select id, body from notes order by id" {
		t.Fatalf("stored = %+v ok=%v, want ids from the db run_query script", e, ok)
	}

	chain := callTool(t, sess, "run_query", f.paramsArgs(chainStmt, "read", "ids"))
	if chain.IsError || !strings.Contains(text(chain), "one") || !strings.Contains(text(chain), "two") {
		t.Fatalf("chained read = %q, want both rows", text(chain))
	}

	f.mcpSet(t, map[string]string{"read": "prompt"})
	const readKeep = `kept.set("n", result.rows.length); return kept.get("ids").length;`
	pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", readKeep))
	req := f.pending(t)
	pl := decodeScriptPayload(t, req.Item)
	if len(pl.Kept) != 1 || pl.Kept[0].ID != e.ID || len(pl.Kept[0].Sites) != 1 || pl.Kept[0].Sites[0].Field != "script" || pl.Kept[0].Sites[0].Part != "transform" {
		t.Fatalf("payload kept = %+v, want ids read in the script", pl.Kept)
	}
	if got := readKeep[pl.Kept[0].Sites[0].From:pl.Kept[0].Sites[0].To]; !strings.Contains(got, "kept.get") {
		t.Fatalf("read site = %q in %q, want the kept.get call", got, readKeep)
	}
	if pl.Script == nil || len(pl.Script.MayKeep) != 1 || pl.Script.MayKeep[0].Name != "n" {
		t.Fatalf("payload script = %+v, want n as a name it may keep", pl.Script)
	}
	f.app.Contract(t, "dbmcp-query-script-kept", "ApprovalsService.List", []approvals.Item{req.Item},
		flowharness.Mask("createdAt", "expiresAt", "cwd"), flowharness.Replace(e.ID, "kv_ids"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.answer(t, true, req.RequestID)
	if res := wait(t, pending); res.IsError || !strings.Contains(text(res), `"output":2`) || !strings.Contains(text(res), `"name":"n"`) {
		t.Fatalf("read script = %q, want the length and n kept", text(res))
	}
}

// TestDbMcpQueryScriptMasked: the script reads the masked rows, never the stored values, and the held raw
// view is that same masked render.
func TestDbMcpQueryScriptMasked(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.maskColumn(t, "body")

	res := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", "return result.rows;"))
	if res.IsError || !strings.Contains(text(res), "[redacted]") || strings.Contains(text(res), "one") || strings.Contains(text(res), "two") {
		t.Fatalf("masked script = %q, want redacted cells only", text(res))
	}

	f.mcpSet(t, map[string]string{"review.read": "review"})
	pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", "return result.rowCount;"))
	req := f.pending(t)
	held := f.pendingResponse(t, req.RequestID)
	if !strings.Contains(held.Raw, "[redacted]") || strings.Contains(held.Raw, "one") || strings.Contains(held.Text, "one") {
		t.Fatalf("held raw %s text %s, want the masked render", held.Raw, held.Text)
	}
	f.answer(t, true, req.RequestID)
	wait(t, pending)
}

// TestDbMcpQueryScriptErrors: refusals before the connection is touched, and failures after the statement ran.
func TestDbMcpQueryScriptErrors(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"write": "allow"})
	if _, err := f.app.W.Kept.Put(keptvars.Put{Scope: externalScope, Name: "token", Value: json.RawMessage(`"s3cret"`), Secret: true, Source: keptSource}); err != nil {
		t.Fatal(err)
	}
	explain := f.queryScriptArgs(scriptStmt, "read", "return 1")
	explain["explain"] = true
	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"syntax":        {f.queryScriptArgs(scriptStmt, "read", "return ("), "script: script syntax error"},
		"explain":       {explain, "does not apply to explain"},
		"unknown kept":  {f.queryScriptArgs(scriptStmt, "read", `return kept.get("nope")`), `no kept variable "nope"`},
		"secret kept":   {f.queryScriptArgs(scriptStmt, "read", `return kept.get("token")`), "is secret or was typed by the user"},
		"write as read": {f.queryScriptArgs("insert into notes (body) values ('x')", "read", "return 1"), "mode"},
	} {
		res := callTool(t, sess, "run_query", c.args)
		if !res.IsError || !strings.Contains(text(res), c.want) {
			t.Errorf("%s = %q, want an error containing %q", name, text(res), c.want)
		}
		if strings.Contains(text(res), "s3cret") {
			t.Errorf("%s leaked a secret: %q", name, text(res))
		}
	}
	if f.queueLen(t) != 0 {
		t.Fatalf("queue %d, want none", f.queueLen(t))
	}
	if got := f.notesBodies(t); len(got) != 2 {
		t.Fatalf("notes = %q, want the seeded rows", got)
	}

	read := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `kept.set("x", 1); throw new Error("boom")`))
	if !read.IsError || !strings.Contains(text(read), "line 1") || !strings.Contains(text(read), "boom") || !strings.Contains(text(read), "its result is not returned") || strings.Contains(text(read), "Its changes stay") {
		t.Fatalf("read failure = %q, want a positioned error with no changes-stay note", text(read))
	}
	if _, ok := f.keptValue(t, "x"); ok {
		t.Fatal("a failed script kept a value")
	}

	write := callTool(t, sess, "run_query", f.queryScriptArgs("insert into notes (body) values ('three')", "write", `throw new Error("boom")`))
	if !write.IsError || !strings.Contains(text(write), "Its changes stay. Do not rerun it.") {
		t.Fatalf("write failure = %q, want the changes-stay note", text(write))
	}
	if got := f.notesBodies(t); len(got) != 3 {
		t.Fatalf("notes = %q, want the insert to have run", got)
	}

	slow := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", "while (true) {}"))
	if !slow.IsError || !strings.Contains(text(slow), "timeout") {
		t.Fatalf("endless script = %q, want a timeout", text(slow))
	}

	f.mcpSet(t, map[string]string{"write": "deny"})
	denied := callTool(t, sess, "run_query", f.queryScriptArgs("insert into notes (body) values ('four')", "write", "return 1"))
	if !denied.IsError || !strings.Contains(text(denied), "does not allow write") {
		t.Fatalf("denied write = %q, want the mode refusal whatever the script", text(denied))
	}
	if got := f.notesBodies(t); len(got) != 3 {
		t.Fatalf("notes = %q, want no further insert", got)
	}
}

// TestDbMcpQueryScriptGoneWhileWaiting: clearing a kept value the script reads while the card waits withdraws
// it and nothing runs.
func TestDbMcpQueryScriptGoneWhileWaiting(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"read": "prompt"})
	meta := f.keep(t, externalScope, "ids", "[1,2]")

	pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `return kept.get("ids")`))
	f.items(t, 1)
	if _, err := f.app.W.KeptVars.Clear(bridge.KeptClearArgs{IDs: []string{meta.ID}}); err != nil {
		t.Fatal(err)
	}
	out := wait(t, pending)
	if !out.IsError || !strings.Contains(text(out), "cleared by the user") || !strings.Contains(text(out), "nothing ran") {
		t.Fatalf("cleared = %q, want a cleared-while-waiting refusal", text(out))
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
}

// TestDbMcpQueryScriptExemption: an exemption is bound to its script; another script is a different call.
func TestDbMcpQueryScriptExemption(t *testing.T) {
	f, sess := newExemptionFixture(t)
	const stmt = "select body from notes where id = 1 /* big */"
	const firstCol = `return result.rows.map(r => r[0]);`

	id := exemptionID(t, callTool(t, sess, "run_query", f.queryScriptArgs(stmt, "read", firstCol)))
	other := f.queryScriptArgs(stmt, "read", "return 2")
	other["exemption"] = map[string]any{"id": id, "reason": "needs the full run"}
	if res := callTool(t, sess, "run_query", other); !res.IsError || !strings.Contains(text(res), "different call") {
		t.Fatalf("other script = %q, want a different-call refusal", text(res))
	}

	id = exemptionID(t, callTool(t, sess, "run_query", f.queryScriptArgs(stmt, "read", firstCol)))
	same := f.queryScriptArgs(stmt, "read", firstCol)
	same["exemption"] = map[string]any{"id": id, "reason": "needs the full run"}
	pending := callToolAsync(sess, "run_query", same)
	items := f.items(t, 1)
	if pl := decodeScriptPayload(t, items[0]); items[0].Kind != approvals.KindDbExemption || pl.Script == nil || pl.Script.Source != firstCol {
		t.Fatalf("item = %s %+v, want a db.exemption carrying the script", items[0].Kind, pl)
	}
	f.answerItems(t, true, items[0].ID)
	if res := wait(t, pending); res.IsError || !strings.Contains(text(res), `"output":["one"]`) {
		t.Fatalf("exempted call = %q, want the script output", text(res))
	}
}

// TestDbMcpQueryScriptSetting: the script argument and its text follow the agent scripts setting, and a call
// with the setting off is refused.
func TestDbMcpQueryScriptSetting(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	described := func() (schema, suffix bool) {
		list, err := sess.ListTools(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range list.Tools {
			if tool.Name == "run_query" {
				b, _ := json.Marshal(tool.InputSchema)
				return strings.Contains(string(b), `"script"`), strings.Contains(tool.Description, "result script") || strings.Contains(tool.Description, "Optional script")
			}
		}
		t.Fatal("run_query not listed")
		return
	}
	if s, d := described(); !s || !d {
		t.Fatalf("scripts on: schema %v, description %v", s, d)
	}
	off := false
	if _, err := f.app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{APIScripts: &off}}}); err != nil {
		t.Fatal(err)
	}
	if s, d := described(); s || d {
		t.Fatalf("scripts off: schema %v, description %v", s, d)
	}
	res := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", "return 1"))
	if !res.IsError || !strings.Contains(text(res), "unexpected additional properties") {
		t.Fatalf("scripts off call = %q", text(res))
	}
	if got := text(callTool(t, sess, "run_query", withMode(f.connID, scriptStmt, "read"))); !strings.Contains(got, `"kind":"tabular"`) {
		t.Fatalf("plain read with scripts off = %q, want rows", got)
	}
}
