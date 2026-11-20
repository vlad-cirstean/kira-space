package ade

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

func toolErrText(t *testing.T, err error) string {
	t.Helper()
	var te agentmcp.ToolError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v; want an agentmcp.ToolError", err)
	}
	return te.Error()
}

func TestSpaceTools_declareIsAdditiveAndIdempotent(t *testing.T) {
	e := newEngine(t, nil)
	e.repo("api")
	e.repo("web")
	e.workflow(flowYAML(agentStep("first", "")))
	id := e.task("api")
	ctx := context.Background()

	info, err := e.board.DeclareRepos(ctx, id, []string{"web", "api"})
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Repos) != 2 {
		t.Fatalf("repos = %+v; want api and web", info.Repos)
	}
	if _, err := e.board.DeclareRepos(ctx, id, []string{"web"}); err != nil {
		t.Fatalf("second declare: %v", err)
	}
	branches, _ := e.repos.AdeTasks.BranchesLive()
	var agent int
	for _, b := range branches {
		if b.TaskID == id && b.Origin == model.AdeBranchOriginAgent {
			agent++
		}
	}
	if agent != 1 {
		t.Fatalf("%d agent-origin branches; want 1 (web only)", agent)
	}
	_, err = e.board.DeclareRepos(ctx, id, []string{"nope"})
	if msg := toolErrText(t, err); !strings.Contains(msg, "Agents > Repositories") || !strings.Contains(msg, "api") {
		t.Fatalf("unknown repo message = %q", msg)
	}
}

// requestBranch asks for one named branch and returns the result: a new branch needs no approval.
func requestBranch(ctx context.Context, b *TaskBoard, taskID, repo, branch string) (agentmcp.BranchInfo, error) {
	rb, err := requestBranchResult(ctx, b, taskID, repo, branch)
	return rb.BranchInfo, err
}

// requestBranchResult is requestBranch with the outcome.
func requestBranchResult(ctx context.Context, b *TaskBoard, taskID, repo, branch string) (agentmcp.RequestedBranch, error) {
	res, err := b.RequestBranches(ctx, agentmcp.SpaceCall{TaskID: taskID}, []agentmcp.BranchSpec{{Repo: repo, Branch: branch}})
	if err != nil {
		return agentmcp.RequestedBranch{}, err
	}
	if len(res.Branches) != 1 || res.Branches[0].Outcome == agentmcp.BranchFailed {
		return agentmcp.RequestedBranch{}, errors.New("request failed: " + res.Branches[0].Next)
	}
	return res.Branches[0], nil
}

func TestSpaceTools_requestBranch(t *testing.T) {
	e := newEngine(t, nil)
	e.repo("api")
	e.repo("web")
	e.prepare("api", "echo ok")
	e.workflow(flowYAML(agentStep("first", "")))
	id := e.task("api")
	other := e.task("web")
	ctx := context.Background()

	info, err := requestBranch(ctx, e.board, id, "api", "feat/api-one")
	if err != nil {
		t.Fatal(err)
	}
	if info.Branch != "feat/api-one" || info.Worktree == "" || info.Setup.State == "none" {
		t.Fatalf("info = %+v", info)
	}
	sb := e.branch(t, id, "api")
	if sb.Origin != model.AdeBranchOriginAgent || sb.Name != info.Branch {
		t.Fatalf("stored branch = %+v", sb)
	}
	waitUntil(t, "setup ready", func() bool {
		s, _ := e.repos.AdeTasks.GetSetup(sb.ID)
		return s != nil && s.State == model.AdeSetupReady
	})

	again, err := requestBranch(ctx, e.board, id, "api", "feat/api-one")
	if err != nil || again.Worktree != info.Worktree || again.Branch != info.Branch {
		t.Fatalf("idempotent re-request: %+v, %v", again, err)
	}
	second, err := requestBranch(ctx, e.board, other, "web", "feat/web-one")
	if err != nil || second.Branch != "feat/web-one" {
		t.Fatalf("second task branch = %+v, %v; want the asked name", second, err)
	}
}

// A branch.request whose setup outlasts the call wait answers waiting and leaves the setup running;
// a later request sees it done. An archived task refuses new branches.
func TestSpaceTools_setupWaitAndArchived(t *testing.T) {
	e := newEngine(t, nil)
	e.repo("api")
	gate := t.TempDir() + "/gate"
	e.prepare("api", "while [ ! -f "+gate+" ]; do sleep 0.05; done")
	e.workflow(flowYAML(agentStep("first", "")))
	id := e.task("api")
	ctx := context.Background()
	e.board.deps.CallWait = 500 * time.Millisecond

	rb, err := requestBranchResult(ctx, e.board, id, "api", "feat/api-one")
	if err != nil {
		t.Fatal(err)
	}
	if rb.Outcome != agentmcp.BranchWaiting || rb.Setup.State != model.AdeSetupRunning {
		t.Fatalf("request during setup = %+v; want waiting with the setup running", rb)
	}
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	sb := e.branch(t, id, "api")
	waitUntil(t, "setup ready", func() bool {
		s, _ := e.repos.AdeTasks.GetSetup(sb.ID)
		return s != nil && s.State == model.AdeSetupReady
	})
	rb, err = requestBranchResult(ctx, e.board, id, "api", "feat/api-one")
	if err != nil || rb.Setup.State != model.AdeSetupReady || rb.Outcome == agentmcp.BranchWaiting {
		t.Fatalf("request after setup = %+v, %v; want ready", rb, err)
	}

	if err := e.board.ArchiveTask(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := requestBranch(ctx, e.board, id, "api", "feat/api-two"); err == nil {
		t.Fatal("archived task accepted a branch")
	}
}

func (e *engine) branch(t *testing.T, taskID, repo string) model.AdeTaskBranch {
	t.Helper()
	sb, err := e.repos.AdeTasks.GetBranch(branchOf(t, e, taskID, repo))
	if err != nil {
		t.Fatal(err)
	}
	return sb
}

func TestSpaceTools_startRunCreatesNoBranch(t *testing.T) {
	e := newEngine(t, map[string][]string{"*": {"done"}})
	e.repo("api")
	e.workflow("id: flow\nname: Flow\nsteps:\n" + agentStep("first", ""))
	id := e.task("api")
	if _, err := e.board.StartRun(context.Background(), adewire.StartRunArgs{TaskID: id}); err != nil {
		t.Fatal(err)
	}
	e.waitRun(id, "first/api", model.AdeRunDone)
	if sb := e.branch(t, id, "api"); sb.Name != "" {
		t.Fatalf("StartRun created branch %q; want none", sb.Name)
	}
	info, err := requestBranch(context.Background(), e.board, id, "api", "feat/api-one")
	if err != nil || info.Branch != "feat/api-one" {
		t.Fatalf("branch.request after start: %+v, %v", info, err)
	}
}
