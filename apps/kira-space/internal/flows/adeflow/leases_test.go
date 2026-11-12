package adeflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P620 resource leases. Contract ade-leases: tests/ui/ade-v2-leases.spec.ts reads the queue
// snapshots of the same scenario: queued, granted, forced, ended.

const leaseScenario = "ade-leases"

var leaseMask = flowharness.Mask("sessionId", "taskId", "since", "expiresAt")

func leaseCall(tool string) fakeagent.MCPCall {
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: tool, Args: map[string]any{"queue": "push"}}
}

func leaseResult(t *testing.T, app *flowharness.App, name string) agentmcp.AcquireLeaseResult {
	t.Helper()
	var raw []byte
	testx.WaitUntil(t, waitFor, func() bool {
		var err error
		raw, err = os.ReadFile(filepath.Join(app.FakeDir, name))
		return err == nil && json.Valid(raw)
	})
	var res agentmcp.AcquireLeaseResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, raw)
	}
	return res
}

func leaseSnapshot(t *testing.T, app *flowharness.App) adewire.LeaseSnapshot {
	t.Helper()
	snap, err := app.W.Leases.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func leaseTitles(es []adewire.LeaseEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.TaskTitle)
	}
	return out
}

func waitHolder(t *testing.T, app *flowharness.App, want string) adewire.LeaseQueue {
	t.Helper()
	var q adewire.LeaseQueue
	testx.WaitUntil(t, waitFor, func() bool {
		q = leaseSnapshot(t, app).Queues[0]
		if want == "" {
			return q.Holder == nil
		}
		return q.Holder != nil && q.Holder.TaskTitle == want
	})
	return q
}

func waitPrompt(t *testing.T, app *flowharness.App, name, sub string) {
	t.Helper()
	testx.WaitUntil(t, waitFor, func() bool {
		raw, _ := os.ReadFile(filepath.Join(app.FakeDir, name))
		return strings.Contains(string(raw), sub)
	})
}

func TestLeases(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	// Fake calls 1 to 3 are the sessions of C, B and A, started in that order. C releases once
	// relC exists, B calls acquire again once relB exists.
	claude(app, map[string][]fakeagent.Action{"*": {
		{MCP: []fakeagent.MCPCall{leaseCall("acquire_lease")}, WaitFile: flag("relC"), MCPAfter: []fakeagent.MCPCall{leaseCall("release_lease")}, Name: "sleep"},
		{MCP: []fakeagent.MCPCall{leaseCall("acquire_lease")}, WaitFile: flag("relB"), MCPAfter: []fakeagent.MCPCall{leaseCall("acquire_lease")}, Name: "sleep"},
		{MCP: []fakeagent.MCPCall{leaseCall("acquire_lease")}, Name: "sleep"},
	}})
	t.Cleanup(func() {
		for _, n := range []string{"relC", "relB"} {
			_ = os.WriteFile(flag(n), nil, 0o644)
		}
	})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	// A sits on today for a day, B started yesterday for three, C is on Later: the list order is A,
	// B, C. Plain day order would put B first; B's span ends after A's, so A leads.
	today := time.Now().Local().Format("2006-01-02")
	yesterday := time.Now().Local().AddDate(0, 0, -1).Format("2006-01-02")
	mk := func(title string, day *string, est string) adewire.Task {
		tk, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: title, CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow", Day: day})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: tk.ID, Patch: adewire.TaskPatch{Est: &est}}); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	taskA := mk("Task A", &today, "1d")
	taskB := mk("Task B", &yesterday, "3d")
	taskC := mk("Task C", nil, "1d")
	if got, want := ade.TaskListOrder(board(t, app), model.DefaultAdeSettings(), today), []string{taskA.ID, taskB.ID, taskC.ID}; !slices.Equal(got, want) {
		t.Fatalf("task list order %v, want %v", got, want)
	}
	if err := app.W.Leases.SaveQueues(ctx, adewire.LeaseQueuesArgs{Names: []string{"push"}}); err != nil {
		t.Fatal(err)
	}

	launch := func(task adewire.Task, branch string) adewire.Launch {
		row := branchOf(t, board(t, app), task.ID, rec.ID)
		br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: branch})
		if err != nil {
			t.Fatal(err)
		}
		l, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID})
		if err != nil {
			t.Fatal(err)
		}
		openLaunch(t, app, l)
		waitSession(t, app, l.SessionID, "running")
		return l
	}

	// 1. C asks first and holds; B then A queue, and A ranks ahead of B whatever the arrival.
	launch(taskC, "feat/c")
	if r := leaseResult(t, app, "mcp-acquire_lease-1.json"); r.State != "held" {
		t.Fatalf("C acquire = %+v, want held", r)
	}
	lb := launch(taskB, "feat/b")
	if r := leaseResult(t, app, "mcp-acquire_lease-2.json"); r.State != "queued" || r.Position != 1 {
		t.Fatalf("B acquire = %+v, want queued at 1", r)
	}
	la := launch(taskA, "feat/a")
	if r := leaseResult(t, app, "mcp-acquire_lease-3.json"); r.State != "queued" || r.Position != 1 {
		t.Fatalf("A acquire = %+v, want queued at 1", r)
	}
	q := waitHolder(t, app, "Task C")
	if got := leaseTitles(q.Waiters); !slices.Equal(got, []string{"Task A", "Task B"}) {
		t.Fatalf("waiters %v, want [Task A Task B]", got)
	}
	app.Contract(t, leaseScenario, "LeasesService.Snapshot#queued", leaseSnapshot(t, app), leaseMask)

	// 2. B asks again: same place.
	if err := os.WriteFile(flag("relB"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if r := leaseResult(t, app, "mcp-acquire_lease-4.json"); r.State != "queued" || r.Position != 2 {
		t.Fatalf("B second acquire = %+v, want queued at 2", r)
	}

	// 3. A is idle, C releases: A is granted and the grant is typed into its terminal.
	stop := func(l adewire.Launch) string {
		return hook(t, app, l.TerminalID, `{"hook_event_name":"Stop","session_id":"s","cwd":"`+l.Cwd+`"}`)
	}
	submit := func(l adewire.Launch) string {
		return hook(t, app, l.TerminalID, `{"hook_event_name":"UserPromptSubmit","session_id":"s","cwd":"`+l.Cwd+`","prompt":"go"}`)
	}
	if out := stop(la); out != "" {
		t.Fatalf("Stop reply %q with nothing pending", out)
	}
	if err := os.WriteFile(flag("relC"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitHolder(t, app, "Task A")
	waitPrompt(t, app, "api-3.prompt", "you now hold lease `push`")
	app.Contract(t, leaseScenario, "LeasesService.Snapshot#granted", leaseSnapshot(t, app), leaseMask)

	// 4. B is mid-turn, so nothing is typed. The user force releases A: B is granted and its next
	// Stop carries the grant as a block reply; A hears it was revoked on its own next Stop.
	submit(lb)
	if err := app.W.Leases.ForceRelease(ctx, adewire.LeaseQueueArgs{Queue: "push"}); err != nil {
		t.Fatal(err)
	}
	q = waitHolder(t, app, "Task B")
	if !q.Holder.NoticePending {
		t.Fatal("B's grant is not marked pending while it is busy")
	}
	app.Contract(t, leaseScenario, "LeasesService.Snapshot#forced", leaseSnapshot(t, app), leaseMask)
	// A took the grant as a turn, so the revoked notice waits for its Stop.
	if out := stop(la); !strings.Contains(out, "revoked your lease `push`") {
		t.Fatalf("A's Stop reply %q, want the revoked notice", out)
	}
	reply := stop(lb)
	var block struct{ Decision, Reason string }
	if err := json.Unmarshal([]byte(reply), &block); err != nil || block.Decision != "block" || !strings.Contains(block.Reason, "you now hold lease `push`") {
		t.Fatalf("Stop reply %q, want a block carrying the grant", reply)
	}
	if out := stop(lb); out != "" {
		t.Fatalf("second Stop reply %q, want none", out)
	}
	if leaseSnapshot(t, app).Queues[0].Holder.NoticePending {
		t.Fatal("B's notice still pending after the reply")
	}
	if raw, _ := os.ReadFile(filepath.Join(app.FakeDir, "api-2.prompt")); strings.Contains(string(raw), "you now hold") {
		t.Fatalf("grant typed into busy B: %q", raw)
	}

	// 5. Request release: B is idle, so the finish-up message is typed at once.
	if err := app.W.Leases.RequestRelease(ctx, adewire.LeaseQueueArgs{Queue: "push"}); err != nil {
		t.Fatal(err)
	}
	waitPrompt(t, app, "api-2.prompt", "asks you to release lease `push`")
	if !leaseSnapshot(t, app).Queues[0].Holder.ReleaseAsked {
		t.Fatal("holder not marked ReleaseAsked")
	}

	// 6. B dies hard: no hook, no courtesy release. Reconcile frees the queue.
	raw, err := os.ReadFile(filepath.Join(app.FakeDir, "api-2.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, lb.SessionID, "stopped")
	waitHolder(t, app, "")
	app.Contract(t, leaseScenario, "LeasesService.Snapshot#ended", leaseSnapshot(t, app), leaseMask)
}
