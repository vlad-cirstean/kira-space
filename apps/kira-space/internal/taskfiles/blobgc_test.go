package taskfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

// TestBlobSweepLiveSetAndAge covers the sweep's interacting rules: a blob stays when an accepted task
// file or any proof revision's image step names it, or when it is younger than the cutoff or was
// reused by a Put since; everything else goes.
func TestBlobSweepLiveSetAndAge(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	blobs, err := store.NewBlobs(root)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	put := func(body string, aged bool) string {
		sum, err := blobs.Put([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if aged {
			p := filepath.Join(root, sum[:2], sum)
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
		return sum
	}
	e.seed("notes/20261101T090000-a1.md", "accepted\n")
	file := put("accepted\n", true)
	cur, pend := put("current", true), put("pending", true)
	orphan, fresh, reused := put("orphan", true), put("fresh", false), put("reused", true)
	if _, err := blobs.Put([]byte("reused")); err != nil { // a submit reusing an old blob
		t.Fatal(err)
	}
	cache := repos.ProofCache{Dialect: e.repo.Dialect}
	step := func(sum string) repos.ProofStepRow {
		return repos.ProofStepRow{StepKey: "s1", Description: "d", View: "image", Payload: []byte(fmt.Sprintf(`{"format":"png","sha256":%q}`, sum))}
	}
	if err := cache.Create(e.repo.DB, repos.ProofNew{ID: "p1", TaskID: e.taskID, Slug: "shot", Name: "n", Source: "agent", Steps: []repos.ProofStepRow{step(cur)}, Now: 1}); err != nil {
		t.Fatal(err)
	}
	if err := cache.Propose(e.repo.DB, repos.ProofEdit{SessionID: "p1", Name: "n", Steps: []repos.ProofStepRow{step(pend)}, Now: 2}); err != nil {
		t.Fatal(err)
	}
	live, err := e.repo.LiveBlobKeys()
	if err != nil {
		t.Fatal(err)
	}
	n, err := blobs.Sweep(live, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("removed %d blobs, want 1", n)
	}
	for name, sum := range map[string]string{"task file": file, "current rev": cur, "pending rev": pend, "fresh": fresh, "reused": reused} {
		if _, ok, _ := blobs.Get(sum); !ok {
			t.Errorf("%s blob swept", name)
		}
	}
	if _, ok, _ := blobs.Get(orphan); ok {
		t.Error("old unreferenced blob kept")
	}
}
