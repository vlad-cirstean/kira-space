package mobileflow_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/flowtest"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P637 phone approvals. Contract mobile-approvals: tests/mobile/approvals.spec.ts and
// tests/ui/ade-mobile-approval.spec.ts read these.

const approvalsScenario = "mobile-approvals"

var apItemMask = flowharness.Mask("createdAt", "expiresAt", "parentTaskId", "codeRepoId")

func apTaskCall(title string) fakeagent.MCPCall {
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "create_task", Tolerate: true,
		Args: map[string]any{"title": title, "workflow": "manual", "brief": "Wire the login form to the API.\n\nHand back the endpoint list."}}
}

func apTaskItem(t *testing.T, app *flowharness.App, title string) approvals.Item {
	t.Helper()
	var got approvals.Item
	testx.WaitUntil(t, waitFor, func() bool {
		all, err := app.W.ApprovalsSvc.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range all {
			if it.Kind == approvals.KindTaskCreate && it.Target.Name == title {
				got = it
				return true
			}
		}
		return false
	})
	return got
}

func apIDs(items []approvals.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	slices.Sort(out)
	return out
}

func apListed(t *testing.T, app *flowharness.App, id string) bool {
	t.Helper()
	all, err := app.W.ApprovalsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range all {
		if it.ID == id {
			return true
		}
	}
	return false
}

func apResult(t *testing.T, app *flowharness.App, tool string, n int) map[string]any {
	t.Helper()
	name := "mcp-" + tool + "-" + strconv.Itoa(n) + ".json"
	var out map[string]any
	testx.WaitUntil(t, waitFor, func() bool {
		raw, err := os.ReadFile(filepath.Join(app.FakeDir, name))
		// The fake agent may still be writing: wait for complete JSON.
		return err == nil && json.Unmarshal(raw, &out) == nil
	})
	return out
}

// cwdSet reports a "cwd" key that holds a value.
func cwdSet(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		if s, ok := x["cwd"]; ok && s != "" {
			return true
		}
		for _, c := range x {
			if cwdSet(c) {
				return true
			}
		}
	case []any:
		for _, c := range x {
			if cwdSet(c) {
				return true
			}
		}
	}
	return false
}

func apAnswer(t *testing.T, p *phone, ids []string, approve bool) (reply, []approvals.Result) {
	t.Helper()
	r := p.post("/api/approvals/answer", map[string]any{"ids": ids, "approve": approve})
	var res []approvals.Result
	if r.Status == http.StatusOK {
		r.json(t, &res)
	}
	return r, res
}

func TestPhoneApprovals(t *testing.T) {
	app := flowharness.New(t)
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"tasks": {{MCP: []fakeagent.MCPCall{
		apTaskCall("API login"), apTaskCall("Denied one"), apTaskCall("Expired one"),
	}, Name: "done"}}}})
	for id, steps := range map[string]string{
		"orch":   "  - id: plan\n    name: Plan\n    kind: agent\n    runs_on: once\n    timeout: 1m\n    prompt: plan the next pass\n",
		"manual": "  - id: ship\n    name: Ship\n    kind: manual\n",
	} {
		entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: id + ".yaml", Yaml: "id: " + id + "\nname: Flow\nsteps:\n" + steps})
		if err != nil || entry.Error != nil {
			t.Fatalf("SaveWorkflowYaml %s: %v %+v", id, err, entry.Error)
		}
	}
	orch, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Ship login", WorkflowID: "orch", Kind: "orchestrator"})
	if err != nil {
		t.Fatal(err)
	}

	st := serve(t, app)
	p := newPhone(t, st.AppURL)
	deviceID := p.pair(app, "Ana's phone")
	setAgent := func(on bool) {
		t.Helper()
		if err := app.W.Mobile.SetDevicePermissions(bridge.MobileDevicePermissionsArgs{ID: deviceID, AgentInput: on}); err != nil {
			t.Fatal(err)
		}
	}
	setAgent(true)
	if _, err := app.W.Mobile.SetAgentInputEnabled(bridge.MobileSetAgentInputArgs{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: orch.ID}); err != nil {
		t.Fatal(err)
	}
	taskItem := apTaskItem(t, app, "API login")

	// A studio item and a branch request that holds an absolute worktree path.
	const dbID, branchID = "db.query:p637", "branch.request:p637"
	worktree := filepath.Join(app.Work, "wt", "feat-x")
	branchPayload, err := json.Marshal(map[string]any{"taskId": "t1", "taskTitle": "Fix login", "items": []adewire.BranchRequestItem{{
		ID: "c1i1", CodeRepoID: "r1", Repo: "api", Kind: "adopt", Branch: "feat/x", Stacked: []string{}, Source: "session",
		Holder: &adewire.BranchHolder{Kind: "other", TaskID: "t2", BranchID: "b2", Worktree: worktree},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	// After the task item: equal milliseconds order by id, which puts the branch request first.
	putAt := max(time.Now().UnixMilli(), taskItem.CreatedAt+1)
	app.W.Approvals.Put(approvals.Item{ID: dbID, Kind: approvals.KindDbQuery, CreatedAt: putAt,
		Origin: approvals.Origin{Cwd: app.Work}, Payload: json.RawMessage(`{"statement":"select 1"}`)})
	app.W.Approvals.Put(approvals.Item{ID: branchID, Kind: approvals.KindBranchRequest, CreatedAt: putAt,
		Origin: approvals.Origin{Cwd: app.Work}, Target: approvals.Target{Name: "Fix login", Location: app.Work},
		Operation: approvals.Operation{Class: "request", Tone: "neutral"}, Reason: "request", Payload: branchPayload})
	defer app.W.Approvals.Remove(dbID)
	defer app.W.Approvals.Remove(branchID)

	// The phone lists Space kinds only, with no absolute path.
	r := p.get("/api/approvals")
	if r.Status != http.StatusOK {
		t.Fatalf("GET /api/approvals = %d %s", r.Status, r.Body)
	}
	var listed []approvals.Item
	r.json(t, &listed)
	if got, want := apIDs(listed), []string{branchID, taskItem.ID}; !slices.Equal(got, want) {
		t.Fatalf("phone approvals = %v, want %v", got, want)
	}
	// origin.cwd stays as an empty string: the shared item schema requires the key.
	if cwdSet(r.raw(t)) || strings.Contains(string(r.Body), app.Work) {
		t.Fatalf("phone approvals leak a path: %s", r.Body)
	}
	for _, it := range listed {
		if it.Kind != approvals.KindBranchRequest {
			continue
		}
		var pl struct {
			Items []struct {
				Holder struct {
					Worktree string `json:"worktree"`
				} `json:"holder"`
			} `json:"items"`
		}
		if err := json.Unmarshal(it.Payload, &pl); err != nil || len(pl.Items) != 1 || pl.Items[0].Holder.Worktree != "feat-x" {
			t.Fatalf("branch payload = %s (%v), want holder worktree feat-x", it.Payload, err)
		}
	}
	app.Contract(t, approvalsScenario, "http:GET /api/approvals#pending", r.raw(t), apItemMask)

	// Approve from the phone: the agent's call unblocks, the desktop and the phone follow.
	stream := p.events()
	mark := app.Events.Mark()
	r, res := apAnswer(t, p, []string{taskItem.ID}, true)
	if r.Status != http.StatusOK || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("approve = %d %s", r.Status, r.Body)
	}
	app.Contract(t, approvalsScenario, "http:POST /api/approvals/answer#approved", r.raw(t))
	created := apResult(t, app, "create_task", 1)
	if created["outcome"] != "created" || created["taskId"] == "" {
		t.Fatalf("result 1 = %v, want created", created)
	}
	childID, _ := created["taskId"].(string)
	// The board is read while the second call waits: the orchestrator's run note is then settled.
	denied := apTaskItem(t, app, "Denied one")
	var b adewire.Board
	testx.WaitUntil(t, waitFor, func() bool {
		if b, err = app.W.AdeTask.Board(ctx); err != nil {
			t.Fatal(err)
		}
		for _, tk := range b.Tasks {
			if tk.ID == orch.ID {
				return len(tk.Runs) == 1 && tk.Runs[0].Note == "waiting for task approval"
			}
		}
		return false
	})
	var child *adewire.Task
	var children int
	for i, tk := range b.Tasks {
		if tk.ParentID == orch.ID {
			children++
		}
		if tk.ID == childID {
			child = &b.Tasks[i]
		}
	}
	if child == nil || child.ParentID != orch.ID || child.Title != "API login" || children != 1 {
		t.Fatalf("board child = %+v (%d children), want API login under the orchestrator", child, children)
	}
	app.Contract(t, approvalsScenario, "AdeTaskService.Board#afterPhoneApprove", b, flowharness.Mask("id", "taskId", "runId", "branchId", "sessionId", "startedAt", "finishedAt", "exitCode",
		"createdAt", "updatedAt", "addedAt", "repliedAt", "sentAt", "attempt", "parentId", "branchIds", "fromTaskId", "codeRepoId"))
	if apListed(t, app, taskItem.ID) {
		t.Fatalf("item %s is still pending on the desktop", taskItem.ID)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		all, err := app.W.PromptsSvc.List()
		if err != nil {
			t.Fatal(err)
		}
		return !slices.ContainsFunc(all, func(pr prompts.Routed) bool { return pr.Kind == prompts.KindApproval && pr.Ref == taskItem.ID })
	})
	// The desktop's push after the answer: every item it still lists is one the test added or a
	// later call, so the UI spec replays the answered item as gone.
	ev := app.Events.WaitAfter(t, mark, "kira:approvals:changed", func(e flowtest.Event) bool {
		items, _ := e.Data.([]approvals.Item)
		return !slices.ContainsFunc(items, func(it approvals.Item) bool { return it.ID == taskItem.ID })
	}, waitFor)
	var rest []approvals.Item
	for _, it := range ev.Data.([]approvals.Item) {
		if it.ID != dbID && it.ID != branchID && it.Target.Name != "Denied one" {
			rest = append(rest, it)
		}
	}
	app.Contract(t, approvalsScenario, "event:kira:approvals:changed#afterPhoneApprove", append([]approvals.Item{}, rest...), apItemMask)
	var frame []approvals.Item
	for {
		f := stream.wait(t, "kira:approvals:changed", waitFor)
		var items []approvals.Item
		if err := json.Unmarshal([]byte(f.Data), &items); err != nil {
			t.Fatalf("frame %q: %v", f.Data, err)
		}
		if !slices.ContainsFunc(items, func(it approvals.Item) bool { return it.ID == taskItem.ID }) {
			frame = items
			if cwdSet(normalize(t, items)) || strings.Contains(f.Data, app.Work) {
				t.Fatalf("pushed approvals leak a path: %s", f.Data)
			}
			app.Contract(t, approvalsScenario, "sse:kira:approvals:changed#afterApprove", normalize(t, items), apItemMask)
			break
		}
	}
	if slices.ContainsFunc(frame, func(it approvals.Item) bool { return it.Kind == approvals.KindDbQuery }) {
		t.Fatalf("pushed approvals hold a studio item: %v", apIDs(frame))
	}

	// Agent input off for the phone: refused, item stays, agent stays blocked.
	setAgent(false)
	r, _ = apAnswer(t, p, []string{denied.ID}, false)
	if r.Status != http.StatusForbidden || r.code(t) != "E_AGENT_INPUT_OFF/device" {
		t.Fatalf("answer without agent input = %d %s", r.Status, r.Body)
	}
	if !apListed(t, app, denied.ID) {
		t.Fatalf("refused answer removed %s", denied.ID)
	}
	if _, err := os.Stat(filepath.Join(app.FakeDir, "mcp-create_task-2.json")); err == nil {
		t.Fatal("refused answer unblocked the agent")
	}
	setAgent(true)

	// Deny blocks the step.
	r, res = apAnswer(t, p, []string{denied.ID}, false)
	if r.Status != http.StatusOK || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("deny = %d %s", r.Status, r.Body)
	}
	app.Contract(t, approvalsScenario, "http:POST /api/approvals/answer#denied", r.raw(t))
	if got := apResult(t, app, "create_task", 2); got["outcome"] != "denied" || got["taskId"] != nil {
		t.Fatalf("result 2 = %v, want denied", got)
	}
	if b, err = app.W.AdeTask.Board(ctx); err != nil {
		t.Fatal(err)
	}
	children = 0
	for _, tk := range b.Tasks {
		if tk.ParentID == orch.ID {
			children++
		}
	}
	if children != 1 {
		t.Fatalf("children after a denial = %d, want 1", children)
	}

	// A studio item is not the phone's to answer.
	r, res = apAnswer(t, p, []string{dbID}, true)
	if r.Status != http.StatusOK || len(res) != 1 || res[0].Code != "E_NOT_FOUND" || res[0].Error == "" {
		t.Fatalf("answer of a studio item = %d %s", r.Status, r.Body)
	}
	if !apListed(t, app, dbID) {
		t.Fatalf("phone answer removed %s", dbID)
	}
	app.Contract(t, approvalsScenario, "http:POST /api/approvals/answer#notPhoneKind", r.raw(t))

	// An expired device token is refused, item stays.
	// Each phone gets one request: the first refusal clears its cookie.
	expired := apTaskItem(t, app, "Expired one")
	q := newPhone(t, st.AppURL)
	q.pair(app, "Bo's phone")
	if _, err := app.DB().Exec(`UPDATE mobile_devices SET expires_at = ?`, time.Now().Add(-time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if r := q.get("/api/approvals"); r.Status != http.StatusUnauthorized || r.code(t) != "E_EXPIRED" {
		t.Fatalf("expired GET /api/approvals = %d %s", r.Status, r.Body)
	}
	r, _ = apAnswer(t, p, []string{expired.ID}, true)
	if r.Status != http.StatusUnauthorized || r.code(t) != "E_EXPIRED" {
		t.Fatalf("expired answer = %d %s", r.Status, r.Body)
	}
	app.Contract(t, approvalsScenario, "http:POST /api/approvals/answer#expired", r.raw(t))
	if !apListed(t, app, expired.ID) {
		t.Fatalf("expired answer removed %s", expired.ID)
	}
	if _, err := os.Stat(filepath.Join(app.FakeDir, "mcp-create_task-3.json")); err == nil {
		t.Fatal("expired answer unblocked the agent")
	}
}

// A phone answers a details.edit request: the agent's note edit is kept on Approve.
func TestPhoneDetailsEdit(t *testing.T) {
	app := flowharness.New(t)
	entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "manual.yaml", Yaml: "id: manual\nname: Flow\nsteps:\n  - id: ship\n    name: Ship\n    kind: manual\n"})
	if err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflowYaml: %v %+v", err, entry.Error)
	}
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Login form", WorkflowID: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"Check the error states.", "Newer note."} {
		if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	where, err := app.W.AdeTask.DetailsFolder(ctx, adewire.DiskChangesArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	notes, err := filepath.Glob(filepath.Join(where.Path, "notes", "*.md"))
	if err != nil || len(notes) != 2 {
		t.Fatalf("note files = %v, %v", notes, err)
	}
	// The newest note is free to edit: the edit that waits for the user is on the older one.
	older := notes[0]
	if raw, err := os.ReadFile(older); err != nil || string(raw) != "Check the error states." {
		older = notes[1]
	}
	const changed = "Check the error states and the empty form."
	if err := os.WriteFile(older, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join("notes", filepath.Base(older))
	reply, err := app.W.AdeBoard.Submit(ctx, agentmcp.SpaceCall{TaskID: task.ID, SessionID: "session-1"},
		agentmcp.SubmitArgs{Paths: []string{rel}, Reason: "the empty form case is missing"})
	if err != nil || !strings.HasPrefix(reply, "accepted 0, pending user 1, invalid 0") {
		t.Fatalf("submit = %q, %v", reply, err)
	}
	id := approvals.KindDetailsEdit + ":" + task.ID

	st := serve(t, app)
	p := newPhone(t, st.AppURL)
	deviceID := p.pair(app, "Ana's phone")
	if err := app.W.Mobile.SetDevicePermissions(bridge.MobileDevicePermissionsArgs{ID: deviceID, AgentInput: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.Mobile.SetAgentInputEnabled(bridge.MobileSetAgentInputArgs{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	r := p.get("/api/approvals")
	var listed []approvals.Item
	r.json(t, &listed)
	if got := apIDs(listed); !slices.Equal(got, []string{id}) {
		t.Fatalf("phone approvals = %v, want [%s]", got, id)
	}
	if strings.Contains(string(r.Body), app.Work) {
		t.Fatalf("phone approvals leak a path: %s", r.Body)
	}
	app.Contract(t, approvalsScenario, "http:GET /api/approvals#detailsEdit", r.raw(t), apItemMask)

	r, res := apAnswer(t, p, []string{id}, true)
	if r.Status != http.StatusOK || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("approve = %d %s", r.Status, r.Body)
	}
	app.Contract(t, approvalsScenario, "http:POST /api/approvals/answer#detailsEdit", r.raw(t))
	if apListed(t, app, id) {
		t.Fatalf("%s still listed after the phone approved", id)
	}
	raw, err := os.ReadFile(notes[0])
	if err != nil || string(raw) != changed {
		t.Fatalf("note after the phone approved = %q, %v", raw, err)
	}
}
