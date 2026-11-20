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
	libRepo, _ := cloneWithMain(app, filepath.Join(app.Work, "lib"))
	// Run one creates feat/old in api and feat/w-old in web: no approval. Run two asks three things at
	// once: two renames and an adopt of a branch another task holds. All three need the user.
	second := []fakeagent.Action{{
		WaitBefore: filepath.Join(app.Work, "pushed"), Parallel: true, Name: "done",
		MCP: []fakeagent.MCPCall{
			adoptCallIn("lib", "feat/held"),
			renameCall("feat/old", "feat/new", true),
			webRenameCall("feat/w-old", "feat/w-new"),
		},
	}}
	claude(app, map[string][]fakeagent.Action{
		"work": {{MCP: requestCalls([2]string{"api", "feat/old"}, [2]string{"web", "feat/w-old"}), Parallel: true, Name: "done"}},
		"api":  second,
		"web":  second,
	})
	api, web := importRepo(t, app, apiRepo.Dir), importRepo(t, app, webRepo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", onceStepYML+strings.ReplaceAll(onceStepYML, "one", "two")+noSandbox))
	f := &brFixture{app: app, repo: apiRepo, api: api.ID, web: web.ID, task: createTask(t, app, "Fix login", "flow", api.ID, web.ID)}
	// Another task holds feat/held of lib, a repo off this task: adopting it adds the repo.
	lib := importRepo(t, app, libRepo.Dir)
	libRepo.Git("branch", "feat/held")
	donor := createTask(t, app, "Donor", "flow")
	if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: lib.ID, Name: "feat/held", TaskID: donor.ID}); err != nil {
		t.Fatal(err)
	}

	f.createFirst(t)
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
	if len(byBranch) != 3 || byBranch["feat/new"].Kind != "rename" || byBranch["feat/w-new"].Kind != "rename" || byBranch["feat/held"].Kind != "adopt" {
		t.Fatalf("items = %+v, want two renames and one adopt", req.Items)
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

	rename, keep, drop := byBranch["feat/new"], byBranch["feat/held"], byBranch["feat/w-new"]
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

	// Each call reads only its own item. The adopt is the only branch.request of run two (after run one's two).
	if got := f.result(t, 3); !strings.Contains(got, "feat/held") || !strings.Contains(got, `"status":"done"`) || strings.Contains(got, "denied") {
		t.Fatalf("adopt call = %s, want feat/held checked out", got)
	}
	if got := f.renameResult(t); !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("api rename call = %s, want done", got)
	}
	if got := f.resultOf(t, "branch.rename", 2); !strings.Contains(got, "denied") {
		t.Fatalf("web rename call = %s, want denied (%s)", got, drop.ID)
	}
	if id := taskOf(t, board(t, app), f.task.ID).BranchRequestID; id != "" {
		t.Fatalf("request %s still open after the answer", id)
	}
}
