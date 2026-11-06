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

// P543 unified notes. Contract ade-notes: tests/ui/ade-v2-notes.spec.ts reads these.

const notesScenario = "ade-notes"

var notesMask = flowharness.Mask("id", "taskId", "runId", "branchId", "sessionId", "startedAt", "finishedAt", "exitCode",
	"createdAt", "updatedAt", "addedAt", "repliedAt", "sentAt", "attempt")

func states(hs []adewire.Handover) string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.State + "/" + h.Source
	}
	return strings.Join(out, " ")
}

func TestUnifiedNotes(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	// The Design session runs in the task folder (work), the Build run in the repo (api).
	claude(app, map[string][]fakeagent.Action{
		"work": {{MCP: []fakeagent.MCPCall{handoverCall("Form wired.\nValidation missing.", true)}, WaitFile: flag("hold")}},
		"api":  {{Name: "done"}},
	})
	t.Cleanup(func() { _ = os.WriteFile(flag("hold"), nil, 0o644) })
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow",
		"  - id: design\n    name: Design\n    kind: manual\n    session: true\n"+
			agentStep("build", "")+
			"  - id: ship\n    name: Ship\n    kind: manual\n"))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	first := "Check the error states."
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{
		Title: "Login form", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow", Notes: first,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Notes != first {
		t.Fatalf("created task notes = %q, want the first note", task.Notes)
	}
	if hs := handovers(t, app, task.ID); len(hs) != 1 || hs[0].State != "note" || hs[0].Source != "user" || hs[0].Body != first || hs[0].FromStepID != "design" {
		t.Fatalf("chain after create = %+v, want one user note during design", hs)
	}

	second := "Use the **new** API."
	added, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: second})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, notesScenario, "AdeTaskService.AddNote", added, notesMask)
	if added.State != "note" || added.Source != "user" || added.FromStepID != "design" || added.FromStepName != "Design" || added.ToStepID != "" || added.Body != second {
		t.Fatalf("AddNote = %+v", added)
	}

	// The agent records a draft; the user moves the task and the draft becomes a handed row.
	launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, launch)
	testx.WaitUntil(t, waitFor, func() bool {
		ds := taskOf(t, board(t, app), task.ID).HandoverDrafts
		return len(ds) == 1 && ds[0].Finished
	})
	if len(taskOf(t, board(t, app), task.ID).HandoverDrafts) != 1 || taskOf(t, board(t, app), task.ID).HandoverIn != nil {
		t.Fatal("notes must not count as a handed handover")
	}
	if _, err := app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(flag("hold"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, launch.SessionID, "stopped")

	third := "Ship on Friday."
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: third}); err != nil {
		t.Fatal(err)
	}
	hs := handovers(t, app, task.ID)
	app.Contract(t, notesScenario, "AdeTaskService.Handovers#chain", adewire.HandoversResult{Handovers: hs}, notesMask)
	if got, want := states(hs), "note/user handed/agent note/user note/user"; got != want {
		t.Fatalf("chain = %q, want %q", got, want)
	}
	if hs[1].Body != "Form wired.\nValidation missing." || hs[1].FromStepID != "design" || hs[1].ToStepID != "build" {
		t.Fatalf("handed row = %+v", hs[1])
	}
	wantNotes := strings.Join([]string{third, second, first}, "\n\n")
	tk := taskOf(t, board(t, app), task.ID)
	app.Contract(t, notesScenario, "AdeTaskBoard.Board#notes", board(t, app), notesMask)
	if tk.Notes != wantNotes {
		t.Fatalf("task notes = %q, want %q", tk.Notes, wantNotes)
	}
	if tk.HandoverIn == nil || tk.HandoverIn.State != "handed" {
		t.Fatalf("handoverIn = %+v, want the handed row", tk.HandoverIn)
	}

	// The next stage's prompt carries the notes and the handover.
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/login"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "build", "done")
	prompt := fakeFile(t, app, "api-1.prompt")
	for _, want := range []string{wantNotes, "Form wired.", "Validation missing."} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("build prompt lacks %q:\n%s", want, prompt)
		}
	}

	// Editing a note keeps its place; an empty body deletes it.
	idsOf := func(hs []adewire.Handover) string {
		out := make([]string, len(hs))
		for i, h := range hs {
			out[i] = h.ID
		}
		return strings.Join(out, ",")
	}
	orderBefore := idsOf(handovers(t, app, task.ID))
	if err := app.W.AdeTask.UpdateHandover(ctx, adewire.UpdateHandoverArgs{ID: added.ID, Body: "Use the new API v2."}); err != nil {
		t.Fatal(err)
	}
	hs = handovers(t, app, task.ID)
	var edited adewire.Handover
	for _, h := range hs {
		if h.ID == added.ID {
			edited = h
		}
	}
	if !edited.Edited || edited.Body != "Use the new API v2." || edited.CreatedAt != added.CreatedAt {
		t.Fatalf("edited note = %+v", edited)
	}
	if idsOf(hs) != orderBefore {
		t.Fatalf("order changed after edit: %q", states(hs))
	}

	// Limits: a note may reach 1 MiB, handover rows stay at 16 KiB.
	long := strings.Repeat("x", 17*1024)
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: long}); err != nil {
		t.Fatalf("17 KiB note refused: %v", err)
	}
	var handed adewire.Handover
	for _, h := range handovers(t, app, task.ID) {
		if h.State == "handed" {
			handed = h
		}
	}
	if err := app.W.AdeTask.UpdateHandover(ctx, adewire.UpdateHandoverArgs{ID: handed.ID, Body: long}); err == nil {
		t.Fatal("17 KiB edit of a handed row accepted")
	}
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: strings.Repeat("x", 1<<20+1)}); err == nil {
		t.Fatal("note over 1 MiB accepted")
	}
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: " \n"}); err == nil {
		t.Fatal("blank note accepted")
	}
	if err := app.W.AdeTask.UpdateHandover(ctx, adewire.UpdateHandoverArgs{ID: added.ID, Body: ""}); err != nil {
		t.Fatal(err)
	}
	for _, h := range handovers(t, app, task.ID) {
		if h.ID == added.ID {
			t.Fatal("emptied note still listed")
		}
	}

	// An archived task takes no new note and keeps the derived notes.
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: "late"}); err == nil {
		t.Fatal("archived task accepted a note")
	}
	arch, err := app.W.AdeTask.ArchivedTask(ctx, adewire.TaskArgs{TaskID: task.ID})
	if err != nil || !strings.Contains(arch.Task.Notes, first) {
		t.Fatalf("archived notes = %q, err %v", arch.Task.Notes, err)
	}
}
