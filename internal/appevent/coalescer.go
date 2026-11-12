package appevent

import (
	"sync"
	"time"
)

// Coalescer batches pushed items and flushes the accumulated batch on whichever comes first: an
// accumulated size (sizeOf-weighted) threshold, an interval timer, or Finish — the shape
// bridge/terminal.go's terminalCoalescer, bridge/grpc.go's grpcCoalescer and
// bridge/codeworkspace.go's searchCoalescer each restated by hand for their own payload type.
// sizeOf lets one accumulator serve all three: a message counts as 1 (grpcCoalescer),
// a file-match group counts by its own match count (searchCoalescer), a terminal chunk counts by
// its byte length (terminalCoalescer) — each push is still exactly one item, one call.
//
// F is whatever "final" extra payload only the terminal flush carries (an exit code, a call
// result, search stats) — threaded through Finish's own parameter rather than a field a flush
// closure reads back later, so a final payload is race-free with no lock of its own beyond this
// type's.
type Coalescer[T, F any] struct {
	interval time.Duration
	maxSize  int
	sizeOf   func(T) int
	flush    func(batch []T, done bool, final F)

	// flushMu orders flushes: taken under mu, held across the flush call after mu drops, so Push
	// and the timer never wait on an emit in progress. Lock order is mu then flushMu.
	flushMu sync.Mutex
	mu      sync.Mutex
	pending []T
	size    int
	timer   *time.Timer
	done    bool
}

// NewCoalescer constructs a Coalescer that flushes when accumulated size (summed via sizeOf over
// every item pushed since the last flush) reaches maxSize, or interval elapses since the first
// unflushed push — whichever comes first.
func NewCoalescer[T, F any](interval time.Duration, maxSize int, sizeOf func(T) int, flush func(batch []T, done bool, final F)) *Coalescer[T, F] {
	return &Coalescer[T, F]{interval: interval, maxSize: maxSize, sizeOf: sizeOf, flush: flush}
}

// Push adds one item, flushing immediately (done=false) once accumulated size crosses maxSize. A
// no-op after Finish.
func (c *Coalescer[T, F]) Push(item T) {
	c.mu.Lock()
	if c.done {
		c.mu.Unlock()
		return
	}
	c.pending = append(c.pending, item)
	c.size += c.sizeOf(item)
	if c.size >= c.maxSize {
		var zero F
		c.flushAndUnlock(false, zero)
		return
	}
	if c.timer == nil {
		c.timer = time.AfterFunc(c.interval, c.onTimer)
	}
	c.mu.Unlock()
}

func (c *Coalescer[T, F]) onTimer() {
	c.mu.Lock()
	if c.done || len(c.pending) == 0 {
		c.mu.Unlock()
		return
	}
	var zero F
	c.flushAndUnlock(false, zero)
}

// Finish is the terminal flush, carrying final — always sent, even with nothing pending, so a
// caller's own "running"/"searching" state can never strand. Safe to call more than once; only the
// first call has effect.
func (c *Coalescer[T, F]) Finish(final F) {
	c.mu.Lock()
	if c.done {
		c.mu.Unlock()
		return
	}
	c.done = true
	c.flushAndUnlock(true, final)
}

// flushAndUnlock takes the batch under mu, queues for flushMu, releases mu, then emits. Flushes
// therefore start in the order their batches were taken. Caller holds mu.
func (c *Coalescer[T, F]) flushAndUnlock(done bool, final F) {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	batch := c.pending
	if batch == nil {
		batch = []T{}
	}
	c.pending = nil
	c.size = 0
	c.flushMu.Lock()
	c.mu.Unlock()
	defer c.flushMu.Unlock()
	c.flush(batch, done, final)
}
