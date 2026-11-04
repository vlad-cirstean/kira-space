package adeflow_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P408 branch requests. Contract ade-branch-request: tests/ui/ade-v2-branch-request.spec.ts reads these.

const brScenario = "ade-branch-request"

var (
	brMask      = flowharness.Mask("id", "taskId", "createdAt", "expiresAt", "codeRepoId")
	nanoFolder  = regexp.MustCompile(`^wt_[0-9a-hjkmnp-tv-z]{10}$`)
	onceStepYML = "  - id: one\n    name: one\n    kind: agent\n    runs_on: once\n    timeout: 1m\n    prompt: ask for branches\n"
)

type brFixture struct {
	app  *flowharness.App
	api  string
	web  string
	task adewire.Task
}

// newBRFixture registers api and web and makes a task on both (or api alone).
func newBRFixture(t *testing.T, actions []fakeagent.Action, steps string, both bool, opts ...flowharness.Opt) *brFixture {
	t.Helper()
	app := flowharness.New(t, opts...)
	claude(app, map[string][]fakeagent.Action{"*": actions})
	apiRepo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	webRepo, _ := cloneWithMain(app, filepath.Join(app.Work, "web"))
	api, web := importRepo(t, app, apiRepo.Dir), importRepo(t, app, webRepo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", steps))
	ids := []string{api.ID}
	if both {
		ids = append(ids, web.ID)
	}
	return &brFixture{app: app, api: api.ID, web: web.ID, task: createTask(t, app, "Fix login", "flow", ids...)}
}

func (f *brFixture) startRun(t *testing.T) {
	t.Helper()
	if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
}

// waitRequest waits for the task's open request and returns it.
func (f *brFixture) waitRequest(t *testing.T) adewire.BranchRequest {
	t.Helper()
	var id string
	testx.WaitUntil(t, waitFor, func() bool {
		id = taskOf(t, board(t, f.app), f.task.ID).BranchRequestID
		return id != ""
	})
	req, err := f.app.W.AdeTask.BranchRequest(adewire.BranchRequestArgs{ID: id})
	if err != nil {
		t.Fatalf("BranchRequest: %v", err)
	}
	return req
}

func (f *brFixture) requestPrompts(t *testing.T) []prompts.Routed {
	t.Helper()
	all, err := f.app.W.PromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	var out []prompts.Routed
	for _, p := range all {
		if p.Kind == prompts.KindBranchRequest {
			out = append(out, p)
		}
	}
	return out
}

func (f *brFixture) result(t *testing.T, n int) string {
	t.Helper()
	var raw []byte
	testx.WaitUntil(t, waitFor, func() bool {
		var err error
		raw, err = os.ReadFile(filepath.Join(f.app.FakeDir, "mcp-request_branches-"+strconv.Itoa(n)+".json"))
		return err == nil
	})
	return string(raw)
}

// requestCall is a request_branches call for {repo, branch} pairs.
func requestCall(branches ...[2]string) fakeagent.MCPCall {
	list := make([]any, len(branches))
	for i, b := range branches {
		list[i] = map[string]any{"repo": b[0], "branch": b[1]}
	}
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "request_branches", Tolerate: true, Args: map[string]any{"branches": list}}
}

func (f *brFixture) branchName(t *testing.T, repoID string) string {
	t.Helper()
	return branchOf(t, board(t, f.app), f.task.ID, repoID).Name
}

func TestBranchRequest(t *testing.T) {
	t.Run("approve one repo, deny the other", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/login"}, [2]string{"web", "feat/login-web"})}, Name: "done"}}, onceStepYML, true)
		f.startRun(t)
		req := f.waitRequest(t)
		if len(req.Items) != 2 || req.Source != "run" || req.Items[0].Kind != "branch" || req.TaskTitle == "" {
			t.Fatalf("request = %+v, want two new-branch items from a run", req)
		}
		f.app.Contract(t, brScenario, "AdeTaskService.BranchRequest", req, brMask)
		if ps := f.requestPrompts(t); len(ps) != 1 || ps[0].Ref != req.ID {
			t.Fatalf("prompts = %+v, want one for the request", ps)
		}
		run, _ := latestRun(t, f.app, f.task.ID, "one")
		if run.Note != "waiting for branch approval" {
			t.Fatalf("run note = %q, want the approval note while it waits", run.Note)
		}

		ans, err := f.app.W.AdeTask.ApproveBranchRequest(adewire.ApproveBranchRequestArgs{ID: req.ID, ItemIDs: []string{req.Items[0].ID}})
		if err != nil || !ans.Resolved {
			t.Fatalf("Approve = %+v, %v", ans, err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		got := f.result(t, 1)
		if !strings.Contains(got, "created") || !strings.Contains(got, "denied") {
			t.Fatalf("agent result = %s, want api created and web denied", got)
		}
		if name := f.branchName(t, f.api); name != "feat/login" {
			t.Fatalf("api branch = %q, want the asked name", name)
		}
		if name := f.branchName(t, f.web); name != "" {
			t.Fatalf("web branch = %q, want none", name)
		}
		wt := branchOf(t, board(t, f.app), f.task.ID, f.api).Worktree
		if !nanoFolder.MatchString(filepath.Base(wt)) || filepath.Base(filepath.Dir(wt)) != "api" {
			t.Fatalf("api worktree = %q, want wt/api/<nanoid>", wt)
		}
		if ps := f.requestPrompts(t); len(ps) != 0 {
			t.Fatalf("prompts after answer = %+v", ps)
		}
		if run, _ = latestRun(t, f.app, f.task.ID, "one"); run.Note != "" {
			t.Fatalf("run note after answer = %q, want it cleared", run.Note)
		}
		if _, err := f.app.W.AdeTask.BranchRequest(adewire.BranchRequestArgs{ID: req.ID}); errCode(err) != "E_NOT_FOUND" {
			t.Fatalf("BranchRequest after answer: %v, want E_NOT_FOUND", err)
		}
		if ans, err := f.app.W.AdeTask.DenyBranchRequest(adewire.BranchRequestArgs{ID: req.ID}); err != nil || ans.Resolved {
			t.Fatalf("Deny after answer = %+v, %v, want resolved=false", ans, err)
		}
	})

	t.Run("unregistered repo is refused before any prompt", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"nope", "feat/x"})}, Name: "done"}}, onceStepYML, false)
		f.startRun(t)
		waitRun(t, f.app, f.task.ID, "one", "done")
		if got := f.result(t, 1); !strings.Contains(got, "not registered") {
			t.Fatalf("agent result = %s, want the not-registered refusal", got)
		}
		if ps := f.requestPrompts(t); len(ps) != 0 {
			t.Fatalf("prompts = %+v, want none", ps)
		}
	})

	t.Run("registered repo off the task is added on approval", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"web", "feat/web-x"})}, Name: "done"}}, onceStepYML, false)
		f.startRun(t)
		req := f.waitRequest(t)
		if len(req.Items) != 1 || req.Items[0].CodeRepoID != f.web || req.Items[0].Kind != "branch" {
			t.Fatalf("request = %+v, want a new-branch item for web", req)
		}
		if n := len(taskOf(t, board(t, f.app), f.task.ID).BranchIDs); n != 1 {
			t.Fatalf("branch rows before approval = %d, want 1", n)
		}
		if _, err := f.app.W.AdeTask.ApproveBranchRequest(adewire.ApproveBranchRequestArgs{ID: req.ID, ItemIDs: []string{req.Items[0].ID}}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		b := board(t, f.app)
		if n := len(taskOf(t, b, f.task.ID).BranchIDs); n != 2 {
			t.Fatalf("branch rows after approval = %d, want 2", n)
		}
		row := branchOf(t, b, f.task.ID, f.web)
		if row.Origin != "agent" || row.Name != "feat/web-x" || !nanoFolder.MatchString(filepath.Base(row.Worktree)) {
			t.Fatalf("web row = %+v, want an agent branch in a wt_ folder", row)
		}
		f.app.Contract(t, brScenario, "AdeTaskBoard.Board.addedBranch", row, brMask, flowharness.Mask("worktree", "addedAt", "name", "tip", "base", "setup"))
	})

	t.Run("deny adds no row", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"web", "feat/web-x"})}, Name: "done"}}, onceStepYML, false)
		f.startRun(t)
		req := f.waitRequest(t)
		if _, err := f.app.W.AdeTask.DenyBranchRequest(adewire.BranchRequestArgs{ID: req.ID}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		if n := len(taskOf(t, board(t, f.app), f.task.ID).BranchIDs); n != 1 {
			t.Fatalf("branch rows after deny = %d, want 1", n)
		}
	})

	t.Run("repeat call finds the branch; deny answers the rest", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{
			MCP:  []fakeagent.MCPCall{requestCall([2]string{"api", "feat/a"}), requestCall([2]string{"api", "feat/a"}, [2]string{"web", "feat/b"})},
			Name: "done",
		}}, onceStepYML, true)
		f.startRun(t)
		first := f.waitRequest(t)
		if _, err := f.app.W.AdeTask.ApproveBranchRequest(adewire.ApproveBranchRequestArgs{ID: first.ID, ItemIDs: []string{first.Items[0].ID}}); err != nil {
			t.Fatal(err)
		}
		var second adewire.BranchRequest
		testx.WaitUntil(t, waitFor, func() bool {
			id := taskOf(t, board(t, f.app), f.task.ID).BranchRequestID
			if id == "" || id == first.ID {
				return false
			}
			second, _ = f.app.W.AdeTask.BranchRequest(adewire.BranchRequestArgs{ID: id})
			return second.ID != ""
		})
		if len(second.Items) != 1 || second.Items[0].CodeRepoID != f.web {
			t.Fatalf("second request = %+v, want web only", second)
		}
		if ans, err := f.app.W.AdeTask.DenyBranchRequest(adewire.BranchRequestArgs{ID: second.ID}); err != nil || !ans.Resolved {
			t.Fatalf("Deny = %+v, %v", ans, err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		got := f.result(t, 2)
		if !strings.Contains(got, "existing") || !strings.Contains(got, "denied") {
			t.Fatalf("second result = %s, want api existing and web denied", got)
		}
	})

	t.Run("no answer times out", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/a"})}, Name: "done"}}, onceStepYML, false,
			flowharness.WithBranchRequestWait(2*time.Second))
		f.startRun(t)
		f.waitRequest(t)
		waitRun(t, f.app, f.task.ID, "one", "done")
		if got := f.result(t, 1); !strings.Contains(got, "timeout") || !strings.Contains(got, "2 seconds") {
			t.Fatalf("agent result = %s, want timeout after 2 seconds", got)
		}
		if ps := f.requestPrompts(t); len(ps) != 0 {
			t.Fatalf("prompts after timeout = %+v", ps)
		}
		if name := f.branchName(t, f.api); name != "" {
			t.Fatalf("branch = %q, want none after a timeout", name)
		}
	})

	t.Run("one request waits per task", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/a"})}, Name: "done"}}, onceStepYML, true)
		f.startRun(t)
		req := f.waitRequest(t)
		_, err := f.app.W.AdeBoard.RequestBranches(ctx, agentmcp.SpaceCall{TaskID: f.task.ID}, []agentmcp.BranchSpec{{Repo: "web", Branch: "feat/w"}})
		if err == nil || !strings.Contains(err.Error(), "already waits") {
			t.Fatalf("second request: %v, want the already-waits refusal", err)
		}
		if _, err := f.app.W.AdeTask.DenyBranchRequest(adewire.BranchRequestArgs{ID: req.ID}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
	})

	t.Run("stopping the session abandons the request", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/a"})}, WaitFile: filepath.Join(t.TempDir(), "never")}},
			"  - id: work\n    name: Work\n    kind: manual\n    session: true\n", false, flowharness.WithTrackerGrace(time.Second))
		f.app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
		launch, err := f.app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.task.ID})
		if err != nil {
			t.Fatal(err)
		}
		openLaunch(t, f.app, launch)
		req := f.waitRequest(t)
		if req.Source != "session" || req.StepName != "Work" {
			t.Fatalf("request = %+v, want a session request from Work", req)
		}
		if err := f.app.W.Terminal.Close(terminal.CloseArgs{TerminalID: launch.TerminalID}); err != nil {
			t.Fatal(err)
		}
		testx.WaitUntil(t, waitFor, func() bool { return taskOf(t, board(t, f.app), f.task.ID).BranchRequestID == "" })
		if ps := f.requestPrompts(t); len(ps) != 0 {
			t.Fatalf("prompts after stop = %+v", ps)
		}
		if _, err := f.app.W.AdeTask.BranchRequest(adewire.BranchRequestArgs{ID: req.ID}); errCode(err) != "E_NOT_FOUND" {
			t.Fatalf("BranchRequest after stop: %v, want E_NOT_FOUND", err)
		}
		if name := f.branchName(t, f.api); name != "" {
			t.Fatalf("branch = %q, want none", name)
		}
	})

	t.Run("legacy kira_space_mcp key loads and is dropped on save", func(t *testing.T) {
		f := newBRFixture(t, nil, onceStepYML, false)
		legacy := "id: old\nname: Old\nkira_space_mcp: true\nsteps:\n" + onceStepYML
		entry, err := f.app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "old.yaml", Yaml: legacy})
		if err != nil || entry.Error != nil || entry.Workflow == nil {
			t.Fatalf("legacy workflow = %+v, %v, want it to load", entry, err)
		}
		saved, err := f.app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "old.yaml", Workflow: *entry.Workflow})
		if err != nil || saved.Error != nil {
			t.Fatalf("SaveWorkflow = %+v, %v", saved, err)
		}
		y, err := f.app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "old.yaml"})
		if err != nil || strings.Contains(y.Yaml, "kira_space_mcp") {
			t.Fatalf("saved yaml = %q, %v, want the legacy key dropped", y.Yaml, err)
		}
	})

	t.Run("a script step names an unnamed branch", func(t *testing.T) {
		f := newBRFixture(t, nil, scriptStep("s", "'exit 0'"), false)
		f.startRun(t)
		waitRun(t, f.app, f.task.ID, "s", "done")
		if name := f.branchName(t, f.api); !strings.HasPrefix(name, "feat/") {
			t.Fatalf("branch = %q, want one derived from the task title under feat/", name)
		}
	})
}
