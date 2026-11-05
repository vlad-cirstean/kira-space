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
	if msg := toolErrText(t, err); !strings.Contains(msg, "Manage repositories") || !strings.Contains(msg, "api") {
		t.Fatalf("unknown repo message = %q", msg)
	}
}

// requestBranch asks for one named branch, approves the request when one opens, and returns the result.
func requestBranch(ctx context.Context, b *TaskBoard, taskID, repo, branch string) (agentmcp.BranchInfo, error) {
	type answer struct {
		res agentmcp.RequestBranchesResult
		err error
	}
	done := make(chan answer, 1)
	go func() {
		res, err := b.RequestBranches(ctx, agentmcp.SpaceCall{TaskID: taskID}, []agentmcp.BranchSpec{{Repo: repo, Branch: branch}})
		done <- answer{res, err}
	}()
	for {
		select {
		case a := <-done:
			if a.err != nil {
				return agentmcp.BranchInfo{}, a.err
			}
			if len(a.res.Branches) != 1 || a.res.Branches[0].Outcome == agentmcp.BranchFailed {
				return agentmcp.BranchInfo{}, errors.New("request failed: " + a.res.Branches[0].Next)
			}
			return a.res.Branches[0].BranchInfo, nil
		case <-time.After(10 * time.Millisecond):
			if id := b.branchRequestOf(taskID); id != "" {
				if req, err := b.BranchRequest(id); err == nil {
					ids := make([]string, len(req.Items))
					for i, it := range req.Items {
						ids[i] = it.ID
					}
					_, _ = b.ApproveBranchRequest(id, ids, nil)
				}
			}
		}
	}
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

func TestSpaceTools_archivedAndBranchStatus(t *testing.T) {
	e := newEngine(t, nil)
	e.repo("api")
	gate := t.TempDir() + "/gate"
	e.prepare("api", "while [ ! -f "+gate+" ]; do sleep 0.05; done")
	e.workflow(flowYAML(agentStep("first", "")))
	id := e.task("api")
	ctx := context.Background()

	if _, err := e.board.BranchStatus(ctx, id, "api", "feat/api-one", 0); err == nil || !strings.Contains(toolErrText(t, err), "request_branches") {
		t.Fatalf("status before branch: %v", err)
	}
	if _, err := requestBranch(ctx, e.board, id, "api", "feat/api-one"); err != nil {
		t.Fatal(err)
	}
	st, err := e.board.BranchStatus(ctx, id, "api", "feat/api-one", 0)
	if err != nil || st.Setup.State != model.AdeSetupRunning {
		t.Fatalf("status during setup = %+v, %v", st, err)
	}
	cctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	if _, err := e.board.BranchStatus(cctx, id, "api", "feat/api-one", time.Minute); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a cancelled wait returned %v", err)
	}
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err = e.board.BranchStatus(ctx, id, "api", "feat/api-one", 30*time.Second)
	if err != nil || st.Setup.State != model.AdeSetupReady {
		t.Fatalf("status after the wait = %+v, %v", st, err)
	}

	if err := e.board.ArchiveTask(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.board.TaskInfo(ctx, id); err == nil || !strings.Contains(toolErrText(t, err), "archived") {
		t.Fatalf("TaskInfo of an archived task: %v", err)
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
		t.Fatalf("request_branches after start: %+v, %v", info, err)
	}
}
