package adeflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/internal/flowtest"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Details auto-update: any save in a task folder pushes the board for that task, so the Details panel
// and its proofs refetch with no focus or Refresh. With no agent run an idle watcher pushes and
// applies nothing; an agent's accepted edit of the newest proof raises no proof event, only the board
// push. Contract ade-details-autoupdate: tests/ui/ade-v2-details-autoupdate.spec.ts reads these and
// asserts the refresh through the UI.

const (
	duScenario  = "ade-details-autoupdate"
	duQuiet     = 300 * time.Millisecond // the watch debounce
	boardWindow = 30 * time.Millisecond  // the board push coalescing window
)

func duMask(taskID string) []flowharness.ContractOption {
	return []flowharness.ContractOption{dfMask, flowharness.Mask("closedAt"), flowharness.Replace(taskID, "T_alerts")}
}

// duNamesTask matches a board push that names the task and has no refs change.
func duNamesTask(t *testing.T, taskID string) func(flowtest.Event) bool {
	return func(ev flowtest.Event) bool {
		var p adewire.BoardChangedEvent
		ev.Decode(t, &p)
		return !p.Refs && slices.Contains(p.TaskIDs, taskID)
	}
}

func duNamesTaskSince(t *testing.T, app *flowharness.App, mark int, taskID string) bool {
	pred := duNamesTask(t, taskID)
	for _, ev := range app.Events.Since(mark, adewire.ChannelBoard) {
		if pred(ev) {
			return true
		}
	}
	return false
}

// A save with no agent run: the idle watcher pushes the board, applies nothing.
func TestDetailsAutoUpdateIdleSave(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", onceStepYML))
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	opts := duMask(task.ID)
	folder := dfFolder(t, app, task.ID)
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: "First note."}); err != nil {
		t.Fatal(err)
	}
	notes := dfNotes(t, folder)
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want one", notes)
	}
	noteRel := notes[0]

	// The watcher starts after the app's startup migration: save until a push shows it is up.
	n := 0
	testx.WaitUntil(t, waitFor, func() bool {
		mark := app.Events.Mark()
		n++
		dfWrite(t, folder, noteRel, "Probe "+string(rune('a'+n%26)))
		time.Sleep(3 * duQuiet)
		return duNamesTaskSince(t, app, mark, task.ID)
	})

	mark := app.Events.Mark()
	dfWrite(t, folder, noteRel, "Edited outside.")
	ev := app.Events.WaitAfter(t, mark, adewire.ChannelBoard, duNamesTask(t, task.ID), waitFor)
	app.ContractEvent(t, duScenario, ev, opts...)

	disk := dfDisk(t, app, task.ID)
	app.Contract(t, duScenario, "AdeTaskService.DetailsDiskChanges#idle", disk, opts...)
	var seen bool
	for _, c := range disk {
		seen = seen || c.Path == noteRel && c.State == "edited" && !c.Pending
	}
	if !seen {
		t.Fatalf("disk changes = %+v, want %s edited", disk, noteRel)
	}
	if got := dfRead(t, folder, noteRel); got != "Edited outside." {
		t.Fatalf("note file = %q, want it untouched: the idle watcher applies nothing", got)
	}

	// A proof folder written by hand pushes too.
	mark = app.Events.Mark()
	writeProof(t, folder, "outside", "Outside", "table")
	app.Events.WaitAfter(t, mark, adewire.ChannelBoard, duNamesTask(t, task.ID), waitFor)

	// Kira's own state.yaml is no task file: no push for it.
	time.Sleep(3 * duQuiet)
	mark = app.Events.Mark()
	dfWrite(t, folder, "state.yaml", "touched: true\n")
	time.Sleep(3 * duQuiet)
	if duNamesTaskSince(t, app, mark, task.ID) {
		t.Fatal("a state.yaml save pushed the board")
	}
}

// An agent's accepted edit of the newest proof raises one proof event and the board push; the rev bumps.
// The Details panel refetches the proofs, whose new rev moves each open step; an open proof window checks its version.
func TestDetailsAutoUpdateAgentProofEdit(t *testing.T) {
	gate := dwHold(t)
	f := newBRFixture(t, []fakeagent.Action{{WaitFile: gate, Name: "done"}}, onceStepYML, false)
	app := f.app
	opts := duMask(f.task.ID)
	folder := dfFolder(t, app, f.task.ID)
	f.startRun(t)
	waitRun(t, app, f.task.ID, "one", "running")
	writeProof(t, folder, "second", "Second", "diagram", "table")
	testx.WaitUntil(t, waitFor, func() bool {
		for _, p := range taskProofs(t, app, f.task.ID) {
			if p.Slug == "second" && len(p.Steps) == 2 && !p.Pending {
				return true
			}
		}
		return false
	})

	// The new proof raises its own proof event; let it land so the mark sees only the edit.
	app.Events.Wait(t, proof.ChannelChanged, nil, waitFor)
	time.Sleep(2 * duQuiet)
	mark := app.Events.Mark()
	writeProof(t, folder, "second", "Second", "table")
	testx.WaitUntil(t, waitFor, func() bool {
		for _, p := range taskProofs(t, app, f.task.ID) {
			if p.Slug == "second" && len(p.Steps) == 1 && !p.Pending {
				return true
			}
		}
		return false
	})
	ev := app.Events.WaitAfter(t, mark, adewire.ChannelBoard, duNamesTask(t, f.task.ID), waitFor)
	app.Contract(t, duScenario, "event:kira:adetask:board#proofEdit", ev.Data, opts...)
	if got := app.Events.Since(mark, proof.ChannelChanged); len(got) != 1 {
		t.Fatalf("proof events = %+v, want one for an accepted edit", got)
	}
	app.Contract(t, duScenario, "AdeTaskService.TaskProofs#edited", adewire.TaskProofsResult{Proofs: taskProofs(t, app, f.task.ID)}, opts...)
	_ = os.WriteFile(gate, nil, 0o644)
	waitRun(t, app, f.task.ID, "one", "done")
}

// P758: a link, note or handover write pushes only its own task and no refs change, so another task's
// Details and every PR fact stay as they are. Contract ade-details-autoupdate (#scopedPush).
func TestDetailsScopedPushes(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	task := createTask(t, app, "Fix login", "", rec.ID)
	opts := duMask(task.ID)

	// push runs write and checks every board push it raised: each names only the task, refs false.
	push := func(name string, write func()) []flowtest.Event {
		t.Helper()
		time.Sleep(3 * boardWindow) // let earlier pushes land
		mark := app.Events.Mark()
		write()
		app.Events.WaitAfter(t, mark, adewire.ChannelBoard, duNamesTask(t, task.ID), waitFor)
		time.Sleep(3 * boardWindow)
		evs := app.Events.Since(mark, adewire.ChannelBoard)
		for _, ev := range evs {
			var p adewire.BoardChangedEvent
			ev.Decode(t, &p)
			if p.Refs || len(p.TaskIDs) != 1 || p.TaskIDs[0] != task.ID {
				t.Fatalf("%s pushed %s, want only task %s with refs false", name, ev.Data, task.ID)
			}
		}
		return evs
	}

	var note adewire.Handover
	evs := push("AddNote", func() {
		var err error
		if note, err = app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: "A note with https://notes.example/a in it."}); err != nil {
			t.Fatal(err)
		}
	})
	app.Contract(t, duScenario, "event:kira:adetask:board#scopedPush", evs[0].Data, opts...)
	push("UpdateHandover", func() {
		if err := app.W.AdeTask.UpdateHandover(ctx, adewire.UpdateHandoverArgs{ID: note.ID, Body: "Edited https://notes.example/a"}); err != nil {
			t.Fatal(err)
		}
	})
	var link adewire.LinkResult
	push("AddLink", func() {
		var err error
		if link, err = app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: task.ID, Text: "Runbook https://wiki.example/runbook"}); err != nil {
			t.Fatal(err)
		}
	})
	push("EditLink", func() {
		if _, err := app.W.AdeTask.EditLink(ctx, adewire.EditLinkArgs{TaskID: task.ID, Slot: "stored", ID: link.Artifact.ID, Text: "https://wiki.example/runbook-v2"}); err != nil {
			t.Fatal(err)
		}
	})
	push("RemoveArtifact", func() {
		if err := app.W.AdeTask.RemoveArtifact(ctx, adewire.RemoveArtifactArgs{TaskID: task.ID, ID: link.Artifact.ID}); err != nil {
			t.Fatal(err)
		}
	})
	push("DismissFoundLink", func() {
		if err := app.W.AdeTask.DismissFoundLink(ctx, adewire.DismissFoundLinkArgs{TaskID: task.ID, URL: "https://notes.example/a"}); err != nil {
			t.Fatal(err)
		}
	})
}
