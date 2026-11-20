//go:build darwin && cgo

package fswatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	backends = append(backends, struct {
		name  string
		start startFunc
	}{"fsevents", startFSEvents})
}

func openFDs(t *testing.T) int {
	t.Helper()
	ents, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(ents)
}

// 64 streams over trees of 500 files start and deliver, with no descriptor per file.
func TestFSEventsManyStreamsNoFDPerFile(t *testing.T) {
	const streams, files = 64, 500
	before := openFDs(t)
	roots := make([]string, streams)
	ws := make([]*Watcher, streams)
	for i := range ws {
		roots[i] = t.TempDir()
		for j := range files {
			write(t, filepath.Join(roots[i], fmt.Sprintf("f%d", j)))
		}
		w, err := watch(Options{Roots: []string{roots[i]}, MaxDepth: Unlimited}, startFSEvents)
		if err != nil {
			t.Fatalf("stream %d: %v", i, err)
		}
		ws[i] = w
		defer w.Close()
	}
	if grew := openFDs(t) - before; grew > 64 {
		t.Fatalf("open fds grew by %d across %d watchers of %d files each", grew, streams, files)
	}
	for i, w := range ws {
		p := filepath.Join(roots[i], "new")
		write(t, p)
		waitEvent(t, w, "event on stream", atPath(p, 0))
	}
}

// A root spelled in other case delivers on a case-insensitive volume.
func TestFSEventsCaseInsensitiveRoot(t *testing.T) {
	root := t.TempDir()
	flipped := strings.ToUpper(root)
	if _, err := os.Stat(flipped); err != nil {
		t.Skip("case-sensitive volume")
	}
	w := newWatch(t, startFSEvents, Options{Roots: []string{flipped}, MaxDepth: Unlimited})
	write(t, filepath.Join(root, "f"))
	waitEvent(t, w, "event in caller spelling", atPath(filepath.Join(filepath.Clean(flipped), "f"), 0))
}
