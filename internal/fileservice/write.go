package fileservice

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/google/renameio/v2"
)

// Pending is a streamed atomic write: Write the bytes, then Commit or Discard.
type Pending struct {
	pf   *renameio.PendingFile
	sub  *os.Root // rooted: the target's dir as a root
	dir  string   // host-wide: the target's dir
	base string
	disp string
	s    *Service
	done bool
}

// Write appends to the temp file; the target is untouched until Commit.
func (p *Pending) Write(b []byte) (int, error) {
	n, err := p.pf.Write(b)
	return n, wrapErr("write", p.disp, err)
}

// Commit fsyncs the temp file, renames it over the target and fsyncs the parent dir (rule 5). A
// dir-fsync error is returned with the new content already in place.
func (p *Pending) Commit() (Info, error) {
	if p.done {
		return Info{}, wrapErr("commit", p.disp, os.ErrClosed)
	}
	defer p.release()
	if err := p.pf.CloseAtomicallyReplace(); err != nil {
		_ = p.pf.Cleanup()
		return Info{}, wrapErr("commit", p.disp, err)
	}
	var (
		fi  fs.FileInfo
		err error
	)
	if p.sub != nil {
		err = syncDir(func() (*os.File, error) { return p.sub.Open(".") })
		if err == nil {
			fi, err = p.sub.Lstat(p.base)
		}
	} else {
		err = syncDir(func() (*os.File, error) { return os.Open(p.dir) })
		if err == nil {
			fi, err = os.Lstat(filepath.Join(p.dir, p.base))
		}
	}
	if err != nil {
		return Info{}, wrapErr("commit", p.disp, err)
	}
	return p.s.infoOf(fi), nil
}

// Discard removes the temp file; idempotent and safe after Commit.
func (p *Pending) Discard() error {
	if p.done {
		return nil
	}
	defer p.release()
	return wrapErr("discard", p.disp, p.pf.Cleanup())
}

func (p *Pending) release() {
	p.done = true
	if p.sub != nil {
		_ = p.sub.Close()
	}
}

func syncDir(open func() (*os.File, error)) error {
	d, err := open()
	if err != nil {
		return err
	}
	err = d.Sync()
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	return err
}

// Create starts a streamed atomic write of name (rules 5 and 6): parents are created, IfVersion is
// checked now, and the write lands on Commit.
func (s *Service) Create(name string, o WriteOptions) (*Pending, error) {
	t, err := s.resolve(name, false, true)
	if err != nil {
		return nil, err
	}
	defer t.close()
	existing, err := s.existingTarget(t, name)
	if err != nil {
		return nil, err
	}
	if v := o.IfVersion; v != nil {
		switch {
		case existing == nil && *v != "":
			return nil, t.fail("create", policy(ErrConflict, "%s is gone", name))
		case existing != nil && *v != versionOf(existing):
			return nil, t.fail("create", policy(ErrConflict, "%s changed", name))
		}
	}
	perm := o.Perm
	if perm == 0 {
		perm = s.filePerm()
	}
	return s.openPending(t, name, permOptions(o, perm))
}

// permOptions maps the write's permission flags to renameio options.
func permOptions(o WriteOptions, perm fs.FileMode) []renameio.Option {
	switch {
	case o.Static:
		return []renameio.Option{renameio.WithStaticPermissions(perm)}
	case o.KeepPerm:
		return []renameio.Option{renameio.WithPermissions(perm), renameio.WithExistingPermissions()}
	}
	return []renameio.Option{renameio.WithPermissions(perm)}
}

// existingTarget is the Lstat of the write target: nil when absent. A host-wide leaf symlink is
// followed so the link survives the write. A folder or a refused file is an error.
func (s *Service) existingTarget(t *target, name string) (fs.FileInfo, error) {
	existing := t.leaf
	if s.follows() {
		var err error
		existing, err = t.fsys.Lstat(t.name)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			existing = nil
		case err != nil:
			return nil, t.fail("create", err)
		case existing.Mode()&fs.ModeSymlink != 0 && s.root == "":
			if real, rerr := filepath.EvalSymlinks(t.name); rerr == nil {
				t.name, t.abs = real, real
				if existing, err = os.Lstat(real); err != nil {
					return nil, t.fail("create", err)
				}
			}
		}
	}
	if existing == nil {
		return nil, nil
	}
	if existing.IsDir() {
		return nil, t.fail("create", policy(ErrInvalidPath, "%s is a folder", name))
	}
	if why := s.refusal(existing); why != "" {
		return nil, t.fail("create", policy(ErrUnsafe, "%s is a %s", name, why))
	}
	return existing, nil
}

// openPending creates the target's parents and its temp file.
func (s *Service) openPending(t *target, name string, opts []renameio.Option) (*Pending, error) {
	var err error
	p := &Pending{disp: name, s: s}
	dir := filepath.Dir(t.name)
	if s.root == "" {
		if err := os.MkdirAll(dir, s.dirPerm()); err != nil {
			return nil, t.fail("create", err)
		}
		p.dir, p.base = dir, filepath.Base(t.name)
		p.pf, err = renameio.NewPendingFile(t.name, append(opts, renameio.WithTempDir(dir))...)
	} else {
		if dir != "." {
			if err := t.root.MkdirAll(dir, s.dirPerm()); err != nil {
				return nil, t.fail("create", err)
			}
		}
		if p.sub, err = t.root.OpenRoot(dir); err != nil {
			return nil, t.fail("create", err)
		}
		p.base = filepath.Base(t.name)
		p.pf, err = renameio.NewPendingFile(p.base, append(opts, renameio.WithRoot(p.sub))...)
		if err != nil {
			_ = p.sub.Close()
		}
	}
	if err != nil {
		return nil, t.fail("create", err)
	}
	return p, nil
}

// Write replaces name atomically with data (rule 5).
func (s *Service) Write(name string, data []byte, o WriteOptions) (Info, error) {
	p, err := s.Create(name, o)
	if err != nil {
		return Info{}, err
	}
	if _, err := p.Write(data); err != nil {
		_ = p.Discard()
		return Info{}, err
	}
	return p.Commit()
}

// tempBase resolves the dir temp files go in; "" is the OS temp dir (host-wide) or the root.
func (s *Service) tempBase(dir string) (string, error) {
	if dir == "" {
		if s.root == "" {
			return os.TempDir(), nil
		}
		return s.root, nil
	}
	return s.Path(dir)
}

// TempFile writes data to a new 0600 file in dir and returns its absolute path and a cleanup that
// removes it. A rooted Service checks dir; the file itself is host-created.
func (s *Service) TempFile(dir, pattern string, data []byte) (string, func(), error) {
	base, err := s.tempBase(dir)
	if err != nil {
		return "", nil, err
	}
	f, err := os.CreateTemp(base, pattern)
	if err != nil {
		return "", nil, wrapErr("tempfile", dir, err)
	}
	p := f.Name()
	cleanup := func() { _ = os.Remove(p) }
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		cleanup()
		return "", nil, wrapErr("tempfile", dir, werr)
	}
	return p, cleanup, nil
}

// TempDir makes a new 0700 dir in dir and returns its absolute path and a cleanup that removes it
// with everything in it.
func (s *Service) TempDir(dir, pattern string) (string, func(), error) {
	base, err := s.tempBase(dir)
	if err != nil {
		return "", nil, err
	}
	p, err := os.MkdirTemp(base, pattern)
	if err != nil {
		return "", nil, wrapErr("tempdir", dir, err)
	}
	return p, func() { _ = os.RemoveAll(p) }, nil
}

// Mkdir makes one dir; ErrExist when it is there.
func (s *Service) Mkdir(name string) error {
	t, err := s.resolve(name, false, true)
	if err != nil {
		return err
	}
	defer t.close()
	return t.fail("mkdir", t.fsys.Mkdir(t.name, s.dirPerm()))
}

// MkdirAll makes a dir and its parents.
func (s *Service) MkdirAll(name string) error {
	t, err := s.resolve(name, true, true)
	if err != nil {
		return err
	}
	defer t.close()
	return t.fail("mkdir", t.fsys.MkdirAll(t.name, s.dirPerm()))
}

// Chtimes sets the access and modification time of name.
func (s *Service) Chtimes(name string, at time.Time) error {
	t, err := s.resolve(name, false, true)
	if err != nil {
		return err
	}
	defer t.close()
	return t.fail("chtimes", t.fsys.Chtimes(t.name, at, at))
}

// WriteFile is Write with a mode and no Info, the shape of os.WriteFile: perm 0 means the policy default.
func (s *Service) WriteFile(name string, data []byte, perm fs.FileMode) error {
	_, err := s.Write(name, data, WriteOptions{Perm: perm})
	return err
}
