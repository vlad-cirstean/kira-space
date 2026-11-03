package adeflow_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Backend half of tests/ui/capture.spec.ts: a pasted PR captured to Today becomes a task on that
// day with the PR head branch fetched and attached for review.
func TestCaptureChangeRequestForReview(t *testing.T) {
	f := newHostLinkFixture(t, false)
	app := f.app
	day := "2026-03-02"

	capture := func(link string) (string, bridge.CaptureItem) {
		app.Clipboard.SetText(link)
		app.Shortcuts.Fire(t, model.DefaultCaptureHotkey)
		opened := app.WindowMgr.OpenedPopups
		key := opened[len(opened)-1]
		item, err := app.W.Capture.WindowItem(ctx, bridge.CaptureWindowArgs{WindowKey: key})
		if err != nil || item == nil || item.Kind != bridge.CaptureKindChangeRequest {
			t.Fatalf("WindowItem = %+v, %v", item, err)
		}
		return key, *item
	}

	key, it := capture("https://github.com/acme/api/pull/11")
	res, err := app.W.Capture.Confirm(ctx, bridge.CaptureConfirmArgs{WindowKey: key, ItemID: it.ID, Title: it.Title, Day: &day})
	if err != nil || res.Notice != "" {
		t.Fatalf("Confirm = %+v, %v", res, err)
	}
	if !slices.Contains(app.WindowMgr.Closed, key) {
		t.Error("popup stays open after a complete capture")
	}
	board, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if board.Plan.Day[res.TaskID] != day {
		t.Errorf("plan day = %q, want %q", board.Plan.Day[res.TaskID], day)
	}
	i := slices.IndexFunc(board.Tasks, func(x adewire.Task) bool { return x.ID == res.TaskID })
	if i < 0 {
		t.Fatalf("task %s not on the board", res.TaskID)
	}
	task := board.Tasks[i]
	if task.Kind != "task" || task.HostURL != "https://github.com/acme/api/pull/11" || len(task.BranchIDs) != 1 {
		t.Fatalf("task = %+v", task)
	}
	j := slices.IndexFunc(board.Branches, func(b adewire.Branch) bool { return b.ID == task.BranchIDs[0] })
	if j < 0 {
		t.Fatalf("branch %s not on the board", task.BranchIDs[0])
	}
	br := board.Branches[j]
	if br.CodeRepoID != f.repoID || br.Name != "feat/login" || br.Kind != "review" || br.Worktree != "" {
		t.Fatalf("branch = %+v", br)
	}
	if out := f.clone.Git("for-each-ref", "refs/remotes/origin/feat/login"); !strings.Contains(out, "refs/remotes/origin/feat/login") {
		t.Errorf("head branch not fetched: %q", out)
	}

	key, it = capture("https://github.com/acme/api/pull/99")
	res, err = app.W.Capture.Confirm(ctx, bridge.CaptureConfirmArgs{WindowKey: key, ItemID: it.ID, Title: "Missing PR"})
	if err != nil || !strings.Contains(strings.ToLower(res.Notice), "not found") {
		t.Fatalf("Confirm = %+v, %v", res, err)
	}
	if slices.Contains(app.WindowMgr.Closed, key) {
		t.Error("popup closed despite a notice")
	}
	board, _ = app.W.AdeTask.Board(ctx)
	for _, x := range board.Tasks {
		if x.ID == res.TaskID && len(x.BranchIDs) != 0 {
			t.Errorf("failed capture attached branches: %+v", x)
		}
	}
}
