package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

// settingsSnapshot maps each user-owned Claude Code settings file to its mode and digest.
// ~/.claude.json is left out on purpose: the CLI itself rewrites it on every run (startup counts,
// project trust, cached account data; P233 A13), so it cannot be compared byte for byte.
func settingsSnapshot(t *testing.T, files []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range files {
		info, err := os.Stat(p)
		if err != nil {
			out[p] = "missing"
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		out[p] = fmt.Sprintf("%v sha256 %s", info.Mode().Perm(), hex.EncodeToString(sum[:8]))
	}
	return out
}

// TestRealClaudeSettingsUntouched is the real-CLI twin of claudeflow.TestClaudeSettingsUntouched:
// no Kira launch, hook injection or restart may rewrite the user's own settings files.
func TestRealClaudeSettingsUntouched(t *testing.T) {
	f := newAdeFixture(t, nil)
	saveAgentSettings(t, f) // the agent settings file is present throughout
	files := []string{
		filepath.Join(f.Home, ".claude", "settings.json"),
		filepath.Join(f.repo.Dir, ".claude", "settings.json"),
		filepath.Join(f.repo.Dir, ".claude", "settings.local.json"),
	}
	writeFile(t, files[0], `{"permissions":{"allow":["Bash(git status)"]}}`+"\n")
	writeFile(t, files[1], `{"permissions":{"deny":["Read(.env)"]}}`+"\n")
	writeFile(t, files[2], `{"enableAllProjectMcpServers":true}`+"\n")
	before := settingsSnapshot(t, files)
	check := func(t *testing.T, flow string) {
		t.Helper()
		var d []string
		for p, v := range settingsSnapshot(t, files) {
			if before[p] != v {
				d = append(d, fmt.Sprintf("%s: %s to %s", p, before[p], v))
			}
		}
		sort.Strings(d)
		if len(d) > 0 {
			t.Fatalf("%s changed Claude Code settings: %s", flow, strings.Join(d, "; "))
		}
	}
	f.seedClaudeJSON(t, f.repo.Dir)

	t.Run("terminal agent session", func(t *testing.T) {
		const id = "settings-agent"
		mark := f.Events.Mark()
		f.openAgent(t, id, f.repo.Dir, "claude --model haiku")
		f.waitAgentEvent(t, mark, id, "SessionStart", 60*time.Second)
		time.Sleep(2 * time.Second)
		f.typeLine(t, id, "Reply with the single word pong.")
		f.waitAgentEvent(t, mark, id, "Stop", 90*time.Second)
		_ = f.W.Terminal.Close(terminal.CloseArgs{TerminalID: id})
		time.Sleep(time.Second)
		check(t, "terminal agent session")
	})

	f.saveWorkflow(t, "flow", agentStepYAML+manualStepYAML)
	task := f.createTask(t, "flow")

	t.Run("ade headless run", func(t *testing.T) {
		if run := f.startRun(t, task, "feat/settings"); run.State != "done" {
			t.Fatalf("run = %+v, want done", run)
		}
		check(t, "ade headless run")
	})

	t.Run("ade manual step", func(t *testing.T) {
		launch, err := f.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
		if err != nil {
			t.Fatal(err)
		}
		f.seedClaudeJSON(t, launch.Cwd)
		mark := f.Events.Mark()
		if _, err := f.W.Terminal.Open(terminal.OpenArgs{
			TerminalID: launch.TerminalID, WindowKey: window, Cwd: launch.Cwd, Cols: 120, Rows: 40,
			Command: launch.Command, LaunchKind: terminal.LaunchKindAgent,
		}); err != nil {
			t.Fatal(err)
		}
		f.waitAgentEvent(t, mark, launch.TerminalID, "SessionStart", 60*time.Second)
		_ = f.W.Terminal.Close(terminal.CloseArgs{TerminalID: launch.TerminalID})
		time.Sleep(time.Second)
		check(t, "ade manual step")
	})

	t.Run("smart script run", func(t *testing.T) {
		if run := smartRun(t, f.realApp, "Reply with the word ok.", []string{"Read"}); run.State != "done" {
			t.Fatalf("run = %s, want done", run.State)
		}
		check(t, "smart script run")
	})

	t.Run("ade smart step", func(t *testing.T) {
		if _, err := f.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
			Name: "settings-lint", Kind: scripts.KindSmart, Color: "blue", UseAdeDir: true,
			Command: "Call finish_step with status done and summary ok.",
			Smart:   &scripts.Smart{Model: "haiku", MaxBudgetUSD: 0.05, Timeout: "3m", Tools: []string{"Read"}},
		}}); err != nil {
			t.Fatal(err)
		}
		f.saveWorkflow(t, "smartflow", "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 2m\n    smart_script: settings-lint\n"+manualStepYAML)
		if r := f.startRun(t, f.createTask(t, "smartflow"), "feat/settings-smart"); r.State != "done" {
			t.Fatalf("smart step run = %s, want done", r.State)
		}
		check(t, "ade smart step")
	})

	f.Restart()
	check(t, "app restart")
}
