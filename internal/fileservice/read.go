package fileservice

import (
	"errors"
	"io"
	"io/fs"
	"os"
)

// statTarget returns the info of the resolved leaf: Refuse reads the Lstat taken while resolving,
// the others follow.
func (t *target) statLeaf(refuseLeaf bool) (fs.FileInfo, error) {
	if refuseLeaf {
		if t.leaf == nil {
			return nil, t.notExist("stat")
		}
		return t.leaf, nil
	}
	return t.fsys.Stat(t.name)
}

func (s *Service) follows() bool { return s.root == "" || s.p.Symlinks == Contain }

// Stat returns the entry's info without reading it. A refused entry (rule 4) is ErrUnsafe.
func (s *Service) Stat(name string) (Info, error) {
	t, err := s.resolve(name, true, true)
	if err != nil {
		return Info{}, err
	}
	defer t.close()
	fi, err := t.statLeaf(!s.follows())
	if err != nil {
		return Info{}, t.fail("stat", err)
	}
	if why := s.refusal(fi); why != "" {
		return Info{}, wrapErr("stat", name, policy(ErrUnsafe, "%s is a %s", name, why))
	}
	return s.infoOf(fi), nil
}

// Lstat returns the entry's own info, a leaf symlink included. Refusals show in Info.Refused.
func (s *Service) Lstat(name string) (Info, error) {
	t, err := s.resolve(name, true, false)
	if err != nil {
		return Info{}, err
	}
	defer t.close()
	fi := t.leaf
	if fi == nil {
		if fi, err = t.fsys.Lstat(t.name); err != nil {
			return Info{}, t.fail("lstat", err)
		}
	}
	return s.infoOf(fi), nil
}

// Exists reports whether name exists; a dangling symlink counts.
func (s *Service) Exists(name string) (bool, error) {
	_, err := s.Lstat(name)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, err
}

// opener opens the resolved file read-only under rule 3 and 4 and returns its fstat.
func (s *Service) opener(t *target, op string) (*os.File, fs.FileInfo, error) {
	flags := os.O_RDONLY
	if s.p.PlainOnly || !s.follows() {
		flags = openFlags
	}
	lfi := t.leaf
	if !s.follows() {
		if lfi == nil {
			return nil, nil, t.notExist(op)
		}
		if lfi.IsDir() {
			return nil, nil, t.fail(op, policy(ErrInvalidPath, "%s is a folder", t.disp))
		}
		if why := s.refusal(lfi); why != "" {
			return nil, nil, t.fail(op, policy(ErrUnsafe, "%s is a %s", t.disp, why))
		}
	}
	f, err := t.fsys.OpenFile(t.name, flags, 0)
	if err != nil {
		if !s.follows() && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrPermission) {
			return nil, nil, t.fail(op, policy(ErrUnsafe, "%s: %v", t.disp, err))
		}
		return nil, nil, t.fail(op, err)
	}
	ffi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, t.fail(op, err)
	}
	if ffi.IsDir() {
		_ = f.Close()
		return nil, nil, t.fail(op, policy(ErrInvalidPath, "%s is a folder", t.disp))
	}
	if why := s.refusal(ffi); why != "" {
		_ = f.Close()
		return nil, nil, t.fail(op, policy(ErrUnsafe, "%s is a %s", t.disp, why))
	}
	if lfi != nil && !os.SameFile(lfi, ffi) { // swapped between lstat and open
		_ = f.Close()
		return nil, nil, t.fail(op, policy(ErrUnsafe, "%s changed while opening", t.disp))
	}
	return f, ffi, nil
}

// Read returns the file and its info, ErrTooLarge when it is over max bytes (rule 7).
func (s *Service) Read(name string, max int64) ([]byte, Info, error) {
	t, err := s.resolve(name, false, true)
	if err != nil {
		return nil, Info{}, err
	}
	defer t.close()
	max = s.maxRead(max)
	f, fi, err := s.opener(t, "read")
	if err != nil {
		return nil, Info{}, err
	}
	defer func() { _ = f.Close() }()
	if fi.Size() > max {
		return nil, Info{}, t.fail("read", policy(ErrTooLarge, "%s is %d bytes, the cap is %d", name, fi.Size(), max))
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, Info{}, t.fail("read", err)
	}
	if int64(len(data)) > max {
		return nil, Info{}, t.fail("read", policy(ErrTooLarge, "%s grew past %d bytes", name, max))
	}
	return data, s.infoOf(fi), nil
}

// Open returns the file for streaming, read-only, under the same checks as Read. The caller closes it.
func (s *Service) Open(name string) (*os.File, Info, error) {
	t, err := s.resolve(name, false, true)
	if err != nil {
		return nil, Info{}, err
	}
	defer t.close()
	f, fi, err := s.opener(t, "open")
	if err != nil {
		return nil, Info{}, err
	}
	return f, s.infoOf(fi), nil
}
