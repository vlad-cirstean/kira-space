package claude

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// agentSettingsProbe is the settings of TestAgentSettingsClaude: a secret reaches a hook, the stdio
// MCP server and the agent env, each writing it to a file the test reads.
type agentSettingsProbe struct {
	secret, hookOut, mcpOut string
}

// saveAgentSettings saves the probe settings through the editor's bound service and sets the secret.
func saveAgentSettings(t *testing.T, f *adeFixture) agentSettingsProbe {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p := agentSettingsProbe{secret: "mark-" + hex.EncodeToString(b), hookOut: filepath.Join(dir, "hook.out"), mcpOut: filepath.Join(dir, "mcp.out")}
	ref := "$" + "{secret:mark}"
	doc := map[string]any{
		"env": map[string]string{"P453_MARK": ref},
		"mcp_servers": map[string]any{"probe": map[string]any{
			"command": "sh",
			"args":    []string{"-c", `printf '%s' "$P453_MARK" > ` + p.mcpOut + "; sleep 5"},
			"env":     map[string]string{"P453_MARK": ref},
		}},
		"hooks": map[string]any{"mark": map[string]any{
			"event":   "session_start",
			"command": `printf '%s' "$P453_MARK" > ` + p.hookOut,
		}},
	}
	text, err := json.Marshal(doc) // JSON is YAML
	if err != nil {
		t.Fatal(err)
	}
	svc := f.W.AgentSettingsSvc
	cur, err := svc.Get()
	if err != nil {
		t.Fatal(err)
	}
	if v, err := svc.Save(agentcfg.SaveArgs{Text: string(text) + "\n", BaseHash: cur.Hash}); err != nil || v.Problem != nil {
		t.Fatalf("Save = %+v, %v", v, err)
	}
	if _, err := svc.SetSecret(agentcfg.SetSecretArgs{Name: "mark", Value: p.secret}); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	return p
}

func (p agentSettingsProbe) waitMarked(t *testing.T, path string) {
	t.Helper()
	testx.WaitUntil(t, 90*time.Second, func() bool {
		raw, _ := os.ReadFile(path)
		return string(raw) == p.secret
	})
}

func requireNoRunFiles(t *testing.T, root string) {
	t.Helper()
	testx.WaitUntil(t, 30*time.Second, func() bool {
		found := false
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.Contains(filepath.Base(p), "-app.") && strings.HasSuffix(p, ".json") {
				found = true
			}
			return nil
		})
		return !found
	})
}

// TestAgentSettingsClaude checks the real claude applies the agent settings: a headless ADE step and a
// terminal session both run the user hook and the stdio server with the resolved secret, Kira's own
// hooks keep working beside the user's, and no run file stays behind.
func TestAgentSettingsClaude(t *testing.T) {
	f := newAdeFixture(t, nil)
	p := saveAgentSettings(t, f)
	f.seedClaudeJSON(t, f.repo.Dir)
	f.saveWorkflow(t, "flow", agentStepYAML+manualStepYAML)
	task := f.createTask(t, "flow")

	t.Run("headless step", func(t *testing.T) {
		if run := f.startRun(t, task, "feat/agent-settings"); run.State != "done" {
			t.Fatalf("run = %+v, want done", run)
		}
		p.waitMarked(t, p.hookOut)
		p.waitMarked(t, p.mcpOut)
		requireNoRunFiles(t, f.SpaceHome)
	})

	t.Run("terminal session", func(t *testing.T) {
		for _, out := range []string{p.hookOut, p.mcpOut} {
			if err := os.Remove(out); err != nil {
				t.Fatal(err)
			}
		}
		launch, err := f.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
		if err != nil {
			t.Fatal(err)
		}
		f.seedClaudeJSON(t, launch.Cwd)
		mark := f.Events.Mark()
		f.openAgent(t, launch.TerminalID, launch.Cwd, launch.Command)
		// Kira's own SessionStart reaches the app beside the user's hook.
		f.waitAgentEvent(t, mark, launch.TerminalID, "SessionStart", 60*time.Second)
		p.waitMarked(t, p.hookOut)
		p.waitMarked(t, p.mcpOut)
		if err := f.W.Terminal.Close(terminal.CloseArgs{TerminalID: launch.TerminalID}); err != nil {
			t.Fatal(err)
		}
		requireNoRunFiles(t, f.SpaceHome)
	})
}
