package dbmcpflow

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// paramsArgs is a run_query call that binds kept variables.
func (f *fixture) paramsArgs(sqlText, mode string, params ...string) map[string]any {
	args := withMode(f.connID, sqlText, mode)
	args["params"] = params
	return args
}

// keptPayload is the part of a db.query or db.exemption payload that carries kept parameters.
type keptPayload struct {
	Statement    string   `json:"statement"`
	Params       []string `json:"params"`
	Placeholders []string `json:"placeholders"`
	Kept         []struct {
		ID, Name, Type, Preview string
		Items                   int
		Sites                   []struct {
			Field           string
			Index, From, To int
		}
	} `json:"kept"`
}

func decodeKept(t *testing.T, it approvals.Item) keptPayload {
	t.Helper()
	var pl keptPayload
	if err := json.Unmarshal(it.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	return pl
}

// notesBodies reads the notes table straight from the fixture file.
func (f *fixture) notesBodies(t *testing.T) []string {
	t.Helper()
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("select body from notes order by id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

// opCommands lists the commands of finished op rows.
func (f *fixture) opCommands(t *testing.T) []string {
	t.Helper()
	recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range recent {
		if r.Command != nil {
			out = append(out, *r.Command)
		}
	}
	return out
}

// TestDbMcpKeptParams: a write binds a kept text value, the card names the variable at its placeholder, and the
// statement runs once approved with the value stored as text (an injection attempt included). A read with
// a kept list binds without a card. The op log names the variable, never the value.
// UI half: tests/ui/studio/dbmcp-kept-params.spec.ts.
func TestDbMcpKeptParams(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	const evil = "x'); drop table notes; --"
	word := f.keep(t, externalScope, "word", `"`+evil+`"`)
	f.keep(t, externalScope, "ids", "[1,2]")

	const write = "insert into notes (body) values (?)"
	pending := callToolAsync(sess, "run_query", f.paramsArgs(write, "write", "word"))
	req := f.pending(t)
	pl := decodeKept(t, req.Item)
	if len(pl.Params) != 1 || pl.Params[0] != "word" || len(pl.Placeholders) != 1 || pl.Placeholders[0] != "? 1" {
		t.Fatalf("params %v placeholders %v, want word at ? 1", pl.Params, pl.Placeholders)
	}
	if len(pl.Kept) != 1 || pl.Kept[0].ID != word.ID || pl.Kept[0].Type != "string" || pl.Kept[0].Preview != `"`+evil+`"` {
		t.Fatalf("kept = %+v, want the word variable with its preview", pl.Kept)
	}
	var stmtSite, paramSite bool
	for _, st := range pl.Kept[0].Sites {
		switch st.Field {
		case "statement":
			stmtSite = pl.Statement[st.From:st.To] == "?"
		case "param":
			paramSite = st.Index == 0
		}
	}
	if !stmtSite || !paramSite {
		t.Fatalf("sites = %+v in %q, want the ? and params[0]", pl.Kept[0].Sites, pl.Statement)
	}
	f.app.Contract(t, "dbmcp-kept-params", "ApprovalsService.List", []approvals.Item{req.Item},
		flowharness.Mask("createdAt", "expiresAt", "cwd"), flowharness.Replace(word.ID, "kv_word"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.app.Contract(t, "dbmcp-kept-params", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})

	f.answer(t, true, req.RequestID)
	if res := wait(t, pending); res.IsError {
		t.Fatalf("write = %q, want it to run", text(res))
	}
	if got := f.notesBodies(t); len(got) != 3 || got[2] != evil {
		t.Fatalf("notes = %q, want the value stored as text", got)
	}

	read := call(t, sess, f.paramsArgs("select body from notes where id in (select value from json_each(?)) order by id", "read", "ids"))
	if read.IsError || !strings.Contains(text(read), "one") || !strings.Contains(text(read), "two") || strings.Contains(text(read), "drop table") {
		t.Fatalf("list read = %q, want rows one and two", text(read))
	}
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("a read queued %d approvals, want none", n)
	}

	var withParams []string
	testx.WaitUntil(t, 10*time.Second, func() bool {
		withParams = withParams[:0]
		for _, c := range f.opCommands(t) {
			if strings.Contains(c, "-- params:") {
				withParams = append(withParams, c)
			}
		}
		return len(withParams) >= 2
	})
	for _, c := range withParams {
		if strings.Contains(c, "drop table") || strings.Contains(c, "[1,2]") || !strings.Contains(c, "{{plain.") {
			t.Errorf("op command = %q, want the placeholder and the variable label only", c)
		}
	}
}

// TestDbMcpKeptParamsGoneWhileWaiting: clearing or replacing a variable while its write waits withdraws the
// card and nothing runs.
func TestDbMcpKeptParamsGoneWhileWaiting(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	meta := f.keep(t, externalScope, "word", `"three"`)

	pending := callToolAsync(sess, "run_query", f.paramsArgs("insert into notes (body) values (?)", "write", "word"))
	f.items(t, 1)
	if _, err := f.app.W.KeptVars.Clear(bridge.KeptClearArgs{IDs: []string{meta.ID}}); err != nil {
		t.Fatal(err)
	}
	out := wait(t, pending)
	if !out.IsError || !strings.Contains(text(out), `"word"`) || !strings.Contains(text(out), "cleared by the user") || !strings.Contains(text(out), "nothing ran") {
		t.Fatalf("cleared = %q, want a cleared-while-waiting refusal naming word", text(out))
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })

	f.keep(t, externalScope, "word", `"four"`)
	pending = callToolAsync(sess, "run_query", f.paramsArgs("insert into notes (body) values (?)", "write", "word"))
	f.items(t, 1)
	f.keep(t, externalScope, "word", `"five"`)
	out = wait(t, pending)
	if !out.IsError || !strings.Contains(text(out), "was replaced") {
		t.Fatalf("replaced = %q, want a replaced-while-waiting refusal", text(out))
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
	if got := f.notesBodies(t); len(got) != 2 {
		t.Fatalf("notes = %q, want the two seeded rows only", got)
	}
}

// TestDbMcpKeptParamsRefusals: params that cannot be bound are refused before any card or run.
func TestDbMcpKeptParamsRefusals(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.keep(t, externalScope, "n", "1")
	f.keep(t, externalScope, "obj", `{"a":1}`)
	f.keep(t, externalScope, "mixed", `[1,"a"]`)
	if _, err := f.app.W.Kept.Put(keptvars.Put{Scope: externalScope, Name: "token", Value: json.RawMessage(`"s3cret"`), Secret: true, Source: keptSource}); err != nil {
		t.Fatal(err)
	}

	explain := f.paramsArgs("select body from notes where id = ?", "read", "n")
	explain["explain"] = true
	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"unknown":          {f.paramsArgs("select ? as x", "read", "nope"), `no saved variable "nope"`},
		"too few names":    {f.paramsArgs("select ?, ? as x", "read", "n"), "2 placeholders but params lists 1"},
		"too many names":   {f.paramsArgs("select ? as x", "read", "n", "n"), "1 placeholders but params lists 2"},
		"second statement": {f.paramsArgs("select ? as x; select 2", "read", "n"), "could not tell what this statement does"},
		"named":            {f.paramsArgs("select :a as x", "read", "n"), "plain ? placeholders"},
		"object":           {f.paramsArgs("select ? as x", "read", "obj"), "is an object"},
		"mixed list":       {f.paramsArgs("select ? as x", "read", "mixed"), "mixes types"},
		"secret":           {f.paramsArgs("select ? as x", "read", "token"), "is secret"},
		"explain":          {explain, "takes no params"},
		"literal":          {f.paramsArgs("select 1 as x", "read", "n"), "0 placeholders but params lists 1"},
	} {
		res := callTool(t, sess, "run_query", c.args)
		if !res.IsError || !strings.Contains(text(res), c.want) {
			t.Errorf("%s = %q, want an error containing %q", name, text(res), c.want)
		}
		if strings.Contains(text(res), "s3cret") {
			t.Errorf("%s leaked a secret value: %q", name, text(res))
		}
	}
	if f.queueLen(t) != 0 {
		t.Fatalf("queue %d, want none", f.queueLen(t))
	}
	if got := f.notesBodies(t); len(got) != 2 {
		t.Fatalf("notes = %q, want the seeded rows", got)
	}
}

// TestDbMcpKeptParamsExemption: an exemption is bound to its params. Repeating the call with other names is
// refused as a different call; with the same names the db.exemption card lists the variables.
// UI half: tests/ui/studio/dbmcp-kept-params.spec.ts.
func TestDbMcpKeptParamsExemption(t *testing.T) {
	f, sess := newExemptionFixture(t)
	meta := f.keep(t, externalScope, "n", "1")
	f.keep(t, externalScope, "m", "2")
	const stmt = "select body from notes where id = ? /* big */"

	id := exemptionID(t, callTool(t, sess, "run_query", f.paramsArgs(stmt, "read", "n")))
	other := f.paramsArgs(stmt, "read", "m")
	other["exemption"] = map[string]any{"id": id, "reason": "needs the full run"}
	if res := callTool(t, sess, "run_query", other); !res.IsError || !strings.Contains(text(res), "different call") {
		t.Fatalf("other params = %q, want a different-call refusal", text(res))
	}

	id = exemptionID(t, callTool(t, sess, "run_query", f.paramsArgs(stmt, "read", "n")))
	same := f.paramsArgs(stmt, "read", "n")
	same["exemption"] = map[string]any{"id": id, "reason": "needs the full run"}
	pending := callToolAsync(sess, "run_query", same)
	items := f.items(t, 1)
	pl := decodeKept(t, items[0])
	if items[0].Kind != approvals.KindDbExemption || len(pl.Params) != 1 || pl.Params[0] != "n" || len(pl.Kept) != 1 || pl.Kept[0].ID != meta.ID {
		t.Fatalf("item = %s %+v, want a db.exemption naming n", items[0].Kind, pl)
	}
	f.app.Contract(t, "dbmcp-kept-params-exemption", "ApprovalsService.List", items,
		flowharness.Mask("createdAt", "expiresAt", "cwd"), flowharness.Replace(meta.ID, "kv_n"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.answerItems(t, true, items[0].ID)
	res := wait(t, pending)
	if res.IsError || !strings.Contains(text(res), "one") {
		t.Fatalf("exempted call = %q, want row one", text(res))
	}
}

// TestDbMcpKeptParamsAllowVerdict: a read the connection allows runs without a card and a variable kept in
// another scope is not found.
func TestDbMcpKeptParamsAllowVerdict(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.keep(t, "session:other", "n", "1")
	f.keep(t, externalScope, "mine", "2")

	if res := callTool(t, sess, "run_query", f.paramsArgs("select body from notes where id = ?", "read", "n")); !res.IsError || !strings.Contains(text(res), `no saved variable "n"`) {
		t.Fatalf("other scope = %q, want not found", text(res))
	}
	res := callTool(t, sess, "run_query", f.paramsArgs("select body from notes where id = ?", "read", "mine"))
	if res.IsError || !strings.Contains(text(res), "two") || strings.Contains(text(res), "one") {
		t.Fatalf("read = %q, want row two only", text(res))
	}
	if f.queueLen(t) != 0 {
		t.Fatalf("queue %d, want none for an allowed read", f.queueLen(t))
	}
	list := text(callTool(t, sess, "list_saved_variables", map[string]any{}))
	if !strings.Contains(list, `"mine"`) || !strings.Contains(list, "params of run_query") {
		t.Fatalf("list = %s, want the variable and the run_query note", list)
	}
}

// TestDbMcpKeptParamsToolSurface: run_query takes params and list_saved_variables is offered with API tools off.
func TestDbMcpKeptParamsToolSurface(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	tools, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var haveList, haveParams bool
	for _, tool := range tools.Tools {
		switch tool.Name {
		case "list_saved_variables":
			haveList = true
		case "run_query":
			raw, merr := json.Marshal(tool.InputSchema)
			if merr != nil {
				t.Fatal(merr)
			}
			haveParams = strings.Contains(string(raw), `"params"`) && strings.Contains(string(raw), `"maxItems":64`)
		}
	}
	if !haveList || !haveParams {
		t.Fatalf("list_saved_variables %v, run_query params %v, want both offered", haveList, haveParams)
	}
}
