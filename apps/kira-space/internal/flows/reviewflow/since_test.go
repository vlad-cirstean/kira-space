package reviewflow_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// A file marked reviewed, then changed: the since-review view reads the reviewed text on the left,
// the branch tip on the right, and its diff holds only the new change (P741). The UI colours the
// two sides like any other diff; frontend half is ade-v2-review-since.spec.ts, same contract.
func TestSinceReviewSides(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo := mainFeature(app)
	id := openRepo(t, gs, repo.Dir)

	const oldText = "line1\nline2\nline3\nline4\n"
	const newText = "line1\nLINE2\nline3\nline4\nline5\n"
	if s := markFile(t, gs, id, "feature", "a.txt", true, nil); s.Kind != "full" {
		t.Fatalf("whole-file mark = %+v, want full", s)
	}
	external(t, gs, func() {
		repo.Commit("change line2, add line5", map[string]string{"a.txt": newText})
	})

	files := call[gitsession.RangeFilesResult](t, gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "feature", Base: "main"})
	e := fileEntry(t, files.Files, "a.txt")
	if !e.Review.ChangedSinceReview || e.Review.ReviewedAtSHA == nil {
		t.Fatalf("a.txt review = %+v, want changedSinceReview with a reviewedAtSha", e.Review)
	}
	left := call[gitsession.BlobResult](t, gs, "file.read", gitrpc.FileReadParams{RepoID: id, Rev: *e.Review.ReviewedAtSHA, Path: "a.txt"})
	right := call[gitsession.BlobResult](t, gs, "file.read", gitrpc.FileReadParams{RepoID: id, Rev: files.BranchTip, Path: "a.txt"})
	if left.Content != oldText || right.Content != newText {
		t.Fatalf("file.read left = %q, right = %q; want the reviewed text, then the tip", left.Content, right.Content)
	}
	snap := call[gitsession.ReviewSnapshotResult](t, gs, "review.snapshot", gitrpc.ReviewSnapshotParams{RepoID: id, Branch: "feature", Path: "a.txt"})
	if snap.Kind != "text" || snap.Text == nil || *snap.Text != oldText || snap.ReviewedAtSHA == nil || *snap.ReviewedAtSHA != *e.Review.ReviewedAtSHA {
		t.Fatalf("snapshot = %+v, want the reviewed text at %s", snap, *e.Review.ReviewedAtSHA)
	}

	d := sinceReview(t, gs, id, "feature", "a.txt")
	var changed []string
	for _, h := range d.Body.Hunks {
		for _, l := range h.Lines {
			switch l.Kind {
			case porcelain.LineAdd:
				changed = append(changed, "+"+l.Text)
			case porcelain.LineDel:
				changed = append(changed, "-"+l.Text)
			}
		}
	}
	if want := []string{"-line2", "+LINE2", "+line5"}; len(changed) != len(want) || changed[0] != want[0] || changed[1] != want[1] || changed[2] != want[2] {
		t.Fatalf("since-review changes = %v, want %v", changed, want)
	}
	if got := d.ReviewedRanges; len(got) != 2 || got[0] != (gitreview.LineRange{Start: 1, End: 1}) || got[1] != (gitreview.LineRange{Start: 3, End: 4}) || d.LineCount != 5 {
		t.Fatalf("reviewedRanges = %v, lineCount = %d; want [1-1 3-4] of 5", got, d.LineCount)
	}

	sha := flowharness.Mask("branchTip", "mergeBase", "reviewedAtSha", "reviewedAt")
	app.Contract(t, "git-review-since", "git:review.files", files, sha)
	app.Contract(t, "git-review-since", "git:file.read#left", left)
	app.Contract(t, "git-review-since", "git:file.read#right", right)
	app.Contract(t, "git-review-since", "git:review.snapshot", snap, sha)
	app.Contract(t, "git-review-since", "git:review.fileDiff#sinceReview", d, sha)
}
