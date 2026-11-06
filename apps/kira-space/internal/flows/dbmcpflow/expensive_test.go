package dbmcpflow

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Backend half of tests/ui/connection-mcp-expensive.spec.ts: a read-class SELECT whose plan estimates
// at least advanced.expensiveQueryRows rows is refused before it runs, the exemption path asks the
// user, and list_connections reports the check per engine. Contract dbmcp-expensive-query.

const expensiveThreshold = 1000

var rowsMeasuredRe = regexp.MustCompile(`measured (\d+) rows, limit (\d+) rows`)

func (f *fixture) setExpensiveThreshold(t *testing.T, rows int) {
	t.Helper()
	patch := model.SettingsPatch{Advanced: &model.AdvancedPatch{ExpensiveQueryRows: &rows}}
	if _, err := f.app.W.Settings.Set(bridge.SettingsSetArgs{Patch: patch}); err != nil {
		t.Fatalf("Settings.Set: %v", err)
	}
}

// mcpCommands records the command of every MCP execute op that ends.
func (f *fixture) mcpCommands() (get func() []string, stop func()) {
	events, unsub := f.app.W.Router.Host().Subscribe()
	var mu sync.Mutex
	mcpOps := map[string]bool{}
	var cmds []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range events {
			var p struct {
				OpID, Kind, Source string
				Command            *string
			}
			_ = json.Unmarshal(e.Payload, &p)
			mu.Lock()
			switch e.Topic {
			case oplog.DbEventOpStart:
				mcpOps[p.OpID] = p.Kind == "execute" && p.Source == "mcp"
			case oplog.DbEventOpEnd:
				if mcpOps[p.OpID] {
					c := ""
					if p.Command != nil {
						c = *p.Command
					}
					cmds = append(cmds, c)
				}
			}
			mu.Unlock()
		}
	}()
	return func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), cmds...)
		}, func() {
			unsub()
			<-done
		}
}

// setAutoExplain flips mcp.auto_explain in the connection's file, the way the MCP tab does.
func (f *fixture) setAutoExplain(t *testing.T, connID string, on bool) {
	t.Helper()
	list, err := f.app.W.Connections.List()
	if err != nil {
		t.Fatal(err)
	}
	var file string
	for _, c := range list {
		if c.ID == connID {
			file = c.File
		}
	}
	cfg := f.app.W.ConnectionConfig
	detail, err := cfg.File(bridge.ConnectionFileArgs{File: file})
	if err != nil {
		t.Fatal(err)
	}
	// The default (on) is omitted from the file.
	if !strings.Contains(detail.Text, "\nmcp:\n") {
		t.Fatalf("connection file has no mcp block:\n%s", detail.Text)
	}
	text := strings.Replace(detail.Text, "\nmcp:\n", "\nmcp:\n  auto_explain: false\n", 1)
	if on {
		text = strings.Replace(detail.Text, "  auto_explain: false\n", "", 1)
	}
	if saved, err := cfg.SaveFile(bridge.ConnectionSaveFileArgs{File: file, Text: text, BaseHash: detail.Hash}); err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v (%v)", saved, err)
	}
}

func resultPlan(t *testing.T, res *mcp.CallToolResult) (rows float64, ok bool) {
	t.Helper()
	var out struct {
		Plan *struct {
			EstimatedRowsRead *float64 `json:"estimatedRowsRead"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatalf("result %q: %v", text(res), err)
	}
	if out.Plan == nil || out.Plan.EstimatedRowsRead == nil {
		return 0, false
	}
	return *out.Plan.EstimatedRowsRead, true
}

func TestDbMcpExpensiveQueryPostgres(t *testing.T) {
	flowharness.Complete(t)
	flowharness.RequireDocker(t)
	const pass, dbName = "flowpass", "flowdb"
	host, port := startPostgres(t, pass, dbName)
	f := newFixture(t)
	f.setExpensiveThreshold(t, expensiveThreshold)
	pg, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "flow pg", Kind: "postgres", Color: "indigo", Mode: "fields",
		Host: ptr(host), Port: ptr(port), Database: ptr(dbName), Username: ptr("postgres"),
		Options: map[string]any{"sslmode": "disable"}, McpEnabled: true, McpAutoExplain: true,
		McpReadMode: "allow", McpWriteMode: "allow", McpDdlMode: "allow",
	}, Password: ptr(pass)})
	if err != nil {
		t.Fatal(err)
	}
	sess := f.session(t)
	path := "database:" + dbName
	run := func(sqlText, mode string) *mcp.CallToolResult {
		t.Helper()
		args := withMode(pg.ID, sqlText, mode)
		args["path"] = path
		return call(t, sess, args)
	}
	for _, s := range []struct{ sql, mode string }{
		{"create schema app", "ddl"},
		{"create table app.big (id int primary key, v text)", "ddl"},
		{"insert into app.big select g, 'x' from generate_series(1, 5000) g", "write"},
		{"analyze app.big", "ddl"},
	} {
		if r := run(s.sql, s.mode); r.IsError {
			t.Fatalf("%q = %q", s.sql, text(r))
		}
	}
	const big = "select * from app.big"
	cmds, stop := f.mcpCommands()
	defer stop()
	waitCmds := func(n int) []string {
		t.Helper()
		var got []string
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if got = cmds(); len(got) >= n {
				return got
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("ops = %v, want %d", got, n)
		return nil
	}
	before := len(cmds())

	// 1. Refused before it runs: one op, the EXPLAIN.
	r := run(big, "read")
	got := text(r)
	m := rowsMeasuredRe.FindStringSubmatch(got)
	if !r.IsError || m == nil || m[2] != "1000" || !strings.Contains(got, "expensive_query") ||
		!strings.Contains(got, "estimated rows read, not rows returned") || !strings.Contains(got, `exemption {"id":"exm_`) {
		t.Fatalf("big read = %q, want an expensive_query refusal with the hint and exemption path", got)
	}
	if measured, _ := strconv.Atoi(m[1]); measured < expensiveThreshold {
		t.Errorf("refusal %q: want measured >= %d", got, expensiveThreshold)
	}
	ops := waitCmds(before + 1)[before:]
	if len(ops) != 1 || !strings.HasPrefix(ops[0], "EXPLAIN") {
		t.Fatalf("ops of the refused call = %v, want exactly one EXPLAIN", ops)
	}

	// 2. A read-class statement declared write is checked too.
	if r := run(big, "write"); !r.IsError || !strings.Contains(text(r), "expensive_query") {
		t.Fatalf("big read declared write = %q, want the same refusal", text(r))
	}

	// 3. A selective read passes and reports its plan.
	r = run("select * from app.big where id = 1", "read")
	if rows, ok := resultPlan(t, r); r.IsError || !ok || rows >= expensiveThreshold {
		t.Fatalf("selective read = %q (plan rows %v), want rows with an estimate under the limit", text(r), rows)
	}

	// 4. No scan node, no estimate: passes.
	if r := run("select 1", "read"); r.IsError {
		t.Fatalf("select 1 = %q", text(r))
	}

	// 5. Exemption: a dialog with the measured value and plan evidence; approve runs it; the id is spent.
	id := exemptionID(t, run(big, "read"))
	exCall := make(chan callOutcome, 1)
	go func() {
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{
			"connectionId": pg.ID, "sql": big, "path": path, "mode": "read",
			"exemption": map[string]any{"id": id, "reason": "the report needs the whole table"},
		}})
		exCall <- callOutcome{res, err}
	}()
	items := f.items(t, 1)
	pl := decodeExemption(t, items[0])
	if len(pl.Limits) != 1 || pl.Limits[0].Limit != "expensive_query" || pl.Limits[0].Unit != "rows" || pl.Limits[0].Max != expensiveThreshold || pl.Limits[0].Measured < expensiveThreshold {
		t.Fatalf("payload = %+v", pl)
	}
	var evidence struct {
		Plan *struct {
			EstimatedRowsRead *float64 `json:"estimatedRowsRead"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(items[0].Payload, &evidence); err != nil || evidence.Plan == nil || evidence.Plan.EstimatedRowsRead == nil {
		t.Fatalf("exemption payload has no plan evidence: %s (%v)", items[0].Payload, err)
	}
	f.app.Contract(t, "dbmcp-expensive-query", "item", items[0], flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(host+":"+strconv.Itoa(port), "localhost:5432"))
	f.answerItems(t, true, items[0].ID)
	if res := wait(t, exCall); res.IsError || !strings.Contains(text(res), `"truncated"`) {
		t.Fatalf("approved exempt call = %q, want rows capped at maxRows", text(res))
	}
	if r := run(big, "read"); !r.IsError || !strings.Contains(text(r), "expensive_query") {
		t.Fatalf("same call without exemption = %q, want refused again", text(r))
	}

	// 6. EXPLAIN failure: the engine's error, no exemption id, no statement op.
	before = len(cmds())
	r = run("select * from app.missing", "read")
	if !r.IsError || !strings.Contains(text(r), "EXPLAIN failed") || strings.Contains(text(r), "exm_") {
		t.Fatalf("missing table = %q, want the EXPLAIN failure without an exemption id", text(r))
	}
	for _, c := range waitCmds(before + 1)[before:] {
		if !strings.HasPrefix(c, "EXPLAIN") {
			t.Errorf("op %q ran after a failed EXPLAIN", c)
		}
	}

	// 7. Not a read: a data-modifying CTE is not checked and runs.
	r = run("with d as (delete from app.big where id = 2 returning 1) select count(*) as n from d", "write")
	if r.IsError {
		t.Fatalf("data-modifying CTE = %q, want it unchecked and run", text(r))
	}
	if r := run("select count(*) as n from app.big where id = 2", "read"); r.IsError || !strings.Contains(text(r), "0") {
		t.Fatalf("row 2 after the delete = %q", text(r))
	}

	// 8. Check off for the connection: the same statement runs.
	f.setAutoExplain(t, pg.ID, false)
	if r := run(big, "read"); r.IsError {
		t.Fatalf("big read with the check off = %q, want rows", text(r))
	}
}

// SQLite has no row estimate: nothing is refused and list_connections says so.
func TestDbMcpExpensiveQueryNoEstimate(t *testing.T) {
	f := newFixture(t)
	f.setExpensiveThreshold(t, expensiveThreshold)
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("create table big (id integer primary key, v text)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("insert into big (v) with recursive c(x) as (select 1 union all select x+1 from c limit 5000) select 'x' from c"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	conn, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "lite", Kind: "sqlite", Color: "teal", Mode: "fields", Database: ptr(f.dbPath), Options: map[string]any{},
		McpEnabled: true, McpAutoExplain: true, McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny",
	}})
	if err != nil {
		t.Fatal(err)
	}
	sess := f.session(t)
	if r := runQuery(t, sess, conn.ID, "select * from big"); r.IsError {
		t.Fatalf("sqlite full scan = %q, want rows (no estimate to check)", text(r))
	}
	views := listViews(t, sess)
	for _, v := range views {
		if v.ID == conn.ID && (v.ExpensiveQuery.Check != "none" || v.ExpensiveQuery.RowsThreshold != expensiveThreshold) {
			t.Errorf("sqlite expensiveQuery = %+v, want none at %d", v.ExpensiveQuery, expensiveThreshold)
		}
	}
}

type expensiveListView struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	ExpensiveQuery struct {
		RowsThreshold int    `json:"rowsThreshold"`
		Check         string `json:"check"`
	} `json:"expensiveQuery"`
}

func listViews(t *testing.T, sess *mcp.ClientSession) []expensiveListView {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_connections", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("list_connections = %v, %v", res, err)
	}
	var views []expensiveListView
	if err := json.Unmarshal([]byte(text(res)), &views); err != nil {
		t.Fatal(err)
	}
	return views
}

func TestDbMcpListConnectionsExpensiveQuery(t *testing.T) {
	f := newFixture(t)
	f.setExpensiveThreshold(t, expensiveThreshold)
	sess := f.session(t)
	type spec struct {
		name, kind  string
		autoExplain bool
		want        string
	}
	specs := []spec{
		{"e-postgres", "postgres", true, "plan"},
		{"e-postgres-off", "postgres", false, "off"},
		{"e-mysql", "mysql", true, "plan"},
		{"e-mariadb", "mariadb", true, "plan"},
		{"e-clickhouse", "clickhouse", true, "plan"},
		{"e-sqlite", "sqlite", true, "none"},
		{"e-mongodb", "mongodb", true, "none"},
		{"e-redis", "redis", true, "none"},
		{"e-kafka", "kafka", true, "none"},
	}
	for _, s := range specs {
		in := model.ConnectionFields{
			Name: s.name, Kind: s.kind, Color: "teal", Mode: "fields", Host: ptr("localhost"), Port: ptr(1234), Options: map[string]any{},
			McpEnabled: true, McpAutoExplain: s.autoExplain, McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny",
		}
		if s.kind == "sqlite" {
			in.Host, in.Port, in.Database = nil, nil, ptr(f.dbPath)
		}
		if _, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: in}); err != nil {
			t.Fatalf("create %s: %v", s.kind, err)
		}
	}
	want := map[string]string{}
	for _, s := range specs {
		want[s.name] = s.want
	}
	seen := 0
	for _, v := range listViews(t, sess) {
		exp, ok := want[v.Name]
		if !ok {
			continue
		}
		seen++
		if v.ExpensiveQuery.Check != exp || v.ExpensiveQuery.RowsThreshold != expensiveThreshold {
			t.Errorf("%s expensiveQuery = %+v, want %s at %d", v.Name, v.ExpensiveQuery, exp, expensiveThreshold)
		}
	}
	if seen != len(want) {
		t.Fatalf("listed %d of %d created connections", seen, len(want))
	}
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_connections", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var all any
	_ = json.Unmarshal([]byte(text(res)), &all)
	f.app.Contract(t, "dbmcp-expensive-query", "list_connections", all, flowharness.Mask("id"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
}
