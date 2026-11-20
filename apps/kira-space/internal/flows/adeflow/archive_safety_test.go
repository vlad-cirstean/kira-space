package adeflow_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// J-ADE-041, J-ADE-003. Contract ade-archive-safety: tests/ui/ade-v2-archive.spec.ts reads it.
func TestArchiveSafetyDirtyUnmergedLocked(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "a"))
	rec := importRepo(t, app, repo.Dir)
	for _, n := range []string{"feat/clean", "feat/dirty", "feat/locked"} {
		gitOut(t, repo.Dir, "branch", n)
	}
	task := createTask(t, app, "Three trees", "")
	for _, n := range []string{"feat/clean", "feat/dirty", "feat/locked"} {
		if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: rec.ID, Name: n, TaskID: task.ID}); err != nil {
			t.Fatalf("AddExistingBranch %s: %v", n, err)
		}
	}
	trees := map[string]adewire.Branch{}
	for _, b := range branchesOf(t, app, task.ID) {
		trees[b.Name] = b
	}
	dirtyDir, lockedDir, cleanDir := trees["feat/dirty"].Worktree, trees["feat/locked"].Worktree, trees["feat/clean"].Worktree
	commitIn(t, dirtyDir, "u1.txt")
	commitIn(t, dirtyDir, "u2.txt")
	tip := gitOut(t, dirtyDir, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(dirtyDir, "u1.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirtyDir, "untracked.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, repo.Dir, "worktree", "lock", lockedDir)

	args := adewire.TaskArgs{TaskID: task.ID}
	risk, err := app.W.AdeTask.ArchiveRisk(ctx, args)
	if err != nil || len(risk.Branches) != 3 {
		t.Fatalf("ArchiveRisk = %+v, %v, want 3 branches", risk, err)
	}
	byTree := map[string]adewire.BranchRisk{}
	for _, r := range risk.Branches {
		byTree[r.Worktree] = r
	}
	if d := byTree[dirtyDir]; len(d.Dirty) != 2 || d.Unmerged != 2 || d.Blocked != "" {
		t.Fatalf("dirty risk = %+v, want 2 dirty files and 2 unmerged", d)
	}
	if l := byTree[lockedDir]; l.Blocked == "" {
		t.Fatalf("locked risk = %+v, want a blocker", l)
	}
	if c := byTree[cleanDir]; len(c.Dirty) != 0 || c.Unmerged != 0 || c.Blocked != "" {
		t.Fatalf("clean risk = %+v, want nothing at risk", c)
	}
	app.Contract(t, "ade-archive-safety", "AdeTaskService.ArchiveRisk#mixed", risk, flowharness.Mask("taskId", "branchId", "worktree"))

	// Locked worktree refuses the whole archive: nothing stopped or removed, row stays live.
	err = app.W.AdeTask.ArchiveTask(ctx, args)
	if err == nil || !strings.Contains(err.Error(), "cannot archive") || !strings.Contains(err.Error(), "feat/locked") {
		t.Fatalf("ArchiveTask with a locked worktree = %v, want a refusal naming feat/locked", err)
	}
	app.Contract(t, "ade-archive-safety", "error:AdeTaskService.ArchiveTask#locked", err.Error())
	for _, d := range []string{dirtyDir, lockedDir, cleanDir} {
		if _, err := os.Stat(d); err != nil {
			t.Fatalf("worktree %s lost by a refused archive: %v", d, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(dirtyDir, "untracked.txt")); err != nil || string(got) != "wip\n" {
		t.Fatalf("untracked file after refused archive = %q, %v", got, err)
	}
	if len(board(t, app).History) != 0 {
		t.Fatal("task archived by a refused archive")
	}

	// Unlock, archive again: all folders go, branch refs and unmerged commits stay.
	gitOut(t, repo.Dir, "worktree", "unlock", lockedDir)
	if err := app.W.AdeTask.ArchiveTask(ctx, args); err != nil {
		t.Fatalf("ArchiveTask after unlock: %v", err)
	}
	for _, d := range []string{dirtyDir, lockedDir, cleanDir} {
		gone(t, d)
	}
	if got := gitOut(t, repo.Dir, "rev-parse", "feat/dirty"); got != tip {
		t.Fatalf("feat/dirty = %s after archive, want the unmerged tip %s", got, tip)
	}
	for _, n := range []string{"feat/clean", "feat/dirty", "feat/locked"} {
		if gitOut(t, repo.Dir, "branch", "--list", n) == "" {
			t.Fatalf("branch ref %s deleted by archive", n)
		}
	}
	if wt := gitOut(t, repo.Dir, "worktree", "list", "--porcelain"); strings.Contains(wt, dirtyDir) || strings.Contains(wt, lockedDir) {
		t.Fatalf("git still lists archived worktrees:\n%s", wt)
	}
	if _, err := app.W.AdeTask.ArchivedTask(ctx, args); err != nil {
		t.Fatalf("ArchivedTask: %v", err)
	}
	if _, err := app.W.AdeTask.RestoreTask(ctx, args); err != nil {
		t.Fatalf("RestoreTask: %v", err)
	}
	for _, b := range branchesOf(t, app, task.ID) {
		if b.Worktree != "" {
			t.Fatalf("restored branch %+v recreated a worktree before start", b)
		}
	}
}

// J-ADE-042. Archive with a live headless run waiting on an adopt request: the run ends cancelled,
// the request is abandoned and no session stays running.
func TestArchiveWhileRunAndRequestLive(t *testing.T) {
	f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{adoptCall("feat/a")}, WaitFile: filepath.Join(t.TempDir(), "never")}},
		onceStepYML, false)
	f.donorHolds(t, "feat/a")
	f.startRun(t)
	req := f.waitRequest(t)
	if len(req.Items) != 1 {
		t.Fatalf("request = %+v", req)
	}
	run := waitRun(t, f.app, f.task.ID, "one", "running")

	args := adewire.TaskArgs{TaskID: f.task.ID}
	if err := f.app.W.AdeTask.ArchiveTask(ctx, args); err != nil {
		t.Fatalf("ArchiveTask with live work: %v", err)
	}
	if _, err := branchRequest(f.app, req.ID); errCode(err) != "E_NOT_FOUND" {
		t.Fatalf("BranchRequest after archive = %v, want E_NOT_FOUND", err)
	}
	if ps := f.requestPrompts(t); len(ps) != 0 {
		t.Fatalf("approval prompts after archive = %+v, want none", ps)
	}
	res, err := f.app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Sessions {
		if s.TaskID == f.task.ID && s.State == "running" {
			t.Fatalf("session %+v still running after archive", s)
		}
	}
	got, err := f.app.W.AdeTask.ArchivedTask(ctx, args)
	if err != nil {
		t.Fatalf("ArchivedTask: %v", err)
	}
	var found bool
	for _, r := range got.Task.Runs {
		if r.ID == run.ID {
			found = true
			if r.State == "running" || r.State == "queued" {
				t.Fatalf("run %+v non-terminal after archive", r)
			}
		}
	}
	if !found {
		t.Fatalf("archived runs = %+v, want %s", got.Task.Runs, run.ID)
	}
	// Archiving twice never sets archived_at again.
	if err := f.app.W.AdeTask.ArchiveTask(ctx, args); err == nil {
		t.Fatal("second ArchiveTask accepted")
	}
	// A start after archive is refused.
	if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID}); err == nil {
		t.Fatal("StartRun of an archived task accepted")
	}
}

// J-ADE-043. Restore while the branch is checked out elsewhere never yields two worktrees on one branch.
func TestRestoreBranchCheckedOutElsewhere(t *testing.T) {
	f := newRunFixtureWith(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", ""), flowharness.WithTrackerGrace(time.Second))
	app := f.app
	f.start(t, "feat/x")
	waitRun(t, app, f.taskID, "one", "done")
	args := adewire.TaskArgs{TaskID: f.taskID}
	if err := app.W.AdeTask.ArchiveTask(ctx, args); err != nil {
		t.Fatal(err)
	}
	// The repo's main checkout takes the branch while the task sleeps in the archive.
	gitOut(t, f.repo.Dir, "checkout", "-q", "feat/x")
	if _, err := app.W.AdeTask.RestoreTask(ctx, args); err != nil {
		t.Fatalf("RestoreTask with the branch checked out at the repo root: %v", err)
	}
	// Starting must not create a second worktree on the branch: it succeeds on the root or refuses.
	_, startErr := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.taskID})
	if startErr == nil {
		waitRun(t, app, f.taskID, "one", "done", "failed")
	}
	on := 0
	for _, blk := range strings.Split(gitOut(t, f.repo.Dir, "worktree", "list", "--porcelain"), "\n\n") {
		if strings.Contains(blk, "branch refs/heads/feat/x") {
			on++
		}
	}
	if on != 1 {
		t.Fatalf("worktrees on feat/x = %d, want exactly 1 (start err %v):\n%s", on, startErr, gitOut(t, f.repo.Dir, "worktree", "list"))
	}
}

// J-BC-027. 2,000 archived tasks: the history wire lists each exactly once, newest first, with the
// title and Jira key the finder matches on. Oldest restores. Restart keeps all.
func TestArchivedHistoryAtScale(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "a"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", manualStep))
	live := createTask(t, app, "Template", "flow", rec.ID)
	const total = 2000
	tx, err := app.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TEMP TABLE seed_task AS SELECT * FROM ade_tasks WHERE id = '` + live.ID + `'`,
	} {
		if _, err := tx.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	ids := make([]string, total)
	for i := 0; i < total; i++ {
		ids[i] = fmt.Sprintf("00000000-0000-7000-8000-%012d", i)
		if _, err := tx.Exec(`UPDATE seed_task SET id = ?, title = ?, jira_key = ?, archived_at = ?`,
			ids[i], fmt.Sprintf("Archived task %04d", i), fmt.Sprintf("KIRA-%d", 1000+i), 1_700_000_000_000+int64(i)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO ade_tasks SELECT * FROM seed_task`); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	if _, err := tx.Exec(`DROP TABLE seed_task`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	app.Restart()

	check := func(label string) {
		hist := board(t, app).History
		if len(hist) != total {
			t.Fatalf("%s: history = %d entries, want %d", label, len(hist), total)
		}
		seen := map[string]bool{}
		for i, e := range hist {
			if seen[e.TaskID] {
				t.Fatalf("%s: task %s listed twice", label, e.TaskID)
			}
			seen[e.TaskID] = true
			if i > 0 && hist[i-1].ArchivedAt < e.ArchivedAt {
				t.Fatalf("%s: history not newest first at %d", label, i)
			}
		}
		byKey := map[string]string{}
		for _, e := range hist {
			byKey[e.JiraKey] = e.Title
		}
		if byKey["KIRA-1042"] != "Archived task 0042" || byKey["KIRA-2999"] != "Archived task 1999" {
			t.Fatalf("%s: jira keys not on the wire: %v", label, byKey["KIRA-1042"])
		}
	}
	check("seeded")
	oldest := ids[0]
	if _, err := app.W.AdeTask.ArchivedTask(ctx, adewire.TaskArgs{TaskID: oldest}); err != nil {
		t.Fatalf("open oldest: %v", err)
	}
	if _, err := app.W.AdeTask.RestoreTask(ctx, adewire.TaskArgs{TaskID: oldest}); err != nil {
		t.Fatalf("restore oldest: %v", err)
	}
	if n := len(board(t, app).History); n != total-1 {
		t.Fatalf("history after restore = %d, want %d", n, total-1)
	}
	if got := taskOf(t, board(t, app), oldest).Title; got != "Archived task 0000" {
		t.Fatalf("restored title = %q", got)
	}
}
