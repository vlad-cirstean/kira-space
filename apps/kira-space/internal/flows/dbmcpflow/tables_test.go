package dbmcpflow

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// P532 table allow list. UI half: tests/ui/connection-mcp-tables.spec.ts, contract dbmcp-tables.
func TestDbMcpTableAllowList(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"create table secret (id integer primary key, token text)",
		"insert into secret (token) values ('hush')",
		"create view secret_view as select * from secret",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	// Everything is open while the file has no mcp.tables key.
	if r := runQuery(t, sess, f.connID, "select token from secret"); r.IsError || !strings.Contains(text(r), "hush") {
		t.Fatalf("select without a list = %q, want rows", text(r))
	}

	if err := f.app.W.ConnectionFiles.Patch(f.connID, configfs.NodeSet{Path: []string{"mcp", "tables"}, Value: []string{"notes"}}); err != nil {
		t.Fatalf("Patch mcp.tables: %v", err)
	}

	refused := func(sqlText, want string) {
		t.Helper()
		r := runQuery(t, sess, f.connID, sqlText)
		if !r.IsError || !strings.Contains(text(r), want) {
			t.Fatalf("%q = %q (error %v), want a refusal containing %q", sqlText, text(r), r.IsError, want)
		}
		if strings.Contains(text(r), "hush") {
			t.Fatalf("%q leaked data: %q", sqlText, text(r))
		}
	}
	allowed := func(sqlText string) {
		t.Helper()
		if r := runQuery(t, sess, f.connID, sqlText); r.IsError {
			t.Fatalf("%q = %q, want it to run", sqlText, text(r))
		}
	}

	allowed("select body from notes order by id")
	allowed("select body from main.notes")
	allowed("with n as (select * from notes) select body from n")
	refused("select token from secret", "not listed: secret")
	refused("select token from main.secret", "not listed: secret")
	refused("select * from secret_view", "not listed: secret_view")
	refused("select body from notes where id in (select id from secret)", "not listed: secret")
	refused("select body from notes n join secret s on s.id = n.id", "not listed: secret")
	// A CTE named like the real table shadows it only for later references, never for its own body.
	refused("with secret as (select * from secret) select * from secret", "not listed: secret")
	// The classifier refuses a multi-statement call first; the gate checks each statement when it does not.
	refused("select 1 from notes; select token from secret", "")
	refused("select token from temp.secret", "could not be parsed")
	refused("select body from notes where", "could not be parsed")
	refused("insert into secret (token) values ('x')", "not listed: secret")
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("a refused statement queued %d approvals", n)
	}
	if n := f.noteCount(t); n != 2 {
		t.Fatalf("notes has %d rows after refused statements, want 2", n)
	}

	// Metadata tools show only listed tables, and refuse a path below an unlisted one.
	tool := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return res
	}
	type node struct {
		Kind, Name, Path string
	}
	nodes := func(path string) []node {
		t.Helper()
		res := tool("list_children", map[string]any{"connectionId": f.connID, "path": path})
		if res.IsError {
			t.Fatalf("list_children %q = %q", path, text(res))
		}
		var out []node
		if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	var mainPath string
	for _, n := range nodes("") {
		if n.Kind == "database" {
			mainPath = n.Path
		}
	}
	if mainPath == "" {
		t.Fatal("list_children found no database node")
	}
	var names []string
	var notesPath string
	for _, n := range nodes(mainPath) {
		names = append(names, n.Name)
		if n.Name == "notes" {
			notesPath = n.Path
		}
	}
	if notesPath == "" || len(names) != 1 {
		t.Fatalf("list_children under a list = %v, want only notes", names)
	}
	secretPath := strings.Replace(notesPath, "notes", "secret", 1)
	if r := tool("describe_table", map[string]any{"connectionId": f.connID, "path": notesPath}); r.IsError {
		t.Fatalf("describe_table notes = %q", text(r))
	}
	if r := tool("describe_table", map[string]any{"connectionId": f.connID, "path": secretPath}); !r.IsError || !strings.Contains(text(r), "not listed") {
		t.Fatalf("describe_table secret = %q, want a refusal", text(r))
	}
	schema := tool("describe_schema", map[string]any{"connectionId": f.connID, "path": mainPath})
	if schema.IsError || strings.Contains(text(schema), "secret") || !strings.Contains(text(schema), "notes") {
		t.Fatalf("describe_schema under a list = %q, want only notes", text(schema))
	}
	conns := tool("list_connections", nil)
	var listed []struct {
		Tables struct {
			All   bool     `json:"all"`
			Names []string `json:"names"`
		} `json:"tables"`
	}
	if err := json.Unmarshal([]byte(text(conns)), &listed); err != nil || len(listed) == 0 {
		t.Fatalf("list_connections = %q (%v)", text(conns), err)
	}
	if listed[0].Tables.All || len(listed[0].Tables.Names) != 1 || listed[0].Tables.Names[0] != "notes" {
		t.Fatalf("list_connections tables = %+v, want exactly notes", listed[0].Tables)
	}

	// The MCP tab lists every live relation to pick from.
	listing, err := f.app.W.DbMcp.Tables(ctx, bridge.ConnectionsIDArgs{ID: f.connID})
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}
	f.app.Contract(t, "dbmcp-tables", "DbMcpService.Tables", listing)

	// [] means none; removing the key means all again.
	if err := f.app.W.ConnectionFiles.Patch(f.connID, configfs.NodeSet{Path: []string{"mcp", "tables"}, Value: []string{}}); err != nil {
		t.Fatalf("Patch empty list: %v", err)
	}
	refused("select body from notes", "not listed: notes")
	if err := f.app.W.ConnectionFiles.Patch(f.connID, configfs.NodeSet{Path: []string{"mcp"}, Value: map[string]any{"enabled": true}}); err != nil {
		t.Fatalf("Patch remove list: %v", err)
	}
	allowed("select token from secret")
}
