package claude

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// holdScript is a shell script that blocks until flag exists; the agent runs it as one Bash call, so
// a restart cuts the turn mid tool call.
func holdScript(t *testing.T, flag string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hold.sh")
	body := "#!/bin/sh\nwhile [ ! -e " + flag + " ]; do sleep 1; done\necho released\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func holdPrompt(script string) string {
	return "Run exactly one Bash command: sh " + script + " . It is safe to run again: if an earlier call was cut off, run it again and inspect nothing first. Wait for it to finish, then call state with action step.finish and args status done."
}

// startHeld starts the ADE run and returns once the agent has issued its Bash call.
func (f *adeFixture) startHeld(t *testing.T, script string) (adewire.Task, adewire.Run) {
	t.Helper()
	step := "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 5m\n" +
		"    allowed_tools: [\"Bash(sh:*)\"]\n" +
		"    prompt: " + strconv.Quote(holdPrompt(script)) + "\n"
	f.saveWorkflow(t, "flow", step+manualStepYAML)
	task := f.createTask(t, "flow")
	b, err := f.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var branchID string
	for _, br := range b.Branches {
		if br.TaskID == task.ID && br.CodeRepoID == f.repoID {
			branchID = br.ID
		}
	}
	if _, err := f.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{branchID: "feat/real-resume"}}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	var run adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		run = f.stepRun(t, task.ID)
		return run.ID != "" && strings.Contains(f.runLog(t, run.ID), "Bash")
	})
	return task, run
}

// claudeSession returns Claude's session id of an ADE session row.
func (f *adeFixture) claudeSession(t *testing.T, sessionID string) string {
	t.Helper()
	res, err := f.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Sessions {
		if s.ID == sessionID {
			return s.AgentSessionID
		}
	}
	t.Fatalf("no session %s in %+v", sessionID, res.Sessions)
	return ""
}

func (f *adeFixture) stepRun(t *testing.T, taskID string) adewire.Run {
	t.Helper()
	b, err := f.W.AdeTask.Board(ctx)
	if err != nil {
		return adewire.Run{}
	}
	var best adewire.Run
	for _, tk := range b.Tasks {
		if tk.ID != taskID {
			continue
		}
		for _, r := range tk.Runs {
			if r.StepID == "one" && r.Attempt >= best.Attempt {
				best = r
			}
		}
	}
	return best
}

// TestAdeResumeRun cuts a real agent off mid Bash call with an app restart, resumes it, releases the
// call and expects the same Claude session to finish through state step.finish. The CLI's interrupted tool_result
// line is not in the run log, so the check is the shared Claude session id.
func TestAdeResumeRun(t *testing.T) {
	f := newAdeFixture(t, nil)
	flag := filepath.Join(t.TempDir(), "flag")
	task, run := f.startHeld(t, holdScript(t, flag))

	f.Restart()
	var stuck adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		stuck = f.stepRun(t, task.ID)
		return stuck.ID == run.ID && stuck.State == "stuck" && stuck.Resumable
	})
	if err := f.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuck.ID}); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var resumed adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		resumed = f.stepRun(t, task.ID)
		return resumed.Attempt == 2 && resumed.State != "running"
	})
	log := f.runLog(t, resumed.ID)
	if resumed.State != "done" || resumed.Outcome == nil || !resumed.Outcome.Reported {
		t.Fatalf("resumed run = %+v, want done through state step.finish; log:\n%s", resumed, tail(log, 1500))
	}
	if got, want := f.claudeSession(t, resumed.SessionID), f.claudeSession(t, run.SessionID); got == "" || got != want {
		t.Fatalf("resumed run on claude session %q, want the interrupted run's %q", got, want)
	}
	f.requireCostUnder(t)
}

// removeTranscript deletes Claude's saved conversation for a session id under the test HOME.
func removeTranscript(t *testing.T, home, sessionID string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no transcript for session %s under %s (%v)", sessionID, home, err)
	}
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAdeResumeLost resumes after the transcript is gone: the CLI refuses without an API call, so the
// run fails with the could-not-resume reason and no cost.
func TestAdeResumeLost(t *testing.T) {
	f := newAdeFixture(t, nil)
	flag := filepath.Join(t.TempDir(), "flag")
	task, run := f.startHeld(t, holdScript(t, flag))

	f.Restart()
	var stuck adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		stuck = f.stepRun(t, task.ID)
		return stuck.ID == run.ID && stuck.State == "stuck" && stuck.Resumable
	})
	removeTranscript(t, f.Home, f.claudeSession(t, run.SessionID))
	if err := f.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuck.ID}); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	var lost adewire.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		lost = f.stepRun(t, task.ID)
		return lost.Attempt == 2 && lost.State != "running"
	})
	o := lost.Outcome
	if lost.State == "done" || o == nil || o.Source != "start" || !strings.HasPrefix(o.Reason, "could not resume") {
		t.Fatalf("lost run = %+v, want a could-not-resume failure; log:\n%s", lost, tail(f.runLog(t, lost.ID), 1500))
	}
	if o.CostUSD != nil && *o.CostUSD > 0 {
		t.Fatalf("lost run cost %.4f USD, want none", *o.CostUSD)
	}
}

// TestSmartScriptResume does the same for a smart script run.
func TestSmartScriptResume(t *testing.T) {
	app := newRealApp(t)
	flag := filepath.Join(t.TempDir(), "flag")
	script := holdScript(t, flag)
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "real", Kind: scripts.KindSmart, Color: "blue",
		Command: "Run exactly one Bash command: sh " + script + " . It is safe to run again: if an earlier call was cut off, run it again and inspect nothing first. Wait for it to finish, then reply with the word ok.",
		Smart:   &scripts.Smart{Model: "haiku", MaxBudgetUSD: 0.2, Timeout: "5m", Tools: []string{"Bash"}, BashPatterns: []string{"sh:*"}},
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
	testx.WaitUntil(t, testTimeout, func() bool {
		page, err := app.W.ScriptRuns.ReadLog(scriptruns.ReadLogArgs{ID: started.RunID})
		if err != nil {
			return false
		}
		for _, c := range page.Chunks {
			if strings.Contains(c.Text, "Bash") {
				return true
			}
		}
		return false
	})

	app.Restart()
	var stuck scriptruns.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		stuck, _ = app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
		return stuck.Resumable
	})
	resumedStart, err := app.W.ScriptRuns.Resume(scriptruns.IDArgs{ID: started.RunID})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := os.WriteFile(flag, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var resumed scriptruns.Run
	testx.WaitUntil(t, testTimeout, func() bool {
		resumed, _ = app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: resumedStart.RunID})
		return resumed.State != "running"
	})
	o := resumed.Outcome
	if resumed.State != "done" || o == nil || !o.Reported || resumed.ResumedFrom != started.RunID || resumed.SessionID != stuck.SessionID {
		t.Fatalf("resumed run = %+v (outcome %+v), want done from %s on session %s", resumed, o, started.RunID, stuck.SessionID)
	}
	app.requireCostUnder(t)
}
