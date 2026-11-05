package adeflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of an agent stopping its task's git operation through list_operations and
// cancel_operation; UI half: tests/ui/operations.spec.ts. Contract space-ops-agent-cancel.

func TestAgentCancelsTaskOperation(t *testing.T) {
	listCall := fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "list_operations"}
	f := newRunFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{listCall}, Name: "done"}},
		agentStep("one", "")+agentStep("two", "    before: approval\n"))
	log := f.app.W.Ops.Log
	start := func(taskID, category string) *oplog.Op {
		op := log.Start(oplog.WithOrigin(ctx, oplog.Origin{Module: oplog.ModuleADE, Trigger: oplog.TriggerMCP, TaskID: taskID}),
			oplog.Meta{Category: category, Kind: "fetch", Subject: "origin"})
		// The cancel func only signals; Finish takes the log lock the caller holds.
		op.SetCancel(func() bool { go op.Finish(oplog.StatusCancelled, ""); return true })
		return op
	}
	mine := start(f.taskID, oplog.CategoryGit)
	other := start("other-task", oplog.CategoryGit)
	session := start(f.taskID, oplog.CategorySession)

	f.start(t, "feat/api-work")
	waitRun(t, f.app, f.taskID, "one", "done")
	var listed struct {
		Operations []agentmcp.OperationInfo `json:"operations"`
	}
	raw, err := os.ReadFile(filepath.Join(f.app.FakeDir, "mcp-list_operations-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatalf("list_operations result %s: %v", raw, err)
	}
	if len(listed.Operations) != 1 || listed.Operations[0].OpID != mine.ID() {
		t.Fatalf("list_operations = %s, want only the task's git op %s", raw, mine.ID())
	}

	// Step two cancels the listed id, then an id of another task's op and of the session op.
	cancel := func(id string) fakeagent.MCPCall {
		return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "cancel_operation", Tolerate: true, Args: map[string]any{"opId": id}}
	}
	claude(f.app, map[string][]fakeagent.Action{"*": {{Name: "done"}, {
		MCP:  []fakeagent.MCPCall{cancel(mine.ID()), cancel(other.ID()), cancel(session.ID())},
		Name: "done",
	}}})
	if err := f.app.W.AdeTask.Approve(ctx, adewire.StepArgs{TaskID: f.taskID, StepID: "two"}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	waitRun(t, f.app, f.taskID, "two", "done")

	byID := map[string]oplog.Record{}
	testx.WaitUntil(t, waitFor, func() bool {
		for _, r := range opsNow(t, f.app) {
			byID[r.ID] = r
		}
		return byID[mine.ID()].Status == oplog.StatusCancelled
	})
	if got := byID[mine.ID()]; got.Status != oplog.StatusCancelled {
		t.Fatalf("task op = %+v, want cancelled", got)
	}
	for _, id := range []string{other.ID(), session.ID()} {
		if got := byID[id]; got.Status != oplog.StatusRunning {
			t.Fatalf("op %s = %+v, want it untouched", id, got)
		}
	}
	f.app.Contract(t, "space-ops-agent-cancel", "record#cancelled", byID[mine.ID()], opMask)
}
