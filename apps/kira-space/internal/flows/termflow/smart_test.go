package termflow_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// settingsPath is the per-launch app settings file in a Continue in terminal command.
var settingsPath = regexp.MustCompile(`'--settings' '[^']*'`)

func TestSmartScriptSpace(t *testing.T) {
	app := flowharness.New(t)
	app.SetHermetic(t, false) // ticks a server of the CLI's own config
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"*": {{
		Name: "done", MCP: []fakeagent.MCPCall{{Server: "fake", Tool: "echo", Args: map[string]any{"text": "hi"}}},
	}}}})
	if err := os.WriteFile(filepath.Join(app.Home, ".claude.json"),
		[]byte(fmt.Sprintf(`{"mcpServers":{"fake":{"command":%q}}}`, filepath.Join(app.BinDir, "fake-mcp"))), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := app.W.ScriptRuns.McpServers()
	servers := list.Servers
	if err != nil || list.Source != "agent" || len(servers) != 1 || servers[0].Name != "fake" || servers[0].Target != "fake-mcp" || servers[0].Status != "connected" {
		t.Fatalf("McpServers = %+v, %v", servers, err)
	}

	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "ask", Kind: scripts.KindSmart, Command: "Say hi about {topic}", Color: "blue",
		Params: []scripts.Param{{Name: "topic", Type: scripts.ParamText}},
		Smart:  &scripts.Smart{MCP: []scripts.MCPChoice{{Server: "fake", Tools: []string{"echo"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	args := scriptruns.RunArgs{ScriptID: rec.ID, Params: map[string][]string{"topic": {"cats"}}}
	pv, err := app.W.ScriptRuns.Preview(args)
	if err != nil {
		t.Fatal(err)
	}
	// Contract smart: tests/ui/automations-smart.spec.ts "contract: a smart script ..." reads the same fixture.
	app.Contract(t, "smart", "CustomScriptsService.Create", rec)
	app.Contract(t, "smart", "ScriptRunsService.Preview", pv, flowharness.Mask("hash"))
	app.Contract(t, "smart", "args:ScriptRunsService.Start", scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash}, flowharness.Mask("hash"))
	if got := strings.Join(pv.Allowed, " "); strings.Contains(got, "mcp__kira-space") || !strings.Contains(got, "mcp__fake__echo") {
		t.Fatalf("allowed = %q, want the fake tool and no Space tools without a task", got)
	}
	started, err := app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash})
	if err != nil {
		t.Fatal(err)
	}
	var run scriptruns.Run
	testx.WaitUntil(t, waitFor, func() bool {
		run, _ = app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
		return run.State == "done"
	})
	if raw, err := os.ReadFile(filepath.Join(app.FakeDir, "automations-1.settings")); err != nil ||
		string(raw) != `{"disableAllHooks":true,"allowedMcpServers":[{"serverName":"kira-ade"},{"serverName":"fake"}],"enableAllProjectMcpServers":false,"enabledMcpjsonServers":[]}` {
		t.Fatalf("--settings = %s, %v", raw, err)
	}
	app.Contract(t, "smart", "ScriptRunsService.Get#done", run, flowharness.Mask("createdAt", "startedAt", "finishedAt", "hash", "costUsd"))
	if o := run.Outcome; o == nil || !o.Reported || o.Source != "agent" {
		t.Fatalf("outcome = %+v", o)
	}
	if want := scripts.PlainText(pv.Prompt) + "\n\n" + agentmcp.ScriptReport(llmprompts.Defaults()); run.Prompt != want {
		t.Fatalf("prompt = %q, want %q", run.Prompt, want)
	}
	page, err := app.W.ScriptRuns.ReadLog(scriptruns.ReadLogArgs{ID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "smart", "ScriptRunsService.ReadLog", page)
	if raw, _ := json.Marshal(page); len(page.Chunks) == 0 {
		t.Fatalf("ReadLog empty: %s", raw)
	}
}

func TestSmartScriptFailedSpace(t *testing.T) {
	app := flowharness.New(t)
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"*": {{Name: "failed"}}}})
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "ask", Kind: scripts.KindSmart, Command: "Say hi", Color: "blue",
	}})
	if err != nil {
		t.Fatal(err)
	}
	args := scriptruns.RunArgs{ScriptID: rec.ID}
	pv, err := app.W.ScriptRuns.Preview(args)
	if err != nil {
		t.Fatal(err)
	}
	started, err := app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash})
	if err != nil {
		t.Fatal(err)
	}
	var run scriptruns.Run
	testx.WaitUntil(t, waitFor, func() bool {
		run, _ = app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
		return run.State == "failed"
	})
	if o := run.Outcome; o == nil || o.Status != "failed" || o.Reason != "summary-failed" || !o.Reported || o.Source != "agent" {
		t.Fatalf("outcome = %+v, want the agent's failure with its summary", o)
	}
	app.Contract(t, "smart", "CustomScriptsService.Create#failed", rec)
	app.Contract(t, "smart", "ScriptRunsService.Get#failed", run, flowharness.Mask("createdAt", "startedAt", "finishedAt", "hash", "costUsd"))
	launch, err := app.W.ScriptRuns.ResumeCommand(scriptruns.ResumeCommandArgs{RunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if want := "claude --resume '" + run.SessionID + "' '--setting-sources' 'project,local'"; !strings.HasPrefix(launch.Command, want+" '--settings' '") {
		t.Fatalf("ResumeCommand = %q, want %q", launch.Command, want)
	}
	app.Contract(t, "smart", "ScriptRunsService.ResumeCommand", scriptruns.ResumeLaunch{Command: settingsPath.ReplaceAllString(strings.Replace(launch.Command, run.SessionID, "sess-1", 1), "'--settings' '<settings>'"), Token: launch.Token}, flowharness.Mask("token"))
}
