//go:build unix

package jsengine

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/dop251/goja"
)

// A //# sourceMappingURL comment makes goja os.ReadFile the path unless source maps are disabled.
// The path here is a FIFO: opening it for writing completes only when somebody opens it for
// reading, in this process or in the worker. goja resolves the URL relative to the script name,
// so a bare file name lands in the working directory: os.TempDir() for the worker, so both
// processes point there.
func TestSourceMapNeverRead(t *testing.T) {
	t.Chdir(os.TempDir())
	name := fmt.Sprintf("kira-jsengine-map-%d.json", time.Now().UnixNano())
	fifo := filepath.Join(os.TempDir(), name)
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("no FIFO support:", err)
	}
	opened := make(chan struct{})
	go func() {
		if f, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
			f.Close()
		}
		close(opened)
	}()
	released := false
	release := func() { // unblock the goroutine without a reader of ours counting
		if !released {
			released = true
			if f, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0); err == nil {
				f.Close()
			}
			select {
			case <-opened:
			case <-time.After(2 * time.Second):
			}
		}
	}
	t.Cleanup(func() { os.Remove(fifo) })
	t.Cleanup(release) // runs first

	src := "return 1\n//# sourceMappingURL=" + name
	if e := Check(src); e != nil {
		t.Fatalf("Check: %v", e)
	}
	if got := runOK(t, src); got != "1" {
		t.Fatalf("Exec: %s", got)
	}
	select {
	case <-opened:
		t.Fatal("the source map path was opened")
	case <-time.After(200 * time.Millisecond):
	}
	// Control: goja itself opens it without the option, which is why it is set.
	done := make(chan struct{})
	go func() {
		_, _ = goja.Parse("x", "(function(){\n"+src+"\n})")
		close(done)
	}()
	select {
	case <-opened:
	case <-time.After(2 * time.Second):
		t.Fatal("goja no longer follows sourceMappingURL by default; the option is moot")
	}
	<-done
}
