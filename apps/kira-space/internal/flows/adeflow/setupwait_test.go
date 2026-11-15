package adeflow_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P643 branch.request waits for setup: after approval the reply holds back until setup is ready, so
// it reports done with setup ready, and state.yaml agrees. A gated prepare script holds setup open.
func TestBranchRequestWaitsForSetup(t *testing.T) {
	f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/wait"})}, Name: "done"}}, onceStepYML, false)
	gate := filepath.Join(t.TempDir(), "gate")
	t.Cleanup(func() { _ = os.WriteFile(gate, nil, 0o644) })
	setPrepare(t, f.app, f.api, "while [ ! -f "+strconv.Quote(gate)+" ]; do sleep 0.05; done")
	f.startRun(t)
	req := f.waitRequest(t)
	if _, err := approveBR(f.app, req.ID, []string{req.Items[0].ID}, nil); err != nil {
		t.Fatal(err)
	}
	reply := filepath.Join(f.app.FakeDir, "mcp-branch.request-1.json")
	stateYAML(t, f.app, f.task.ID, func(d map[string]any) bool {
		repos, _ := d["repos"].([]any)
		if len(repos) != 1 {
			return false
		}
		setup, _ := repos[0].(map[string]any)["setup"].(map[string]any)
		return setup["status"] == "waiting"
	})
	time.Sleep(300 * time.Millisecond)
	if raw, _ := os.ReadFile(reply); len(raw) > 0 {
		t.Fatalf("branch.request answered while setup ran: %s", raw)
	}
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, f.task.ID, "one", "done")
	r, errText := stateReply(t, f.app, "mcp-branch.request-1.json")
	if errText != "" {
		t.Fatal(errText)
	}
	if it := replyItem(t, r); it["status"] != "done" {
		t.Fatalf("branch.request item = %v, want done after setup", it)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		repos, _ := stateYAML(t, f.app, f.task.ID, nil)["repos"].([]any)
		if len(repos) != 1 {
			return false
		}
		setup, _ := repos[0].(map[string]any)["setup"].(map[string]any)
		return setup["status"] == "done"
	})
}
