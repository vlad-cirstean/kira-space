package adeflow_test

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/ids"
)

func isV7(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u.Version() == 7
}

// Backend half of tests/ui/ade-ids.spec.ts.
func TestAdeIDs(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", ""))
	f.start(t, "feat/ids")
	run := waitRun(t, f.app, f.taskID, "one", "done")
	br := branchOf(t, board(t, f.app), f.taskID, f.repoID)

	for name, id := range map[string]string{"task": f.taskID, "branch": br.ID, "run": run.ID} {
		if !isV7(id) {
			t.Errorf("%s id %q is not a v7 UUID", name, id)
		}
	}
	if !ids.Is(ids.Worktree, filepath.Base(br.Worktree)) {
		t.Errorf("worktree dir %q is not a wt_ handle", filepath.Base(br.Worktree))
	}

	second := createTask(t, f.app, "Second", "flow", f.repoID)
	if second.ID <= f.taskID {
		t.Errorf("task ids %q then %q do not sort in creation order", f.taskID, second.ID)
	}

	// A legacy v4 task (no children yet) lists, renames and archives.
	legacy := createTask(t, f.app, "Legacy", "flow")
	v4 := uuid.NewString()
	tx, err := f.app.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TEMP TABLE legacy_task AS SELECT * FROM ade_tasks WHERE id = '` + legacy.ID + `'`,
		`UPDATE legacy_task SET id = '` + v4 + `'`,
		`INSERT INTO ade_tasks SELECT * FROM legacy_task`,
		`DROP TABLE legacy_task`,
	} {
		if _, err := tx.Exec(q); err != nil {
			t.Fatalf("seed v4 task: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	title := "Legacy renamed"
	if _, err := f.app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: v4, Patch: adewire.TaskPatch{Title: &title}}); err != nil {
		t.Fatalf("rename v4 task: %v", err)
	}
	if got := taskOf(t, board(t, f.app), v4).Title; got != title {
		t.Fatalf("v4 task title = %q, want %q", got, title)
	}
	if err := f.app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: v4}); err != nil {
		t.Fatalf("archive v4 task: %v", err)
	}

	f.app.Contract(t, "ade-ids", "AdeTaskService.Board", board(t, f.app))
}
