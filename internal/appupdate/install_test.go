package appupdate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/procgroup"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// AU/install_test.go earns its keep against CLAUDE.md's unit-test bar: process-lifecycle
// concurrency (hand-off ordering, cancellation, whole-group kill) is exactly the kind of thing
// this repo's own testing rule calls out. Not tested here: validateScript (two line
// compares), the dev-build guard and the already-in-progress guard (one `if` each), and the
// deadline path (same code as Cancel, a different ctx.Err()).

func newTestInstaller(t *testing.T, script string) (*Installer, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "install.log")
	i := &Installer{
		app:     Space,
		running: "1.0.0",
		shell:   "/bin/sh",
		logPath: logPath,
		fetch: func(ctx context.Context) ([]byte, error) {
			return []byte(script), nil
		},
	}
	return i, dir
}

// killIfAlive is the t.Cleanup safety net every test below registers immediately after learning a
// pid — a failing assertion must never leave a sleep 30 (or its parent) running past the test.
// procgroup.Kill treats "already gone" as success, so this needs no aliveness check of its own.
func killIfAlive(t *testing.T, pid int) {
	t.Cleanup(func() {
		_ = procgroup.Kill(pid, syscall.SIGKILL)
	})
}

func readPidFile(t *testing.T, path string) int {
	t.Helper()
	var pid int
	testx.WaitUntil(t, 2*time.Second, func() bool {
		data, err := os.ReadFile(path)
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			return false
		}
		p, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return false
		}
		pid = p
		return true
	})
	return pid
}

func TestInstaller_Stage_HandsOffOnStaged(t *testing.T) {
	dir := t.TempDir()
	selfPidFile := filepath.Join(dir, "self.pid")
	script := fmt.Sprintf("#!/bin/sh\n%s\necho $$ > %q\nprintf 'staged v9.9.9\\n' >&3\nexec 3>&-\nsleep 30\n",
		installContract, selfPidFile)

	i, _ := newTestInstaller(t, script)

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- i.Stage(context.Background()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Stage() = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stage did not return within 2s of the staged hand-off")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Stage took %s, want <= 2s", elapsed)
	}

	pid := readPidFile(t, selfPidFile)
	killIfAlive(t, pid)
	if !testx.ProcessAlive(pid) {
		t.Fatal("script process is gone, want it still alive (outliving the handshake)")
	}

	i.mu.Lock()
	state := i.state
	i.mu.Unlock()
	if state != stateHandedOff {
		t.Fatalf("state = %v, want stateHandedOff", state)
	}
}

func TestInstaller_Stage_ReportsScriptFailure(t *testing.T) {
	script := fmt.Sprintf("#!/bin/sh\n%s\necho 'ERROR: boom' >&2\nexit 1\n", installContract)
	i, _ := newTestInstaller(t, script)

	err := i.Stage(context.Background())
	if err == nil {
		t.Fatal("Stage() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Stage() error = %q, want it to contain %q", err.Error(), "boom")
	}

	i.mu.Lock()
	state := i.state
	i.mu.Unlock()
	if state != stateIdle {
		t.Fatalf("state = %v, want stateIdle after a reported failure", state)
	}
}

func TestInstaller_Cancel_KillsWholeGroup(t *testing.T) {
	dir := t.TempDir()
	selfPidFile := filepath.Join(dir, "self.pid")
	childPidFile := filepath.Join(dir, "child.pid")
	script := fmt.Sprintf("#!/bin/sh\n%s\necho $$ > %q\nsleep 30 &\necho $! > %q\nwait\n",
		installContract, selfPidFile, childPidFile)

	i, _ := newTestInstaller(t, script)

	done := make(chan error, 1)
	go func() { done <- i.Stage(context.Background()) }()

	selfPid := readPidFile(t, selfPidFile)
	childPid := readPidFile(t, childPidFile)
	killIfAlive(t, selfPid)
	killIfAlive(t, childPid)

	i.Cancel()

	select {
	case err := <-done:
		ie := testx.AsIpcErr(t, err)
		if ie.Code != "E_CANCELLED" {
			t.Fatalf("Stage() code = %q, want E_CANCELLED", ie.Code)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("Stage did not return within 12s of Cancel")
	}

	testx.WaitUntil(t, 2*time.Second, func() bool { return !testx.ProcessAlive(selfPid) })
	testx.WaitUntil(t, 2*time.Second, func() bool { return !testx.ProcessAlive(childPid) })
}
