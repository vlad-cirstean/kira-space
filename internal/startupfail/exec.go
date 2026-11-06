package startupfail

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// realLookPath/realStat/realGetenv are Deps' zero-value fallbacks — exec.LookPath/os.Stat/
// os.Getenv, unwrapped, so a caller providing no Deps at all gets the real OS (the usual
// seam-with-real-default pattern).
func realLookPath(name string) (string, error)  { return exec.LookPath(name) }
func realStat(path string) (os.FileInfo, error) { return os.Stat(path) }

// realGetenv exists as its own named function (rather than os.Getenv used inline) for one reason:
// scripts/verify-packaging.sh's check S8 fails the build if apps/kira-space/main.go itself
// contains "os.Getenv" — so the one environment read this package needs (KIRA_NO_STARTUP_ALERT)
// must happen entirely inside internal/startupfail, never in main.go.
func realGetenv(key string) string { return os.Getenv(key) }

// realRun is the argv-only spawn — toolexec's argv-only run (the house precedent) adapted
// to also carry stdin bytes (pbcopy reads its clipboard payload on stdin) and return stdout
// as a string (present() needs to read which button `display alert` returned). Setpgid (not
// Setsid), WaitDelay, and cmd.Env = os.Environ() are toolexec's own choices for the same reason: a
// short-lived, non-interactive spawn that still needs the user's real session environment to reach
// its target — osascript needs a real Aqua session to show anything at all, pbcopy the same
// pasteboard server.
func realRun(ctx context.Context, path string, args []string, stdin []byte) (string, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = os.Environ()
	cmd.WaitDelay = gracefulStopDelay
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), err
}
