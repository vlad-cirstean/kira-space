package adeflow_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

func toolCall(tool string, args map[string]any) fakeagent.MCPCall {
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: tool, Tolerate: true, Args: args}
}

// J-RUN-032, J-ADE-056, OQ-ADE-6. One run calls task tools with valid and invalid input interleaved.
// Invalid input is refused with a message and changes nothing; after the accepted finish_step every
// tool, finish_step included, answers "step already finished" and the outcome stays.
func TestAgentToolsInvalidInputAndAfterFinish(t *testing.T) {
	var branches []any
	for i := 0; i < 21; i++ {
		branches = append(branches, map[string]any{"repo": "api", "branch": fmt.Sprintf("feat/b%d", i)})
	}
	calls := []fakeagent.MCPCall{
		toolCall("task_info", nil), // 1 ok
		toolCall("declare_repos", map[string]any{"repos": []any{"no-such-repo"}}),     // 1 refused
		toolCall("request_branches", map[string]any{"branches": branches}),            // 1 refused (21)
		toolCall("acquire_lease", map[string]any{"queue": ""}),                        // 1 refused
		toolCall("release_lease", map[string]any{"queue": ""}),                        // 1 ok: leaves every queue
		toolCall("rename_branch", map[string]any{"name": "main"}),                     // 1 refused: protected
		toolCall("finish_step", map[string]any{"status": "bogus"}),                    // 1 refused
		toolCall("finish_step", map[string]any{"status": "done"}),                     // 2 ok
		toolCall("task_info", nil),                                                    // 2 refused: finished
		toolCall("release_lease", map[string]any{"queue": ""}),                        // 2 refused: finished
		toolCall("finish_step", map[string]any{"status": "failed", "reason": "late"}), // 3 refused
	}
	f := newRunFixtureWith(t, []fakeagent.Action{{MCP: calls, Name: ""}}, agentStep("one", ""))
	app := f.app
	refsBefore := gitOut(t, f.repo.Dir, "branch", "--list", "--format=%(refname)")
	f.start(t, "feat/api-tools")
	run := waitRun(t, app, f.taskID, "one", "done")
	if o := run.Outcome; o == nil || o.Status != "done" || o.Source != "agent" {
		t.Fatalf("outcome = %+v, want done by the agent: a late failed must not change it", o)
	}

	refused := map[string]string{
		"mcp-declare_repos-1.json":    "",
		"mcp-request_branches-1.json": "",
		"mcp-acquire_lease-1.json":    "",
		"mcp-rename_branch-1.json":    "",
		"mcp-finish_step-1.json":      "does not equal any of",
		"mcp-task_info-2.json":        "step already finished",
		"mcp-release_lease-2.json":    "step already finished",
		"mcp-finish_step-3.json":      "step already finished",
	}
	for name, want := range refused {
		text, _ := mcpReply(t, app, name)
		if text == "" {
			t.Errorf("%s: not an error reply", name)
		}
		if want != "" && !strings.Contains(text, want) {
			t.Errorf("%s = %q, want %q", name, text, want)
		}
	}
	if text, _ := mcpReply(t, app, "mcp-request_branches-1.json"); !strings.Contains(text, "20") {
		t.Errorf("21-branch request refusal = %q, want the limit named", text)
	}
	for _, ok := range []string{"mcp-task_info-1.json", "mcp-release_lease-1.json", "mcp-finish_step-2.json"} {
		raw := fakeFile(t, app, ok)
		if strings.Contains(raw, `"isError":true`) {
			t.Errorf("%s = %s, want success", ok, raw)
		}
	}
	// State unchanged: the only new ref is the run's own branch.
	refsAfter := gitOut(t, f.repo.Dir, "branch", "--list", "--format=%(refname)")
	for _, line := range strings.Split(refsAfter, "\n") {
		if !strings.Contains(refsBefore, line) && line != "refs/heads/feat/api-tools" {
			t.Errorf("unexpected branch ref %s created by refused tool calls", line)
		}
	}
	if got := branchOf(t, board(t, app), f.taskID, f.repoID); got.Name != "feat/api-tools" {
		t.Errorf("branch renamed to %q by a refused rename_branch", got.Name)
	}
}
