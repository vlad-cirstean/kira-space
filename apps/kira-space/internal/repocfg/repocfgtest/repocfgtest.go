// Package repocfgtest builds repo config stores and writes repo files for tests.
package repocfgtest

import (
	"database/sql"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// New starts a Store over db with its folder in dir, with no watch: tests reload through Set.
func New(t testing.TB, db *sql.DB, dir string) *repocfg.Store {
	t.Helper()
	s, err := repocfg.New(repocfg.Options{DB: db, Dir: dir})
	if err != nil {
		t.Fatalf("repocfg.New: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("repocfg.Start: %v", err)
	}
	return s
}

// Set writes the file of the repo at path: the file it has, or a new one. edit changes the current
// File (the zero File with that path when there is none). The load runs before Set returns.
func Set(t testing.TB, s *repocfg.Store, path string, edit func(*repocfg.File)) {
	t.Helper()
	svc := &repocfg.Service{Store: s}
	file := s.FileFor(path)
	f := repocfg.File{Path: repocfg.Key(path)}
	args := repocfg.SaveArgs{Path: path, File: file}
	if file != "" {
		v, err := svc.File(repocfg.FileArgs{File: file})
		if err != nil {
			t.Fatalf("repocfgtest: read %s: %v", file, err)
		}
		args.BaseHash = v.Hash
		cur, ok := s.Value(file)
		if !ok {
			t.Fatalf("repocfgtest: %s has no value", file)
		}
		f = cur
	}
	edit(&f)
	text, err := configfs.RenderYAML(f, "")
	if err != nil {
		t.Fatalf("repocfgtest: render: %v", err)
	}
	args.Text = string(text)
	if _, err := svc.SaveFile(args); err != nil {
		t.Fatalf("repocfgtest: save %s: %v", path, err)
	}
}
