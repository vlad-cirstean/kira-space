package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// answersOf is the answers list of a parsed state.yaml.
func answersOf(doc map[string]any) []map[string]any {
	list, _ := doc["answers"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, a := range list {
		if m, ok := a.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// P643 Part 5: a state call that waits for the user ends at the call wait, nothing is cancelled.
// The reply is pending and names the state.yaml field; the item stays open; a late approval applies
// with no caller and lands in answers[] and repos[]. The run stays alive on a gate meanwhile.
func TestCallWaitBranchRename(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "gate")
	t.Cleanup(func() { _ = os.WriteFile(gate, nil, 0o644) })
	f, _, _ := newHeldRenameFixture(t, renameCall("feat/old", "feat/late", false), noSandbox, gate, flowharness.WithCallWait(time.Second))
	req := f.answerCreate(t, nil)

	reply, errText := stateReply(t, f.app, "mcp-branch.rename-1.json")
	if errText != "" {
		t.Fatalf("branch.rename after the wait: error %q, want a pending item", errText)
	}
	it := replyItem(t, reply)
	msg, _ := it["message"].(string)
	if it["status"] != "pending" || !strings.Contains(msg, "answers[]") || strings.Contains(strings.ToLower(msg), "call again") {
		t.Fatalf("item = %v, want pending naming answers[], no call-again text", it)
	}
	if got := f.waitRequest(t); got.ID != req.ID {
		t.Fatalf("request after the wait = %q, want the same open request %q", got.ID, req.ID)
	}
	stateYAML(t, f.app, f.task.ID, func(d map[string]any) bool {
		p, _ := d["pending"].([]any)
		return len(p) == 1
	})

	if _, err := approveBR(f.app, req.ID, []string{req.Items[0].ID}, nil); err != nil {
		t.Fatal(err)
	}
	doc := stateYAML(t, f.app, f.task.ID, func(d map[string]any) bool {
		for _, a := range answersOf(d) {
			if a["kind"] == "branch.rename" && a["status"] == "done" {
				return true
			}
		}
		return false
	})
	repos, _ := doc["repos"].([]any)
	if len(repos) != 1 || repos[0].(map[string]any)["branch"] != "feat/late" {
		t.Fatalf("repos after the late approval = %v, want feat/late", repos)
	}
	if pend, _ := doc["pending"].([]any); len(pend) != 0 {
		t.Fatalf("pending after the answer = %v, want none", pend)
	}
	if name := f.branchName(t, f.api); name != "feat/late" {
		t.Fatalf("branch = %q, want feat/late renamed by the late approval", name)
	}
	_ = os.WriteFile(gate, nil, 0o644)
	waitRun(t, f.app, f.task.ID, "two", "done")
}

// The owner's end withdraws an open call: the run finishing while a request waits leaves no item.
func TestCallWaitOwnerEndWithdraws(t *testing.T) {
	f, _, _ := newRenameFixture(t, renameCall("feat/old", "feat/gone", false), noSandbox, flowharness.WithCallWait(time.Second))
	f.answerCreate(t, nil)
	waitRun(t, f.app, f.task.ID, "two", "done")
	testx.WaitUntil(t, waitFor, func() bool { return taskOf(t, board(t, f.app), f.task.ID).BranchRequestID == "" })
}
