package reviewflow_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// mainFeature is main with a.txt (3 lines), then feature adding line4.
func mainFeature(app *flowharness.App) *flowharness.Repo {
	repo := app.NewRepo("proj")
	repo.Commit("base commit", map[string]string{"a.txt": "line1\nline2\nline3\n"})
	repo.Git("checkout", "-q", "-b", "feature")
	repo.Commit("feature commit", map[string]string{"a.txt": "line1\nline2\nline3\nline4\n"})
	return repo
}

// Guards partial marks: ranges shift with lines inserted above them.
func TestPartialRangesSurviveAnInsertion(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo := app.NewRepo("proj")
	var body strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&body, "line%d\n", i)
	}
	repo.Commit("base commit", map[string]string{"a.txt": body.String()})
	repo.Git("checkout", "-q", "-b", "feature")
	id := openRepo(t, gs, repo.Dir)

	markFile(t, gs, id, "feature", "a.txt", true, []gitreview.LineRange{lr(10, 20)})
	external(t, gs, func() {
		repo.Commit("insert 5 lines above", map[string]string{"a.txt": "i1\ni2\ni3\ni4\ni5\n" + body.String()})
	})

	if got := sinceReview(t, gs, id, "feature", "a.txt").ReviewedRanges; len(got) != 1 || got[0] != lr(15, 25) {
		t.Fatalf("ReviewedRanges = %v, want [15-25]", got)
	}
}

// Guards demotion: unmarking part of a fully reviewed file leaves the complement reviewed.
func TestUnmarkingPartOfAFullFileDemotesIt(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo := mainFeature(app)
	id := openRepo(t, gs, repo.Dir)

	if s := markFile(t, gs, id, "feature", "a.txt", true, nil); s.Kind != "full" {
		t.Fatalf("whole-file mark = %+v, want full", s)
	}
	if s := markFile(t, gs, id, "feature", "a.txt", false, []gitreview.LineRange{lr(2, 2)}); s.Kind != "partial" {
		t.Fatalf("after unmarking line 2 = %+v, want partial", s)
	}
	got := sinceReview(t, gs, id, "feature", "a.txt").ReviewedRanges
	if len(got) != 2 || got[0] != lr(1, 1) || got[1] != lr(3, 4) {
		t.Fatalf("ReviewedRanges = %v, want [1-1 3-4]", got)
	}
}

// Guards whole-file marks on files with no text snapshot: a binary file and a deleted file both
// mark full and unmark to none, and a ranged mark on binary is refused.
func TestMarkFilesWithoutLines(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo := mainFeature(app)
	repo.Commit("add binary", map[string]string{"img.bin": "\x89PNG\x00\x01"})
	repo.Git("rm", "-q", "a.txt")
	repo.Commit("delete a.txt", nil)
	id := openRepo(t, gs, repo.Dir)

	t.Run("binary", func(t *testing.T) {
		if s := markFile(t, gs, id, "feature", "img.bin", true, nil); s.Kind != "full" {
			t.Fatalf("binary mark = %+v, want full", s)
		}
		if e := fileEntry(t, reviewFiles(t, gs, id, "feature", "main"), "img.bin"); e.Review.Kind != "full" {
			t.Fatalf("review.files binary = %+v, want full", e.Review)
		}
		badRequest(t, gs, "review.mark", gitrpc.ReviewMarkParams{
			RepoID: id, Branch: "feature", Path: "img.bin", Reviewed: true, Ranges: []gitreview.LineRange{lr(1, 1)},
		}, "text")
	})
	t.Run("deleted", func(t *testing.T) {
		if s := markFile(t, gs, id, "feature", "a.txt", true, nil); s.Kind != "full" {
			t.Fatalf("deleted-file mark = %+v, want full", s)
		}
		if e := fileEntry(t, reviewFiles(t, gs, id, "feature", "main"), "a.txt"); e.Review.Kind != "full" {
			t.Fatalf("review.files deleted = %+v, want full", e.Review)
		}
		if s := markFile(t, gs, id, "feature", "a.txt", false, nil); s.Kind != "none" {
			t.Fatalf("deleted-file unmark = %+v, want none", s)
		}
	})
}

// Guards shared review state: a mark in one window shows in another.
func TestReviewStateSharedAcrossConnections(t *testing.T) {
	app := flowharness.New(t)
	a, b := app.OpenGitStream(), app.OpenGitStream()
	repo := mainFeature(app)
	idA, idB := openRepo(t, a, repo.Dir), openRepo(t, b, repo.Dir)

	markFile(t, a, idA, "feature", "a.txt", true, nil)
	if e := fileEntry(t, reviewFiles(t, b, idB, "feature", "main"), "a.txt"); e.Review.Kind != "full" {
		t.Fatalf("second window's a.txt = %+v, want full", e.Review)
	}
}

// Guards the review refusals: each E_BAD_REQUEST names the field.
func TestReviewRefusals(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo := mainFeature(app)
	id := openRepo(t, gs, repo.Dir)

	badRequest(t, gs, "review.fileDiff", gitrpc.ReviewFileDiffParams{RepoID: id, Branch: "feature", Base: "main", Path: "", Mode: "sinceReview"}, "path")
	badRequest(t, gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "-evil", Base: "main"}, "branch")
	badRequest(t, gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "no-such-branch", Base: "main"}, "branch")

	// Both elements are checked, not just the first.
	for _, ranges := range [][]gitreview.LineRange{{lr(-9, 0)}, {lr(1, 2), lr(0, 1)}} {
		badRequest(t, gs, "review.mark", gitrpc.ReviewMarkParams{
			RepoID: id, Branch: "feature", Path: "a.txt", Reviewed: true, Ranges: ranges,
		}, "range")
	}
}

// Guards the easy regression: an unrelated ref move fires refsChanged and drops no mark.
func TestRefsChangedKeepsReviewState(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo := mainFeature(app)
	id := openRepo(t, gs, repo.Dir)

	markFile(t, gs, id, "feature", "a.txt", true, nil)
	external(t, gs, func() { repo.Git("branch", "unrelated-branch") })

	if e := fileEntry(t, reviewFiles(t, gs, id, "feature", "main"), "a.txt"); e.Review.Kind != "full" {
		t.Fatalf("a.txt after an unrelated refsChanged = %+v, want still full", e.Review)
	}
	if d := sinceReview(t, gs, id, "feature", "a.txt"); d.DeltaSource != "unchanged" {
		t.Fatalf("DeltaSource = %q, want unchanged", d.DeltaSource)
	}
}

// Backend half of the review window's refresh: a commit on the reviewed branch reaches the stream
// as refsChanged, and review.files then answers with the new tip and file.
func TestReviewFilesAfterCommit(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo := mainFeature(app)
	id := openRepo(t, gs, repo.Dir)

	call[gitsession.RangeFilesResult](t, gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "feature", Base: "main"})
	var tip string
	external(t, gs, func() {
		tip = repo.Commit("second feature commit", map[string]string{"b.txt": "new\n"})
	})

	got := call[gitsession.RangeFilesResult](t, gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "feature", Base: "main"})
	if got.BranchTip != tip {
		t.Fatalf("branchTip = %s, want the new tip %s", got.BranchTip, tip)
	}
	fileEntry(t, got.Files, "b.txt")
}

// J-REV-001: reviewed marks follow the diff, not the commits. A rebase that leaves a file's change as it
// was keeps the mark; a file whose content changed keeps its mark but reads changed since review.
func TestReviewMarksAcrossRebase(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo, _ := twoFileFeature(app)
	repo.Commit("feature b", map[string]string{"b.txt": "alpha\nbeta\ngamma\ndelta\n"})
	id := openRepo(t, gs, repo.Dir)
	markFile(t, gs, id, "feature", "a.txt", true, nil)
	markFile(t, gs, id, "feature", "b.txt", true, nil)

	// main moves on in an unrelated file; feature rebases onto it with both changes intact.
	external(t, gs, func() {
		repo.Git("checkout", "-q", "main")
		repo.Commit("main moves", map[string]string{"c.txt": "unrelated\n"})
		repo.Git("checkout", "-q", "feature")
		repo.Git("rebase", "main")
	})
	files := reviewFiles(t, gs, id, "feature", "main")
	for _, p := range []string{"a.txt", "b.txt"} {
		if e := fileEntry(t, files, p); e.Review.Kind != "full" || e.Review.ChangedSinceReview {
			t.Fatalf("%s after a same-content rebase = %+v, want full and unchanged since review", p, e.Review)
		}
	}

	// A rewrite of b.txt's change flags it changed since review (pin: the mark stays, the delta shows);
	// a.txt stays clean.
	external(t, gs, func() {
		repo.Commit("rework b", map[string]string{"b.txt": "alpha\nbeta\ngamma\ndelta\nepsilon\n"})
	})
	files = reviewFiles(t, gs, id, "feature", "main")
	if e := fileEntry(t, files, "a.txt"); e.Review.Kind != "full" || e.Review.ChangedSinceReview {
		t.Fatalf("a.txt after an unrelated rework = %+v, want full and unchanged", e.Review)
	}
	if e := fileEntry(t, files, "b.txt"); !e.Review.ChangedSinceReview {
		t.Fatalf("b.txt after its content changed = %+v, want changed since review", e.Review)
	}
	if d := sinceReview(t, gs, id, "feature", "b.txt"); d.DeltaSource == "unchanged" {
		t.Fatalf("b.txt since-review delta source = %q after a change", d.DeltaSource)
	}
}
