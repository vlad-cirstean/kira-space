package configfs

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
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
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("configfs: watcher: %w", err)
	}
	if err := w.Add(parent); err != nil {
		w.Close()
		return nil, fmt.Errorf("configfs: watch %s: %w", parent, err)
	}
	// Add after the parent: a dir created in between is caught by the parent's event.
	_ = w.Add(dir)

	done := make(chan struct{})
	d := &debouncer{fn: onChange}
	go func() {
		defer close(done)
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if relevantEvent(dir, ext, ev) {
					if filepath.Clean(ev.Name) == dir && ev.Has(fsnotify.Create) {
						_ = w.Add(dir)
					}
					d.schedule()
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.Warn("config watch", "scope", "config", "dir", dir, "err", err)
			}
		}
	}()
	return func() {
		d.stop()
		w.Close()
		<-done
	}, nil
}

// relevantEvent reports whether ev touches dir itself or a visible *<ext> file in it.
func relevantEvent(dir, ext string, ev fsnotify.Event) bool {
	name := filepath.Clean(ev.Name)
	if name == dir {
		return true
	}
	base := filepath.Base(name)
	return filepath.Dir(name) == dir && strings.HasSuffix(base, ext) && !strings.HasPrefix(base, ".") && base != DocsFile
}

// debouncer coalesces bursts into one fn call after watchDebounce and drops calls once stopped.
type debouncer struct {
	fn      func()
	mu      sync.Mutex
	timer   *time.Timer
	stopped bool
	running sync.WaitGroup // in-flight fn calls; stop waits for them
}

func (d *debouncer) schedule() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(watchDebounce, d.fire)
}

func (d *debouncer) fire() {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	d.running.Add(1)
	d.mu.Unlock()
	defer d.running.Done()
	d.fn()
}

func (d *debouncer) stop() {
	d.mu.Lock()
	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
	}
	d.mu.Unlock()
	d.running.Wait()
}
