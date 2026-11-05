package claude

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// branchHold is how long the test leaves the request unanswered. It proves the real claude keeps an MCP
// call open past a 60 s limit, so a user who answers late still reaches the waiting agent.
const branchHold = 65 * time.Second

var nanoFolder = regexp.MustCompile(`^wt_[0-9a-hjkmnp-tv-z]{10}$`)

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

// TestAdeBranchRequest runs a headless step on a task with no branch. The agent must ask for one through
// request_branches, wait for the approval, then work in the returned worktree.
func TestAdeBranchRequest(t *testing.T) {
	f := newAdeFixture(t, nil)
	step := "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 6m\n" +
		"    allowed_tools: [\"Write\", \"Bash(git:*)\"]\n" +
		"    prompt: You have no branch yet. Call request_branches for the repo api with the branch name feat/hello and wait for the answer. " +
		"Then, in the worktree it returns, create the file hello.txt containing hi and commit it with git. " +
		"Then call finish_step with status done and summary ok.\n"
	f.saveWorkflow(t, "flow", step+manualStepYAML)
	task := f.createTask(t, "flow")
	if _, err := f.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	var req adewire.BranchRequest
	testx.WaitUntil(t, 3*time.Minute, func() bool {
		b, err := f.W.AdeTask.Board(ctx)
		if err != nil {
			return false
		}
		for _, tk := range b.Tasks {
			if tk.ID == task.ID && tk.BranchRequestID != "" {
				req, err = openBranchRequest(f, tk.BranchRequestID)
				return err == nil
			}
		}
		return false
	})
	asked := time.Now()
	if len(req.Items) != 1 || req.Items[0].Repo != "api" || req.Items[0].Kind != "branch" || req.Items[0].Branch != "feat/hello" {
		t.Fatalf("request = %+v, want one new branch in api", req)
	}
	time.Sleep(branchHold)
	opts, err := json.Marshal(map[string][]string{"itemIds": {req.Items[0].ID}})
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
	if !strings.Contains(log, "mcp__kira-ade__request_branches") {
		t.Fatalf("run log lacks the request_branches call:\n%s", tail(log, 2500))
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
	if br.Name != "feat/hello" || !nanoFolder.MatchString(filepath.Base(br.Worktree)) {
		t.Fatalf("branch = %+v, want feat/hello in a wt_ worktree folder", br)
	}
	if _, err := os.Stat(filepath.Join(br.Worktree, "hello.txt")); err != nil {
		t.Fatalf("hello.txt missing from the worktree: %v\nlog:\n%s", err, tail(log, 2500))
	}
	f.requireCostUnder(t)
}
