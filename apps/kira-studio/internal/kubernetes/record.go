package kubernetes

import (
	"context"
	"sync"
)

// Recorder runs one cluster call under an Operations panel row.
type Recorder interface {
	Record(ctx context.Context, command string, fn func(context.Context, *OpNote) error) error
}

// OpNote carries what a recorded call learned while running.
type OpNote struct {
	mu      sync.Mutex
	rows    *int
	command string
}

// SetRows records the result size.
func (n *OpNote) SetRows(k int) {
	n.mu.Lock()
	n.rows = &k
	n.mu.Unlock()
}

// SetCommand replaces the command text the row was started with.
func (n *OpNote) SetCommand(c string) {
	n.mu.Lock()
	n.command = c
	n.mu.Unlock()
}

// Rows returns the recorded result size, nil when none was set.
func (n *OpNote) Rows() *int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.rows
}

// Command returns the replacement command text, empty when none was set.
func (n *OpNote) Command() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.command
}

// WithRecorder routes every cluster call through r.
func WithRecorder(r Recorder) Option { return func(s *Service) { s.rec = r } }

// record runs fn under a row. An unknown cluster returns before any row starts; a background call
// runs without one.
func (s *Service) record(ctx context.Context, clusterID string, background bool, command func(ctxName string) string, fn func(context.Context, *OpNote) error) error {
	_, name, _, err := s.target(clusterID)
	if err != nil {
		return err
	}
	if background || s.rec == nil {
		return fn(ctx, &OpNote{})
	}
	return s.rec.Record(ctx, command(name), fn)
}
