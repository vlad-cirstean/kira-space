package claude

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// branchHold is how long the test leaves the rename unanswered. It proves the real claude keeps an MCP
// call open past a 60 s limit, so a user who answers late still reaches the waiting agent.
const branchHold = 65 * time.Second

// openBranchRequest reads the task's open request from its approvals item.
func openBranchRequest(f *adeFixture, id string) (adewire.BranchRequest, error) {
	items, err := f.W.ApprovalsSvc.List()
	if err != nil {
		return adewire.BranchRequest{}, err
	}
	for _, it := range items {
		if it.ID != "branch.request:"+id {
			continue
		}
		req := adewire.BranchRequest{ID: id}
		err := json.Unmarshal(it.Payload, &req)
		return req, err
	}
	return adewire.BranchRequest{}, errors.New("branch request is not open")
}

// TestAdeBranchRequest runs a headless step on a task with no branch. The agent creates feat/hello on a
// chosen base with no approval, renames it (the user approves after a hold), then tries to remove its own
// worktree, which Kira refuses while the run holds it.
func TestAdeBranchRequest(t *testing.T) {
	f := newAdeFixture(t, nil)
	// feat/base exists only on origin, one commit ahead of main: the agent starts its branch there.
	f.repo.Git("checkout", "-q", "-b", "feat/base", "main")
	f.repo.Commit("base marker", map[string]string{"base-marker.txt": "marker\n"})
	f.repo.Git("push", "-q", "origin", "feat/base")
	f.repo.Checkout("main")
	f.repo.Git("branch", "-D", "feat/base")
	step := "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 8m\n" +
		"    allowed_tools: [\"Write\", \"Bash(git:*)\"]\n" +
		"    prompt: You have no branch yet. Call state with action branch.request and args repo api, branch feat/hello and base feat/base. It creates the branch at once. " +
		"In the worktree it returns, create the file hello.txt containing hi and commit it with git. " +
		"Then call state with action branch.rename and args repo api, from feat/hello and to feat/greet, and wait for the answer. " +
		"Then call state with action worktree.remove and args repo api and branch feat/greet. Kira refuses it because your run holds the branch, so do not retry. " +
		"Then call state with action step.finish and args status done.\n"
	f.saveWorkflow(t, "flow", step+manualStepYAML)
	task := f.createTask(t, "flow")
	if _, err := f.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Only the rename asks: the create must never open an approvals item.
	var req adewire.BranchRequest
	var rename adewire.BranchRequestItem
	testx.WaitUntil(t, 3*time.Minute, func() bool {
		b, err := f.W.AdeTask.Board(ctx)
		if err != nil {
			return false
		}
		for _, tk := range b.Tasks {
			if tk.ID != task.ID || tk.BranchRequestID == "" {
				continue
			}
			r, err := openBranchRequest(f, tk.BranchRequestID)
			if err != nil {
				return false
			}
			for _, it := range r.Items {
				if it.Kind == "rename" {
					req, rename = r, it
					return true
				}
			}
			t.Fatalf("open request holds no rename: %+v", r)
		}
		return false
	})
	asked := time.Now()
	if len(req.Items) != 1 || rename.Repo != "api" {
		t.Fatalf("request = %+v, want one rename in api", req)
	}
	time.Sleep(branchHold)
	opts, err := json.Marshal(map[string][]string{"itemIds": {rename.ID}})
	if err != nil {
		t.Fatal(err)
	}
	id := "branch.request:" + req.ID
	res, err := f.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{id}, Approve: true, Options: map[string]json.RawMessage{id: opts}})
	if err != nil || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("Approve after %s = %+v, %v; the CLI dropped the call before the answer", time.Since(asked).Round(time.Second), res, err)
	}

	var run adewire.Run
	testx.WaitUntil(t, 4*time.Minute, func() bool {
		b, err := f.W.AdeTask.Board(ctx)
		if err != nil {
			return false
		}
		for _, tk := range b.Tasks {
			for _, r := range tk.Runs {
				if tk.ID == task.ID && r.StepID == "one" && r.State != "running" && r.State != "queued" {
					run = r
					return true
				}
			}
		}
		return false
	})
	log := f.runLog(t, run.ID)
	if run.State != "done" {
		t.Fatalf("run = %+v, want done; log:\n%s", run, tail(log, 2500))
	}
	if !strings.Contains(log, "mcp__kira-space__state") {
		t.Fatalf("run log lacks the state call:\n%s", tail(log, 2500))
	}

	// The agent cannot remove the worktree its own run holds: it stays on disk, the branch is renamed.
	if !strings.Contains(log, "background run is working on feat/greet") {
		t.Fatalf("log lacks the worktree.remove refusal:\n%s", tail(log, 2500))
	}
	b, err := f.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var br adewire.Branch
	for _, x := range b.Branches {
		if x.TaskID == task.ID && x.CodeRepoID == f.repoID {
			br = x
		}
	}
	if br.Name != "feat/greet" || br.Base != "feat/base" {
		t.Fatalf("branch = %+v, want feat/greet on feat/base", br)
	}
	if _, err := os.Stat(filepath.Join(br.Worktree, "hello.txt")); err != nil {
		t.Fatalf("worktree lost its hello.txt: %v\nlog:\n%s", err, tail(log, 2500))
	}
	out, err := exec.Command("git", "-C", f.repo.Dir, "show", "feat/greet:hello.txt").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "hi" {
		t.Fatalf("git show feat/greet:hello.txt = %q, %v; the branch or its commit is gone\nlog:\n%s", out, err, tail(log, 2500))
	}
	if err := exec.Command("git", "-C", f.repo.Dir, "show", "feat/greet:base-marker.txt").Run(); err != nil {
		t.Fatalf("feat/greet does not descend from feat/base: %v", err)
	}
	f.requireCostUnder(t)
}
