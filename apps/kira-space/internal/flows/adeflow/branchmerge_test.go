package adeflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Concurrent branch calls of one task merge into one approvals item (P520). Contract approvals-space:
// tests/ui/approvals.spec.ts renders the merged item.

const mergeScenario = "approvals-space"

var callItemID = regexp.MustCompile(`^c\di0$`)

// waitItems waits until the task's open request holds n items.
func (f *brFixture) waitItems(t *testing.T, n int) adewire.BranchRequest {
	t.Helper()
	var req adewire.BranchRequest
	testx.WaitUntil(t, waitFor, func() bool {
		id := taskOf(t, board(t, f.app), f.task.ID).BranchRequestID
		if id == "" {
			return false
		}
		var err error
		req, err = branchRequest(f.app, id)
		return err == nil && len(req.Items) == n
	})
	return req
}

func TestBranchRequestMerge(t *testing.T) {
	app := flowharness.New(t)
	apiRepo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	webRepo, _ := cloneWithMain(app, filepath.Join(app.Work, "web"))
	// Run one creates feat/old in api. Run two, in that worktree, asks three things at once.
	claude(app, map[string][]fakeagent.Action{
		"work": {{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/old"})}, Name: "done"}},
		"api": {{
			WaitBefore: filepath.Join(app.Work, "pushed"), Parallel: true, Name: "done",
			MCP: []fakeagent.MCPCall{
				requestCall([2]string{"web", "feat/b"}),
				requestCall([2]string{"web", "feat/c"}),
				renameCall("feat/old", "feat/new", true),
			},
		}},
	})
	api, web := importRepo(t, app, apiRepo.Dir), importRepo(t, app, webRepo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", onceStepYML+strings.ReplaceAll(onceStepYML, "one", "two")+noSandbox))
	f := &brFixture{app: app, api: api.ID, web: web.ID, task: createTask(t, app, "Fix login", "flow", api.ID, web.ID)}

	f.startRun(t)
	first := f.waitRequest(t)
	mustApprove(t, app, first.ID, itemIDsOf(first), nil)
	waitRun(t, app, f.task.ID, "one", "done")
	apiRepo.Git("push", "-q", "-u", "origin", "feat/old")
	if err := os.WriteFile(filepath.Join(app.Work, "pushed"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	req := f.waitItems(t, 3)
	byBranch := map[string]adewire.BranchRequestItem{}
	for _, it := range req.Items {
		if !callItemID.MatchString(it.ID) || it.Source != "run" {
			t.Fatalf("item = %+v, want a c<call>i0 id from a run", it)
		}
		byBranch[it.Branch] = it
	}
	if len(byBranch) != 3 || byBranch["feat/new"].Kind != "rename" || byBranch["feat/b"].Kind != "branch" {
		t.Fatalf("items = %+v, want one rename and two new branches", req.Items)
	}
	if ps := f.requestPrompts(t); len(ps) != 1 {
		t.Fatalf("prompts = %+v, want one for the merged request", ps)
	}
	// Calls race, so item order and call numbers vary; the contract stores them sorted by branch.
	stored := *branchItem(app, req.ID)
	sorted := adewire.BranchRequest{Items: slices.Clone(req.Items)}
	slices.SortFunc(sorted.Items, func(a, b adewire.BranchRequestItem) int { return strings.Compare(a.Branch, b.Branch) })
	stable := map[string]string{}
	for i := range sorted.Items {
		stable[sorted.Items[i].ID] = "c" + strconv.Itoa(i) + "i0"
		sorted.Items[i].ID = stable[sorted.Items[i].ID]
		sorted.Items[i].Session = ""
	}
	raw, err := json.Marshal(map[string]any{"taskId": req.TaskID, "taskTitle": req.TaskTitle, "items": sorted.Items})
	if err != nil {
		t.Fatal(err)
	}
	stored.Payload = raw
	// The item id stays a stable placeholder so the UI spec answers the id it rendered.
	mergeMask := brListMask
	f.app.Contract(t, mergeScenario, "ApprovalsService.List", []approvals.Item{stored}, mergeMask)

	rename, keep, drop := byBranch["feat/new"], byBranch["feat/b"], byBranch["feat/c"]
	shown := make([]string, len(sorted.Items))
	for i, it := range sorted.Items {
		shown[i] = stable[it.ID]
	}
	slices.Sort(shown) // the rendered call order
	picked, err := json.Marshal(brOptions{ItemIDs: []string{stable[keep.ID], stable[rename.ID]}, RemoteItemIDs: []string{stable[rename.ID]}, ShownItemIDs: shown})
	if err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, mergeScenario, "args:ApprovalsService.Answer", approvals.AnswerArgs{
		IDs: []string{stored.ID}, Approve: true, Options: map[string]json.RawMessage{stored.ID: picked},
	}, mergeMask)
	mustApprove(t, app, req.ID, []string{rename.ID, keep.ID}, []string{rename.ID})
	waitRun(t, app, f.task.ID, "two", "done")

	// Each call reads only its own items, in call order (result 1 is the first run's request).
	if got := f.result(t, 2); !strings.Contains(got, "created") || strings.Contains(got, "feat/c") || strings.Contains(got, "denied") {
		t.Fatalf("first call = %s, want only feat/b created", got)
	}
	if got := f.result(t, 3); !strings.Contains(got, "denied") || strings.Contains(got, "feat/b") {
		t.Fatalf("second call = %s, want only feat/c denied (%s)", got, drop.ID)
	}
	if got := f.renameResult(t); !strings.Contains(got, `"outcome":"renamed"`) || !strings.Contains(got, `"remote":"renamed"`) {
		t.Fatalf("rename call = %s, want renamed on both sides", got)
	}
	if id := taskOf(t, board(t, app), f.task.ID).BranchRequestID; id != "" {
		t.Fatalf("request %s still open after the answer", id)
	}
}

func TestBranchRequestsTwoTasks(t *testing.T) {
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {
		{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/a"})}, Name: "done"},
		{MCP: []fakeagent.MCPCall{requestCall([2]string{"api", "feat/b"})}, Name: "done"},
	}})
	apiRepo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	api := importRepo(t, app, apiRepo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", onceStepYML))
	one := &brFixture{app: app, api: api.ID, task: createTask(t, app, "Task one", "flow", api.ID)}
	two := &brFixture{app: app, api: api.ID, task: createTask(t, app, "Task two", "flow", api.ID)}
	one.startRun(t)
	two.startRun(t)

	reqOne, reqTwo := one.waitRequest(t), two.waitRequest(t)
	if reqOne.ID == reqTwo.ID || reqOne.TaskID == reqTwo.TaskID {
		t.Fatalf("requests = %+v and %+v, want one per task", reqOne, reqTwo)
	}
	mustDeny(t, app, reqOne.ID)
	waitRun(t, app, one.task.ID, "one", "done")
	if run, _ := latestRun(t, app, two.task.ID, "one"); run.State != "running" {
		t.Fatalf("other task's run = %q, want it still waiting on its own request", run.State)
	}
	if _, err := branchRequest(app, reqTwo.ID); err != nil {
		t.Fatalf("other request: %v", err)
	}
	mustDeny(t, app, reqTwo.ID)
	waitRun(t, app, two.task.ID, "one", "done")
}
