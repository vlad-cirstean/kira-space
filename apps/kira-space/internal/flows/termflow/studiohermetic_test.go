package termflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// TestHermeticStudio: Studio launches are hermetic with no switch. Continue in terminal carries
// project,local sources, and a smart run ticks an MCP server from the shared agent settings, not the
// CLI's. Frontend half: automations-smart.spec.ts.
func TestHermeticStudio(t *testing.T) {
	app := flowharness.New(t)
	app.Scenario(scenario(fakeagent.Action{Name: "done"}))
	dir := filepath.Join(app.SpaceHome, "agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	const settings = "mcp_servers:\n  app-srv:\n    url: http://127.0.0.1:9/mcp\n    headers:\n      Authorization: Bearer SENTINEL-APP\n"
	if err := os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}

	list, err := app.W.ScriptRuns.McpServers()
	if err != nil || list.Source != "app" || len(list.Servers) != 1 || list.Servers[0].Name != "app-srv" {
		t.Fatalf("McpServers = %+v, %v, want the shared settings' app-srv", list, err)
	}
	app.Contract(t, "automations-mcp-app-db", "ScriptRunsService.McpServers", list)

	rec := newSmart(t, app, "use the tool", func(f *scripts.CustomScriptFields) {
		f.Smart = &scripts.Smart{MCP: []scripts.MCPChoice{{Server: "app-srv", Tools: []string{"echo"}}}}
	})
	args := scriptruns.RunArgs{ScriptID: rec.ID}
	id, _ := startSmart(t, app, args)
	run := waitSmart(t, app, id, "done")
	argv := strings.Split(callFile(t, app, 1, ".args"), "\n")
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "--strict-mcp-config") || strings.Count(joined, "--mcp-config") != 2 {
		t.Fatalf("argv = %v, want --strict-mcp-config and two --mcp-config", argv)
	}
	if raw, err := os.ReadFile(filepath.Join(app.FakeDir, "automations-1.settings")); err == nil && strings.Contains(string(raw), "disableAllHooks") {
		t.Fatal("--settings keeps disableAllHooks under hermetic")
	}
	page, err := app.W.ScriptRuns.ReadLog(scriptruns.ReadLogArgs{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range page.Chunks {
		if strings.Contains(c.Text, "SENTINEL-APP") {
			t.Fatalf("log holds the app server header: %q", c.Text)
		}
	}

	resume, err := app.W.ScriptRuns.ResumeCommand(scriptruns.ResumeCommandArgs{RunID: id})
	if want := "claude --resume '" + run.SessionID + "' '--setting-sources' 'project,local'"; err != nil || !strings.HasPrefix(resume.Command, want) || !strings.Contains(resume.Command, "'--settings' ") || !strings.Contains(resume.Command, "'--mcp-config' ") {
		t.Fatalf("ResumeCommand = %q, %v, want %q then the app MCP file", resume.Command, err, want)
	}

	// Smart-run blocker: a server missing from the shared settings blocks the preview.
	gone := newSmart(t, app, "use another", func(f *scripts.CustomScriptFields) {
		f.Smart = &scripts.Smart{MCP: []scripts.MCPChoice{{Server: "user-srv", Tools: []string{"echo"}}}}
	})
	if pv := previewOf(t, app, scriptruns.RunArgs{ScriptID: gone.ID}); !strings.Contains(pv.Blocker, "user-srv") {
		t.Fatalf("blocker = %q, want one naming user-srv", pv.Blocker)
	}
}
