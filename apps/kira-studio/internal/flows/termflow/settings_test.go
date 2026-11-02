package termflow

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func appSettingsFiles(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(filepath.Base(p), "-app.") && strings.HasSuffix(p, ".json") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// TestAgentSettingsStudio: Studio reads the agent settings Kira Space hosts (KIRA_SPACE_HOME/agent) and
// the credential store both apps open. A smart script is isolated and gets none; Continue in terminal
// gets the app MCP file, and the token carries the env.
func TestAgentSettingsStudio(t *testing.T) {
	const secret = "STUDIO-SETTINGS-SECRET"
	t.Setenv(fakeagent.EnvRecordEnv, "P453_MARK")
	app := flowharness.New(t)
	app.Scenario(scenario(fakeagent.Action{Name: "done"}))
	dir := filepath.Join(app.SpaceHome, "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "env:\n  P453_MARK: ${secret:mark}\nmcp_servers:\n  gh:\n    url: http://127.0.0.1:9/mcp\n    headers:\n      Authorization: Bearer ${secret:mark}\n"
	if err := os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	sec, err := agentcfg.OpenSecrets(filepath.Join(app.SpaceHome, "agent-secrets.db"), secrets.New())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sec.Close() }()
	if err := sec.Put("mark", secret); err != nil {
		t.Fatal(err)
	}

	rec := newSmart(t, app, "Say hi", nil)
	runID, _ := startSmart(t, app, scriptruns.RunArgs{ScriptID: rec.ID})
	run := waitSmart(t, app, runID, "done")
	waitCall(t, app, 1)
	if got := callFile(t, app, 1, ".mcp0"); strings.Contains(got, `"gh"`) {
		t.Fatalf("an isolated smart script got the app MCP file: %s", got)
	}
	if _, err := os.Stat(filepath.Join(app.FakeDir, "automations-1.appenv")); err == nil {
		t.Fatal("an isolated smart script got the app env")
	}

	resume, err := app.W.ScriptRuns.ResumeCommand(scriptruns.ResumeCommandArgs{RunID: run.ID})
	if err != nil || !strings.Contains(resume.Command, "--mcp-config") || resume.Token == "" {
		t.Fatalf("ResumeCommand = %+v, %v, want an app file and a token", resume, err)
	}
	if strings.Contains(resume.Command, secret) {
		t.Fatalf("resume command holds the secret: %q", resume.Command)
	}
	if len(appSettingsFiles(app.KiraHome)) == 0 {
		t.Fatalf("no app file after ResumeCommand under %s", app.KiraHome)
	}
	cwd := filepath.Join(app.Home, "term", "work") // the fake names its files after the cwd's parent
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "t-resume", Cwd: cwd, Cols: 100, Rows: 30, WindowKey: "w-main",
		Command: resume.Command, LaunchKind: terminal.LaunchKindAgent, AgentLaunchToken: resume.Token,
	}); err != nil {
		t.Fatalf("Terminal.Open: %v", err)
	}
	termFile := func(ext string) string {
		raw, _ := os.ReadFile(filepath.Join(app.FakeDir, "term-1"+ext))
		return string(raw)
	}
	testx.WaitUntil(t, wait, func() bool { return termFile(".args") != "" })
	if got := termFile(".mcp0"); !strings.Contains(got, "Bearer "+secret) {
		t.Fatalf("app mcp file = %q, want the resolved header", got)
	}
	if got := termFile(".appenv"); strings.TrimSpace(got) != "P453_MARK="+secret {
		t.Fatalf(".appenv = %q, want the resolved value", got)
	}
	testx.WaitUntil(t, wait, func() bool { return len(appSettingsFiles(app.KiraHome)) == 0 })
}
