package adeflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// A repo whose git is busy past the board deadline shows its last good facts with updating set; the
// other repos keep their facts. Releasing the repo clears the flag.
func TestRepoUpdatingWhileGitBusy(t *testing.T) {
	app := flowharness.New(t)
	slow := app.NewRepo("slow")
	slow.Commit("base", map[string]string{"a.txt": "a\n"})
	slow.Branch("feat-slow", "")
	fast := app.NewRepo("fast")
	fast.Commit("base", map[string]string{"a.txt": "a\n"})
	fast.Branch("feat-fast", "")
	slowID, fastID := importRepo(t, app, slow.Dir).ID, importRepo(t, app, fast.Dir).ID
	add := func(repoID, name string) {
		t.Helper()
		if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: repoID, Name: name}); err != nil {
			t.Fatalf("AddExistingBranch %s: %v", name, err)
		}
	}
	add(slowID, "feat-slow")
	add(fastID, "feat-fast")
	warm := board(t, app)
	if st, _ := repoState(warm, slowID); st.Updating {
		t.Fatalf("warm board reports updating: %+v", st)
	}
	fastBefore := branchOf(t, warm, taskIDOfBranch(t, warm, "feat-fast"), fastID)

	entry, done, err := app.W.Git.Registry().Acquire(ctx, "git", slow.Dir)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	held, release := make(chan struct{}), make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- entry.Repo.Write(context.Background(), func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()

	// Any board write drops the snapshot; the next read recomputes both repos.
	fast.Branch("feat-fast-2", "")
	add(fastID, "feat-fast-2")
	start := time.Now()
	b := board(t, app)
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("board took %v with one repo busy", took)
	}
	st, _ := repoState(b, slowID)
	if !st.Updating {
		t.Fatalf("busy repo state = %+v, want updating", st)
	}
	if other, _ := repoState(b, fastID); other.Updating {
		t.Fatalf("idle repo reports updating: %+v", other)
	}
	if got := branchOf(t, b, taskIDOfBranch(t, b, "feat-fast"), fastID); got.Tip != fastBefore.Tip || got.Base != fastBefore.Base {
		t.Fatalf("idle repo facts changed: %+v vs %+v", got, fastBefore)
	}
	app.Contract(t, "ade-repo-updating", "AdeTaskService.Board#repo-updating", st)

	close(release)
	released = true
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		st, _ := repoState(board(t, app), slowID)
		return !st.Updating
	})
}

func taskIDOfBranch(t *testing.T, b adewire.Board, name string) string {
	t.Helper()
	for _, br := range b.Branches {
		if br.Name == name {
			return br.TaskID
		}
	}
	t.Fatalf("board has no branch %s", name)
	return ""
}
