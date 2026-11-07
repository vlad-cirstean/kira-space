// Package fsmove copies, verifies and tests config trees (config sync apply).
package fsmove

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// CopyTree copies src to dst: directories, regular files and symlinks (kept as links), each with its
// source mode. os.CopyFS would follow links and loosen modes.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			return os.Chmod(target, info.Mode().Perm())
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			if err := os.RemoveAll(target); err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return copyFile(p, target, info.Mode().Perm())
		default:
			return nil
		}
	})
}

func copyFile(src, dst string, mode fs.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

// CopyVerified copies src to dst through dst+".tmp" and a verify, so dst is never half written.
func CopyVerified(src, dst string) error {
	tmp := dst + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("fsmove: %w", err)
	}
	if err := CopyTree(src, tmp); err != nil {
		return fmt.Errorf("fsmove: copy %s: %w", src, err)
	}
	if err := VerifyTree(src, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("fsmove: %w", err)
	}
	return nil
}

// VerifyTree fails unless every entry of src Exists in dst with the same type, size and content. A
// dst superset is fine: it is what a resumed move sees after a half-finished delete of src.
func VerifyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if err := sameEntry(p, filepath.Join(dst, rel)); err != nil {
			return fmt.Errorf("fsmove: verify %s: %w", rel, err)
		}
		return nil
	})
}

func sameEntry(p, q string) error {
	a, err := os.Lstat(p)
	if err != nil {
		return err
	}
	b, err := os.Lstat(q)
	if err != nil {
		return err
	}
	if a.Mode().Type() != b.Mode().Type() {
		return errors.New("type differs")
	}
	switch {
	case a.Mode()&fs.ModeSymlink != 0:
		la, err := os.Readlink(p)
		if err != nil {
			return err
		}
		lb, err := os.Readlink(q)
		if err != nil {
			return err
		}
		if la != lb {
			return errors.New("link target differs")
		}
	case a.Mode().IsRegular():
		if a.Size() != b.Size() {
			return errors.New("size differs")
		}
		ha, err := fileHash(p)
		if err != nil {
			return err
		}
		hb, err := fileHash(q)
		if err != nil {
			return err
		}
		if !bytes.Equal(ha, hb) {
			return errors.New("content differs")
		}
	}
	return nil
}

func fileHash(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// Exists reports whether p exists; a dangling symlink counts.
func Exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// IsEmptyDir reports whether p is missing or an empty directory.
func IsEmptyDir(p string) (bool, error) {
	entries, err := os.ReadDir(p)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}
