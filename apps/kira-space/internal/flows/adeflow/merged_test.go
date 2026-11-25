package adeflow_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// P757 merged into main. Contract ade-merged: tests/ui/ade-v2-merged.spec.ts serves each case's
// branch as a fixture row and asserts the same merged state in the UI. Keyed by case.

// anyRemoteForge claims whatever origin URL the repo has, so a real bare origin still fetches while
// the fake host answers the change request lookups.
type anyRemoteForge struct{ *fakehost.Forge }

func (anyRemoteForge) ParseRemote(string) (githost.Repo, bool) {
	return githost.Repo{Host: fakehost.Host, Owner: "acme", Name: "api"}, true
}

var mergedMask = flowharness.Mask("startedAt", "finishedAt", "sessionId", "tip", "createdAt", "addedAt", "lastCommitAt", "dirtyToken", "mergedAt")

type mergedFx struct {
	*baseFx
	forge *fakehost.Forge
	other *flowharness.Repo
}

func newMergedFx(t *testing.T) *mergedFx {
	t.Helper()
	forge := fakehost.New()
	app := flowharness.New(t, flowharness.WithGitHost(anyRemoteForge{forge}), flowharness.WithoutGh())
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	repo, bare := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	f := &baseFx{app: app, repo: repo, bare: bare, repoID: rec.ID}
	return &mergedFx{baseFx: f, forge: forge, other: bare.Clone(filepath.Join(app.Work, "other"))}
}

// adopt attaches the pushed branch as a new task's branch and returns its board row.
func (f *mergedFx) adopt(t *testing.T, name string) adewire.Branch {
	t.Helper()
	if _, err := f.app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: f.repoID, Name: name}); err != nil {
		t.Fatalf("AddExistingBranch %s: %v", name, err)
	}
	return f.named(t, name)
}

func (f *mergedFx) named(t *testing.T, name string) adewire.Branch {
	t.Helper()
	for _, br := range board(t, f.app).Branches {
		if br.Name == name {
			return br
		}
	}
	t.Fatalf("board has no branch %s", name)
	return adewire.Branch{}
}

// started creates a task on base and starts its branch, which holds no own commit.
func (f *mergedFx) started(t *testing.T, title, name string, base adewire.BaseChoice) adewire.Branch {
	t.Helper()
	task, err := f.app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{
		Title: title, CodeRepoIDs: []string{f.repoID}, WorkflowID: "flow",
		Bases: map[string]adewire.BaseChoice{f.repoID: base},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	br := branchOf(t, board(t, f.app), task.ID, f.repoID)
	if err := f.startRun(t, task, br, name); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, task.ID, "one", "done")
	return f.named(t, name)
}

// mergeOnRemote merges origin/branch into main on the remote with mode (--no-ff, --ff-only, --squash).
func (f *mergedFx) mergeOnRemote(t *testing.T, branch, mode string) {
	t.Helper()
	f.other.Git("fetch", "-q")
	f.other.Git("checkout", "-q", "-B", "main", "origin/main")
	f.other.Git("merge", "-q", mode, "origin/"+branch)
	if mode == "--squash" {
		f.other.Git("commit", "-q", "-m", "squash "+branch)
	}
	f.other.Git("push", "-q", "origin", "main")
}

// followUp commits on the branch in its worktree and returns the new tip.
func (f *mergedFx) followUp(t *testing.T, br adewire.Branch, file string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(br.Worktree, file), []byte(file+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, br.Worktree, "add", "-A")
	gitOut(t, br.Worktree, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "follow-up "+file)
	return gitOut(t, br.Worktree, "rev-parse", "HEAD")
}

// settle refreshes and waits for the PR merge tail, then returns the branch from a fresh board.
func (f *mergedFx) settle(t *testing.T, name string) adewire.Branch {
	t.Helper()
	f.refresh(t)
	f.app.W.AdeBoard.WaitRefreshTails()
	return f.named(t, name)
}

func (f *mergedFx) prMerged(number int, branch, headSha string) {
	f.forge.AddChangeRequest(githost.ChangeRequest{
		Number: number, Title: branch, State: "merged", HeadRef: branch, HeadOwner: "acme", BaseRef: "main", HeadSha: headSha,
		URL: fmt.Sprintf("https://forge.test/acme/api/pull/%d", number), UpdatedAt: time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC).UnixMilli(),
	})
}

func (f *mergedFx) wantMerged(t *testing.T, br adewire.Branch, want bool) {
	t.Helper()
	if br.Kind != "mine" {
		t.Fatalf("%s kind = %q, want mine", br.Name, br.Kind)
	}
	if br.MergedIntoMain != want || (br.MergedAt != nil) != want {
		t.Fatalf("%s mergedIntoMain=%v mergedAt=%v ahead=%d, want merged %v", br.Name, br.MergedIntoMain, br.MergedAt, br.Ahead, want)
	}
}

func (f *mergedFx) record(t *testing.T, key string, br adewire.Branch) {
	t.Helper()
	f.app.Contract(t, "ade-merged", "AdeTaskService.Branch#"+key, br, mergedMask)
}

func TestMergedIntoMain(t *testing.T) {
	t.Run("stacked parent merged", func(t *testing.T) {
		for _, mode := range []string{"--no-ff", "--ff-only"} {
			f := newMergedFx(t)
			f.pushBranch(t, "feat/parent")
			parent := f.adopt(t, "feat/parent")
			child := f.started(t, "Stacked child", "feat/child", adewire.BaseChoice{BranchID: parent.ID})
			if child.Ahead != 0 || child.MergedIntoMain {
				t.Fatalf("child before the parent merges = ahead %d merged %v, want 0 false", child.Ahead, child.MergedIntoMain)
			}
			f.mergeOnRemote(t, "feat/parent", mode)
			child = f.settle(t, "feat/child")
			f.wantMerged(t, child, false)
			if child.Ahead != 0 {
				t.Fatalf("child after the parent merged (%s) ahead = %d, want 0", mode, child.Ahead)
			}
			f.wantMerged(t, f.named(t, "feat/parent"), true)
			if mode == "--no-ff" {
				f.record(t, "stacked-parent-merged", child)
			}
		}
	})

	t.Run("develop reaches main", func(t *testing.T) {
		f := newMergedFx(t)
		f.pushBranch(t, "develop")
		f.started(t, "Develop task", "feat/dev-task", adewire.BaseChoice{Ref: "develop"})
		f.mergeOnRemote(t, "develop", "--ff-only")
		br := f.settle(t, "feat/dev-task")
		f.wantMerged(t, br, false)
		f.record(t, "develop-into-main", br)
	})

	t.Run("commit after merge", func(t *testing.T) {
		f := newMergedFx(t)
		f.pushBranch(t, "feat/follow")
		br := f.adopt(t, "feat/follow")
		if br.Ahead != 1 {
			t.Fatalf("adopted branch ahead = %d, want 1", br.Ahead)
		}
		f.mergeOnRemote(t, "feat/follow", "--no-ff")
		br = f.settle(t, "feat/follow")
		f.wantMerged(t, br, true)

		f.followUp(t, br, "follow.txt")
		br = f.settle(t, "feat/follow")
		f.wantMerged(t, br, false)
		if br.Ahead != 1 {
			t.Fatalf("branch after the follow-up commit ahead = %d, want 1", br.Ahead)
		}
		if _, err := f.app.W.AdeTask.RebasePreview(ctx, adewire.OntoArgs{BranchID: br.ID}); err != nil {
			t.Fatalf("RebasePreview of a branch with new commits after its merge: %v", err)
		}
		f.record(t, "commit-after-merge", br)

		gitOut(t, br.Worktree, "push", "-q", "origin", "feat/follow")
		f.mergeOnRemote(t, "feat/follow", "--no-ff")
		f.wantMerged(t, f.settle(t, "feat/follow"), true)
	})

	t.Run("pr from another head", func(t *testing.T) {
		f := newMergedFx(t)
		f.pushBranch(t, "feat/reused")
		f.adopt(t, "feat/reused")
		f.prMerged(7, "feat/reused", f.repo.Git("rev-parse", "origin/main"))
		br := f.settle(t, "feat/reused")
		if f.forge.Count("ForBranch:feat/reused") == 0 {
			t.Fatal("refresh never looked up the change request of the branch")
		}
		f.wantMerged(t, br, false)
		f.record(t, "pr-other-head", br)
	})

	t.Run("merged", func(t *testing.T) {
		f := newMergedFx(t)
		f.pushBranch(t, "feat/done")
		f.adopt(t, "feat/done")
		f.mergeOnRemote(t, "feat/done", "--no-ff")
		br := f.settle(t, "feat/done")
		f.wantMerged(t, br, true)
		f.record(t, "merged", br)
	})

	t.Run("squash pr head", func(t *testing.T) {
		f := newMergedFx(t)
		tip := f.pushBranch(t, "feat/squash")
		f.adopt(t, "feat/squash")
		f.mergeOnRemote(t, "feat/squash", "--squash")
		f.prMerged(8, "feat/squash", tip)
		br := f.settle(t, "feat/squash")
		f.wantMerged(t, br, true)
		f.record(t, "squash-pr-head", br)

		f.followUp(t, br, "after-squash.txt")
		br = f.settle(t, "feat/squash")
		f.wantMerged(t, br, false)
	})
}
