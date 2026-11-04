package gitprepare

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"syscall"
	"time"

	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/procgroup"
)

// DefaultPrepareTimeout is the hard timeout a repo gets until it sets worktreePrepareTimeout
// (P145 F2; supersedes G25 D12's fixed cap). A runaway script (an interactive prompt it silently
// hangs on, an infinite build loop) is killed at the deadline rather than left to run indefinitely.
const DefaultPrepareTimeout = 15 * time.Minute

// gracefulStopDelay is SIGTERM's own grace window before escalating to SIGKILL — mirrors
// githost/github/runner.go's own constant of the same name and purpose exactly (this package's own
// precedent for "a spawned non-git binary gets a real chance to clean up, then dies for real").
//
// A var, not a const (mirrors internal/preconnect/supervisor.go's own killGrace/settleWindow
// precedent, and gitclient/runner.go's own identical fix): runner_test.go
// lowers it so a regression test proving cmd.Cancel's SIGKILL escalation timer is stopped
// once the process exits cleanly doesn't cost 2s of real wall-clock time.
var gracefulStopDelay = 2 * time.Second

// Spec is one prepare-script invocation — everything Run needs. Shell/LoginShell/Script are fed
// straight to BuildArgv; Env is the FULL child environment (BuildEnv's own output, never appended
// to os.Environ() a second time by this file).
type Spec struct {
	Shell      string
	LoginShell bool
	Script     string
	Dir        string
	Env        []string
	// Timeout is the hard deadline for the whole run; required (> 0).
	Timeout time.Duration
	// OnBatch, when non-nil, is called with every throttled, capped batch of sanitized output
	// lines as they arrive — never called again after Run returns. May be called from a
	// goroutine other than Run's own caller; callers that touch shared state from it must
	// synchronize themselves (gitsession's own conn.Emit already does).
	OnBatch func([]Line)
	// Wrap is an argv prefix the shell command runs behind, such as a sandbox runner and its profile.
	Wrap []string
	// OnStart, when non-nil, is called with the child's pid right after it starts.
	OnStart func(pid int)
}

// Result is Run's own outcome — Output is the FINAL, capped/sanitized transcript (never the live
// stream OnBatch already delivered piecemeal).
type Result struct {
	ExitCode  int
	TimedOut  bool
	Cancelled bool
	Output    []Line
	Truncated bool
	// Incomplete is the signal, distinct from Truncated (which means "hit the
	// size cap"): true when the script itself exited successfully but a background process it
	// spawned still held stdout/stderr open past WaitDelay, so the transcript may be missing
	// whatever that lingering process still had buffered or went on to write.
	Incomplete bool
}

// Runner is the spawn seam (D18/F5) — the one thing gitsession's own tests fake; NewOSRunner is the
// only production implementation.
type Runner interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}

// NewOSRunner returns the real, os/exec-backed Runner.
func NewOSRunner() Runner { return osRunner{} }

type osRunner struct{}

// A var, not a plain func (internal/preconnect/supervisor.go's own killSignal is this codebase's
// precedent) so a test can observe whether cmd.Cancel's SIGKILL escalation was actually invoked.
var killGroup = procgroup.Kill

// Run spawns loginshell.BuildArgv(spec.Shell, spec.LoginShell, spec.Script) under
// spec.Dir/spec.Env: stdin is never set on the *exec.Cmd, which os/exec documents as reading from
// the null device — so the child never inherits this process's own stdin and is never attached
// to a pty; Setsid puts it in its own session so a group signal reaches whatever it forks, not just
// the shell itself; a spec.Timeout deadline is layered onto ctx (which is ALSO ctx's own
// cancellation — either one triggers the same SIGTERM-then-SIGKILL sequence cmd.Cancel/WaitDelay
// implement, mirroring githost/github/runner.go's own precedent exactly). Output is sanitized,
// capped and streamed through outputCollector as it arrives — never buffered raw and processed
// only at exit.
func (osRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	if spec.Timeout <= 0 {
		return Result{}, errors.New("gitprepare: Spec.Timeout must be positive")
	}
	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	argv := append(slices.Clone(spec.Wrap), loginshell.BuildArgv(spec.Shell, spec.LoginShell, spec.Script)...)
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// stopEscalate is read after cmd.Wait() returns, below — without stopping it there, a group
	// that exits cleanly on SIGTERM left this timer armed for the full gracefulStopDelay
	// regardless, and a SIGKILL fired at cmd.Process.Pid's process group after that delay could —
	// narrowly, but — land on an unrelated group that has since reused the same pid. No lock is
	// needed: cmd.Wait is documented to block until any goroutine running Cancel has finished,
	// which is exactly the happens-before this needs.
	stopEscalate := procgroup.GracefulCancel(cmd, gracefulStopDelay, killGroup)

	collector := newOutputCollector(spec.OnBatch)
	cmd.Stdout = collector.stdoutWriter()
	cmd.Stderr = collector.stderrWriter()

	if err := cmd.Start(); err != nil {
		return Result{}, err
	}
	if spec.OnStart != nil {
		spec.OnStart(cmd.Process.Pid)
	}

	tickerDone := make(chan struct{})
	tickerExited := make(chan struct{})
	ticker := time.NewTicker(batchInterval)
	go func() {
		defer close(tickerExited)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				collector.tick()
			case <-tickerDone:
				return
			}
		}
	}()

	waitErr := cmd.Wait()
	stopEscalate()
	close(tickerDone)
	<-tickerExited
	collector.flush()

	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	cancelled := !timedOut && ctx.Err() != nil

	if (timedOut || cancelled) && cmd.Process != nil {
		// Wait returning only means cmd.Process (SIGTERM's own direct target,
		// the shell) has exited — any OTHER process-group member that ignores SIGTERM can still be
		// running, and escalate.Stop() just above cancelled the only thing that would otherwise
		// have killed it, letting it outlive the documented hard timeout. Send SIGKILL to the whole
		// group immediately and unconditionally here instead (tolerate ESRCH: the group may already
		// be fully gone). The pid-reuse worry (why Stop() exists at all, on the
		// DELAYED escalation timer) does not apply to this immediate, same-instant kill: Linux does
		// not free a PGID for reuse while any member of it is still alive, and this fires the
		// instant Wait proves cmd.Process itself — a member of exactly this group — has just been
		// reaped, with no gracefulStopDelay-sized window for anything to reuse it in between.
		_ = killGroup(cmd.Process.Pid, syscall.SIGKILL)
	}

	result := Result{
		Output: collector.finalLines(), Truncated: collector.isTruncated(),
		TimedOut: timedOut, Cancelled: cancelled,
	}
	if waitErr == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	hasExitErr := errors.As(waitErr, &exitErr)
	if hasExitErr {
		result.ExitCode = exitErr.ExitCode()
	}
	if errors.Is(waitErr, exec.ErrWaitDelay) {
		// The script's own process can exit successfully while a background
		// child it spawned still holds stdout/stderr open — WaitDelay's own deadline then stops
		// waiting on that redirection and Wait returns a wrapped ErrWaitDelay (alone, on a clean
		// exit; joined with the *exec.ExitError above, on a non-zero one). Neither a timeout/
		// cancellation nor a genuine spawn failure: cmd.ProcessState is still populated (the
		// process itself already exited), so take the exit code from there when no *exec.ExitError
		// already supplied one, and mark the transcript as possibly incomplete rather than
		// discarding it as a bare RPC error.
		if !hasExitErr && cmd.ProcessState != nil {
			result.ExitCode = cmd.ProcessState.ExitCode()
		}
		result.Incomplete = true
		return result, nil
	}
	if hasExitErr {
		return result, nil
	}
	if timedOut || cancelled {
		// The process was killed by our own cancellation/timeout — Wait's error in that case is
		// "signal: killed" or similar, not a genuine spawn failure; TimedOut/Cancelled already say
		// what happened, so this is a normal (if unsuccessful) result, not a Go error.
		return result, nil
	}
	return Result{}, waitErr
}
