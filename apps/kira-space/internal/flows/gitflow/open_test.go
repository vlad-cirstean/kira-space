package gitflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpath"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
)

// Guards the P230 class: an empty git.gitPath must reach exec as a real absolute path.
func TestOpenDefaultGitPath(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("first", map[string]string{"a.txt": "a\n"})

	init := call[gitrpc.AppInitResult](t, r.gs, "app.init", nil)
	if init.Git.Kind != "ok" || !filepath.IsAbs(init.Git.Path) {
		t.Fatalf("app.init git = %+v, want ok with an absolute path", init.Git)
	}
	sum := r.open(repo.Dir)
	if sum.RepoID == "" || sum.Root == "" {
		t.Fatalf("repo.open summary = %+v, want repoId and root", sum)
	}
	wantRoot, _ := filepath.EvalSymlinks(repo.Dir)
	if gotRoot, _ := filepath.EvalSymlinks(sum.Root); gotRoot != wantRoot {
		t.Fatalf("root = %q, want %q", sum.Root, repo.Dir)
	}
	if sum.Head.Kind != "branch" || sum.Head.Name != "main" {
		t.Fatalf("head = %+v, want branch main", sum.Head)
	}
	if err := r.gs.Request("repo.close", gitrpc.RepoCloseParams{RepoID: sum.RepoID}, nil); err != nil {
		t.Fatalf("repo.close: %v", err)
	}
}

// Guards the P230 C3 class: every repo shape reports the root and common dir git itself reports.
func TestOpenShapes(t *testing.T) {
	r := newRig(t)
	main := r.app.NewRepo("main")
	main.Commit("first", map[string]string{"sub/dir/a.txt": "a\n"})
	linked := main.LinkedWorktree("side")
	nfd := r.app.NewRepo(flowharness.NFDName)
	nfd.Commit("first", map[string]string{"a.txt": "a\n"})
	bare := r.app.NewBare("remote")

	resolve := func(p string) string {
		got, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	want := func(repo *flowharness.Repo, args ...string) string {
		return resolve(strings.TrimSpace(repo.Git(append([]string{"rev-parse", "--path-format=absolute"}, args...)...)))
	}

	t.Run("not a repository", func(t *testing.T) {
		dir := filepath.Join(r.app.Work, "plain")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		res := call[gitrpc.RepoOpenResult](t, r.gs, "repo.open", gitrpc.RepoOpenParams{Path: dir})
		if res.Kind != "notARepository" {
			t.Fatalf("kind = %q, want notARepository", res.Kind)
		}
	})
	t.Run("subdirectory resolves to the root", func(t *testing.T) {
		sum := r.open(filepath.Join(main.Dir, "sub", "dir"))
		if resolve(sum.Root) != want(main, "--show-toplevel") {
			t.Fatalf("root = %q, want %q", sum.Root, main.Dir)
		}
		if resolve(sum.CommonDir) != want(main, "--git-common-dir") {
			t.Fatalf("commonDir = %q", sum.CommonDir)
		}
	})
	t.Run("linked worktree", func(t *testing.T) {
		sum := r.open(linked.Dir)
		if !sum.IsLinkedWorktree {
			t.Fatalf("isLinkedWorktree = false for %s", linked.Dir)
		}
		if resolve(sum.Root) != want(linked, "--show-toplevel") {
			t.Fatalf("root = %q, want %q", sum.Root, linked.Dir)
		}
		if resolve(sum.CommonDir) != want(linked, "--git-common-dir") {
			t.Fatalf("commonDir = %q", sum.CommonDir)
		}
		if sum.Head.Kind != "branch" || sum.Head.Name != "side" {
			t.Fatalf("head = %+v, want branch side", sum.Head)
		}
	})
	t.Run("bare repository", func(t *testing.T) {
		sum := r.open(bare.Dir)
		if !sum.IsBare {
			t.Fatalf("isBare = false for %s", bare.Dir)
		}
		if sum.Head.Kind != "unborn" {
			t.Fatalf("head = %+v, want unborn", sum.Head)
		}
	})
	t.Run("NFD directory name", func(t *testing.T) {
		if _, err := os.Stat(gitpath.NFC(nfd.Dir)); err != nil {
			t.Skip("file system is not normalization-insensitive (macOS only): the NFC spelling of the path does not exist")
		}
		sum := r.open(nfd.Dir)
		if resolve(sum.Root) != want(nfd, "--show-toplevel") {
			t.Fatalf("root = %q, want %q", sum.Root, nfd.Dir)
		}
		if sum.Head.Name != "main" {
			t.Fatalf("head = %+v", sum.Head)
		}
	})
}
