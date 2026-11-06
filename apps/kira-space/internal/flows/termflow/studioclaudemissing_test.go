package termflow_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

// A Claude Code launch without the CLI fails with the Kira error and opens no session.
func TestClaudeCodeLaunchWithoutClaude(t *testing.T) {
	app := flowharness.New(t)
	if err := os.Remove(filepath.Join(app.BinDir, "claude")); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(app.BinDir, "stub-shell")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", stub)
	t.Cleanup(claude.SetSystemPaths())
	t.Setenv("PATH", app.BinDir)

	_, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "cc1", WindowKey: "w1", Cwd: app.Home, Cols: 80, Rows: 24,
		Command: "claude", LaunchKind: terminal.LaunchKindAgent,
	})
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeNotFound) || !strings.HasPrefix(ie.Message, "Claude Code CLI not found") {
		t.Fatalf("Open = %v, want the claude-missing Kira error", err)
	}
	app.StudioContract(t, "terminal", "TerminalService.Open#claudeMissing", map[string]any{"code": ie.Code, "message": ie.Message})
	if _, ok := app.W.TermRegistry.WindowOf("cc1"); ok {
		t.Fatal("a failed launch left a session registered")
	}
}
