package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Saving a listed file again keeps the discard token; a new path voids it and the refusal pushes the
// board, so the row reloads with the current list and token instead of looping on the old one.
func TestDiscardTokenFollowsShownSet(t *testing.T) {
	f, br := doneFixture(t, "feat/api-token")
	app := f.app
	wip := filepath.Join(br.Worktree, "wip.txt")
	if err := os.WriteFile(wip, []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seen := freshBranch(t, app, f.taskID, f.repoID).DirtyToken
	if seen == "" {
		t.Fatal("dirty row carries no dirtyToken")
	}
	if err := os.WriteFile(wip, []byte("wip, saved again with more\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := freshBranch(t, app, f.taskID, f.repoID).DirtyToken; got != seen {
		t.Fatalf("token after re-saving a listed file = %q, want unchanged %q", got, seen)
	}

	if err := os.WriteFile(filepath.Join(br.Worktree, "late.txt"), []byte("late\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: br.ID, Name: "feat/api-token-2", DiscardToken: seen})
	if errCode(err) != "E_BAD_REQUEST" || !strings.Contains(err.Error(), "changed since you reviewed") {
		t.Fatalf("SetTaskBranch with a stale token = %v, want the changed-since refusal", err)
	}
	// The refusal pushed the board: the row now carries the token of the set with late.txt.
	testx.WaitUntil(t, waitFor, func() bool {
		b := branchOf(t, board(t, app), f.taskID, f.repoID)
		return b.DirtyToken != "" && b.DirtyToken != seen
	})
	fresh := branchOf(t, board(t, app), f.taskID, f.repoID).DirtyToken
	if _, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: br.ID, Name: "feat/api-token-2", DiscardToken: fresh}); err != nil {
		t.Fatalf("SetTaskBranch with the reloaded token: %v", err)
	}
}

// Archive stops a running agent first, then compares the token once: an agent save after the dialog
// opened does not refuse the archive.
func TestArchiveWithRunningAgentEditing(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Sh: "echo wip > wip.txt", WaitFile: filepath.Join(t.TempDir(), "never")}}, agentStep("one", ""))
	app := f.app
	f.start(t, "feat/api-live")
	waitRun(t, app, f.taskID, "one", "running")
	br := branchOf(t, board(t, app), f.taskID, f.repoID)
	wip := filepath.Join(br.Worktree, "wip.txt")
	testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(wip); return err == nil })

	risk, err := app.W.AdeTask.ArchiveRisk(ctx, adewire.TaskArgs{TaskID: f.taskID})
	if err != nil || len(risk.Branches) != 1 || risk.Branches[0].DirtyToken == "" {
		t.Fatalf("ArchiveRisk = %+v, %v, want the dirty worktree", risk, err)
	}
	// The agent saves again after the dialog listed wip.txt.
	if err := os.WriteFile(wip, []byte("wip, saved again by the agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{risk.Branches[0].BranchID: risk.Branches[0].DirtyToken}
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.ArchiveTaskArgs{TaskID: f.taskID, DiscardTokens: tokens}); err != nil {
		t.Fatalf("ArchiveTask with a live agent: %v", err)
	}
	if _, err := os.Stat(br.Worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree after archive: %v, want gone", err)
	}
}
