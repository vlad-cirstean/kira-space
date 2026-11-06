package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P546 panel edit forward. Contract ade-panel-forward: tests/ui/ade-v2-panel-forward.spec.ts reads these.

const pfScenario = "ade-panel-forward"

const pfSteps = "  - id: design\n    name: Design\n    kind: manual\n    session: true\n" +
	"  - id: ship\n    name: Ship\n    kind: manual\n"

var pfMask = flowharness.Mask("at", "key", "sessionId", "taskId")

func pfOffer(t *testing.T, app *flowharness.App, taskID string) adewire.PanelForward {
	t.Helper()
	got, err := app.W.AdeTask.PanelForward(ctx, adewire.TaskArgs{TaskID: taskID})
	if err != nil {
		t.Fatalf("PanelForward: %v", err)
	}
	return got
}

func pfPrompts(t *testing.T, app *flowharness.App) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(app.FakeDir, "*.prompt"))
	var sb strings.Builder
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		sb.Write(raw)
	}
	return sb.String()
}

func TestPanelForward(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	f := newRunFixture(t, []fakeagent.Action{{WaitFile: hold}, {WaitFile: hold}}, pfSteps)
	app, taskID := f.app, f.taskID
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	task := adewire.TaskArgs{TaskID: taskID}

	// 1. No agent runs: edits record nothing and a forward refuses.
	body := "alpha\nbravo\ncharlie\ndelta\necho\nfoxtrot\ngolf\nSENTINEL-FAR\n"
	note, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: taskID, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if got := pfOffer(t, app, taskID); len(got.Items) != 0 {
		t.Fatalf("offer without an agent = %+v, want no items", got)
	}
	if _, err := app.W.AdeTask.ForwardPanelEdits(ctx, task); err == nil || !strings.Contains(err.Error(), "No agent") {
		t.Fatalf("ForwardPanelEdits without an agent = %v, want the noAgent refusal", err)
	}

	// 2. An interactive session runs in the step: the net change of each item is offered.
	launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, launch)
	waitSession(t, app, launch.SessionID, "running")

	svc := app.W.ProofStore
	proofID := closedProof(t, svc, "Checks", "table")
	for _, b := range []string{"alpha\nbravo EDIT\ncharlie\ndelta\necho\nfoxtrot\ngolf\nSENTINEL-FAR\n", "alpha\nbravo final\ncharlie\ndelta\necho\nfoxtrot\ngolf\nSENTINEL-FAR\n"} {
		if err := app.W.AdeTask.UpdateHandover(ctx, adewire.UpdateHandoverArgs{ID: note.ID, Body: b}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: taskID, Text: "Runbook https://wiki.example/runbook"}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.AdeTask.LinkProof(ctx, adewire.LinkProofArgs{TaskID: taskID, ProofID: proofID}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.AdeTask.UnlinkProof(ctx, adewire.LinkProofArgs{TaskID: taskID, ProofID: proofID}); err != nil {
		t.Fatal(err)
	}
	title := "Fix login form"
	if _, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: taskID, Patch: adewire.TaskPatch{Title: &title}}); err != nil {
		t.Fatal(err)
	}

	offer := pfOffer(t, app, taskID)
	kinds := make([]string, len(offer.Items))
	for i, it := range offer.Items {
		kinds[i] = it.Kind
	}
	if strings.Join(kinds, ",") != "note,link,field" || len(offer.Targets) != 1 || offer.Targets[0].SessionID != launch.SessionID ||
		offer.Unreachable != 0 || offer.StepID != "design" {
		t.Fatalf("offer = %+v, want note, link, field (the proof round trip is net empty) and one target", offer)
	}
	for _, want := range []string{"-bravo", "+bravo final", "+ Fix login form", "- Fix login", "https://wiki.example/runbook"} {
		if !strings.Contains(offer.Preview, want) {
			t.Fatalf("preview lacks %q:\n%s", want, offer.Preview)
		}
	}
	if strings.Contains(offer.Preview, "SENTINEL-FAR") || strings.Contains(offer.Preview, "EDIT") {
		t.Fatalf("preview carries unchanged or intermediate text:\n%s", offer.Preview)
	}
	hhmm := time.UnixMilli(note.CreatedAt).Format("15:04")
	pfClock := flowharness.Replace(hhmm, "12:00")
	app.Contract(t, pfScenario, "AdeTaskService.PanelForward#pending", offer, pfMask, pfClock)

	// 3. Forward: the agent gets exactly the preview, and the set clears.
	app.Contract(t, pfScenario, "args:AdeTaskService.ForwardPanelEdits", task)
	res, err := app.W.AdeTask.ForwardPanelEdits(ctx, task)
	if err != nil || len(res.Sent) != 1 || res.Sent[0] != launch.SessionID || len(res.Failed) != 0 {
		t.Fatalf("ForwardPanelEdits = %+v, %v, want the session in sent", res, err)
	}
	app.Contract(t, pfScenario, "AdeTaskService.ForwardPanelEdits", res, pfMask)
	want := strings.TrimSpace(offer.Preview)
	testx.WaitUntil(t, waitFor, func() bool { return strings.Contains(pfPrompts(t, app), want) })
	if got := pfOffer(t, app, taskID); len(got.Items) != 0 {
		t.Fatalf("offer after the forward = %+v, want none", got)
	}

	// 4. The agent ends: the set drops and a forward refuses.
	if _, err := app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: taskID, Text: "Docs https://wiki.example/docs"}); err != nil {
		t.Fatal(err)
	}
	if got := pfOffer(t, app, taskID); len(got.Items) != 1 {
		t.Fatalf("offer = %+v, want the docs link", got)
	}
	if err := app.W.AdeTask.CloseSession(ctx, adewire.CloseSessionArgs{SessionID: launch.SessionID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.ForwardPanelEdits(ctx, task); err == nil || !strings.Contains(err.Error(), "No agent") {
		t.Fatalf("ForwardPanelEdits after the agent ended = %v, want the noAgent refusal", err)
	}
	if got := pfOffer(t, app, taskID); len(got.Items) != 0 {
		t.Fatalf("offer after the agent ended = %+v, want none", got)
	}

	// 6. A step move drops what is pending.
	again, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, again)
	waitSession(t, app, again.SessionID, "running")
	title = "Fix login again"
	if _, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: taskID, Patch: adewire.TaskPatch{Title: &title}}); err != nil {
		t.Fatal(err)
	}
	if got := pfOffer(t, app, taskID); len(got.Items) != 1 {
		t.Fatalf("offer = %+v, want the title", got)
	}
	if _, err := app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: taskID, CloseSessions: true}); err != nil {
		t.Fatal(err)
	}
	if got := pfOffer(t, app, taskID); len(got.Items) != 0 || got.StepID != "" {
		t.Fatalf("offer after the step move = %+v, want none", got)
	}
}

// A background run takes no message: the edit is offered, the forward refuses, Dismiss clears it.
func TestPanelForwardHeadless(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	f := newRunFixture(t, []fakeagent.Action{{WaitFile: hold, Name: "done"}}, agentStep("one", "")+manualStep)
	app, taskID := f.app, f.taskID
	task := adewire.TaskArgs{TaskID: taskID}
	f.start(t, "feat/api-pf")
	run := waitRun(t, app, taskID, "one", "running")
	waitSession(t, app, run.SessionID, "running")

	added, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: taskID, Body: "use the new endpoint"})
	if err != nil {
		t.Fatal(err)
	}
	offer := pfOffer(t, app, taskID)
	if len(offer.Items) != 1 || len(offer.Targets) != 0 || offer.Unreachable != 1 {
		t.Fatalf("offer = %+v, want one item, no target and one unreachable run", offer)
	}
	clock := flowharness.Replace(time.UnixMilli(added.CreatedAt).Format("15:04"), "12:00")
	app.Contract(t, pfScenario, "AdeTaskService.PanelForward#headless", offer, pfMask, clock)
	if _, err := app.W.AdeTask.ForwardPanelEdits(ctx, task); err == nil || !strings.Contains(err.Error(), "Background runs") {
		t.Fatalf("ForwardPanelEdits = %v, want the unreachable refusal", err)
	}
	if got := pfOffer(t, app, taskID); len(got.Items) != 1 {
		t.Fatalf("offer after the refusal = %+v, want the set kept", got)
	}
	app.Contract(t, pfScenario, "args:AdeTaskService.DismissPanelEdits", task)
	if err := app.W.AdeTask.DismissPanelEdits(ctx, task); err != nil {
		t.Fatal(err)
	}
	if got := pfOffer(t, app, taskID); len(got.Items) != 0 {
		t.Fatalf("offer after Dismiss = %+v, want none", got)
	}
}
