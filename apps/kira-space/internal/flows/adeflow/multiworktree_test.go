package adeflow_test

import (
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// Several branches of one repo on a task. Contract ade-multi-worktree: tests/ui/ade-v2-multi-worktree.spec.ts reads it.
func TestMultiWorktree(t *testing.T) {
	app := flowharness.New(t)
	repoA, _ := cloneWithMain(app, filepath.Join(app.Work, "a"))
	a := importRepo(t, app, repoA.Dir)
	gitOut(t, repoA.Dir, "branch", "feat/one")
	gitOut(t, repoA.Dir, "branch", "feat/two")
	task := createTask(t, app, "Two branches", "")

	for _, name := range []string{"feat/one", "feat/two"} {
		if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: a.ID, Name: name, TaskID: task.ID}); err != nil {
			t.Fatalf("AddExistingBranch %s: %v", name, err)
		}
	}
	rows := branchesOf(t, app, task.ID)
	if len(rows) != 2 || rows[0].Name == rows[1].Name || rows[0].Worktree == rows[1].Worktree {
		t.Fatalf("rows = %+v, want two branches with two worktrees", rows)
	}
	for _, r := range rows {
		if !nanoFolder.MatchString(filepath.Base(r.Worktree)) || filepath.Base(filepath.Dir(r.Worktree)) != "a" {
			t.Fatalf("worktree %q, want wt/a/<wt_id>", r.Worktree)
		}
	}
	mask := flowharness.Mask("id", "taskId", "codeRepoId", "worktree", "addedAt", "tip", "base", "setup")
	app.Contract(t, "ade-multi-worktree", "AdeTaskBoard.Board.branches", rows, mask)

	// One waiting slot per repo: a third row is a draft, a fourth is refused.
	if _, err := app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: task.ID, CodeRepoID: a.ID}); err != nil {
		t.Fatalf("AddTaskRepo draft: %v", err)
	}
	if n := len(branchesOf(t, app, task.ID)); n != 3 {
		t.Fatalf("rows = %d, want 3 with the draft", n)
	}
	if _, err := app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: task.ID, CodeRepoID: a.ID}); errCode(err) != "E_CONFLICT" {
		t.Fatalf("second draft = %v, want E_CONFLICT", err)
	}

	// Removing a named row leaves the other rows alone and adds no draft.
	if err := app.W.AdeTask.RemoveTaskBranch(ctx, adewire.RemoveTaskRowArgs{BranchID: rows[0].ID}); err != nil {
		t.Fatalf("RemoveTaskBranch: %v", err)
	}
	gone(t, rows[0].Worktree)
	if left := branchesOf(t, app, task.ID); len(left) != 2 {
		t.Fatalf("rows = %+v, want the other branch and the draft", left)
	}
}
