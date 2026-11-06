package preconnect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/testx"
)

// processAlive/waitUntil are testx.ProcessAlive/testx.WaitUntil (P107 I2-28).
var (
	processAlive = testx.ProcessAlive
	waitUntil    = testx.WaitUntil
)

func entryPID(t *testing.T, s *Supervisor, connectionID string) int {
	t.Helper()
	s.mu.Lock()
	e, ok := s.entries[connectionID]
	s.mu.Unlock()
	if !ok {
		t.Fatalf("no tracked entry for %q", connectionID)
	}
	return e.pid
}

func entryDead(s *Supervisor, connectionID string) *Exit {
	s.mu.Lock()
	e, ok := s.entries[connectionID]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dead
}

// exitCollector is a small OnExit sink safe for concurrent use across the tests below.
type exitCollector struct {
	mu   sync.Mutex
	exit []Exit
}

func (c *exitCollector) handle(e Exit) {
	c.mu.Lock()
	c.exit = append(c.exit, e)
	c.mu.Unlock()
}

func (c *exitCollector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.exit)
}

func (c *exitCollector) last() Exit {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exit[len(c.exit)-1]
}

// TestOneShotExitZero pins the settle-window race in Start: a script that exits 0 before the
// window elapses must win the race and be classified one-shot, never tracked as a sidecar.
func TestOneShotExitZero(t *testing.T) {
	s := New()
	start := time.Now()
	got, err := s.Start(context.Background(), "c1", "true", nil, 0)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Kind != KindOneShot {
		t.Errorf("Kind = %q, want %q", got.Kind, KindOneShot)
	}
	if elapsed >= settleWindow {
		t.Errorf("Start took %s, want well inside the settle window (%s)", elapsed, settleWindow)
	}
}

// TestFailureBeforeSettleCarriesStderrTail guards the ordering the spawn goroutine depends on:
// stderr must be drained to EOF before cmd.Wait() is called, or the rejection message loses the
// tail it is composed from. It also covers signal-vs-exit-code classification.
func TestFailureBeforeSettleCarriesStderrTail(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"exit code with stderr", "echo boom >&2; exit 3", "Pre-connect script failed (exit 3): boom"},
		{"self-signal", "kill -TERM $$", "Pre-connect script failed (signal SIGTERM)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			_, err := s.Start(context.Background(), "c1", tt.command, nil, 0)
			if err == nil {
				t.Fatalf("Start: want an error, got none")
			}
			if err.Error() != tt.want {
				t.Errorf("Start error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

// TestDiedBetweenStartAndArm covers the race the `dead` field exists for: a sidecar that exits
// after Start resolved but before Arm was called must have its exit buffered and replayed
// synchronously by Arm, not dropped and not fired early.
func TestDiedBetweenStartAndArm(t *testing.T) {
	s := New()
	var oe exitCollector
	s.OnExit(oe.handle)

	got, err := s.Start(context.Background(), "c1", "sleep 0.2; echo dying >&2; exit 7", nil, 0)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Kind != KindSidecar {
		t.Fatalf("Kind = %q, want %q", got.Kind, KindSidecar)
	}

	waitUntil(t, 2*time.Second, func() bool { return entryDead(s, "c1") != nil })
	if oe.count() != 0 {
		t.Fatalf("OnExit fired before Arm: %+v", oe.exit)
	}

	s.Arm("c1")
	if oe.count() != 1 {
		t.Fatalf("OnExit did not fire synchronously from Arm, count=%d", oe.count())
	}
	exit := oe.last()
	if exit.Code == nil || *exit.Code != 7 {
		t.Errorf("Code = %v, want 7", exit.Code)
	}
	if exit.Signal != "" {
		t.Errorf("Signal = %q, want empty", exit.Signal)
	}
	if exit.LastStderr == nil || *exit.LastStderr != "dying" {
		t.Errorf("LastStderr = %v, want \"dying\"", exit.LastStderr)
	}
}

// TestSelfInflictedKillDoesNotFireOnExit covers the `killing` flag: an exit this supervisor
// itself caused (Stop, or a Start superseding a previous entry) must stay silent, or every
// disconnect would surface to the user as a dropped connection.
func TestSelfInflictedKillDoesNotFireOnExit(t *testing.T) {
	t.Run("Stop on an armed sidecar", func(t *testing.T) {
		s := New()
		var oe exitCollector
		s.OnExit(oe.handle)

		if _, err := s.Start(context.Background(), "c1", "sleep 30", nil, 0); err != nil {
			t.Fatalf("Start: %v", err)
		}
		s.Arm("c1")
		s.Stop("c1")
		time.Sleep(1 * time.Second)
		if oe.count() != 0 {
			t.Fatalf("OnExit fired %d times after a self-inflicted Stop, want 0", oe.count())
		}
	})

	t.Run("Start superseding a previous entry", func(t *testing.T) {
		s := New()
		var oe exitCollector
		s.OnExit(oe.handle)

		if _, err := s.Start(context.Background(), "c1", "sleep 30", nil, 0); err != nil {
			t.Fatalf("Start (1): %v", err)
		}
		s.Arm("c1")
		if _, err := s.Start(context.Background(), "c1", "sleep 30", nil, 0); err != nil {
			t.Fatalf("Start (2): %v", err)
		}
		time.Sleep(1 * time.Second)
		if oe.count() != 0 {
			t.Fatalf("OnExit fired %d times after a superseding Start, want 0", oe.count())
		}
		s.StopAll()
	})
}

// TestStopOnAlreadyDeadEntrySkipsSignalling: an entry whose sidecar died
// on its own before Arm was ever called (the exact case entryDead/TestDiedBetweenStartAndArm
// above covers) is deliberately left tracked in s.entries for Arm to consume — but Arm may never
// come (a connection removed, or a connect that fails, before arming). killEntry used to signal
// -e.pid unconditionally regardless: harmless against the real, still-around pid in this test, but
// on a busy machine a reaped pid can be recycled as an unrelated process group's leader, so
// signalling it would hit whatever that pid now is instead of a no-op. killSignal is swapped for a
// recorder so this test can assert the *attempt* never happens for an already-dead entry, without
// needing an actual pid recycling (which isn't something a test can force on demand).
func TestStopOnAlreadyDeadEntrySkipsSignalling(t *testing.T) {
	old := killSignal
	var calls []int
	killSignal = func(pid int, sig syscall.Signal) error {
		calls = append(calls, pid)
		return old(pid, sig)
	}
	t.Cleanup(func() { killSignal = old })

	// A script that survives long enough to settle as a sidecar, then exits on its own — the
	// settle window is 80ms in this package's test init, so "sleep 0.2" reliably settles first.
	s := New()
	got, err := s.Start(context.Background(), "c1", "sleep 0.2; exit 0", nil, 0)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Kind != KindSidecar {
		t.Fatalf("Kind = %q, want %q", got.Kind, KindSidecar)
	}
	waitUntil(t, 2*time.Second, func() bool { return entryDead(s, "c1") != nil })

	// Arm is never called — exactly the "died between Start and an Arm that never came" case.
	calls = nil
	s.Stop("c1")

	if len(calls) != 0 {
		t.Fatalf("killSignal called %d time(s) for an already-dead entry, want 0: pids %v", len(calls), calls)
	}
}

// TestSigtermEscalatesToSigkill covers killEntry's escalation: a script that ignores SIGTERM must
// still be dead by the time Stop returns, and Stop must block for the real exit.
func TestSigtermEscalatesToSigkill(t *testing.T) {
	s := New()
	if _, err := s.Start(context.Background(), "c1", `trap "" TERM; sleep 30`, nil, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	pid := entryPID(t, s, "c1")

	start := time.Now()
	s.Stop("c1")
	elapsed := time.Since(start)

	if processAlive(pid) {
		t.Fatalf("pid %d is still alive after Stop", pid)
	}
	if elapsed < killGrace {
		t.Errorf("Stop returned after %s, want at least killGrace (%s) since SIGTERM was ignored", elapsed, killGrace)
	}
}

// TestBackgroundedSetsidChildDoesNotDelayStart is F1 (P108 Part 3): a helper that left the
// process group (setsid) while holding the stderr pipe must not delay Start or Stop. The shell
// exits 0 at once, so Start resolves one-shot, not after a pipe-EOF wait.
func TestBackgroundedSetsidChildDoesNotDelayStart(t *testing.T) {
	s := New()
	var oe exitCollector
	s.OnExit(oe.handle)

	begin := time.Now()
	got, err := s.Start(context.Background(), "c1", "setsid sleep 2 & exit 0", nil, 0)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Kind != KindOneShot {
		t.Fatalf("Kind = %q, want %q", got.Kind, KindOneShot)
	}
	s.Stop("c1")
	if elapsed := time.Since(begin); elapsed >= time.Second {
		t.Errorf("Start+Stop took %s, want well under the helper's own 2s lifetime", elapsed)
	}
	if oe.count() != 0 {
		t.Fatalf("OnExit fired %d times, want 0", oe.count())
	}
}

// TestOneShotWithBackgroundChildKeepsRealExitAndIsReaped is P168 Part 2 F1: a script that exits
// while a background child in its group still holds stderr must be classified by its own exit
// code (not as a sidecar after the pipe closes), and the child must die on Stop.
func TestOneShotWithBackgroundChildKeepsRealExitAndIsReaped(t *testing.T) {
	t.Run("non-zero exit is an error and kills the child", func(t *testing.T) {
		pidFile := t.TempDir() + "/pid"
		s := New()
		_, err := s.Start(context.Background(), "c1", fmt.Sprintf("sleep 30 & echo $! > %s; echo boom >&2; exit 3", pidFile), nil, 0)
		if err == nil || !strings.Contains(err.Error(), "exit 3") || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("Start error = %v, want exit 3 with stderr tail", err)
		}
		waitUntil(t, 5*time.Second, func() bool { return !processAlive(readPID(t, pidFile)) })
	})

	t.Run("exit zero is one-shot and Stop kills the child", func(t *testing.T) {
		pidFile := t.TempDir() + "/pid"
		s := New()
		got, err := s.Start(context.Background(), "c1", fmt.Sprintf("sleep 30 >/dev/null 2>&1 & echo $! > %s", pidFile), nil, 0)
		if err != nil || got.Kind != KindOneShot {
			t.Fatalf("Start = %+v, %v, want one-shot", got, err)
		}
		pid := readPID(t, pidFile)
		if !processAlive(pid) {
			t.Fatalf("background child %d died before Stop", pid)
		}
		s.Stop("c1")
		waitUntil(t, 5*time.Second, func() bool { return !processAlive(pid) })
	})
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	var pid int
	waitUntil(t, 2*time.Second, func() bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil && pid > 0
	})
	return pid
}

// TestStartAbortsOnContextCancellationDuringSettle: a caller racing Start with
// a context that gets cancelled before the script settles (connections.Service.Disconnect/Remove
// against an in-flight Connect) must get an aborted Start back promptly, with the spawned process
// killed — it never got far enough to settle into s.entries, so an external Stop(id)
// call would otherwise find nothing tracked and leave it running untracked.
func TestStartAbortsOnContextCancellationDuringSettle(t *testing.T) {
	pidFile, err := os.CreateTemp(t.TempDir(), "pid")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = pidFile.Close()

	s := New()
	ctx, cancel := context.WithCancel(context.Background())
	// Well inside settleWindow (80ms in this package's test init): the process must still be
	// running (in its own "sleep 30", past the quick "echo $$" write) when ctx is cancelled.
	time.AfterFunc(20*time.Millisecond, cancel)

	start := time.Now()
	_, err = s.Start(ctx, "c1", fmt.Sprintf("echo $$ > %s; sleep 30", pidFile.Name()), nil, 0)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start error = %v, want context.Canceled", err)
	}
	// Start also waits for the killed group to empty (a lingering zombie costs up to 2*killGrace).
	if elapsed >= time.Second {
		t.Errorf("Start took %s after cancellation, want far under the script's 30s sleep", elapsed)
	}
	// Never tracked — the ctx.Done() branch returns before ever reaching s.entries[connectionID].
	if entryDead(s, "c1") != nil || s.entries["c1"] != nil {
		t.Errorf("c1 is tracked in s.entries after an aborted Start, want untracked")
	}

	var pid int
	waitUntil(t, 2*time.Second, func() bool {
		data, readErr := os.ReadFile(pidFile.Name())
		if readErr != nil || strings.TrimSpace(string(data)) == "" {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil && pid > 0
	})
	waitUntil(t, 2*time.Second, func() bool { return !processAlive(pid) })
}

// TestProcessGroupKillReachesGrandchild covers Setpgid + kill(-pgid): a pre-connect script that
// backgrounds its own child must not leave that grandchild running after Stop.
func TestProcessGroupKillReachesGrandchild(t *testing.T) {
	s := New()
	if _, err := s.Start(context.Background(), "c1", "sleep 300 & sleep 300", nil, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	pgid := entryPID(t, s, "c1")

	s.Stop("c1")

	// This sandbox's minimal container init reparents and reaps orphaned grandchildren more
	// slowly than a real system init (launchd/systemd) does, so kill(-pgid, 0) can keep
	// answering for a zombie for a second or two after it has already received and acted on
	// the signal — poll rather than asserting ESRCH on the first check.
	waitUntil(t, 5*time.Second, func() bool { return syscall.Kill(-pgid, 0) == syscall.ESRCH })
}

func fileReady(path string) *ReadyCheck {
	return &ReadyCheck{Addr: path, Check: func(context.Context) error {
		_, err := os.Stat(path)
		return err
	}}
}

// TestStartWaitsForProbePastSettleWindow pins the race fix: a script alive past settleWindow is
// not ready until the probe says so.
func TestStartWaitsForProbePastSettleWindow(t *testing.T) {
	flag := t.TempDir() + "/up"
	s := New()
	t.Cleanup(s.StopAll)

	start := time.Now()
	got, err := s.Start(context.Background(), "c1", fmt.Sprintf("sleep 0.4; touch %s; exec sleep 30", flag), fileReady(flag), 0)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Kind != KindSidecar {
		t.Errorf("Kind = %q, want sidecar", got.Kind)
	}
	if elapsed < 400*time.Millisecond {
		t.Errorf("Start returned after %s, before the probe could succeed", elapsed)
	}
	if _, statErr := os.Stat(flag); statErr != nil {
		t.Errorf("Start returned before the script became ready: %v", statErr)
	}
}

func TestStartProbeTimeoutKillsScript(t *testing.T) {
	old := readyTimeout
	readyTimeout = 300 * time.Millisecond
	t.Cleanup(func() { readyTimeout = old })

	pidFile := t.TempDir() + "/pid"
	s := New()
	_, err := s.Start(context.Background(), "c1", fmt.Sprintf("echo $$ > %s; sleep 30", pidFile), fileReady(t.TempDir()+"/never"), 0)
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("Start error = %v, want a not-ready error", err)
	}
	data, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		t.Fatalf("read pid: %v", readErr)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	waitUntil(t, 2*time.Second, func() bool { return !processAlive(pid) })
}

func TestStartCancelDuringProbeWait(t *testing.T) {
	pidFile := t.TempDir() + "/pid"
	s := New()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(150*time.Millisecond, cancel)

	_, err := s.Start(ctx, "c1", fmt.Sprintf("echo $$ > %s; sleep 30", pidFile), fileReady(t.TempDir()+"/never"), 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start error = %v, want context.Canceled", err)
	}
	data, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		t.Fatalf("read pid: %v", readErr)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	waitUntil(t, 2*time.Second, func() bool { return !processAlive(pid) })
}

// TestStartDelay pins the delay floor: Start returns at max(delay, readiness), an exit-0 script
// waits the delay out, and a failure or cancel ends it at once.
func TestStartDelay(t *testing.T) {
	alwaysReady := &ReadyCheck{Addr: "always", Check: func(context.Context) error { return nil }}
	neverReady := &ReadyCheck{Addr: "never", Check: func(context.Context) error { return errors.New("closed") }}
	oldTimeout := readyTimeout
	t.Cleanup(func() { readyTimeout = oldTimeout })

	tests := []struct {
		name       string
		command    string
		ready      *ReadyCheck
		timeout    time.Duration
		delay      time.Duration
		wantKind   string
		wantErr    string
		minElapsed time.Duration
		maxElapsed time.Duration
	}{
		{name: "exit 0 waits out delay", command: "true", delay: 300 * time.Millisecond, wantKind: KindOneShot, minElapsed: 300 * time.Millisecond},
		{name: "probe ready before delay", command: "exec sleep 30", ready: alwaysReady, delay: 300 * time.Millisecond, wantKind: KindSidecar, minElapsed: 300 * time.Millisecond},
		{name: "settle before delay", command: "exec sleep 30", delay: 300 * time.Millisecond, wantKind: KindSidecar, minElapsed: 300 * time.Millisecond},
		{name: "failure ends at once", command: "echo boom >&2; exit 3", delay: 5 * time.Second, wantErr: "boom", maxElapsed: time.Second},
		{name: "timeout counts from delay end", command: "exec sleep 30", ready: neverReady, timeout: 200 * time.Millisecond, delay: 300 * time.Millisecond, wantErr: "not ready", minElapsed: 500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readyTimeout = oldTimeout
			if tt.timeout > 0 {
				readyTimeout = tt.timeout
			}
			s := New()
			t.Cleanup(s.StopAll)
			start := time.Now()
			got, err := s.Start(context.Background(), "c1", tt.command, tt.ready, tt.delay)
			elapsed := time.Since(start)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Start error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil || got.Kind != tt.wantKind {
				t.Fatalf("Start = %+v, %v; want kind %q", got, err, tt.wantKind)
			}
			if elapsed < tt.minElapsed {
				t.Errorf("Start took %s, want >= %s", elapsed, tt.minElapsed)
			}
			if tt.maxElapsed > 0 && elapsed >= tt.maxElapsed {
				t.Errorf("Start took %s, want < %s", elapsed, tt.maxElapsed)
			}
		})
	}

	t.Run("cancel during delay kills group", func(t *testing.T) {
		pidFile := t.TempDir() + "/pid"
		s := New()
		t.Cleanup(s.StopAll)
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(100*time.Millisecond, cancel)
		start := time.Now()
		_, err := s.Start(ctx, "c1", fmt.Sprintf("echo $$ > %s; exec sleep 30", pidFile), nil, 5*time.Second)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Start error = %v, want context.Canceled", err)
		}
		if elapsed := time.Since(start); elapsed >= time.Second {
			t.Errorf("Start took %s after cancel, want < 1s", elapsed)
		}
		data, readErr := os.ReadFile(pidFile)
		if readErr != nil {
			t.Fatalf("read pid: %v", readErr)
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		waitUntil(t, 2*time.Second, func() bool { return !processAlive(pid) })
	})
}
