package adeflow_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mcpauth"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P547 Kira DB audit trail. Contract ade-db-audit: tests/ui/ade-v2-db-audit.spec.ts reads it.

const dbAuditScenario = "ade-db-audit"

var dbAuditMask = flowharness.Mask("id", "sessionId", "startedAt", "durationMs", "raisedAt", "resolvedAt")

// bigLimit trips on a marker in the statement, a stand-in for the EXPLAIN threshold.
type bigLimit struct{}

func (bigLimit) Name() string { return dbmcp.LimitExpensiveQuery }

func (bigLimit) Check(_ context.Context, c dbmcp.LimitCall) (*dbmcp.LimitHit, error) {
	if c.Exempt || !strings.Contains(c.SQL, "/* big */") {
		return nil, nil
	}
	return &dbmcp.LimitHit{Limit: dbmcp.LimitExpensiveQuery, Measured: 5000, Max: 1000, Unit: dbmcp.UnitRows}, nil
}

func callTool(sess *mcp.ClientSession, tool string, args map[string]any) chan *mcp.CallToolResult {
	ch := make(chan *mcp.CallToolResult, 1)
	go func() {
		res, _ := sess.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		ch <- res
	}()
	return ch
}

func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func await(t *testing.T, ch chan *mcp.CallToolResult) *mcp.CallToolResult {
	t.Helper()
	select {
	case res := <-ch:
		if res == nil {
			t.Fatal("tool call failed at the transport")
		}
		return res
	case <-time.After(waitFor):
		t.Fatal("tool call did not return")
		return nil
	}
}

// pendingItem waits for one open db.* approval and returns its id.
func pendingItem(t *testing.T, app *flowharness.App) approvals.Item {
	t.Helper()
	var it approvals.Item
	testx.WaitUntil(t, waitFor, func() bool {
		items, _ := app.W.ApprovalsSvc.List()
		for _, i := range items {
			if strings.HasPrefix(i.Kind, "db.") {
				it = i
				return true
			}
		}
		return false
	})
	return it
}

func answerItem(t *testing.T, app *flowharness.App, it approvals.Item, approve bool) {
	t.Helper()
	res, err := app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{it.ID}, Approve: approve})
	if err != nil || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("answer %s = %+v, %v", it.ID, res, err)
	}
}

func dbAudit(t *testing.T, app *flowharness.App, taskID string) adewire.DbAuditResult {
	t.Helper()
	res, err := app.W.AdeTask.DbAudit(ctx, adewire.TaskArgs{TaskID: taskID})
	if err != nil {
		t.Fatalf("DbAudit: %v", err)
	}
	return res
}

var errCodeRe = regexp.MustCompile(`^(E_[A-Z_]+): `)

func TestDbAuditTrail(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second), flowharness.WithDbMcpLimits(bigLimit{}))
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"create table people (id integer primary key, name text, email text)",
		"insert into people (name, email) values ('bob', 'bob@example.com')",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	dbPtr := dbPath
	conn, err := app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "audit db", Kind: "sqlite", Color: "teal", Mode: "fields", Database: &dbPtr, Options: map[string]any{},
		McpEnabled: true, McpDescription: "scratch", McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny",
	}})
	if err != nil {
		t.Fatal(err)
	}
	claude(app, map[string][]fakeagent.Action{"*": {{WaitFile: hold}}})
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "audit", flowYAML("audit", "  - id: work\n    name: Work\n    kind: manual\n    session: true\n    db_mcp: true\n"))
	task := createTask(t, app, "Audit me", "audit", rec.ID)
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	cfgRaw, err := os.ReadFile(mcpConfigPath(t, launch.Command))
	if err != nil {
		t.Fatal(err)
	}
	e, ok := mcpEntry(t, string(cfgRaw), dbSrvName)
	if !ok {
		t.Fatal("launch config has no session server")
	}
	openLaunch(t, app, launch)
	waitSession(t, app, launch.SessionID, "running")
	sess := sessionClient(t, e)
	defer sess.Close()
	args := func(sqlText, mode string, extra map[string]any) map[string]any {
		a := map[string]any{"connectionId": conn.ID, "sql": sqlText, "path": "", "mode": mode}
		for k, v := range extra {
			a[k] = v
		}
		return a
	}

	// 1-3. A listing, a read with a literal, a syntax error.
	await(t, callTool(sess, "list_connections", map[string]any{}))
	read := await(t, callTool(sess, "run_query", args("SELECT * FROM people WHERE email = 'bob@example.com'", "read", nil)))
	if read.IsError {
		t.Fatalf("read = %s", resultText(read))
	}
	bad := await(t, callTool(sess, "run_query", args("SELECT * FROM missing WHERE name = 'carol'", "read", nil)))
	m := errCodeRe.FindStringSubmatch(resultText(bad))
	if !bad.IsError || m == nil {
		t.Fatalf("syntax error = %q, want an E_ code", resultText(bad))
	}
	badCode := m[1]

	// 4-5. Two prompt-mode writes: one approved, one denied.
	insert := "INSERT INTO people (name, email) VALUES ('carol', 'pw-secret-1')"
	ch := callTool(sess, "run_query", args(insert, "write", nil))
	answerItem(t, app, pendingItem(t, app), true)
	if res := await(t, ch); res.IsError {
		t.Fatalf("approved write = %s", resultText(res))
	}
	ch = callTool(sess, "run_query", args(insert, "write", nil))
	answerItem(t, app, pendingItem(t, app), false)
	if res := await(t, ch); !res.IsError {
		t.Fatalf("denied write = %s, want a refusal", resultText(res))
	}

	// 6-7. A limit refusal, then the exemption request, denied.
	big := "SELECT * FROM people /* big */ WHERE email = 'bob@example.com'"
	refusal := await(t, callTool(sess, "run_query", args(big, "read", nil)))
	id := regexp.MustCompile(`exm_[0-9a-z]{12}`).FindString(resultText(refusal))
	if !refusal.IsError || id == "" {
		t.Fatalf("limit refusal = %q", resultText(refusal))
	}
	ch = callTool(sess, "run_query", args(big, "read", map[string]any{"exemption": map[string]any{"id": id, "reason": "quarterly report needs the full table"}}))
	item := pendingItem(t, app)
	if item.Kind != approvals.KindDbExemption {
		t.Fatalf("item = %s, want db.exemption", item.Kind)
	}
	answerItem(t, app, item, false)
	if res := await(t, ch); !res.IsError {
		t.Fatalf("denied exemption = %s, want a refusal", resultText(res))
	}

	got := dbAudit(t, app, task.ID)
	if len(got.Entries) != 7 {
		t.Fatalf("entries = %d, want 7: %+v", len(got.Entries), got.Entries)
	}
	// Newest first.
	want := []struct {
		tool, status string
		approvals    []string
	}{
		{"run_query", "refused", []string{"denied"}},
		{"run_query", "refused", nil},
		{"run_query", "refused", []string{"denied"}},
		{"run_query", "ok", []string{"approved"}},
		{"run_query", "error", nil},
		{"run_query", "ok", nil},
		{"list_connections", "ok", nil},
	}
	for i, w := range want {
		en := got.Entries[i]
		if en.Tool != w.tool || en.Status != w.status || len(en.Approvals) != len(w.approvals) {
			t.Fatalf("entry %d = %s %s %d approvals, want %s %s %v", i, en.Tool, en.Status, len(en.Approvals), w.tool, w.status, w.approvals)
		}
		for j, o := range w.approvals {
			if en.Approvals[j].Outcome != o {
				t.Fatalf("entry %d approval %d = %s, want %s", i, j, en.Approvals[j].Outcome, o)
			}
		}
		if en.DurationMs == nil || en.StepName != "Work" || en.SessionID != launch.SessionID || en.ConnectionName == "" && en.Tool != "list_connections" {
			t.Fatalf("entry %d = %+v, want duration, step name Work, the launch session", i, en)
		}
	}
	if x := got.Entries[0].Approvals[0]; x.Subject != approvals.KindDbExemption || x.Reason != "exemption" || len(x.ExemptionLimits) != 1 || x.ExemptionLimits[0].Name != dbmcp.LimitExpensiveQuery {
		t.Fatalf("exemption approval = %+v", x)
	}
	if got.Entries[4].Error != dbmcp.AuditErrorText(badCode) {
		t.Fatalf("syntax error row = %q, want %q", got.Entries[4].Error, dbmcp.AuditErrorText(badCode))
	}
	if s := got.Entries[5].Statement; s != "SELECT * FROM people WHERE email = '…'" {
		t.Fatalf("read statement = %q", s)
	}
	raw, _ := json.Marshal(got)
	for _, secret := range []string{"bob@example.com", "carol", "pw-secret-1", "params", `"bob"`} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("trail contains %q: %s", secret, raw)
		}
	}
	app.Contract(t, dbAuditScenario, "AdeTaskService.DbAudit#list", got, dbAuditMask)

	// 8. The external server (global token) is never audited.
	if _, err := app.W.DbMcp.SetEnabled(bridge.DbMcpSetEnabledArgs{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	st := app.W.DbMcp.Status()
	url := dbURLRe.FindStringSubmatch(st.Command)
	helper, hok, err := mcpauth.LoadHelperToken(mcpauth.HelperTokenPathNamed(app.SpaceHome, "mcp-db"))
	if url == nil || err != nil || !hok {
		t.Fatalf("external server: %v %v %v", url, hok, err)
	}
	ext := sessionClient(t, sessionEntry{URL: url[1], Headers: map[string]string{"Authorization": "Bearer " + helper}})
	defer ext.Close()
	if res := await(t, callTool(ext, "run_query", args("SELECT 1", "read", nil))); res.IsError {
		t.Fatalf("external read = %s", resultText(res))
	}
	if n := len(dbAudit(t, app, task.ID).Entries); n != 7 {
		t.Fatalf("entries after an external call = %d, want 7", n)
	}
	var rows int
	if err := app.DB().QueryRow(`SELECT COUNT(*) FROM ade_db_audit WHERE task_id <> ?`, task.ID).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rows of other tasks = %d (%v), want 0", rows, err)
	}

	// 9. A parked call survives a restart as a settled row, never running or pending.
	ch = callTool(sess, "run_query", args(insert, "write", nil))
	pendingItem(t, app)
	app.Restart()
	_ = ch
	for _, c := range dbAudit(t, app, task.ID).Entries {
		if c.Status == "running" {
			t.Fatalf("call %s still running after a restart", c.ID)
		}
		for _, a := range c.Approvals {
			if a.Outcome == "pending" {
				t.Fatalf("approval %s still pending after a restart", a.ID)
			}
		}
	}
	// 10. The trail outlives archiving and dies with the task row.
	before := len(dbAudit(t, app, task.ID).Entries)
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: task.ID}); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	if n := len(dbAudit(t, app, task.ID).Entries); n != before || n == 0 {
		t.Fatalf("entries after archive = %d, want %d", n, before)
	}
	if _, err := app.DB().Exec(`DELETE FROM ade_tasks WHERE id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := app.DB().QueryRow(`SELECT COUNT(*) FROM ade_db_audit WHERE task_id = ?`, task.ID).Scan(&left); err != nil || left != 0 {
		t.Fatalf("rows after delete = %d (%v), want 0", left, err)
	}
}
