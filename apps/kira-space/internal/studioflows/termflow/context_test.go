package termflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
)

// TestAgentContextStudio: Studio reads the agent context Kira Space hosts (KIRA_SPACE_HOME/agent):
// a smart script gets it, and Continue in terminal carries the flag.
func TestAgentContextStudio(t *testing.T) {
	const marker = "STUDIO-CONTEXT-MARKER\n"
	app := studioflowharness.New(t)
	app.Scenario(scenario(fakeagent.Action{Name: "done"}))
	dir := filepath.Join(app.SpaceHome, "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "context.md"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := newSmart(t, app, "Say hi", nil)
	runID, _ := startSmart(t, app, scriptruns.RunArgs{ScriptID: rec.ID})
	run := waitSmart(t, app, runID, "done")
	waitCall(t, app, 1)
	if args := callFile(t, app, 1, ".args"); !strings.Contains(args, "--append-system-prompt-file") {
		t.Fatalf("smart script argv lacks the context flag:\n%s", args)
	}
	if got := callFile(t, app, 1, ".context"); got != marker {
		t.Fatalf("smart script context = %q, want %q", got, marker)
	}

	resume, err := app.W.ScriptRuns.ResumeCommand(scriptruns.ResumeCommandArgs{RunID: run.ID})
	if err != nil || !strings.Contains(resume.Command, "'--append-system-prompt-file' '") {
		t.Fatalf("ResumeCommand = %+v, %v, want the context flag", resume, err)
	}
}
