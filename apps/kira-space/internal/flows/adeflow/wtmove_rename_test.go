package adeflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	claudeharness "github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// A repo renamed since the worktree was made still moves, found by its folder name. A rebase run's
// stack paths are rewritten. A transcript folder already at the target is merged, not skipped.
func TestWorktreesMoveAfterRename(t *testing.T) {
	app := flowharness.New(t)
	home := app.Home
	repo := app.NewRepo("api")
	repo.Commit("base", map[string]string{"tracked.txt": "one\n"})
	imp := importRepo(t, app, repo.Dir)
	task := createTask(t, app, "a", "", imp.ID)
	dir := ids.New(ids.Worktree)
	dbExec(t, app, "UPDATE ade_task_branches SET worktree_dir = ? WHERE task_id = ? AND code_repo_id = ?", dir, task.ID, imp.ID)
	oldPath := filepath.Join(home, "wt", "api", dir)
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	repo.Git("worktree", "add", "-q", "-b", "feat-"+dir, oldPath)
	dbExec(t, app, "UPDATE code_repos SET name = 'renamed' WHERE id = ?", imp.ID)
	newPath := filepath.Join(home, ade.WorktreesDir, "renamed", dir)

	spec := `{"stack":[{"branchId":"b","worktree":"` + oldPath + `","before":"x"},{"branchId":"c","worktree":"/elsewhere"}],"push":true}`
	dbExec(t, app, "INSERT INTO ade_runs (id, task_id, state, purpose, attempt, step_id, spec_json) VALUES ('run1', ?, 'stuck', 'rebase', 1, 'rebase', ?)", task.ID, spec)
	projects := filepath.Join(home, ".claude", "projects")
	for _, f := range []string{
		filepath.Join(projects, claudeharness.ProjectDirName(oldPath), "old.jsonl"),
		filepath.Join(projects, claudeharness.ProjectDirName(newPath), "new.jsonl"),
	} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	app.Restart()

	if worktreeRecord(t, repo.Dir, newPath) == "" {
		t.Fatal("renamed repo's worktree did not move")
	}
	got := dbString(t, app, "SELECT json_extract(spec_json, '$.stack[0].worktree') || '|' || json_extract(spec_json, '$.stack[1].worktree') || '|' || json_extract(spec_json, '$.stack[0].before') || '|' || json_extract(spec_json, '$.push') FROM ade_runs WHERE id = 'run1'")
	if want := newPath + "|/elsewhere|x|1"; got != want {
		t.Fatalf("spec = %q, want %q", got, want)
	}
	for _, f := range []string{"old.jsonl", "new.jsonl"} {
		if _, err := os.Stat(filepath.Join(projects, claudeharness.ProjectDirName(newPath), f)); err != nil {
			t.Fatalf("transcript %s missing after merge: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "wt")); err == nil {
		t.Fatal("old wt folder survives with nothing in it")
	}
}
