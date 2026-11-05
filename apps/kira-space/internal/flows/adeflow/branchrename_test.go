package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Branch rename. Contract ade-branch-rename: tests/ui/ade-v2-branch-rename.spec.ts reads these.

const renameScenario = "ade-branch-rename"

// noSandbox turns the sandbox off for the second step, so the run is not bound to the branch it renames.
const noSandbox = "    sandbox:\n      access: off\n"

func renameCall(from, to string, remote bool) fakeagent.MCPCall {
	return fakeagent.MCPCall{
		Server: agentmcp.ServerName, Tool: "rename_branch", Tolerate: true,
		Args: map[string]any{"repo": "api", "from": from, "to": to, "renameRemote": remote},
	}
}

// newRenameFixture sets up two agent runs: the first creates feat/old, the second asks to rename it.
// sandbox is extra YAML for the second step; a sandboxed run is bound to its branch and cannot rename it.
func newRenameFixture(t *testing.T, rename fakeagent.MCPCall, sandbox string) (*brFixture, *flowharness.Bare, *flowharness.Repo) {
	t.Helper()
	app := flowharness.New(t)
	apiRepo, bare := cloneWithMain(app, filepath.Join(app.Work, "api"))
	// The first run starts in the repo folder ("work"), the second in the new worktree ("api").
	claude(app, map[string][]fakeagent.Action{
		"work": {{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/old"})}, Name: "done"}},
		"api":  {{WaitBefore: filepath.Join(app.Work, "pushed"), MCP: []fakeagent.MCPCall{rename}, Name: "done"}},
	})
	api := importRepo(t, app, apiRepo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", onceStepYML+strings.ReplaceAll(onceStepYML, "one", "two")+sandbox))
	f := &brFixture{app: app, api: api.ID, task: createTask(t, app, "Fix login", "flow", api.ID)}
	return f, bare, apiRepo
}

// answerCreate runs the first agent run to its branch, pushes the branch with tracking when repo is given,
// then lets the second step run and returns its rename request.
func (f *brFixture) answerCreate(t *testing.T, repo *flowharness.Repo) adewire.BranchRequest {
	t.Helper()
	f.startRun(t)
	first := f.waitRequest(t)
	if _, err := f.app.W.AdeTask.ApproveBranchRequest(adewire.ApproveBranchRequestArgs{ID: first.ID, ItemIDs: []string{first.Items[0].ID}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, f.task.ID, "one", "done")
	if repo != nil {
		repo.Git("push", "-q", "-u", "origin", "feat/old")
	}
	if err := os.WriteFile(filepath.Join(f.app.Work, "pushed"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return f.waitRequest(t)
}

func (f *brFixture) renameResult(t *testing.T) string {
	t.Helper()
	var raw []byte
	testx.WaitUntil(t, waitFor, func() bool {
		var err error
		raw, err = os.ReadFile(filepath.Join(f.app.FakeDir, "mcp-rename_branch-1.json"))
		return err == nil
	})
	return string(raw)
}

func TestBranchRename(t *testing.T) {
	t.Run("approve renames branch, row and remote; worktree stays", func(t *testing.T) {
		f, bare, repo := newRenameFixture(t, renameCall("feat/old", "feat/new", true), noSandbox)
		req := f.answerCreate(t, repo)
		before := branchOf(t, board(t, f.app), f.task.ID, f.api).Worktree
		item := req.Items[0]
		if item.Kind != "rename" || item.From != "feat/old" || item.Branch != "feat/new" || item.Upstream != "origin/feat/old" {
			t.Fatalf("rename item = %+v, want feat/old to feat/new tracking origin/feat/old", item)
		}
		f.app.Contract(t, renameScenario, "AdeTaskService.BranchRequest", req, brMask)
		if _, err := f.app.W.AdeTask.ApproveBranchRequest(adewire.ApproveBranchRequestArgs{ID: req.ID, ItemIDs: []string{item.ID}, RenameRemote: true}); err != nil {
			t.Fatal(err)
		}
		got := f.renameResult(t)
		if !strings.Contains(got, `"outcome":"renamed"`) || !strings.Contains(got, `"remote":"renamed"`) {
			t.Fatalf("agent result = %s, want renamed on both sides", got)
		}
		row := branchOf(t, board(t, f.app), f.task.ID, f.api)
		if row.Name != "feat/new" || row.Worktree != before {
			t.Fatalf("row = %q at %q, want feat/new at %q", row.Name, row.Worktree, before)
		}
		heads := bare.Git("branch", "--list")
		if !strings.Contains(heads, "feat/new") || strings.Contains(heads, "feat/old") {
			t.Fatalf("remote branches = %q, want only feat/new", heads)
		}
		f.app.Contract(t, renameScenario, "AdeTaskBoard.Board.renamedBranch", row, brMask, flowharness.Mask("worktree", "addedAt", "tip", "base", "setup"))
	})

	t.Run("local rename leaves the remote branch and drops tracking", func(t *testing.T) {
		f, bare, repo := newRenameFixture(t, renameCall("feat/old", "feat/new", false), noSandbox)
		req := f.answerCreate(t, repo)
		if _, err := f.app.W.AdeTask.ApproveBranchRequest(adewire.ApproveBranchRequestArgs{ID: req.ID, ItemIDs: []string{req.Items[0].ID}}); err != nil {
			t.Fatal(err)
		}
		if got := f.renameResult(t); !strings.Contains(got, `"outcome":"renamed"`) || !strings.Contains(got, `"remote":"kept"`) {
			t.Fatalf("agent result = %s, want renamed locally and remote kept", got)
		}
		if heads := bare.Git("branch", "--list"); !strings.Contains(heads, "feat/old") || strings.Contains(heads, "feat/new") {
			t.Fatalf("remote branches = %q, want only feat/old", heads)
		}
		if out, err := repo.GitErr("config", "--get", "branch.feat/new.merge"); err == nil {
			t.Fatalf("feat/new still tracks %q, want no upstream", out)
		}
	})

	t.Run("deny keeps the name", func(t *testing.T) {
		f, _, _ := newRenameFixture(t, renameCall("feat/old", "feat/new", false), noSandbox)
		req := f.answerCreate(t, nil)
		if _, err := f.app.W.AdeTask.DenyBranchRequest(adewire.BranchRequestArgs{ID: req.ID}); err != nil {
			t.Fatal(err)
		}
		if got := f.renameResult(t); !strings.Contains(got, `"outcome":"denied"`) {
			t.Fatalf("agent result = %s, want denied", got)
		}
		if name := f.branchName(t, f.api); name != "feat/old" {
			t.Fatalf("branch = %q, want feat/old", name)
		}
	})

	t.Run("main is refused before any prompt", func(t *testing.T) {
		f, _, _ := newRenameFixture(t, renameCall("feat/old", "main", false), noSandbox)
		f.startRun(t)
		first := f.waitRequest(t)
		if _, err := f.app.W.AdeTask.ApproveBranchRequest(adewire.ApproveBranchRequestArgs{ID: first.ID, ItemIDs: []string{first.Items[0].ID}}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		if err := os.WriteFile(filepath.Join(f.app.Work, "pushed"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := f.renameResult(t); !strings.Contains(got, `"outcome":"failed"`) || !strings.Contains(got, "main branch") {
			t.Fatalf("agent result = %s, want the main-branch refusal", got)
		}
		if ps := f.requestPrompts(t); len(ps) != 0 {
			t.Fatalf("prompts = %+v, want none", ps)
		}
	})

	t.Run("a sandboxed run keeps its branch name", func(t *testing.T) {
		f, _, _ := newRenameFixture(t, renameCall("feat/old", "feat/new", false), "")
		f.startRun(t)
		first := f.waitRequest(t)
		if _, err := f.app.W.AdeTask.ApproveBranchRequest(adewire.ApproveBranchRequestArgs{ID: first.ID, ItemIDs: []string{first.Items[0].ID}}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		if err := os.WriteFile(filepath.Join(f.app.Work, "pushed"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := f.renameResult(t); !strings.Contains(got, `"outcome":"failed"`) || !strings.Contains(got, "sandbox") {
			t.Fatalf("agent result = %s, want the sandbox refusal", got)
		}
		if name := f.branchName(t, f.api); name != "feat/old" {
			t.Fatalf("branch = %q, want feat/old", name)
		}
	})
}
