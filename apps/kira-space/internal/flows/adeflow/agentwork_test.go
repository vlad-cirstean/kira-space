package adeflow_test

import (
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

func TestAgentCommitReachesBoard(t *testing.T) {
	commit := "echo agent > agent.txt && git add -A && git commit -q -m 'agent change'"
	f := newRunFixture(t, []fakeagent.Action{{Name: "done", Sh: commit}}, agentStep("one", ""))
	app := f.app
	f.start(t, "feat/api-work")
	waitRun(t, app, f.taskID, "one", "done")

	br := branchOf(t, board(t, app), f.taskID, f.repoID)
	if br.Ahead != 1 || br.Dirty == nil || len(br.Dirty) != 0 {
		t.Fatalf("branch after the agent commit = ahead %d dirty %+v, want the one agent commit and a clean tree", br.Ahead, br.Dirty)
	}

	// The remote moves on, the worktree rebases: pushing needs a lease-checked force push.
	gitOut(t, br.Worktree, "push", "-q", "-u", "origin", "feat/api-work")
	other := f.bare.Clone(filepath.Join(app.Work, "other"))
	other.Commit("main moves", map[string]string{"m.txt": "m\n"})
	other.Git("push", "-q", "origin", "main")
	gitOut(t, br.Worktree, "fetch", "-q")
	gitOut(t, br.Worktree, "rebase", "-q", "origin/main")
	br = branchOf(t, board(t, app), f.taskID, f.repoID)
	if br.UpstreamAhead != 2 || br.UpstreamBehind != 1 || br.Behind != 0 {
		t.Fatalf("diverged branch = upstream %d/%d behind %d, want 2/1 and rebased onto main", br.UpstreamAhead, br.UpstreamBehind, br.Behind)
	}
	if !board(t, app).Plan.Unpushed[br.ID] {
		t.Fatal("plan does not flag the rebased branch as unpushed")
	}
	res, err := app.W.AdeTask.ForcePush(ctx, adewire.BranchArgs{BranchID: br.ID})
	if err != nil || res.Error != nil {
		t.Fatalf("ForcePush = %+v, %v", res, err)
	}
	if remoteTip, local := f.bare.Git("rev-parse", "refs/heads/feat/api-work"), gitOut(t, br.Worktree, "rev-parse", "HEAD"); remoteTip != local {
		t.Fatalf("remote branch tip %s, want the rebased local tip %s", remoteTip, local)
	}

	// The branch lands in the develop integration branch first: RecordMerge notes it.
	other.Git("fetch", "-q")
	other.Git("checkout", "-q", "-b", "develop", "origin/main")
	other.Git("merge", "-q", "--no-ff", "-m", "merge feature into develop", "origin/feat/api-work")
	other.Git("push", "-q", "origin", "develop")
	setRepoFile(t, app, f.repoID, func(rf *repocfg.File) { rf.IntegrationBranches = []string{"develop"} })
	if err := app.W.AdeTask.RecordMerge(ctx, adewire.RecordMergeArgs{BranchID: br.ID, Target: "develop"}); err == nil {
		t.Fatal("RecordMerge accepted a develop the local repo has not fetched yet")
	}
	f.repo.Git("fetch", "-q")
	if err := app.W.AdeTask.RecordMerge(ctx, adewire.RecordMergeArgs{BranchID: br.ID, Target: "main"}); err == nil {
		t.Fatal("RecordMerge accepted a target that is not an integration branch")
	}
	if err := app.W.AdeTask.RecordMerge(ctx, adewire.RecordMergeArgs{BranchID: br.ID, Target: "develop"}); err != nil {
		t.Fatal(err)
	}
	br = branchOf(t, board(t, app), f.taskID, f.repoID)
	if len(br.Integration) != 1 || br.Integration[0].Target != "develop" || br.Integration[0].Status != "merged" || !br.Integration[0].Recorded {
		t.Fatalf("integration after RecordMerge = %+v, want develop merged and recorded", br.Integration)
	}
	if br.MergedIntoMain {
		t.Fatal("branch reports merged into main before main has it")
	}

	// Merging into main on the remote and refreshing marks the branch merged.
	other.Git("checkout", "-q", "main")
	other.Git("merge", "-q", "--no-ff", "-m", "merge feature", "origin/feat/api-work")
	other.Git("push", "-q", "origin", "main")
	refresh, err := app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{CodeRepoIDs: []string{f.repoID}})
	if err != nil || refresh.Repos[0].Error != nil {
		t.Fatalf("Refresh = %+v, %v", refresh, err)
	}
	br = branchOf(t, board(t, app), f.taskID, f.repoID)
	if !br.MergedIntoMain || br.MergedAt == nil {
		t.Fatalf("branch after the merge on the remote = %+v, want merged into main", br)
	}
}
