package adeflow_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// P469 stage move sessions. Contract ade-stage-move: tests/ui/ade-v2-stage-move.spec.ts reads these.

const smScenario = "ade-stage-move"

var smMask = flowharness.Mask("id", "taskId", "branchId", "runId", "sessionId", "cwd", "agentSessionId", "startedAt",
	"endedAt", "lastActiveAt", "finishedAt", "createdAt", "updatedAt", "lastActivityAt", "terminalId")

func smSteps() string {
	step := func(id, extra string) string {
		return "  - id: " + id + "\n    name: " + id + "\n    kind: manual\n" + extra
	}
	return step("spec", "    session: true\n") + step("build", "    session: true\n") + step("ship", "")
}

func TestStageMoveSessions(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	names := []string{"s1", "s2", "branch", "s3", "bare"}
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Minute))
	holds := make([]fakeagent.Action, len(names))
	for i, n := range names {
		holds[i] = fakeagent.Action{WaitFile: flag(n)}
	}
	claude(app, map[string][]fakeagent.Action{"*": holds})
	t.Cleanup(func() {
		for _, n := range names {
			_ = os.WriteFile(flag(n), nil, 0o644)
		}
	})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", smSteps()))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	launch := func(taskID string) string {
		l, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: taskID})
		if err != nil {
			t.Fatalf("LaunchStep: %v", err)
		}
		openLaunch(t, app, l)
		waitSession(t, app, l.SessionID, "running")
		return l.SessionID
	}
	state := func(id string) string { return sessionByID(t, app, id).State }
	stepDone := func(taskID string, closeSessions bool) adewire.Task {
		task, err := app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: taskID, CloseSessions: closeSessions})
		if err != nil {
			t.Fatalf("StepDone: %v", err)
		}
		return task
	}

	task := createTask(t, app, "Fix login", "flow", rec.ID)
	s1 := launch(task.ID)
	if got := stepDone(task.ID, false); got.EntryStepID != "build" || state(s1) != "running" {
		t.Fatalf("keep: entry %q, s1 %q, want build and running", got.EntryStepID, state(s1))
	}

	s2 := launch(task.ID)
	sessions, err := app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, smScenario, "AdeTaskService.Sessions#stage-open", sessions, smMask)
	if got := stepDone(task.ID, true); got.EntryStepID != "ship" || state(s1) != "stopped" || state(s2) != "stopped" {
		t.Fatalf("close: entry %q, s1 %q, s2 %q, want ship and both stopped", got.EntryStepID, state(s1), state(s2))
	}
	app.Contract(t, smScenario, "AdeTaskService.StepDone#closed", taskOf(t, board(t, app), task.ID), smMask)

	// A branch session is no stage session; a move that leaves a session-less step closes nothing.
	row := branchOf(t, board(t, app), task.ID, rec.ID)
	br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: "feat/api-login"})
	if err != nil {
		t.Fatal(err)
	}
	adhoc, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, adhoc)
	waitSession(t, app, adhoc.SessionID, "running")
	if _, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "build", FromStepID: "ship", CloseSessions: true}); err != nil {
		t.Fatal(err)
	}
	if state(adhoc.SessionID) != "running" {
		t.Fatalf("branch session = %q after a close-sessions move, want running", state(adhoc.SessionID))
	}
	s3 := launch(task.ID)
	moved, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "done", FromStepID: "build", CloseSessions: true})
	if err != nil || moved.EntryStepID != "done" {
		t.Fatalf("SetTaskStep = %+v, %v, want entry done", moved, err)
	}
	if state(s3) != "stopped" || state(adhoc.SessionID) != "running" {
		t.Fatalf("s3 %q, branch %q, want stopped and running", state(s3), state(adhoc.SessionID))
	}

	// Repo-less task: its step session runs in the task folder and closes the same way.
	bare := createTask(t, app, "Plan trip", "flow")
	sb := launch(bare.ID)
	if got := stepDone(bare.ID, true); got.EntryStepID != "build" || state(sb) != "stopped" {
		t.Fatalf("repo-less: entry %q, session %q, want build and stopped", got.EntryStepID, state(sb))
	}
}
