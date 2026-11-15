package adeflow_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P408 branch requests. Contract ade-branch-request: tests/ui/ade-v2-branch-request.spec.ts reads these.

const brScenario = "ade-branch-request"

var (
	brMask = flowharness.Mask("id", "taskId", "createdAt", "expiresAt", "codeRepoId")
	// The approvals item keeps its ids: the UI spec renders and answers them.
	brListMask  = flowharness.Mask("taskId", "createdAt", "expiresAt", "codeRepoId")
	nanoFolder  = regexp.MustCompile(`^wt_[0-9a-hjkmnp-tv-z]{10}$`)
	onceStepYML = "  - id: one\n    name: one\n    kind: agent\n    runs_on: once\n    timeout: 1m\n    prompt: ask for branches\n"
)

type brFixture struct {
	app  *flowharness.App
	repo *flowharness.Repo // the api clone
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
	return &brFixture{app: app, repo: apiRepo, api: api.ID, web: web.ID, task: createTask(t, app, "Fix login", "flow", ids...)}
}

func (f *brFixture) startRun(t *testing.T) {
	t.Helper()
	if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
}

// waitRequest waits for the task's open request, holding at least items items (default 1), and
// returns it. Calls of one run merge into one request as they arrive, so a test waits for them all.
func (f *brFixture) waitRequest(t *testing.T, items ...int) adewire.BranchRequest {
	t.Helper()
	want := 1
	if len(items) > 0 {
		want = items[0]
	}
	var req adewire.BranchRequest
	testx.WaitUntil(t, waitFor, func() bool {
		id := taskOf(t, board(t, f.app), f.task.ID).BranchRequestID
		if id == "" {
			return false
		}
		r, err := branchRequest(f.app, id)
		if err != nil || len(r.Items) < want {
			return false
		}
		req = r
		return true
	})
	return req
}

// itemOf is the request item of a repo nick.
func itemOf(t *testing.T, req adewire.BranchRequest, repo string) adewire.BranchRequestItem {
	t.Helper()
	for _, it := range req.Items {
		if it.Repo == repo {
			return it
		}
	}
	t.Fatalf("request %+v has no item for %s", req, repo)
	return adewire.BranchRequestItem{}
}

func (f *brFixture) requestPrompts(t *testing.T) []prompts.Routed {
	t.Helper()
	all, err := f.app.W.PromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	var out []prompts.Routed
	for _, p := range all {
		if p.Kind == prompts.KindApproval && strings.HasPrefix(p.Ref, branchItemPrefix) {
			out = append(out, p)
		}
	}
	return out
}

// result waits for the n-th branch.request reply of the agent and returns its recorded JSON.
func (f *brFixture) result(t *testing.T, n int) string {
	t.Helper()
	return f.resultOf(t, "branch.request", n)
}

// resultOf is result for any state action (branch.rename, branch.change, ...).
func (f *brFixture) resultOf(t *testing.T, action string, n int) string {
	t.Helper()
	var raw []byte
	testx.WaitUntil(t, waitFor, func() bool {
		var err error
		raw, err = os.ReadFile(filepath.Join(f.app.FakeDir, "mcp-"+action+"-"+strconv.Itoa(n)+".json"))
		return err == nil && len(raw) > 0
	})
	return string(raw)
}

// results joins the first n branch.request replies.
func (f *brFixture) results(t *testing.T, n int) string {
	t.Helper()
	var all []string
	for i := 1; i <= n; i++ {
		all = append(all, f.result(t, i))
	}
	return strings.Join(all, "\n")
}

func branchArgs(repo, branch, base string) map[string]any {
	a := map[string]any{"repo": repo, "branch": branch}
	if base != "" {
		a["base"] = base
	}
	return a
}

// requestCalls is one branch.request call per {repo, branch} pair; run them in a Parallel action so
// they merge into one approval.
func requestCalls(branches ...[2]string) []fakeagent.MCPCall {
	out := make([]fakeagent.MCPCall, len(branches))
	for i, b := range branches {
		out[i] = stateCall("branch.request", branchArgs(b[0], b[1], ""))
		out[i].Tolerate = true
	}
	return out
}

// requestCall is one branch.request call for a {repo, branch} pair.
func requestCall(branch [2]string) fakeagent.MCPCall { return requestCalls(branch)[0] }

// requestCallsBase is requestCalls for {repo, branch, base} triples; "" omits the base.
func requestCallsBase(branches ...[3]string) []fakeagent.MCPCall {
	out := make([]fakeagent.MCPCall, len(branches))
	for i, b := range branches {
		out[i] = stateCall("branch.request", branchArgs(b[0], b[1], b[2]))
		out[i].Tolerate = true
	}
	return out
}

// requestCallBase is one branch.request call for a {repo, branch, base} triple.
func requestCallBase(branch [3]string) fakeagent.MCPCall { return requestCallsBase(branch)[0] }

// pushBase creates name from main with one commit and pushes it, then resets the local copy to main:
// only origin holds the commit. It returns the origin tip.
func (f *brFixture) pushBase(t *testing.T, name string) string {
	t.Helper()
	f.repo.Git("checkout", "-q", "-b", name, "main")
	sha := f.repo.Commit("work on "+name, map[string]string{strings.ReplaceAll(name, "/", "-") + ".txt": name + "\n"})
	f.repo.Git("push", "-q", "origin", name)
	f.repo.Checkout("main")
	f.repo.Git("branch", "-f", name, "main")
	return sha
}

// branchNamed is the task's branch row with that name.
func (f *brFixture) branchNamed(t *testing.T, name string) adewire.Branch {
	t.Helper()
	for _, br := range board(t, f.app).Branches {
		if br.TaskID == f.task.ID && br.Name == name {
			return br
		}
	}
	t.Fatalf("task has no branch %q", name)
	return adewire.Branch{}
}

func headOf(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

func (f *brFixture) branchName(t *testing.T, repoID string) string {
	t.Helper()
	return branchOf(t, board(t, f.app), f.task.ID, repoID).Name
}

// agentOps waits for the finished agent ops of kind under the tool's mcp ops (one op per call), at
// least n of them, and checks their task.
func (f *brFixture) agentOps(t *testing.T, kind, tool string, n int) []oplog.Record {
	t.Helper()
	var got []oplog.Record
	testx.WaitUntil(t, waitFor, func() bool {
		got = got[:0]
		rows := opsNow(t, f.app)
		calls := map[string]bool{}
		for _, r := range rows {
			if r.Category == oplog.CategoryMCP && r.Kind == tool {
				calls[r.ID] = true
			}
		}
		for _, r := range rows {
			if r.Category == oplog.CategoryAgent && r.Kind == kind && calls[r.ParentID] && r.Status != oplog.StatusRunning {
				got = append(got, r)
			}
		}
		return len(got) >= n
	})
	for _, r := range got {
		if r.TaskID != f.task.ID {
			t.Fatalf("%s op %+v is not for task %s", kind, r, f.task.ID)
		}
	}
	return got
}

// agentOp is agentOps for the one op of kind.
func (f *brFixture) agentOp(t *testing.T, kind, tool string) oplog.Record {
	t.Helper()
	got := f.agentOps(t, kind, tool, 1)
	if len(got) != 1 {
		t.Fatalf("%s ops under %s = %+v, want one", kind, tool, got)
	}
	return got[0]
}

func opError(r oplog.Record) string {
	if r.Error == nil {
		return ""
	}
	return *r.Error
}

func TestBranchRequest(t *testing.T) {
	t.Run("approve one repo, deny the other", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: requestCalls([2]string{"api", "feat/login"}, [2]string{"web", "feat/login-web"}), Parallel: true, Name: "done"}}, onceStepYML, true)
		f.startRun(t)
		req := f.waitRequest(t, 2)
		if len(req.Items) != 2 || req.Items[0].Source != "run" || req.Items[0].Kind != "branch" || req.TaskTitle == "" {
			t.Fatalf("request = %+v, want two new-branch items from a run", req)
		}
		f.app.Contract(t, brScenario, "ApprovalsService.List#branch", []approvals.Item{orderedBranchItem(t, f.app, req.ID)}, brListMask)
		if ps := f.requestPrompts(t); len(ps) != 1 || ps[0].Ref != branchItemPrefix+req.ID {
			t.Fatalf("prompts = %+v, want one for the request", ps)
		}
		run, _ := latestRun(t, f.app, f.task.ID, "one")
		if run.Note != "waiting for branch approval" {
			t.Fatalf("run note = %q, want the approval note while it waits", run.Note)
		}

		ans, err := approveBR(f.app, req.ID, []string{itemOf(t, req, "api").ID}, nil)
		if err != nil || !ans.Resolved {
			t.Fatalf("Approve = %+v, %v", ans, err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		got := f.results(t, 2)
		if !strings.Contains(got, `"status":"done"`) || !strings.Contains(got, `"status":"denied"`) {
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
		if _, err := branchRequest(f.app, req.ID); errCode(err) != "E_NOT_FOUND" {
			t.Fatalf("BranchRequest after answer: %v, want E_NOT_FOUND", err)
		}
		for _, op := range f.agentOps(t, "agentApproval", "branch.request", 2) {
			if op.Status != oplog.StatusOK || op.Command == nil || !strings.HasPrefix(*op.Command, "Fix login") {
				t.Fatalf("approval op = %+v, want ok for the task", op)
			}
		}
		if ans, err := denyBR(f.app, req.ID); err != nil || ans.Resolved {
			t.Fatalf("Deny after answer = %+v, %v, want resolved=false", ans, err)
		}
	})

	t.Run("approved branches of two repos are created together", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: requestCalls([2]string{"api", "feat/login"}, [2]string{"web", "feat/login-web"}), Parallel: true, Name: "done"}}, onceStepYML, true)
		f.startRun(t)
		req := f.waitRequest(t, 2)
		mustApprove(t, f.app, req.ID, []string{req.Items[0].ID, req.Items[1].ID}, nil)
		waitRun(t, f.app, f.task.ID, "one", "done")
		if got := f.results(t, 2); strings.Count(got, `"status":"done"`) != 2 {
			t.Fatalf("agent result = %s, want both branches created", got)
		}
		if name := f.branchName(t, f.api); name != "feat/login" {
			t.Fatalf("api branch = %q, want the asked name", name)
		}
		if name := f.branchName(t, f.web); name != "feat/login-web" {
			t.Fatalf("web branch = %q, want the asked name", name)
		}
		for _, op := range f.agentOps(t, "agentApproval", "branch.request", 2) {
			if op.Status != oplog.StatusOK {
				t.Fatalf("approval op = %+v, want ok", op)
			}
		}
		for _, op := range f.agentOps(t, "agentBranches", "branch.request", 1) {
			if op.Status != oplog.StatusOK || op.Command == nil || (*op.Command != "api, web" && *op.Command != "api" && *op.Command != "web") {
				t.Fatalf("branches op = %+v, want ok for the approved repos", op)
			}
		}
	})

	t.Run("one repo's failure leaves the other created and fails the op", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: requestCalls([2]string{"api", "feat/login"}, [2]string{"web", "feat/login-web"}), Parallel: true, Name: "done"}}, onceStepYML, true)
		f.pushBase(t, "feat/api-base")
		draft := branchOf(t, board(t, f.app), f.task.ID, f.api)
		if err := f.app.W.AdeTask.SetBranchBase(ctx, adewire.SetBranchBaseArgs{BranchID: draft.ID, Base: adewire.BaseChoice{Ref: "feat/api-base"}}); err != nil {
			t.Fatal(err)
		}
		f.startRun(t)
		req := f.waitRequest(t, 2)
		f.repo.Git("push", "-q", "origin", "--delete", "feat/api-base")
		f.repo.Git("branch", "-D", "feat/api-base")
		mustApprove(t, f.app, req.ID, []string{req.Items[0].ID, req.Items[1].ID}, nil)
		waitRun(t, f.app, f.task.ID, "one", "done")
		got := f.results(t, 2)
		if !strings.Contains(got, `"status":"failed"`) || !strings.Contains(got, `"status":"done"`) || !strings.Contains(got, `"id":"api/feat/login"`) {
			t.Fatalf("agent result = %s, want api failed and web created", got)
		}
		if name := f.branchName(t, f.web); name != "feat/login-web" {
			t.Fatalf("web branch = %q, want it created despite api failing", name)
		}
		failedOps := 0
		for _, op := range f.agentOps(t, "agentBranches", "branch.request", 1) {
			if op.Status == oplog.StatusError && opError(op) == "failed: api" {
				failedOps++
			}
		}
		if failedOps != 1 {
			t.Fatalf("branches ops failed for api = %d, want 1", failedOps)
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
		if _, err := approveBR(f.app, req.ID, []string{req.Items[0].ID}, nil); err != nil {
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
		if _, err := denyBR(f.app, req.ID); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		if n := len(taskOf(t, board(t, f.app), f.task.ID).BranchIDs); n != 1 {
			t.Fatalf("branch rows after deny = %d, want 1", n)
		}
		if op := f.agentOp(t, "agentApproval", "branch.request"); op.Status != oplog.StatusCancelled || opError(op) != "denied" {
			t.Fatalf("approval op = %+v, want cancelled: denied", op)
		}
	})

	t.Run("repeat call finds the branch; deny answers the rest", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{
			MCP:  []fakeagent.MCPCall{requestCall([2]string{"api", "feat/a"}), requestCall([2]string{"api", "feat/a"}), requestCall([2]string{"web", "feat/b"})},
			Name: "done",
		}}, onceStepYML, true)
		f.startRun(t)
		first := f.waitRequest(t)
		if _, err := approveBR(f.app, first.ID, []string{first.Items[0].ID}, nil); err != nil {
			t.Fatal(err)
		}
		var second adewire.BranchRequest
		testx.WaitUntil(t, waitFor, func() bool {
			id := taskOf(t, board(t, f.app), f.task.ID).BranchRequestID
			if id == "" || id == first.ID {
				return false
			}
			second, _ = branchRequest(f.app, id)
			return second.ID != ""
		})
		if len(second.Items) != 1 || second.Items[0].CodeRepoID != f.web {
			t.Fatalf("second request = %+v, want web only", second)
		}
		if ans, err := denyBR(f.app, second.ID); err != nil || !ans.Resolved {
			t.Fatalf("Deny = %+v, %v", ans, err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		if got := f.result(t, 2); !strings.Contains(got, `"status":"done"`) {
			t.Fatalf("second result = %s, want api found again", got)
		}
		if got := f.result(t, 3); !strings.Contains(got, `"status":"denied"`) {
			t.Fatalf("third result = %s, want web denied", got)
		}
	})

	t.Run("agent base: the item shows it and the worktree starts there", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCallBase([3]string{"api", "feat/login", "feat/api-base"})}, Name: "done"}}, onceStepYML, false)
		tip := f.pushBase(t, "feat/api-base")
		f.startRun(t)
		req := f.waitRequest(t)
		if len(req.Items) != 1 || req.Items[0].Kind != "branch" || req.Items[0].Base != "feat/api-base" {
			t.Fatalf("request = %+v, want one new-branch item from feat/api-base", req)
		}
		f.app.Contract(t, brScenario, "ApprovalsService.List#branchBase", []approvals.Item{*branchItem(f.app, req.ID)}, brListMask)
		if _, err := approveBR(f.app, req.ID, []string{req.Items[0].ID}, nil); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		if got := f.result(t, 1); !strings.Contains(got, `"status":"done"`) {
			t.Fatalf("agent result = %s, want created with the base", got)
		}
		row := f.branchNamed(t, "feat/login")
		if row.Base != "feat/api-base" || row.BaseBranchID != "" || row.BasePendingFrom != "" {
			t.Fatalf("row base = %q, parent %q, pending %q; want feat/api-base, no parent, not pending", row.Base, row.BaseBranchID, row.BasePendingFrom)
		}
		if head := headOf(t, row.Worktree); head != tip {
			t.Fatalf("worktree HEAD = %s, want the origin tip %s of the base", head, tip)
		}
	})

	t.Run("agent base stacks on a task branch", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{
			MCP:  []fakeagent.MCPCall{requestCall([2]string{"api", "feat/a"}), requestCallBase([3]string{"api", "feat/b", "feat/a"})},
			Name: "done",
		}}, onceStepYML, false)
		f.startRun(t)
		first := f.waitRequest(t)
		if _, err := approveBR(f.app, first.ID, []string{first.Items[0].ID}, nil); err != nil {
			t.Fatal(err)
		}
		var second adewire.BranchRequest
		testx.WaitUntil(t, waitFor, func() bool {
			id := taskOf(t, board(t, f.app), f.task.ID).BranchRequestID
			if id == "" || id == first.ID {
				return false
			}
			second, _ = branchRequest(f.app, id)
			return second.ID != ""
		})
		if len(second.Items) != 1 || second.Items[0].Base != "feat/a" {
			t.Fatalf("second request = %+v, want one item from feat/a", second)
		}
		if _, err := approveBR(f.app, second.ID, []string{second.Items[0].ID}, nil); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		a, b := f.branchNamed(t, "feat/a"), f.branchNamed(t, "feat/b")
		if b.BaseBranchID != a.ID || b.Base != "feat/a" {
			t.Fatalf("feat/b base = %q parent %q, want feat/a row %s", b.Base, b.BaseBranchID, a.ID)
		}
		if err := exec.Command("git", "-C", b.Worktree, "merge-base", "--is-ancestor", headOf(t, a.Worktree), "HEAD").Run(); err != nil {
			t.Fatalf("feat/b does not descend from feat/a: %v", err)
		}
	})

	t.Run("a bad base fails that branch before any prompt", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: requestCallsBase([3]string{"api", "feat/x", "no/such"}, [3]string{"api", "feat/y", "bad..name"}), Parallel: true, Name: "done"}}, onceStepYML, false)
		f.startRun(t)
		waitRun(t, f.app, f.task.ID, "one", "done")
		got := f.results(t, 2)
		if strings.Count(got, `"status":"failed"`) != 2 || strings.Count(got, "Cannot use base") != 2 {
			t.Fatalf("agent result = %s, want both branches failed with Cannot use base", got)
		}
		if ps := f.requestPrompts(t); len(ps) != 0 {
			t.Fatalf("prompts = %+v, want none", ps)
		}
	})

	t.Run("the same branch with two bases fails the call", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{Name: "done"}}, onceStepYML, false)
		_, err := f.app.W.AdeBoard.RequestBranches(ctx, agentmcp.SpaceCall{TaskID: f.task.ID}, []agentmcp.BranchSpec{
			{Repo: "api", Branch: "feat/x", Base: "main"}, {Repo: "api", Branch: "feat/x", Base: "develop"},
		})
		if err == nil || !strings.Contains(err.Error(), "asked twice with different bases") {
			t.Fatalf("RequestBranches = %v, want the contradiction refusal", err)
		}
		if ps := f.requestPrompts(t); len(ps) != 0 {
			t.Fatalf("prompts = %+v, want none", ps)
		}
	})

	t.Run("a user-set draft base wins", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{
			MCP:  []fakeagent.MCPCall{requestCallBase([3]string{"api", "feat/x", "main"}), requestCall([2]string{"api", "feat/x"})},
			Name: "done",
		}}, onceStepYML, false)
		f.pushBase(t, "feat/api-base")
		draft := branchOf(t, board(t, f.app), f.task.ID, f.api)
		if err := f.app.W.AdeTask.SetBranchBase(ctx, adewire.SetBranchBaseArgs{BranchID: draft.ID, Base: adewire.BaseChoice{Ref: "feat/api-base"}}); err != nil {
			t.Fatal(err)
		}
		f.startRun(t)
		req := f.waitRequest(t)
		if len(req.Items) != 1 || req.Items[0].Base != "feat/api-base" {
			t.Fatalf("request = %+v, want the item to keep the user's base", req)
		}
		mustDeny(t, f.app, req.ID)
		waitRun(t, f.app, f.task.ID, "one", "done")
		if got := f.result(t, 1); !strings.Contains(got, "failed") || !strings.Contains(got, "feat/api-base") || !strings.Contains(got, "set by the user") {
			t.Fatalf("first result = %s, want a refusal naming the user's base", got)
		}
	})

	t.Run("an existing branch keeps its base", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{
			MCP:  []fakeagent.MCPCall{requestCallBase([3]string{"api", "feat/login", "feat/api-base"}), requestCallBase([3]string{"api", "feat/login", "main"})},
			Name: "done",
		}}, onceStepYML, false)
		f.pushBase(t, "feat/api-base")
		f.startRun(t)
		req := f.waitRequest(t)
		if _, err := approveBR(f.app, req.ID, []string{req.Items[0].ID}, nil); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		if got := f.result(t, 2); !strings.Contains(got, `"status":"done"`) || !strings.Contains(got, "keeps its base feat/api-base") {
			t.Fatalf("second result = %s, want existing with the kept-base note", got)
		}
	})

	t.Run("a second call joins the open request and leaves alone", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/a"})}, Name: "done"}}, onceStepYML, true)
		f.startRun(t)
		req := f.waitRequest(t)
		type answer struct {
			res agentmcp.RequestBranchesResult
			err error
		}
		joined, cancel := context.WithCancel(ctx)
		defer cancel()
		done := make(chan answer, 1)
		go func() {
			res, err := f.app.W.AdeBoard.RequestBranches(joined, agentmcp.SpaceCall{TaskID: f.task.ID}, []agentmcp.BranchSpec{{Repo: "web", Branch: "feat/w"}})
			done <- answer{res, err}
		}()
		merged := f.waitItems(t, 2)
		if merged.ID != req.ID || merged.Items[0].ID != "c0i0" || merged.Items[1].ID != "c1i0" {
			t.Fatalf("merged = %+v, want the same request with items c0i0 and c1i0", merged)
		}
		// The joined caller leaving takes only its own item.
		cancel()
		if a := <-done; a.err == nil {
			t.Fatalf("cancelled call = %+v, want an error", a.res)
		}
		if left := f.waitItems(t, 1); left.Items[0].ID != "c0i0" {
			t.Fatalf("after leaving: %+v, want the first call's item", left.Items)
		}
		mustDeny(t, f.app, req.ID)
		waitRun(t, f.app, f.task.ID, "one", "done")
	})

	t.Run("approval leaves a call that joined after the view rendered pending", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/a"})}, Name: "done"}}, onceStepYML, true)
		f.startRun(t)
		req := f.waitRequest(t)
		joined, cancel := context.WithCancel(ctx)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := f.app.W.AdeBoard.RequestBranches(joined, agentmcp.SpaceCall{TaskID: f.task.ID}, []agentmcp.BranchSpec{{Repo: "web", Branch: "feat/w"}})
			done <- err
		}()
		f.waitItems(t, 2)
		// The user's view held only the first call's item.
		if ans, err := answerBR(f.app, req.ID, true, brOptions{ItemIDs: []string{"c0i0"}, ShownItemIDs: []string{"c0i0"}}); err != nil || !ans.Resolved {
			t.Fatalf("Approve = %+v, %v", ans, err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		left, err := branchRequest(f.app, req.ID)
		if err != nil || len(left.Items) != 1 || left.Items[0].ID != "c1i0" {
			t.Fatalf("after approval: %+v, %v, want the late call's item still open", left, err)
		}
		select {
		case err := <-done:
			t.Fatalf("late call answered early: %v", err)
		default:
		}
		mustDeny(t, f.app, req.ID)
		if err := <-done; err != nil {
			t.Fatalf("late call: %v", err)
		}
	})

	t.Run("denial leaves a call that joined after the view rendered pending", func(t *testing.T) {
		f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/a"})}, Name: "done"}}, onceStepYML, true)
		f.startRun(t)
		req := f.waitRequest(t)
		joined, cancel := context.WithCancel(ctx)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := f.app.W.AdeBoard.RequestBranches(joined, agentmcp.SpaceCall{TaskID: f.task.ID}, []agentmcp.BranchSpec{{Repo: "web", Branch: "feat/w"}})
			done <- err
		}()
		f.waitItems(t, 2)
		if ans, err := answerBR(f.app, req.ID, false, brOptions{ShownItemIDs: []string{"c0i0"}}); err != nil || !ans.Resolved {
			t.Fatalf("Deny = %+v, %v", ans, err)
		}
		left, err := branchRequest(f.app, req.ID)
		if err != nil || len(left.Items) != 1 || left.Items[0].ID != "c1i0" {
			t.Fatalf("after denial: %+v, %v, want the late call's item still open", left, err)
		}
		select {
		case err := <-done:
			t.Fatalf("late call answered early: %v", err)
		default:
		}
		mustDeny(t, f.app, req.ID)
		if err := <-done; err != nil {
			t.Fatalf("late call: %v", err)
		}
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
		if req.Items[0].Source != "session" || req.Items[0].StepName != "Work" {
			t.Fatalf("request = %+v, want a session request from Work", req)
		}
		if err := f.app.W.Terminal.Close(terminal.CloseArgs{TerminalID: launch.TerminalID}); err != nil {
			t.Fatal(err)
		}
		testx.WaitUntil(t, waitFor, func() bool { return taskOf(t, board(t, f.app), f.task.ID).BranchRequestID == "" })
		if ps := f.requestPrompts(t); len(ps) != 0 {
			t.Fatalf("prompts after stop = %+v", ps)
		}
		if _, err := branchRequest(f.app, req.ID); errCode(err) != "E_NOT_FOUND" {
			t.Fatalf("BranchRequest after stop: %v, want E_NOT_FOUND", err)
		}
		if name := f.branchName(t, f.api); name != "" {
			t.Fatalf("branch = %q, want none", name)
		}
		// An interactive session gets the same op as a run.
		if op := f.agentOp(t, "agentApproval", "branch.request"); op.Status != oplog.StatusCancelled || opError(op) != "abandoned" {
			t.Fatalf("approval op = %+v, want cancelled: abandoned", op)
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

// J-ADE-053: twenty branches asked, seven approved. Exactly those seven exist, at the requested bases.
func TestBranchRequestTwentyAskedSevenApproved(t *testing.T) {
	const asked, approved = 20, 7
	var specs [][3]string
	for i := range asked {
		base := ""
		if i%2 == 1 {
			base = "main"
		}
		specs = append(specs, [3]string{"api", "feat/b" + strconv.Itoa(100+i), base})
	}
	f := newBRFixture(t, []fakeagent.Action{{MCP: requestCallsBase(specs...), Parallel: true, Name: "done"}}, onceStepYML, false)
	f.startRun(t)
	req := f.waitItems(t, asked)
	f.app.Contract(t, brScenario, "ApprovalsService.List#twenty", []approvals.Item{orderedBranchItem(t, f.app, req.ID)}, brListMask)
	if ps := f.requestPrompts(t); len(ps) != 1 {
		t.Fatalf("prompts = %+v, want one for the whole request", ps)
	}
	// Calls arrive in any order: pick items by branch name, not position.
	idOf := map[string]string{}
	for _, it := range req.Items {
		idOf[it.Branch] = it.ID
	}
	var keep []string
	for i := 0; i < asked; i += 3 {
		if len(keep) < approved {
			keep = append(keep, idOf[specs[i][1]])
		}
	}
	mustApprove(t, f.app, req.ID, keep, nil)
	waitRun(t, f.app, f.task.ID, "one", "done")
	got := f.results(t, asked)
	if n := strings.Count(got, `"status":"done"`); n != approved {
		t.Fatalf("agent results list %d done, want %d: %s", n, approved, got)
	}
	if n := strings.Count(got, `"status":"denied"`); n != asked-approved {
		t.Fatalf("agent result lists %d denied, want %d: %s", n, asked-approved, got)
	}
	main := gitOut(t, f.repo.Dir, "rev-parse", "main")
	var made int
	for i := range specs {
		out := gitOut(t, f.repo.Dir, "branch", "--list", specs[i][1])
		want := slices.Contains(keep, idOf[specs[i][1]])
		if (strings.TrimSpace(out) != "") != want {
			t.Fatalf("branch %s exists = %q, want exists %v", specs[i][1], out, want)
		}
		if want {
			made++
			if tip := gitOut(t, f.repo.Dir, "rev-parse", specs[i][1]); tip != main {
				t.Fatalf("branch %s tip = %s, want the base main %s", specs[i][1], tip, main)
			}
		}
	}
	if made != approved {
		t.Fatalf("created %d branches, want %d", made, approved)
	}
	var rows int
	for _, br := range board(t, f.app).Branches {
		if br.TaskID != f.task.ID {
			continue
		}
		rows++
		if br.Name == "" || br.Worktree == "" {
			t.Fatalf("task row %+v, want every row named with a worktree: no orphan", br)
		}
	}
	if rows != approved {
		t.Fatalf("task branch rows = %d, want %d, one per created branch", rows, approved)
	}
	if n := strings.Count(gitOut(t, f.repo.Dir, "worktree", "list"), "feat/b"); n != approved {
		t.Fatalf("worktrees on feat/b* = %d, want %d", n, approved)
	}
}

// J-ADE-054: a request waits for the user, not for a minute of the clock. It stays open past 65 s of
// clock time; the answer then still reaches the agent.
func TestBranchRequestSurvivesClockAdvance(t *testing.T) {
	clock := fakeclock.New(scheduleStart)
	f := newBRFixture(t, []fakeagent.Action{{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/late"})}, Name: "done"}}, onceStepYML, false,
		flowharness.WithClock(clock))
	f.startRun(t)
	req := f.waitRequest(t)
	clock.Advance(65 * time.Second)
	again, err := branchRequest(f.app, req.ID)
	if err != nil || len(again.Items) != 1 {
		t.Fatalf("BranchRequest after 65 s = %+v, %v, want it still open", again, err)
	}
	if ps := f.requestPrompts(t); len(ps) != 1 {
		t.Fatalf("prompts after 65 s = %+v, want the approval still queued", ps)
	}
	mustApprove(t, f.app, req.ID, itemIDsOf(again), nil)
	waitRun(t, f.app, f.task.ID, "one", "done")
	if got := f.result(t, 1); !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("agent result = %s, want done after the late answer", got)
	}
	if name := f.branchName(t, f.api); name != "feat/late" {
		t.Fatalf("api branch = %q, want feat/late", name)
	}
}
