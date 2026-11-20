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

// startStreams starts n watchers over fresh roots of files files each.
func startStreams(t *testing.T, n, files int) ([]string, []*Watcher) {
	t.Helper()
	roots := make([]string, n)
	ws := make([]*Watcher, n)
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
		t.Cleanup(func() { _ = w.Close() })
	}
	return roots, ws
}

// 64 streams over trees of 500 files start and deliver. A stream holds a fixed handful of
// descriptors in the OS framework (about 11), so the bound is the measured cost of a stream over an
// empty root: descriptors per file would blow past it.
func TestFSEventsManyStreamsNoFDPerFile(t *testing.T) {
	const probe, streams, files = 8, 64, 500
	before := openFDs(t)
	startStreams(t, probe, 0)
	mid := openFDs(t)
	perStream := (mid - before + probe - 1) / probe
	roots, ws := startStreams(t, streams, files)
	if grew, limit := openFDs(t)-mid, streams*(perStream+1); grew > limit {
		t.Fatalf("open fds grew by %d across %d watchers of %d files each, want at most %d (%d per empty stream)", grew, streams, files, limit, perStream)
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
