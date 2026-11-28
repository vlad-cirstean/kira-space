package adeflow_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// An agent names one of several branches of one repo (P762). Contract ade-agent-multi-worktree:
// tests/ui/ade-v2-agent-multi-worktree.spec.ts reads these.

const multiScenario = "ade-agent-multi-worktree"

// sortedRows are the task's rows by name, the order the contract stores.
func (f *brFixture) sortedRows(t *testing.T) []adewire.Branch {
	t.Helper()
	rows := branchesOf(t, f.app, f.task.ID)
	slices.SortFunc(rows, func(a, b adewire.Branch) int { return strings.Compare(a.Name, b.Name) })
	return rows
}

func (f *brFixture) removeBy(args agentmcp.RemoveWorktreeArgs) (agentmcp.RemoveWorktreeResult, error) {
	return f.app.W.AdeBoard.RemoveWorktree(ctx, agentmcp.SpaceCall{TaskID: f.task.ID}, args)
}

func TestAgentMultiWorktree(t *testing.T) {
	t.Run("repo and branch resolve with two branches in one repo", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		one := f.agentCreate(t, "api", "feat/one")
		two := f.agentCreate(t, "api", "feat/two")
		res, err := f.app.W.AdeBoard.RemoveWorktree(ctx, agentmcp.SpaceCall{TaskID: f.task.ID}, agentmcp.RemoveWorktreeArgs{Repo: "api", Branch: "feat/two"})
		if err != nil || res.Outcome != agentmcp.BranchRemoved {
			t.Fatalf("RemoveWorktree = %+v, %v; want removed", res, err)
		}
		gone(t, two.Worktree)
		if _, err := os.Stat(one.Worktree); err != nil {
			t.Fatalf("feat/one lost its worktree: %v", err)
		}
		var names []string
		for _, r := range branchesOf(t, f.app, f.task.ID) {
			if r.Name != "" {
				names = append(names, r.Name)
			}
		}
		if len(names) != 1 || names[0] != "feat/one" {
			t.Fatalf("named rows = %v, want [feat/one]", names)
		}
	})

	t.Run("worktree alone picks the row", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		one := f.agentCreate(t, "api", "feat/one")
		two := f.agentCreate(t, "api", "feat/two")
		f.app.Contract(t, multiScenario, "AdeTaskService.Board#before", f.sortedRows(t), rowMask)
		res, err := f.removeBy(agentmcp.RemoveWorktreeArgs{Worktree: two.Worktree})
		if err != nil || res.Outcome != agentmcp.BranchRemoved || res.Repo != "api" || res.Branch != "feat/two" || res.Worktree != two.Worktree {
			t.Fatalf("RemoveWorktree = %+v, %v; want feat/two of api removed at %s", res, err, two.Worktree)
		}
		gone(t, two.Worktree)
		if _, err := os.Stat(one.Worktree); err != nil {
			t.Fatalf("feat/one lost its worktree: %v", err)
		}
		f.app.Contract(t, multiScenario, "AdeTaskService.Board#afterRemove", f.sortedRows(t), rowMask)
		op, ok := findOp(opsNow(t, f.app), oplog.CategoryAgent, "agentWorktreeRemove")
		if !ok || op.Status != oplog.StatusOK || op.Command == nil || *op.Command != "api/feat/two" {
			t.Fatalf("remove op = %+v, want ok for api/feat/two", op)
		}
	})

	t.Run("worktree with a disagreeing branch or repo is an error", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		one := f.agentCreate(t, "api", "feat/one")
		two := f.agentCreate(t, "api", "feat/two")
		for _, c := range []struct {
			args agentmcp.RemoveWorktreeArgs
			want string
		}{
			{agentmcp.RemoveWorktreeArgs{Worktree: one.Worktree, Branch: "feat/two"}, "holds branch feat/one, not feat/two"},
			{agentmcp.RemoveWorktreeArgs{Worktree: one.Worktree, Repo: "web"}, "belongs to api, not web"},
			{agentmcp.RemoveWorktreeArgs{Worktree: "/nowhere"}, "is not a worktree of this task"},
		} {
			if _, err := f.removeBy(c.args); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("RemoveWorktree %+v error = %v, want one holding %q", c.args, err, c.want)
			}
		}
		for _, p := range []string{one.Worktree, two.Worktree} {
			if _, err := os.Stat(p); err != nil {
				t.Fatalf("a refused remove lost %s: %v", p, err)
			}
		}
	})

	t.Run("rename and change resolve in a two-branch repo", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		f.agentCreate(t, "api", "feat/one")
		two := f.agentCreate(t, "api", "feat/two")
		call := agentmcp.SpaceCall{TaskID: f.task.ID}
		for _, args := range []agentmcp.RenameBranchArgs{
			{Repo: "api", From: "feat/two", To: "main"},
			{Worktree: two.Worktree, From: "feat/two", To: "main"},
		} {
			res, err := f.app.W.AdeBoard.RenameBranch(ctx, call, args)
			if err != nil || res.Outcome != agentmcp.BranchFailed || strings.Contains(res.Next, "matches several repos") || !strings.Contains(res.Next, "main") {
				t.Fatalf("RenameBranch %+v = %+v, %v; want the reserved-name refusal", args, res, err)
			}
		}
		res, err := f.app.W.AdeBoard.ChangeBranch(ctx, call, agentmcp.ChangeBranchArgs{Worktree: two.Worktree, From: "feat/two", To: "no/such"})
		if err != nil || res.Outcome != agentmcp.BranchFailed || !strings.Contains(res.Next, "does not exist locally") {
			t.Fatalf("ChangeBranch = %+v, %v; want the missing-local-branch refusal", res, err)
		}
		f.noApproval(t)
	})

	t.Run("the tool path", func(t *testing.T) {
		f := newRemoveFixture(t, []fakeagent.Action{{Name: "done"}})
		f.agentCreate(t, "api", "feat/one")
		two := f.agentCreate(t, "api", "feat/two")
		// The path is known only now: register the run's actions again.
		claude(f.app, map[string][]fakeagent.Action{"*": {{MCP: []fakeagent.MCPCall{stateCall("worktree.remove", map[string]any{"worktree": two.Worktree})}, Name: "done"}}})
		f.startRun(t)
		waitRun(t, f.app, f.task.ID, "one", "done")
		if got := f.resultOf(t, "worktree.remove", 1); !strings.Contains(got, `"status":"done"`) {
			t.Fatalf("agent result = %s, want done", got)
		}
		gone(t, two.Worktree)
	})
}
