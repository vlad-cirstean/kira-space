package adeflow_test

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// Backend half of re-review on edit: a reviewed file edited afterwards reports changedSinceReview
// (uncommitted: content hash, committed: blob oid), and the stat stamp of the listed files moves
// with the edit. Frontend half: tests/ui/ade-v2-review-rereview.spec.ts reads the ade-review-rereview contract.
func TestReviewRereviewOnEdit(t *testing.T) {
	f, br := doneFixture(t, "feat/api-rereview")
	app, wt := f.app, br.Worktree
	writeIn(t, wt, "base.txt", "a\nb\n")
	gitOut(t, wt, "add", "base.txt")
	gitOut(t, wt, "commit", "-q", "-m", "add base.txt")
	writeIn(t, wt, "work.txt", "one\n")

	if _, err := app.W.AdeTask.OpenReviewWindow(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	rec := app.WindowMgr.Opened[len(app.WindowMgr.Opened)-1]
	tgt, err := app.W.AdeTask.ReviewWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: rec.Key})
	if err != nil || tgt == nil {
		t.Fatalf("ReviewWindowTarget = %+v, %v", tgt, err)
	}

	const contract = "ade-review-rereview"
	branchArgs := adewire.BranchArgs{BranchID: br.ID}
	maskWorking := []flowharness.ContractOption{
		flowharness.Mask("head", "reviewedAt", "reviewedAtSha"), flowharness.Replace(wt, "<worktree>"),
	}
	listed, err := app.W.AdeTask.ReviewWorking(ctx, branchArgs)
	if err != nil || listed.ContentHashes["work.txt"] == "" {
		t.Fatalf("ReviewWorking = %+v, %v, want a hash for work.txt", listed, err)
	}
	markArgs := adewire.ReviewWorkingMarkArgs{
		BranchID: br.ID, Paths: []string{"work.txt"}, Reviewed: true,
		ContentHashes: map[string]string{"work.txt": listed.ContentHashes["work.txt"]},
	}
	if _, err := app.W.AdeTask.ReviewWorkingMark(ctx, markArgs); err != nil {
		t.Fatal(err)
	}
	reviewed, err := app.W.AdeTask.ReviewWorking(ctx, branchArgs)
	if got := reviewed.Reviews["work.txt"]; err != nil || got.Kind != "full" || got.ChangedSinceReview {
		t.Fatalf("after mark = %+v, %v, want full and unchanged", got, err)
	}
	app.Contract(t, contract, "AdeTaskService.ReviewWorking#reviewed", reviewed, maskWorking...)

	stampArgs := adewire.ReviewWorkingStampArgs{BranchID: br.ID, Paths: []string{"base.txt", "work.txt"}}
	app.Contract(t, contract, "args:AdeTaskService.ReviewWorkingStamp", stampArgs)
	before, err := app.W.AdeTask.ReviewWorkingStamp(ctx, stampArgs)
	if err != nil || before.Stamp == "" {
		t.Fatalf("stamp = %+v, %v, want a digest", before, err)
	}
	if again, err := app.W.AdeTask.ReviewWorkingStamp(ctx, stampArgs); err != nil || again != before {
		t.Fatalf("stamp of untouched files = %+v, %v, want %+v", again, err, before)
	}
	app.Contract(t, contract, "AdeTaskService.ReviewWorkingStamp#before", before, flowharness.Mask("stamp"))

	// A different size, so mtime granularity cannot hide the edit.
	writeIn(t, wt, "work.txt", "one\ntwo\n")
	after, err := app.W.AdeTask.ReviewWorkingStamp(ctx, stampArgs)
	if err != nil || after.Stamp == before.Stamp {
		t.Fatalf("stamp after edit = %+v, %v, want a new digest", after, err)
	}
	app.Contract(t, contract, "AdeTaskService.ReviewWorkingStamp#after", after, flowharness.Mask("stamp"))
	changed, err := app.W.AdeTask.ReviewWorking(ctx, branchArgs)
	if got := changed.Reviews["work.txt"]; err != nil || got.Kind != "full" || !got.ChangedSinceReview {
		t.Fatalf("after edit = %+v, %v, want full and changedSinceReview", got, err)
	}
	app.Contract(t, contract, "AdeTaskService.ReviewWorking#changed", changed, maskWorking...)

	markArgs.ContentHashes = map[string]string{"work.txt": changed.ContentHashes["work.txt"]}
	if _, err := app.W.AdeTask.ReviewWorkingMark(ctx, markArgs); err != nil {
		t.Fatal(err)
	}
	if again, err := app.W.AdeTask.ReviewWorking(ctx, branchArgs); err != nil || again.Reviews["work.txt"].ChangedSinceReview {
		t.Fatalf("after re-mark = %+v, %v, want unchanged", again.Reviews["work.txt"], err)
	}

	if _, err := app.W.AdeTask.ReviewWorkingStamp(ctx, adewire.ReviewWorkingStampArgs{BranchID: br.ID, Paths: []string{"../x"}}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("stamp of ../x = %v, want E_BAD_REQUEST", err)
	}
	tooMany := slices.Repeat([]string{"base.txt"}, 5001)
	if _, err := app.W.AdeTask.ReviewWorkingStamp(ctx, adewire.ReviewWorkingStampArgs{BranchID: br.ID, Paths: tooMany}); errCode(err) != "E_CONTRACT" {
		t.Fatalf("stamp of 5001 paths = %v, want E_CONTRACT", err)
	}

	gs := app.OpenGitStream()
	var opened gitrpc.RepoOpenResult
	gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: filepath.Join(app.Work, "api")}, &opened)
	filesParams := gitrpc.ReviewFilesParams{RepoID: tgt.GitRepoID, Branch: tgt.Branch, Base: tgt.Base}
	var mark gitrpc.ReviewMarkResult
	gs.MustRequest("review.mark", gitrpc.ReviewMarkParams{RepoID: tgt.GitRepoID, Branch: tgt.Branch, Path: "base.txt", Reviewed: true}, &mark)
	if mark.Review.Kind != "full" || mark.Review.ChangedSinceReview {
		t.Fatalf("review.mark = %+v, want full and unchanged", mark.Review)
	}
	fileStatus := func(res gitsession.RangeFilesResult) gitsession.ReviewFileStatus {
		for _, e := range res.Files {
			if e.Change.Path == "base.txt" {
				return e.Review
			}
		}
		t.Fatalf("review.files = %+v, want base.txt", res.Files)
		return gitsession.ReviewFileStatus{}
	}
	var files gitsession.RangeFilesResult
	gs.MustRequest("review.files", filesParams, &files)
	if got := fileStatus(files); got.Kind != "full" || got.ChangedSinceReview {
		t.Fatalf("review.files base.txt = %+v, want full and unchanged", got)
	}

	writeIn(t, wt, "base.txt", "a\nb\nc\n")
	gitOut(t, wt, "add", "base.txt")
	gitOut(t, wt, "commit", "-q", "-m", "edit base.txt")
	deadline := time.Now().Add(10 * time.Second)
	for {
		gs.MustRequest("review.files", filesParams, &files)
		if fileStatus(files).ChangedSinceReview || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond) // the committed side reads the cached ref snapshot
	}
	if got := fileStatus(files); got.Kind != "full" || !got.ChangedSinceReview {
		t.Fatalf("review.files after commit = %+v, want full and changedSinceReview", got)
	}
	app.Contract(t, contract, "review.files#changed", files, flowharness.Mask("branchTip", "mergeBase", "reviewedAt", "reviewedAtSha"))
}
