// Package fswatch is the one file-watching seam: every directory watch in the app goes through
// Watch, on FSEvents (macOS with cgo) or fsnotify (everywhere else). Leaf package: no app imports.
package fswatch

import (
	"fmt"
	"sync"
	"time"
)

// Op is a hint about what happened to a path. FSEvents coalesces several operations into one
// event, so test with Has, never ==, and stat the path for its current state.
type Op uint8

const (
	Create Op = 1 << iota
	Write
	Remove
	Rename
	Meta // chmod, owner, xattr, inode metadata
)

// Has reports whether o carries any bit of x.
func (o Op) Has(x Op) bool { return o&x != 0 }

// Event is one change under a watched root.
type Event struct {
	Path   string // caller's spelling of the matched root plus the relative path, Clean and NFC; "" when Rescan
	Op     Op     // hint only
	Dir    bool   // FSEvents ItemIsDir; fsnotify: Lstat on Create and Rename, false otherwise
	Rescan bool   // something changed, unknown what: rescan everything you care about
}

// Unlimited is the MaxDepth for no depth bound.
const Unlimited = -1

// Options describe one watch.
type Options struct {
	Roots    []string               // existing directories; nested roots allowed
	MaxDepth int                    // deepest watched dir below a root (root is 0); Unlimited for none
	SkipDir  func(path string) bool // dir below a root, caller spelling (Clean, NFC): true = not watched, subtree dropped; nil = none. Runs on the watcher goroutine: keep it cheap, never call Close from it.
	Latency  time.Duration          // FSEvents coalescing latency; fsnotify ignores it; 0 = deliver at once
}

// Watch starts watching o.Roots. Contract:
//
//   - Roots must exist and be directories, else Watch fails (wrapping fs.ErrNotExist for a missing
//     one). Roots resolve through symlinks for the backend, but events carry the caller's spelling:
//     callers compare against the paths they passed and never resolve symlinks themselves. On macOS
//     the root prefix also matches case-insensitively.
//   - A dir below a root is decided once: watched iff its depth is within MaxDepth, every ancestor
//     below the root is watched, and SkipDir says no. The verdict holds until the dir sees Create,
//     Remove or Rename, or a Rescan. An event is delivered iff its path is a root or its parent is a
//     root or a watched dir, so a skipped dir's own Create still arrives.
//   - A new dir delivers one event for itself. Files made in it before the backend watched it may
//     raise none: walk the dir if its contents matter.
//   - Dropped events (backend overflow or drop flags, or a full output buffer of 1024) become one
//     Event{Rescan: true}, sent after the watch is rebuilt to cover the current tree.
//   - Close is idempotent and safe from any goroutine, the reading one included. On return no
//     goroutine of the watcher runs. Events then yields what is already buffered and closes.
func Watch(o Options) (*Watcher, error) { return watch(o, defaultBackend) }

// Backend names the event source this build uses ("fsevents" or "fsnotify"), for one log line per
// consumer: a darwin build that falls back to fsnotify (CGO_ENABLED=0) says so out loud.
func Backend() string { return backendName }

// Watcher is one running watch.
type Watcher struct {
	sc   *scope
	be   backend
	sink *sink

	quit chan struct{} // closed first by Close: forwarding stops
	done chan struct{} // closed when the watcher goroutine has returned

	once     sync.Once
	closeErr error
}

// rawEvent is a backend event before scope: path as the OS reported it.
type rawEvent struct {
	path   string
	op     Op
	dir    bool
	rescan bool
}

type backendOptions struct {
	roots   []string // top-level resolved roots; the scope adds every watched dir through add
	latency time.Duration
}

type startFunc func(backendOptions) (backend, error)

// backend is the OS event source. run is the watcher goroutine's body: it is the sole reader of the
// source, feeds batches to w.handle, serves w.sink.pendingC, and returns once w.quit closes (or, for a
// stream that must keep draining, once stop says so). stop ends the source and returns after run
// has exited (done closed).
type backend interface {
	run(w *Watcher)
	add(dir string) error
	remove(dir string)
	stop(done <-chan struct{}) error
}

const outBuffer = 1024

func watch(o Options, start startFunc) (*Watcher, error) {
	sc, err := newScope(o)
	if err != nil {
		return nil, err
	}
	be, err := start(backendOptions{roots: sc.topRoots(), latency: o.Latency})
	if err != nil {
		return nil, fmt.Errorf("fswatch: start %s: %w", backendName, err)
	}
	sc.add, sc.remove = be.add, be.remove
	w := &Watcher{sc: sc, be: be, sink: newSink(outBuffer), quit: make(chan struct{}), done: make(chan struct{})}
	// The goroutine takes the initial walk so the scope has one owner; Watch returns once the walk
	// is done, so a change made right after Watch is seen.
	ready := make(chan error, 1)
	go func() {
		defer close(w.done)
		defer close(w.sink.out)
		ready <- sc.walkAll()
		be.run(w)
	}()
	if err := <-ready; err != nil {
		_ = w.Close()
		return nil, err
	}
	return w, nil
}

// Events yields changes; it closes after Close.
func (w *Watcher) Events() <-chan Event { return w.sink.out }

// Close stops the watch; see Watch.
func (w *Watcher) Close() error {
	w.once.Do(func() {
		close(w.quit)
		w.closeErr = w.be.stop(w.done)
	})
	return w.closeErr
}

// handle applies scope to a batch and queues the survivors. Never blocks.
func (w *Watcher) handle(batch []rawEvent) {
	for _, r := range batch {
		if r.rescan {
			w.sc.rescan() // rebuild the watch first: after a Rescan it covers the current tree
			w.sink.offer(Event{Rescan: true})
			continue
		}
		if ev, ok := w.sc.admit(r); ok {
			w.sink.offer(ev)
		}
	}
}
