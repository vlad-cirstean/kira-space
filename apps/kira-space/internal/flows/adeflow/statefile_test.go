package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

func revOf(doc map[string]any) int {
	n, _ := doc["rev"].(int)
	return n
}

// P643 Part 5: state.yaml follows the board. A pending request, then its approval and the setup end
// reach the file with the status words; a rewrite with no change keeps rev; an agent's edit is
// restored from Kira's bytes.
func TestDetailsStateFile(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "gate")
	t.Cleanup(func() { _ = os.WriteFile(gate, nil, 0o644) })
	f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/state"})}, WaitFile: gate, Name: "done"}}, onceStepYML, false)
	f.startRun(t)
	req := f.waitRequest(t)

	pending := stateYAML(t, f.app, f.task.ID, func(d map[string]any) bool {
		p, _ := d["pending"].([]any)
		return len(p) == 1
	})
	if p := pending["pending"].([]any)[0].(map[string]any); p["kind"] != "branch.request" {
		t.Fatalf("pending item = %v, want a branch.request", p)
	}
	task, _ := pending["task"].(map[string]any)
	if task["id"] != f.task.ID {
		t.Fatalf("state.yaml task = %v, want %s", task, f.task.ID)
	}

	if _, err := approveBR(f.app, req.ID, []string{req.Items[0].ID}, nil); err != nil {
		t.Fatal(err)
	}
	ready := stateYAML(t, f.app, f.task.ID, func(d map[string]any) bool {
		repos, _ := d["repos"].([]any)
		if len(repos) != 1 {
			return false
		}
		setup, _ := repos[0].(map[string]any)["setup"].(map[string]any)
		return setup["status"] == "done"
	})
	if revOf(ready) <= revOf(pending) {
		t.Fatalf("rev %d after the changes, was %d: want it to grow", revOf(ready), revOf(pending))
	}

	folder := dfFolder(t, f.app, f.task.ID)
	// Settled: two reads with no board change between them hold the same rev (byte-equal skip).
	again := stateYAML(t, f.app, f.task.ID, nil)
	if revOf(again) != revOf(stateYAML(t, f.app, f.task.ID, nil)) {
		t.Fatal("rev moved with no change")
	}

	// Agent edit: Kira owns the file, a submit restores it.
	before := dfRead(t, folder, "state.yaml")
	dfWrite(t, folder, "state.yaml", "rev: 999\nhacked: true\n")
	dfSubmit(t, f.app, f.task.ID, agentmcp.SubmitArgs{})
	if got := dfRead(t, folder, "state.yaml"); strings.Contains(got, "hacked") || got != before {
		t.Fatalf("state.yaml after an agent edit and submit:\n%s\nwant Kira's bytes back", got)
	}
	_ = os.WriteFile(gate, nil, 0o644)
	waitRun(t, f.app, f.task.ID, "one", "done")
}
