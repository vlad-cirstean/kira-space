// Package toolexec is the argv-only, bounded-output local-tool runner mcpinstall's `claude mcp
// add` spawn and the former editor-install spawns both carried verbatim: the
// SIGTERM-then-SIGKILL process-group cancellation, bounded stderr capture, and the PATH-then-
// candidates executable probe every discovery package in this codebase repeats.
package toolexec

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kirathecat/kira-studio/internal/procgroup"
)

// GracefulStopDelay bounds Run's own cmd.Cancel SIGTERM-then-SIGKILL escalation and cmd.WaitDelay
// — a var, not a const (a test seam), so a test can shrink it rather than
// costing real wall-clock time.
var GracefulStopDelay = 2 * time.Second

// MaxDetailBytes bounds how much of a child's stderr a caller retains for a UI string — one line,
// not a diagnostic log.
const MaxDetailBytes = 4096

// ExecError is Run's own failure shape for a spawn that ran and exited non-zero — distinct from a
// spawn that could not start at all (a plain error), so a caller can tell "the tool refused" from
// "the tool could not be launched".
type ExecError struct {
	ExitCode int
	// Stderr is bounded to MaxDetailBytes and truncated to its first line — never the whole of a
	// child's stderr verbatim (it may itself carry a path or something a user typed).
	Stderr string
}

func (e *ExecError) Error() string {
	if e.Stderr == "" {
		return "exit " + strconv.Itoa(e.ExitCode)
	}
	return "exit " + strconv.Itoa(e.ExitCode) + ": " + e.Stderr
}

// FirstLineBounded returns s's first line, trimmed and bounded to max bytes.
func FirstLineBounded(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > max {
		s = s[:max]
	}
	return s
}

// IsExecutable reports whether path exists, is a regular file (or a symlink to one), and has at
// least one executable bit set — the same check exec.LookPath makes internally, exposed here so
// an absolute-path candidate check (one that never goes through LookPath) can make it too.
func IsExecutable(stat func(string) (os.FileInfo, error), path string) bool {
	info, err := stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

// Locate finds name first via lookPath, then by testing each absolute candidate in order with
// stat — every path considered is recorded in probed regardless of outcome, so a miss can be
// explained, not just reported.
func Locate(lookPath func(string) (string, error), stat func(string) (os.FileInfo, error), name string, candidates []string) (path string, probed []string, found bool) {
	if resolved, err := lookPath(name); err == nil {
		probed = append(probed, resolved)
		return resolved, probed, true
	}
	probed = append(probed, name+" (on PATH)")

	for _, candidate := range candidates {
		probed = append(probed, candidate)
		if IsExecutable(stat, candidate) {
			return candidate, probed, true
		}
	}
	return "", probed, false
}

// killGroup signals pid's whole process group — ESRCH (already gone) is not an error, every other
// Kill failure is. A var, not a plain func, so a test can observe whether Run's own SIGKILL
// escalation was actually invoked.
var killGroup = procgroup.Kill

// Run is the argv-only spawn every local tool in this codebase makes: os/exec never interprets
// args, so a path or token containing a space is passed as one argument by construction — no sh,
// no -c, no string command line, no interpolation. Setpgid (not Setsid) targets a group signal at
// a stray grandchild without detaching from the caller's own session.
//
// cmd.Cancel overrides exec.CommandContext's own default (an immediate, ungraceful
// Process.Kill() of the direct child only) with a SIGTERM-then-SIGKILL escalation: without it, a
// spawn that forked something long-lived left the grandchild running after the parent was
// killed/timed out. The escalation timer is stopped once cmd.Run()
// returns, whether that was a clean exit or the escalation itself firing — otherwise a group that
// exits cleanly on SIGTERM left the timer armed regardless, and a SIGKILL fired after the full
// delay could land on an unrelated group that has since reused the same pid.
func Run(ctx context.Context, path string, args []string) error {
	_, err := spawn(ctx, spec{path: path, args: args, env: os.Environ()})
	return err
}

// MaxStdoutBytes bounds RunIO's captured stdout.
const MaxStdoutBytes = 1 << 20

// ErrOutputTooLarge is RunIO's refusal when a child writes more than MaxStdoutBytes.
var ErrOutputTooLarge = errors.New("toolexec: child output exceeds limit")

// RunIO is Run with an explicit working directory, environment and stdin, returning the child's
// bounded stdout (also alongside an *ExecError, so a caller can read a structured failure). An empty dir keeps the caller's own; env is used as given.
func RunIO(ctx context.Context, path string, args []string, dir string, env []string, stdin []byte) ([]byte, error) {
	return spawn(ctx, spec{path: path, args: args, dir: dir, env: env, stdin: stdin, capture: true})
}

// RunIOTracked is RunIO that reports the child's pid once started: onStart returns a done func
// called after the child is reaped. A caller uses it to track a live process for its lifetime.
func RunIOTracked(ctx context.Context, path string, args []string, dir string, env []string, stdin []byte, onStart func(pid int) (done func())) ([]byte, error) {
	return spawn(ctx, spec{path: path, args: args, dir: dir, env: env, stdin: stdin, capture: true, onStart: onStart})
}

type spec struct {
	path, dir string
	args, env []string
	stdin     []byte
	capture   bool
	onStart   func(pid int) (done func())
}

type boundedBuffer struct {
	buf      bytes.Buffer
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > MaxStdoutBytes {
		b.overflow = true
		return 0, ErrOutputTooLarge
	}
	return b.buf.Write(p)
}

func spawn(ctx context.Context, sp spec) ([]byte, error) {
	cmd := exec.CommandContext(ctx, sp.path, sp.args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Run passes the environment unmodified: a local tool needs the user's own HOME/PATH to find
	// its own config/install, unlike a spawn this app has already fully resolved and can run
	// against a scrubbed environment (RunIO's callers).
	cmd.Env = sp.env
	cmd.Dir = sp.dir
	stopEscalate := procgroup.GracefulCancel(cmd, GracefulStopDelay, killGroup)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	var stdout boundedBuffer
	if sp.capture {
		cmd.Stdout = &stdout
		cmd.Stdin = bytes.NewReader(sp.stdin)
	}
	var err error
	if sp.onStart == nil {
		err = cmd.Run()
	} else if err = cmd.Start(); err == nil {
		done := sp.onStart(cmd.Process.Pid)
		err = cmd.Wait()
		if done != nil {
			done()
		}
	}
	// On timeout/cancel a SIGTERM-ignoring group member can outlive the direct child; stopEscalate
	// would disarm the SIGKILL that targets it, so kill the group outright first.
	if ctx.Err() != nil && cmd.Process != nil {
		_ = procgroup.Kill(cmd.Process.Pid, syscall.SIGKILL)
	}
	stopEscalate()
	if stdout.overflow {
		return nil, ErrOutputTooLarge
	}
	if err == nil {
		return stdout.buf.Bytes(), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.buf.Bytes(), &ExecError{ExitCode: exitErr.ExitCode(), Stderr: FirstLineBounded(stderr.String(), MaxDetailBytes)}
	}
	return nil, err
}
