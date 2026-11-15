package adeflow_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func toolCall(tool string, args map[string]any) fakeagent.MCPCall {
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: tool, Tolerate: true, Args: args}
}

// stateCall is one state tool call of the fake agent. Its result file is mcp-<action>-<n>.json.
func stateCall(action string, args map[string]any) fakeagent.MCPCall {
	return toolCall("state", map[string]any{"action": action, "args": args})
}

// stateReply reads a recorded state reply: the structured reply, or the text of an MCP error.
func stateReply(t *testing.T, app *flowharness.App, name string) (reply map[string]any, errText string) {
	t.Helper()
	var raw []byte
	testx.WaitUntil(t, waitFor, func() bool {
		var err error
		raw, err = os.ReadFile(filepath.Join(app.FakeDir, name))
		return err == nil && json.Valid(raw)
	})
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, raw)
	}
	if out["isError"] == true {
		text, _ := out["text"].(string)
		return nil, text
	}
	return out, ""
}

// replyItem is the first item of a state reply.
func replyItem(t *testing.T, reply map[string]any) map[string]any {
	t.Helper()
	items, _ := reply["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("reply %v has no items", reply)
	}
	return items[0].(map[string]any)
}

// J-RUN-032, J-ADE-056, OQ-ADE-6. One run calls state with valid and invalid input interleaved.
// Invalid input is an invalid item with a fix and changes nothing; after the accepted step.finish
// every call, step.finish included, answers "step already finished" and the outcome stays.
func TestStateToolInvalidInputAndAfterFinish(t *testing.T) {
	calls := []fakeagent.MCPCall{
		stateCall("repo.declare", map[string]any{"repos": []any{"no-such-repo"}}),                // 1 invalid: unknown repo
		stateCall("branch.request", map[string]any{"repo": "api"}),                               // 1 invalid: no branch
		stateCall("lease.acquire", map[string]any{"queue": ""}),                                  // 1 invalid
		stateCall("lease.release", map[string]any{"queue": ""}),                                  // 1 done: leaves every queue
		stateCall("branch.rename", map[string]any{"repo": "api", "from": "main", "to": "other"}), // 1 invalid: protected
		stateCall("step.finish", map[string]any{"status": "bogus"}),                              // 1 invalid
		stateCall("step.finish", map[string]any{"status": "done"}),                               // 2 done
		stateCall("lease.release", map[string]any{"queue": ""}),                                  // 2 refused: finished
		stateCall("step.finish", map[string]any{"status": "failed", "reason": "late"}),           // 3 refused
	}
	f := newRunFixtureWith(t, []fakeagent.Action{{MCP: calls, Name: ""}}, agentStep("one", ""))
	app := f.app
	refsBefore := gitOut(t, f.repo.Dir, "branch", "--list", "--format=%(refname)")
	f.start(t, "feat/api-tools")
	run := waitRun(t, app, f.taskID, "one", "done")
	if o := run.Outcome; o == nil || o.Status != "done" || o.Source != "agent" {
		t.Fatalf("outcome = %+v, want done by the agent: a late failed must not change it", o)
	}

	for _, name := range []string{"mcp-repo.declare-1.json", "mcp-branch.request-1.json", "mcp-lease.acquire-1.json", "mcp-branch.rename-1.json", "mcp-step.finish-1.json"} {
		reply, errText := stateReply(t, app, name)
		if errText != "" {
			t.Errorf("%s: MCP error %q, want an invalid item: a bad call is never an error", name, errText)
			continue
		}
		it := replyItem(t, reply)
		if it["status"] != "invalid" || it["fix"] == nil || it["fix"] == "" {
			t.Errorf("%s item = %v, want invalid with a fix", name, it)
		}
	}
	if reply, _ := stateReply(t, app, "mcp-step.finish-1.json"); !strings.Contains(fmt.Sprint(replyItem(t, reply)["message"]), "must be one of") {
		t.Errorf("bogus status message = %v, want the allowed values", reply)
	}
	for _, name := range []string{"mcp-lease.release-2.json", "mcp-step.finish-3.json"} {
		if _, errText := stateReply(t, app, name); !strings.Contains(errText, "step already finished") {
			t.Errorf("%s = %q, want step already finished", name, errText)
		}
	}
	if reply, errText := stateReply(t, app, "mcp-lease.release-1.json"); errText != "" || replyItem(t, reply)["status"] != "done" {
		t.Errorf("lease.release-1 = %v %q, want done", reply, errText)
	}
	// The accepted finish keeps the reply text and the artifacts block as two content blocks.
	if raw, _ := os.ReadFile(filepath.Join(app.FakeDir, "mcp-step.finish-2.json")); !strings.Contains(string(raw), "step done") || !strings.Contains(string(raw), `\"artifacts\"`) {
		t.Errorf("step.finish-2 = %s, want step done and the artifacts block", raw)
	}
	// State unchanged: the only new ref is the run's own branch.
	refsAfter := gitOut(t, f.repo.Dir, "branch", "--list", "--format=%(refname)")
	for _, line := range strings.Split(refsAfter, "\n") {
		if !strings.Contains(refsBefore, line) && line != "refs/heads/feat/api-tools" {
			t.Errorf("unexpected branch ref %s created by refused tool calls", line)
		}
	}
	if got := branchOf(t, board(t, app), f.taskID, f.repoID); got.Name != "feat/api-tools" {
		t.Errorf("branch renamed to %q by a refused branch.rename", got.Name)
	}
}
