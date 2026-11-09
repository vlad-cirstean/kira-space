package adeflow_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

const reviewSyncContract = "ade-review-sync"

func reviewFilePaths(res gitsession.RangeFilesResult) []string {
	paths := make([]string, 0, len(res.Files))
	for _, e := range res.Files {
		paths = append(paths, e.Change.Path)
	}
	slices.Sort(paths)
	return paths
}

// Backend half of ade-review-sync: a review whose branch and base trail their remotes reports it,
// Fetch updates the remote spellings, Update fast-forwards only what is strictly behind. Frontend
// half: tests/ui/ade-v2-review-sync.spec.ts reads the ade-review-sync contract.
func TestReviewSync(t *testing.T) {
	f, br := doneFixture(t, "feat/api-sync")
	app, wt := f.app, br.Worktree
	maskSync := []flowharness.ContractOption{flowharness.Mask("lastFetchAt"), flowharness.Replace(wt, "<worktree>")}

	commitIn(t, wt, "mine.txt")
	gitOut(t, wt, "push", "-q", "-u", "origin", "feat/api-sync")
	mainBase := gitOut(t, wt, "rev-parse", "origin/main")

	mate := f.bare.Clone(filepath.Join(app.Work, "mate"))
	mate.Git("checkout", "-q", "-b", "feat/api-sync", "origin/feat/api-sync")
	mate.Commit("teammate", map[string]string{"teammate.txt": "t\n"})
	mate.Git("push", "-q", "origin", "feat/api-sync")
	mate.Checkout("main")
	mate.Commit("main moved", map[string]string{"main-new.txt": "m\n"})
	mate.Git("push", "-q", "origin", "main")

	if _, err := app.W.AdeTask.OpenReviewWindow(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	rec := app.WindowMgr.Opened[len(app.WindowMgr.Opened)-1]
	tgt, err := app.W.AdeTask.ReviewWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: rec.Key})
	if err != nil || tgt == nil {
		t.Fatalf("ReviewWindowTarget = %+v, %v", tgt, err)
	}
	gs := app.OpenGitStream()
	var opened gitrpc.RepoOpenResult
	gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: filepath.Join(app.Work, "api")}, &opened)
	filesParams := gitrpc.ReviewFilesParams{RepoID: tgt.GitRepoID, Branch: tgt.Branch, Base: tgt.Base}
	listFiles := func() gitsession.RangeFilesResult {
		t.Helper()
		var res gitsession.RangeFilesResult
		gs.MustRequest("review.files", filesParams, &res)
		return res
	}
	args := adewire.BranchArgs{BranchID: br.ID}
	app.Contract(t, reviewSyncContract, "args:AdeTaskService.ReviewSync", args)

	before, err := app.W.AdeTask.ReviewSync(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if before.Remote != "origin" || before.Base.State != "remote" || before.Base.Name != "origin/main" ||
		before.Branch.State != "current" || before.Branch.Worktree != wt {
		t.Fatalf("sync before fetch = %+v, want a remote base and a current branch checked out in %s", before, wt)
	}
	app.Contract(t, reviewSyncContract, "AdeTaskService.ReviewSync#before", before, maskSync...)
	if got := reviewFilePaths(listFiles()); !slices.Equal(got, []string{"mine.txt"}) {
		t.Fatalf("files before fetch = %v, want mine.txt", got)
	}

	if res, err := app.W.AdeTask.ReviewFetch(ctx, args); err != nil || res.Error != nil {
		t.Fatalf("ReviewFetch = %+v, %v", res, err)
	}
	afterFetch, err := app.W.AdeTask.ReviewSync(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if afterFetch.LastFetchAt == nil || afterFetch.Branch.State != "behind" || afterFetch.Branch.Behind != 1 || afterFetch.Branch.Ahead != 0 {
		t.Fatalf("sync after fetch = %+v, want the branch 1 behind", afterFetch)
	}
	app.Contract(t, reviewSyncContract, "AdeTaskService.ReviewSync#afterFetch", afterFetch, maskSync...)
	// Three dot: the moved base changes the base tip, not the merge base or the list.
	res := listFiles()
	if res.MergeBase != mainBase || !slices.Equal(reviewFilePaths(res), []string{"mine.txt"}) {
		t.Fatalf("after fetch merge base %s files %v, want %s and mine.txt", res.MergeBase, reviewFilePaths(res), mainBase)
	}

	updateArgs := adewire.ReviewUpdateLocalArgs{BranchID: br.ID, Side: "branch"}
	app.Contract(t, reviewSyncContract, "args:AdeTaskService.ReviewUpdateLocal", updateArgs)
	afterUpdate, err := app.W.AdeTask.ReviewUpdateLocal(ctx, updateArgs)
	if err != nil {
		t.Fatal(err)
	}
	if afterUpdate.Branch.State != "current" || gitOut(t, wt, "rev-parse", "HEAD") != gitOut(t, wt, "rev-parse", "origin/feat/api-sync") {
		t.Fatalf("sync after update = %+v, want the worktree at origin/feat/api-sync", afterUpdate)
	}
	app.Contract(t, reviewSyncContract, "AdeTaskService.ReviewUpdateLocal#afterUpdate", afterUpdate, maskSync...)
	if got := reviewFilePaths(listFiles()); !slices.Equal(got, []string{"mine.txt", "teammate.txt"}) {
		t.Fatalf("files after update = %v, want mine.txt and teammate.txt", got)
	}

	// A remote spelling is never written.
	if _, err := app.W.AdeTask.ReviewUpdateLocal(ctx, adewire.ReviewUpdateLocalArgs{BranchID: br.ID, Side: "base"}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("update of a remote base = %v, want E_BAD_REQUEST", err)
	}
	if _, err := app.W.AdeTask.ReviewUpdateLocal(ctx, adewire.ReviewUpdateLocalArgs{BranchID: br.ID, Side: "other"}); err == nil {
		t.Fatal("update accepted an unknown side")
	}

	// Diverged: a local and a remote commit. Neither ref moves.
	commitIn(t, wt, "local.txt")
	mate.Checkout("feat/api-sync")
	mate.Commit("teammate again", map[string]string{"teammate2.txt": "t2\n"})
	mate.Git("push", "-q", "origin", "feat/api-sync")
	if res, err := app.W.AdeTask.ReviewFetch(ctx, args); err != nil || res.Error != nil {
		t.Fatalf("ReviewFetch = %+v, %v", res, err)
	}
	diverged, err := app.W.AdeTask.ReviewSync(ctx, args)
	if err != nil || diverged.Branch.State != "diverged" || diverged.Branch.Ahead != 1 || diverged.Branch.Behind != 1 {
		t.Fatalf("sync = %+v, %v, want a diverged branch", diverged, err)
	}
	app.Contract(t, reviewSyncContract, "AdeTaskService.ReviewSync#diverged", diverged, maskSync...)
	local, remote := gitOut(t, wt, "rev-parse", "HEAD"), gitOut(t, wt, "rev-parse", "origin/feat/api-sync")
	if _, err := app.W.AdeTask.ReviewUpdateLocal(ctx, updateArgs); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("update of a diverged branch = %v, want E_BAD_REQUEST", err)
	}
	if gitOut(t, wt, "rev-parse", "HEAD") != local || gitOut(t, wt, "rev-parse", "origin/feat/api-sync") != remote {
		t.Fatal("a refused update moved a ref")
	}

	// An unreachable remote reports its error and changes nothing.
	f.repo.Git("remote", "set-url", "origin", "file://"+filepath.Join(app.Work, "no-such-remote.git"))
	failed, err := app.W.AdeTask.ReviewFetch(ctx, args)
	if err != nil || failed.Error == nil {
		t.Fatalf("fetch from a missing remote = %+v, %v, want an error", failed, err)
	}
	app.Contract(t, reviewSyncContract, "AdeTaskService.ReviewFetch#fetchError", failed, maskSync...)
}

// A stacked branch reviews against its local parent; Update moves that parent whether or not a
// worktree has it checked out.
func TestReviewSyncStackedBase(t *testing.T) {
	f := newBaseFx(t)
	parentTask, parent := f.task(t, "Parent", adewire.BaseChoice{})
	childTask, child := f.task(t, "Child", adewire.BaseChoice{BranchID: parent.ID})
	if err := f.startRun(t, parentTask, parent, "feat/api-parent"); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, parentTask.ID, "one", "done")
	parentWt := branchOf(t, board(t, f.app), parentTask.ID, f.repoID).Worktree
	commitIn(t, parentWt, "parent.txt")
	gitOut(t, parentWt, "push", "-q", "-u", "origin", "feat/api-parent")
	if err := f.startRun(t, childTask, child, "feat/api-child"); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, childTask.ID, "one", "done")
	child = branchOf(t, board(t, f.app), childTask.ID, f.repoID)

	mate := f.bare.Clone(filepath.Join(f.app.Work, "mate"))
	pushParent := func(name string) {
		t.Helper()
		mate.Git("fetch", "-q", "origin")
		mate.Git("checkout", "-q", "-B", "feat/api-parent", "origin/feat/api-parent")
		mate.Commit(name, map[string]string{name + ".txt": name + "\n"})
		mate.Git("push", "-q", "origin", "feat/api-parent")
	}
	args := adewire.BranchArgs{BranchID: child.ID}
	update := adewire.ReviewUpdateLocalArgs{BranchID: child.ID, Side: "base"}
	fetch := func() adewire.ReviewSync {
		t.Helper()
		if res, err := f.app.W.AdeTask.ReviewFetch(ctx, args); err != nil || res.Error != nil {
			t.Fatalf("ReviewFetch = %+v, %v", res, err)
		}
		got, err := f.app.W.AdeTask.ReviewSync(ctx, args)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	pushParent("one")
	if got := fetch(); got.Base.Name != "feat/api-parent" || got.Base.State != "behind" || got.Base.Worktree != parentWt {
		t.Fatalf("base after fetch = %+v, want the local parent 1 behind, checked out in %s", got.Base, parentWt)
	}
	got, err := f.app.W.AdeTask.ReviewUpdateLocal(ctx, update)
	if err != nil || got.Base.State != "current" {
		t.Fatalf("update of a checked-out parent = %+v, %v", got, err)
	}
	if _, err := f.repo.GitErr("cat-file", "-e", "origin/feat/api-parent:one.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.GitErr("cat-file", "-e", "feat/api-parent:one.txt"); err != nil {
		t.Fatalf("local parent lacks one.txt after the update: %v", err)
	}

	pushParent("two")
	gitOut(t, parentWt, "checkout", "-q", "--detach")
	if got := fetch(); got.Base.State != "behind" || got.Base.Worktree != "" {
		t.Fatalf("base after the second fetch = %+v, want behind and checked out nowhere", got.Base)
	}
	if got, err := f.app.W.AdeTask.ReviewUpdateLocal(ctx, update); err != nil || got.Base.State != "current" {
		t.Fatalf("update of a parent checked out nowhere = %+v, %v", got, err)
	}
	if want, got := f.repo.Git("rev-parse", "origin/feat/api-parent"), f.repo.Git("rev-parse", "feat/api-parent"); got != want {
		t.Fatalf("local parent at %s, want origin's %s", got, want)
	}
}
