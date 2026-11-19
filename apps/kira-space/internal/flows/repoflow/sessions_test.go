package repoflow_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

func TestHeadsAndWorktreeLinks(t *testing.T) {
	app := flowharness.New(t)
	cw := app.W.CodeWorkspace

	a := app.NewRepo("alpha")
	a.Commit("init", map[string]string{"a.txt": "a\n"})
	a.Branch("topic", "")
	wt1 := a.LinkedWorktree("wt-one")
	wt2 := a.LinkedWorktree("wt-two")
	b := app.NewRepo("beta")
	b.Commit("init", map[string]string{"b.txt": "b\n"})

	// Import a worktree first so the anchor rule (main worktree wins over sort order) is exercised.
	recWt1 := importVia(t, app, wt1.Dir)
	recA := importVia(t, app, a.Dir)
	recWt2 := importVia(t, app, wt2.Dir)
	recB := importVia(t, app, b.Dir)

	headOf := func(id string) (string, string) {
		t.Helper()
		heads, err := cw.RepoHeads(ctx, bridge.CodeRepoHeadsArgs{IDs: []string{id}})
		if err != nil || len(heads) != 1 || heads[0].Head == nil {
			t.Fatalf("RepoHeads(%s) = %+v, %v", id, heads, err)
		}
		return heads[0].Head.Kind, heads[0].Head.Name
	}
	if k, n := headOf(recWt1.ID); k != "branch" || n != "wt-one" {
		t.Fatalf("wt-one head = %s %s", k, n)
	}
	wt2.Checkout("topic")
	if k, n := headOf(recWt2.ID); k != "branch" || n != "topic" {
		t.Fatalf("after checkout in worktree head = %s %s, want topic", k, n)
	}
	if k, n := headOf(recA.ID); k != "branch" || n != "main" {
		t.Fatalf("main worktree head moved: %s %s", k, n)
	}
	b.Git("checkout", "-q", "--detach")
	if k, _ := headOf(recB.ID); k != "detached" {
		t.Fatalf("beta head kind = %s, want detached", k)
	}

	all, err := cw.RepoHeads(ctx, bridge.CodeRepoHeadsArgs{})
	if err != nil || len(all) != 4 {
		t.Fatalf("RepoHeads(all) = %d rows, %v", len(all), err)
	}

	links, err := cw.RepoWorktreeLinks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parent := map[string]string{}
	for _, l := range links {
		if l.Error != "" {
			t.Errorf("link %s error %s", l.ID, l.Error)
		}
		parent[l.ID] = l.ParentID
	}
	if parent[recA.ID] != "" || parent[recB.ID] != "" || parent[recWt1.ID] != recA.ID || parent[recWt2.ID] != recA.ID {
		t.Fatalf("links = %v, want both worktrees under %s and the others top-level", parent, recA.ID)
	}

	t.Run("deleted worktree dir degrades to a row error", func(t *testing.T) {
		if err := os.RemoveAll(wt1.Dir); err != nil {
			t.Fatal(err)
		}
		heads, err := cw.RepoHeads(ctx, bridge.CodeRepoHeadsArgs{IDs: []string{recWt1.ID}})
		if err != nil || len(heads) != 1 || heads[0].Head != nil || heads[0].Error == "" {
			t.Fatalf("RepoHeads = %+v, %v, want one row with an error", heads, err)
		}
		links, err := cw.RepoWorktreeLinks(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range links {
			if l.ID == recWt1.ID && (l.ParentID != "" || l.Error == "") {
				t.Errorf("gone worktree link = %+v, want top-level with error", l)
			}
		}
	})
}

// catFileCount counts running `git cat-file` processes whose working directory is under root.
func catFileCount(t *testing.T, root string) int {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Skip("no /proc")
	}
	n := 0
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		cmd, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil || !strings.Contains(string(cmd), "cat-file") {
			continue
		}
		cwd, err := os.Readlink(filepath.Join("/proc", e.Name(), "cwd"))
		if err == nil && strings.HasPrefix(cwd, root) {
			n++
		}
	}
	return n
}

func TestRemoveRepoClosesSessions(t *testing.T) {
	app := flowharness.New(t)
	cw := app.W.CodeWorkspace
	r := app.NewRepo("doomed")
	r.Commit("init", map[string]string{"a.txt": "needle\n"})
	root := real(t, r.Dir)
	rec := importVia(t, app, r.Dir)

	if catFileCount(t, root) != 0 {
		t.Fatal("cat-file running before any workspace opened")
	}
	if err := cw.OpenWorkspace(ctx, idArgs(rec.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := cw.ReadDiff(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "a.txt"}); err != nil {
		t.Fatal(err)
	}
	if catFileCount(t, root) == 0 {
		t.Log("no cat-file process observed while the workspace was open")
	}

	if err := cw.RemoveRepo(idArgs(rec.ID)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return catFileCount(t, root) == 0 })

	if _, err := cw.ReadFile(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "a.txt"}); errCode(err) != "E_NOT_FOUND" {
		t.Fatalf("ReadFile after remove = %v, want E_NOT_FOUND", err)
	}
	if err := cw.OpenWorkspace(ctx, idArgs(rec.ID)); errCode(err) != "E_NOT_FOUND" {
		t.Fatalf("OpenWorkspace after remove = %v, want E_NOT_FOUND", err)
	}
	if list, _ := cw.ListRepos(); len(list) != 0 {
		t.Fatalf("ListRepos = %+v, want empty", list)
	}
	// The repo itself is untouched.
	if _, err := os.Stat(filepath.Join(r.Dir, ".git")); err != nil {
		t.Fatal(err)
	}
}
