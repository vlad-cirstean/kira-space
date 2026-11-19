package repoflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

func TestBrowseReadDiff(t *testing.T) {
	app := flowharness.New(t)
	r := app.NewRepo("browse")
	r.Commit("init", map[string]string{
		".gitignore":       "build/\n*.log\n",
		"src/main.go":      "package main\n",
		"src/deep/a/b.txt": "deep\n",
		"README.md":        "# hi\n",
	})
	r.WriteBinary("blob.bin", 4096)
	r.Git("add", "blob.bin")
	r.Commit("binary", nil)
	// Local state: modified, untracked, ignored, oversized.
	r.Write("src/main.go", "package main\n\nfunc main() {}\n")
	r.Write("new.txt", "untracked\n")
	r.Write("build/out.o", "x")
	r.Write("debug.log", "x")
	big := make([]byte, codeworkspace.MaxReadBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	mustWrite(t, filepath.Join(r.Dir, "big.txt"), big)

	outside := filepath.Join(app.Work, "outside.txt")
	mustWrite(t, outside, []byte("secret\n"))
	if err := os.Symlink(outside, filepath.Join(r.Dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("README.md", filepath.Join(r.Dir, "inside")); err != nil {
		t.Fatal(err)
	}

	rec := importVia(t, app, r.Dir)
	cw := app.W.CodeWorkspace
	if err := cw.OpenWorkspace(ctx, idArgs(rec.ID)); err != nil {
		t.Fatal(err)
	}

	read := func(path string) (codeworkspace.FileContent, error) {
		return cw.ReadFile(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: path})
	}

	t.Run("read classifies text, binary, oversized, missing", func(t *testing.T) {
		txt, err := read("src/main.go")
		if err != nil || txt.Kind != "found" || txt.Text != "package main\n\nfunc main() {}\n" || txt.Language == "" {
			t.Fatalf("text = %+v, %v", txt, err)
		}
		if b, err := read("blob.bin"); err != nil || b.Kind != "binary" {
			t.Fatalf("binary = %+v, %v", b, err)
		}
		if b, err := read("big.txt"); err != nil || b.Kind != "tooLarge" || b.LimitBytes != codeworkspace.MaxReadBytes {
			t.Fatalf("large = %+v, %v", b, err)
		}
		if m, err := read("gone.txt"); err != nil || m.Kind != "missing" {
			t.Fatalf("missing = %+v, %v", m, err)
		}
		if in, err := read("inside"); err != nil || in.Kind != "found" || in.Text != "# hi\n" {
			t.Fatalf("in-root symlink = %+v, %v", in, err)
		}
	})

	t.Run("escapes refused", func(t *testing.T) {
		for _, p := range []string{"escape", "../outside.txt", "src/../../outside.txt", outside} {
			if c, err := read(p); errCode(err) != "E_CONTRACT" {
				t.Errorf("read %q = %+v, %v, want E_CONTRACT", p, c, err)
			}
			_, err := cw.ReadDiff(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: p})
			if err == nil {
				t.Errorf("ReadDiff %q succeeded", p)
			}
		}
	})

	t.Run("diff against HEAD", func(t *testing.T) {
		d, err := cw.ReadDiff(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "src/main.go"})
		if err != nil {
			t.Fatal(err)
		}
		head := r.Git("show", "HEAD:src/main.go") + "\n"
		if d.Head.Kind != "found" || d.Head.Text != head || d.Worktree.Text != "package main\n\nfunc main() {}\n" {
			t.Fatalf("diff = %+v", d)
		}
		added, err := cw.ReadDiff(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "new.txt"})
		if err != nil || added.Head.Kind != "missing" || added.Worktree.Kind != "found" {
			t.Fatalf("added diff = %+v, %v", added, err)
		}
		if err := os.Remove(filepath.Join(r.Dir, "README.md")); err != nil {
			t.Fatal(err)
		}
		deleted, err := cw.ReadDiff(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "README.md"})
		if err != nil || deleted.Head.Kind != "found" || deleted.Worktree.Kind != "missing" {
			t.Fatalf("deleted diff = %+v, %v", deleted, err)
		}
	})

	t.Run("ifStat and ifHeadOid skip unchanged sides", func(t *testing.T) {
		first, err := read("src/main.go")
		if err != nil || first.Stat == nil {
			t.Fatalf("first read = %+v, %v, want a stat", first, err)
		}
		same, err := cw.ReadFile(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "src/main.go", IfStat: first.Stat})
		if err != nil || same.Kind != "unchanged" || same.Text != "" {
			t.Fatalf("matching ifStat = %+v, %v, want unchanged without text", same, err)
		}

		d, err := cw.ReadDiff(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "src/main.go"})
		if err != nil || d.Worktree.Stat == nil || d.Head.Oid == "" {
			t.Fatalf("diff = %+v, %v, want worktree stat and head oid", d, err)
		}
		gated := bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "src/main.go", IfStat: d.Worktree.Stat, IfHeadOid: d.Head.Oid}
		g, err := cw.ReadDiff(ctx, gated)
		if err != nil || g.Worktree.Kind != "unchanged" || g.Head.Kind != "unchanged" || g.Worktree.Text != "" || g.Head.Text != "" {
			t.Fatalf("matching gate = %+v, %v, want both sides unchanged", g, err)
		}

		// Same size, different bytes, mtime restored: a stat-only gate would miss this.
		abs := filepath.Join(r.Dir, "src/main.go")
		orig, err := os.ReadFile(abs)
		if err != nil {
			t.Fatal(err)
		}
		before, err := os.Stat(abs)
		if err != nil {
			t.Fatal(err)
		}
		edited := []byte(strings.Replace(string(orig), "main", "mein", 1))
		if err := os.WriteFile(abs, edited, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(abs, before.ModTime(), before.ModTime()); err != nil {
			t.Fatal(err)
		}
		sameSize, err := cw.ReadFile(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "src/main.go", IfStat: first.Stat})
		if err != nil || sameSize.Kind != "found" || !strings.Contains(sameSize.Text, "mein") {
			t.Fatalf("same-size edit with restored mtime = %+v, %v, want found with new text", sameSize, err)
		}
		g, err = cw.ReadDiff(ctx, gated)
		if err != nil || g.Worktree.Kind != "found" {
			t.Fatalf("diff gate after same-size edit = %+v, %v, want worktree found", g, err)
		}
		if err := os.WriteFile(abs, orig, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(abs, before.ModTime().Add(time.Hour), before.ModTime().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		rewritten, err := cw.ReadFile(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "src/main.go", IfStat: first.Stat})
		if err != nil || rewritten.Kind != "unchanged" {
			t.Fatalf("identical rewrite with new mtime = %+v, %v, want unchanged", rewritten, err)
		}

		r.Write("src/main.go", "package main\n\nfunc main() { println(1) }\n")
		after, err := cw.ReadFile(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "src/main.go", IfStat: first.Stat})
		if err != nil || after.Kind != "found" || !strings.Contains(after.Text, "println") {
			t.Fatalf("read after write = %+v, %v, want fresh content", after, err)
		}
		g, err = cw.ReadDiff(ctx, gated)
		if err != nil || g.Worktree.Kind != "found" || g.Head.Kind != "unchanged" {
			t.Fatalf("gate after write = %+v, %v, want worktree found, head unchanged", g, err)
		}

		r.Git("add", "src/main.go")
		r.Commit("rewrite main", nil)
		g, err = cw.ReadDiff(ctx, gated)
		if err != nil || g.Head.Kind != "found" || g.Head.Oid == d.Head.Oid {
			t.Fatalf("gate after commit = %+v, %v, want a re-read head with a new oid", g, err)
		}
	})
}

// Closing a workspace twice, or one that never opened, is harmless; an empty id is refused; a read
// after close reopens it.
func TestCloseWorkspace(t *testing.T) {
	app := flowharness.New(t)
	r := app.NewRepo("close")
	r.Commit("init", map[string]string{"d0/f0.txt": "x\n"})
	rec := importVia(t, app, r.Dir)
	cw := app.W.CodeWorkspace
	if err := cw.OpenWorkspace(ctx, idArgs(rec.ID)); err != nil {
		t.Fatal(err)
	}
	if err := cw.CloseWorkspace(idArgs(rec.ID)); err != nil {
		t.Fatal(err)
	}
	// Closing twice, or an id that never opened, is harmless; an empty id is refused.
	if err := cw.CloseWorkspace(idArgs(rec.ID)); err != nil {
		t.Fatalf("second CloseWorkspace = %v", err)
	}
	if err := cw.CloseWorkspace(idArgs("")); errCode(err) != "E_CONTRACT" {
		t.Fatalf("CloseWorkspace with no id = %v, want E_CONTRACT", err)
	}
	if _, err := cw.ReadFile(ctx, bridge.CodeWorkspaceReadFileArgs{ID: rec.ID, Path: "d0/f0.txt"}); err != nil {
		t.Fatalf("ReadFile after close: %v", err)
	}
}
