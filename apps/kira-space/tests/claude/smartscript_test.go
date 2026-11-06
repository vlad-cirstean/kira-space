package claude

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// studioSmartScriptMCP is TestSmartScriptRun's MCP server subtests: a user server and an app
// server load through the real claude.
func studioSmartScriptMCP(t *testing.T) {
	t.Run("user MCP server loads through claude", func(t *testing.T) {
		app := newRealApp(t)
		app.SetHermetic(t, false) // hermetic runs tick the app's own servers instead
		if err := os.Remove(filepath.Join(app.BinDir, "claude")); err != nil {
			t.Fatal(err)
		}
		fakeMCP := filepath.Join(app.BinDir, "fake-mcp")
		cfg := fmt.Sprintf(`{"mcpServers":{"fake":{"command":%q,"env":{"TOKEN":"SENTINEL-ENV"}},"other":{"command":%q}}}`, fakeMCP, fakeMCP)
		if err := os.WriteFile(filepath.Join(app.Home, ".claude.json"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		list, err := app.W.ScriptRuns.McpServers()
		if err != nil {
			t.Fatalf("McpServers: %v", err)
		}
		servers := list.Servers
		listed := ""
		for _, s := range servers {
			if s.Name == "fake" {
				listed = s.Status
			}
		}
		if listed != "connected" {
			t.Fatalf("claude mcp list reports fake as %q, want connected: %+v", listed, servers)
		}
		run := smartRun(t, app, "Call the tool mcp__fake__echo with text zeta-7421. Then finish, with the text the tool returned as the summary.",
			[]string{"Read"}, scripts.MCPChoice{Server: "fake", Tools: []string{"echo"}})
		o := run.Outcome
		if run.State != "done" || o == nil || !o.Reported || o.CostUSD == nil {
			t.Fatalf("run = %s, outcome = %+v, want done and reported", run.State, o)
		}
		page, err := app.W.ScriptRuns.ReadLog(scriptruns.ReadLogArgs{ID: run.ID})
		if err != nil {
			t.Fatal(err)
		}
		var log strings.Builder
		for _, c := range page.Chunks {
			log.WriteString(c.Text + "\n")
		}
		if !strings.Contains(log.String(), "▸ mcp__fake__echo") {
			t.Errorf("log has no mcp__fake__echo tool call:\n%s", tail(log.String(), 1500))
		}
		if strings.Contains(log.String(), "mcp__other__") {
			t.Errorf("an unticked user server was loaded:\n%s", tail(log.String(), 1500))
		}
		if !strings.Contains(o.Summary, "zeta-7421") {
			t.Errorf("summary %q lacks the echoed text", o.Summary)
		}
		_ = filepath.WalkDir(app.KiraHome, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if data, _ := os.ReadFile(path); strings.Contains(string(data), "SENTINEL-ENV") {
					t.Errorf("%s holds a user server credential", path)
				}
			}
			return nil
		})
		t.Logf("PASS user MCP server: cost %.4f USD", *o.CostUSD)
	})

	t.Run("app MCP server loads through claude", func(t *testing.T) {
		app := newRealApp(t)
		if err := os.Remove(filepath.Join(app.BinDir, "claude")); err != nil {
			t.Fatal(err)
		}
		fakeMCP := filepath.Join(app.BinDir, "fake-mcp")
		dir := filepath.Join(app.SpaceHome, "agent")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		yaml := fmt.Sprintf("mcp_servers:\n  fake:\n    command: %q\n", fakeMCP)
		if err := os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		// A user-config server of the same kind must stay out of a hermetic run.
		cfg := fmt.Sprintf(`{"mcpServers":{"other":{"command":%q}}}`, fakeMCP)
		if err := os.WriteFile(filepath.Join(app.Home, ".claude.json"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		list, err := app.W.ScriptRuns.McpServers()
		if err != nil || list.Source != "app" || len(list.Servers) != 1 || list.Servers[0].Name != "fake" {
			t.Fatalf("McpServers = %+v, %v, want the app's fake", list, err)
		}
		run := smartRun(t, app, "Call the tool mcp__fake__echo with text zeta-7421. Then finish, with the text the tool returned as the summary.",
			[]string{"Read"}, scripts.MCPChoice{Server: "fake", Tools: []string{"echo"}})
		o := run.Outcome
		if run.State != "done" || o == nil || !o.Reported || o.CostUSD == nil || !strings.Contains(o.Summary, "zeta-7421") {
			t.Fatalf("run = %s, outcome = %+v, want done, reported, echoing zeta-7421", run.State, o)
		}
		t.Logf("PASS app MCP server: cost %.4f USD", *o.CostUSD)
	})
}
