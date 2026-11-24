package adeflow_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// interrupted runs the task's step until the fake claude holds, then restarts the app.
func (f *runFixture) interrupted(t *testing.T, branch string) adewire.Run {
	t.Helper()
	f.start(t, branch)
	waitRun(t, f.app, f.taskID, "one", "running")
	testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(filepath.Join(f.app.FakeDir, "api-1.args")); return err == nil })
	f.app.Restart()
	return waitRun(t, f.app, f.taskID, "one", "stuck")
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

func TestResumeAfterRestart(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	f := newRunFixture(t, []fakeagent.Action{{WaitFile: hold}, {Name: "done"}}, agentStep("one", ""))
	stuck := f.interrupted(t, "feat/api-resume")
	if !stuck.Resumable || stuck.Outcome == nil || stuck.Outcome.Source != "restart" {
		t.Fatalf("run after restart = %+v, want resumable with a restart outcome", stuck)
	}
	first := fakeFile(t, f.app, "api-1.args")
	claudeID := argAfter(first, "--session-id")
	if claudeID == "" {
		t.Fatalf("call 1 args lack --session-id:\n%s", first)
	}
	f.app.Contract(t, "ade-resume", "AdeTaskService.Run#stuck", stuck, runMask)

	if err := f.app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuck.ID}); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	done := waitRun(t, f.app, f.taskID, "one", "done")
	if done.Attempt != 2 || done.ID == stuck.ID {
		t.Fatalf("resumed run = %+v, want attempt 2 as a new run", done)
	}
	f.app.Contract(t, "ade-resume", "AdeTaskService.Run#resumed", done, runMask)

	second := fakeFile(t, f.app, "api-2.args")
	if got := argAfter(second, "--resume"); got != claudeID {
		t.Fatalf("call 2 --resume = %q, want %q", got, claudeID)
	}
	if strings.Contains(second, "--session-id") {
		t.Fatalf("call 2 args carry --session-id:\n%s", second)
	}
	prompt := fakeFile(t, f.app, "api-2.prompt")
	if !strings.Contains(prompt, "Kira Space closed while you worked") || !strings.Contains(prompt, "step.finish") {
		t.Fatalf("call 2 prompt lacks the resume line or the finish instruction:\n%s", prompt)
	}

	old := taskOf(t, board(t, f.app), f.taskID).Runs
	for _, r := range old {
		if r.ID == stuck.ID && r.Resumable {
			t.Fatalf("old attempt still resumable: %+v", r)
		}
	}
	err := f.app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuck.ID})
	if err == nil {
		t.Fatal("ResumeRun accepted an attempt that was already resumed")
	}
	f.app.Contract(t, "ade-resume", "AdeTaskService.ResumeRun#refused", err.Error())
}

func TestResumeLostConversation(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	f := newRunFixture(t, []fakeagent.Action{{WaitFile: hold}, {Name: "lost"}, {Name: "done"}}, agentStep("one", ""))
	stuck := f.interrupted(t, "feat/api-resume-lost")
	if err := f.app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuck.ID}); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	var lost adewire.Run
	testx.WaitUntil(t, waitFor, func() bool {
		lost, _ = latestRun(t, f.app, f.taskID, "one")
		return lost.Attempt == 2 && (lost.State == "stuck" || lost.State == "failed")
	})
	o := lost.Outcome
	if lost.Resumable || o.Status != "failed" || o.Source != "start" || !strings.HasPrefix(o.Reason, "could not resume") {
		t.Fatalf("lost run = %+v, want a failed could-not-resume outcome that is not resumable", lost)
	}
	f.app.Contract(t, "ade-resume", "AdeTaskService.Run#lost", lost, runMask)

	if err := f.app.W.AdeTask.RetryRun(ctx, adewire.RunArgs{RunID: lost.ID}); err != nil {
		t.Fatalf("RetryRun: %v", err)
	}
	done := waitRun(t, f.app, f.taskID, "one", "done")
	if done.Attempt != 3 {
		t.Fatalf("retried run = %+v, want attempt 3", done)
	}
	third := fakeFile(t, f.app, "api-3.args")
	if !strings.Contains(third, "--session-id") || strings.Contains(third, "--resume") {
		t.Fatalf("retry args = %q, want a fresh --session-id and no --resume", third)
	}
}

func TestResumeRefused(t *testing.T) {
	refuse := func(t *testing.T, f *runFixture, r adewire.Run) {
		t.Helper()
		if r.Resumable {
			t.Fatalf("run = %+v, want not resumable", r)
		}
		if err := f.app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: r.ID}); err == nil {
			t.Fatal("ResumeRun accepted a run that is not resumable")
		}
	}

	t.Run("script step", func(t *testing.T) {
		f := newRunFixture(t, nil, scriptStep("check", "'exit 2'"))
		f.start(t, "feat/api-refuse-script")
		refuse(t, f, waitRun(t, f.app, f.taskID, "check", "stuck", "failed"))
	})

	t.Run("stopped by you", func(t *testing.T) {
		flag := filepath.Join(t.TempDir(), "release")
		f := newRunFixture(t, []fakeagent.Action{{Name: "done", WaitFile: flag}}, agentStep("one", ""))
		f.start(t, "feat/api-refuse-stop")
		running := waitRun(t, f.app, f.taskID, "one", "running")
		if err := f.app.W.AdeTask.StopRun(ctx, adewire.RunArgs{RunID: running.ID}); err != nil {
			t.Fatal(err)
		}
		refuse(t, f, waitRun(t, f.app, f.taskID, "one", "stuck", "failed"))
	})

	t.Run("needs input", func(t *testing.T) {
		f := newRunFixture(t, []fakeagent.Action{{Name: "needs_input"}}, agentStep("one", ""))
		f.start(t, "feat/api-refuse-input")
		refuse(t, f, waitRun(t, f.app, f.taskID, "one", "stuck", "failed"))
	})
}

// J-RUN-022. Three runs live at a restart: all three come back interrupted and resumable, none is
// silently done, nothing starts by itself; resuming one continues it alone and a second resume of the
// same attempt is refused.
func TestResumeOneOfThreeInterrupted(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{WaitFile: hold}, {WaitFile: hold}, {WaitFile: hold}, {Name: "done"}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	var tasks []adewire.Task
	for i := 0; i < 3; i++ {
		task := createTask(t, app, fmt.Sprintf("Task %d", i), "flow", rec.ID)
		br := branchOf(t, board(t, app), task.ID, rec.ID)
		if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: fmt.Sprintf("feat/api-par-%d", i)}}); err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
		waitRun(t, app, task.ID, "one", "running")
	}
	testx.WaitUntil(t, waitFor, func() bool {
		for i := 1; i <= 3; i++ {
			if _, err := os.Stat(filepath.Join(app.FakeDir, fmt.Sprintf("api-%d.args", i))); err != nil {
				return false
			}
		}
		return true
	})
	app.Restart()
	stuck := map[string]adewire.Run{}
	for _, task := range tasks {
		r := waitRun(t, app, task.ID, "one", "stuck")
		if !r.Resumable || r.Outcome == nil || r.Outcome.Source != "restart" {
			t.Fatalf("run after restart = %+v, want interrupted and resumable", r)
		}
		stuck[task.ID] = r
	}
	if err := app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuck[tasks[0].ID].ID}); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	waitRun(t, app, tasks[0].ID, "one", "done")
	if err := app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuck[tasks[0].ID].ID}); err == nil {
		t.Fatal("second ResumeRun of the same attempt accepted")
	}
	for _, task := range tasks[1:] {
		r, _ := latestRun(t, app, task.ID, "one")
		if r.State != "stuck" || r.ID != stuck[task.ID].ID || !r.Resumable {
			t.Fatalf("task %s run = %+v, want it still interrupted", task.ID, r)
		}
	}
	for _, task := range tasks {
		for _, r := range taskOf(t, board(t, app), task.ID).Runs {
			if r.State == "done" && task.ID != tasks[0].ID {
				t.Fatalf("task %s has a done run %+v after the restart", task.ID, r)
			}
		}
	}
}

// A resumed run is a new attempt: its step timeout arms at resume, not at the original start.
func TestResumeAfterRestartFreshTimeout(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	step := "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 2s\n    prompt: work on {branch} in {repo}\n"
	f := newRunFixture(t, []fakeagent.Action{{WaitFile: hold}, {Name: "done", WaitBefore: hold}}, step)
	f.start(t, "feat/api-fresh-timeout")
	waitRun(t, f.app, f.taskID, "one", "running")
	testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(filepath.Join(f.app.FakeDir, "api-1.args")); return err == nil })
	time.Sleep(1500 * time.Millisecond)
	f.app.Restart()
	stuck := waitRun(t, f.app, f.taskID, "one", "stuck")

	if err := f.app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuck.ID}); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	waitRun(t, f.app, f.taskID, "one", "running")
	time.Sleep(1500 * time.Millisecond)
	if r := waitRun(t, f.app, f.taskID, "one", "running"); r.Outcome != nil && r.Outcome.Source == "timeout" {
		t.Fatalf("resumed run = %+v, want still live", r)
	}
	if err := os.WriteFile(hold, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, f.app, f.taskID, "one", "done"); r.Attempt != 2 {
		t.Fatalf("resumed run = %+v, want done at attempt 2", r)
	}
}
