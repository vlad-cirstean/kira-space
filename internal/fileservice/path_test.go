package fileservice

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fixture is a root with a plain file, a contained link, an escaping link, an escaping dir link, a
// dangling link, a hard link and a FIFO, next to a secret outside it.
type fixture struct {
	root, outside string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	base := t.TempDir()
	f := fixture{root: filepath.Join(base, "root"), outside: filepath.Join(base, "outside")}
	for _, d := range []string{f.root, f.outside, filepath.Join(f.root, "dir")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, body string) {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(f.outside, "secret"), "secret")
	write(filepath.Join(f.root, "real.md"), "real")
	write(filepath.Join(f.root, "dir", "in.md"), "in")
	links := map[string]string{
		"inlink":   filepath.Join(f.root, "real.md"),
		"outlink":  filepath.Join(f.outside, "secret"),
		"dirlink":  f.outside,
		"dangling": filepath.Join(f.outside, "nothing"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(f.root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Link(filepath.Join(f.root, "real.md"), filepath.Join(f.root, "hard.md")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(f.root, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func mustNew(t *testing.T, p Policy) *Service {
	t.Helper()
	s, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRootedNames(t *testing.T) {
	f := newFixture(t)
	contain := mustNew(t, Policy{Root: f.root})
	refuse := mustNew(t, Policy{Root: f.root, Symlinks: Refuse, PlainOnly: true})
	cases := []struct {
		name          string
		contain, strict error
	}{
		{"/abs", ErrEscapes, ErrInvalidPath},
		{"../x", ErrEscapes, ErrInvalidPath},
		{"a/../../x", ErrEscapes, ErrInvalidPath},
		{"", ErrInvalidPath, ErrInvalidPath},
		{"a//b", nil, ErrInvalidPath},
		{"a/./b", nil, ErrInvalidPath},
		{"c/", nil, ErrInvalidPath},
		{"a\x00b", ErrInvalidPath, ErrInvalidPath},
		{".", ErrInvalidPath, ErrInvalidPath},
	}
	for _, c := range cases {
		for _, tc := range []struct {
			svc  *Service
			want error
		}{{contain, c.contain}, {refuse, c.strict}} {
			_, err := tc.svc.Write(c.name, []byte("x"), WriteOptions{})
			if tc.want == nil {
				if err != nil {
					t.Errorf("Write(%q) = %v, want ok", c.name, err)
				}
				continue
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("Write(%q) = %v, want %v", c.name, err, tc.want)
			}
			if _, err := tc.svc.Stat(c.name); c.name != "." && !errors.Is(err, tc.want) {
				t.Errorf("Stat(%q) = %v, want %v", c.name, err, tc.want)
			}
		}
	}
	if err := contain.RemoveAll("."); err == nil {
		t.Error("RemoveAll(.) succeeded")
	}
	host := Host()
	for _, name := range []string{"relative.md", ""} {
		if _, err := host.Stat(name); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("host Stat(%q) = %v, want ErrInvalidPath", name, err)
		}
	}
}

func TestContainSymlinks(t *testing.T) {
	f := newFixture(t)
	s := mustNew(t, Policy{Root: f.root})
	if data, _, err := s.Read("inlink", 0); err != nil || string(data) != "real" {
		t.Errorf("Read(inlink) = %q %v", data, err)
	}
	for _, name := range []string{"outlink", "dirlink/secret", "dangling", "dirlink/new.md"} {
		if _, _, err := s.Read(name, 0); !errors.Is(err, ErrEscapes) {
			t.Errorf("Read(%q) = %v, want ErrEscapes", name, err)
		}
		if _, err := s.Write(name, []byte("y"), WriteOptions{}); !errors.Is(err, ErrEscapes) {
			t.Errorf("Write(%q) = %v, want ErrEscapes", name, err)
		}
		if _, err := s.Path(name); !errors.Is(err, ErrEscapes) {
			t.Errorf("Path(%q) = %v, want ErrEscapes", name, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(f.outside, "secret")); err != nil || string(got) != "secret" {
		t.Fatalf("outside file touched: %q %v", got, err)
	}
	// A missing leaf and a missing tail are writable; the link itself is removable.
	if _, err := s.Write("new/deep/file.md", []byte("n"), WriteOptions{}); err != nil {
		t.Errorf("Write(missing tail) = %v", err)
	}
	if err := s.Remove("outlink"); err != nil {
		t.Errorf("Remove(outlink) = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(f.outside, "secret")); err != nil {
		t.Errorf("Remove followed the link: %v", err)
	}
	if err := s.Mkdir("dir"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Mkdir(existing) = %v, want ErrExist", err)
	}
}

func TestRefusePlainOnly(t *testing.T) {
	f := newFixture(t)
	s := mustNew(t, Policy{Root: f.root, Symlinks: Refuse, PlainOnly: true})
	for _, name := range []string{"inlink", "outlink", "dirlink/secret", "dangling", "hard.md", "real.md", "pipe"} {
		if _, _, err := s.Read(name, 0); !errors.Is(err, ErrUnsafe) {
			t.Errorf("Read(%q) = %v, want ErrUnsafe", name, err)
		}
		if _, err := s.Write(name, []byte("y"), WriteOptions{}); !errors.Is(err, ErrUnsafe) {
			t.Errorf("Write(%q) = %v, want ErrUnsafe", name, err)
		}
		if _, err := s.Stat(name); !errors.Is(err, ErrUnsafe) {
			t.Errorf("Stat(%q) = %v, want ErrUnsafe", name, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(f.outside, "secret")); err != nil || string(got) != "secret" {
		t.Fatalf("outside file touched: %q %v", got, err)
	}
	// real.md and hard.md share an inode; drop one and the other is usable.
	if err := os.Remove(filepath.Join(f.root, "hard.md")); err != nil {
		t.Fatal(err)
	}
	if data, _, err := s.Read("real.md", 0); err != nil || string(data) != "real" {
		t.Errorf("Read(real.md) = %q %v", data, err)
	}
	ents, err := s.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"inlink": "symlink", "outlink": "symlink", "dirlink": "symlink", "dangling": "symlink", "pipe": "special file", "real.md": "", "dir": ""}
	for _, e := range ents {
		if why, ok := want[e.Name]; ok && e.Refused != why {
			t.Errorf("ReadDir refused[%q] = %q, want %q", e.Name, e.Refused, why)
		}
	}
}

func TestErrorsArePathErrors(t *testing.T) {
	f := newFixture(t)
	s := mustNew(t, Policy{Root: f.root})
	_, _, err := s.Read("missing.md", 0)
	var pe *fs.PathError
	if !errors.As(err, &pe) || pe.Path != "missing.md" || !os.IsNotExist(err) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Read(missing) = %v", err)
	}
	if strings.Contains(err.Error(), f.root) {
		t.Fatalf("error leaks the root: %v", err)
	}
	if _, _, err := s.Read("real.md", 2); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Read over cap = %v", err)
	}
	stale := Version("1-1")
	if _, err := s.Write("real.md", []byte("x"), WriteOptions{IfVersion: &stale}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write = %v", err)
	}
	if err := s.Remove("missing.md"); err != nil {
		t.Fatalf("Remove(missing) = %v", err)
	}
	if err := s.Chtimes("real.md", time.Unix(1_000_000, 0)); err != nil {
		t.Fatal(err)
	}
}
