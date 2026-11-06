package shell

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// QuitGate lets a quit ask first when agents or runs are live.
type QuitGate interface {
	// Live counts the work a quit would stop.
	Live() int
	// Ask raises the confirmation popup; count is the Live value that triggered it.
	Ask(count int)
	// Withdraw drops the popup.
	Withdraw()
}

// QuitStatus is what the confirmation dialog shows: the live work a quit would stop.
type QuitStatus struct {
	Asking  bool `json:"asking"`
	Agents  int  `json:"agents"`
	Runs    int  `json:"runs"`
	Scripts int  `json:"scripts"`
}

// Total is the number of live items.
func (s QuitStatus) Total() int { return s.Agents + s.Runs + s.Scripts }

// Quitter is the Go analogue of src/main/index.ts:151-163's before-quit handler. It is wired
// three ways: application.Options.ShouldQuit (which covers Cmd+Q, the Apple menu, the Dock and
// App.Quit() alike), application.Options.OnShutdown, and the menu's own Quit item.
//
// Waits for every live window's flush ack, not just the first — restoring
// 18fe7bb^:src/main/index.ts:47-60's Promise.all-over-every-window behaviour, which the original
// Go port collapsed to a single sync.Once-closed channel. With two windows open, the old
// shape let window A's ack release the wait while window B's own tabsSave was still in flight,
// racing teardown's db.Close() against it.
type Quitter struct {
	events         Signaller
	beforeFlush    func() // sync.OnceFunc: metrics ticker Stop (index.ts:156)
	teardown       func() // sync.OnceFunc: the ordered shutdown
	timeout        time.Duration
	liveWindowKeys func() []string // seeds the pending set at the moment quitting starts

	app     atomic.Pointer[application.App] // written by Attach on main, read from other goroutines
	started atomic.Bool
	done    atomic.Bool
	// asking is true while the gate's popup is up.
	asking atomic.Bool
	gate   atomic.Pointer[QuitGate]

	mu        sync.Mutex
	pending   map[string]struct{} // window keys still owing an ack; nil until flushThenQuit runs
	flushed   chan struct{}       // closed once, when the pending set first empties
	closeOnce sync.Once
}

// NewQuitter takes the two teardown halves already wrapped in sync.OnceFunc by the caller, so
// main.go states the order in one place. flushTimeout is index.ts's FLUSH_TIMEOUT_MS, and it
// remains one single cap for the whole handshake, not one per window — matching Electron's own
// single FLUSH_TIMEOUT_MS. liveWindowKeys is read once, at the moment quitting starts
// (flushThenQuit), not at construction time — main.go passes its shell.WindowRegistry's own Keys
// method. events is a Signaller — each app's own *bridge.Events satisfies it by embedding
// *appevent.Events.
func NewQuitter(events Signaller, beforeFlush, teardown func(), flushTimeout time.Duration, liveWindowKeys func() []string) *Quitter {
	return &Quitter{
		events:         events,
		beforeFlush:    beforeFlush,
		teardown:       teardown,
		timeout:        flushTimeout,
		liveWindowKeys: liveWindowKeys,
		flushed:        make(chan struct{}),
	}
}

// SetGate makes a quit with live work ask first. Call it once, after construction.
func (q *Quitter) SetGate(g QuitGate) { q.gate.Store(&g) }

func (q *Quitter) gateOf() QuitGate {
	if g := q.gate.Load(); g != nil {
		return *g
	}
	return nil
}

// Asking reports whether the confirmation popup is up.
func (q *Quitter) Asking() bool { return q.asking.Load() }

// ConfirmQuit is the popup's "Quit anyway": the quit proceeds without asking again. It does nothing
// when no popup is up.
func (q *Quitter) ConfirmQuit() {
	if !q.asking.Swap(false) {
		return
	}
	if g := q.gateOf(); g != nil {
		g.Withdraw()
	}
	q.start()
}

// CancelQuit drops the popup; a later quit asks again.
func (q *Quitter) CancelQuit() {
	if !q.asking.Swap(false) {
		return
	}
	if g := q.gateOf(); g != nil {
		g.Withdraw()
	}
}

func (q *Quitter) start() {
	if q.started.CompareAndSwap(false, true) {
		go q.flushThenQuit()
	}
}

// Attach supplies the app once application.New has returned. ShouldQuit is passed to
// application.New as a method value before that, which is why the app cannot be a constructor
// argument.
func (q *Quitter) Attach(app *application.App) {
	q.app.Store(app)
}

// ShouldQuit is application.Options.ShouldQuit. It NEVER blocks: the renderer's ack is
// itself a bound call, which arrives through the main thread (application_darwin.go:431's
// processURLRequest), so a handler that waited here would deadlock the very ack it waits for and
// guarantee the timeout — losing exactly the debounced tab save the handshake exists to protect
// (apps/kira-space/frontend/src/studio/state/tabs.ts:131-137 awaits a tabsSave round trip before acking).
func (q *Quitter) ShouldQuit() bool {
	if q.done.Load() {
		return true
	}
	if g := q.gateOf(); g != nil && !q.started.Load() {
		if n := g.Live(); n > 0 {
			// Re-ask when already asking: the user may have closed the popup window; Ask is idempotent.
			q.asking.Store(true)
			go g.Ask(n)
			return false
		}
		if q.asking.Swap(false) {
			g.Withdraw()
		}
	}
	q.start()
	return false // NSTerminateCancel — Electron's event.preventDefault()
}

// RequestQuit is the menu Quit item's click handler. App.Quit() routes through
// applicationShouldTerminate: too, so this is the same path, not a second one.
func (q *Quitter) RequestQuit() {
	if app := q.app.Load(); app != nil {
		app.Quit()
	}
}

// Flushed is one window's ack, bound as Lifecycle.Flushed (IPC.appFlushed) — fire-and-forget and
// idempotent: an unknown key (never live at flushThenQuit's start, or already removed — a
// repeated ack, or a late one past the timeout) is a no-op, not a panic on a closed channel. Also
// the release valve for a window that closes mid-handshake without ever acking through this
// channel at all: main.go's own WindowClosing listener calls this with that window's key too, so
// it is removed from the pending set rather than being waited out for the full timeout.
func (q *Quitter) Flushed(windowKey string) {
	q.mu.Lock()
	if _, owing := q.pending[windowKey]; !owing {
		q.mu.Unlock()
		return
	}
	delete(q.pending, windowKey)
	empty := len(q.pending) == 0
	q.mu.Unlock()
	if empty {
		q.release()
	}
}

// release closes q.flushed exactly once, however many times the pending set independently empties
// out from concurrent Flushed calls.
func (q *Quitter) release() { q.closeOnce.Do(func() { close(q.flushed) }) }

// Shutdown is application.Options.OnShutdown — the path a signal or a Run() error takes, where
// ShouldQuit never fires. Both halves are sync.OnceFunc, so the ordinary path's earlier calls make
// this a no-op rather than a double teardown (P56 D3).
func (q *Quitter) Shutdown() {
	q.beforeFlush()
	q.teardown()
}

func (q *Quitter) flushThenQuit() {
	q.beforeFlush()

	q.mu.Lock()
	q.pending = make(map[string]struct{})
	for _, key := range q.liveWindowKeys() {
		q.pending[key] = struct{}{}
	}
	noWindows := len(q.pending) == 0
	q.mu.Unlock()
	if noWindows {
		// Nothing to wait for — Electron's own Promise.all([]) resolves immediately rather than
		// waiting out the timeout for a handshake with no participants.
		q.release()
	}

	// Broadcast (not Signal, P8 C9): every window must flush before quitting, not only the
	// focused one — Signal's focused-only delivery is for menu commands, not this handshake.
	q.events.Broadcast(appevent.ChannelFlushBeforeClose)
	select {
	case <-q.flushed:
	case <-time.After(q.timeout):
		slog.Warn("quit flush timed out", "scope", "lifecycle", "timeoutMs", q.timeout.Milliseconds())
	}
	q.teardown()
	q.done.Store(true)
	if app := q.app.Load(); app != nil {
		app.Quit() // second pass: ShouldQuit now returns true
	} else {
		slog.Warn("quit before app attached", "scope", "lifecycle")
	}
}
