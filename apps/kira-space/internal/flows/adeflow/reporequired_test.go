package adeflow_test

import (
	"errors"
	"os"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// P598 starting work on a task with no repo is refused until a repo is chosen. Contract
// ade-repo-required: tests/ui/ade-v2-repo-required.spec.ts reads these.

const rrScenario = "ade-repo-required"

// noRepoErr asserts err is the E_TASK_NO_REPO refusal and returns it as stored in the contract.
func noRepoErr(t *testing.T, err error) map[string]string {
	t.Helper()
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != "E_TASK_NO_REPO" {
		t.Fatalf("error = %v, want E_TASK_NO_REPO", err)
	}
	return map[string]string{"code": ie.Code, "message": ie.Message}
}

func TestTaskStartNeedsRepo(t *testing.T) {
	const outsideStep = "  - id: one\n    name: one\n    kind: agent\n    outside_repo: true\n    timeout: 1m\n    prompt: survey the folder\n"
	const sessionStep = "  - id: work\n    name: Work\n    kind: manual\n    session: true\n"

	t.Run("run refuses a task with no repo", func(t *testing.T) {
		f := newNRFixture(t, nil, agentStep("one", ""))
		_, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID})
		f.app.Contract(t, rrScenario, "AdeTaskService.StartRun#noRepo", noRepoErr(t, err))
		if _, ok := latestRun(t, f.app, f.task.ID, "one"); ok {
			t.Fatal("a run exists after the refusal")
		}
		if _, err := os.Stat(f.dir); !os.IsNotExist(err) {
			t.Fatalf("task folder: %v, want none", err)
		}
	})

	t.Run("run after choosing a repo", func(t *testing.T) {
		f := newNRFixture(t, map[string][]fakeagent.Action{"*": {{Name: "done"}}}, agentStep("one", ""))
		row, err := f.app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: f.task.ID, CodeRepoID: f.api})
		if err != nil {
			t.Fatalf("AddTaskRepo: %v", err)
		}
		f.app.Contract(t, rrScenario, "AdeTaskService.AddTaskRepo", row, nrMask)
		br := branchOf(t, board(t, f.app), f.task.ID, f.api)
		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID, BranchNames: map[string]string{br.ID: "feat/api-work"}}); err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		one := waitRun(t, f.app, f.task.ID, "one", "done")
		if one.BranchID != br.ID {
			t.Fatalf("run branch = %q, want %q", one.BranchID, br.ID)
		}
		if s := sessionByID(t, f.app, one.SessionID); s.Cwd == f.dir {
			t.Fatalf("session cwd = %q, want a repo worktree", s.Cwd)
		}
		if _, err := os.Stat(f.dir); !os.IsNotExist(err) {
			t.Fatalf("task folder: %v, want none", err)
		}
		f.app.Contract(t, rrScenario, "AdeTaskService.Board#started", taskOf(t, board(t, f.app), f.task.ID), nrMask)
	})

	t.Run("session step refuses, then opens in the repo", func(t *testing.T) {
		f := newNRFixture(t, nil, sessionStep)
		f.app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
		_, err := f.app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.task.ID})
		f.app.Contract(t, rrScenario, "AdeTaskService.LaunchStep#noRepo", noRepoErr(t, err))
		if _, err := f.app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: f.task.ID, CodeRepoID: f.api}); err != nil {
			t.Fatalf("AddTaskRepo: %v", err)
		}
		launch, err := f.app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.task.ID})
		if err != nil {
			t.Fatalf("LaunchStep: %v", err)
		}
		if launch.Cwd == f.dir || launch.Cwd == "" {
			t.Fatalf("launch cwd = %q, want the repo", launch.Cwd)
		}
		f.app.Contract(t, rrScenario, "AdeTaskService.LaunchStep", launch, nrMask)
		if _, err := os.Stat(f.dir); !os.IsNotExist(err) {
			t.Fatalf("task folder: %v, want none", err)
		}
	})

	t.Run("outside-repo step still runs in the task folder", func(t *testing.T) {
		f := newNRFixture(t, map[string][]fakeagent.Action{"*": {{Name: "done"}}}, outsideStep)
		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID}); err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		one := waitRun(t, f.app, f.task.ID, "one", "done")
		if s := sessionByID(t, f.app, one.SessionID); one.BranchID != "" || s.Cwd != f.dir {
			t.Fatalf("run branch %q cwd %q, want none and %q", one.BranchID, s.Cwd, f.dir)
		}
	})

	t.Run("auto start into a repo step waits for a repo", func(t *testing.T) {
		f := newNRFixture(t, map[string][]fakeagent.Action{"*": {{Name: "done"}}}, outsideStep+agentStep("two", ""))
		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID}); err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		if _, ok := latestRun(t, f.app, f.task.ID, "two"); ok {
			t.Fatal("step two started without a repo")
		}
		f.app.Contract(t, rrScenario, "AdeTaskService.Board#waitingRepo", taskOf(t, board(t, f.app), f.task.ID), nrMask)
		_, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID})
		noRepoErr(t, err)
		if _, err := f.app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: f.task.ID, CodeRepoID: f.api}); err != nil {
			t.Fatalf("AddTaskRepo: %v", err)
		}
		br := branchOf(t, board(t, f.app), f.task.ID, f.api)
		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID, BranchNames: map[string]string{br.ID: "feat/api-two"}}); err != nil {
			t.Fatalf("StartRun after repo: %v", err)
		}
		if two := waitRun(t, f.app, f.task.ID, "two", "done"); two.BranchID != br.ID {
			t.Fatalf("step two branch = %q, want %q", two.BranchID, br.ID)
		}
	})
}

