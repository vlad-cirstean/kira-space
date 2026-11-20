package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func revOf(doc map[string]any) int {
	n, _ := doc["rev"].(int)
	return n
}

// P643 Part 5: state.yaml follows the board. A pending rename, then its approval
// reaches the file with the status words; a rewrite with no change keeps rev; an agent's edit is
// restored from Kira's bytes.
func TestDetailsStateFile(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "gate")
	t.Cleanup(func() { _ = os.WriteFile(gate, nil, 0o644) })
	f, _, _ := newHeldRenameFixture(t, renameCall("feat/old", "feat/state", false), noSandbox, gate)
	req := f.answerCreate(t, nil)

	pending := stateYAML(t, f.app, f.task.ID, func(d map[string]any) bool {
		p, _ := d["pending"].([]any)
		return len(p) == 1
	})
	if p := pending["pending"].([]any)[0].(map[string]any); p["kind"] != "branch.rename" {
		t.Fatalf("pending item = %v, want a branch.rename", p)
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
		row, _ := repos[0].(map[string]any)
		setup, _ := row["setup"].(map[string]any)
		return row["branch"] == "feat/state" && setup["status"] == "done"
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
	dfSync(t, f.app, f.task.ID, "state.yaml")
	if got := dfRead(t, folder, "state.yaml"); strings.Contains(got, "hacked") || got != before {
		t.Fatalf("state.yaml after an agent edit and submit:\n%s\nwant Kira's bytes back", got)
	}
	_ = os.WriteFile(gate, nil, 0o644)
	waitRun(t, f.app, f.task.ID, "two", "done")
}
