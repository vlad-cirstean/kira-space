package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// The Review code controls read ahead and dirty from the board and preview the base the window
// diffs against: both must be real git facts.
func TestReviewOpenFacts(t *testing.T) {
	f, br := doneFixture(t, "feat/api-open")
	app := f.app
	refresh := func() adewire.Branch {
		t.Helper()
		if _, err := app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{CodeRepoIDs: []string{br.CodeRepoID}}); err != nil {
			t.Fatal(err)
		}
		return branchOf(t, board(t, app), f.taskID, f.repoID)
	}

	if err := os.WriteFile(filepath.Join(br.Worktree, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := refresh()
	if got.Ahead != 0 || len(got.Dirty) != 1 {
		t.Fatalf("uncommitted only: ahead %d, dirty %d, want 0 and 1", got.Ahead, len(got.Dirty))
	}
	// Contract ade-review-open: tests/ui/ade-v2-review-open.spec.ts "contract: ..." reads these.
	app.Contract(t, "ade-review-open", "AdeTaskService.Branch#uncommitted", got, flowharness.Mask("startedAt", "finishedAt", "tip", "createdAt", "addedAt", "lastCommitAt", "sha", "dirtyToken"))

	gitOut(t, br.Worktree, "add", "-A")
	gitOut(t, br.Worktree, "commit", "-q", "-m", "add wip")
	got = refresh()
	if got.Ahead != 1 || len(got.Dirty) != 0 {
		t.Fatalf("committed: ahead %d, dirty %d, want 1 and 0", got.Ahead, len(got.Dirty))
	}
	app.Contract(t, "ade-review-open", "AdeTaskService.Branch#committed", got, flowharness.Mask("startedAt", "finishedAt", "tip", "createdAt", "addedAt", "lastCommitAt", "sha", "dirtyToken"))

	app.Contract(t, "ade-review-open", "args:AdeTaskService.OpenReviewWindow", adewire.BranchArgs{BranchID: br.ID})
	if _, err := app.W.AdeTask.OpenReviewWindow(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	rec := app.WindowMgr.Opened[len(app.WindowMgr.Opened)-1]
	tgt, err := app.W.AdeTask.ReviewWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: rec.Key})
	if err != nil || tgt == nil {
		t.Fatalf("ReviewWindowTarget = %+v, %v", tgt, err)
	}
	if tgt.Base != got.Base && strings.TrimPrefix(tgt.Base, "origin/") != got.Base {
		t.Fatalf("window base %q, board base %q: the preview names another branch", tgt.Base, got.Base)
	}
}
