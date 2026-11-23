package adeflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
)

func writeIn(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Backend half of the review window's uncommitted section: staged, unstaged, untracked and deleted
// files of the task worktree, read against the worktree's own HEAD.
func TestReviewWorking(t *testing.T) {
	f, br := doneFixture(t, "feat/api-working")
	app, wt := f.app, br.Worktree
	for _, name := range []string{"keep.txt", "staged.txt", "gone.txt"} {
		commitIn(t, wt, name)
	}
	head := gitOut(t, wt, "rev-parse", "HEAD")

	writeIn(t, wt, "keep.txt", "keep.txt\nunstaged edit\n")
	writeIn(t, wt, "staged.txt", "staged.txt\nstaged edit\n")
	gitOut(t, wt, "add", "staged.txt")
	writeIn(t, wt, "fresh.txt", "fresh\n")
	if err := os.Remove(filepath.Join(wt, "gone.txt")); err != nil {
		t.Fatal(err)
	}

	app.Contract(t, "ade-review-working", "args:AdeTaskService.ReviewWorking", adewire.BranchArgs{BranchID: br.ID})
	got, err := app.W.AdeTask.ReviewWorking(ctx, adewire.BranchArgs{BranchID: br.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Worktree != wt || got.Head != head {
		t.Fatalf("worktree %q head %q, want %q %q", got.Worktree, got.Head, wt, head)
	}
	kinds := map[string]porcelain.FileChangeKind{}
	for _, c := range got.Files {
		kinds[c.Path] = c.Kind
	}
	want := map[string]porcelain.FileChangeKind{
		"keep.txt": porcelain.FileModified, "staged.txt": porcelain.FileModified,
		"fresh.txt": porcelain.FileAdded, "gone.txt": porcelain.FileDeleted,
	}
	if len(got.Files) != len(want) {
		t.Fatalf("files = %+v, want %v", got.Files, want)
	}
	for p, k := range want {
		if kinds[p] != k {
			t.Fatalf("%s kind = %q, want %q (files %+v)", p, kinds[p], k, got.Files)
		}
	}
	app.Contract(t, "ade-review-working", "AdeTaskService.ReviewWorking", got, flowharness.Mask("head"), flowharness.Replace(wt, "<worktree>"))

	app.Contract(t, "ade-review-working", "args:AdeTaskService.ReviewWorkingDiff", adewire.ReviewWorkingDiffArgs{BranchID: br.ID, Path: "keep.txt"})
	d, err := app.W.AdeTask.ReviewWorkingDiff(ctx, adewire.ReviewWorkingDiffArgs{BranchID: br.ID, Path: "keep.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Head.Kind != "found" || d.Head.Text != "keep.txt\n" || d.Worktree.Text != "keep.txt\nunstaged edit\n" {
		t.Fatalf("keep.txt diff = %+v", d)
	}
	app.Contract(t, "ade-review-working", "AdeTaskService.ReviewWorkingDiff", d, flowharness.Mask("oid"))
	if d, err = app.W.AdeTask.ReviewWorkingDiff(ctx, adewire.ReviewWorkingDiffArgs{BranchID: br.ID, Path: "gone.txt"}); err != nil || d.Worktree.Kind != "missing" || d.Head.Kind != "found" {
		t.Fatalf("gone.txt diff = %+v, %v, want a missing worktree side", d, err)
	}
	app.Contract(t, "ade-review-working", "AdeTaskService.ReviewWorkingDiff#deleted", d, flowharness.Mask("oid"))
	if d, err = app.W.AdeTask.ReviewWorkingDiff(ctx, adewire.ReviewWorkingDiffArgs{BranchID: br.ID, Path: "fresh.txt"}); err != nil || d.Head.Kind != "missing" || d.Worktree.Text != "fresh\n" {
		t.Fatalf("fresh.txt diff = %+v, %v, want a missing HEAD side", d, err)
	}
	if _, err := app.W.AdeTask.ReviewWorkingDiff(ctx, adewire.ReviewWorkingDiffArgs{BranchID: br.ID, Path: "../escape"}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("../escape = %v, want E_BAD_REQUEST", err)
	}

	gitOut(t, wt, "add", "-A")
	gitOut(t, wt, "commit", "-q", "-m", "commit the work")
	if got, err = app.W.AdeTask.ReviewWorking(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil || len(got.Files) != 0 {
		t.Fatalf("after commit = %+v, %v, want no files", got, err)
	}
}

// A branch that is not created yet has nothing to review; it is refused, never read as empty.
func TestReviewWorkingRefusesUncreatedBranch(t *testing.T) {
	f := newRunFixture(t, nil, agentStep("one", ""))
	br := branchOf(t, board(t, f.app), f.taskID, f.repoID)
	if _, err := f.app.W.AdeTask.ReviewWorking(ctx, adewire.BranchArgs{BranchID: br.ID}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("uncreated branch = %v, want E_BAD_REQUEST", err)
	}
}
