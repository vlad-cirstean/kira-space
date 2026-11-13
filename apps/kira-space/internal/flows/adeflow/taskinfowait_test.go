package adeflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P623 task_info wait and registered-repo gating. The reply's repos are read after the wait, so they agree with the branch's
// setup state: ready, not the running state they held when the call started.
func TestTaskInfoWaitsForSetup(t *testing.T) {
	info := fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "task_info", Args: map[string]any{"repo": "api", "branch": "feat/wait", "waitSeconds": 30}}
	plain := fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "task_info"}
	withAll := fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "task_info", Args: map[string]any{"includeRegistered": true}}
	f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/wait"}), info, plain, withAll}, Name: "done"}}, onceStepYML, false)
	gate := filepath.Join(t.TempDir(), "gate")
	setPrepare(t, f.app, f.api, "while [ ! -f "+strconv.Quote(gate)+" ]; do sleep 0.05; done")
	f.startRun(t)
	req := f.waitRequest(t)
	if _, err := approveBR(f.app, req.ID, []string{req.Items[0].ID}, nil); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		_, err := os.Stat(filepath.Join(f.app.FakeDir, "mcp-request_branches-1.json"))
		return err == nil
	})
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, f.task.ID, "one", "done")
	var reply struct {
		Repos  []struct{ Setup struct{ State string } }
		Branch struct{ Setup struct{ State string } }
	}
	raw := fakeFile(t, f.app, "mcp-task_info-1.json")
	if err := json.Unmarshal([]byte(raw), &reply); err != nil {
		t.Fatalf("task_info reply %s: %v", raw, err)
	}
	if len(reply.Repos) != 1 || reply.Branch.Setup.State != "ready" || reply.Repos[0].Setup.State != "ready" {
		t.Fatalf("task_info reply = %s, want repos[0] and branch setup both ready", raw)
	}
	if n := registeredIn(t, fakeFile(t, f.app, "mcp-task_info-2.json")); n != 0 {
		t.Fatalf("registered = %d for a task with repos, want none", n)
	}
	if n := registeredIn(t, fakeFile(t, f.app, "mcp-task_info-3.json")); n != 2 {
		t.Fatalf("registered with includeRegistered = %d, want both repos", n)
	}
}

// registeredIn counts the task_info reply's registered list: empty for a task with repos, filled with
// includeRegistered.
func registeredIn(t *testing.T, raw string) int {
	t.Helper()
	var reply struct{ Registered []json.RawMessage }
	if err := json.Unmarshal([]byte(raw), &reply); err != nil {
		t.Fatalf("task_info reply %s: %v", raw, err)
	}
	return len(reply.Registered)
}
