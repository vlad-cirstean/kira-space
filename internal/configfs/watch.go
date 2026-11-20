package configfs

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/kirathecat/kira-studio/internal/fswatch"
)

const watchDebounce = 200 * time.Millisecond

// Watch calls onChange when a file of the folder changes; see WatchDir.
func (f *Folder[T]) Watch(onChange func()) (stop func(), err error) {
	return WatchDir(f.Dir, f.Ext, onChange)
}

// WatchDir calls onChange (debounced 200ms, one call per burst) when a *<ext> file in dir is created,
// written, renamed or removed. Hidden files (temp files) and AGENTS.md are ignored. A missing dir is
// fine: its parent is watched until the dir appears, then onChange fires once for it. stop ends the
// watch and returns after any in-flight onChange finishes.
func WatchDir(dir, ext string, onChange func()) (stop func(), err error) {
	dir = filepath.Clean(dir)
	parent := filepath.Dir(dir)
	// fswatch reports Clean, NFC paths; compare against that spelling.
	ndir := norm.NFC.String(dir)
	// MaxDepth 1 with only dir let through: the seam adds dir when it appears, so a missing dir is fine.
	w, err := fswatch.Watch(fswatch.Options{Roots: []string{parent}, MaxDepth: 1, SkipDir: func(p string) bool { return p != ndir }})
	if err != nil {
		return nil, fmt.Errorf("configfs: watch %s: %w", parent, err)
	}

	done := make(chan struct{})
	d := fswatch.NewDebouncer(watchDebounce, 0, onChange)
	go func() {
		defer close(done)
		for ev := range w.Events() {
			if relevantEvent(ndir, ext, ev) {
				d.Schedule()
			}
		}
	}()
	return func() {
		d.Stop()
		_ = w.Close()
		<-done
	}, nil
}

// relevantEvent reports whether ev touches dir itself or a visible *<ext> file in it; a Rescan always does.
func relevantEvent(dir, ext string, ev fswatch.Event) bool {
	if ev.Rescan || ev.Path == dir {
		return true
	}
	base := filepath.Base(ev.Path)
	return filepath.Dir(ev.Path) == dir && strings.HasSuffix(base, ext) && !strings.HasPrefix(base, ".") && base != DocsFile
}
