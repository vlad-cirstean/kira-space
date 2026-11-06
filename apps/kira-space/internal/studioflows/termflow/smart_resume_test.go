package termflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// resumeCall returns the args and stdin of the fake claude call that carried --resume.
func resumeCall(t *testing.T, app *flowharness.App) (args, prompt string) {
	t.Helper()
	testx.WaitUntil(t, wait, func() bool {
		files, _ := filepath.Glob(filepath.Join(app.FakeDir, "*.args"))
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err == nil && strings.Contains(string(raw), "--resume") {
				args = string(raw)
				p, _ := os.ReadFile(strings.TrimSuffix(f, ".args") + ".prompt")
				prompt = string(p)
				return true
			}
		}
		return false
	})
	return args, prompt
}

func argAfter(args, flag string) string {
	lines := strings.Split(args, "\n")
	for i, l := range lines {
		if l == flag && i+1 < len(lines) {
			return lines[i+1]
		}
	}
	return ""
}

func TestSmartScriptResume(t *testing.T) {
	runMask := flowharness.Mask("createdAt", "startedAt", "finishedAt", "hash", "costUsd")

	t.Run("quit", func(t *testing.T) {
		app := flowharness.New(t)
		hold := filepath.Join(t.TempDir(), "hold")
		app.Scenario(scenario(fakeagent.Action{WaitFile: hold}, fakeagent.Action{Name: "done"}))
		rec := newSmart(t, app, "Say hi", nil)
		id, _ := startSmart(t, app, scriptruns.RunArgs{ScriptID: rec.ID})
		testx.WaitUntil(t, wait, func() bool {
			files, _ := filepath.Glob(filepath.Join(app.FakeDir, "*.args"))
			return len(files) > 0
		})
		running := waitSmart(t, app, id, "running")

		app.Restart(t)
		interrupted := waitSmart(t, app, id, "failed")
		if !interrupted.Resumable || interrupted.Outcome == nil || interrupted.Outcome.Source != "restart" || interrupted.SessionID != running.SessionID {
			t.Fatalf("run after quit = %+v, want resumable with a restart outcome and the same session", interrupted)
		}
		app.Contract(t, "smart-resume", "ScriptRunsService.Get#interrupted", interrupted, runMask)

		started, err := app.W.ScriptRuns.Resume(scriptruns.IDArgs{ID: id})
		if err != nil {
			t.Fatalf("Resume: %v", err)
		}
		resumed := waitSmart(t, app, started.RunID, "done")
		if resumed.ID == id || resumed.ResumedFrom != id || resumed.SessionID != running.SessionID || resumed.Resumable {
			t.Fatalf("resumed run = %+v, want a new run from %s on session %s", resumed, id, running.SessionID)
		}
		app.Contract(t, "smart-resume", "ScriptRunsService.Get#resumed", resumed, runMask)
		args, _ := resumeCall(t, app)
		if argAfter(args, "--resume") != running.SessionID || strings.Contains(args, "--session-id") {
			t.Fatalf("resume args = %q, want --resume %s and no --session-id", args, running.SessionID)
		}
		if !strings.HasPrefix(resumed.Prompt, "Kira ") || !strings.Contains(resumed.Prompt, "closed while you worked") {
			t.Fatalf("resumed prompt = %q, want the resume line first", resumed.Prompt)
		}
		if _, err := app.W.ScriptRuns.Resume(scriptruns.IDArgs{ID: id}); errCode(err) != "E_BAD_REQUEST" {
			t.Fatalf("second Resume = %v, want E_BAD_REQUEST", err)
		}
	})

	t.Run("crash", func(t *testing.T) {
		app := flowharness.New(t)
		app.Scenario(scenario(fakeagent.Action{Name: "done"}))
		rec := newSmart(t, app, "Say hi", nil)
		id, _ := startSmart(t, app, scriptruns.RunArgs{ScriptID: rec.ID})
		done := waitSmart(t, app, id, "done")
		// A crashed process leaves the row running; Recover ends it on the next boot.
		if _, err := app.DB().Exec(`UPDATE script_runs SET state = 'running', outcome_json = '', finished_at = NULL, resume_id = '' WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
		app.Restart(t)
		interrupted := waitSmart(t, app, id, "failed")
		if !interrupted.Resumable || interrupted.SessionID != done.SessionID {
			t.Fatalf("run after crash = %+v, want resumable on session %s", interrupted, done.SessionID)
		}
		started, err := app.W.ScriptRuns.Resume(scriptruns.IDArgs{ID: id})
		if err != nil {
			t.Fatalf("Resume: %v", err)
		}
		if resumed := waitSmart(t, app, started.RunID, "done"); resumed.ResumedFrom != id {
			t.Fatalf("resumed run = %+v, want resumedFrom %s", resumed, id)
		}
	})

	t.Run("secret param", func(t *testing.T) {
		app := flowharness.New(t)
		hold := filepath.Join(t.TempDir(), "hold")
		app.Scenario(scenario(fakeagent.Action{WaitFile: hold}))
		rec := newSmart(t, app, "Say hi", func(f *scripts.CustomScriptFields) {
			f.Params = []scripts.Param{{Name: "token", Type: scripts.ParamText, Secret: true}}
		})
		id, _ := startSmart(t, app, scriptruns.RunArgs{ScriptID: rec.ID, Params: map[string][]string{"token": {"s3cret"}}})
		waitSmart(t, app, id, "running")
		app.Restart(t)
		interrupted := waitSmart(t, app, id, "failed")
		if !interrupted.Resumable {
			t.Fatalf("run = %+v, want resumable", interrupted)
		}
		_, err := app.W.ScriptRuns.Resume(scriptruns.IDArgs{ID: id})
		if errCode(err) != "E_BAD_REQUEST" || !strings.Contains(err.Error(), "token") {
			t.Fatalf("Resume = %v, want E_BAD_REQUEST naming token", err)
		}
		app.Contract(t, "smart-resume", "ScriptRunsService.Resume#missing", err.Error())
	})
}
