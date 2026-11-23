package claude

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude/mcpinstall"
	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/scripts"
	_ "modernc.org/sqlite"
)

const dbServer = "kira-studio"

var storedName = regexp.MustCompile(`\b(?:plain|secret)\.([A-Za-z]\w*)\s*=[^=]`)

// enableDbMcp creates an MCP-exposed SQLite connection, turns the DB MCP server on and returns
// the connection name plus the URL and headersHelper path the app would register with Claude Code.
// tweak edits the connection fields before they are saved; it gets the database file path.
func enableDbMcp(t *testing.T, app *realApp, tweak ...func(*model.ConnectionFields, string)) (connName, url, helper string) {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "real.sqlite")
	if err := os.WriteFile(dbFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	connName = "real-claude-sqlite"
	fields := model.ConnectionFields{
		Name: connName, Kind: "sqlite", Color: "blue", Mode: "fields", Database: &dbFile, Options: map[string]any{},
		McpEnabled: true, McpDescription: "scratch database", McpReadMode: "allow", McpWriteMode: "deny", McpDdlMode: "deny",
	}
	for _, f := range tweak {
		f(&fields, dbFile)
	}
	conn, err := app.W.Connections.Create(connections.Input{ConnectionFields: fields})
	if err != nil {
		t.Fatalf("Connections.Create: %v", err)
	}
	if _, err := app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: conn.ID}); err != nil {
		t.Fatalf("Connections.Connect: %v", err)
	}
	st, err := app.W.DbMcp.SetEnabled(bridge.DbMcpSetEnabledArgs{Enabled: true})
	if err != nil || !st.Running {
		t.Fatalf("DbMcp.SetEnabled = %+v, %v, want running", st, err)
	}
	// The harness's installer fake records what InstallAgent would register; the call is
	// synchronous, so Installs is stable after it.
	app.W.DbMcp.InstallAgent(ctx)
	if len(app.McpInstaller.Installs) != 1 {
		t.Fatalf("installer fake saw %d installs, want 1", len(app.McpInstaller.Installs))
	}
	in := app.McpInstaller.Installs[0]
	return connName, in.URL, in.Token
}

// TestDbMcpThroughClaude has the real claude call list on the embedded DB MCP server.
func TestDbMcpThroughClaude(t *testing.T) {
	app := newRealApp(t)
	connName, url, helper := enableDbMcp(t, app)
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Call the MCP tool mcp__"+dbServer+"__list (it lists database connections) once, then reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", "mcp__"+dbServer+"__list",
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	uses := toolUses(msgs, "mcp__"+dbServer+"__list")
	if len(uses) == 0 {
		t.Fatalf("no list tool use in the stream; result=%q", resultMsg(t, msgs).Result)
	}
	text, isErr, ok := toolResult(msgs, uses[0].ID)
	if !ok || isErr {
		t.Fatalf("list result missing or an error (ok=%v, text=%q)", ok, text)
	}
	if !strings.Contains(text, connName) {
		t.Fatalf("list result lacks %q:\n%s", connName, text)
	}
	if res := resultMsg(t, msgs); res.IsError {
		t.Fatalf("claude run ended in error: %+v", res)
	}
}

// TestDbMcpRunQueryModeThroughClaude has the real claude declare a mode on run_query (P527): the
// schema requires it, and a read runs on the read lane.
func TestDbMcpRunQueryModeThroughClaude(t *testing.T) {
	app := newRealApp(t)
	connName, url, helper := enableDbMcp(t, app)
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	tool := "mcp__" + dbServer + "__run_query"
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Call list, then call run_query once on the connection named "+connName+" with sql \"SELECT 1 AS n\" and an empty path. Declare the least powerful mode that fits. Then reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", "mcp__"+dbServer+"__list,"+tool,
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	uses := toolUses(msgs, tool)
	if len(uses) == 0 {
		t.Fatalf("no run_query tool use in the stream:\n%s", tail(out, 1500))
	}
	var in struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(uses[len(uses)-1].Input, &in); err != nil || in.Mode != "read" {
		t.Fatalf("run_query input = %s (%v), want mode read", uses[len(uses)-1].Input, err)
	}
	text, isErr, ok := toolResult(msgs, uses[len(uses)-1].ID)
	if !ok || isErr || !strings.Contains(text, "\"n\"") {
		t.Fatalf("run_query result = %q (ok=%v, error=%v), want the n column", text, ok, isErr)
	}
}

// TestDbMcpBrowseThroughClaude has the real claude read rows through browse_table (P535) on a
// connection whose read is denied and masked is allowed. The flow tests cover the masking itself.
func TestDbMcpBrowseThroughClaude(t *testing.T) {
	app := newRealApp(t)
	connName, url, helper := enableDbMcp(t, app, func(f *model.ConnectionFields, dbFile string) {
		db, err := sql.Open("sqlite", dbFile)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.Exec(`CREATE TABLE notes (id INTEGER PRIMARY KEY, title TEXT, email TEXT);
			INSERT INTO notes (title, email) VALUES ('a','a@x.io'),('b','b@x.io'),('c','c@x.io'),('d','d@x.io')`); err != nil {
			t.Fatal(err)
		}
		f.McpReadMode, f.McpMaskedMode = "deny", "allow"
	})
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	tool := "mcp__" + dbServer + "__browse_table"
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Show me 3 rows of the notes table in the connection named "+connName+". Then reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", "mcp__"+dbServer+"__list,"+tool+",mcp__"+dbServer+"__describe",
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	uses := toolUses(msgs, tool)
	if len(uses) == 0 {
		t.Fatalf("no browse_table tool use in the stream:\n%s", tail(out, 1500))
	}
	text, isErr, ok := toolResult(msgs, uses[len(uses)-1].ID)
	if !ok || isErr || !strings.Contains(text, "title") {
		t.Fatalf("browse_table result = %q (ok=%v, error=%v), want rows", text, ok, isErr)
	}
}

// TestDbMcpBrowsePreferredThroughClaude (P609): with read and masked both allowed, run_query is a real
// option, yet the real claude counts with browse_table's count and in filter and pages with offset.
func TestDbMcpBrowsePreferredThroughClaude(t *testing.T) {
	app := newRealApp(t)
	connName, url, helper := enableDbMcp(t, app, func(f *model.ConnectionFields, dbFile string) {
		db, err := sql.Open("sqlite", dbFile)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.Exec(`CREATE TABLE notes (id INTEGER PRIMARY KEY, title TEXT);
			INSERT INTO notes (title) VALUES ('a'),('b'),('c'),('d'),('e')`); err != nil {
			t.Fatal(err)
		}
		f.McpReadMode, f.McpMaskedMode = "allow", "allow"
	})
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	browseTool, queryTool := "mcp__"+dbServer+"__browse_table", "mcp__"+dbServer+"__run_query"
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"In the connection named "+connName+", count the rows of the notes table whose id is 1, 2 or 5, then list every note two per page. Then reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", "mcp__"+dbServer+"__list,"+browseTool+","+queryTool+",mcp__"+dbServer+"__describe",
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	if uses := toolUses(msgs, queryTool); len(uses) > 0 {
		t.Fatalf("claude called run_query %d time(s), want browse_table only: %s", len(uses), uses[0].Input)
	}
	var counted, paged bool
	for _, u := range toolUses(msgs, browseTool) {
		var in struct {
			Count   bool `json:"count"`
			Offset  int  `json:"offset"`
			Filters []struct {
				Op string `json:"op"`
			} `json:"filters"`
		}
		if err := json.Unmarshal(u.Input, &in); err != nil {
			t.Fatalf("browse_table input %s: %v", u.Input, err)
		}
		for _, f := range in.Filters {
			counted = counted || (in.Count && f.Op == "in")
		}
		paged = paged || in.Offset > 0
	}
	if !counted || !paged {
		t.Fatalf("browse_table calls: count with in filter = %v, offset paging = %v, want both:\n%s", counted, paged, tail(out, 2500))
	}
}

// TestDbMcpScriptThroughClaude has the real claude run an allow-mode script over MCP, then read
// the output from the run result itself. The run_script read filter is the model's choice, so the
// flow test covers it.
func TestDbMcpScriptThroughClaude(t *testing.T) {
	app := newRealApp(t)
	_, url, helper := enableDbMcp(t, app)
	if _, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "probe", Command: "printf 'alpha\\nbeta-marker\\ngamma\\n'",
		MCP: &scripts.MCPAccess{Enabled: true, Run: scripts.MCPRunAllow, Description: "Prints three lines."},
	}}); err != nil {
		t.Fatalf("CustomScripts.Create: %v", err)
	}
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	out, _ := app.claudeP(t, t.TempDir(), nil,
		"You must use the kira-studio MCP tools, loading them with ToolSearch if they are deferred. Call list, then run_script for the script probe with waitSeconds 20. Reply with the output line containing marker.",
		"--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", "ToolSearch,mcp__"+dbServer+"__list,mcp__"+dbServer+"__run_script",
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	runs := toolUses(msgs, "mcp__"+dbServer+"__run_script")
	if len(runs) == 0 {
		t.Fatalf("no run_script use:\n%s", tail(out, 1500))
	}
	runText, isErr, ok := toolResult(msgs, runs[0].ID)
	if !ok || isErr || !strings.Contains(runText, "beta-marker") {
		t.Fatalf("run_script result = %q (ok=%v, err=%v), want the marker line inline", runText, ok, isErr)
	}
}

// TestDbMcpInstallRegisters registers the server through the real installer, with the arguments
// the app computes, under the temp HOME, and has the real claude list it connected.
func TestDbMcpInstallRegisters(t *testing.T) {
	app := newRealApp(t)
	_, url, helper := enableDbMcp(t, app)
	inst := mcpinstall.New(mcpinstall.Deps{LookPath: func(string) (string, error) { return app.Claude, nil }})
	if got, err := inst.Registered(ctx, dbServer); err != nil || got {
		t.Fatalf("Registered before install = %v, %v, want false", got, err)
	}
	res := inst.Install(ctx, dbServer, url, helper)
	if res.Outcome != harness.OutcomeInstalled {
		t.Fatalf("Install = %+v, want installed", res)
	}
	if got, err := inst.Registered(ctx, dbServer); err != nil || !got {
		t.Fatalf("Registered after install = %v, %v, want true", got, err)
	}
	raw, err := os.ReadFile(filepath.Join(app.Home, ".claude.json"))
	if err != nil || !strings.Contains(string(raw), dbServer) {
		t.Fatalf("temp .claude.json lacks %s (err %v):\n%s", dbServer, err, tail(string(raw), 600))
	}
	out, errOut, err := app.run(t, t.TempDir(), nil, "mcp", "list")
	if err != nil {
		t.Fatalf("claude mcp list: %v\n%s", err, tail(out+errOut, 800))
	}
	var line string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, dbServer) {
			line = l
		}
	}
	if line == "" || !strings.Contains(line, "Connected") {
		t.Fatalf("claude mcp list has no connected %s:\n%s", dbServer, out)
	}
}

// TestDbMcpSessionStep runs a headless agent step with db_mcp on while the Settings switch is off:
// the real claude must reach the session server and call list.
func TestDbMcpSessionStep(t *testing.T) {
	f := newAdeFixture(t, nil)
	dbFile := filepath.Join(t.TempDir(), "real.sqlite")
	if err := os.WriteFile(dbFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	connName := "real-claude-sqlite"
	conn, err := f.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: connName, Kind: "sqlite", Color: "blue", Mode: "fields", Database: &dbFile, Options: map[string]any{},
		McpEnabled: true, McpDescription: "scratch database", McpReadMode: "allow", McpWriteMode: "deny", McpDdlMode: "deny",
	}})
	if err != nil {
		t.Fatalf("Connections.Create: %v", err)
	}
	if _, err := f.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: conn.ID}); err != nil {
		t.Fatalf("Connections.Connect: %v", err)
	}
	f.saveWorkflow(t, "flow", "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 3m\n    db_mcp: true\n"+
		"    prompt: Call the list tool of the kira-studio-session server once, then call state with action step.finish and args status done.\n"+
		manualStepYAML)
	run := f.startRun(t, f.createTask(t, "flow"), "feat/real-claude-db")
	log := f.runLog(t, run.ID)
	if run.State != "done" {
		t.Fatalf("run = %+v, want done; log:\n%s", run, tail(log, 1500))
	}
	if !strings.Contains(log, "mcp__kira-studio-session__list") || !strings.Contains(log, connName) {
		t.Fatalf("run log lacks the list call or %q:\n%s", connName, tail(log, 1500))
	}
}

// TestDbMcpKeptChainThroughClaude (P686, P716): Claude Code accepts the generated run_query schema with
// `script` (nullable object) and `params`. The prompt names the task only: the model learns kept variables
// from the tool descriptions alone, keeps the ids in a script and binds them by name in a second call. The
// flow tests cover every rule; this checks the real client and model.
func TestDbMcpKeptChainThroughClaude(t *testing.T) {
	app := newRealApp(t)
	connName, url, helper := enableDbMcp(t, app, func(f *model.ConnectionFields, dbFile string) {
		db, err := sql.Open("sqlite", dbFile)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.Exec(`CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT);
			INSERT INTO notes (body) VALUES ('one'),('two')`); err != nil {
			t.Fatal(err)
		}
	})
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	tool := "mcp__" + dbServer + "__run_query"
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Use the kira-studio MCP tools, loading them with ToolSearch if they are deferred. The connection named "+connName+
			" has a notes table (id, body). Read every note's id in one read query, then in a second read query fetch the bodies of those ids, ordered by id."+
			" Do not put the ids in your own text or your own sql: they must never pass through you. Reply with the bodies.",
		"--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", "ToolSearch,mcp__"+dbServer+"__list,"+tool,
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	var scripted, bound bool
	var keptName string
	for _, u := range toolUses(msgs, tool) {
		var in struct {
			Script *string  `json:"script"`
			Params []string `json:"params"`
		}
		if err := json.Unmarshal(u.Input, &in); err != nil {
			t.Fatalf("run_query input %s: %v", u.Input, err)
		}
		text, isErr, ok := toolResult(msgs, u.ID)
		switch {
		case in.Script != nil && storedName.MatchString(*in.Script):
			name := storedName.FindStringSubmatch(*in.Script)[1]
			if strings.Contains(text, "[1,2]") {
				t.Fatalf("scripted run_query result = %q, want no ids", text)
			}
			// A script the model got wrong (error, nothing set) is its own retry, not a product fault.
			if !ok || isErr || !strings.Contains(text, `"tempSet":[{"name":"`+name+`"`) {
				continue
			}
			keptName, scripted = name, true
		case keptName != "" && len(in.Params) == 1 && in.Params[0] == keptName:
			if !ok || isErr || !strings.Contains(text, "one") || !strings.Contains(text, "two") {
				t.Fatalf("params run_query result = %q (ok=%v, error=%v), want both bodies", text, ok, isErr)
			}
			bound = true
		}
	}
	if !scripted || !bound {
		t.Fatalf("scripted call = %v, params call = %v, want both:\n%s", scripted, bound, tail(out, 2500))
	}
}

// TestDbMcpConfirmThroughClaude has the real claude run a read the connection allows while asked to have
// the user confirm it first (P742): the agent sets confirm, the user sees a confirm item with the agent's
// sentence, and approving it lets the query run.
func TestDbMcpConfirmThroughClaude(t *testing.T) {
	app := newRealApp(t)
	connName, url, helper := enableDbMcp(t, app)
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var seen *approvals.Item
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
			}
			items, err := app.W.ApprovalsSvc.List()
			if err != nil {
				continue
			}
			for _, it := range items {
				if it.Kind != approvals.KindDbQuery {
					continue
				}
				if _, err := app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{it.ID}, Approve: true}); err == nil {
					mu.Lock()
					if seen == nil {
						seen = &it
					}
					mu.Unlock()
				}
			}
		}
	}()

	tool := "mcp__" + dbServer + "__run_query"
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Call list, then call run_query once on the connection named "+connName+" with sql \"SELECT 1 AS n\", mode read and an empty path. Treat it as sensitive: the user must approve it in the approval prompt, not in chat; make the tool ask for that prompt. Then reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", "mcp__"+dbServer+"__list,"+tool,
		"--output-format", "stream-json", "--verbose")
	close(stop)
	wg.Wait()
	msgs := parseStream(out)

	mu.Lock()
	defer mu.Unlock()
	if seen == nil || seen.Reason != "confirm" || strings.TrimSpace(seen.AgentNote) == "" {
		t.Fatalf("approval item = %+v, want a db.query confirm item with the agent's sentence:\n%s", seen, tail(out, 1500))
	}
	uses := toolUses(msgs, tool)
	if len(uses) == 0 {
		t.Fatalf("no run_query tool use in the stream:\n%s", tail(out, 1500))
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if err := json.Unmarshal(uses[len(uses)-1].Input, &in); err != nil || strings.TrimSpace(in.Confirm) == "" {
		t.Fatalf("run_query input = %s (%v), want a confirm sentence", uses[len(uses)-1].Input, err)
	}
	text, isErr, ok := toolResult(msgs, uses[len(uses)-1].ID)
	if !ok || isErr || !strings.Contains(text, "\"n\"") {
		t.Fatalf("run_query result = %q (ok=%v, error=%v), want the n column", text, ok, isErr)
	}
}
