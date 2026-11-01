package configfs

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func waitFire(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("no onChange within 5s")
	}
}

func expectQuiet(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("unexpected onChange")
	case <-time.After(3 * watchDebounce):
	}
}

func startWatch(t *testing.T, dir string) <-chan struct{} {
	t.Helper()
	ch := make(chan struct{}, 16)
	stop, err := WatchDir(dir, ".yaml", func() { ch <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	return ch
}

func TestWatch_burstsAndFiltering(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ch := startWatch(t, dir)
	f := filepath.Join(dir, "a.yaml")

	for i := 0; i < 5; i++ {
		if err := os.WriteFile(f, []byte{byte('0' + i)}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	waitFire(t, ch)
	expectQuiet(t, ch)

	if err := os.WriteFile(filepath.Join(dir, ".a.yaml.tmp-1"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	expectQuiet(t, ch)

	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	waitFire(t, ch)
}

func TestWatch_dirCreatedLater(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "workflows")
	ch := startWatch(t, dir)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	waitFire(t, ch)
	expectQuiet(t, ch)
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFire(t, ch)
}

func TestWatch_atomicWriteFiresOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "workflows")
	ch := startWatch(t, dir)
	if err := WriteAtomic(filepath.Join(dir, "flow.yaml"), []byte("a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFire(t, ch)
	expectQuiet(t, ch)
}

func TestDebouncer_stopWaitsForInFlightCall(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var finished atomic.Bool
	d := &debouncer{fn: func() {
		close(started)
		<-release
		finished.Store(true)
	}}
	d.schedule()
	waitFire(t, started)
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(release)
	}()
	d.stop()
	if !finished.Load() {
		t.Fatal("stop returned before the in-flight onChange finished")
	}
}
