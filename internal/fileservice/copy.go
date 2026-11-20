package fileservice

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Hash is the sha256 hex of the file, streamed.
func (s *Service) Hash(name string) (string, error) {
	f, _, err := s.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", wrapErr("hash", name, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Copy copies the tree at from to dst's to through to+".tmp", verifies it, then renames it (rule 10).
func (s *Service) Copy(from string, dst *Service, to string) error {
	src, err := s.resolve(from, true, false)
	if err != nil {
		return err
	}
	defer src.close()
	final, err := dst.resolve(to, false, false)
	if err != nil {
		return err
	}
	defer final.close()
	tmp, err := dst.resolve(to+".tmp", false, false)
	if err != nil {
		return err
	}
	defer tmp.close()
	if err := tmp.fsys.RemoveAll(tmp.name); err != nil {
		return tmp.fail("copy", err)
	}
	if err := ensureDir(final, dst); err != nil {
		return final.fail("copy", err)
	}
	if err := copyTree(src.fsys, src.name, tmp.fsys, tmp.name); err != nil {
		return src.fail("copy", err)
	}
	if err := verifyTree(src.fsys, src.name, tmp.fsys, tmp.name); err != nil {
		return src.fail("copy", err)
	}
	if err := tmp.fsys.Rename(tmp.name, final.name); err != nil {
		return final.fail("copy", err)
	}
	return final.fail("copy", syncDir(func() (*os.File, error) { return final.fsys.OpenFile(filepath.Dir(final.name), os.O_RDONLY, 0) }))
}

func ensureDir(t *target, s *Service) error {
	dir := filepath.Dir(t.name)
	if dir == "." {
		return nil
	}
	return t.fsys.MkdirAll(dir, s.dirPerm())
}

func copyTree(sf fsys, sn string, df fsys, dn string) error {
	fi, err := sf.Lstat(sn)
	if err != nil {
		return err
	}
	switch {
	case fi.IsDir():
		if err := df.MkdirAll(dn, 0o700); err != nil {
			return err
		}
		ents, err := readDir(sf, sn)
		if err != nil {
			return err
		}
		for _, e := range ents {
			if err := copyTree(sf, filepath.Join(sn, e.Name()), df, filepath.Join(dn, e.Name())); err != nil {
				return err
			}
		}
		return df.Chmod(dn, fi.Mode().Perm())
	case fi.Mode()&fs.ModeSymlink != 0:
		link, err := sf.Readlink(sn)
		if err != nil {
			return err
		}
		if err := df.RemoveAll(dn); err != nil {
			return err
		}
		return df.Symlink(link, dn)
	case fi.Mode().IsRegular():
		return copyFile(sf, sn, df, dn, fi.Mode().Perm())
	}
	return nil
}

func copyFile(sf fsys, sn string, df fsys, dn string, mode fs.FileMode) (err error) {
	in, err := sf.OpenFile(sn, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := df.OpenFile(dn, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
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
	if err := out.Sync(); err != nil {
		return err
	}
	return df.Chmod(dn, mode)
}

// verifyTree fails unless every entry of the source exists in the copy with the same type, size and
// content. A copy superset is fine.
func verifyTree(sf fsys, sn string, df fsys, dn string) error {
	if err := sameEntry(sf, sn, df, dn); err != nil {
		return err
	}
	fi, err := sf.Lstat(sn)
	if err != nil || !fi.IsDir() {
		return err
	}
	ents, err := readDir(sf, sn)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if err := verifyTree(sf, filepath.Join(sn, e.Name()), df, filepath.Join(dn, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func sameEntry(sf fsys, sn string, df fsys, dn string) error {
	a, err := sf.Lstat(sn)
	if err != nil {
		return err
	}
	b, err := df.Lstat(dn)
	if err != nil {
		return err
	}
	if a.Mode().Type() != b.Mode().Type() {
		return errors.New("verify " + sn + ": type differs")
	}
	switch {
	case a.Mode()&fs.ModeSymlink != 0:
		la, err := sf.Readlink(sn)
		if err != nil {
			return err
		}
		lb, err := df.Readlink(dn)
		if err != nil {
			return err
		}
		if la != lb {
			return errors.New("verify " + sn + ": link target differs")
		}
	case a.Mode().IsRegular():
		if a.Size() != b.Size() {
			return errors.New("verify " + sn + ": size differs")
		}
		ha, err := fileHash(sf, sn)
		if err != nil {
			return err
		}
		hb, err := fileHash(df, dn)
		if err != nil {
			return err
		}
		if !bytes.Equal(ha, hb) {
			return errors.New("verify " + sn + ": content differs")
		}
	}
	return nil
}

func fileHash(f fsys, name string) ([]byte, error) {
	r, err := f.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
