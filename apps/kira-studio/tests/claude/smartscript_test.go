package claude

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// smartRun creates a smart script with the given tools, runs it and waits for it to leave running.
func smartRun(t *testing.T, app *realApp, prompt string, tools []string, mcp ...scripts.MCPChoice) scriptruns.Run {
	t.Helper()
	// The harness puts a fake claude first on PATH; drop it so the login shell finds the real one.
	if err := os.Remove(filepath.Join(app.BinDir, "claude")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "real", Kind: scripts.KindSmart, Command: prompt, Color: "blue",
		Smart: &scripts.Smart{Model: "haiku", MaxBudgetUSD: 0.05, Timeout: "3m", Tools: tools, MCP: mcp},
	}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	args := scriptruns.RunArgs{ScriptID: rec.ID}
	pv, err := app.W.ScriptRuns.Preview(args)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	started, err := app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var run scriptruns.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		run, _ = app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
		return run.State != "running"
	})
	return run
}

// TestSmartScriptRun runs smart scripts through the real claude: a reported success with a cost,
// and a prompt that asks for a tool the script was not given.
func TestSmartScriptRun(t *testing.T) {
	t.Run("reports done with a cost", func(t *testing.T) {
		run := smartRun(t, newRealApp(t), "Reply with the word ok.", []string{"Read", "Grep", "Glob"})
		o := run.Outcome
		if run.State != "done" || o == nil || !o.Reported || o.CostUSD == nil || *o.CostUSD <= 0 {
			t.Fatalf("run = %s, outcome = %+v, want done, reported, cost > 0", run.State, o)
		}
		t.Logf("PASS reports done: cost %.4f USD", *o.CostUSD)
	})

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

	t.Run("a tool outside the allowlist does not run", func(t *testing.T) {
		start := time.Now()
		run := smartRun(t, newRealApp(t), "Use the Bash tool to create a file named marker.txt containing hi. If you cannot, say so.", []string{"Read", "Grep", "Glob"})
		if run.Outcome == nil || !run.Outcome.Reported {
			t.Fatalf("outcome = %+v, want a reported status", run.Outcome)
		}
		if _, err := os.Stat(filepath.Join(run.Cwd, "marker.txt")); err == nil {
			t.Fatal("marker.txt exists: Bash ran without being allowed")
		}
		t.Logf("PASS no marker.txt: status %s in %s", run.Outcome.Status, time.Since(start).Round(time.Second))
	})
}
