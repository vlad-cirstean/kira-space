package adeflow_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

type baseFx struct {
	app    *flowharness.App
	repo   *flowharness.Repo
	bare   *flowharness.Bare
	repoID string
}

// newBaseFx is an app with one imported clone "api" and an agent workflow "flow".
func newBaseFx(t *testing.T) *baseFx {
	t.Helper()
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	repo, bare := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	return &baseFx{app: app, repo: repo, bare: bare, repoID: rec.ID}
}

// pushBranch creates name from main with one commit and pushes it; the local copy stays.
func (f *baseFx) pushBranch(t *testing.T, name string) string {
	t.Helper()
	f.repo.Git("checkout", "-q", "-b", name, "main")
	sha := f.repo.Commit("work on "+name, map[string]string{strings.ReplaceAll(name, "/", "-") + ".txt": name + "\n"})
	f.repo.Git("push", "-q", "origin", name)
	f.repo.Checkout("main")
	return sha
}

func (f *baseFx) task(t *testing.T, title string, base adewire.BaseChoice) (adewire.Task, adewire.Branch) {
	t.Helper()
	args := adewire.CreateTaskArgs{Title: title, CodeRepoIDs: []string{f.repoID}, WorkflowID: "flow"}
	if base != (adewire.BaseChoice{}) {
		args.Bases = map[string]adewire.BaseChoice{f.repoID: base}
		if base.Ref == "develop" {
			f.app.Contract(t, "ade-base", "args:AdeTaskService.CreateTask.bases#develop", base)
		}
	}
	task, err := f.app.W.AdeTask.CreateTask(ctx, args)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task, branchOf(t, board(t, f.app), task.ID, f.repoID)
}

func (f *baseFx) startRun(t *testing.T, task adewire.Task, br adewire.Branch, name string) error {
	t.Helper()
	_, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: name}})
	return err
}

func (f *baseFx) refresh(t *testing.T) {
	t.Helper()
	if _, err := f.app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{CodeRepoIDs: []string{f.repoID}}); err != nil {
		t.Fatal(err)
	}
}

func TestTaskBase(t *testing.T) {
	t.Run("picker lists remote-only branches and the planner branches", func(t *testing.T) {
		f := newBaseFx(t)
		f.pushBranch(t, "develop")
		f.repo.Git("branch", "-D", "develop")
		task, br := f.task(t, "Fix login", adewire.BaseChoice{})
		got, err := f.app.W.AdeTask.RepoBranches(ctx, adewire.RepoBranchesArgs{CodeRepoID: f.repoID, BranchID: br.ID})
		if err != nil {
			t.Fatal(err)
		}
		picks := map[string]adewire.BasePick{}
		for _, p := range got.Branches {
			picks[p.Name] = p
		}
		if got.MainName != "main" || !picks["develop"].Remote || picks["develop"].Local {
			t.Fatalf("picker = %+v, want main and remote-only develop", got)
		}
		// Contract ade-base: tests/ui/ade-v2-base-rebase.spec.ts "contract: ..." reads these.
		shown := got
		shown.Branches = []adewire.BasePick{picks["develop"]}
		f.app.Contract(t, "ade-base", "AdeTaskService.RepoBranches", shown)
		codes := map[string]string{}
		for _, p := range got.Branches {
			if p.BranchID != "" {
				codes[p.BranchID] = p.Excluded
			}
		}
		if codes[br.ID] != "self" {
			t.Fatalf("picker excluded %q for the branch of task %s, want self", codes[br.ID], task.ID)
		}
		_, child := f.task(t, "Stacked fix", adewire.BaseChoice{BranchID: br.ID})
		got, err = f.app.W.AdeTask.RepoBranches(ctx, adewire.RepoBranchesArgs{CodeRepoID: f.repoID, BranchID: br.ID})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range got.Branches {
			if p.BranchID == child.ID && p.Excluded != "stacked" {
				t.Fatalf("picker excluded %q for a branch stacked on it, want stacked", p.Excluded)
			}
		}
	})

	t.Run("new task starts from a remote-only base", func(t *testing.T) {
		f := newBaseFx(t)
		f.pushBranch(t, "develop")
		f.repo.Git("branch", "-D", "develop")
		task, br := f.task(t, "Fix login", adewire.BaseChoice{Ref: "develop"})
		if err := f.startRun(t, task, br, "feat/api-dev"); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, task.ID, "one", "done")
		br = branchOf(t, board(t, f.app), task.ID, f.repoID)
		if got, want := gitOut(t, br.Worktree, "rev-parse", "HEAD"), gitOut(t, f.repo.Dir, "rev-parse", "origin/develop"); got != want {
			t.Fatalf("branch starts at %s, want origin/develop %s", got, want)
		}
		if br.Base != "develop" || br.Ahead != 0 || br.Behind != 0 {
			t.Fatalf("board branch base=%q ahead=%d behind=%d, want develop 0 0", br.Base, br.Ahead, br.Behind)
		}
		f.app.Contract(t, "ade-base", "AdeTaskService.Branch#on-develop", br, flowharness.Mask("startedAt", "finishedAt", "tip", "createdAt", "addedAt", "lastCommitAt", "dirtyToken"))
	})

	t.Run("the remote wins over a stale local twin", func(t *testing.T) {
		f := newBaseFx(t)
		stale := f.pushBranch(t, "develop")
		f.repo.Checkout("develop")
		f.repo.Commit("develop moves", map[string]string{"moved.txt": "moved\n"})
		f.repo.Git("push", "-q", "origin", "develop")
		f.repo.Checkout("main")
		f.repo.Git("branch", "-f", "develop", stale)
		task, br := f.task(t, "Fix login", adewire.BaseChoice{Ref: "develop"})
		if err := f.startRun(t, task, br, "feat/api-remote-first"); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, task.ID, "one", "done")
		br = branchOf(t, board(t, f.app), task.ID, f.repoID)
		if got, want := gitOut(t, br.Worktree, "rev-parse", "HEAD"), gitOut(t, f.repo.Dir, "rev-parse", "origin/develop"); got != want || got == stale {
			t.Fatalf("branch starts at %s, want origin/develop %s (local twin %s)", got, want, stale)
		}
	})

	t.Run("a branch stacked on a draft waits for its parent", func(t *testing.T) {
		f := newBaseFx(t)
		parentTask, parent := f.task(t, "Parent", adewire.BaseChoice{})
		childTask, child := f.task(t, "Child", adewire.BaseChoice{BranchID: parent.ID})
		if child.BaseBranchID != parent.ID {
			t.Fatalf("child baseBranchId = %q, want %q", child.BaseBranchID, parent.ID)
		}
		err := f.startRun(t, childTask, child, "feat/api-child")
		if err == nil || !strings.Contains(err.Error(), "is not created yet") {
			t.Fatalf("StartRun on a child of a draft = %v, want 'is not created yet'", err)
		}
		if err := f.startRun(t, parentTask, parent, "feat/api-parent"); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, parentTask.ID, "one", "done")
		parentWt := branchOf(t, board(t, f.app), parentTask.ID, f.repoID).Worktree
		gitOut(t, parentWt, "commit", "-q", "--allow-empty", "-m", "parent work")
		if err := f.startRun(t, childTask, child, "feat/api-child"); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, childTask.ID, "one", "done")
		child = branchOf(t, board(t, f.app), childTask.ID, f.repoID)
		if got, want := gitOut(t, child.Worktree, "rev-parse", "HEAD"), gitOut(t, parentWt, "rev-parse", "HEAD"); got != want {
			t.Fatalf("child starts at %s, want the parent's tip %s", got, want)
		}
		if child.BaseBranchID != parent.ID || child.Base != "feat/api-parent" {
			t.Fatalf("child base = %q (%q), want the parent branch", child.Base, child.BaseBranchID)
		}
	})

	t.Run("a draft retargets its base", func(t *testing.T) {
		f := newBaseFx(t)
		f.pushBranch(t, "release")
		task, br := f.task(t, "Fix login", adewire.BaseChoice{})
		if err := f.app.W.AdeTask.SetBranchBase(ctx, adewire.SetBranchBaseArgs{BranchID: br.ID, Base: adewire.BaseChoice{Ref: "release"}}); err != nil {
			t.Fatal(err)
		}
		if err := f.startRun(t, task, br, "feat/api-release"); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, task.ID, "one", "done")
		br = branchOf(t, board(t, f.app), task.ID, f.repoID)
		if got, want := gitOut(t, br.Worktree, "rev-parse", "HEAD"), gitOut(t, f.repo.Dir, "rev-parse", "origin/release"); got != want || br.Base != "release" {
			t.Fatalf("branch starts at %s on base %q, want origin/release %s", got, br.Base, want)
		}
	})

	t.Run("SetBranchBase refuses what cannot be a base", func(t *testing.T) {
		f := newBaseFx(t)
		f.pushBranch(t, "release")
		web := importRepo(t, f.app, flowharness.NewRepo(t, filepath.Join(f.app.Work, "web")).Dir)
		_, a := f.task(t, "A", adewire.BaseChoice{})
		_, b := f.task(t, "B", adewire.BaseChoice{BranchID: a.ID})
		webTask, err := f.app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "W", CodeRepoIDs: []string{web.ID}})
		if err != nil {
			t.Fatal(err)
		}
		webBranch := branchOf(t, board(t, f.app), webTask.ID, web.ID)
		set := func(id string, c adewire.BaseChoice) error {
			return f.app.W.AdeTask.SetBranchBase(ctx, adewire.SetBranchBaseArgs{BranchID: id, Base: c})
		}
		for _, c := range []struct {
			name string
			err  error
			want string
		}{
			{"itself", set(a.ID, adewire.BaseChoice{BranchID: a.ID}), "its own base"},
			{"a branch of another repo", set(a.ID, adewire.BaseChoice{BranchID: webBranch.ID}), "same repo"},
			{"a cycle", set(a.ID, adewire.BaseChoice{BranchID: b.ID}), "cycle"},
			{"an unknown ref", set(a.ID, adewire.BaseChoice{Ref: "nope"}), "not found"},
		} {
			if c.err == nil || !strings.Contains(c.err.Error(), c.want) {
				t.Errorf("%s: SetBranchBase = %v, want an error with %q", c.name, c.err, c.want)
			}
		}
		// A created branch changes base only through Rebase.
		task, br := f.task(t, "C", adewire.BaseChoice{})
		if err := f.startRun(t, task, br, "feat/api-created"); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, task.ID, "one", "done")
		if err := set(br.ID, adewire.BaseChoice{Ref: "release"}); err == nil || !strings.Contains(err.Error(), "use Change base") {
			t.Fatalf("SetBranchBase on a created branch = %v, want 'use Change base'", err)
		}
	})

	t.Run("a deleted base reads as missing and refuses Rebase", func(t *testing.T) {
		f := newBaseFx(t)
		f.pushBranch(t, "develop")
		f.repo.Git("branch", "-D", "develop")
		task, br := f.task(t, "Fix login", adewire.BaseChoice{Ref: "develop"})
		if err := f.startRun(t, task, br, "feat/api-gone"); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, task.ID, "one", "done")
		f.bare.Git("branch", "-D", "develop")
		f.repo.Git("fetch", "-q", "--prune", "origin")
		f.refresh(t)
		testx.WaitUntil(t, waitFor, func() bool { return branchOf(t, board(t, f.app), task.ID, f.repoID).BaseMissing })
		_, err := f.app.W.AdeTask.Rebase(ctx, adewire.RebaseArgs{BranchID: br.ID})
		if err == nil || !strings.Contains(err.Error(), "no longer exists") {
			t.Fatalf("Rebase on a missing base = %v, want 'no longer exists'", err)
		}
		prev, err := f.app.W.AdeTask.RebasePreview(ctx, adewire.OntoArgs{BranchID: br.ID})
		if err != nil || len(prev.Blockers) != 1 || prev.Blockers[0].Kind != "baseMissing" {
			t.Fatalf("preview = %+v, %v, want one baseMissing blocker", prev, err)
		}
	})

	t.Run("a renamed base is chosen again with Change base", func(t *testing.T) {
		f := newBaseFx(t)
		f.pushBranch(t, "develop")
		f.repo.Git("branch", "-D", "develop")
		task, br := f.task(t, "Fix login", adewire.BaseChoice{Ref: "develop"})
		if err := f.startRun(t, task, br, "feat/api-renamed"); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, task.ID, "one", "done")
		f.bare.Git("branch", "-m", "develop", "dev")
		f.repo.Git("fetch", "-q", "--prune", "origin")
		f.refresh(t)
		testx.WaitUntil(t, waitFor, func() bool { return branchOf(t, board(t, f.app), task.ID, f.repoID).BaseMissing })
		res, err := f.app.W.AdeTask.Rebase(ctx, adewire.RebaseArgs{BranchID: br.ID, Onto: &adewire.BaseChoice{Ref: "dev"}})
		if err != nil || !res.NoOp {
			t.Fatalf("Rebase onto dev = %+v, %v, want a no-op (the branch already sits on it)", res, err)
		}
		got := branchOf(t, board(t, f.app), task.ID, f.repoID)
		if got.BaseMissing || got.Base != "dev" || got.BasePendingFrom != "" {
			t.Fatalf("branch after Change base = base %q missing %v pending %q, want dev, found, none", got.Base, got.BaseMissing, got.BasePendingFrom)
		}
		for _, r := range taskOf(t, board(t, f.app), task.ID).Runs {
			if r.Purpose == model.AdeRunPurposeRebase {
				t.Fatalf("a no-op Change base left a rebase run: %+v", r)
			}
		}
	})
}
