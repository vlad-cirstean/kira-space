// The earned unit test: concurrency (a reader goroutine, an ordered signal-then-wait-then
// -kill Close racing an exiting process) plus cleanup with interacting rules (idempotent Close,
// write-after-close, exactly-once exit). Every case drives a real PTY — on a host with no
// /dev/ptmx these fail loudly rather than skip (a skipped test that silently never runs is worse
// than a red one).
package terminal

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// collector is a small, race-safe sink for a session's onData/onExit callbacks, shared by every
// test below.
type collector struct {
	mu       sync.Mutex
	data     bytes.Buffer
	exits    int
	exitCode int
	exitErr  error
}

func (c *collector) onData(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data.Write(b)
}

func (c *collector) onExit(code int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.exits++
	c.exitCode = code
	c.exitErr = err
}

func (c *collector) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data.String()
}

func (c *collector) exitCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exits
}

// waitFor polls cond every 10ms until it is true or timeout elapses, failing the test otherwise —
// used throughout instead of a fixed sleep, since a shell's own output timing is not deterministic.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("condition not met within %s", timeout)
	}
}

func openTestSession(t *testing.T, id, cwd string, cols, rows uint16) (*Registry, *Session, *collector) {
	t.Helper()
	reg := NewRegistry()
	col := &collector{}
	sess, err := reg.Open(OpenParams{
		ID: id, WindowKey: "w1", Cwd: cwd, Cols: cols, Rows: rows,
		OnData: col.onData, OnExit: col.onExit,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(sess.Close)
	return reg, sess, col
}

// openShellSession opens a session whose shell is the named binary on PATH; skips only when that
// binary is absent from the machine.
func openShellSession(t *testing.T, shell, id string) (*Session, *collector) {
	t.Helper()
	path, err := exec.LookPath(shell)
	if err != nil {
		t.Skipf("%s not installed", shell)
	}
	t.Setenv("SHELL", path)
	_, sess, col := openTestSession(t, id, t.TempDir(), 80, 24)
	return sess, col
}

// startJob types line, then echoes $!, waits for the job to be a live `sleep` and returns its pid.
// Close right after the fork can land SIGHUP in the child's pre-exec window, where the shell's own
// handler swallows it; hence the comm wait.
func startJob(t *testing.T, sess *Session, col *collector, line string) int {
	t.Helper()
	if err := sess.Write([]byte(line + "\necho started-$!\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var pid int
	waitFor(t, 5*time.Second, func() bool {
		text := col.text()
		// LastIndex, not Index: the pty echoes the typed "echo started-$!" first.
		idx := strings.LastIndex(text, "started-")
		if idx < 0 {
			return false
		}
		rest := text[idx+len("started-"):]
		end := strings.IndexAny(rest, "\r\n")
		if end < 0 {
			return false
		}
		n, err := strconv.Atoi(strings.TrimSpace(rest[:end]))
		if err != nil || n == 0 {
			return false
		}
		pid = n
		return true
	})
	waitFor(t, 5*time.Second, func() bool {
		comm, _, _ := procInfo(pid)
		return comm == "sleep"
	})
	return pid
}

func TestSessionSpawnsAtCwd(t *testing.T) {
	dir := t.TempDir()
	_, sess, col := openTestSession(t, "spawn-at-cwd", dir, 80, 24)

	if err := sess.Write([]byte("pwd\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return strings.Contains(col.text(), dir) })
}

// TestSessionRunsInitialCommand guards P85 §2's one non-obvious property: the command's own exit
// status reaches onExit rather than the wrapper shell's — the signal P86 will count sessions with.
func TestSessionRunsInitialCommand(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	col := &collector{}
	sess, err := reg.Open(OpenParams{
		ID: "initial-command", WindowKey: "w1", Cwd: dir, Cols: 80, Rows: 24,
		Command: "printf ready; exit 7",
		OnData:  col.onData, OnExit: col.onExit,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(sess.Close)

	waitFor(t, 5*time.Second, func() bool { return col.exitCount() > 0 })
	if !strings.Contains(col.text(), "ready") {
		t.Fatalf("output = %q, want it to contain %q", col.text(), "ready")
	}
	col.mu.Lock()
	code := col.exitCode
	col.mu.Unlock()
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
}

func TestSessionResizeAppliesWinsize(t *testing.T) {
	dir := t.TempDir()
	_, sess, col := openTestSession(t, "resize-winsize", dir, 80, 24)

	if err := sess.Resize(100, 30); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if err := sess.Write([]byte("stty size\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// stty size prints "rows cols" — the real ioctl's answer, not the value Resize stored.
	waitFor(t, 5*time.Second, func() bool { return strings.Contains(col.text(), "30 100") })
}

// TestSessionCloseKillsProcessGroup pins forwarding shells: bash and zsh hang up their jobs on
// SIGHUP. dash never forwards, so it is covered by the survivors test instead.
func TestSessionCloseKillsProcessGroup(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			sess, col := openShellSession(t, shell, "close-kills-group-"+shell)
			childPID := startJob(t, sess, col, "sleep 300 &")
			shellPID := sess.cmd.Process.Pid

			// Different groups prove job control is on; a shell change that drops it would silently
			// make this a single-group test.
			shellPG, errS := syscall.Getpgid(shellPID)
			childPG, errC := syscall.Getpgid(childPID)
			if errS != nil || errC != nil || shellPG == childPG {
				t.Fatalf("child must run in its own process group: shell pgid=%d (%v), child pgid=%d (%v)",
					shellPG, errS, childPG, errC)
			}
			shellBefore, childBefore := procStatus(shellPID), procStatus(childPID)

			start := time.Now()
			sess.Close()
			closeTook := time.Since(start)

			select {
			case <-sess.done:
			default:
				t.Fatal("Close returned before the session exited; the SIGKILL/ptmx fallback must not be needed")
			}
			// kill(pid, 0) succeeds on a zombie; reaping an orphan is its reaper's job, not Close's.
			deadline := time.Now().Add(closeGracePeriod + 2*time.Second)
			for processAlive(shellPID) || processAlive(childPID) {
				if time.Now().After(deadline) {
					text := col.text()
					if len(text) > 300 {
						text = text[len(text)-300:]
					}
					t.Fatalf("processes alive after Close (took %s):\n shell before:\n%s\n child before:\n%s\n child after:\n%s\n output tail: %q",
						closeTook, shellBefore, childBefore, procStatus(childPID), text)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

// processAlive reports whether pid is running; gone or zombie (dead, not yet reaped) is not alive.
func processAlive(pid int) bool {
	_, state, _ := procInfo(pid)
	return state != "" && state != "Z" && state != "X"
}

func TestSessionCloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	_, sess, col := openTestSession(t, "close-idempotent", dir, 80, 24)

	sess.Close()
	sess.Close() // must not panic, must not double-report the exit

	if got := col.exitCount(); got != 1 {
		t.Fatalf("exitCount = %d, want 1", got)
	}
}

func TestSessionWriteAfterCloseIsNoop(t *testing.T) {
	dir := t.TempDir()
	_, sess, col := openTestSession(t, "write-after-close", dir, 80, 24)

	sess.Close()
	before := col.text()

	if err := sess.Write([]byte("echo should-not-appear\n")); err != nil {
		t.Fatalf("Write after Close returned an error, want nil: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := col.text(); got != before {
		t.Fatalf("Write after Close produced output: %q", got)
	}
}

func TestSessionExitEmitsOnce(t *testing.T) {
	dir := t.TempDir()
	_, sess, col := openTestSession(t, "exit-emits-once", dir, 80, 24)

	if err := sess.Write([]byte("exit\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return col.exitCount() == 1 })

	// onData never runs after onExit (§4 rule 4): give the reader goroutine a moment to (wrongly)
	// call onData again if it were going to, then confirm the exit code and that nothing further
	// arrived.
	time.Sleep(100 * time.Millisecond)
	col.mu.Lock()
	exits, code := col.exits, col.exitCode
	col.mu.Unlock()
	if exits != 1 {
		t.Fatalf("exitCount = %d, want 1", exits)
	}
	if code != 0 {
		t.Fatalf("exitCode = %d, want 0", code)
	}
}

func TestRegistryCloseWindowClosesOnlyThatWindow(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	colA := &collector{}
	colB := &collector{}

	sessA, err := reg.Open(OpenParams{ID: "a", WindowKey: "window-a", Cwd: dir, Cols: 80, Rows: 24, OnData: colA.onData, OnExit: colA.onExit})
	if err != nil {
		t.Fatalf("Open a: %v", err)
	}
	t.Cleanup(sessA.Close)
	sessB, err := reg.Open(OpenParams{ID: "b", WindowKey: "window-b", Cwd: dir, Cols: 80, Rows: 24, OnData: colB.onData, OnExit: colB.onExit})
	if err != nil {
		t.Fatalf("Open b: %v", err)
	}
	t.Cleanup(sessB.Close)

	reg.CloseWindow("window-a")

	waitFor(t, closeGracePeriod+2*time.Second, func() bool { return colA.exitCount() == 1 })
	if got := colB.exitCount(); got != 0 {
		t.Fatalf("window-b session exited (%d times) after CloseWindow(window-a)", got)
	}

	// b is still alive — writing to it must still work.
	if err := sessB.Write([]byte("echo still-alive\n")); err != nil {
		t.Fatalf("Write to b: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return strings.Contains(colB.text(), "still-alive") })
}

func TestRegistryRejectsDuplicateID(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	col := &collector{}
	sess, err := reg.Open(OpenParams{ID: "dup", WindowKey: "w1", Cwd: dir, Cols: 80, Rows: 24, OnData: col.onData, OnExit: col.onExit})
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	t.Cleanup(sess.Close)

	var spawned atomic.Bool
	col2 := &collector{}
	_, err = reg.Open(OpenParams{ID: "dup", WindowKey: "w1", Cwd: dir, Cols: 80, Rows: 24, OnData: func(b []byte) {
		spawned.Store(true)
		col2.onData(b)
	}, OnExit: col2.onExit})

	if err == nil {
		t.Fatal("second Open with a live id returned no error")
	}
	if err != ErrDuplicateSession {
		t.Fatalf("second Open error = %v, want ErrDuplicateSession", err)
	}
	if spawned.Load() {
		t.Fatal("second Open spawned a process for a duplicate id")
	}
}

func TestRegistryOpenRacingCloseWindowRegistersNothing(t *testing.T) {
	reg := NewRegistry()
	started, release := make(chan struct{}), make(chan struct{})
	reg.spawn = func(p OpenParams) (*Session, error) {
		close(started)
		<-release
		return newSession(p)
	}
	errc := make(chan error, 1)
	col := &collector{}
	go func() {
		_, err := reg.Open(OpenParams{ID: "late", WindowKey: "w1", Cwd: t.TempDir(), Cols: 80, Rows: 24, OnData: col.onData, OnExit: col.onExit})
		errc <- err
	}()
	<-started
	reg.CloseWindow("w1")
	close(release)
	if err := <-errc; err != ErrRegistryClosed {
		t.Fatalf("Open error = %v, want ErrRegistryClosed", err)
	}
	if reg.get("late") != nil || len(reg.AgentSessions()) != 0 {
		t.Fatal("session registered under a closed window")
	}

	reg.CloseAll()
	if _, err := reg.Open(OpenParams{ID: "after", WindowKey: "w2", Cwd: t.TempDir(), Cols: 80, Rows: 24}); err != ErrRegistryClosed {
		t.Fatalf("Open after CloseAll = %v, want ErrRegistryClosed", err)
	}
}

func TestSessionDraftFollowsKeystrokes(t *testing.T) {
	reg, sess, _ := openTestSession(t, "draft", t.TempDir(), 80, 24)
	check := func(label string, want bool, write func() error) {
		t.Helper()
		if err := write(); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if got := reg.Draft("draft"); got != want {
			t.Fatalf("%s: draft = %v, want %v", label, got, want)
		}
	}
	check("inject", false, func() error { return reg.Inject("draft", []byte("pasted\r")) })
	check("reports and arrows", false, func() error {
		return sess.Write([]byte("\x1b[I\x1b[<0;10;5M\x1b[<0;10;5m\x1b[O\x1b[A"))
	})
	check("a key press", true, func() error { return reg.Write("draft", []byte("a")) })
	check("inject keeps the draft", true, func() error { return reg.Inject("draft", []byte("x\r")) })
	check("enter submits", false, func() error { return reg.Write("draft", []byte("\r")) })
	check("paste with a newline", true, func() error { return reg.Write("draft", []byte("\x1b[200~a\rb\x1b[201~")) })
	check("ctrl-u clears", false, func() error { return reg.Write("draft", []byte{0x15}) })
	check("paste split across writes", true, func() error { return reg.Write("draft", []byte("\x1b[200~a\r")) })
	check("end of paste keeps the draft", true, func() error { return reg.Write("draft", []byte("b\x1b[201~")) })
	check("typed then enter in one write", false, func() error { return reg.Write("draft", []byte("go\r")) })
}

func TestRegistryOnChangeCoalesces(t *testing.T) {
	reg := NewRegistry()
	block, entered := make(chan struct{}), make(chan struct{}, 100)
	var inflight, maxInflight, calls atomic.Int32
	reg.OnChange = func() {
		n := inflight.Add(1)
		for {
			m := maxInflight.Load()
			if n <= m || maxInflight.CompareAndSwap(m, n) {
				break
			}
		}
		calls.Add(1)
		entered <- struct{}{}
		<-block
		inflight.Add(-1)
	}
	dir := t.TempDir()
	opened := make(chan struct{})
	go func() {
		defer close(opened)
		for i := range 10 {
			col := &collector{}
			sess, err := reg.Open(OpenParams{ID: "s" + strconv.Itoa(i), WindowKey: "w", Cwd: dir, Cols: 80, Rows: 24, Agent: true, OnData: col.onData, OnExit: col.onExit})
			if err != nil {
				t.Errorf("Open %d: %v", i, err)
				return
			}
			t.Cleanup(sess.Close)
		}
	}()
	select {
	case <-opened:
	case <-time.After(20 * time.Second):
		t.Fatal("Open blocked while OnChange was blocked")
	}
	close(block)
	waitFor(t, 5*time.Second, func() bool { return calls.Load() >= 2 })
	reg.changeWG.Wait()
	if got := maxInflight.Load(); got != 1 {
		t.Fatalf("OnChange ran %d at once, want 1", got)
	}
}
