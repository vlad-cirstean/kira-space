package reviewflow_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// A file the branch deleted lists as deleted, reads found at the merge base and missing at the
// branch tip: the sides the review window shows as the old text alone.
func TestDeletedFileReads(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo := mainFeature(app)
	repo.Git("rm", "-q", "a.txt")
	repo.Commit("delete a.txt", nil)
	id := openRepo(t, gs, repo.Dir)

	files := call[gitsession.RangeFilesResult](t, gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "feature", Base: "main"})
	if e := fileEntry(t, files.Files, "a.txt"); e.Change.Kind != "deleted" {
		t.Fatalf("a.txt change = %+v, want deleted", e.Change)
	}
	left := call[gitsession.BlobResult](t, gs, "file.read", gitrpc.FileReadParams{RepoID: id, Rev: files.MergeBase, Path: "a.txt"})
	right := call[gitsession.BlobResult](t, gs, "file.read", gitrpc.FileReadParams{RepoID: id, Rev: files.BranchTip, Path: "a.txt"})
	if left.Kind != "found" || left.Content != "line1\nline2\nline3\n" || right.Kind != "missing" {
		t.Fatalf("file.read left = %+v, right = %+v; want the old text found, then missing", left, right)
	}
	app.Contract(t, "git-review-deleted", "git:review.files", files, flowharness.Mask("branchTip", "mergeBase"))
	app.Contract(t, "git-review-deleted", "git:file.read#left", left)
	app.Contract(t, "git-review-deleted", "git:file.read#right", right)
}
