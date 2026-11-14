package flowharness

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Snapshot is a byte copy of the app home (database, WAL, shm, config files) at one moment.
type Snapshot struct{ files map[string][]byte }

// Snapshot copies every file under SpaceHome while the app runs. Taken between two writes it is what a
// SIGKILL at that point leaves on disk: the main database file, its WAL and shm, no checkpoint.
func (a *App) Snapshot() *Snapshot {
	a.t.Helper()
	s := &Snapshot{files: map[string][]byte{}}
	err := filepath.WalkDir(a.SpaceHome, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(a.SpaceHome, p)
		s.files[rel] = b
		return nil
	})
	if err != nil {
		a.t.Fatalf("snapshot: %v", err)
	}
	return s
}

// CrashTo stops the app, puts the snapshot's files back (every other file under SpaceHome is removed),
// and rebuilds on them, as a relaunch after a crash at the snapshot's moment does. A real kill is
// not used: it would need the whole app out of process, and the snapshot is exact and deterministic.
func (a *App) CrashTo(s *Snapshot) {
	a.t.Helper()
	a.Quit(a.t)
	if err := os.RemoveAll(a.SpaceHome); err != nil {
		a.t.Fatalf("crash: %v", err)
	}
	for rel, b := range s.files {
		p := filepath.Join(a.SpaceHome, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			a.t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			a.t.Fatal(err)
		}
	}
	a.build()
}

// Has reports whether the snapshot holds a non-empty file whose path ends in suffix.
func (s *Snapshot) Has(suffix string) bool {
	for rel, b := range s.files {
		if len(b) > 0 && filepath.Ext(rel) == suffix {
			return true
		}
	}
	return false
}
