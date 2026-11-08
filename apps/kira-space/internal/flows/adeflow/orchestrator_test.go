package adeflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P571 orchestrator tasks. Contract ade-orchestrator: tests/ui/ade-v2-orchestrator.spec.ts reads these.

const orScenario = "ade-orchestrator"

var (
	orBoardMask = flowharness.Mask("id", "taskId", "runId", "branchId", "sessionId", "startedAt", "finishedAt", "exitCode",
		"createdAt", "updatedAt", "addedAt", "repliedAt", "sentAt", "attempt", "parentId", "branchIds", "fromTaskId", "codeRepoId")
	// The approvals item keeps its id: the UI spec renders and answers it.
	orItemMask = flowharness.Mask("createdAt", "expiresAt", "parentTaskId", "codeRepoId")

	// orchStepYML is the orchestrator's one step: it runs without a repo.
	orchStepYML = "  - id: plan\n    name: Plan\n    kind: agent\n    runs_on: once\n    timeout: 1m\n    prompt: plan the next pass\n"
	// childSteps is a child's workflow: an agent step, then a manual one the user closes.
	childSteps = agentStep("one", "") + "  - id: ship\n    name: Ship\n    kind: manual\n"
)

type orFixture struct {
	app  *flowharness.App
	api  string
	orch adewire.Task
}

// newORFixture registers api, saves workflows orch and flow (and manual, a one-step manual workflow) and
// creates an orchestrator task without repos. The fake agent's "tasks" key serves the orchestrator.
func newORFixture(t *testing.T, tasks, api []fakeagent.Action, opts ...flowharness.Opt) *orFixture {
	t.Helper()
	app := flowharness.New(t, opts...)
	claude(app, map[string][]fakeagent.Action{"tasks": tasks, "api": api})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "orch", flowYAML("orch", orchStepYML))
	saveWorkflow(t, app, "flow", flowYAML("flow", childSteps))
	saveWorkflow(t, app, "manual", flowYAML("manual", "  - id: ship\n    name: Ship\n    kind: manual\n"))
	orch, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Ship login", WorkflowID: "orch", Kind: "orchestrator"})
	if err != nil {
		t.Fatalf("CreateTask orchestrator: %v", err)
	}
	if orch.Kind != "orchestrator" {
		t.Fatalf("kind = %q, want orchestrator", orch.Kind)
	}
	return &orFixture{app: app, api: rec.ID, orch: orch}
}

func (f *orFixture) startOrchestrator(t *testing.T) {
	t.Helper()
	if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.orch.ID}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
}

// createTaskCall is a create_task call.
func createTaskCall(title, workflow, brief string, repos ...map[string]any) fakeagent.MCPCall {
	args := map[string]any{"title": title, "workflow": workflow, "brief": brief}
	if len(repos) > 0 {
		list := make([]any, len(repos))
		for i, r := range repos {
			list[i] = r
		}
		args["repos"] = list
	}
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "create_task", Tolerate: true, Args: args}
}

func spaceCall(tool string, args map[string]any) fakeagent.MCPCall {
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: tool, Tolerate: true, Args: args}
}

func taskItems(app *flowharness.App) []approvals.Item {
	all, err := app.W.ApprovalsSvc.List()
	if err != nil {
		return nil
	}
	var out []approvals.Item
	for _, it := range all {
		if it.Kind == approvals.KindTaskCreate {
			out = append(out, it)
		}
	}
	return out
}

// waitTaskItem waits for the one open task.create item.
func waitTaskItem(t *testing.T, app *flowharness.App) approvals.Item {
	t.Helper()
	var got approvals.Item
	testx.WaitUntil(t, waitFor, func() bool {
		items := taskItems(app)
		if len(items) == 1 {
			got = items[0]
		}
		return len(items) == 1
	})
	return got
}

func answerTask(t *testing.T, app *flowharness.App, it approvals.Item, approve bool) {
	t.Helper()
	res, err := app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{it.ID}, Approve: approve})
	if err != nil || res[0].Error != "" {
		t.Fatalf("answer %s = %+v, %v", it.ID, res, err)
	}
}

type taskItemPayload struct {
	ParentTaskID string `json:"parentTaskId"`
	ParentTitle  string `json:"parentTitle"`
	Title        string `json:"title"`
	Workflow     struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"workflow"`
	Repos []struct {
		CodeRepoID string `json:"codeRepoId"`
		Name       string `json:"name"`
		Base       string `json:"base"`
	} `json:"repos"`
	Brief string `json:"brief"`
}

func itemPayload(t *testing.T, it approvals.Item) taskItemPayload {
	t.Helper()
	var pl taskItemPayload
	if err := json.Unmarshal(it.Payload, &pl); err != nil {
		t.Fatalf("payload: %v", err)
	}
	return pl
}

// mcpResult waits for the fake agent's n-th result of tool and decodes it.
func mcpResult(t *testing.T, app *flowharness.App, tool string, n int) map[string]any {
	t.Helper()
	name := "mcp-" + tool + "-" + strconv.Itoa(n) + ".json"
	testx.WaitUntil(t, waitFor, func() bool {
		raw := fakeFileOrEmpty(app, name)
		return raw != ""
	})
	var out map[string]any
	if err := json.Unmarshal([]byte(fakeFile(t, app, name)), &out); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}

func fakeFileOrEmpty(app *flowharness.App, name string) string {
	raw, err := os.ReadFile(filepath.Join(app.FakeDir, name))
	if err != nil {
		return ""
	}
	return string(raw)
}

func childrenOf(t *testing.T, app *flowharness.App, parent string) []adewire.Task {
	t.Helper()
	var out []adewire.Task
	for _, tk := range board(t, app).Tasks {
		if tk.ParentID == parent {
			out = append(out, tk)
		}
	}
	return out
}

func TestOrchestratorSpawn(t *testing.T) {
	const brief = "Wire the login form to the API.\n\nHand back the endpoint list."
	f := newORFixture(t,
		[]fakeagent.Action{{MCP: []fakeagent.MCPCall{
			createTaskCall("API login", "flow", brief, map[string]any{"repo": "api", "base": "main"}),
			createTaskCall("Denied one", "flow", "never made"),
			createTaskCall("Slow one", "flow", "nobody answers"),
			createTaskCall("Withdrawn one", "flow", "parent archived"),
		}, Name: "done"}},
		[]fakeagent.Action{{Name: "done"}},
		flowharness.WithBranchRequestWait(3*time.Second))
	f.startOrchestrator(t)

	// One approvals item per call, naming the child, workflow, repos and brief.
	it := waitTaskItem(t, f.app)
	if it.Origin.Task != "Ship login" || it.Target.Name != "API login" || it.Operation.Class != "create" || it.Origin.Source != "run" {
		t.Fatalf("item = %+v, want the orchestrator's request for API login", it)
	}
	pl := itemPayload(t, it)
	if pl.ParentTaskID != f.orch.ID || pl.Workflow.ID != "flow" || len(pl.Repos) != 1 || pl.Repos[0].Base != "main" || pl.Repos[0].Name != "api" || pl.Brief != brief {
		t.Fatalf("payload = %+v", pl)
	}
	f.app.Contract(t, orScenario, "ApprovalsService.List#taskCreate", []approvals.Item{it}, orItemMask)
	if run, _ := latestRun(t, f.app, f.orch.ID, "plan"); run.Note != "waiting for task approval" {
		t.Fatalf("run note = %q, want the approval note while it waits", run.Note)
	}

	answerTask(t, f.app, it, true)
	res := mcpResult(t, f.app, "create_task", 1)
	if res["outcome"] != "created" || res["taskId"] == "" {
		t.Fatalf("result 1 = %v, want created", res)
	}
	childID, _ := res["taskId"].(string)
	// The board is read while the second call waits: the orchestrator's run note is then settled.
	it = waitTaskItem(t, f.app)
	b := board(t, f.app)
	child := taskOf(t, b, childID)
	if child.ParentID != f.orch.ID || child.ParentTitle != "Ship login" || child.ParentArchived || child.Kind != "task" || child.Title != "API login" {
		t.Fatalf("child = %+v, want a task under the orchestrator", child)
	}
	if got := branchOf(t, b, childID, f.api).Base; got != "main" {
		t.Fatalf("child branch base = %q, want main", got)
	}
	hs := handovers(t, f.app, childID)
	if len(hs) != 1 || hs[0].Source != "orchestrator" || hs[0].State != "handed" || hs[0].ToStepID != "one" || hs[0].Body != brief || hs[0].FromStepName != "Ship login" || hs[0].FromTaskID != f.orch.ID {
		t.Fatalf("child handovers = %+v, want the brief into step one", hs)
	}
	f.app.Contract(t, orScenario, "AdeTaskService.Board#orchestrator", b, orBoardMask)

	// The child's first step prompt carries the brief.
	br := branchOf(t, b, childID, f.api)
	if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: childID, BranchNames: map[string]string{br.ID: "feat/api-login"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, childID, "one", "done")
	if got := fakeFile(t, f.app, "api-1.prompt"); !strings.Contains(got, brief) {
		t.Fatalf("child prompt = %q, want the brief", got)
	}

	// A denied call creates nothing.
	answerTask(t, f.app, it, false)
	if res = mcpResult(t, f.app, "create_task", 2); res["outcome"] != "denied" || res["taskId"] != nil {
		t.Fatalf("result 2 = %v, want denied", res)
	}
	if n := len(childrenOf(t, f.app, f.orch.ID)); n != 1 {
		t.Fatalf("children after a denial = %d, want 1", n)
	}

	// An unanswered call times out and withdraws its item.
	if slow := waitTaskItem(t, f.app); slow.Target.Name != "Slow one" {
		t.Fatalf("item = %+v, want the slow call", slow)
	}
	if res = mcpResult(t, f.app, "create_task", 3); res["outcome"] != "timeout" || !strings.Contains(res["next"].(string), "3 seconds") {
		t.Fatalf("result 3 = %v, want timeout after 3 seconds", res)
	}
	for _, it := range taskItems(f.app) {
		if it.Target.Name == "Slow one" {
			t.Fatalf("item %+v is still open after the timeout", it)
		}
	}

	// Archiving the orchestrator withdraws a pending call; its children stay.
	waitTaskItem(t, f.app)
	if err := f.app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: f.orch.ID}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return len(taskItems(f.app)) == 0 })
	if n := len(childrenOf(t, f.app, f.orch.ID)); n != 1 {
		t.Fatalf("children after the archive = %d, want 1", n)
	}
	archived := taskByID(t, f.app, childID)
	if !archived.ParentArchived {
		t.Fatalf("child = %+v, want its parent shown archived", archived)
	}
}

// taskByID reads a live task from the board.
func taskByID(t *testing.T, app *flowharness.App, id string) adewire.Task {
	t.Helper()
	return taskOf(t, board(t, app), id)
}

func TestOrchestratorPass(t *testing.T) {
	f := newORFixture(t, []fakeagent.Action{{Name: "done"}, {Name: "done"}}, nil)
	f.startOrchestrator(t)
	waitRun(t, f.app, f.orch.ID, "plan", "done")
	if hs := handovers(t, f.app, f.orch.ID); len(hs) != 1 || hs[0].ToStepID != "done" || hs[0].Body != "summary-done" {
		t.Fatalf("pass 1 handovers = %+v, want the final run row", hs)
	}

	msg := "Focus on the API tasks."
	args := adewire.NextPassArgs{TaskID: f.orch.ID, Notes: &msg}
	res, err := f.app.W.AdeTask.NextPass(ctx, args)
	if err != nil || res.StepID != "plan" || !res.Started {
		t.Fatalf("NextPass = %+v, %v, want plan started", res, err)
	}
	f.app.Contract(t, orScenario, "args:AdeTaskService.NextPass", args)
	f.app.Contract(t, orScenario, "AdeTaskService.NextPass", res)
	testx.WaitUntil(t, waitFor, func() bool {
		r, ok := latestRun(t, f.app, f.orch.ID, "plan")
		return ok && r.Attempt >= 2 && r.State == "done"
	})

	// The run's prompt carries the message and the previous pass's final handover.
	prompt := fakeFile(t, f.app, "tasks-2.prompt")
	if !strings.Contains(prompt, msg) || !strings.Contains(prompt, "summary-done") {
		t.Fatalf("pass 2 prompt = %q, want the message and the previous handover", prompt)
	}
	hs := handovers(t, f.app, f.orch.ID)
	var pass *adewire.Handover
	for i := range hs {
		if hs[i].Source == "pass" {
			pass = &hs[i]
		}
	}
	if pass == nil || pass.State != "handed" || pass.FromStepID != "done" || pass.ToStepID != "plan" || !strings.Contains(pass.Body, msg) || !strings.Contains(pass.Body, "summary-done") {
		t.Fatalf("handovers = %+v, want a handed pass row done -> plan", hs)
	}
	// Pass 1's run stays history behind the watermark; both runs are on the task.
	task := taskByID(t, f.app, f.orch.ID)
	if len(task.Runs) != 2 || task.Since["plan/"] != 1 {
		t.Fatalf("task runs = %d, since = %v, want two runs and pass 1 behind the watermark", len(task.Runs), task.Since)
	}

	// Refusals: a pass starts only from done, and only on an orchestrator.
	fresh, err := f.app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Idle", WorkflowID: "orch", Kind: "orchestrator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.W.AdeTask.NextPass(ctx, adewire.NextPassArgs{TaskID: fresh.ID}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("NextPass on an unfinished orchestrator: %v, want E_BAD_REQUEST", err)
	}
	plain := createTask(t, f.app, "Plain", "manual")
	if _, err := f.app.W.AdeTask.NextPass(ctx, adewire.NextPassArgs{TaskID: plain.ID}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("NextPass on a plain task: %v, want E_BAD_REQUEST", err)
	}
}

func TestOrchestratorChildChain(t *testing.T) {
	f := newORFixture(t,
		[]fakeagent.Action{
			{MCP: []fakeagent.MCPCall{
				createTaskCall("With notes", "flow", "brief one", map[string]any{"repo": "api"}),
				createTaskCall("Without row", "manual", "brief two"),
			}, Name: "done"},
			{MCP: []fakeagent.MCPCall{
				spaceCall("list_tasks", nil),
				spaceCall("task_chain", map[string]any{"limit": 5}),
			}, Name: "done"},
		},
		[]fakeagent.Action{{Name: "done"}})
	f.startOrchestrator(t)
	var ids [2]string
	for i := range ids {
		answerTask(t, f.app, waitTaskItem(t, f.app), true)
		ids[i], _ = mcpResult(t, f.app, "create_task", i+1)["taskId"].(string)
		if ids[i] == "" {
			t.Fatalf("call %d created no task", i+1)
		}
	}
	waitRun(t, f.app, f.orch.ID, "plan", "done")

	// The first child runs, then the user closes its last stage with notes.
	br := branchOf(t, board(t, f.app), ids[0], f.api)
	if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: ids[0], BranchNames: map[string]string{br.ID: "feat/with-notes"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, ids[0], "one", "done")
	notes := "Endpoints: POST /login."
	if _, err := f.app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: ids[0], Notes: &notes}); err != nil {
		t.Fatal(err)
	}
	// The second child finishes with no handover text at all.
	if _, err := f.app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: ids[1]}); err != nil {
		t.Fatal(err)
	}

	hs := handovers(t, f.app, f.orch.ID)
	var mirrors []adewire.Handover
	for _, h := range hs {
		if h.Source == "child" {
			mirrors = append(mirrors, h)
		}
	}
	if len(mirrors) != 2 {
		t.Fatalf("parent handovers = %+v, want two child rows", hs)
	}
	// Newest first: the second child, then the first.
	if m := mirrors[0]; m.State != "note" || m.FromTaskID != ids[1] || m.FromTaskTitle != "Without row" || m.Body != "" {
		t.Fatalf("mirror of the second child = %+v, want an empty note", m)
	}
	if m := mirrors[1]; m.State != "note" || m.FromTaskID != ids[0] || m.FromTaskTitle != "With notes" || m.Body != notes || m.FromStepName != "Ship" {
		t.Fatalf("mirror of the first child = %+v, want the final notes", m)
	}
	f.app.Contract(t, orScenario, "AdeTaskService.Handovers#parent", adewire.HandoversResult{Handovers: hs}, orBoardMask)
	if err := f.app.W.AdeTask.UpdateHandover(ctx, adewire.UpdateHandoverArgs{ID: mirrors[1].ID, Body: "edited"}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("editing a child row: %v, want E_BAD_REQUEST", err)
	}
	if got := taskByID(t, f.app, f.orch.ID); got.HandoverIn != nil && got.HandoverIn.Source == "child" || strings.Contains(got.Notes, notes) {
		t.Fatalf("parent = %+v, want child rows out of its handover and notes", got)
	}

	// The next pass's prompt leaves child rows out.
	if res, err := f.app.W.AdeTask.NextPass(ctx, adewire.NextPassArgs{TaskID: f.orch.ID}); err != nil || !res.Started {
		t.Fatalf("NextPass = %+v, %v", res, err)
	}
	listed := mcpResult(t, f.app, "list_tasks", 1)
	tasks, _ := listed["tasks"].([]any)
	if len(tasks) != 2 {
		t.Fatalf("list_tasks = %v, want both children", listed)
	}
	if first := tasks[0].(map[string]any); first["id"] != ids[1] || first["finished"] != true || first["archived"] != false {
		t.Fatalf("newest child = %v, want the second child finished", first)
	}
	if got := mcpResult(t, f.app, "task_chain", 1); len(got["rows"].([]any)) == 0 {
		t.Fatalf("task_chain = %v, want the own chain", got)
	}
	if prompt := fakeFile(t, f.app, "tasks-2.prompt"); strings.Contains(prompt, notes) {
		t.Fatalf("pass 2 prompt = %q, want no child notes in it", prompt)
	}

	// Engine reads: paging, child chains, archived children, strangers.
	call := agentmcp.SpaceCall{TaskID: f.orch.ID}
	eng := f.app.W.AdeTask.Engine
	page1, err := eng.TaskChain(ctx, call, agentmcp.TaskChainArgs{Limit: 1})
	if err != nil || len(page1.Rows) != 1 || !page1.More || page1.Next != page1.Rows[0].ID {
		t.Fatalf("page 1 = %+v, %v, want one row and a cursor", page1, err)
	}
	page2, err := eng.TaskChain(ctx, call, agentmcp.TaskChainArgs{Limit: 1, Before: page1.Next})
	if err != nil || len(page2.Rows) != 1 || page2.Rows[0].ID == page1.Rows[0].ID {
		t.Fatalf("page 2 = %+v, %v, want the next older row", page2, err)
	}
	childChain, err := eng.TaskChain(ctx, call, agentmcp.TaskChainArgs{TaskID: ids[0], Limit: 10})
	if err != nil || len(childChain.Rows) == 0 || childChain.Rows[len(childChain.Rows)-1].Source != "orchestrator" {
		t.Fatalf("child chain = %+v, %v, want the brief at the oldest end", childChain, err)
	}
	plain := createTask(t, f.app, "Stranger", "manual")
	if _, err := eng.TaskChain(ctx, call, agentmcp.TaskChainArgs{TaskID: plain.ID}); err == nil || !strings.Contains(err.Error(), "not one of your child tasks") {
		t.Fatalf("chain of a stranger: %v, want a refusal", err)
	}
	if err := f.app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: ids[1]}); err != nil {
		t.Fatal(err)
	}
	archived, err := eng.ListTasks(ctx, call, agentmcp.ListTasksArgs{State: "archived"})
	if err != nil || len(archived.Tasks) != 1 || archived.Tasks[0].ID != ids[1] || !archived.Tasks[0].Archived || !archived.Tasks[0].Finished {
		t.Fatalf("archived children = %+v, %v", archived, err)
	}
	live, err := eng.ListTasks(ctx, call, agentmcp.ListTasksArgs{State: "live"})
	if err != nil || len(live.Tasks) != 1 || live.Tasks[0].ID != ids[0] || live.Tasks[0].LastHandover != notes {
		t.Fatalf("live children = %+v, %v", live, err)
	}
	if _, err := eng.TaskChain(ctx, call, agentmcp.TaskChainArgs{TaskID: ids[1]}); err != nil {
		t.Fatalf("chain of an archived child: %v, want it readable", err)
	}
}

func TestOrchestratorToolGrants(t *testing.T) {
	f := newORFixture(t, []fakeagent.Action{{Name: "done"}, {Name: "done"}}, []fakeagent.Action{{Name: "done"}})
	f.startOrchestrator(t)
	waitRun(t, f.app, f.orch.ID, "plan", "done")
	plain := createTask(t, f.app, "Plain", "orch")
	if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: plain.ID}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, plain.ID, "plan", "done")
	stepTask := createTask(t, f.app, "On api", "flow", f.api)
	br := branchOf(t, board(t, f.app), stepTask.ID, f.api)
	if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: stepTask.ID, BranchNames: map[string]string{br.ID: "feat/api-grants"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, stepTask.ID, "one", "done")

	for _, want := range orchestratorToolNames() {
		if args := fakeFile(t, f.app, "tasks-1.args"); !strings.Contains(args, want) {
			t.Errorf("orchestrator run lacks %q:\n%s", want, args)
		}
		for _, file := range []string{"tasks-2.args", "api-1.args"} {
			if args := fakeFile(t, f.app, file); strings.Contains(args, want) {
				t.Errorf("%s carries %q, want it for an orchestrator only", file, want)
			}
		}
	}
}

// A child whose last step is automatic finishes without StepDone; its parent chain still gets one finish row.
func TestOrchestratorChildChainAutomaticFinish(t *testing.T) {
	f := newORFixture(t,
		[]fakeagent.Action{{MCP: []fakeagent.MCPCall{
			createTaskCall("Automatic", "auto", "brief", map[string]any{"repo": "api"}),
		}, Name: "done"}},
		[]fakeagent.Action{{Name: "done"}})
	saveWorkflow(t, f.app, "auto", flowYAML("auto", agentStep("one", "")+scriptStep("check", "'exit 0'")))
	f.startOrchestrator(t)
	answerTask(t, f.app, waitTaskItem(t, f.app), true)
	id, _ := mcpResult(t, f.app, "create_task", 1)["taskId"].(string)
	if id == "" {
		t.Fatal("create_task created no task")
	}
	waitRun(t, f.app, f.orch.ID, "plan", "done")
	br := branchOf(t, board(t, f.app), id, f.api)
	if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: id, BranchNames: map[string]string{br.ID: "feat/automatic"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, id, "check", "done")
	testx.WaitUntil(t, waitFor, func() bool { return taskByID(t, f.app, id).EntryStepID == "done" })

	var mirrors []adewire.Handover
	for _, h := range handovers(t, f.app, f.orch.ID) {
		if h.Source == "child" {
			mirrors = append(mirrors, h)
		}
	}
	if len(mirrors) != 1 || mirrors[0].FromTaskID != id || mirrors[0].FromStepName == "" {
		t.Fatalf("parent child rows = %+v, want exactly one named finish row", mirrors)
	}
}
