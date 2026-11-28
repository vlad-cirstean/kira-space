package adeflow_test

import (
	"os"
	"testing"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// An agent names one of several branches of one repo (P762). Contract ade-agent-multi-worktree:
// tests/ui/ade-v2-agent-multi-worktree.spec.ts reads these.

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
}
