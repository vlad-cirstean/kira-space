package terminal

import (
	"encoding/base64"
	"os"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// The three wire values a LaunchKind arg accepts (packages/shared/domain/tabs.ts's
// terminalLaunchKindSchema, field for field). Only LaunchKindAgent counts toward
// OpenParams.Agent; a custom script that happens to run `claude` counts as a script, deliberately
// ("so the implementation does not 'fix' it into a heuristic").
const (
	LaunchKindShell  = "shell"
	LaunchKindAgent  = "agent"
	LaunchKindScript = "script"
)

// ValidLaunchKind reports whether v is "" (treated as LaunchKindShell, so an older caller keeps
// working) or one of the three named kinds above.
func ValidLaunchKind(v string) bool {
	switch v {
	case "", LaunchKindShell, LaunchKindAgent, LaunchKindScript:
		return true
	default:
		return false
	}
}

// MaxDim is the [1, 1000] bound on cols/rows — generous for any real display, tight enough that a
// malformed arg cannot ask the pty for an absurd winsize.
const MaxDim = 1000

// MaxCommandBytes bounds an Open command well under macOS's ARG_MAX (1 MiB for argv plus
// environment together, P85 §4) — a clear E_BAD_REQUEST instead of an opaque E2BIG out of exec.
const MaxCommandBytes = 64 * 1024

// ValidDim reports whether n is within [1, MaxDim].
func ValidDim(n int) bool { return n >= 1 && n <= MaxDim }

// DefaultCwd is the user's home directory, for an unscoped terminal launch (a repo-scoped
// terminal keeps using internal/gitsession's own worktree-cwd resolution) — "" when it can't be
// resolved (P91 §7.1: a missing home directory must not fail boot).
func DefaultCwd() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// Service is BoundService's own "generic half": Write/Resize/Close and the
// coalescing output pump, over a Registry and an Emitter. A separate type from BoundService, not
// methods on it directly, since Service also exports OpenWithCoalescedOutput and positional
// Write/Resize/Close — embedding it into BoundService would bind those too.
type Service struct {
	Emit     appevent.Emitter
	Registry *Registry
}

// Write decodes dataB64 and forwards it to the pty. A no-op for an id with no live session
// (Registry.Write's own rule) — a renderer can have a keystroke in flight when a shell exits.
func (s *Service) Write(terminalID, dataB64 string) error {
	if terminalID == "" {
		return ipcerr.Required("terminalId")
	}
	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return ipcerr.Contract("data must be base64")
	}
	if err := s.Registry.Write(terminalID, data); err != nil {
		return ipcerr.Internal(err.Error())
	}
	return nil
}

// Resize applies cols/rows to the real winsize (pty.Setsize) — a no-op for an id with no live
// session.
func (s *Service) Resize(terminalID string, cols, rows int) error {
	if terminalID == "" {
		return ipcerr.Required("terminalId")
	}
	if !ValidDim(cols) || !ValidDim(rows) {
		return ipcerr.Contract("cols/rows must be within [1, 1000]")
	}
	if err := s.Registry.Resize(terminalID, uint16(cols), uint16(rows)); err != nil {
		return ipcerr.Internal(err.Error())
	}
	return nil
}

// Close kills terminalID's own session — idempotent, per Session.Close.
func (s *Service) Close(terminalID string) error {
	if terminalID == "" {
		return ipcerr.Required("terminalId")
	}
	s.Registry.Close(terminalID)
	return nil
}

// OpenWithCoalescedOutput wires p's OnData/OnExit to a fresh coalescing output pump addressed at
// windowKey/terminalID, then calls Registry.Open — the "output pump" half of Open that is
// byte-identical between apps. Each app's own Open builds everything else about p (Command, Agent)
// and calls this instead of Registry.Open directly. tap, when non-nil, sees every output chunk on
// the reader goroutine before the coalescer does; onExit runs after the exit event is sent.
func (s *Service) OpenWithCoalescedOutput(p OpenParams, windowKey, terminalID string, tap func([]byte), onExit func()) (*Session, error) {
	coalescer := newOutputCoalescer(s.Emit, windowKey, terminalID)
	coalescer.onDone = func() { s.Registry.untrackOutput(terminalID, coalescer) }
	p.OnData = coalescer.push
	if tap != nil {
		p.OnData = func(b []byte) {
			tap(b)
			coalescer.push(b)
		}
	}
	p.OnExit = func(code int, exitErr error) {
		msg := ""
		if exitErr != nil {
			msg = exitErr.Error()
		}
		coalescer.finish(code, msg)
		if onExit != nil {
			onExit()
		}
	}
	sess, err := s.Registry.Open(p)
	if err == nil {
		s.Registry.trackOutput(terminalID, sess, coalescer)
	}
	return sess, err
}

// ---- the coalescing terminal-output push channel ----

const (
	coalesceInterval = 16 * time.Millisecond
	coalesceMaxBytes = 64 * 1024
)

// Event is ChannelTerminal's own payload (packages/shared/protocol/events.ts's terminalEventSchema,
// field for field) — one coalesced chunk of a session's output, or its exit.
type Event struct {
	TerminalID string `json:"terminalId"`
	Data       string `json:"data,omitempty"` // base64, absent on the exit event
	// End is the cumulative byte count of the session's output including this chunk. A window
	// adopting a moved terminal uses it to order live chunks against the held bytes (P722).
	End      int64  `json:"end"`
	Exited   bool   `json:"exited"`
	ExitCode *int   `json:"exitCode,omitempty"`
	Error    string `json:"error,omitempty"`
}

// outputCoalescerFinal is the extra payload only the exit flush carries.
type outputCoalescerFinal struct {
	exitCode int
	errMsg   string
}

// maxHeldBytes caps the output buffered while a terminal moves between windows.
const maxHeldBytes = 1 << 20

// outputCoalescer wraps appevent.Coalescer[[]byte, outputCoalescerFinal]: flush on 16 KiB
// accumulated or a 16ms timer, whichever first; the exit event always flushes, even with nothing
// pending, so the renderer's "running" state can never strand.
//
// mu orders every flush against hold, adopt and release: a flush either emits and advances end, or
// buffers, never both. Lock order across the package is Registry.mu then mu; flush never takes
// Registry.mu while holding mu.
type outputCoalescer struct {
	emit       appevent.Emitter
	terminalID string
	gen        *appevent.Coalescer[[]byte, outputCoalescerFinal]
	// onDone runs, outside mu, after an exit event was emitted rather than held.
	onDone func()

	mu        sync.Mutex
	windowKey string // EmitTo target
	end       int64  // cumulative bytes emitted or handed over by adopt
	held      bool
	pending   string // window the hold ends in, set by Reown; "" until then
	buf       [][]byte
	bufBytes  int
	total     int64 // bytes produced while held, dropped chunks included
	truncated bool
	final     *outputCoalescerFinal
}

func newOutputCoalescer(emit appevent.Emitter, windowKey, terminalID string) *outputCoalescer {
	c := &outputCoalescer{emit: emit, windowKey: windowKey, terminalID: terminalID}
	c.gen = appevent.NewCoalescer(coalesceInterval, coalesceMaxBytes, func(b []byte) int { return len(b) }, c.flush)
	return c
}

// push is the session's own reader goroutine calling in — one call at a time per session.
func (c *outputCoalescer) push(b []byte) { c.gen.Push(b) }

// finish is the terminal flush — always sent, even with nothing pending.
func (c *outputCoalescer) finish(exitCode int, errMsg string) {
	c.gen.Finish(outputCoalescerFinal{exitCode: exitCode, errMsg: errMsg})
}

func (c *outputCoalescer) flush(batch [][]byte, done bool, final outputCoalescerFinal) {
	n := 0
	for _, b := range batch {
		n += len(b)
	}
	raw := make([]byte, 0, n)
	for _, b := range batch {
		raw = append(raw, b...)
	}

	c.mu.Lock()
	if c.held {
		c.holdLocked(raw)
		if done {
			c.final = &final
		}
		c.mu.Unlock()
		return
	}
	c.emitLocked(raw, done, &final)
	c.mu.Unlock()
	if done && c.onDone != nil {
		c.onDone()
	}
}

// emitLocked sends raw (and, when done, the exit) to windowKey and advances end. Caller holds mu.
func (c *outputCoalescer) emitLocked(raw []byte, done bool, final *outputCoalescerFinal) {
	c.end += int64(len(raw))
	ev := Event{TerminalID: c.terminalID, End: c.end, Exited: done}
	if len(raw) > 0 {
		ev.Data = base64.StdEncoding.EncodeToString(raw)
	}
	if done && final != nil {
		ec := final.exitCode
		ev.ExitCode = &ec
		ev.Error = final.errMsg
	}
	c.emit.EmitTo(c.windowKey, appevent.ChannelTerminal, ev)
}

// holdLocked buffers raw, dropping the oldest chunks past maxHeldBytes. Caller holds mu.
func (c *outputCoalescer) holdLocked(raw []byte) {
	if len(raw) == 0 {
		return
	}
	c.total += int64(len(raw))
	c.buf = append(c.buf, raw)
	c.bufBytes += len(raw)
	for c.bufBytes > maxHeldBytes && len(c.buf) > 1 {
		c.bufBytes -= len(c.buf[0])
		c.buf[0] = nil
		c.buf = c.buf[1:]
		c.truncated = true
	}
	if c.bufBytes > maxHeldBytes {
		c.buf[0] = c.buf[0][c.bufBytes-maxHeldBytes:]
		c.bufBytes = maxHeldBytes
		c.truncated = true
	}
}

func (c *outputCoalescer) clearHoldLocked() {
	c.held, c.pending = false, ""
	c.buf, c.bufBytes, c.total, c.truncated, c.final = nil, 0, 0, false, nil
}

// hold stops emission: later output is buffered. end is what the current owner already received.
func (c *outputCoalescer) hold() (end int64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.held {
		return 0, ErrAlreadyMoving
	}
	c.held = true
	return c.end, nil
}

// reown retargets the coalescer at to. Held: the hold ends there at adopt. Not held: events go
// there at once.
func (c *outputCoalescer) reown(to string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.held {
		c.pending = to
		return
	}
	c.windowKey = to
}

// reownEnded retargets a held pump whose session already ended and whose owner is from.
func (c *outputCoalescer) reownEnded(from, to string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	owner := c.pending
	if owner == "" {
		owner = c.windowKey
	}
	if !c.held || c.final == nil || owner != from {
		return false
	}
	c.pending = to
	return true
}

// adopt ends the hold in window key and hands over the buffer and any recorded exit.
func (c *outputCoalescer) adopt(key string) (AdoptResult, error) {
	c.mu.Lock()
	if !c.held {
		c.mu.Unlock()
		return AdoptResult{}, ErrNotHeld
	}
	if c.pending != key {
		c.mu.Unlock()
		return AdoptResult{}, ErrNotOwner
	}
	data := joinChunks(c.buf, c.bufBytes)
	c.end += c.total
	res := AdoptResult{End: c.end, Truncated: c.truncated}
	if len(data) > 0 {
		res.Data = base64.StdEncoding.EncodeToString(data)
	}
	exited := c.final != nil
	if exited {
		res.Exited, res.ExitCode, res.Error = true, &c.final.exitCode, c.final.errMsg
	}
	c.windowKey = key
	c.clearHoldLocked()
	c.mu.Unlock()
	if exited && c.onDone != nil {
		c.onDone()
	}
	return res, nil
}

// release ends the hold in the current window: one event with the buffer, then the exit if any.
func (c *outputCoalescer) release() {
	c.mu.Lock()
	if !c.held {
		c.mu.Unlock()
		return
	}
	data := joinChunks(c.buf, c.bufBytes)
	c.end += c.total - int64(len(data))
	exited := c.final != nil
	var final *outputCoalescerFinal
	if exited {
		f := *c.final
		final = &f
	}
	c.clearHoldLocked()
	if len(data) > 0 || exited {
		c.emitLocked(data, exited, final)
	}
	c.mu.Unlock()
	if exited && c.onDone != nil {
		c.onDone()
	}
}

// forget drops a hold whose target window closed. It reports whether an exit was recorded, so the
// caller can untrack a session that already ended.
func (c *outputCoalescer) forget(key string) (dropped, exited bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.held {
		return false, false
	}
	target := c.pending
	if target == "" {
		target = c.windowKey
	}
	if target != key {
		return false, false
	}
	exited = c.final != nil
	c.clearHoldLocked()
	return true, exited
}

func joinChunks(chunks [][]byte, n int) []byte {
	out := make([]byte, 0, n)
	for _, b := range chunks {
		out = append(out, b...)
	}
	return out
}
