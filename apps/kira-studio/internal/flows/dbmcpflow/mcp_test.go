package dbmcpflow

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/mcpauth"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/harness/claude/mcpinstall"
	"github.com/kirathecat/kira-studio/internal/testx"
)

var ctx = context.Background()

func ptr[T any](v T) *T { return &v }

var urlRe = regexp.MustCompile(`"url":"([^"]+)"`)

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// fixture is an app with the MCP server on and one exposed SQLite connection: reads allowed,
// writes need approval, DDL denied.
type fixture struct {
	app    *flowharness.App
	url    string
	connID string
	dbPath string
}

func newFixture(t *testing.T, opts ...flowharness.Opt) *fixture {
	t.Helper()
	app := flowharness.New(t, opts...)
	dbPath := filepath.Join(t.TempDir(), "mcp.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"create table notes (id integer primary key, body text)",
		"insert into notes (body) values ('one'), ('two')",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	conn, err := app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "notes db", Kind: "sqlite", Color: "teal", Mode: "fields", Database: ptr(dbPath), Options: map[string]any{},
		McpEnabled: true, McpDescription: "scratch notes", McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny",
	}})
	if err != nil {
		t.Fatal(err)
	}
	st, err := app.W.DbMcp.SetEnabled(bridge.DbMcpSetEnabledArgs{Enabled: true})
	if err != nil || !st.Running {
		t.Fatalf("SetEnabled = %+v (%v), want running", st, err)
	}
	if got := app.W.DbMcp.Status(); !got.Running || got.Command != st.Command {
		t.Fatalf("Status = %+v, want running with SetEnabled's command", got)
	}
	m := urlRe.FindStringSubmatch(st.Command)
	if m == nil {
		t.Fatalf("Status.Command has no server url: %q", st.Command)
	}
	return &fixture{app: app, url: m[1], connID: conn.ID, dbPath: dbPath}
}

func (f *fixture) token(t *testing.T) string { t.Helper(); return helperToken(t, f.app) }

func (f *fixture) connect(t *testing.T, token string) (*mcp.ClientSession, error) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "flow-client", Version: "1"}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: f.url, HTTPClient: &http.Client{Transport: bearer{token}, Timeout: 30 * time.Second},
	}, nil)
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func runQuery(t *testing.T, sess *mcp.ClientSession, connID, sqlText string) *mcp.CallToolResult {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{"connectionId": connID, "sql": sqlText, "path": ""}})
	if err != nil {
		t.Fatalf("run_query %q: %v", sqlText, err)
	}
	return res
}

func (f *fixture) noteCount(t *testing.T) int {
	t.Helper()
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("select count(*) from notes").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *fixture) pending(t *testing.T) bridge.DbMcpApprovalRequest {
	t.Helper()
	var req *bridge.DbMcpApprovalRequest
	testx.WaitUntil(t, 10*time.Second, func() bool {
		req = f.app.W.DbMcp.PendingApprovals().Pending
		return req != nil
	})
	return *req
}

func TestDbMcpApprovalFlow(t *testing.T) {
	f := newFixture(t)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatalf("connect with the helper token: %v", err)
	}
	defer sess.Close()

	list, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_connections"})
	if err != nil || list.IsError || !strings.Contains(text(list), f.connID) || !strings.Contains(text(list), "scratch notes") {
		t.Fatalf("list_connections = %q (%v), want the exposed connection with its description", text(list), err)
	}
	if read := runQuery(t, sess, f.connID, "select body from notes order by id"); read.IsError || !strings.Contains(text(read), "two") {
		t.Fatalf("read run_query = %q, want rows without approval", text(read))
	}
	noPath, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{"connectionId": f.connID, "sql": "select 1"}})
	if err != nil || !noPath.IsError || !strings.Contains(text(noPath), "missing properties") || !strings.Contains(text(noPath), `"path"`) {
		t.Fatalf("run_query without path = %q (%v), want a schema rejection naming path", text(noPath), err)
	}
	if snap := f.app.W.DbMcp.PendingApprovals(); snap.Pending != nil {
		t.Fatalf("a read queued an approval: %+v", snap)
	}

	const write = "insert into notes (body) values ('three')"
	type outcome struct {
		res *mcp.CallToolResult
		err error
	}
	call := func() chan outcome {
		ch := make(chan outcome, 1)
		go func() {
			res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{"connectionId": f.connID, "sql": write, "path": ""}})
			ch <- outcome{res, err}
		}()
		return ch
	}

	denied := call()
	req := f.pending(t)
	// Contract dbmcp: tests/ui/dbmcp-approval.spec.ts renders this request in the approval dialog.
	f.app.Contract(t, "dbmcp", "DbMcpService.PendingApprovals", bridge.DbMcpApprovalSnapshot{Pending: &req}, flowharness.Mask("expiresAtMs"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.app.Contract(t, "dbmcp", "args:DbMcpService.ApproveQuery", bridge.DbMcpApprovalArgs{RequestID: req.RequestID})
	if req.Statement != write || req.ConnectionID != f.connID || req.Class == "" {
		t.Fatalf("pending approval = %+v, want the write statement on the connection", req)
	}
	if tg := req.Target; tg.Color != "teal" || tg.ReadOnly || tg.Database != f.dbPath || tg.Host != "" || len(tg.Path) != 0 {
		t.Fatalf("approval target = %+v, want teal, writable, the fixture file, no host, no path", tg)
	}
	if _, err := f.app.W.DbMcp.DenyQuery(bridge.DbMcpApprovalArgs{RequestID: req.RequestID}); err != nil {
		t.Fatal(err)
	}
	out := <-denied
	if out.err != nil || out.res == nil || !out.res.IsError {
		t.Fatalf("denied write = %+v, want a tool error", out)
	}
	if n := f.noteCount(t); n != 2 {
		t.Fatalf("denied write changed the table: %d rows", n)
	}

	approved := call()
	req = f.pending(t)
	if _, err := f.app.W.DbMcp.ApproveQuery(bridge.DbMcpApprovalArgs{RequestID: req.RequestID}); err != nil {
		t.Fatal(err)
	}
	out = <-approved
	if out.err != nil || out.res == nil || out.res.IsError {
		t.Fatalf("approved write = %+v (%s), want success", out, text(out.res))
	}
	if n := f.noteCount(t); n != 3 {
		t.Fatalf("approved write left %d rows, want 3", n)
	}
	if snap := f.app.W.DbMcp.PendingApprovals(); snap.Pending != nil || snap.Queued != 0 {
		t.Fatalf("approvals left behind: %+v", snap)
	}

	if ddl := runQuery(t, sess, f.connID, "drop table notes"); !ddl.IsError {
		t.Fatalf("DDL on a deny connection = %q, want a tool error", text(ddl))
	}

	old := f.token(t)
	st := f.app.W.DbMcp.Regenerate()
	if !st.Running {
		t.Fatalf("Regenerate = %+v, want still running", st)
	}
	if fresh := f.token(t); fresh == old {
		t.Fatal("Regenerate kept the token")
	}
	if s, err := f.connect(t, old); err == nil {
		s.Close()
		t.Fatal("old token still connects after Regenerate")
	}
	s2, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatalf("new token rejected: %v", err)
	}
	s2.Close()

	if st, err := f.app.W.DbMcp.SetEnabled(bridge.DbMcpSetEnabledArgs{Enabled: false}); err != nil || st.Running {
		t.Fatalf("SetEnabled(false) = %+v (%v), want stopped", st, err)
	}
}

// noClaude is a real mcpinstall installer that finds nothing and must not spawn anything.
func noClaude(t *testing.T) harness.MCPInstaller {
	return mcpinstall.New(mcpinstall.Deps{
		Candidates: claude.Candidates,
		LookPath:   func(string) (string, error) { return "", os.ErrNotExist },
		Stat:       func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		Run: func(context.Context, string, []string) error {
			t.Error("mcpinstall spawned a process although claude is missing")
			return nil
		},
	})
}

// P328: an op records who started it. run_query over MCP is "mcp"; a tree call through the
// bridge is "user".
func TestDbMcpOpsCarrySource(t *testing.T) {
	f := newFixture(t)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatalf("connect with the helper token: %v", err)
	}
	defer sess.Close()

	if res := runQuery(t, sess, f.connID, "select body from notes order by id"); res.IsError {
		t.Fatalf("run_query = %q", text(res))
	}
	if _, err := f.app.W.Tree.Children(bridge.TreeChildrenArgs{ConnectionID: f.connID, Path: "", Refresh: true}); err != nil {
		t.Fatalf("Tree.Children: %v", err)
	}

	var rows []model.OpRecord
	testx.WaitUntil(t, 10*time.Second, func() bool {
		recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 50})
		if err != nil {
			t.Fatalf("Ops.Recent: %v", err)
		}
		rows = rows[:0]
		for _, r := range recent {
			if (r.Kind == "execute" || r.Kind == "children") && r.Status != "running" {
				rows = append(rows, r)
			}
		}
		return len(rows) == 2
	})
	bySource := map[string]string{}
	for _, r := range rows {
		bySource[r.Kind] = r.Source
	}
	if bySource["execute"] != "mcp" || bySource["children"] != "user" {
		t.Fatalf("op sources = %v, want execute=mcp children=user", bySource)
	}
	// Contract ops-source: tests/ui/operations.spec.ts renders these rows' source column.
	f.app.Contract(t, "ops-source", "OpsService.Recent", rows, flowharness.Mask("id", "startedAt", "durationMs", "connectionId"))
}

func TestInstallAgentWithoutCli(t *testing.T) {
	app := flowharness.New(t)
	app.W.DbMcp.Installer = noClaude(t)

	if res := app.W.DbMcp.InstallAgent(ctx); res.Outcome != harness.OutcomeNotFound {
		t.Fatalf("InstallAgent with the server off = %+v, want notFound", res)
	}
	st, err := app.W.DbMcp.SetEnabled(bridge.DbMcpSetEnabledArgs{Enabled: true})
	if err != nil || !st.Running {
		t.Fatalf("SetEnabled = %+v (%v)", st, err)
	}
	if st.AgentAvailable || len(st.Probed) == 0 || !strings.Contains(st.Command, "claude mcp add-json") {
		t.Fatalf("Status = %+v, want claude unavailable, the probed paths and a copyable command", st)
	}
	if !strings.Contains(st.Command, `"timeout":2100000`) {
		t.Fatalf("Status.Command = %q, want the 35 minute server timeout", st.Command)
	}
	res := app.W.DbMcp.InstallAgent(ctx)
	if res.Outcome != harness.OutcomeNotFound || len(res.Probed) == 0 {
		t.Fatalf("InstallAgent = %+v, want notFound with the probed paths", res)
	}
	if strings.Contains(st.Command, helperToken(t, app)) {
		t.Fatal("the copyable command carries the token")
	}
	if entries, _ := os.ReadDir(filepath.Join(app.Home, ".claude")); len(entries) != 0 {
		t.Fatalf("install wrote into ~/.claude: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(app.Home, ".claude.json")); err == nil {
		t.Fatal("install wrote ~/.claude.json")
	}
}

func helperToken(t *testing.T, app *flowharness.App) string {
	t.Helper()
	plain, ok, err := mcpauth.LoadHelperToken(mcpauth.HelperTokenPathNamed(app.KiraHome, "mcp-db"))
	if err != nil || !ok {
		t.Fatalf("helper token: ok=%v err=%v", ok, err)
	}
	return plain
}

type callOutcome struct {
	res *mcp.CallToolResult
	err error
}

func callAsync(sess *mcp.ClientSession, connID, sqlText string) chan callOutcome {
	ch := make(chan callOutcome, 1)
	go func() {
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{"connectionId": connID, "sql": sqlText, "path": ""}})
		ch <- callOutcome{res, err}
	}()
	return ch
}

func (f *fixture) setPreview(t *testing.T, on bool) bridge.SettingsSetArgs {
	t.Helper()
	args := bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{PreviewResponses: &on}}}
	if _, err := f.app.W.Settings.Set(args); err != nil {
		t.Fatal(err)
	}
	return args
}

func (f *fixture) maskColumn(t *testing.T, column string) {
	t.Helper()
	_, err := f.app.W.MaskRules.Upsert(bridge.MaskRulesUpsertArgs{ConnectionID: f.connID, Fields: model.MaskRuleFields{TableName: "notes", ColumnName: column, Kind: "redact"}})
	if err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) answer(t *testing.T, approve bool, id string) {
	t.Helper()
	args := bridge.DbMcpApprovalArgs{RequestID: id}
	var err error
	if approve {
		_, err = f.app.W.DbMcp.ApproveQuery(args)
	} else {
		_, err = f.app.W.DbMcp.DenyQuery(args)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func wait(t *testing.T, ch chan callOutcome) *mcp.CallToolResult {
	t.Helper()
	select {
	case out := <-ch:
		if out.err != nil || out.res == nil {
			t.Fatalf("run_query = %+v, want a tool result", out)
		}
		return out.res
	case <-time.After(30 * time.Second):
		t.Fatal("run_query did not return")
		return nil
	}
}

// With dbMcp.previewResponses on, a run_query result that already ran is held until the user sends
// or withholds it. Contract dbmcp-preview: tests/ui/dbmcp-preview.spec.ts renders it.
func TestDbMcpResponsePreview(t *testing.T) {
	f := newFixture(t)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatalf("connect with the helper token: %v", err)
	}
	defer sess.Close()

	const read = "select id, body from notes order by id"
	setArgs := f.setPreview(t, true)
	f.app.Contract(t, "dbmcp-preview", "args:SettingsService.Set", setArgs)
	f.maskColumn(t, "body")

	done := callAsync(sess, f.connID, read)
	req := f.pending(t)
	if req.Stage != "response" || req.Response == nil || req.Class != "read" || req.Statement != read {
		t.Fatalf("pending = %+v, want a response-stage read of the statement", req)
	}
	held, err := f.app.W.DbMcp.PendingResponse(bridge.DbMcpApprovalArgs{RequestID: req.RequestID})
	if err != nil || held.IsError || req.Response.Bytes != len(held.Text) {
		t.Fatalf("PendingResponse = %+v (%v), want the %d held bytes", held, err, req.Response.Bytes)
	}
	if strings.Contains(held.Text, "one") || strings.Contains(held.Text, "two") || !strings.Contains(held.Text, "[redacted]") {
		t.Fatalf("held text = %q, want the masked rows", held.Text)
	}
	replace := flowharness.Replace(f.dbPath, "/data/mcp.db")
	f.app.Contract(t, "dbmcp-preview", "DbMcpService.PendingApprovals", bridge.DbMcpApprovalSnapshot{Pending: &req}, flowharness.Mask("expiresAtMs"), replace)
	f.app.Contract(t, "dbmcp-preview", "DbMcpService.PendingResponse", held)
	f.app.Contract(t, "dbmcp-preview", "args:DbMcpService.ApproveQuery", bridge.DbMcpApprovalArgs{RequestID: req.RequestID})
	f.answer(t, true, req.RequestID)
	if sent := wait(t, done); sent.IsError || text(sent) != held.Text {
		t.Fatalf("sent = %q, want the held bytes %q", text(sent), held.Text)
	}
	if _, err := f.app.W.DbMcp.PendingResponse(bridge.DbMcpApprovalArgs{RequestID: req.RequestID}); err == nil {
		t.Fatal("PendingResponse still answers after the request resolved")
	}

	done = callAsync(sess, f.connID, read)
	f.answer(t, false, f.pending(t).RequestID)
	withheld := wait(t, done)
	if !withheld.IsError || !strings.Contains(text(withheld), "withheld") || strings.Contains(text(withheld), "[redacted]") {
		t.Fatalf("withheld read = %q, want an error with no rows", text(withheld))
	}

	done = callAsync(sess, f.connID, "insert into notes (body) values ('three')")
	if q := f.pending(t); q.Stage != "query" {
		t.Fatalf("first request = %+v, want the query stage", q)
	} else {
		f.answer(t, true, q.RequestID)
	}
	testx.WaitUntil(t, 10*time.Second, func() bool {
		p := f.app.W.DbMcp.PendingApprovals().Pending
		return p != nil && p.Stage == "response"
	})
	f.answer(t, false, f.pending(t).RequestID)
	if w := wait(t, done); !w.IsError || !strings.Contains(text(w), "ran") {
		t.Fatalf("withheld write = %q, want an error saying it ran", text(w))
	}
	if n := f.noteCount(t); n != 3 {
		t.Fatalf("withheld write left %d rows, want 3 (the write ran)", n)
	}

	done = callAsync(sess, f.connID, read)
	req = f.pending(t)
	f.maskColumn(t, "id")
	f.answer(t, true, req.RequestID)
	if w := wait(t, done); !w.IsError || !strings.Contains(text(w), "masking rules") || strings.Contains(text(w), "[redacted]") {
		t.Fatalf("read after a rule change = %q, want it withheld for the changed masking rules", text(w))
	}

	f.setPreview(t, false)
	if live := runQuery(t, sess, f.connID, read); live.IsError || !strings.Contains(text(live), "[redacted]") {
		t.Fatalf("read with preview off = %q, want rows straight away", text(live))
	}
	if snap := f.app.W.DbMcp.PendingApprovals(); snap.Pending != nil {
		t.Fatalf("preview off still queued %+v", snap.Pending)
	}
}

// A connection's mcpReviewResults overrides dbMcp.previewResponses: review always holds, send never
// does, inherit follows the app setting. Contract dbmcp-review-connection:
// tests/ui/dbmcp-review-connection.spec.ts renders it.
func TestDbMcpReviewPerConnection(t *testing.T) {
	f := newFixture(t)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatalf("connect with the helper token: %v", err)
	}
	defer sess.Close()

	const read = "select id, body from notes order by id"
	current := func() model.ConnectionSummary {
		list, err := f.app.W.Connections.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range list {
			if c.ID == f.connID {
				return c
			}
		}
		t.Fatal("fixture connection missing")
		return model.ConnectionSummary{}
	}
	// The setting lives in the connection's file; the edit is the file's mcp.review_results key.
	setReview := func(value string) (model.ConnectionSummary, error) {
		err := f.app.W.ConnectionFiles.Patch(f.connID, configfs.NodeSet{Path: []string{"mcp", "review_results"}, Value: value})
		return current(), err
	}
	replace := flowharness.Replace(f.dbPath, "/data/mcp.db")

	got, err := setReview("review")
	if err != nil || got.McpReviewResults != "review" {
		t.Fatalf("file edit = %+v (%v), want review", got, err)
	}
	f.app.Contract(t, "dbmcp-review-connection", "ConnectionsService.List#review", got, flowharness.Mask("createdAt", "updatedAt"), replace)

	done := callAsync(sess, f.connID, read)
	req := f.pending(t)
	if req.Stage != "response" || req.Response == nil {
		t.Fatalf("pending = %+v, want a response-stage hold with the app setting off", req)
	}
	held, err := f.app.W.DbMcp.PendingResponse(bridge.DbMcpApprovalArgs{RequestID: req.RequestID})
	if err != nil || held.IsError {
		t.Fatalf("PendingResponse = %+v (%v)", held, err)
	}
	f.app.Contract(t, "dbmcp-review-connection", "DbMcpService.PendingApprovals", bridge.DbMcpApprovalSnapshot{Pending: &req}, flowharness.Mask("expiresAtMs"), replace)
	f.app.Contract(t, "dbmcp-review-connection", "DbMcpService.PendingResponse", held)
	f.answer(t, true, req.RequestID)
	if sent := wait(t, done); sent.IsError || text(sent) != held.Text {
		t.Fatalf("sent = %q, want the held bytes %q", text(sent), held.Text)
	}

	f.setPreview(t, true)
	if got, err = setReview("send"); err != nil || got.McpReviewResults != "send" {
		t.Fatalf("file edit = %+v (%v), want send", got, err)
	}
	if live := runQuery(t, sess, f.connID, read); live.IsError || !strings.Contains(text(live), "two") {
		t.Fatalf("read with send = %q, want rows straight away", text(live))
	}
	if snap := f.app.W.DbMcp.PendingApprovals(); snap.Pending != nil {
		t.Fatalf("send still queued %+v", snap.Pending)
	}

	if got, err = setReview("inherit"); err != nil || got.McpReviewResults != "inherit" {
		t.Fatalf("file edit = %+v (%v), want inherit", got, err)
	}
	done = callAsync(sess, f.connID, read)
	if q := f.pending(t); q.Stage != "response" {
		t.Fatalf("pending = %+v, want inherit to follow the app setting on", q)
	}
	f.answer(t, false, f.pending(t).RequestID)
	if w := wait(t, done); !w.IsError {
		t.Fatalf("withheld read = %q, want an error", text(w))
	}

	if _, err := setReview("maybe"); err == nil {
		t.Fatal("file edit accepted an invalid review setting")
	}
	if cur := current(); cur.McpReviewResults != "inherit" {
		t.Fatalf("review setting after a refused file edit = %q, want inherit", cur.McpReviewResults)
	}
}
