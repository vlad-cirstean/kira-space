package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P407 stage handover. Contract ade-handover: tests/ui/ade-v2-handover.spec.ts reads these.

const hoScenario = "ade-handover"

var hoMask = flowharness.Mask("id", "taskId", "runId", "branchId", "sessionId", "startedAt", "finishedAt", "exitCode",
	"createdAt", "updatedAt", "addedAt", "repliedAt", "sentAt", "attempt")

func handoverPrompts(t *testing.T, app *flowharness.App) []prompts.Routed {
	t.Helper()
	all, err := app.W.PromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	var out []prompts.Routed
	for _, p := range all {
		if p.Kind == prompts.KindHandover {
			out = append(out, p)
		}
	}
	return out
}

func handoverCall(notes string, finished bool) fakeagent.MCPCall {
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "handover", Tolerate: true,
		Args: map[string]any{"notes": notes, "finished": finished}}
}

func handovers(t *testing.T, app *flowharness.App, taskID string) []adewire.Handover {
	t.Helper()
	res, err := app.W.AdeTask.Handovers(ctx, adewire.TaskArgs{TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	return res.Handovers
}

func TestStageHandover(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	// Call 1 is run `one`; 2 is the first work session; 3 a session that tries to write after the task moved on.
	claude(app, map[string][]fakeagent.Action{"*": {
		{Name: "done"},
		{MCP: []fakeagent.MCPCall{handoverCall("Form wired; validation missing.", true)}, WaitFile: flag("hold1")},
		{Sh: "until [ -f " + flag("late") + " ]; do sleep 0.1; done", MCP: []fakeagent.MCPCall{handoverCall("too late", true)}, WaitFile: flag("hold2")},
		{Name: "done"},
	}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")+
		"  - id: work\n    name: Work\n    kind: manual\n    status: in-progress\n    session: true\n"+
		agentStep("two", "")+
		"  - id: ship\n    name: Ship\n    kind: manual\n"))
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/api-work"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "one", "done")
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	// A finished run leaves its summary for the stage it moved to.
	hs := handovers(t, app, task.ID)
	if len(hs) != 1 || hs[0].Source != "run" || hs[0].FromStepID != "one" || hs[0].ToStepID != "work" || hs[0].Body != "summary-done" {
		t.Fatalf("after run one: %+v, want one handed run row one -> work", hs)
	}
	if in := taskOf(t, board(t, app), task.ID).HandoverIn; in == nil || in.Body != "summary-done" {
		t.Fatalf("board handoverIn = %+v, want the run summary", in)
	}

	// An interactive session writes a draft and says it is finished: the board offers the stage move.
	first, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, first)
	var draft adewire.Handover
	testx.WaitUntil(t, waitFor, func() bool {
		ds := taskOf(t, board(t, app), task.ID).HandoverDrafts
		if len(ds) == 1 {
			draft = ds[0]
		}
		return len(ds) == 1 && ds[0].Finished
	})
	if draft.State != "draft" || draft.Source != "agent" || draft.FromStepID != "work" || draft.Body != "Form wired; validation missing." {
		t.Fatalf("draft = %+v", draft)
	}
	if ps := handoverPrompts(t, app); len(ps) != 1 || ps[0].Ref != task.ID {
		t.Fatalf("prompts = %+v, want one handover prompt for the task", ps)
	}
	app.Contract(t, hoScenario, "AdeTaskService.Board#draft", board(t, app), hoMask)

	// Dismissing closes the prompt but keeps the draft.
	if err := app.W.AdeTask.DismissHandover(ctx, adewire.TaskArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if ps := handoverPrompts(t, app); len(ps) != 0 {
		t.Fatalf("prompts after dismiss = %+v", ps)
	}
	if ds := taskOf(t, board(t, app), task.ID).HandoverDrafts; len(ds) != 1 || !ds[0].Dismissed {
		t.Fatalf("drafts after dismiss = %+v, want the draft kept and dismissed", ds)
	}

	// Edit the draft; an empty body deletes it.
	if err := app.W.AdeTask.UpdateHandover(ctx, adewire.UpdateHandoverArgs{ID: draft.ID, Body: "Form wired."}); err != nil {
		t.Fatal(err)
	}
	if ds := taskOf(t, board(t, app), task.ID).HandoverDrafts; len(ds) != 1 || ds[0].Body != "Form wired." || !ds[0].Edited {
		t.Fatalf("drafts after edit = %+v", ds)
	}
	if err := app.W.AdeTask.UpdateHandover(ctx, adewire.UpdateHandoverArgs{ID: draft.ID, Body: ""}); err != nil {
		t.Fatal(err)
	}
	if ds := taskOf(t, board(t, app), task.ID).HandoverDrafts; len(ds) != 0 {
		t.Fatalf("drafts after delete = %+v", ds)
	}
	if err := os.WriteFile(flag("hold1"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, first.SessionID, "stopped")

	// A second session of the same stage, then the user moves the task with notes while it still runs.
	second, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, second)
	waitSession(t, app, second.SessionID, "running")
	notes := "Validation still missing; start with the error states."
	moved, err := app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: task.ID, Notes: &notes})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, hoScenario, "AdeTaskService.StepDone", moved, hoMask)
	if moved.EntryStepID != "two" || moved.HandoverIn == nil || moved.HandoverIn.Body != notes ||
		moved.HandoverIn.Source != "user" || moved.HandoverIn.FromStepID != "work" || moved.HandoverIn.ToStepID != "two" {
		t.Fatalf("after StepDone: entry %q, handoverIn %+v", moved.EntryStepID, moved.HandoverIn)
	}
	if len(moved.HandoverDrafts) != 0 {
		t.Fatalf("drafts after the move = %+v, want none", moved.HandoverDrafts)
	}
	hs = handovers(t, app, task.ID)
	app.Contract(t, hoScenario, "AdeTaskService.Handovers", adewire.HandoversResult{Handovers: hs}, hoMask)
	if len(hs) != 2 {
		t.Fatalf("handovers = %+v, want the run row and the stage move", hs)
	}

	// The session of the stage the task left cannot write any more.
	if err := os.WriteFile(flag("late"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		raw, err := os.ReadFile(filepath.Join(app.FakeDir, "mcp-handover-2.json"))
		return err == nil && strings.Contains(string(raw), "moved on to")
	})
	if ds := taskOf(t, board(t, app), task.ID).HandoverDrafts; len(ds) != 0 {
		t.Fatalf("stale write created drafts %+v", ds)
	}
	if err := os.WriteFile(flag("hold2"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// The next agent stage gets the handover and the task notes in its prompt.
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "two", "done")
	prompt := fakeFile(t, app, "api-4.prompt")
	if !strings.Contains(prompt, notes) || !strings.Contains(prompt, "Work") {
		t.Fatalf("stage two prompt lacks the handover from Work:\n%s", prompt)
	}

	// Moving a task back with notes records a user row; oversize notes refuse.
	big := strings.Repeat("x", agentmcp.MaxHandoverBytes+1)
	if _, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "one", Notes: &big}); err == nil {
		t.Fatal("SetTaskStep accepted oversize notes")
	}
	back := "Redo the form from scratch."
	if _, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "one", Notes: &back}); err != nil {
		t.Fatal(err)
	}
	if in := taskOf(t, board(t, app), task.ID).HandoverIn; in == nil || in.Body != back || in.ToStepID != "one" || in.Source != "user" {
		t.Fatalf("handoverIn after move back = %+v", in)
	}
}
