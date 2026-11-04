package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
)

func branchesOf(t *testing.T, app *flowharness.App, taskID string) []adewire.Branch {
	t.Helper()
	var out []adewire.Branch
	for _, br := range board(t, app).Branches {
		if br.TaskID == taskID {
			out = append(out, br)
		}
	}
	return out
}

func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("worktree %s survives: %v", path, err)
	}
}

// Same scenario as tests/ui/ade-v2-task-branches.spec.ts, asserted on the board.
func TestTaskReposAndBranches(t *testing.T) {
	app := flowharness.New(t)
	repoA, _ := cloneWithMain(app, filepath.Join(app.Work, "a"))
	repoB, _ := cloneWithMain(app, filepath.Join(app.Work, "b"))
	repoC, _ := cloneWithMain(app, filepath.Join(app.Work, "c"))
	a, b, c := importRepo(t, app, repoA.Dir), importRepo(t, app, repoB.Dir), importRepo(t, app, repoC.Dir)
	gitOut(t, repoA.Dir, "branch", "feat/a-existing")
	gitOut(t, repoA.Dir, "branch", "feat/a-other")
	gitOut(t, repoB.Dir, "branch", "feat/b-one")

	// Candidates cover one repo when asked, every repo when not.
	names := func(codeRepoID string) map[string]bool {
		res, err := app.W.AdeTask.CandidateBranches(ctx, adewire.CandidateBranchesArgs{CodeRepoID: codeRepoID})
		if err != nil {
			t.Fatalf("CandidateBranches(%q): %v", codeRepoID, err)
		}
		out := map[string]bool{}
		for _, c := range res.Branches {
			out[c.Name] = true
		}
		return out
	}
	if got := names(a.ID); len(got) != 2 || !got["feat/a-existing"] || !got["feat/a-other"] {
		t.Fatalf("candidates of repo a = %v, want only its two branches", got)
	}
	if got := names(""); len(got) != 3 || !got["feat/b-one"] {
		t.Fatalf("candidates of every repo = %v, want three", got)
	}

	task := createTask(t, app, "Edit repos", "")
	if _, err := app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: task.ID, CodeRepoID: a.ID}); err != nil {
		t.Fatalf("AddTaskRepo: %v", err)
	}
	draft := branchOf(t, board(t, app), task.ID, a.ID)
	if draft.Name != "" {
		t.Fatalf("added repo has branch %q, want none", draft.Name)
	}

	// A repo added without a branch gets one later, in the same row.
	if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: a.ID, Name: "feat/a-existing", TaskID: task.ID}); err != nil {
		t.Fatalf("AddExistingBranch into a repo without a branch: %v", err)
	}
	rows := branchesOf(t, app, task.ID)
	if len(rows) != 1 || rows[0].Name != "feat/a-existing" || rows[0].ID == draft.ID {
		t.Fatalf("rows = %+v, want one row on feat/a-existing", rows)
	}
	filled := rows[0]
	if filled.Worktree == "" {
		t.Fatal("attached branch has no worktree")
	}
	// A second branch of the same repo gets its own row; removing it leaves no draft behind.
	if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: a.ID, Name: "feat/a-other", TaskID: task.ID}); err != nil {
		t.Fatalf("second branch in the repo: %v", err)
	}
	var other adewire.Branch
	for _, r := range branchesOf(t, app, task.ID) {
		if r.Name == "feat/a-other" {
			other = r
		}
	}
	if other.ID == "" || other.ID == filled.ID {
		t.Fatalf("rows = %+v, want a second row on feat/a-other", branchesOf(t, app, task.ID))
	}
	if err := app.W.AdeTask.RemoveTaskBranch(ctx, adewire.RemoveTaskRowArgs{BranchID: other.ID}); err != nil {
		t.Fatalf("RemoveTaskBranch of one of two: %v", err)
	}
	if rows = branchesOf(t, app, task.ID); len(rows) != 1 || rows[0].ID != filled.ID {
		t.Fatalf("rows = %+v, want only the first branch", rows)
	}

	// Remove branch: the repo stays, git keeps the ref, the worktree goes.
	risk, err := app.W.AdeTask.TaskBranchRisk(ctx, adewire.BranchArgs{BranchID: filled.ID})
	if err != nil || risk.Worktree != filled.Worktree || len(risk.Dirty) != 0 {
		t.Fatalf("risk = %+v, %v, want a clean worktree", risk, err)
	}
	if err := app.W.AdeTask.RemoveTaskBranch(ctx, adewire.RemoveTaskRowArgs{BranchID: filled.ID}); err != nil {
		t.Fatalf("RemoveTaskBranch: %v", err)
	}
	if rows = branchesOf(t, app, task.ID); len(rows) != 1 || rows[0].Name != "" {
		t.Fatalf("rows = %+v, want one repo without a branch", rows)
	}
	if gitOut(t, repoA.Dir, "branch", "--list", "feat/a-existing") == "" {
		t.Fatal("RemoveTaskBranch deleted the git branch")
	}
	gone(t, filled.Worktree)

	// Add branch: a new name creates the branch and worktree now.
	made, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: rows[0].ID, Name: "feat/a-new"})
	if err != nil || made.Name != "feat/a-new" || made.Worktree == "" {
		t.Fatalf("SetTaskBranch new = %+v, %v", made, err)
	}
	if gitOut(t, repoA.Dir, "branch", "--list", "feat/a-new") == "" {
		t.Fatal("new branch not created in git")
	}
	if _, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: made.ID, Name: "feat/a-existing", Existing: false}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("new name that exists in git = %v, want E_BAD_REQUEST", err)
	}
	if branchOf(t, board(t, app), task.ID, a.ID).Name != "feat/a-new" {
		t.Fatal("a refused edit changed the row")
	}

	// Uncommitted changes need a confirmed discard.
	if err := os.WriteFile(filepath.Join(made.Worktree, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: made.ID, Name: "feat/a-other", Existing: true}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("SetTaskBranch over a dirty worktree = %v, want E_BAD_REQUEST", err)
	}
	if err := app.W.AdeTask.RemoveTaskBranch(ctx, adewire.RemoveTaskRowArgs{BranchID: made.ID}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("RemoveTaskBranch over a dirty worktree = %v, want E_BAD_REQUEST", err)
	}
	if _, err := os.Stat(made.Worktree); err != nil {
		t.Fatalf("refused edits removed the worktree: %v", err)
	}

	// Edit branch: the created row points at another existing branch; the old worktree goes.
	edited, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: made.ID, Name: "feat/a-other", Existing: true, DiscardChanges: true})
	if err != nil || edited.Name != "feat/a-other" || edited.Worktree == "" {
		t.Fatalf("SetTaskBranch existing = %+v, %v", edited, err)
	}
	gone(t, made.Worktree)
	if rows = branchesOf(t, app, task.ID); len(rows) != 1 || rows[0].ID != edited.ID {
		t.Fatalf("rows = %+v, want only the edited row", rows)
	}

	// Edit and remove repos.
	if _, err := app.W.AdeTask.UpdateTaskRepo(ctx, adewire.UpdateTaskRepoArgs{BranchID: edited.ID, CodeRepoID: b.ID}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("UpdateTaskRepo on a created row = %v, want E_BAD_REQUEST", err)
	}
	added, err := app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: task.ID, CodeRepoID: b.ID})
	if err != nil {
		t.Fatal(err)
	}
	draftC, err := app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: task.ID, CodeRepoID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: task.ID, CodeRepoID: c.ID}); errCode(err) != "E_CONFLICT" {
		t.Fatalf("second draft in a repo = %v, want E_CONFLICT", err)
	}
	if _, err := app.W.AdeTask.UpdateTaskRepo(ctx, adewire.UpdateTaskRepoArgs{BranchID: added.ID, CodeRepoID: c.ID}); errCode(err) != "E_CONFLICT" {
		t.Fatalf("UpdateTaskRepo into a repo with a draft = %v, want E_CONFLICT", err)
	}
	if err := app.W.AdeTask.RemoveTaskRepo(ctx, adewire.RemoveTaskRowArgs{BranchID: draftC.ID}); err != nil {
		t.Fatal(err)
	}
	swapped, err := app.W.AdeTask.UpdateTaskRepo(ctx, adewire.UpdateTaskRepoArgs{BranchID: added.ID, CodeRepoID: c.ID})
	if err != nil || swapped.CodeRepoID != c.ID {
		t.Fatalf("UpdateTaskRepo = %+v, %v", swapped, err)
	}
	if err := app.W.AdeTask.RemoveTaskRepo(ctx, adewire.RemoveTaskRowArgs{BranchID: swapped.ID}); err != nil {
		t.Fatalf("RemoveTaskRepo: %v", err)
	}
	if rows = branchesOf(t, app, task.ID); len(rows) != 1 || rows[0].CodeRepoID != a.ID {
		t.Fatalf("rows = %+v, want only repo a", rows)
	}
	if err := app.W.AdeTask.RemoveTaskRepo(ctx, adewire.RemoveTaskRowArgs{BranchID: edited.ID}); err != nil {
		t.Fatalf("RemoveTaskRepo of a created row: %v", err)
	}
	gone(t, edited.Worktree)
	if rows = branchesOf(t, app, task.ID); len(rows) != 0 {
		t.Fatalf("rows = %+v, want none", rows)
	}
}

func TestTaskBranchEditRefusals(t *testing.T) {
	t.Run("a review task", func(t *testing.T) {
		app := flowharness.New(t)
		repo := app.NewRepo("api")
		repo.Commit("base", map[string]string{"a.txt": "base\n"})
		repo.Git("checkout", "-q", "-b", "sara/topic", "main")
		repo.Commit("topic", map[string]string{"b.txt": "topic\n"})
		repo.Checkout("main")
		repo.Git("config", "user.email", "me@example.com")
		rec := importRepo(t, app, repo.Dir)
		res, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: rec.ID, Name: "sara/topic"})
		if err != nil || res.Task.Kind != "review" {
			t.Fatalf("review attach = %+v, %v", res.Task, err)
		}
		id := res.Branch.ID
		if err := app.W.AdeTask.RemoveTaskRepo(ctx, adewire.RemoveTaskRowArgs{BranchID: id}); errCode(err) != "E_BAD_REQUEST" {
			t.Fatalf("RemoveTaskRepo = %v, want E_BAD_REQUEST", err)
		}
		if err := app.W.AdeTask.RemoveTaskBranch(ctx, adewire.RemoveTaskRowArgs{BranchID: id}); errCode(err) != "E_BAD_REQUEST" {
			t.Fatalf("RemoveTaskBranch = %v, want E_BAD_REQUEST", err)
		}
		if _, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: id, Name: "sara/topic", Existing: true}); errCode(err) != "E_BAD_REQUEST" {
			t.Fatalf("SetTaskBranch = %v, want E_BAD_REQUEST", err)
		}
	})

	t.Run("a review branch on a normal task leaves", func(t *testing.T) {
		app := flowharness.New(t)
		repo := app.NewRepo("api")
		repo.Commit("base", map[string]string{"a.txt": "base\n"})
		repo.Git("checkout", "-q", "-b", "sara/topic", "main")
		repo.Commit("topic", map[string]string{"b.txt": "topic\n"})
		repo.Checkout("main")
		repo.Git("config", "user.email", "me@example.com")
		rec := importRepo(t, app, repo.Dir)
		task := createTask(t, app, "Mine", "")
		row, err := app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: task.ID, CodeRepoID: rec.ID})
		if err != nil {
			t.Fatal(err)
		}
		row, err = app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: "sara/topic", Existing: true})
		if err != nil || row.Kind != "review" {
			t.Fatalf("attach a colleague's branch = %+v, %v, want a review row", row, err)
		}
		if err := app.W.AdeTask.RemoveTaskRepo(ctx, adewire.RemoveTaskRowArgs{BranchID: row.ID}); err != nil {
			t.Fatalf("RemoveTaskRepo of a review row on a normal task: %v", err)
		}
	})

	t.Run("an invalid new name", func(t *testing.T) {
		app := flowharness.New(t)
		repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
		rec := importRepo(t, app, repo.Dir)
		task := createTask(t, app, "Mine", "", rec.ID)
		row := branchOf(t, board(t, app), task.ID, rec.ID)
		for _, name := range []string{"main", "bad name", "-x", "a..b", "HEAD", strings.Repeat("a", 256)} {
			if _, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: name}); errCode(err) != "E_BAD_REQUEST" {
				t.Errorf("SetTaskBranch(%q) = %v, want E_BAD_REQUEST", name, err)
			}
		}
		if got := branchOf(t, board(t, app), task.ID, rec.ID); got.Name != "" {
			t.Fatalf("refused names left name %q", got.Name)
		}
	})

	t.Run("a running session", func(t *testing.T) {
		dir := t.TempDir()
		flag := filepath.Join(dir, "hold")
		app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
		claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}, {WaitFile: flag}}})
		repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
		rec := importRepo(t, app, repo.Dir)
		saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")+
			"  - id: work\n    name: Work\n    kind: manual\n    status: In progress\n    session: true\n"))
		task := createTask(t, app, "Fix login", "flow", rec.ID)
		br := branchOf(t, board(t, app), task.ID, rec.ID)
		if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/api-work"}}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, app, task.ID, "one", "done")
		app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
		launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
		if err != nil {
			t.Fatal(err)
		}
		openLaunch(t, app, launch)
		waitSession(t, app, launch.SessionID, "running")

		br = branchOf(t, board(t, app), task.ID, rec.ID)
		if err := app.W.AdeTask.RemoveTaskBranch(ctx, adewire.RemoveTaskRowArgs{BranchID: br.ID}); errCode(err) != "E_BAD_REQUEST" {
			t.Fatalf("RemoveTaskBranch under a session = %v, want E_BAD_REQUEST", err)
		}
		if _, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: br.ID, Name: "feat/api-other"}); errCode(err) != "E_BAD_REQUEST" {
			t.Fatalf("SetTaskBranch under a session = %v, want E_BAD_REQUEST", err)
		}
		if got := branchOf(t, board(t, app), task.ID, rec.ID); got.ID != br.ID || got.Name != "feat/api-work" {
			t.Fatalf("refused edits changed the row: %+v", got)
		}
		if err := os.WriteFile(flag, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	})
}

// A row with runs is archived, not deleted: its runs stay as history.
func TestTaskBranchDetachKeepsRunHistory(t *testing.T) {
	f, br := doneFixture(t, "feat/api-history")
	if err := f.app.W.AdeTask.RemoveTaskBranch(ctx, adewire.RemoveTaskRowArgs{BranchID: br.ID}); err != nil {
		t.Fatalf("RemoveTaskBranch: %v", err)
	}
	count := func(q string, args ...any) int {
		var n int
		if err := f.app.DB().QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM ade_task_branches WHERE id = ? AND archived_at IS NOT NULL`, br.ID); n != 1 {
		t.Fatalf("detached row with runs: %d archived rows, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM ade_runs WHERE branch_id = ?`, br.ID); n == 0 {
		t.Fatal("runs of the detached row are gone")
	}
	if runs := taskOf(t, board(t, f.app), f.taskID).Runs; len(runs) != 0 {
		t.Fatalf("task wire lists runs of a detached branch: %+v", runs)
	}
	live := branchOf(t, board(t, f.app), f.taskID, f.repoID)
	if live.Name != "" || live.ID == br.ID {
		t.Fatalf("replacement = %+v, want a fresh row without a branch", live)
	}
	if err := f.app.W.AdeTask.RemoveTaskRepo(ctx, adewire.RemoveTaskRowArgs{BranchID: live.ID}); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM ade_task_branches WHERE id = ?`, live.ID); n != 0 {
		t.Fatalf("run-less repo row survives a remove: %d", n)
	}
	if err := f.app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: f.taskID}); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM ade_task_branches WHERE task_id = ?`, f.taskID); n != 1 {
		t.Fatalf("archived task keeps %d rows, want the one with history", n)
	}
}
