package adapterhost

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
)

// ErrStreamFull is what enqueue returns when the session's queue has no room for a frame right
// now. HandleDataFrame's own dispatch is the only caller of enqueue/enqueueLocal, and enginehost's
// own retry-with-backoff around it (stream.go) has no counterpart here — a locally-produced or
// forwarded frame that finds the queue full is dropped, not retried (enqueueLocal's own comment
// explains why).
var ErrStreamFull = errors.New("adapterhost: stream sink full")

// StreamSession is the whole of what the router needs from a renderer connection — the same
// method set as bridge.StreamSession (A11's per-consumer-interface discipline; this package must
// not import bridge or Wails, the same rule P54 §2 D14 gives enginehost.Sink).
type StreamSession interface {
	Send(frame []byte) error
	Receive() ([]byte, error)
}

const (
	// sessionQueueFrames/sessionQueueBytes match enginehost/stream.go's own dataQueueFrames/
	// dataQueueBytes (64 frames / 32 MiB) — the router's queue replaces that one as the thing
	// backpressure now bounds.
	sessionQueueFrames = 64
	sessionQueueBytes  = 32 << 20

	// maxDataFrameBytes mirrors Wails' own streamMaxFrameBytes, same as enginehost's own copy of
	// this constant. A pathological Go-produced page can approach it (MaxPageSize 10 000 rows,
	// MaxCellBytes 64 KiB) — this is the check that catches it. The FlatBuffers wire format
	// carries raw bytes with no base64 inflation, so this budget is more headroom than it
	// used to be, not less.
	maxDataFrameBytes = 64 << 20

	// sessionMaxInFlightOps bounds HandleDataFrameAsync's own goroutine-per-frame: without
	// a cap, a burst of frames — a renderer bug looping, or many rapid UI actions arriving
	// faster than the driver can answer them — spawns unbounded goroutines, each holding open a
	// driver call. Matches sessionQueueFrames' own order of magnitude: answering more ops than that
	// concurrently would queue their responses behind writeLoop's own bound anyway.
	sessionMaxInFlightOps = 64
)

// Session: one writer goroutine per attached renderer connection, owning a bounded queue
// that HandleDataFrame's locally-produced responses enqueue into (via enqueueLocal). A dedicated
// writer goroutine is what keeps there being exactly one caller of conn.Send, since a blocking
// conn.Send called from two goroutines directly would be a data race waiting to be found on macOS
// (application.StreamConn.Send blocks; TrySend is the non-blocking one).
type Session struct {
	conn StreamSession

	queue       chan []byte
	queuedBytes atomic.Int64
	roomFreed   chan struct{}
	done        chan struct{}
	closeOnce   sync.Once

	// ctx/cancel back handleDataOp's own per-op context: it used to be
	// context.Background(), so an op outlived its session indefinitely — a renderer reload/close
	// abandons the pending request client-side, but nothing ever told the adapter call itself to
	// stop. Close cancels ctx, so a session going away unblocks whatever op is still
	// running against it (a well-behaved adapter observes ctx and returns; CancelOp's explicit,
	// opID-addressed path is unaffected — RunOp derives its own context from this one either way).
	ctx    context.Context
	cancel context.CancelFunc

	// inFlight is HandleDataFrameAsync's own concurrency semaphore — see sessionMaxInFlightOps.
	inFlight chan struct{}
}

func newSession(conn StreamSession) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		conn:      conn,
		queue:     make(chan []byte, sessionQueueFrames),
		roomFreed: make(chan struct{}, 1),
		done:      make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
		inFlight:  make(chan struct{}, sessionMaxInFlightOps),
	}
	go s.writeLoop()
	return s
}

// acquireSlot blocks until an in-flight op slot is free, or the session closes — whichever comes
// first. Returns false in the latter case, in which case there is no slot to release.
func (s *Session) acquireSlot() bool {
	select {
	case s.inFlight <- struct{}{}:
		return true
	case <-s.done:
		return false
	}
}

// releaseSlot must be called exactly once for every acquireSlot call that returned true.
func (s *Session) releaseSlot() {
	<-s.inFlight
}

func (s *Session) writeLoop() {
	for {
		select {
		case frame, ok := <-s.queue:
			if !ok {
				return
			}
			s.queuedBytes.Add(-int64(len(frame)))
			// Non-blocking: enqueueResponse's own wait loop below only ever needs the most recent
			// signal, and a writeLoop that blocked here waiting for a slow waiter to notice would
			// itself become the bottleneck it exists to avoid.
			select {
			case s.roomFreed <- struct{}{}:
			default:
			}
			if err := s.conn.Send(frame); err != nil {
				s.Close()
				// Close the conn too so the renderer sees onclose rather than silent no-answer (F5,
				// P168 Part 5); *application.StreamConn has Close, the interface stays Wails-free.
				if c, ok := s.conn.(interface{ Close() error }); ok {
					_ = c.Close()
				}
				return
			}
		case <-s.done:
			return
		}
	}
}

// enqueueLocal is HandleDataFrame's own path for an unsolicited event frame this process produced
// itself (a cache:stats push) — not correlated with any renderer-side pending request, so a full
// queue can safely drop it: the next stats-changed notification supersedes it, and nothing is
// left waiting on this one specifically.
func (s *Session) enqueueLocal(frame []byte) {
	if err := s.enqueue(frame); err != nil {
		slog.Warn("adapterhost: dropping an event frame, session queue full", "scope", "adapterhost")
	}
}

// enqueueResponse is HandleDataFrame's own path for a response frame, correlated by id with a
// renderer-side pending request (port.ts's `pending` map). Unlike enqueueLocal's events, silently
// dropping this frame would leave that request unanswered forever: a data op's request has no
// client-side timeout of its own (cancellation is meant to be the only escape hatch),
// and cancellation itself only resolves the pending promise by producing a response through this
// same path. So this never drops: it blocks for room in the queue instead (safe because
// HandleDataFrame already runs on its own goroutine per inbound frame — this never serialises
// behind the next Receive()), unblocking either once the writer drains the queue or once the
// session closes, at which point port.ts's own onclose already rejects every pending request.
//
// "room in the queue" used to mean only the channel's own sessionQueueFrames
// (64) capacity — a burst of large data-plane pages, each well under maxDataFrameBytes on its
// own, could sit in the queue simultaneously with nothing checking their combined size against
// sessionQueueBytes (32 MiB) at all, since the byte counter was updated but never consulted here.
// The wait loop below mirrors enqueue's own check (current queuedBytes against the budget, not
// queuedBytes-plus-this-frame — a single frame up to maxDataFrameBytes must still get through
// while the queue is otherwise empty, rather than deadlocking against a budget smaller than the
// frame itself) before attempting the channel send, parking on roomFreed in between attempts
// instead of busy-polling.
func (s *Session) enqueueResponse(frame []byte) {
	for {
		if s.queuedBytes.Load() < sessionQueueBytes {
			select {
			case s.queue <- frame:
				s.queuedBytes.Add(int64(len(frame)))
				return
			case <-s.done:
				return
			}
		}
		select {
		case <-s.roomFreed:
		case <-s.done:
			return
		}
	}
}

func (s *Session) enqueue(frame []byte) error {
	if len(frame) > maxDataFrameBytes {
		slog.Error("adapterhost: dropping oversized data frame", "scope", "adapterhost",
			"bytes", len(frame), "limit", maxDataFrameBytes)
		return nil
	}
	if s.queuedBytes.Load() >= sessionQueueBytes {
		return ErrStreamFull
	}
	select {
	case s.queue <- frame:
		s.queuedBytes.Add(int64(len(frame)))
		return nil
	default:
		return ErrStreamFull
	}
}

// Close stops this session's writer goroutine and cancels every op still running against it
// (ctx, above). Idempotent.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.cancel()
	})
}
