package adeflow_test

import (
	"fmt"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// check sets its own status; later and gate set none, so the step and review defaults apply.
const statusFlow = "  - id: check\n    name: Check\n    kind: manual\n    status: in-review\n    session: true\n" +
	"  - id: later\n    name: Later\n    kind: manual\n    session: true\n" +
	"  - id: gate\n    name: Gate\n    kind: review\n    runs_on: each repo\n    timeout: 1m\n    review:\n      send_to: new\n"

func TestTaskStatuses(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("implement", "")+statusFlow)
	app := f.app
	state := func(id string) (string, string) {
		tk := taskOf(t, board(t, app), id)
		return tk.StatusID, tk.StatusBy
	}
	expect := func(id, status, by string) {
		t.Helper()
		if s, b := state(id); s != status || b != by {
			t.Fatalf("task %s status = %s/%s, want %s/%s", id, s, b, status, by)
		}
	}
	move := func(id, status string) (adewire.Task, error) {
		return app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: id, Patch: adewire.TaskPatch{StatusID: &status}})
	}
	step := func(id, stepID string) {
		t.Helper()
		if _, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: id, StepID: stepID}); err != nil {
			t.Fatalf("SetTaskStep %s: %v", stepID, err)
		}
	}

	b := board(t, app)
	if len(b.Statuses) != 4 || b.Statuses[0].ID != "todo" || b.Statuses[3].ID != "done" || b.Statuses[1].Color != "amber" {
		t.Fatalf("default statuses = %+v", b.Statuses)
	}
	if want := (adewire.StatusDefaults{New: "todo", Step: "in-progress", Review: "in-review", Finished: "done"}); b.StatusDefaults != want {
		t.Fatalf("default slots = %+v, want %+v", b.StatusDefaults, want)
	}
	expect(f.taskID, "todo", "auto")

	// Contract ade-task-statuses: tests/ui/ade-v2-task-status.spec.ts reads these.
	args := adewire.UpdateTaskArgs{TaskID: f.taskID, Patch: adewire.TaskPatch{StatusID: ptr("in-review")}}
	app.Contract(t, "ade-task-statuses", "args:AdeTaskService.UpdateTask", args)
	moved, err := app.W.AdeTask.UpdateTask(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if moved.StatusID != "in-review" || moved.StatusBy != "user" {
		t.Fatalf("moved = %s/%s, want in-review/user", moved.StatusID, moved.StatusBy)
	}
	app.Contract(t, "ade-task-statuses", "AdeTaskService.UpdateTask#moved", moved, taskMask)
	if _, err := move(f.taskID, "todo"); err != nil {
		t.Fatal(err)
	}
	expect(f.taskID, "todo", "user")
	if _, err := move(f.taskID, "nope"); err == nil {
		t.Fatal("unknown status id accepted")
	}

	step(f.taskID, "check")
	expect(f.taskID, "in-review", "auto")
	if _, err := move(f.taskID, "todo"); err != nil {
		t.Fatal(err)
	}
	expect(f.taskID, "todo", "user")
	expect(f.taskID, "todo", "user") // sticks across a board read
	step(f.taskID, "done")
	expect(f.taskID, "done", "auto")

	step(f.taskID, "implement")
	expect(f.taskID, "todo", "auto") // first step, nothing run: not started
	f.start(t, "feat/api-status")
	waitRun(t, app, f.taskID, "implement", "done")
	testx.WaitUntil(t, waitFor, func() bool { s, _ := state(f.taskID); return s == "in-review" })
	expect(f.taskID, "in-review", "auto") // the next step, check, sets its own status

	step(f.taskID, "later")
	expect(f.taskID, "in-progress", "auto") // step default
	step(f.taskID, "gate")
	expect(f.taskID, "in-review", "auto") // review default

	other := createTask(t, app, "Other", "flow", f.repoID)
	if _, err := move(other.ID, "in-progress"); err != nil {
		t.Fatal(err)
	}
	old := createTask(t, app, "Old", "flow", f.repoID)
	if _, err := move(old.ID, "in-progress"); err != nil {
		t.Fatal(err)
	}
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.ArchiveTaskArgs{TaskID: old.ID}); err != nil {
		t.Fatal(err)
	}
	archivedStatus := func() string {
		t.Helper()
		got, err := app.W.AdeTask.ArchivedTask(ctx, adewire.TaskArgs{TaskID: old.ID})
		if err != nil {
			t.Fatal(err)
		}
		return got.Task.StatusID
	}

	cur := func() adewire.SaveStatusesArgs {
		b := board(t, app)
		return adewire.SaveStatusesArgs{Statuses: append([]adewire.TaskStatusDef(nil), b.Statuses...), Defaults: b.StatusDefaults}
	}
	save := func(a adewire.SaveStatusesArgs) (adewire.SaveStatusesResult, error) {
		return app.W.AdeTask.SaveStatuses(ctx, a)
	}
	mustSave := func(a adewire.SaveStatusesArgs) adewire.SaveStatusesResult {
		t.Helper()
		res, err := save(a)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// Rename and recolour change display only; every task keeps its status id.
	rename := cur()
	rename.Statuses[1].Name, rename.Statuses[1].Color = "Doing", "orange"
	app.Contract(t, "ade-task-statuses", "args:AdeTaskService.SaveStatuses", rename)
	renamed := mustSave(rename)
	app.Contract(t, "ade-task-statuses", "AdeTaskService.SaveStatuses#saved", renamed)
	if renamed.Statuses[1].ID != "in-progress" || renamed.Statuses[1].Name != "Doing" || renamed.Statuses[1].Color != "orange" {
		t.Fatalf("renamed = %+v", renamed.Statuses)
	}
	expect(f.taskID, "in-review", "auto")
	expect(other.ID, "in-progress", "user")
	if got := archivedStatus(); got != "in-progress" {
		t.Fatalf("archived status = %s, want in-progress", got)
	}

	// A new default applies at the next automatic moment only.
	slot := cur()
	slot.Defaults.Review = "done"
	mustSave(slot)
	expect(f.taskID, "in-review", "auto")
	step(f.taskID, "later")
	expect(f.taskID, "in-progress", "auto")
	step(f.taskID, "gate")
	expect(f.taskID, "done", "auto")

	// An empty finish default keeps the current status.
	keep := cur()
	keep.Defaults.Finished = ""
	mustSave(keep)
	if _, err := move(f.taskID, "in-review"); err != nil {
		t.Fatal(err)
	}
	step(f.taskID, "done")
	expect(f.taskID, "in-review", "user")
	if got := board(t, app).StatusDefaults.Finished; got != "" {
		t.Fatalf("finished default = %q, want empty", got)
	}

	refuse := func(name string, a adewire.SaveStatusesArgs) {
		t.Helper()
		if _, err := save(a); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	noTarget := cur()
	noTarget.Statuses = append(noTarget.Statuses[:1], noTarget.Statuses[2:]...)
	noTarget.Defaults.Step = ""
	refuse("removing a status without a reassign target", noTarget)
	badTarget := noTarget
	badTarget.Reassign = map[string]string{"in-progress": "nope"}
	refuse("reassign to an unknown status", badTarget)
	noNew := cur()
	noNew.Defaults.New = ""
	refuse("no new-task default", noNew)
	unknownSlot := cur()
	unknownSlot.Defaults.Step = "nope"
	refuse("default naming an unknown status", unknownSlot)
	dup := cur()
	dup.Statuses[1].Name = "to do"
	refuse("duplicate name", dup)
	many := cur()
	for i := 0; i < 17; i++ {
		many.Statuses = append(many.Statuses, adewire.TaskStatusDef{Name: fmt.Sprintf("Extra %d", i), Color: "grey"})
	}
	refuse("21 statuses", many)
	badColor := cur()
	badColor.Statuses[0].Color = "mauve"
	refuse("unknown colour", badColor)

	// Delete: tasks move to the chosen status, archived ones too; a default naming it moves with it.
	toTodo := cur()
	toTodo.Statuses = append(toTodo.Statuses[:1], toTodo.Statuses[2:]...)
	toTodo.Reassign = map[string]string{"in-progress": "todo"}
	// The editor moves the defaults that name the removed status itself; the server also does it when
	// they arrive unmoved (toTodo below).
	sent := adewire.SaveStatusesArgs{
		Statuses: []adewire.TaskStatusDef{renamed.Statuses[0], renamed.Statuses[2], renamed.Statuses[3]},
		Defaults: renamed.Defaults,
		Reassign: toTodo.Reassign,
	}
	sent.Defaults.Step = "todo"
	app.Contract(t, "ade-task-statuses", "args:AdeTaskService.SaveStatuses#delete", sent)
	deleted := mustSave(toTodo)
	if len(deleted.Statuses) != 3 || deleted.Defaults.Step != "todo" {
		t.Fatalf("deleted = %+v", deleted)
	}
	expect(other.ID, "todo", "user")
	if got := archivedStatus(); got != "todo" {
		t.Fatalf("archived status = %s, want todo", got)
	}
	expect(f.taskID, "in-review", "user")

	// A new status named like a removed one gets a fresh id.
	again := cur()
	again.Statuses = append(again.Statuses, adewire.TaskStatusDef{Name: "Doing", Color: "teal"})
	res := mustSave(again)
	if last := res.Statuses[len(res.Statuses)-1]; last.ID == "" || last.ID == "in-progress" {
		t.Fatalf("new status id = %q", last.ID)
	}
}

func ptr[T any](v T) *T { return &v }
