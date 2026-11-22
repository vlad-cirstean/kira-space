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
		res := tool("describe", map[string]any{"connectionId": f.connID, "path": path})
		if res.IsError {
			t.Fatalf("describe %q = %q", path, text(res))
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
		t.Fatal("describe found no database node")
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
		t.Fatalf("describe under a list = %v, want only notes", names)
	}
	secretPath := strings.Replace(notesPath, "notes", "secret", 1)
	if r := tool("describe", map[string]any{"connectionId": f.connID, "path": notesPath}); r.IsError {
		t.Fatalf("describe notes = %q", text(r))
	}
	if r := tool("describe", map[string]any{"connectionId": f.connID, "path": secretPath}); !r.IsError || !strings.Contains(text(r), "not listed") {
		t.Fatalf("describe secret = %q, want a refusal", text(r))
	}
	schema := tool("describe", map[string]any{"connectionId": f.connID, "path": mainPath})
	if schema.IsError || strings.Contains(text(schema), "secret") || !strings.Contains(text(schema), "notes") {
		t.Fatalf("describe under a list = %q, want only notes", text(schema))
	}
	conns := tool("list", map[string]any{"what": []string{"connections"}})
	var listed struct {
		Connections []struct {
			Tables []string `json:"tables"`
		} `json:"connections"`
	}
	if err := json.Unmarshal([]byte(text(conns)), &listed); err != nil || len(listed.Connections) == 0 {
		t.Fatalf("list = %q (%v)", text(conns), err)
	}
	if tables := listed.Connections[0].Tables; len(tables) != 1 || tables[0] != "notes" {
		t.Fatalf("list tables = %+v, want exactly notes", tables)
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

// J-MCP-002: with the allow list on, no spelling of an unlisted table reaches its rows. Each
// statement is refused before anything runs, nothing queues an approval, and the data stays put.
func TestDbMcpTableAllowListBypassAttempts(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.execSQL(t,
		"create table secret (id integer primary key, token text)",
		"insert into secret (token) values ('hush')",
		"create view secret_view as select * from secret",
	)
	if err := f.app.W.ConnectionFiles.Patch(f.connID, configfs.NodeSet{Path: []string{"mcp", "tables"}, Value: []string{"notes"}}); err != nil {
		t.Fatalf("Patch mcp.tables: %v", err)
	}
	attempts := []string{
		`select token from "secret"`,
		`select token from [secret]`,
		"select token from `secret`",
		`select token from SECRET`,
		`select token from main."secret"`,
		`select token from notes, secret`,
		`select token from notes natural join secret`,
		`select token from notes cross join secret`,
		`select body from notes union select token from secret`,
		`select body from notes union all select token from secret_view`,
		`select body from notes where exists (select 1 from secret)`,
		`select token from (select * from secret)`,
		`select (select token from secret)`,
		`select /* notes */ token from secret`,
		"select token from secret -- notes",
		`with n as (select 1) select token from secret`,
		`select * from (values ((select token from secret)))`,
		`select token from json_each('[1]'), secret`,
		`insert into notes (body) select token from secret`,
		`update notes set body = (select token from secret)`,
		`delete from notes where id in (select id from secret)`,
	}
	for _, stmt := range attempts {
		r := runQuery(t, sess, f.connID, stmt)
		if !r.IsError || strings.Contains(text(r), "hush") {
			t.Fatalf("%q = %q (error %v), want a refusal without the data", stmt, text(r), r.IsError)
		}
		if !strings.Contains(text(r), "not listed") && !strings.Contains(text(r), "could not be parsed") {
			t.Fatalf("%q refused as %q, want the allow-list or parse refusal", stmt, text(r))
		}
		if n := f.queueLen(t); n != 0 {
			t.Fatalf("%q queued %d approvals before the allow list refused it", stmt, n)
		}
	}
	if n := f.noteCount(t); n != 2 {
		t.Fatalf("notes has %d rows after the attempts, want 2", n)
	}
	var tokens int
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow("select count(*) from secret").Scan(&tokens); err != nil || tokens != 1 {
		t.Fatalf("secret holds %d rows (%v), want the one seeded", tokens, err)
	}
}
