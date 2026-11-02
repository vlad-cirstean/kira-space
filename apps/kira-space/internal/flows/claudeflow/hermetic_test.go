package claudeflow_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const hermeticYAML = `mcp_servers:
  app-srv:
    url: http://127.0.0.1:9/mcp
    headers:
      Authorization: Bearer ${secret:tok}
`

func saveAgentSettings(t *testing.T, app *flowharness.App, text string) {
	t.Helper()
	svc := app.W.AgentSettingsSvc
	cur, err := svc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(agentcfg.SaveArgs{Text: text, BaseHash: cur.Hash}); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// TestHermeticClaude: a hermetic launch reads project and local settings only, drops user config,
// claude.ai connectors and auto memory, and ticks MCP servers from the app's agent settings. Hermetic
// off keeps the P444 path. Frontend half: automations-smart.spec.ts.
func TestHermeticClaude(t *testing.T) {
	const secret = "hermetic-secret-value"
	t.Setenv(fakeagent.EnvRecordEnv, "ENABLE_CLAUDEAI_MCP_SERVERS,CLAUDE_CODE_DISABLE_AUTO_MEMORY")
	app := flowharness.New(t)
	repo := app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n"})
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	rec := importRepo(t, app, repo.Dir)
	saveAgentSettings(t, app, hermeticYAML)
	if _, err := app.W.AgentSettingsSvc.SetSecret(agentcfg.SetSecretArgs{Name: "tok", Value: secret}); err != nil {
		t.Fatal(err)
	}

	t.Run("terminal_launch", func(t *testing.T) {
		argv := launchArgv(t, app, rec, "flow1")
		if got := flagValues(argv, "--setting-sources"); len(got) != 1 || got[0] != "project,local" {
			t.Fatalf("--setting-sources = %q, want project,local", got)
		}
		for _, a := range argv {
			if a == "--strict-mcp-config" {
				t.Fatal("TUI launch passed --strict-mcp-config, want local-scope MCP kept")
			}
		}
		var env string
		testx.WaitUntil(t, waitFor, func() bool {
			files, _ := filepath.Glob(filepath.Join(app.FakeDir, "*.appenv"))
			for _, f := range files {
				b, _ := os.ReadFile(f)
				env += string(b)
			}
			return strings.Contains(env, "ENABLE_CLAUDEAI_MCP_SERVERS=false")
		})
		if !strings.Contains(env, "CLAUDE_CODE_DISABLE_AUTO_MEMORY=1") {
			t.Fatalf("launch env = %q, want auto memory off", env)
		}
	})

	script, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "ask", Kind: scripts.KindSmart, Command: "Reply ok.", Color: "blue",
		Smart: &scripts.Smart{MCP: []scripts.MCPChoice{{Server: "app-srv", Tools: []string{"echo"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	run := func(t *testing.T) []string {
		t.Helper()
		args := scriptruns.RunArgs{ScriptID: script.ID}
		pv, err := app.W.ScriptRuns.Preview(args)
		if err != nil || pv.Blocker != "" {
			t.Fatalf("Preview = %+v, %v", pv, err)
		}
		return agentArgv(t, app, func() {
			started, err := app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash})
			if err != nil {
				t.Fatal(err)
			}
			testx.WaitUntil(t, waitFor, func() bool {
				r, _ := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
				return r.State == "done"
			})
		})
	}

	t.Run("smart_ticks_app_server", func(t *testing.T) {
		list, err := app.W.ScriptRuns.McpServers()
		if err != nil || list.Source != "app" || len(list.Servers) != 1 || list.Servers[0].Name != "app-srv" {
			t.Fatalf("McpServers = %+v, %v, want the app's app-srv", list, err)
		}
		app.Contract(t, "automations-mcp-app", "ScriptRunsService.McpServers", list)
		argv := run(t)
		if got := flagValues(argv, "--setting-sources"); len(got) != 1 || got[0] != "" {
			t.Fatalf("--setting-sources = %q, want one empty value", got)
		}
		var strict bool
		for _, a := range argv {
			strict = strict || a == "--strict-mcp-config"
		}
		if !strict || len(flagValues(argv, "--mcp-config")) != 2 {
			t.Fatalf("argv = %q, want --strict-mcp-config and two --mcp-config", argv)
		}
		for _, p := range flagValues(argv, "--settings") {
			if raw, _ := os.ReadFile(p); strings.Contains(string(raw), "disableAllHooks") {
				t.Fatalf("--settings %s keeps disableAllHooks under hermetic", raw)
			}
		}
		stems, _ := filepath.Glob(filepath.Join(app.FakeDir, "automations-*.mcp*"))
		var held bool
		for _, f := range stems {
			if b, _ := os.ReadFile(f); strings.Contains(string(b), "Bearer "+secret) {
				held = true
			}
		}
		if !held {
			t.Fatalf("no recorded mcp file holds the resolved header (files %v)", stems)
		}
		testx.WaitUntil(t, waitFor, func() bool {
			var left []string
			_ = filepath.WalkDir(app.SpaceHome, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() && strings.HasSuffix(p, "-app.mcp.json") && !strings.HasPrefix(filepath.Base(p), "tui-") {
					left = append(left, p)
				}
				return nil
			})
			return len(left) == 0
		})
	})

	t.Run("user_server_blocks", func(t *testing.T) {
		if _, err := app.W.CustomScripts.Update(bridge.CustomScriptsUpdateArgs{ID: script.ID, Fields: scripts.CustomScriptFields{
			Name: "ask", Kind: scripts.KindSmart, Command: "Reply ok.", Color: "blue",
			Smart: &scripts.Smart{MCP: []scripts.MCPChoice{{Server: "user-srv", Tools: []string{"echo"}}}},
		}}); err != nil {
			t.Fatal(err)
		}
		pv, err := app.W.ScriptRuns.Preview(scriptruns.RunArgs{ScriptID: script.ID})
		if err != nil || pv.Blocker == "" || !strings.Contains(pv.Blocker, "user-srv") {
			t.Fatalf("Preview = %+v, %v, want a blocker naming user-srv", pv, err)
		}
		app.Contract(t, "automations-mcp-app", "ScriptRunsService.Preview#blocked", pv, flowharness.Mask("hash"))
	})

	t.Run("hermetic_off", func(t *testing.T) {
		app.SetHermetic(t, false)
		list, err := app.W.ScriptRuns.McpServers()
		if err != nil || list.Source != "agent" {
			t.Fatalf("McpServers source = %q, %v, want agent", list.Source, err)
		}
		argv := launchArgv(t, app, rec, "flow2")
		if got := flagValues(argv, "--setting-sources"); len(got) != 0 {
			t.Fatalf("--setting-sources = %q, want none with hermetic off", got)
		}
	})
}

// launchArgv starts a task step on the repo and returns the argv of the claude TUI it opens.
func launchArgv(t *testing.T, app *flowharness.App, rec model.CodeRepo, flow string) []string {
	t.Helper()
	saveWorkflow(t, app, flow, "id: "+flow+"\nname: Flow\nsteps:\n  - id: work\n    name: Work\n    kind: manual\n    status: In progress\n    session: true\n")
	tk, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix " + flow, CodeRepoIDs: []string{rec.ID}, WorkflowID: flow})
	if err != nil {
		t.Fatal(err)
	}
	l, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: tk.ID})
	if err != nil {
		t.Fatal(err)
	}
	return agentArgv(t, app, func() {
		if _, err := app.W.Terminal.Open(terminal.OpenArgs{
			TerminalID: l.TerminalID, WindowKey: window, Cwd: l.Cwd, Cols: 100, Rows: 30,
			Command: l.Command, LaunchKind: terminal.LaunchKindAgent,
		}); err != nil {
			t.Fatal(err)
		}
	})
}
