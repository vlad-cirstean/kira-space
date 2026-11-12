package terminal

import (
	"encoding/base64"
	"os"
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
	return s.Registry.Open(p)
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
	Exited     bool   `json:"exited"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	Error      string `json:"error,omitempty"`
}

// outputCoalescerFinal is the extra payload only the exit flush carries.
type outputCoalescerFinal struct {
	exitCode int
	errMsg   string
}

// outputCoalescer wraps appevent.Coalescer[[]byte, outputCoalescerFinal]: flush on 16 KiB
// accumulated or a 16ms timer, whichever first; the exit event always flushes, even with nothing
// pending, so the renderer's "running" state can never strand.
type outputCoalescer struct {
	emit       appevent.Emitter
	windowKey  string
	terminalID string
	gen        *appevent.Coalescer[[]byte, outputCoalescerFinal]
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
	var data string
	if len(batch) > 0 {
		n := 0
		for _, b := range batch {
			n += len(b)
		}
		buf := make([]byte, 0, n)
		for _, b := range batch {
			buf = append(buf, b...)
		}
		data = base64.StdEncoding.EncodeToString(buf)
	}
	var exitCode *int
	var errMsg string
	if done {
		ec := final.exitCode
		exitCode = &ec
		errMsg = final.errMsg
	}
	c.emit.EmitTo(c.windowKey, appevent.ChannelTerminal, Event{
		TerminalID: c.terminalID, Data: data, Exited: done, ExitCode: exitCode, Error: errMsg,
	})
}
