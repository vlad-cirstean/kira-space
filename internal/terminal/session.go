// Package terminal is the PTY layer: one Session per live shell, held by a Registry keyed
// by a client-supplied id (the tab id) and, for teardown, by windowKey. It is a domain package —
// it must not import internal/bridge (internal/layering_test.go's
// TestDomainPackagesDoNotImportBridge picks this package up automatically from `go list`, not by
// a maintained exemption list); internal/bridge/terminal.go is the one place that turns this
// package's plain errors into ipcerr responses and its callbacks into push-channel events.
package terminal

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// readBufSize is the reader goroutine's own read(2) buffer (§4 rule 1) — large enough that a
// build's kilobyte-bursts of output do not need many round trips, small enough to stay a
// stack-friendly one-shot allocation per read.
const readBufSize = 32 * 1024

// closeGracePeriod is how long Close waits for a SIGHUP'd process group to exit on its own before
// escalating to SIGKILL — this repo's own existing handshake bound
// (internal/shell/closeflush.go's closeFlushTimeout).
const closeGracePeriod = 2 * time.Second

// closeKillWait bounds how long Close waits, after SIGKILL, for readLoop's blocked ptmx.Read to
// return. readLoop closes the master exitDrain after the shell exits
// (both platforms share pollablePtmx), so a surviving job no longer stalls Close; this
// bound remains for a shell that somehow survives SIGKILL regardless. Close's own callers —
// Registry.CloseAll/CloseWindow, in turn ShutdownBound (on the app-quit
// teardown path, before db.Close()) and shell.OpenWindow's own per-window close — must never hang
// on one stuck session, so this is a second, independent bound, not a substitute for the SIGKILL
// escalation above it.
const closeKillWait = 2 * time.Second

// exitDrain is how long readLoop keeps reading output still buffered after the shell exits before
// it closes the master. A job left holding the slave (disown, nohup, dash's background jobs) never
// sends EIO, so without this the session would outlive its shell.
const exitDrain = 200 * time.Millisecond

// ErrDuplicateSession is Open's error when id is already live, or another Open for the same id is
// still spawning — the registry rejects it before touching the PTY, per §4/§17.1's
// TestRegistryRejectsDuplicateID.
var ErrDuplicateSession = errors.New("terminal: session id already in use")

// ErrRegistryClosed is Open's error when CloseAll ran, or CloseWindow ran for the session's own
// window, while the spawn was in flight — the new session is closed instead of registered.
var ErrRegistryClosed = errors.New("terminal: window or registry closed during open")

// Session is one live PTY: the process, the master fd, and the single reader goroutine that owns
// both (§4). Every exported method is safe to call from any goroutine.
type Session struct {
	id    string
	shell string
	// cwd and agent are set once, from OpenParams, in newSession — Registry.AgentSessions' own
	// read of them (P86 §8.2).
	cwd    string
	agent  bool
	cmd    *exec.Cmd
	ptmx   *os.File
	onData func([]byte)
	onExit func(code int, err error)
	onEnd  func(code int, cause CloseCause)
	cause  CloseCause
	// unregister removes this session from its Registry — called exactly once, from the reader
	// goroutine, whether the exit was requested (Close) or spontaneous (the shell exiting on its
	// own, e.g. `exit`/Ctrl-D).
	unregister func()

	mu     sync.Mutex
	closed bool // true once Close has begun, or the reader goroutine has observed the process exit
	// done closes once the reader goroutine has called onExit — Close's own "wait up to 2s for
	// Wait()" step blocks on it.
	done chan struct{}
}

// ID is this session's own id (the tab id it was opened with).
func (s *Session) ID() string { return s.id }

// PID is the process id of the session's shell, the root of everything it runs.
func (s *Session) PID() int { return s.cmd.Process.Pid }

// Shell is the resolved shell path this session started — TerminalService.Open's own {shell}
// result field.
func (s *Session) Shell() string { return s.shell }

// newSession spawns shell as a login, interactive process at p.Cwd, with the given initial
// size, and returns before the reader goroutine has started — Registry.Open starts it once the
// session is registered, so onData/onExit can never fire on a session no lookup can find yet.
// p.Command, when non-empty, runs as one extra `-c <command>` argv pair — the shell
// stays login and interactive either way, so a launched command sees the same PATH a plain
// terminal in this app sees.
func newSession(p OpenParams) (*Session, error) {
	handleIgnoredSighup()
	shellPath := loginShell()
	args := []string{"-l", "-i"}
	if p.Command != "" {
		args = append(args, "-c", p.Command)
	}
	cmd := exec.Command(shellPath, args...)
	cmd.Dir = p.Cwd
	cmd.Env = append(os.Environ(), sessionEnv()...)
	// p.Env is extra environment appended after sessionEnv() — this package stays
	// agnostic and only forwards the strings; internal/bridge/terminal.go decides what they are
	// (KIRA_TERMINAL_ID/KIRA_AGENT_HOOK_SOCKET/KIRA_AGENT_HOOK_TOKEN for a Claude Code launch with
	// hooks enabled, nothing for a plain terminal or a script).
	cmd.Env = append(cmd.Env, p.Env...)
	// Setsid: true makes this shell its own process-group leader — Close signals the whole group
	// (syscall.Kill(-pid, …)), so a process the shell hangs up (an `npm run dev` under bash or zsh)
	// dies with the tab. Jobs the shell does not hang up (dash, disown, nohup) outlive it.
	// pty.StartWithSize already makes the pty this process's controlling terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: p.Rows, Cols: p.Cols})
	if err != nil {
		return nil, err
	}
	ptmx, err = pollablePtmx(ptmx)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}

	return &Session{
		id:     p.ID,
		shell:  shellPath,
		cwd:    p.Cwd,
		agent:  p.Agent,
		cmd:    cmd,
		ptmx:   ptmx,
		onData: p.OnData,
		onExit: p.OnExit,
		onEnd:  p.OnEnd,
		done:   make(chan struct{}),
	}, nil
}

var sighupOnce sync.Once

// handleIgnoredSighup keeps shells hangable when the app itself started with SIGHUP ignored (nohup,
// a detached launcher): an ignored signal survives exec, so every shell would ignore the hang-up
// Close relies on and its jobs would outlive the tab. A handled signal resets to default at exec.
// Skipped when SIGHUP is not ignored, so the app's own SIGHUP behaviour stays as it was.
func handleIgnoredSighup() {
	sighupOnce.Do(func() {
		if signal.Ignored(syscall.SIGHUP) {
			signal.Notify(make(chan os.Signal, 1), syscall.SIGHUP)
		}
	})
}

// readLoop is the one reader goroutine per session: an io.Reader loop over ptmx into
// a readBufSize buffer, calling onData for every chunk read. The read ends on a read error (EOF, or
// EIO — what Linux returns when the slave side closes) or when a closer goroutine closes the master
// exitDrain after the shell exits, whichever is first. It then marks the session closed, closes the
// master fd, calls onExit exactly once, and unregisters — in that order, so onData never runs
// after onExit and a later explicit Close is a no-op.
func (s *Session) readLoop() {
	var (
		code    int
		waitErr error
	)
	exited := make(chan struct{})
	readDone := make(chan struct{})
	go func() {
		code, waitErr = waitExitCode(s.cmd)
		close(exited)
	}()
	go func() {
		select {
		case <-readDone:
		case <-exited:
			t := time.NewTimer(exitDrain)
			defer t.Stop()
			select {
			case <-readDone:
			case <-t.C:
				_ = s.ptmx.Close()
			}
		}
	}()

	buf := make([]byte, readBufSize)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.onData(data)
		}
		if err != nil {
			break
		}
	}

	close(readDone)
	<-exited

	s.mu.Lock()
	s.closed = true
	cause := s.cause
	s.mu.Unlock()

	_ = s.ptmx.Close()
	if s.onEnd != nil {
		s.onEnd(code, cause)
	}
	s.onExit(code, waitErr)
	// Unregister before done: closeFor returns on done, and callers then expect the session gone.
	s.unregister()
	close(s.done)
}

// waitExitCode calls cmd.Wait() exactly once (the session's waiter goroutine — see readLoop) and
// extracts an exit code the way the rest of this codebase's process-running helpers do: 0 on a
// clean exit, exec.ExitError's own code (−1 for a signal-terminated process, e.g. this package's
// own SIGKILL) otherwise, and the raw error for anything else (Wait itself failing to start).
func waitExitCode(cmd *exec.Cmd) (int, error) {
	err := cmd.Wait()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return -1, err
}

// Write sends b to the pty's master fd — a keystroke, a paste, a mouse report. A no-op, not an
// error, once the session is closed (§4 rule 3): the renderer can have a keystroke in flight when
// a shell exits, and turning that into a visible error would be noise.
func (s *Session) Write(b []byte) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil
	}
	_, err := s.ptmx.Write(b)
	return err
}

// Resize applies cols/rows to the real winsize (TIOCSWINSZ) — a no-op once closed, for the same
// reason Write is.
func (s *Session) Resize(cols, rows uint16) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil
	}
	return setWinsize(s.ptmx, &pty.Winsize{Rows: rows, Cols: cols})
}

// Close is idempotent and ordered: mark closed under the mutex → SIGHUP the shell's
// process group (jobs outside it are deliberately not signalled — the shell forwards the hangup or
// not, as in real terminals) → wait up to closeGracePeriod for the reader goroutine to observe
// the exit and call Wait() (the `done` channel) → SIGKILL the process group if it hasn't
// → wait up to closeKillWait more, then close the master fd ourselves and wait one more
// closeKillWait bound before giving up and logging — never blocking forever. Closing the fd first
// (before either signal) would race the reader into a use-after-close; signalling first lets the
// reader observe EOF/EIO and exit on its own in the common case, with SIGKILL and then a forced
// ptmx.Close as the bound on a shell that ignores SIGHUP or sits outside the signalled process
// group (job control).
func (s *Session) Close() { s.closeFor(CauseUser) }

// closeFor is Close recording why; the first closer's cause stands.
func (s *Session) closeFor(cause CloseCause) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.cause = cause
	s.mu.Unlock()

	pid := s.cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGHUP)

	select {
	case <-s.done:
		return
	case <-time.After(closeGracePeriod):
	}

	_ = syscall.Kill(-pid, syscall.SIGKILL)

	select {
	case <-s.done:
		return
	case <-time.After(closeKillWait):
	}

	// Still not done: the shell survived SIGKILL, or readLoop is stuck in a blocked read. The
	// master is non-blocking and netpoller-registered (pollablePtmx, both platforms),
	// so Close interrupts the pending Read; the wait below is the bound for the case it somehow
	// doesn't: a logged, permanent leak of this one goroutine/process beats every caller of Close
	// (app-quit teardown, a single window's own close) hanging forever on it.
	_ = s.ptmx.Close()

	select {
	case <-s.done:
	case <-time.After(closeKillWait):
		slog.Warn("terminal: session did not exit after SIGKILL and ptmx close",
			"scope", "terminal", "id", s.id, "pid", pid)
	}
}

// CloseCause says why a session was closed from outside; CauseNone is a spontaneous exit.
type CloseCause int

const (
	CauseNone CloseCause = iota
	CauseUser
	CauseWindow
	CauseQuit
)

// OpenParams is Registry.Open's own args — terminalId is client-supplied (the tab id), so the
// renderer can subscribe to its output before this call returns and no output can race the
// subscription (bridge/terminal.go's own reasoning, §3.2).
type OpenParams struct {
	ID        string
	WindowKey string
	Cwd       string
	Cols      uint16
	Rows      uint16
	// Command, when non-empty, runs as `$SHELL -l -i -c Command` (P85 §2.1) instead of a plain
	// login shell. Empty means a plain login shell — P83's own behaviour, unchanged.
	Command string
	// Env is extra environment appended after sessionEnv() (P86 §5.4) — the domain package never
	// composes it, only forwards it; internal/bridge/terminal.go decides what these strings are.
	Env []string
	// Agent marks this session as a Claude Code launch (P86 §4/§8.2) — counted by
	// Registry.AgentSessions and Registry.OnChange, never inferred from Command (P85 OQ-3's own
	// "no heuristic" rule, carried forward).
	Agent bool
	// OnData is called from the session's own reader goroutine for every chunk read — never
	// concurrently with itself, and never after OnExit (§4 rule 4).
	OnData func([]byte)
	// OnExit is called exactly once, from the reader goroutine, whether the shell exited on its
	// own or was killed by Close.
	OnExit func(code int, err error)
	// OnEnd, when set, runs once just before OnExit with why the session ended.
	OnEnd func(code int, cause CloseCause)
}

// AgentSession is one live Claude Code launch's own wire-agnostic shape — Registry.AgentSessions'
// return type, turned into the wire projection by internal/bridge/terminal.go (P86 §11: the count's
// authority is Go, since Registry holds every live session across every window).
type AgentSession struct {
	ID  string
	Cwd string
}

// Registry holds one Session per live id, and indexes them by windowKey for CloseWindow.
type Registry struct {
	mu       sync.Mutex
	sessions map[string]*Session
	byWindow map[string]map[string]struct{}
	// closed is set by CloseAll; pending maps a reserved id to its window while its spawn is in
	// flight, and doomed marks those whose window closed meanwhile.
	closed  bool
	pending map[string]string
	doomed  map[string]struct{}
	spawn   func(OpenParams) (*Session, error)

	// OnChange, when set, is called — outside the mutex, so it may safely call back into
	// AgentSessions below — after Open registers a new agent session and after remove deletes one.
	// readLoop calls onExit *before* unregister (remove), so emitting a "what changed"
	// signal from OnExit would report a dying session as still live; OnChange fires from remove,
	// after the map entry is already gone, which is the one place that ordering is correct.
	OnChange func()
}

func NewRegistry() *Registry {
	return &Registry{
		sessions: map[string]*Session{},
		byWindow: map[string]map[string]struct{}{},
		pending:  map[string]string{},
		doomed:   map[string]struct{}{},
		spawn:    newSession,
	}
}

// Open spawns a new session at p.Cwd with the given initial size and registers it under p.ID.
// p.ID must not already be live — checked, and reserved, before the PTY is spawned, so a second
// concurrent Open for the same id never races a spawn in flight and never starts a second process
// (the TestRegistryRejectsDuplicateID).
func (r *Registry) Open(p OpenParams) (*Session, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, ErrRegistryClosed
	}
	if _, exists := r.sessions[p.ID]; exists {
		r.mu.Unlock()
		return nil, ErrDuplicateSession
	}
	r.sessions[p.ID] = nil // reserved: a spawn is in flight for this id
	r.pending[p.ID] = p.WindowKey
	r.mu.Unlock()

	sess, err := r.spawn(p)
	r.mu.Lock()
	doomed := r.closed
	_, win := r.doomed[p.ID]
	if win {
		doomed = true
	}
	delete(r.pending, p.ID)
	delete(r.doomed, p.ID)
	if err != nil || doomed {
		delete(r.sessions, p.ID)
		r.mu.Unlock()
		if err != nil {
			return nil, err
		}
		// Close waits on readLoop's done channel, so the loop must run to reap the shell.
		sess.unregister = func() {}
		go sess.readLoop()
		if win {
			sess.closeFor(CauseWindow)
		} else {
			sess.closeFor(CauseQuit)
		}
		return nil, ErrRegistryClosed
	}
	sess.unregister = func() { r.remove(p.WindowKey, p.ID) }

	r.sessions[p.ID] = sess
	if r.byWindow[p.WindowKey] == nil {
		r.byWindow[p.WindowKey] = map[string]struct{}{}
	}
	r.byWindow[p.WindowKey][p.ID] = struct{}{}
	r.mu.Unlock()

	if p.Agent && r.OnChange != nil {
		r.OnChange()
	}

	go sess.readLoop()
	return sess, nil
}

// get returns id's own live session, or nil when there is none — including while a same-id Open
// is still spawning (the reservation in Open, above).
func (r *Registry) get(id string) *Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[id]
}

// Cwd is id's live working directory; false only when no live session has id.
func (r *Registry) Cwd(id string) (string, bool) {
	sess := r.get(id)
	if sess == nil {
		return "", false
	}
	return sess.liveCwd(), true
}

// Write is a no-op, not an error, for an id with no live session — mirroring Session.Write's own
// close-is-silent rule for the "already gone by the time this arrived" case.
func (r *Registry) Write(id string, b []byte) error {
	if sess := r.get(id); sess != nil {
		return sess.Write(b)
	}
	return nil
}

// Resize is a no-op for an id with no live session, for the same reason Write is.
func (r *Registry) Resize(id string, cols, rows uint16) error {
	if sess := r.get(id); sess != nil {
		return sess.Resize(cols, rows)
	}
	return nil
}

// Close kills id's own session — idempotent, and a no-op when id names no live session (already
// closed, or never opened).
func (r *Registry) Close(id string) { r.closeFor(id, CauseUser) }

func (r *Registry) closeFor(id string, cause CloseCause) {
	if sess := r.get(id); sess != nil {
		sess.closeFor(cause)
	}
}

// remove is Session's own unregister callback — called from the reader goroutine once, whether
// the exit was requested or spontaneous. **The ordering here is load-bearing**: readLoop (above)
// calls onExit *before* this, so OnChange must fire from here, after the map entry is gone, never
// from onExit — otherwise a dying session would still count as live.
func (r *Registry) remove(windowKey, id string) {
	r.mu.Lock()
	sess := r.sessions[id]
	delete(r.sessions, id)
	if m := r.byWindow[windowKey]; m != nil {
		delete(m, id)
		if len(m) == 0 {
			delete(r.byWindow, windowKey)
		}
	}
	r.mu.Unlock()

	if sess != nil && sess.agent && r.OnChange != nil {
		r.OnChange()
	}
}

// AgentSessions returns every live session opened with Agent: true, across every window — sorted
// by id for a deterministic wire order. This is the app-wide count's sole authority (P86 §11):
// state/terminals.ts's own map is per-window, so only this registry ever sees the whole picture.
func (r *Registry) AgentSessions() []AgentSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AgentSession, 0, len(r.sessions))
	for _, sess := range r.sessions {
		if sess != nil && sess.agent {
			out = append(out, AgentSession{ID: sess.id, Cwd: sess.cwd})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// WindowOf returns id's own live session's window key, reading the byWindow index CloseWindow
// already keys off of — AdeTaskService.FocusSession's own lookup (P129 Part 7 §0.9): "is this session's
// PTY owned by some window, and which one". false for an id with no live session.
func (r *Registry) WindowOf(id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for windowKey, ids := range r.byWindow {
		if _, ok := ids[id]; ok {
			return windowKey, true
		}
	}
	return "", false
}

// CloseWindow closes every session opened under windowKey — a window's own close handler
// (main.go's WindowClosing, §4's teardown table), so a terminal never outlives the window that
// opened it even when the renderer never gets to ack.
func (r *Registry) CloseWindow(key string) {
	r.mu.Lock()
	for id, w := range r.pending {
		if w == key {
			r.doomed[id] = struct{}{}
		}
	}
	ids := make([]string, 0, len(r.byWindow[key]))
	for id := range r.byWindow[key] {
		ids = append(ids, id)
	}
	r.mu.Unlock()

	for _, id := range ids {
		r.closeFor(id, CauseWindow)
	}
}

// CloseAll closes every live session — app teardown. Closes them
// concurrently: each Close can cost up to closeGracePeriod+2*closeKillWait now that Close
// itself is bounded, and teardown runs this before db.Close(), so a sequential loop would pay that
// cost once per open terminal instead of ~once total.
func (r *Registry) CloseAll() {
	r.mu.Lock()
	r.closed = true
	ids := make([]string, 0, len(r.sessions))
	for id, sess := range r.sessions {
		if sess != nil {
			ids = append(ids, id)
		}
	}
	r.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(len(ids))
	for _, id := range ids {
		go func(id string) {
			defer wg.Done()
			r.closeFor(id, CauseQuit)
		}(id)
	}
	wg.Wait()
}
