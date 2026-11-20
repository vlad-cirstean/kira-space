package fileservice

import (
	"time"

	"github.com/kirathecat/kira-studio/internal/fswatch"
)

// Aliases of the fswatch contract (P699), so consumers import only the service.
type (
	Event     = fswatch.Event
	Op        = fswatch.Op
	Watcher   = fswatch.Watcher
	Debouncer = fswatch.Debouncer
)

// Op bits and the depth sentinel of the fswatch contract.
const (
	Create    = fswatch.Create
	Write     = fswatch.Write
	Remove    = fswatch.Remove
	Rename    = fswatch.Rename
	Meta      = fswatch.Meta
	Unlimited = fswatch.Unlimited
)

// NewDebouncer is fswatch.NewDebouncer.
func NewDebouncer(quiet, maxWait time.Duration, fn func()) *Debouncer {
	return fswatch.NewDebouncer(quiet, maxWait, fn)
}

// WatchBackend names the event source in use ("fsevents" or "fsnotify").
func WatchBackend() string { return fswatch.Backend() }

// WatchOptions describe one watch. Names are existing dirs, checked like every name.
type WatchOptions struct {
	Names    []string
	MaxDepth int
	SkipDir  func(path string) bool
	Latency  time.Duration
}

// Watch watches the dirs named in o (rule 11). Events carry the absolute spelling of the root the
// Service reports: a rooted Service's resolved root joined with the name.
func (s *Service) Watch(o WatchOptions) (*Watcher, error) {
	roots := make([]string, 0, len(o.Names))
	for _, n := range o.Names {
		p, err := s.Path(n)
		if err != nil {
			return nil, err
		}
		roots = append(roots, p)
	}
	return fswatch.Watch(fswatch.Options{Roots: roots, MaxDepth: o.MaxDepth, SkipDir: o.SkipDir, Latency: o.Latency})
}
