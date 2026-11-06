package repos

import (
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestAdeTaskRepo_DetachBranch(t *testing.T) {
	br := func(id, repo, name, kind string, pos int) model.AdeTaskBranch {
		return model.AdeTaskBranch{ID: id, TaskID: "T", CodeRepoID: repo, Name: name, Kind: kind, Position: pos, AddedAt: 1}
	}
	tests := []struct {
		name      string
		detach    string
		withRun   bool
		replace   *model.AdeTaskBranch
		wantRow   bool // old row still in the table (archived)
		wantStack string
	}{
		{name: "never created and run-less is deleted", detach: "d"},
		{name: "created is archived and dependents fall back to its name", detach: "c", wantRow: true, wantStack: "feat/c"},
		{name: "never created with a run is archived", detach: "d", withRun: true, wantRow: true},
		{name: "replacement takes the position", detach: "d", replace: ptr(br("n", "r3", "", model.AdeBranchKindMine, 9))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := storage.OpenAt(t.TempDir())
			if err != nil {
				t.Fatalf("storage.OpenAt: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			for _, id := range []string{"r1", "r2", "r3"} {
				if _, err := db.DB.Exec(`INSERT INTO code_repos (id, name, root, repo_id, sort_order, created_at) VALUES (?, ?, ?, ?, 0, '')`, id, id, "/"+id, id); err != nil {
					t.Fatalf("code repo: %v", err)
				}
			}
			tasks := &AdeTaskRepo{DB: db.DB}
			c := br("c", "r1", "feat/c", model.AdeBranchKindMine, 0)
			d := br("d", "r2", "", model.AdeBranchKindMine, 1)
			stacked := br("s", "r1", "", model.AdeBranchKindMine, 2)
			stacked.BaseBranchID, stacked.Base = tc.detach, "ignored"
			queued := br("q", "r2", "", model.AdeBranchKindMine, 3)
			queued.QueuedAfter = tc.detach
			if _, err := tasks.CreateTask(model.AdeTask{ID: "T", Kind: model.AdeTaskKindTask, Title: "t", CreatedAt: 1}, []model.AdeTaskBranch{c, d, stacked, queued}, nil, nil); err != nil {
				t.Fatalf("create: %v", err)
			}
			if tc.withRun {
				if err := tasks.InsertRun(model.AdeRun{ID: "run", TaskID: "T", BranchID: "d", Attempt: 1, State: model.AdeRunDone}); err != nil {
					t.Fatalf("run: %v", err)
				}
			}
			if err := tasks.DetachBranch(tc.detach, tc.replace, 5); err != nil {
				t.Fatalf("DetachBranch: %v", err)
			}
			got, err := tasks.GetBranch(tc.detach)
			switch {
			case tc.wantRow && (err != nil || got.ArchivedAt == nil):
				t.Fatalf("old row = %+v, %v; want archived", got, err)
			case !tc.wantRow && !errors.Is(err, ErrBranchMissing):
				t.Fatalf("old row err = %v; want deleted", err)
			}
			s, _ := tasks.GetBranch("s")
			if s.BaseBranchID != "" || (tc.wantStack != "" && s.Base != tc.wantStack) {
				t.Fatalf("stacked = %+v; want base %q and no base branch", s, tc.wantStack)
			}
			if q, _ := tasks.GetBranch("q"); q.QueuedAfter != "" {
				t.Fatalf("queued after = %q; want freed", q.QueuedAfter)
			}
			if tc.replace != nil {
				n, err := tasks.GetBranch("n")
				if err != nil || n.Position != 1 || n.ArchivedAt != nil {
					t.Fatalf("replacement = %+v, %v; want live at position 1", n, err)
				}
			}
		})
	}
}

func TestAdeTaskRepo_DetachBranch_missing(t *testing.T) {
	db, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatalf("storage.OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := (&AdeTaskRepo{DB: db.DB}).DetachBranch("nope", nil, 1); !errors.Is(err, ErrBranchMissing) {
		t.Fatalf("err = %v; want ErrBranchMissing", err)
	}
}

func ptr[T any](v T) *T { return &v }
