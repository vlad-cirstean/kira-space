package fswatch

import (
	"errors"
	"log/slog"
	"os"

	"github.com/fsnotify/fsnotify"
)

// fsnotifyBackend is the event source off macOS, and on macOS without cgo. fsnotify has no
// recursive watch: the scope adds each watched dir.
type fsnotifyBackend struct{ fsw *fsnotify.Watcher }

func startFSNotify(backendOptions) (backend, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &fsnotifyBackend{fsw: fsw}, nil
}

func (b *fsnotifyBackend) add(dir string) error { return b.fsw.Add(dir) }

// remove drops a watch the kernel may already have dropped: a renamed dir keeps its inotify watch.
func (b *fsnotifyBackend) remove(dir string) { _ = b.fsw.Remove(dir) }

func (b *fsnotifyBackend) run(w *Watcher) {
	for {
		select {
		case ev, ok := <-b.fsw.Events:
			if !ok {
				return
			}
			w.handle([]rawEvent{fromFSNotify(ev)})
		case err, ok := <-b.fsw.Errors:
			if !ok {
				return
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				w.handle([]rawEvent{{rescan: true}})
			} else {
				slog.Warn("fswatch: watcher error", "scope", "fswatch", "err", err)
			}
		case w.sink.pendingC() <- Event{Rescan: true}:
			w.sink.flushed()
		case <-w.quit:
			return
		}
	}
}

// stop closes fsw only after run has returned, so run never adds to a closed watcher.
func (b *fsnotifyBackend) stop(done <-chan struct{}) error {
	<-done
	return b.fsw.Close()
}

func fromFSNotify(ev fsnotify.Event) rawEvent {
	var op Op
	for _, m := range [...]struct {
		from fsnotify.Op
		to   Op
	}{{fsnotify.Create, Create}, {fsnotify.Write, Write}, {fsnotify.Remove, Remove}, {fsnotify.Rename, Rename}, {fsnotify.Chmod, Meta}} {
		if ev.Has(m.from) {
			op |= m.to
		}
	}
	r := rawEvent{path: ev.Name, op: op}
	if op&(Create|Rename) != 0 {
		if fi, err := os.Lstat(ev.Name); err == nil && fi.IsDir() {
			r.dir = true
		}
	}
	return r
}
