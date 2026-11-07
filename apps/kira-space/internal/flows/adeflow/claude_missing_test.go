package adeflow_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// A launch without the claude CLI fails with the Kira error before any terminal opens.
func TestLaunchWithoutClaude(t *testing.T) {
	app := flowharness.New(t, flowharness.WithoutClaude())
	stub := filepath.Join(app.Root, "stub-shell")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", stub)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", "  - id: work\n    name: Work\n    kind: manual\n    status: in-progress\n    session: true\n"))
	task := createTask(t, app, "Fix login", "flow", rec.ID)

	_, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeNotFound) || !strings.HasPrefix(ie.Message, "Claude Code CLI not found") {
		t.Fatalf("LaunchStep = %v, want the claude-missing Kira error", err)
	}
	app.Contract(t, "ade-claude-missing", "AdeTaskService.LaunchStep#claudeMissing", map[string]any{"code": ie.Code, "message": ie.Message})
	res, err := app.W.AdeTask.Sessions(ctx)
	if err != nil || len(res.Sessions) != 0 {
		t.Fatalf("sessions after the failed launch = %+v, %v, want none", res.Sessions, err)
	}
}
