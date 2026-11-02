package harness

import (
	"errors"
	"os"
	"path/filepath"
)

// Isolator is a provider whose hermetic TUI launch needs files on disk. Optional role.
type Isolator interface {
	// Isolate fills dir (empty, Kira-owned) for one hermetic launch; environ is the launch's base env.
	Isolate(dir string, environ []string) error
}

// IsolationGlob is the leftover pattern a start-up sweep removes.
const IsolationGlob = "*-iso"

// PrepareIsolation makes <parent>/<stem>-iso (0700) and has p fill it; dir "" and a no-op release
// when p is no Isolator. Release removes the dir; safe twice.
func PrepareIsolation(p Provider, parent, stem string, environ []string) (dir string, release func(), err error) {
	iso, ok := p.(Isolator)
	if !ok {
		return "", func() {}, nil
	}
	if parent == "" {
		return "", func() {}, errors.New("harness: no folder for the isolation dir")
	}
	dir = filepath.Join(parent, stem+"-iso")
	release = func() { _ = os.RemoveAll(dir) }
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", func() {}, err
	}
	if err := iso.Isolate(dir, environ); err != nil {
		release()
		return "", func() {}, err
	}
	return dir, release, nil
}
