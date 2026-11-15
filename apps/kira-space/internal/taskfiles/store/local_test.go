package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func newTest(t *testing.T) *Local {
	t.Helper()
	l, err := NewLocal(filepath.Join(t.TempDir(), "task"))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func put(t *testing.T, l *Local, rel, body string) {
	t.Helper()
	if _, err := l.WriteAtomic(context.Background(), rel, []byte(body), nil); err != nil {
		t.Fatal(err)
	}
}

func TestLocalPaths(t *testing.T) {
	l := newTest(t)
	ctx := context.Background()
	for _, rel := range []string{"", "/abs", "../x", "a/../../x", "a//b", "a/./b", "a\\b", "a/", strings.Repeat("x", 300), "a\x00b"} {
		if _, err := l.Stat(ctx, rel); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Stat(%q) = %v, want ErrInvalidPath", rel, err)
		}
		if _, err := l.WriteAtomic(ctx, rel, nil, nil); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("WriteAtomic(%q) = %v, want ErrInvalidPath", rel, err)
		}
	}
	if _, err := l.Stat(ctx, "missing.md"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("missing = %v", err)
	}
}

func TestLocalRefusesUnsafeFiles(t *testing.T) {
	l := newTest(t)
	ctx := context.Background()
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	put(t, l, "real.md", "x")
	if err := os.Symlink(outside, filepath.Join(l.Root(), "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(l.Root(), "dirlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(l.Root(), "real.md"), filepath.Join(l.Root(), "hard.md")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(l.Root(), "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"link.md", "dirlink/secret", "hard.md", "pipe"} {
		if _, _, err := l.Read(ctx, rel, 1<<20); !errors.Is(err, ErrUnsafe) {
			t.Errorf("Read(%q) = %v, want ErrUnsafe", rel, err)
		}
		if _, err := l.WriteAtomic(ctx, rel, []byte("y"), nil); !errors.Is(err, ErrUnsafe) {
			t.Errorf("WriteAtomic(%q) = %v, want ErrUnsafe", rel, err)
		}
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "secret" {
		t.Fatalf("outside file touched: %q %v", got, err)
	}
	ents, err := l.List(ctx, ListOptions{MaxDepth: 3, MaxFiles: 10})
	if err != nil {
		t.Fatal(err)
	}
	refused := map[string]string{}
	for _, e := range ents {
		refused[e.Rel] = e.Refused
	}
	want := map[string]string{"link.md": "symlink", "dirlink": "symlink", "hard.md": "hard link", "real.md": "hard link", "pipe": "special file"}
	for rel, why := range want {
		if refused[rel] != why {
			t.Errorf("List refused[%q] = %q, want %q", rel, refused[rel], why)
		}
	}
}

func TestLocalCaps(t *testing.T) {
	l := newTest(t)
	ctx := context.Background()
	put(t, l, "big.md", strings.Repeat("a", 100))
	if _, _, err := l.Read(ctx, "big.md", 99); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Read over cap = %v", err)
	}
	if _, _, err := l.Read(ctx, "big.md", 100); err != nil {
		t.Fatalf("Read at cap = %v", err)
	}
	put(t, l, "a/b/c/deep.md", "x")
	for i := range 4 {
		put(t, l, filepath.Join("notes", string(rune('a'+i))+".md"), "n")
	}
	ents, err := l.List(ctx, ListOptions{MaxDepth: 3, MaxFiles: 100})
	if err != nil {
		t.Fatal(err)
	}
	var rels []string
	for _, e := range ents {
		rels = append(rels, e.Rel)
	}
	if got := strings.Join(rels, " "); got != "a/b/c big.md notes/a.md notes/b.md notes/c.md notes/d.md" {
		t.Fatalf("List = %s", got)
	}
	if !ents[0].Dir {
		t.Fatal("folder at the depth limit is not a Dir entry")
	}
	capped, err := l.List(ctx, ListOptions{MaxDepth: 3, MaxFiles: 3})
	files := 0
	for _, e := range capped {
		if !e.Dir {
			files++
		}
	}
	if !errors.Is(err, ErrTooManyFiles) || files != 3 {
		t.Fatalf("List over MaxFiles = %d files, %v", files, err)
	}
}

func TestLocalAtomicWrite(t *testing.T) {
	l := newTest(t)
	ctx := context.Background()
	absent := Version("")
	first, err := l.WriteAtomic(ctx, "notes/n.md", []byte("one"), &absent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.WriteAtomic(ctx, "notes/n.md", []byte("x"), &absent); !errors.Is(err, ErrConflict) {
		t.Fatalf("create over existing = %v", err)
	}
	stale := Version("1-1")
	if _, err := l.WriteAtomic(ctx, "notes/n.md", []byte("x"), &stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version = %v", err)
	}
	second, err := l.WriteAtomic(ctx, "notes/n.md", []byte("two!"), &first.Version)
	if err != nil {
		t.Fatal(err)
	}
	data, info, err := l.Read(ctx, "notes/n.md", 100)
	if err != nil || !bytes.Equal(data, []byte("two!")) || info.Version != second.Version {
		t.Fatalf("read back %q %v %v", data, info, err)
	}
	ents, _ := os.ReadDir(filepath.Join(l.Root(), "notes"))
	if len(ents) != 1 {
		t.Fatalf("temp file left behind: %v", ents)
	}
	if _, err := l.WriteAtomic(ctx, "notes", []byte("x"), nil); err == nil {
		t.Fatal("write over a folder succeeded")
	}
}

func TestLocalRemoveRename(t *testing.T) {
	l := newTest(t)
	ctx := context.Background()
	put(t, l, "handovers/draft-plan.md", "d")
	put(t, l, "handovers/0001-plan-to-implement.md", "h")
	if err := l.Rename(ctx, "handovers/draft-plan.md", "handovers/0001-plan-to-implement.md"); !errors.Is(err, ErrExist) {
		t.Fatalf("rename onto existing = %v", err)
	}
	if err := l.Rename(ctx, "handovers/draft-plan.md", "handovers/0002-plan-to-implement.md"); err != nil {
		t.Fatal(err)
	}
	if err := l.Remove(ctx, "handovers"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Stat(ctx, "handovers/0002-plan-to-implement.md"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("after Remove = %v", err)
	}
}
