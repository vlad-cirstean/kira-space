package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude/mcpinstall"
	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

const dbServer = "kira-db"

// enableDbMcp creates an MCP-exposed SQLite connection, turns the DB MCP server on and returns
// the connection name plus the URL and headersHelper path the app would register with Claude Code.
func enableDbMcp(t *testing.T, app *realApp) (connName, url, helper string) {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "real.sqlite")
	if err := os.WriteFile(dbFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	connName = "real-claude-sqlite"
	conn, err := app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: connName, Kind: "sqlite", Color: "blue", Mode: "fields", Database: &dbFile, Options: map[string]any{},
		McpEnabled: true, McpDescription: "scratch database", McpReadMode: "allow", McpWriteMode: "deny", McpDdlMode: "deny",
	}})
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

// TestDbMcpThroughClaude has the real claude call list_connections on the embedded DB MCP server.
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

	out := app.claudeP(t, t.TempDir(),
		"Call the list_connections tool once, then reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", "mcp__"+dbServer+"__list_connections",
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	uses := toolUses(msgs, "mcp__"+dbServer+"__list_connections")
	if len(uses) == 0 {
		t.Fatalf("no list_connections tool use in the stream:\n%s", tail(out, 1500))
	}
	text, isErr, ok := toolResult(msgs, uses[0].ID)
	if !ok || isErr {
		t.Fatalf("list_connections result missing or an error (ok=%v, text=%q)", ok, text)
	}
	if !strings.Contains(text, connName) {
		t.Fatalf("list_connections result lacks %q:\n%s", connName, text)
	}
	if res := resultMsg(t, msgs); res.IsError {
		t.Fatalf("claude run ended in error: %+v", res)
	}
}

// TestDbMcpScriptThroughClaude has the real claude run an allow-mode script over MCP, then read
// its output with get_script_logs: the run result itself carries no output. The filter is the
// model's choice, so the flow test covers it.
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

	out := app.claudeP(t, t.TempDir(),
		"You must use the kira-db MCP tools, loading them with ToolSearch if they are deferred. Call list_scripts, then run_script for the script probe with waitSeconds 20, then call get_script_logs for that run with contains set to marker. Reply with the matching line.",
		"--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", "ToolSearch,mcp__"+dbServer+"__list_scripts,mcp__"+dbServer+"__run_script,mcp__"+dbServer+"__get_script_logs",
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	runs := toolUses(msgs, "mcp__"+dbServer+"__run_script")
	logs := toolUses(msgs, "mcp__"+dbServer+"__get_script_logs")
	if len(runs) == 0 || len(logs) == 0 {
		t.Fatalf("run_script uses %d, get_script_logs uses %d, want both:\n%s", len(runs), len(logs), tail(out, 1500))
	}
	runText, isErr, ok := toolResult(msgs, runs[0].ID)
	if !ok || isErr || strings.Contains(runText, "beta-marker") {
		t.Fatalf("run_script result missing, an error or holding output (ok=%v, text=%q)", ok, runText)
	}
	logText, isErr, ok := toolResult(msgs, logs[0].ID)
	if !ok || isErr || !strings.Contains(logText, "beta-marker") {
		t.Fatalf("get_script_logs result = %q (ok=%v, err=%v), want the marker line", logText, ok, isErr)
	}
}

// TestDbMcpInstallRegisters registers the server through the real installer, with the arguments
// the app computes, under the temp HOME, and has the real claude list it connected.
func TestDbMcpInstallRegisters(t *testing.T) {
	app := newRealApp(t)
	_, url, helper := enableDbMcp(t, app)
	res := mcpinstall.New(mcpinstall.Deps{LookPath: func(string) (string, error) { return app.Claude, nil }}).Install(ctx, dbServer, url, helper)
	if res.Outcome != harness.OutcomeInstalled {
		t.Fatalf("Install = %+v, want installed", res)
	}
	raw, err := os.ReadFile(filepath.Join(app.Home, ".claude.json"))
	if err != nil || !strings.Contains(string(raw), dbServer) {
		t.Fatalf("temp .claude.json lacks %s (err %v):\n%s", dbServer, err, tail(string(raw), 600))
	}
	out, errOut, err := app.run(t, t.TempDir(), "mcp", "list")
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
