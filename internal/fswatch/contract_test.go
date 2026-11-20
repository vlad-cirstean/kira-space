package fswatch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
)

// backends lists every backend compiled in; darwin_test.go adds FSEvents.
var backends = []struct {
	name  string
	start startFunc
}{{"fsnotify", startFSNotify}}

func eachBackend(t *testing.T, fn func(t *testing.T, start startFunc)) {
	t.Helper()
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) { fn(t, b.start) })
	}
}

func newWatch(t *testing.T, start startFunc, o Options) *Watcher {
	t.Helper()
	w, err := watch(o, start)
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func waitEvent(t *testing.T, w *Watcher, what string, pred func(Event) bool) Event {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-w.Events():
			if !ok {
				t.Fatalf("events closed waiting for %s", what)
			}
			if pred(ev) {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// waitEvents waits until every predicate has matched some event, in any order: kqueue reports the
// directory change that creates a rename target and the file's own rename in no fixed order.
func waitEvents(t *testing.T, w *Watcher, what string, preds map[string]func(Event) bool) {
	t.Helper()
	left := map[string]func(Event) bool{}
	for k, p := range preds {
		left[k] = p
	}
	deadline := time.After(10 * time.Second)
	for len(left) > 0 {
		select {
		case ev, ok := <-w.Events():
			if !ok {
				t.Fatalf("events closed waiting for %s", what)
			}
			for k, p := range left {
				if p(ev) {
					delete(left, k)
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s: still missing %d of %d", what, len(left), len(preds))
		}
	}
}

func atPath(p string, op Op) func(Event) bool {
	return func(ev Event) bool { return ev.Path == p && (op == 0 || ev.Op.Has(op)) }
}

var sentinelSeq atomic.Int64

// collect writes a sentinel file in root and returns every event seen before it: events keep
// order on both backends, so what is absent from the result did not arrive.
func collect(t *testing.T, w *Watcher, root string) []Event {
	t.Helper()
	s := filepath.Join(root, fmt.Sprintf("sentinel-%d", sentinelSeq.Add(1)))
	write(t, s)
	var seen []Event
	waitEvent(t, w, "sentinel", func(ev Event) bool {
		if ev.Path == s {
			return true
		}
		seen = append(seen, ev)
		return false
	})
	return seen
}

func write(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdirs(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestFileOpsInRoot(t *testing.T) {
	eachBackend(t, func(t *testing.T, start startFunc) {
		root := t.TempDir()
		w := newWatch(t, start, Options{Roots: []string{root}, MaxDepth: Unlimited})
		a, b := filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")
		write(t, a)
		waitEvent(t, w, "create", atPath(a, Create|Write))
		if err := os.Rename(a, b); err != nil {
			t.Fatal(err)
		}
		waitEvents(t, w, "rename source and target", map[string]func(Event) bool{
			"source": atPath(a, Rename|Remove), "target": atPath(b, 0),
		})
		if err := os.Remove(b); err != nil {
			t.Fatal(err)
		}
		waitEvent(t, w, "remove", atPath(b, Remove))
	})
}

func TestNewDirWithinDepth(t *testing.T) {
	eachBackend(t, func(t *testing.T, start startFunc) {
		root := t.TempDir()
		w := newWatch(t, start, Options{Roots: []string{root}, MaxDepth: 3})
		top, deep := filepath.Join(root, "a"), filepath.Join(root, "a", "b")
		mkdirs(t, deep) // a burst: b may exist before a is watched, so only a is certain to announce itself
		ev := waitEvent(t, w, "new dir", atPath(top, Create))
		if !ev.Dir {
			t.Errorf("Dir not set on a dir create: %+v", ev)
		}
		f := filepath.Join(deep, "f")
		write(t, f)
		waitEvent(t, w, "file in new dir", atPath(f, 0))
		write(t, f)
		waitEvent(t, w, "second write in new dir", atPath(f, Write))
	})
}

func TestDepthAndSkip(t *testing.T) {
	eachBackend(t, func(t *testing.T, start startFunc) {
		root := t.TempDir()
		skip := func(p string) bool { return filepath.Base(p) == "skip" }
		w := newWatch(t, start, Options{Roots: []string{root}, MaxDepth: 1, SkipDir: skip})
		d1, d2, sk := filepath.Join(root, "d1"), filepath.Join(root, "d1", "d2"), filepath.Join(root, "skip")
		mkdirs(t, d1)
		waitEvent(t, w, "d1", atPath(d1, Create))
		mkdirs(t, d2)
		waitEvent(t, w, "d2 own create", atPath(d2, Create)) // parent d1 is watched
		mkdirs(t, sk)
		waitEvent(t, w, "skipped dir own create", atPath(sk, Create))
		write(t, filepath.Join(d2, "f"))
		write(t, filepath.Join(sk, "f"))
		for _, ev := range collect(t, w, root) {
			if ev.Path == filepath.Join(d2, "f") || ev.Path == filepath.Join(sk, "f") {
				t.Errorf("event beyond depth or under skipped dir: %+v", ev)
			}
		}
	})
}

func TestSymlinkedRootKeepsCallerSpelling(t *testing.T) {
	eachBackend(t, func(t *testing.T, start startFunc) {
		real := t.TempDir()
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		w := newWatch(t, start, Options{Roots: []string{link}, MaxDepth: Unlimited})
		write(t, filepath.Join(real, "f"))
		waitEvent(t, w, "event in link spelling", atPath(filepath.Join(link, "f"), 0))
	})
}

func TestNestedRootsAttributeToDeepest(t *testing.T) {
	eachBackend(t, func(t *testing.T, start startFunc) {
		root := t.TempDir()
		sub := filepath.Join(root, "sub")
		mkdirs(t, sub)
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(sub, link); err != nil {
			t.Fatal(err)
		}
		w := newWatch(t, start, Options{Roots: []string{root, link}, MaxDepth: 0})
		write(t, filepath.Join(sub, "f"))
		waitEvent(t, w, "deepest root spelling", atPath(filepath.Join(link, "f"), 0))
		for _, ev := range collect(t, w, root) {
			if ev.Path == filepath.Join(sub, "f") {
				t.Errorf("event attributed to the outer root: %+v", ev)
			}
		}
	})
}

func TestNFDRootReportsNFC(t *testing.T) {
	eachBackend(t, func(t *testing.T, start startFunc) {
		nfd := "café" // e + combining acute
		root := filepath.Join(t.TempDir(), nfd)
		mkdirs(t, root)
		w := newWatch(t, start, Options{Roots: []string{root}, MaxDepth: Unlimited})
		write(t, filepath.Join(root, "f"))
		want := filepath.Join(norm.NFC.String(filepath.Dir(root)), norm.NFC.String(nfd), "f")
		waitEvent(t, w, "NFC path", atPath(want, 0))
	})
}

func TestMissingRootFails(t *testing.T) {
	eachBackend(t, func(t *testing.T, start startFunc) {
		_, err := watch(Options{Roots: []string{filepath.Join(t.TempDir(), "nope")}, MaxDepth: 0}, start)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("err = %v, want fs.ErrNotExist", err)
		}
		if _, err := watch(Options{}, start); err == nil {
			t.Fatal("no roots accepted")
		}
	})
}

func TestRootRemoved(t *testing.T) {
	eachBackend(t, func(t *testing.T, start startFunc) {
		root := filepath.Join(t.TempDir(), "r")
		mkdirs(t, root)
		w := newWatch(t, start, Options{Roots: []string{root}, MaxDepth: Unlimited})
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		waitEvent(t, w, "root event or rescan", func(ev Event) bool { return ev.Rescan || ev.Path == root })
	})
}

func TestCloseIdempotentAndFromReader(t *testing.T) {
	eachBackend(t, func(t *testing.T, start startFunc) {
		root := t.TempDir()
		w := newWatch(t, start, Options{Roots: []string{root}, MaxDepth: Unlimited})
		closed := make(chan struct{})
		go func() { // the reading goroutine closes on its first event
			<-w.Events()
			_ = w.Close()
			close(closed)
		}()
		write(t, filepath.Join(root, "f"))
		select {
		case <-closed:
		case <-time.After(10 * time.Second):
			t.Fatal("Close from the reading goroutine did not return")
		}
		if err := w.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
		for range w.Events() { // buffered leftovers, then closed
		}
	})
}

func TestCloseWhileConsumerStalled(t *testing.T) {
	eachBackend(t, func(t *testing.T, start startFunc) {
		root := t.TempDir()
		w := newWatch(t, start, Options{Roots: []string{root}, MaxDepth: Unlimited})
		for i := range 1500 { // more than the output buffer, nobody reading
			write(t, filepath.Join(root, fmt.Sprintf("f%d", i)))
		}
		done := make(chan struct{})
		go func() { _ = w.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatal("Close blocked on a stalled consumer")
		}
		for range w.Events() {
		}
	})
}
