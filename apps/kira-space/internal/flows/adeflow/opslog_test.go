package adeflow_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of the ADE rows of the Operations panel; UI half: tests/ui/operations.spec.ts.
// Contract space-ops-ade.

const opsScenario = "space-ops-ade"

var opMask = flowharness.Mask("id", "startedAt", "durationMs", "parentId", "taskId", "steps")

func opsNow(t *testing.T, app *flowharness.App) []oplog.Record {
	t.Helper()
	rows, err := app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func findOp(rows []oplog.Record, category, kind string) (oplog.Record, bool) {
	for _, r := range rows {
		if r.Category == category && r.Kind == kind {
			return r, true
		}
	}
	return oplog.Record{}, false
}

func noAppModule(t *testing.T, rows []oplog.Record) {
	t.Helper()
	for _, r := range rows {
		if r.Module == oplog.ModuleApp {
			t.Errorf("row %s %s has module app", r.Category, r.Kind)
		}
	}
}

func TestOpsAde(t *testing.T) {
	t.Run("run", func(t *testing.T) {
		f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", ""))
		f.start(t, "feat/api-work")
		waitRun(t, f.app, f.taskID, "one", "done")
		testx.WaitUntil(t, waitFor, func() bool {
			r, ok := findOp(opsNow(t, f.app), oplog.CategorySession, "run")
			return ok && r.Status == oplog.StatusOK
		})
		rows := opsNow(t, f.app)
		wt, ok := findOp(rows, oplog.CategoryGit, "worktreeAdd")
		if !ok || wt.Module != oplog.ModuleADE || wt.Trigger != oplog.TriggerUser || wt.Status != oplog.StatusOK || wt.Background {
			t.Fatalf("worktreeAdd = %+v, want a foreground ade user row", wt)
		}
		run, _ := findOp(rows, oplog.CategorySession, "run")
		if run.Module != oplog.ModuleADE || run.Subject != "Fix login" || run.Source == "" {
			t.Fatalf("run op = %+v", run)
		}
		noAppModule(t, rows)
		f.app.Contract(t, opsScenario, "record#run", run, opMask, flowharness.Replace(f.repo.Dir, "<repo>"))
	})

	t.Run("cancel a running agent", func(t *testing.T) {
		flag := filepath.Join(t.TempDir(), "release")
		f := newRunFixture(t, []fakeagent.Action{{Name: "done", WaitFile: flag}}, agentStep("first", ""))
		f.start(t, "feat/api-work")
		var running oplog.Record
		testx.WaitUntil(t, waitFor, func() bool {
			r, ok := findOp(opsNow(t, f.app), oplog.CategorySession, "run")
			running = r
			return ok && r.Status == oplog.StatusRunning
		})
		if !running.Cancellable {
			t.Fatalf("running session op %+v is not cancellable", running)
		}
		f.app.Contract(t, opsScenario, "record#running", running, opMask)
		if err := f.app.W.Ops.Cancel(bridge.OpsCancelArgs{OpID: running.ID}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.taskID, "first", "stuck")
		testx.WaitUntil(t, waitFor, func() bool {
			r, _ := findOp(opsNow(t, f.app), oplog.CategorySession, "run")
			return r.Status == oplog.StatusCancelled
		})
	})

	t.Run("mcp request_branches", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/ops"})}, Name: "done"}}, onceStepYML, false)
		f.startRun(t)
		req := f.waitRequest(t)
		if _, err := approveBR(f.app, req.ID, []string{req.Items[0].ID}, nil); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		var call oplog.Record
		testx.WaitUntil(t, waitFor, func() bool {
			r, ok := findOp(opsNow(t, f.app), oplog.CategoryMCP, "request_branches")
			call = r
			return ok && r.Status == oplog.StatusOK
		})
		if call.Module != oplog.ModuleADE || call.Trigger != oplog.TriggerMCP || call.Source != "MCP request_branches" || call.Command == nil {
			t.Fatalf("mcp op = %+v", call)
		}
		var child oplog.Record
		for _, r := range opsNow(t, f.app) {
			if r.Category == oplog.CategoryGit && r.ParentID == call.ID {
				child = r
			}
		}
		if child.ID == "" {
			t.Fatal("no git op is a child of the MCP call")
		}
		if child.Kind != "worktreeAdd" || child.Module != oplog.ModuleADE || len(child.Steps) == 0 || child.Command == nil {
			t.Fatalf("child op = %+v, want an ade worktreeAdd with steps", child)
		}
		folder := regexp.MustCompile(`/kira-space-wts/api/(\w+)`).FindStringSubmatch(*child.Command)
		if folder == nil {
			t.Fatalf("no kira-space-wts/api/<folder> worktree in %q", *child.Command)
		}
		f.app.Contract(t, opsScenario, "record#child", child, flowharness.Mask("id", "startedAt", "durationMs", "parentId", "taskId"),
			flowharness.Replace(f.app.Work, "<work>"), flowharness.Replace(folder[1], "<folder-id>"))
		f.app.Contract(t, opsScenario, "record#mcp", call, opMask)
		noAppModule(t, opsNow(t, f.app))
	})
}

func TestOpsBackgroundDedupe(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", ""))
	if err := os.WriteFile(filepath.Join(f.repo.Dir, ".git", "HEAD"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		_, _ = f.app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{CodeRepoIDs: []string{f.repoID}})
	}
	var failed []oplog.Record
	for _, r := range opsNow(t, f.app) {
		if r.Status == oplog.StatusError && r.Background {
			failed = append(failed, r)
		}
	}
	if len(failed) != 1 || failed[0].Repeat != 3 || failed[0].Error == nil {
		t.Fatalf("background failures = %+v, want one row repeated 3 times", failed)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		var n int
		_ = f.app.DB().QueryRow(`SELECT COUNT(*) FROM op_log WHERE id = ? AND repeat_count = 3`, failed[0].ID).Scan(&n)
		return n == 1
	})
	f.app.Contract(t, opsScenario, "record#repeated", failed[0], opMask, flowharness.Mask("error"), flowharness.Replace(f.repo.Dir, "<repo>"))
}

func TestOpsCheckRefFormat(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", ""))
	br := branchOf(t, board(t, f.app), f.taskID, f.repoID)
	if _, err := f.app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: br.ID, Name: "bad..name"}); err == nil {
		t.Fatal("an invalid branch name was accepted")
	}
	var found []oplog.Record
	for _, r := range opsNow(t, f.app) {
		cmd := ""
		if r.Command != nil {
			cmd = *r.Command
		}
		if strings.Contains(cmd, "check-ref-format") {
			found = append(found, r)
		}
	}
	if len(found) == 0 {
		t.Fatal("no check-ref-format spawn recorded")
	}
	for _, r := range found {
		if r.Status != oplog.StatusOK || r.Module != oplog.ModuleADE {
			t.Errorf("check-ref-format op = %+v, want ok in module ade (an expected non-zero exit)", r)
		}
	}
}
