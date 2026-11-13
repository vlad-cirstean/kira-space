package adeflow_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Branch change. Contract ade-branch-change: tests/ui/ade-v2-branch-change.spec.ts reads these.

const changeScenario = "ade-branch-change"

func changeCall(from, to string) fakeagent.MCPCall {
	return fakeagent.MCPCall{
		Server: agentmcp.ServerName, Tool: "change_branch", Tolerate: true,
		Args: map[string]any{"repo": "api", "from": from, "to": to},
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newChangeFixture sets up two agent runs: the first creates feat/old, the second calls change_branch.
// It stops after the first run with the agent still waiting, so the test can move the worktree first.
func newChangeFixture(t *testing.T, from, to string) (*brFixture, *flowharness.Repo, string) {
	t.Helper()
	f, _, repo := newRenameFixture(t, changeCall(from, to), noSandbox)
	f.startRun(t)
	first := f.waitRequest(t)
	if _, err := approveBR(f.app, first.ID, []string{first.Items[0].ID}, nil); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, f.task.ID, "one", "done")
	own := branchOf(t, board(t, f.app), f.task.ID, f.api).Worktree
	if own == "" {
		t.Fatal("feat/old has no worktree after the first run")
	}
	return f, repo, own
}

// release lets the waiting agent call the tool.
func (f *brFixture) release(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.app.Work, "pushed"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *brFixture) changeResult(t *testing.T) string {
	t.Helper()
	var raw []byte
	testx.WaitUntil(t, waitFor, func() bool {
		var err error
		raw, err = os.ReadFile(filepath.Join(f.app.FakeDir, "mcp-change_branch-1.json"))
		return err == nil && len(raw) > 0 // the fake agent writes the file in place; skip a read of the empty file
	})
	return string(raw)
}

func TestBranchChange(t *testing.T) {
	t.Run("mismatch is reported and approve re-points the row", func(t *testing.T) {
		f, repo, own := newChangeFixture(t, "feat/old", "feat/other")
		repo.Git("branch", "feat/other")
		gitIn(t, own, "switch", "-q", "feat/other")
		// The out-of-band switch reaches the board through the repo watcher, not a board write.
		var before adewire.Branch
		testx.WaitUntil(t, waitFor, func() bool {
			before = branchOf(t, board(t, f.app), f.task.ID, f.api)
			return before.Worktree == "" && before.Detached != nil && before.Detached.Head == "feat/other"
		})
		f.app.Contract(t, changeScenario, "AdeTaskService.Board#mismatch", []adewire.Branch{before}, adoptMask)
		info, err := f.app.W.AdeBoard.BranchStatus(ctx, f.task.ID, "api", "feat/old", 0)
		if err != nil || !strings.Contains(info.Next, "change_branch") || !strings.Contains(info.Next, "feat/other") {
			t.Fatalf("branch status next = %q, %v; want a change_branch hint naming feat/other", info.Next, err)
		}
		task, err := f.app.W.AdeBoard.TaskInfo(ctx, f.task.ID, true)
		if err != nil || len(task.Repos) != 1 || task.Repos[0].Mismatch == nil || task.Repos[0].Mismatch.CheckedOut != "feat/other" {
			t.Fatalf("task info = %+v, %v; want a mismatch on feat/other", task, err)
		}

		f.release(t)
		req := f.waitRequest(t)
		item := req.Items[0]
		if item.Kind != "switch" || item.From != "feat/old" || item.Branch != "feat/other" {
			t.Fatalf("switch item = %+v, want feat/old to feat/other", item)
		}
		f.app.Contract(t, changeScenario, "ApprovalsService.List#branch", []approvals.Item{*branchItem(f.app, req.ID)}, brListMask)
		if _, err := approveBR(f.app, req.ID, []string{item.ID}, nil); err != nil {
			t.Fatal(err)
		}
		if got := f.changeResult(t); !strings.Contains(got, `"outcome":"changed"`) {
			t.Fatalf("agent result = %s, want changed", got)
		}
		row := branchOf(t, board(t, f.app), f.task.ID, f.api)
		if row.ID != before.ID || row.Name != "feat/other" || row.Worktree != own || row.Detached != nil {
			t.Fatalf("row = %q at %q detached %+v, want same id, feat/other at %q attached", row.Name, row.Worktree, row.Detached, own)
		}
		f.app.Contract(t, changeScenario, "AdeTaskService.Board#changed", []adewire.Branch{row}, adoptMask)
	})

	t.Run("approve switches a folder that holds the old branch", func(t *testing.T) {
		f, repo, own := newChangeFixture(t, "feat/old", "feat/b")
		repo.Git("branch", "feat/b")
		f.release(t)
		req := f.waitRequest(t)
		mustApprove(t, f.app, req.ID, []string{req.Items[0].ID}, nil)
		if got := f.changeResult(t); !strings.Contains(got, `"outcome":"changed"`) {
			t.Fatalf("agent result = %s, want changed", got)
		}
		if head := gitIn(t, own, "symbolic-ref", "HEAD"); head != "refs/heads/feat/b" {
			t.Fatalf("folder HEAD = %q, want refs/heads/feat/b", head)
		}
		if name := f.branchName(t, f.api); name != "feat/b" {
			t.Fatalf("branch = %q, want feat/b", name)
		}
	})

	t.Run("deny keeps row and folder", func(t *testing.T) {
		f, repo, own := newChangeFixture(t, "feat/old", "feat/b")
		repo.Git("branch", "feat/b")
		f.release(t)
		req := f.waitRequest(t)
		mustDeny(t, f.app, req.ID)
		if got := f.changeResult(t); !strings.Contains(got, `"outcome":"denied"`) {
			t.Fatalf("agent result = %s, want denied", got)
		}
		if head := gitIn(t, own, "symbolic-ref", "HEAD"); head != "refs/heads/feat/old" {
			t.Fatalf("folder HEAD = %q, want refs/heads/feat/old", head)
		}
		if name := f.branchName(t, f.api); name != "feat/old" {
			t.Fatalf("branch = %q, want feat/old", name)
		}
	})

	refusals := []struct {
		name, to, want string
		setup          func(t *testing.T, repo *flowharness.Repo, own string)
	}{
		{"dirty folder that needs a switch", "feat/b", "uncommitted", func(t *testing.T, repo *flowharness.Repo, own string) {
			repo.Git("branch", "feat/b")
			if err := os.WriteFile(filepath.Join(own, "base.txt"), []byte("changed\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"branch checked out in the main checkout", "feat/held", "checked out in", func(t *testing.T, repo *flowharness.Repo, own string) {
			repo.Git("checkout", "-q", "-b", "feat/held")
		}},
		{"branch missing locally", "feat/nope", "does not exist locally", func(*testing.T, *flowharness.Repo, string) {}},
	}
	for _, tc := range refusals {
		t.Run("refused before any prompt: "+tc.name, func(t *testing.T) {
			f, repo, own := newChangeFixture(t, "feat/old", tc.to)
			tc.setup(t, repo, own)
			f.release(t)
			if got := f.changeResult(t); !strings.Contains(got, `"outcome":"failed"`) || !strings.Contains(got, tc.want) {
				t.Fatalf("agent result = %s, want a refusal naming %q", got, tc.want)
			}
			if ps := f.requestPrompts(t); len(ps) != 0 {
				t.Fatalf("prompts = %+v, want none", ps)
			}
			if name := f.branchName(t, f.api); name != "feat/old" {
				t.Fatalf("branch = %q, want feat/old", name)
			}
		})
	}

	for _, tc := range []struct {
		name string
		bind bool // the session row carries the branch id; otherwise it is task-level (step, review agent)
	}{{"branch session", true}, {"task-level session", false}} {
		t.Run("refused while another agent session is open: "+tc.name, func(t *testing.T) {
			f, repo, _ := newChangeFixture(t, "feat/old", "feat/b")
			repo.Git("branch", "feat/b")
			row := branchOf(t, board(t, f.app), f.task.ID, f.api)
			branchID := ""
			if tc.bind {
				branchID = row.ID
			}
			if _, err := f.app.DB().Exec(`INSERT INTO ade_sessions (id, task_id, mode, state, started_at, last_active_at, terminal_id, branch_id, agent_session_id, cwd)
				VALUES ('other-tui', ?, 'tui', 'running', 1, 1, 'other-term', ?, 'agent-1', ?)`, f.task.ID, branchID, row.Worktree); err != nil {
				t.Fatal(err)
			}
			f.release(t)
			if got := f.changeResult(t); !strings.Contains(got, `"outcome":"failed"`) || !strings.Contains(got, "another agent session is open") {
				t.Fatalf("agent result = %s, want a refusal naming the other session", got)
			}
			if ps := f.requestPrompts(t); len(ps) != 0 {
				t.Fatalf("prompts = %+v, want none", ps)
			}
			if name := f.branchName(t, f.api); name != "feat/old" {
				t.Fatalf("branch = %q, want feat/old", name)
			}
		})
	}
}
