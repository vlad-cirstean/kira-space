package fileservice

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// fsys is the file system a call runs on: an os.Root (rooted) or the host.
type fsys interface {
	OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error)
	Lstat(name string) (fs.FileInfo, error)
	Stat(name string) (fs.FileInfo, error)
	Mkdir(name string, perm fs.FileMode) error
	MkdirAll(name string, perm fs.FileMode) error
	Remove(name string) error
	RemoveAll(name string) error
	Rename(from, to string) error
	Chtimes(name string, atime, mtime time.Time) error
	Chmod(name string, mode fs.FileMode) error
	Readlink(name string) (string, error)
	Symlink(oldname, newname string) error
}

type hostFS struct{}

func (hostFS) OpenFile(n string, f int, p fs.FileMode) (*os.File, error) { return os.OpenFile(n, f, p) }
func (hostFS) Lstat(n string) (fs.FileInfo, error)                       { return os.Lstat(n) }
func (hostFS) Stat(n string) (fs.FileInfo, error)                        { return os.Stat(n) }
func (hostFS) Mkdir(n string, p fs.FileMode) error                       { return os.Mkdir(n, p) }
func (hostFS) MkdirAll(n string, p fs.FileMode) error                    { return os.MkdirAll(n, p) }
func (hostFS) Remove(n string) error                                     { return os.Remove(n) }
func (hostFS) RemoveAll(n string) error                                  { return os.RemoveAll(n) }
func (hostFS) Rename(a, b string) error                                  { return os.Rename(a, b) }
func (hostFS) Chtimes(n string, a, m time.Time) error                    { return os.Chtimes(n, a, m) }
func (hostFS) Chmod(n string, m fs.FileMode) error                       { return os.Chmod(n, m) }
func (hostFS) Readlink(n string) (string, error)                         { return os.Readlink(n) }
func (hostFS) Symlink(a, b string) error                                 { return os.Symlink(a, b) }

// target is one resolved name: where to run, what to call it in errors.
type target struct {
	fsys  fsys
	root  *os.Root    // nil host-wide
	name  string      // the name on fsys: rooted, a slash path inside the root; host-wide, absolute
	disp  string      // the name in errors
	abs   string      // absolute OS path (watch, argv, temp dirs)
	leaf  fs.FileInfo // Refuse only: Lstat of the leaf, nil when missing
	isTop bool        // names the root itself
}

func (t *target) close() {
	if t.root != nil {
		_ = t.root.Close()
	}
}

// wrap turns any error into a *fs.PathError of op on the display name; policy errors pass inside.
func wrapErr(op, disp string, err error) error {
	if err == nil {
		return nil
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		inner := pe.Err
		if strings.Contains(inner.Error(), "path escapes from parent") {
			inner = policy(ErrEscapes, "%s", disp)
		}
		return &fs.PathError{Op: op, Path: disp, Err: inner}
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return &fs.PathError{Op: op, Path: disp, Err: le.Err}
	}
	return &fs.PathError{Op: op, Path: disp, Err: err}
}

func (t *target) fail(op string, err error) error { return wrapErr(op, t.disp, err) }

// notExist is the error for a missing name.
func (t *target) notExist(op string) error { return &fs.PathError{Op: op, Path: t.disp, Err: fs.ErrNotExist} }

// cleanName applies rule 1 and returns the name to resolve. root true allows "." (the root itself).
func (s *Service) cleanName(name string, root bool) (string, error) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", policy(ErrInvalidPath, "%q", name)
	}
	if s.root == "" {
		if !filepath.IsAbs(name) {
			return "", policy(ErrInvalidPath, "%q is not absolute", name)
		}
		return filepath.Clean(name), nil
	}
	contain := s.p.Symlinks == Contain
	bad := ErrInvalidPath
	if contain {
		bad = ErrEscapes
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", policy(bad, "absolute path %q", name)
	}
	if contain {
		name = path.Clean(filepath.ToSlash(name))
	} else if name != "." && path.Clean(name) != name {
		return "", policy(ErrInvalidPath, "%q", name)
	}
	for _, seg := range strings.Split(name, "/") {
		switch seg {
		case "..":
			return "", policy(bad, "%q", name)
		case "", ".":
			if name != "." {
				return "", policy(ErrInvalidPath, "%q", name)
			}
		}
	}
	if name == "." && !root {
		return "", policy(ErrInvalidPath, "%q names the root", name)
	}
	return name, nil
}

// resolve checks name and prepares the call. followLeaf false keeps a leaf symlink as it is
// (Remove, Rename, Lstat). Close the target when done.
func (s *Service) resolve(name string, root, followLeaf bool) (*target, error) {
	clean, err := s.cleanName(name, root)
	if err != nil {
		return nil, wrapErr("resolve", name, err)
	}
	if s.root == "" {
		return &target{fsys: hostFS{}, name: clean, disp: name, abs: clean}, nil
	}
	r, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, wrapErr("resolve", name, err)
	}
	t := &target{fsys: r, root: r, disp: name, isTop: clean == "."}
	t.abs = filepath.Join(s.root, filepath.FromSlash(clean))
	switch s.p.Symlinks {
	case Refuse:
		t.name = filepath.FromSlash(clean)
		if err := t.walk(clean); err != nil {
			t.close()
			return nil, wrapErr("resolve", name, err)
		}
	default:
		rel, err := s.contain(clean, followLeaf)
		if err != nil {
			t.close()
			return nil, wrapErr("resolve", name, err)
		}
		t.name = rel
		t.abs = filepath.Join(s.root, rel)
	}
	return t, nil
}

// walk Lstats every component of clean through the root (rule 3).
func (t *target) walk(clean string) error {
	if clean == "." {
		fi, err := t.root.Lstat(".")
		if err != nil {
			return err
		}
		t.leaf = fi
		return nil
	}
	segs := strings.Split(clean, "/")
	cur := ""
	for i, seg := range segs {
		cur = path.Join(cur, seg)
		fi, err := t.root.Lstat(filepath.FromSlash(cur))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return policy(ErrUnsafe, "%s is a symlink", clean)
		}
		if i < len(segs)-1 && !fi.IsDir() {
			return fs.ErrNotExist
		}
		if i == len(segs)-1 {
			t.leaf = fi
		}
	}
	return nil
}

// contain is rule 2: the resolved name of clean relative to the resolved root. A missing tail resolves
// through its deepest existing parent; a dangling symlink on the path is ErrEscapes. followLeaf false
// resolves only the parent of the leaf.
func (s *Service) contain(clean string, followLeaf bool) (string, error) {
	if clean == "." {
		return ".", nil
	}
	joined := filepath.Join(s.root, filepath.FromSlash(clean))
	if !followLeaf {
		dir, base := filepath.Split(joined)
		resolvedDir, err := s.resolveExisting(dir, clean)
		if err != nil {
			return "", err
		}
		return s.under(filepath.Join(resolvedDir, base))
	}
	resolved, err := s.resolveExisting(joined, clean)
	if err != nil {
		return "", err
	}
	return s.under(resolved)
}

// resolveExisting EvalSymlinks p, falling back to the deepest existing ancestor plus the missing tail.
func (s *Service) resolveExisting(p, clean string) (string, error) {
	tail := ""
	cur := filepath.Clean(p)
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(resolved, tail), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if fi, lerr := os.Lstat(cur); lerr == nil && fi.Mode()&fs.ModeSymlink != 0 {
			return "", policy(ErrEscapes, "dangling symlink %q", clean)
		}
		parent := filepath.Dir(cur)
		if parent == cur || cur == s.root {
			return "", err
		}
		tail = filepath.Join(filepath.Base(cur), tail)
		cur = parent
	}
}

// under returns resolved as a name relative to the root, or ErrEscapes.
func (s *Service) under(resolved string) (string, error) {
	rel, err := filepath.Rel(s.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", policy(ErrEscapes, "%q", resolved)
	}
	return rel, nil
}

// Path returns the checked absolute path of name, for argv, an editor or git. A rooted Contain
// Service returns the resolved path. The path is a snapshot: use the Service for the I/O itself.
func (s *Service) Path(name string) (string, error) {
	t, err := s.resolve(name, true, true)
	if err != nil {
		return "", err
	}
	defer t.close()
	return t.abs, nil
}

// ValidName reports whether name passes the strict rooted name rule (clean relative slash path, no
// empty, "." or ".." segment), for callers that keep their own name check ahead of a Service.
func ValidName(name string) error {
	if name == "" || strings.ContainsRune(name, 0) || strings.HasPrefix(name, "/") || path.Clean(name) != name {
		return policy(ErrInvalidPath, "%q", name)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "." || seg == ".." {
			return policy(ErrInvalidPath, "%q", name)
		}
	}
	return nil
}
