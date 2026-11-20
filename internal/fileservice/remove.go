package fileservice

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Remove deletes a file or an empty dir; a missing name is nil. A leaf symlink is removed, not followed.
func (s *Service) Remove(name string) error {
	t, err := s.resolve(name, false, false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer t.close()
	if err := t.fsys.Remove(t.name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return t.fail("remove", err)
	}
	return nil
}

// RemoveAll deletes name and everything below it; a missing name is nil. It refuses the root itself.
func (s *Service) RemoveAll(name string) error {
	t, err := s.resolve(name, false, false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer t.close()
	if s.root == "" && t.name == filepath.Dir(t.name) {
		return t.fail("removeall", policy(ErrInvalidPath, "%q is a file system root", name))
	}
	return t.fail("removeall", t.fsys.RemoveAll(t.name))
}

// Rename moves a file or dir within the Service (rule 9).
func (s *Service) Rename(from, to string, o RenameOptions) error {
	src, err := s.resolve(from, false, false)
	if err != nil {
		return err
	}
	defer src.close()
	dst, err := s.resolve(to, false, false)
	if err != nil {
		return err
	}
	defer dst.close()
	if s.follows() {
		if _, err := src.fsys.Lstat(src.name); err != nil {
			return src.fail("rename", err)
		}
	} else if src.leaf == nil {
		return src.notExist("rename")
	}
	if o.NoReplace {
		var exists bool
		if s.follows() {
			_, lerr := dst.fsys.Lstat(dst.name)
			exists = lerr == nil
		} else {
			exists = dst.leaf != nil
		}
		if exists {
			return dst.fail("rename", policy(ErrExist, "%s", to))
		}
	}
	dir := filepath.Dir(dst.name)
	if s.root != "" && dir != "." {
		if err := dst.root.MkdirAll(dir, s.dirPerm()); err != nil {
			return dst.fail("rename", err)
		}
	} else if s.root == "" {
		if err := os.MkdirAll(dir, s.dirPerm()); err != nil {
			return dst.fail("rename", err)
		}
	}
	if err := src.fsys.Rename(src.name, dst.name); err != nil {
		return src.fail("rename", err)
	}
	for _, d := range uniqueDirs(src.name, dst.name) {
		if err := syncDir(func() (*os.File, error) { return dst.fsys.OpenFile(d, os.O_RDONLY, 0) }); err != nil {
			return dst.fail("rename", err)
		}
	}
	return nil
}

func uniqueDirs(a, b string) []string {
	da, db := filepath.Dir(a), filepath.Dir(b)
	if da == db {
		return []string{db}
	}
	return []string{da, db}
}
