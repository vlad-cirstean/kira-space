package adeflow_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
)

// Backend half of reviewing a branch with only uncommitted changes. Frontend half:
// tests/ui/ade-v2-review-uncommitted.spec.ts reads the ade-review-uncommitted contract.
func TestReviewUncommittedOnly(t *testing.T) {
	f, br := doneFixture(t, "feat/api-uncommitted")
	app, wt := f.app, br.Worktree
	const contract = "ade-review-uncommitted"
	branchMask := flowharness.Mask("startedAt", "finishedAt", "tip", "createdAt", "addedAt", "lastCommitAt", "sha")
	refresh := func() adewire.Branch {
		t.Helper()
		if _, err := app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{CodeRepoIDs: []string{br.CodeRepoID}}); err != nil {
			t.Fatal(err)
		}
		return branchOf(t, board(t, app), f.taskID, f.repoID)
	}

	writeIn(t, wt, "wip.txt", "wip\n")
	got := refresh()
	if got.Ahead != 0 || len(got.Dirty) != 1 {
		t.Fatalf("uncommitted only: ahead %d, dirty %d, want 0 and 1", got.Ahead, len(got.Dirty))
	}
	app.Contract(t, contract, "AdeTaskService.Branch#uncommittedOnly", got, branchMask)

	app.Contract(t, contract, "args:AdeTaskService.OpenReviewWindow", adewire.BranchArgs{BranchID: br.ID})
	if _, err := app.W.AdeTask.OpenReviewWindow(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil {
		t.Fatalf("OpenReviewWindow on uncommitted-only branch: %v", err)
	}
	rec := app.WindowMgr.Opened[len(app.WindowMgr.Opened)-1]
	tgt, err := app.W.AdeTask.ReviewWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: rec.Key})
	if err != nil || tgt == nil {
		t.Fatalf("ReviewWindowTarget = %+v, %v", tgt, err)
	}

	working, err := app.W.AdeTask.ReviewWorking(ctx, adewire.BranchArgs{BranchID: br.ID})
	if err != nil || len(working.Files) != 1 || working.Files[0].Path != "wip.txt" || working.Head == "" {
		t.Fatalf("ReviewWorking = %+v, %v, want wip.txt", working, err)
	}
	app.Contract(t, contract, "AdeTaskService.ReviewWorking#uncommittedOnly", working,
		flowharness.Mask("head", "reviewedAt", "reviewedAtSha"), flowharness.Replace(wt, "<worktree>"))

	gs := app.OpenGitStream()
	var opened gitrpc.RepoOpenResult
	gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: filepath.Join(app.Work, "api")}, &opened)
	resolveParams := gitrpc.ReviewResolveBaseParams{RepoID: tgt.GitRepoID, Branch: tgt.Branch, Base: &tgt.Base}
	var res gitreview.BaseResolution
	gs.MustRequest("review.resolveBase", resolveParams, &res)
	if res.Range.Kind != "empty" {
		t.Fatalf("range = %+v, want empty", res.Range)
	}
	app.Contract(t, contract, "git:review.resolveBase#uncommittedOnly", res, flowharness.Mask("base", "baseSha", "headSha", "mergeBase"))

	// Mark the file, commit it, leave more edits: the branch is mixed and the mark carries over.
	markArgs := adewire.ReviewWorkingMarkArgs{
		BranchID: br.ID, Paths: []string{"wip.txt"}, Reviewed: true,
		ContentHashes: map[string]string{"wip.txt": working.ContentHashes["wip.txt"]},
	}
	if _, err := app.W.AdeTask.ReviewWorkingMark(ctx, markArgs); err != nil {
		t.Fatal(err)
	}
	gitOut(t, wt, "add", "-A")
	gitOut(t, wt, "commit", "-q", "-m", "add wip")
	writeIn(t, wt, "more.txt", "more\n")
	got = refresh()
	if got.Ahead != 1 || len(got.Dirty) != 1 {
		t.Fatalf("mixed: ahead %d, dirty %d, want 1 and 1", got.Ahead, len(got.Dirty))
	}
	app.Contract(t, contract, "AdeTaskService.Branch#mixed", got, branchMask)

	gs.MustRequest("review.resolveBase", resolveParams, &res)
	if res.Range.Kind != "ready" || res.Range.CommitCount == nil || *res.Range.CommitCount != 1 {
		t.Fatalf("range = %+v, want ready with 1 commit", res.Range)
	}
	working, err = app.W.AdeTask.ReviewWorking(ctx, adewire.BranchArgs{BranchID: br.ID})
	if err != nil || len(working.Files) != 1 || working.Files[0].Path != "more.txt" || !reflect.DeepEqual(working.Promoted, []string{"wip.txt"}) {
		t.Fatalf("ReviewWorking after commit = %+v, %v, want more.txt listed and wip.txt promoted", working, err)
	}
}
