package repos

import (
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Restore brings back only the rows archived with the task and refuses a name live elsewhere.
func TestAdeTaskRepo_RestoreTask(t *testing.T) {
	db, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatalf("storage.OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tasks := &AdeTaskRepo{DB: db.DB}
	for _, id := range []string{"r1", "r2"} {
		if _, err := db.DB.Exec(`INSERT INTO code_repos (id, name, root, repo_id, sort_order, created_at) VALUES (?, ?, ?, ?, 0, 0)`, id, id, "/"+id, id); err != nil {
			t.Fatalf("seed repo %s: %v", id, err)
		}
	}
	mk := func(id string, branches ...model.AdeTaskBranch) {
		t.Helper()
		if _, err := tasks.CreateTask(model.AdeTask{ID: id, Kind: model.AdeTaskKindTask, Title: id, CreatedAt: 1}, branches, nil, nil); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	br := func(id, task, repo, name string) model.AdeTaskBranch {
		return model.AdeTaskBranch{ID: id, TaskID: task, CodeRepoID: repo, Name: name, Kind: model.AdeBranchKindMine}
	}
	mk("T", br("b1", "T", "r1", "feat-a"), br("b2", "T", "r2", "feat-b"))
	if err := tasks.DetachBranch("b2", nil, 5); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if err := tasks.ArchiveTask("T", 10); err != nil {
		t.Fatalf("archive: %v", err)
	}
	mk("U", br("u1", "U", "r1", "feat-a"))
	var held *BranchHeldError
	if err := tasks.RestoreTask("T"); !errors.As(err, &held) || held.Branch != "feat-a" || held.TaskTitle != "U" || !errors.Is(err, ErrBranchOnTask) {
		t.Fatalf("restore over live name = %v; want BranchHeldError feat-a/U", err)
	}
	if got, _ := tasks.GetTask("T"); got.ArchivedAt == nil {
		t.Fatal("refused restore changed the task")
	}
	if err := tasks.ArchiveTask("U", 20); err != nil {
		t.Fatalf("archive U: %v", err)
	}
	if err := tasks.RestoreTask("T"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	live, err := tasks.BranchesLive()
	if err != nil || len(live) != 1 || live[0].ID != "b1" {
		t.Fatalf("live branches = %+v, %v; want only b1 (b2 detached earlier)", live, err)
	}
	plan, err := tasks.PlanRows()
	if err != nil || len(plan) != 1 || plan[0].TaskID != "T" || plan[0].Day != nil {
		t.Fatalf("plan = %+v, %v; want T in Later", plan, err)
	}
	if err := tasks.RestoreTask("T"); !errors.Is(err, ErrTaskNotArchived) {
		t.Fatalf("restore live = %v; want ErrTaskNotArchived", err)
	}
	if err := tasks.RestoreTask("nope"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("restore missing = %v; want ErrTaskNotFound", err)
	}
}
