package adeflow_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// Worktrees without a prompt (P702): an agent creates branches and worktrees and removes its own;
// the user's remove needs no confirm dialog, only a labelled discard when changes would be lost.
// Contract ade-worktree-remove: tests/ui/ade-v2-worktree-remove.spec.ts reads these.

const removeScenario = "ade-worktree-remove"

var rowMask = flowharness.Mask("id", "taskId", "codeRepoId", "worktree", "addedAt", "tip", "base", "setup", "createdAt", "dirtyToken")

// agentCreate creates name for the task as the agent does and returns its row.
func (f *brFixture) agentCreate(t *testing.T, repo, name string) adewire.Branch {
	t.Helper()
	res, err := f.app.W.AdeBoard.RequestBranches(ctx, agentmcp.SpaceCall{TaskID: f.task.ID}, []agentmcp.BranchSpec{{Repo: repo, Branch: name}})
	if err != nil || len(res.Branches) != 1 || res.Branches[0].Outcome != agentmcp.BranchCreated {
		t.Fatalf("RequestBranches %s = %+v, %v; want created", name, res, err)
	}
	return f.branchNamed(t, name)
}

// agentRemove is the agent's worktree.remove for a branch of api.
func (f *brFixture) agentRemove(t *testing.T, branch string) agentmcp.RemoveWorktreeResult {
	t.Helper()
	res, err := f.app.W.AdeBoard.RemoveWorktree(ctx, agentmcp.SpaceCall{TaskID: f.task.ID}, agentmcp.RemoveWorktreeArgs{Repo: "api", Branch: branch})
	if err != nil {
		t.Fatalf("RemoveWorktree %s: %v", branch, err)
	}
	return res
}

func (f *brFixture) refusedRemove(t *testing.T, branch, want string) {
	t.Helper()
	res := f.agentRemove(t, branch)
	if res.Outcome != agentmcp.BranchFailed || !strings.Contains(res.Next, want) {
		t.Fatalf("RemoveWorktree = %+v, want a failure holding %q", res, want)
	}
}

func newRemoveFixture(t *testing.T, actions []fakeagent.Action, opts ...flowharness.Opt) *brFixture {
	t.Helper()
	return newBRFixture(t, actions, onceStepYML, true, opts...)
}

// A new branch, a branch in a second repo and a free existing branch all land at once: no approvals
// item, no open request, no approval op.
func TestAgentBranchCreateNoPrompt(t *testing.T) {
	calls := requestCalls([2]string{"api", "feat/one"}, [2]string{"web", "feat/two"})
	free := stateCall("branch.request", map[string]any{"repo": "api", "branch": "feat/free", "existing": true})
	free.Tolerate = true
	f := newRemoveFixture(t, []fakeagent.Action{{MCP: append(calls, free), Parallel: true, Name: "done"}})
	f.repo.Git("branch", "feat/free")
	f.startRun(t)
	waitRun(t, f.app, f.task.ID, "one", "done")
	if got := f.results(t, 3); strings.Count(got, `"status":"done"`) != 3 {
		t.Fatalf("agent results = %s, want three done", got)
	}
	f.noApproval(t)
	f.agentOps(t, "agentBranches", "branch.request", 1)
	rows := branchesOf(t, f.app, f.task.ID)
	if len(rows) != 3 {
		t.Fatalf("rows = %+v, want three named rows with worktrees", rows)
	}
	for _, r := range rows {
		if r.Name == "" || r.Worktree == "" || r.Origin != "agent" {
			t.Fatalf("row %+v, want an agent row with a worktree", r)
		}
	}
	// The calls race: the contract stores the rows sorted by name.
	slices.SortFunc(rows, func(a, b adewire.Branch) int { return strings.Compare(a.Name, b.Name) })
	f.app.Contract(t, removeScenario, "AdeTaskService.Board#afterCreate", rows, rowMask)
}

func TestAgentWorktreeRemove(t *testing.T) {
	t.Run("a clean worktree goes, the branch stays", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		row := f.agentCreate(t, "api", "feat/clean")
		res := f.agentRemove(t, "feat/clean")
		if res.Outcome != agentmcp.BranchRemoved || res.Worktree != row.Worktree || !strings.Contains(res.Next, "stays in git") {
			t.Fatalf("RemoveWorktree = %+v, want removed at %s", res, row.Worktree)
		}
		gone(t, row.Worktree)
		if gitOut(t, f.repo.Dir, "branch", "--list", "feat/clean") == "" {
			t.Fatal("the git branch was deleted")
		}
		if got := branchOf(t, board(t, f.app), f.task.ID, f.api); got.Name != "" || got.Worktree != "" {
			t.Fatalf("row = %+v, want the repo left without a branch", got)
		}
		op, ok := findOp(opsNow(t, f.app), oplog.CategoryAgent, "agentWorktreeRemove")
		if !ok || op.Status != oplog.StatusOK {
			t.Fatalf("remove op = %+v, want ok", op)
		}
		f.noApproval(t)
		f.app.Contract(t, removeScenario, "AdeTaskService.Board#afterRemove", taskOf(t, board(t, f.app), f.task.ID), brMask)
	})

	t.Run("unmerged commits do not block: the branch keeps them", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		row := f.agentCreate(t, "api", "feat/ahead")
		if err := os.WriteFile(filepath.Join(row.Worktree, "work.txt"), []byte("work\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitOut(t, row.Worktree, "add", "-A")
		gitOut(t, row.Worktree, "commit", "-q", "-m", "work")
		tip := headOf(t, row.Worktree)
		if res := f.agentRemove(t, "feat/ahead"); res.Outcome != agentmcp.BranchRemoved {
			t.Fatalf("RemoveWorktree = %+v, want removed", res)
		}
		gone(t, row.Worktree)
		if got := gitOut(t, f.repo.Dir, "rev-parse", "feat/ahead"); got != tip {
			t.Fatalf("branch tip = %s, want the commit %s kept", got, tip)
		}
	})

	t.Run("uncommitted changes are refused: only the user discards", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		row := f.agentCreate(t, "api", "feat/dirty")
		if err := os.WriteFile(filepath.Join(row.Worktree, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		f.refusedRemove(t, "feat/dirty", "1 uncommitted changes (wip.txt). Commit them first, or ask the user; only the user can discard them.")
		if _, err := os.Stat(filepath.Join(row.Worktree, "wip.txt")); err != nil {
			t.Fatalf("refused remove lost the file: %v", err)
		}
		if got := f.branchNamed(t, "feat/dirty"); got.Worktree != row.Worktree {
			t.Fatalf("row = %+v, want it kept", got)
		}
	})

	t.Run("a locked worktree is refused", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		row := f.agentCreate(t, "api", "feat/locked")
		gitOut(t, f.repo.Dir, "worktree", "lock", row.Worktree)
		f.refusedRemove(t, "feat/locked", "locked")
		if _, err := os.Stat(row.Worktree); err != nil {
			t.Fatalf("refused remove lost the worktree: %v", err)
		}
	})

	t.Run("a branch in the repo checkout has no worktree to remove", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		f.repo.Git("checkout", "-q", "-b", "feat/main-held")
		if _, err := f.app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: f.api, Name: "feat/main-held", TaskID: f.task.ID}); err != nil {
			t.Fatal(err)
		}
		row := f.branchNamed(t, "feat/main-held")
		if row.Worktree != "" && row.Worktree != f.repo.Dir {
			t.Skipf("AddExistingBranch moved the branch to %s; the main-checkout refusal needs a row without a Kira worktree", row.Worktree)
		}
		f.refusedRemove(t, "feat/main-held", "no Kira worktree")
		if head := headRef(t, f.repo.Dir); head != "feat/main-held" {
			t.Fatalf("repo checkout HEAD = %q, want it untouched", head)
		}
	})

	t.Run("not the agent's branch", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		f.repo.Git("config", "user.email", "me@example.com")
		f.repo.Git("checkout", "-q", "-b", "sara/topic", "main")
		f.repo.Commit("topic", map[string]string{"b.txt": "topic\n"})
		f.repo.Checkout("main")
		row := branchOf(t, board(t, f.app), f.task.ID, f.api)
		made, err := f.app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: "sara/topic", Existing: true})
		if err != nil || made.Kind != "review" {
			t.Fatalf("attach = %+v, %v, want a review row", made, err)
		}
		f.refusedRemove(t, "sara/topic", "not yours to remove")
	})

	t.Run("a branch the task does not hold is an error", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		if _, err := f.app.W.AdeBoard.RemoveWorktree(ctx, agentmcp.SpaceCall{TaskID: f.task.ID}, agentmcp.RemoveWorktreeArgs{Repo: "api", Branch: "feat/none"}); err == nil || !strings.Contains(err.Error(), "is not on this task") {
			t.Fatalf("RemoveWorktree = %v, want the not-on-this-task error", err)
		}
	})

	t.Run("a session open on the worktree blocks it", func(t *testing.T) {
		hold := filepath.Join(t.TempDir(), "hold")
		app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
		claude(app, map[string][]fakeagent.Action{"*": {{WaitFile: hold}}})
		repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
		rec := importRepo(t, app, repo.Dir)
		saveWorkflow(t, app, "flow", flowYAML("flow", "  - id: work\n    name: Work\n    kind: manual\n    session: true\n"))
		f := &brFixture{app: app, repo: repo, api: rec.ID, task: createTask(t, app, "Fix login", "flow", rec.ID)}
		row := f.agentCreate(t, "api", "feat/open")
		app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
		launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.task.ID})
		if err != nil {
			t.Fatal(err)
		}
		openLaunch(t, app, launch)
		waitSession(t, app, launch.SessionID, "running")
		f.refusedRemove(t, "feat/open", "a session is open")
		if _, err := os.Stat(row.Worktree); err != nil {
			t.Fatalf("refused remove lost the worktree: %v", err)
		}
		if err := os.WriteFile(hold, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("the agent's own working directory is refused", func(t *testing.T) {
		own := stateCall("worktree.remove", map[string]any{"repo": "api", "branch": "feat/own"})
		own.Tolerate = true
		f := newRemoveFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{own}, Name: "done"}})
		row := f.agentCreate(t, "api", "feat/own")
		f.startRun(t)
		waitRun(t, f.app, f.task.ID, "one", "done")
		if got := f.resultOf(t, "worktree.remove", 1); !strings.Contains(got, "your working directory") {
			t.Fatalf("agent result = %s, want the working-directory refusal", got)
		}
		if _, err := os.Stat(row.Worktree); err != nil {
			t.Fatalf("the agent removed its own worktree: %v", err)
		}
	})
}

// The user removes without a confirm dialog: clean goes at once; changes need the labelled discard.
func TestUserWorktreeRemoveNoConfirm(t *testing.T) {
	f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
	clean := f.agentCreate(t, "api", "feat/user-clean")
	if err := f.app.W.AdeTask.RemoveTaskBranch(ctx, adewire.RemoveTaskRowArgs{BranchID: clean.ID}); err != nil {
		t.Fatalf("RemoveTaskBranch clean: %v", err)
	}
	gone(t, clean.Worktree)

	dirty := f.agentCreate(t, "api", "feat/user-dirty")
	if err := os.WriteFile(filepath.Join(dirty.Worktree, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.app.W.AdeTask.RemoveTaskBranch(ctx, adewire.RemoveTaskRowArgs{BranchID: dirty.ID}); errCode(err) != "E_BAD_REQUEST" || !strings.Contains(err.Error(), "use Discard changes and remove") {
		t.Fatalf("RemoveTaskBranch dirty = %v, want E_BAD_REQUEST naming the discard action", err)
	}
	if _, err := os.Stat(dirty.Worktree); err != nil {
		t.Fatalf("refused remove lost the worktree: %v", err)
	}
	// Changing the branch refuses the same way, then discards on the user's word.
	if _, err := f.app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: dirty.ID, Name: "feat/user-other"}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("SetTaskBranch dirty = %v, want E_BAD_REQUEST", err)
	}
	f.app.Contract(t, removeScenario, "AdeTaskService.Board#dirty", branchOf(t, board(t, f.app), f.task.ID, f.api), rowMask)
	seen := freshBranch(t, f.app, f.task.ID, f.api).DirtyToken
	if seen == "" {
		t.Fatal("dirty row carries no dirtyToken")
	}
	// A file written after the user looked voids the token: nothing is discarded.
	if err := os.WriteFile(filepath.Join(dirty.Worktree, "late.txt"), []byte("late\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: dirty.ID, Name: "feat/user-other", DiscardToken: seen}); errCode(err) != "E_BAD_REQUEST" || !strings.Contains(err.Error(), "changed since you reviewed") {
		t.Fatalf("SetTaskBranch with a stale token = %v, want E_BAD_REQUEST naming the change", err)
	}
	if _, err := os.Stat(filepath.Join(dirty.Worktree, "late.txt")); err != nil {
		t.Fatalf("late.txt lost by a stale token: %v", err)
	}
	seen = freshBranch(t, f.app, f.task.ID, f.api).DirtyToken
	changed, err := f.app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: dirty.ID, Name: "feat/user-other", DiscardToken: seen})
	if err != nil || changed.Name != "feat/user-other" {
		t.Fatalf("SetTaskBranch with discard = %+v, %v", changed, err)
	}
	gone(t, dirty.Worktree)
	if err := os.WriteFile(filepath.Join(changed.Worktree, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.app.W.AdeTask.RemoveTaskBranch(ctx, adewire.RemoveTaskRowArgs{BranchID: changed.ID, DiscardToken: freshBranch(t, f.app, f.task.ID, f.api).DirtyToken}); err != nil {
		t.Fatalf("RemoveTaskBranch with discard: %v", err)
	}
	gone(t, changed.Worktree)
	if out, err := exec.Command("git", "-C", f.repo.Dir, "branch", "--list", "feat/user-other").Output(); err != nil || strings.TrimSpace(string(out)) == "" {
		t.Fatalf("git branch lost: %q, %v", out, err)
	}
}
