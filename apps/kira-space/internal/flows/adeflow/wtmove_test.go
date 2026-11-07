package adeflow_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	claudeharness "github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/ids"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func dbExec(t *testing.T, app *flowharness.App, q string, args ...any) {
	t.Helper()
	if _, err := app.DB().Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func dbString(t *testing.T, app *flowharness.App, q string, args ...any) string {
	t.Helper()
	var s string
	if err := app.DB().QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return s
}

// worktreeRecord returns the `worktree list --porcelain` record that names path, "" when none does.
func worktreeRecord(t *testing.T, repoDir, path string) string {
	t.Helper()
	for _, rec := range strings.Split(gitIn(t, repoDir, "worktree", "list", "--porcelain"), "\n\n") {
		if strings.HasPrefix(rec, "worktree "+path+"\n") || strings.TrimSpace(rec) == "worktree "+path {
			return rec
		}
	}
	return ""
}

func sortedRecords(list string) string {
	recs := strings.Split(strings.TrimSpace(list), "\n\n")
	sort.Strings(recs)
	return strings.Join(recs, "\n\n")
}

// Boot moves task worktrees from <home>/wt to <home>/kira-space-wts with git, keeps their changes and
// locks, rewrites the stored paths, renames Claude's transcript folder and leaves every unsafe case
// alone until a later boot.
func TestWorktreesMoveOnce(t *testing.T) {
	app := flowharness.New(t)
	home := app.Home
	apiRepo := app.NewRepo("api")
	apiRepo.Commit("base", map[string]string{"tracked.txt": "one\n", "other.txt": "two\n"})
	webRepo := app.NewRepo("web")
	webRepo.Commit("base", map[string]string{"tracked.txt": "one\n"})
	api, web := importRepo(t, app, apiRepo.Dir), importRepo(t, app, webRepo.Dir)

	type wt struct{ dir, oldPath, newPath string }
	mk := func(repo *flowharness.Repo, repoName, repoID, title string) wt {
		task := createTask(t, app, title, "", repoID)
		dir := ids.New(ids.Worktree)
		dbExec(t, app, "UPDATE ade_task_branches SET worktree_dir = ? WHERE task_id = ? AND code_repo_id = ?", dir, task.ID, repoID)
		w := wt{dir: dir, oldPath: filepath.Join(home, "wt", repoName, dir), newPath: filepath.Join(home, ade.WorktreesDir, repoName, dir)}
		if err := os.MkdirAll(filepath.Dir(w.oldPath), 0o755); err != nil {
			t.Fatal(err)
		}
		repo.Git("worktree", "add", "-q", "-b", "feat-"+dir, w.oldPath)
		return w
	}
	a, b, c, d := mk(apiRepo, "api", api.ID, "a"), mk(apiRepo, "api", api.ID, "b"), mk(apiRepo, "api", api.ID, "c"), mk(apiRepo, "api", api.ID, "d")
	e := mk(webRepo, "web", web.ID, "e")

	if err := os.WriteFile(filepath.Join(a.oldPath, "tracked.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.oldPath, "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.oldPath, "staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, a.oldPath, "add", "staged.txt")
	wantStatus := gitIn(t, a.oldPath, "status", "--porcelain")
	apiRepo.Git("worktree", "lock", "--reason", "usb", b.oldPath)
	if err := os.RemoveAll(c.oldPath); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(home, "wt", "other", "notes.txt")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(d.newPath, "blocker.txt")
	if err := os.MkdirAll(d.newPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	task := dbString(t, app, "SELECT task_id FROM ade_task_branches WHERE worktree_dir = ?", a.dir)
	dbExec(t, app, "INSERT INTO ade_sessions (id, task_id, state, started_at, last_active_at, agent_session_id, cwd) VALUES ('s1', ?, 'stopped', 1, 1, 'x', ?)", task, a.oldPath)
	dbExec(t, app, "INSERT OR IGNORE INTO windows (key, sort_order) VALUES ('w1', 0)")
	dbExec(t, app, "INSERT INTO tabs (id, window_key, sort_order, kind, path, state_json) VALUES ('t1', 'w1', 0, 'terminal', '', json_object('cwd', ?))", a.oldPath+"/sub")
	dbExec(t, app, "INSERT INTO tabs (id, window_key, sort_order, kind, path, state_json) VALUES ('t2', 'w1', 1, 'local-file', ?, '{}')", a.oldPath+"/tracked.txt")
	dbExec(t, app, "INSERT INTO script_runs (id, trigger_kind, state, created_at, script_id, script_name, cwd) VALUES ('r1', 'manual', 'done', 1, 's', 's', ?)", b.oldPath)
	transcript := filepath.Join(home, ".claude", "projects", claudeharness.ProjectDirName(a.oldPath), "s.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app.Restart()

	for _, w := range []wt{a, b, e} {
		if _, err := os.Stat(w.oldPath); err == nil {
			t.Fatalf("%s still at the old path", w.dir)
		}
	}
	if got := gitIn(t, a.newPath, "status", "--porcelain"); got != wantStatus {
		t.Fatalf("status after move = %q, want %q", got, wantStatus)
	}
	if worktreeRecord(t, apiRepo.Dir, a.newPath) == "" || worktreeRecord(t, webRepo.Dir, e.newPath) == "" {
		t.Fatal("moved worktrees are not listed at the new path")
	}
	if rec := worktreeRecord(t, apiRepo.Dir, b.newPath); !strings.Contains(rec, "locked usb") {
		t.Fatalf("worktree b lost its lock or reason: %q", rec)
	}
	if worktreeRecord(t, apiRepo.Dir, c.oldPath) == "" || worktreeRecord(t, apiRepo.Dir, d.oldPath) == "" {
		t.Fatal("missing and blocked worktrees must stay at the old path")
	}
	if _, err := os.Stat(blocker); err != nil {
		t.Fatalf("blocker overwritten: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign file removed: %v", err)
	}
	if got := dbString(t, app, "SELECT cwd FROM ade_sessions WHERE id = 's1'"); got != a.newPath {
		t.Fatalf("session cwd = %q, want %q", got, a.newPath)
	}
	if got := dbString(t, app, "SELECT json_extract(state_json, '$.cwd') FROM tabs WHERE id = 't1'"); got != a.newPath+"/sub" {
		t.Fatalf("terminal tab cwd = %q", got)
	}
	if got := dbString(t, app, "SELECT path FROM tabs WHERE id = 't2'"); got != a.newPath+"/tracked.txt" {
		t.Fatalf("local-file tab path = %q", got)
	}
	if got := dbString(t, app, "SELECT cwd FROM script_runs WHERE id = 'r1'"); got != b.newPath {
		t.Fatalf("script run cwd = %q, want %q", got, b.newPath)
	}
	moved := filepath.Join(home, ".claude", "projects", claudeharness.ProjectDirName(a.newPath), "s.jsonl")
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("transcript not renamed: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(transcript)); err == nil {
		t.Fatal("old transcript folder still present")
	}

	// Second boot with the blocker gone: d moves, nothing else changes.
	if err := os.RemoveAll(d.newPath); err != nil {
		t.Fatal(err)
	}
	before := gitIn(t, apiRepo.Dir, "worktree", "list", "--porcelain")
	app.Restart()
	if worktreeRecord(t, apiRepo.Dir, d.newPath) == "" {
		t.Fatal("d did not move once the blocker was gone")
	}
	after := gitIn(t, apiRepo.Dir, "worktree", "list", "--porcelain")
	if sortedRecords(strings.ReplaceAll(before, d.oldPath, d.newPath)) != sortedRecords(after) {
		t.Fatalf("second boot changed more than d:\nbefore %s\nafter %s", before, after)
	}
	if got := dbString(t, app, "SELECT cwd FROM ade_sessions WHERE id = 's1'"); got != a.newPath {
		t.Fatalf("session cwd changed on rerun: %q", got)
	}

	// Nothing left to move: the old folder goes away.
	if err := os.RemoveAll(filepath.Join(home, "wt", "other")); err != nil {
		t.Fatal(err)
	}
	apiRepo.Git("worktree", "prune")
	app.Restart()
	if _, err := os.Stat(filepath.Join(home, "wt")); err == nil {
		t.Fatal("old wt folder survives with nothing in it")
	}
}
