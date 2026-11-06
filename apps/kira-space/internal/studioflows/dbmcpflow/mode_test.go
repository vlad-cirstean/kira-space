package dbmcpflow

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// addConn creates an exposed SQLite connection on the fixture's file with the given permissions.
func (f *fixture) addConn(t *testing.T, name, kind, read, write, ddl string, readOnly bool) string {
	t.Helper()
	fields := model.ConnectionFields{
		Name: name, Kind: kind, Color: "teal", Mode: "fields", Database: ptr(f.dbPath), Options: map[string]any{},
		McpEnabled: true, McpReadMode: read, McpWriteMode: write, McpDdlMode: ddl, ReadOnly: readOnly,
	}
	c, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: fields})
	if err != nil {
		t.Fatal(err)
	}
	return c.ID
}

func call(t *testing.T, sess *mcp.ClientSession, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: args})
	if err != nil {
		t.Fatalf("run_query %v: %v", args, err)
	}
	return res
}

func withMode(connID, sql, mode string) map[string]any {
	return map[string]any{"connectionId": connID, "sql": sql, "path": "", "mode": mode}
}

func (f *fixture) session(t *testing.T) *mcp.ClientSession {
	t.Helper()
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// The schema rejects a call without a valid mode before the handler runs.
func TestDbMcpModeRequired(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	missing := call(t, sess, map[string]any{"connectionId": f.connID, "sql": "select 1", "path": ""})
	if !missing.IsError || !strings.Contains(text(missing), `"mode"`) {
		t.Fatalf("run_query without mode = %q, want a rejection naming mode", text(missing))
	}
	bogus := call(t, sess, withMode(f.connID, "select 1", "admin"))
	if !bogus.IsError {
		t.Fatalf("run_query with mode admin = %q, want a tool error", text(bogus))
	}
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("a rejected call queued %d approvals", n)
	}
}

// mode=read runs on the read lane, which refuses a write even when the statement is declared read
// by mistake; the text classifier only tightens, so a write under mode=read never reaches the lane.
func TestDbMcpModeReadOnLane(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	if r := call(t, sess, withMode(f.connID, "select body from notes order by id", "read")); r.IsError || !strings.Contains(text(r), "two") {
		t.Fatalf("read = %q, want rows", text(r))
	}
	if !f.app.W.Router.ReadLaneOpen(f.connID) {
		t.Fatal("no read lane open after a mode=read query")
	}
	for _, stmt := range []string{"insert into notes (body) values ('x')", "drop table notes"} {
		r := call(t, sess, withMode(f.connID, stmt, "read"))
		if !r.IsError {
			t.Fatalf("%q under mode=read = %q, want a refusal", stmt, text(r))
		}
	}
	if n := f.noteCount(t); n != 2 {
		t.Fatalf("mode=read changed the table: %d rows", n)
	}
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("mode=read queued %d approvals", n)
	}
}

// A declared mode higher than the statement needs is allowed and shown; a lower one is refused.
func TestDbMcpModeTightenOnly(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	done := make(chan *mcp.CallToolResult, 1)
	go func() {
		res, _ := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: withMode(f.connID, "select body from notes where id = 1", "write")})
		done <- res
	}()
	req := f.pending(t)
	var pl struct {
		Mode        string `json:"mode"`
		Detected    string `json:"detected"`
		Enforcement string `json:"enforcement"`
	}
	if err := json.Unmarshal(req.Item.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	if pl.Mode != "write" || pl.Detected != "read" || req.Item.Operation.Class != "write" {
		t.Fatalf("approval = %+v class %q, want declared write, detected read", pl, req.Item.Operation.Class)
	}
	// Contract dbmcp-approval-mode: tests/ui/dbmcp-approval-mode.spec.ts renders this item.
	f.app.Contract(t, "dbmcp-approval-mode", "ApprovalsService.List", []any{req.Item}, studioflowharness.Mask("createdAt", "expiresAt"), studioflowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.answerItems(t, false, req.Item.ID)
	if res := <-done; res == nil || !res.IsError {
		t.Fatalf("denied over-declared select = %+v, want a tool error", res)
	}
	// Under-declared: a write declared read is refused before any approval.
	if r := call(t, sess, withMode(f.connID, "update notes set body = 'x'", "read")); !r.IsError {
		t.Fatalf("update declared read = %q, want a refusal", text(r))
	}
	// DDL declared write is refused too.
	if r := call(t, sess, withMode(f.connID, "drop table notes", "write")); !r.IsError {
		t.Fatalf("drop declared write = %q, want a refusal", text(r))
	}
	if n := f.noteCount(t); n != 2 {
		t.Fatalf("a refused call changed the table: %d rows", n)
	}
}

// The declared mode's own permission is the gate; read-only connections and kinds without a write
// console deny write and ddl whatever the permission says.
func TestDbMcpModeGate(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	denyAll := f.addConn(t, "deny all", "sqlite", "deny", "deny", "deny", false)
	if r := call(t, sess, withMode(denyAll, "select 1", "read")); !r.IsError || !strings.Contains(text(r), "read") {
		t.Fatalf("read on a deny connection = %q, want a refusal naming the mode", text(r))
	}
	frozen := f.addConn(t, "frozen", "sqlite", "allow", "allow", "allow", true)
	if r := call(t, sess, withMode(frozen, "select body from notes", "read")); r.IsError {
		t.Fatalf("read on a read-only connection = %q, want rows", text(r))
	}
	for _, mode := range []string{"write", "ddl"} {
		r := call(t, sess, withMode(frozen, "select 1", mode))
		if !r.IsError || !strings.Contains(text(r), "does not allow "+mode) {
			t.Fatalf("mode=%s on a read-only connection = %q, want a refusal", mode, text(r))
		}
	}
	ddl := f.addConn(t, "ddl ok", "sqlite", "allow", "allow", "allow", false)
	if r := call(t, sess, withMode(ddl, "create table gate (id integer)", "ddl")); r.IsError {
		t.Fatalf("ddl on an allow connection = %q", text(r))
	}
	if r := call(t, sess, withMode(ddl, "create table gate2 (id integer)", "write")); !r.IsError {
		t.Fatalf("ddl statement declared write = %q, want a refusal", text(r))
	}
}

// list_connections reports per-mode access and the read enforcement; the Studio mirror in
// packages/shared/domain/dbmcp.ts is pinned to this output.
func TestDbMcpListConnectionsAccess(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.addConn(t, "frozen", "sqlite", "allow", "allow", "allow", true)
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_connections"})
	if err != nil || res.IsError {
		t.Fatalf("list_connections = %q (%v)", text(res), err)
	}
	var conns []struct {
		Name        string `json:"name"`
		Enforcement string `json:"enforcement"`
		Access      struct {
			Write struct {
				Reason string `json:"reason"`
			} `json:"write"`
			DDL struct {
				Reason string `json:"reason"`
			} `json:"ddl"`
			Summary string `json:"summary"`
		} `json:"access"`
	}
	if err := json.Unmarshal([]byte(text(res)), &conns); err != nil {
		t.Fatalf("list_connections JSON: %v", err)
	}
	type row struct{ Name, Enforcement, Summary string }
	rows := []row{}
	for _, c := range conns {
		rows = append(rows, row{c.Name, c.Enforcement, c.Access.Summary})
		switch c.Name {
		case "notes db":
			if c.Enforcement != "session" || c.Access.Summary != "read, write (approval)" || c.Access.DDL.Reason != "permission denied" {
				t.Fatalf("notes db = %+v", c)
			}
		case "frozen":
			if c.Access.Summary != "read" || c.Access.Write.Reason != "read-only connection" {
				t.Fatalf("frozen = %+v", c)
			}
		}
	}
	if len(rows) != 2 {
		t.Fatalf("list_connections = %d rows, want 2: %+v", len(rows), rows)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	// Contract dbmcp-list-access: tests/ui/connection-mcp-access.spec.ts mirrors these summaries.
	f.app.Contract(t, "dbmcp-list-access", "list_connections", rows)
}

// A lane opens on the first read, is rebuilt after a reconnect and closes on disconnect.
func TestDbMcpReadLaneLifecycle(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	r := f.app.W.Router
	if r.ReadLaneOpen(f.connID) {
		t.Fatal("lane open before any read")
	}
	if res := call(t, sess, withMode(f.connID, "select 1", "read")); res.IsError {
		t.Fatalf("read = %q", text(res))
	}
	if !r.ReadLaneOpen(f.connID) {
		t.Fatal("no lane after a read")
	}
	if _, err := f.app.W.Connections.Disconnect(bridge.ConnectionsIDArgs{ID: f.connID}); err != nil {
		t.Fatal(err)
	}
	if r.ReadLaneOpen(f.connID) {
		t.Fatal("lane still open after Disconnect")
	}
	if res := call(t, sess, withMode(f.connID, "select 1", "read")); res.IsError {
		t.Fatalf("read after reconnect = %q", text(res))
	}
	if !r.ReadLaneOpen(f.connID) {
		t.Fatal("no lane after reconnect and read")
	}
}

// cancel_operation stops a read running on the lane, and the lane stays usable.
func TestDbMcpCancelReadLaneOp(t *testing.T) {
	f := newFixture(t)
	a := f.session(t)
	b := f.session(t)
	pending := make(chan *mcp.CallToolResult, 1)
	go func() {
		res, _ := a.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: withMode(f.connID, longQuery, "read")})
		pending <- res
	}()
	op := waitListed(t, b, func(o listedOp) bool { return o.Type == "op" && o.Kind == "execute" })
	if res := cancelOp(t, b, op.OpID); res.IsError || !strings.Contains(text(res), `"cancelled":true`) {
		t.Fatalf("cancel_operation = %q", text(res))
	}
	select {
	case res := <-pending:
		if res == nil || !res.IsError || !strings.Contains(strings.ToLower(text(res)), "cancel") {
			t.Fatalf("cancelled read = %+v, want a cancel error", res)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("read still running after cancel_operation")
	}
	f.waitStatus(t, op.OpID, "cancelled")
	if res := call(t, b, withMode(f.connID, "select 1", "read")); res.IsError {
		t.Fatalf("read after cancel = %q", text(res))
	}
}

// startPostgres runs a Postgres container for the test and returns its host and mapped port.
func startPostgres(t *testing.T, pass, dbName string) (string, int) {
	t.Helper()
	c, err := tcpostgres.Run(ctx, testsupport.ImageFor("postgres", "postgres:17.11-alpine"),
		tcpostgres.WithDatabase(dbName), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword(pass),
		testcontainers.WithWaitStrategy(tcwait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	return host, int(mapped.Num())
}

// The database itself refuses a write hidden in a function under mode=read (SQLSTATE 25006).
func TestDbMcpReadLanePostgres(t *testing.T) {
	studioflowharness.Complete(t)
	studioflowharness.RequireDocker(t)
	const pass, dbName = "flowpass", "flowdb"
	host, port := startPostgres(t, pass, dbName)
	f := newFixture(t)
	pg, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "flow pg", Kind: "postgres", Color: "indigo", Mode: "fields",
		Host: ptr(host), Port: ptr(port), Database: ptr(dbName), Username: ptr("postgres"),
		Options: map[string]any{"sslmode": "disable"}, McpEnabled: true, McpReadMode: "allow", McpWriteMode: "allow", McpDdlMode: "allow",
	}, Password: ptr(pass)})
	if err != nil {
		t.Fatal(err)
	}
	sess := f.session(t)
	for _, stmt := range []string{
		"create schema app",
		"create table app.t (id int)",
		"create function app.kira_write_fn() returns int language sql volatile as $$ insert into app.t values (1) returning 1 $$",
	} {
		args := withMode(pg.ID, stmt, "ddl")
		args["path"] = "database:" + dbName
		if r := call(t, sess, args); r.IsError {
			t.Fatalf("%q = %q", stmt, text(r))
		}
	}
	read := withMode(pg.ID, "select app.kira_write_fn()", "read")
	read["path"] = "database:" + dbName
	if r := call(t, sess, read); !r.IsError || !strings.Contains(strings.ToLower(text(r)), "read-only") {
		t.Fatalf("hidden write under mode=read = %q, want the database's read-only refusal", text(r))
	}
	count := withMode(pg.ID, "select count(*) as n from app.t", "read")
	count["path"] = "database:" + dbName
	if r := call(t, sess, count); r.IsError || !strings.Contains(text(r), "0") {
		t.Fatalf("row count = %q, want 0 (the write must not have run)", text(r))
	}
}

// Turning MCP exposure off in the connection file closes the connection's read lane.
func TestDbMcpExposureOffClosesLane(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	if res := call(t, sess, withMode(f.connID, "select 1", "read")); res.IsError {
		t.Fatalf("read = %q", text(res))
	}
	if !f.app.W.Router.ReadLaneOpen(f.connID) {
		t.Fatal("no lane after a read")
	}
	cfg := f.app.W.ConnectionConfig
	list, err := f.app.W.Connections.List()
	if err != nil {
		t.Fatal(err)
	}
	var file string
	for _, c := range list {
		if c.ID == f.connID {
			file = c.File
		}
	}
	detail, err := cfg.File(bridge.ConnectionFileArgs{File: file})
	if err != nil || !strings.Contains(detail.Text, "enabled: true") {
		t.Fatalf("File = %+v (%v), want an exposed connection file", detail, err)
	}
	off := strings.Replace(detail.Text, "enabled: true", "enabled: false", 1)
	if saved, err := cfg.SaveFile(bridge.ConnectionSaveFileArgs{File: file, Text: off, BaseHash: detail.Hash}); err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v (%v)", saved, err)
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return !f.app.W.Router.ReadLaneOpen(f.connID) })
}
