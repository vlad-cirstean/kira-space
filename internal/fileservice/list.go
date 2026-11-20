package fileservice

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// readDir lists the dir at name on fsys, sorted by name, skipping entries that vanish.
func readDir(f fsys, name string) ([]fs.DirEntry, error) {
	d, err := f.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = d.Close() }()
	ents, err := d.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	return ents, nil
}

func (s *Service) requireDir(t *target, op string) error {
	if !s.follows() {
		if t.leaf == nil {
			return t.notExist(op)
		}
		if !t.leaf.IsDir() {
			return t.fail(op, policy(ErrInvalidPath, "%s is not a folder", t.disp))
		}
	}
	return nil
}

// ReadDir lists a dir sorted by name; an entry's Info is its own (a symlink is not followed).
func (s *Service) ReadDir(name string) ([]Entry, error) {
	t, err := s.resolve(name, true, true)
	if err != nil {
		return nil, err
	}
	defer t.close()
	if err := s.requireDir(t, "readdir"); err != nil {
		return nil, err
	}
	ents, err := readDir(t.fsys, t.name)
	if err != nil {
		return nil, t.fail("readdir", err)
	}
	out := make([]Entry, 0, len(ents))
	for _, e := range ents {
		fi, err := e.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, t.fail("readdir", err)
		}
		out = append(out, Entry{Name: e.Name(), Info: s.infoOf(fi)})
	}
	return out, nil
}

// Walk calls fn for every entry under name, depth first in name order: files and dirs, never the
// walked dir itself. Symlinks are listed, never followed. fn may return fs.SkipDir (do not enter this
// dir, or skip the rest of this dir for a file) or fs.SkipAll; any other error stops the walk.
func (s *Service) Walk(name string, o WalkOptions, fn func(Entry) error) error {
	t, err := s.resolve(name, true, true)
	if err != nil {
		return err
	}
	defer t.close()
	if err := s.requireDir(t, "walk"); err != nil {
		return err
	}
	w := &walker{s: s, t: t, o: o, fn: fn}
	err = w.dir(t.name, "", 1)
	if errors.Is(err, fs.SkipAll) {
		return nil
	}
	return err
}

type walker struct {
	s     *Service
	t     *target
	o     WalkOptions
	fn    func(Entry) error
	files int
}

func (w *walker) dir(dir, relDir string, depth int) error {
	ents, err := readDir(w.t.fsys, dir)
	if err != nil {
		return w.t.fail("walk", err)
	}
	for _, e := range ents {
		rel := e.Name()
		if relDir != "" {
			rel = relDir + "/" + e.Name()
		}
		fi, err := e.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return w.t.fail("walk", err)
		}
		isDir := fi.IsDir()
		if isDir && w.o.SkipDir != nil && w.o.SkipDir(rel) {
			continue
		}
		if !isDir {
			if w.files++; w.o.MaxFiles > 0 && w.files > w.o.MaxFiles {
				return w.t.fail("walk", policy(ErrTooManyFiles, "more than %d files", w.o.MaxFiles))
			}
		}
		err = w.fn(Entry{Name: rel, Info: w.s.infoOf(fi)})
		switch {
		case errors.Is(err, fs.SkipDir) && !isDir:
			return nil
		case errors.Is(err, fs.SkipDir):
			continue
		case err != nil:
			return err
		}
		if isDir && (w.o.MaxDepth == 0 || depth < w.o.MaxDepth) {
			if err := w.dir(filepath.Join(dir, e.Name()), rel, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// Glob returns the names matching pattern: absolute paths host-wide, rel slash names rooted.
func (s *Service) Glob(pattern string) ([]string, error) {
	if s.root == "" {
		if !filepath.IsAbs(pattern) {
			return nil, wrapErr("glob", pattern, policy(ErrInvalidPath, "%q is not absolute", pattern))
		}
		m, err := filepath.Glob(pattern)
		return m, wrapErr("glob", pattern, err)
	}
	r, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, wrapErr("glob", pattern, err)
	}
	defer func() { _ = r.Close() }()
	m, err := fs.Glob(r.FS(), pattern)
	return m, wrapErr("glob", pattern, err)
}
