package adeflow_test

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const flatSpec = "  - id: spec\n    name: Spec\n    kind: manual\n    status: todo\n    session: true\n    prompt: Write the spec for {task}\n"

var taskMask = flowharness.Mask("id", "createdAt", "branchIds", "taskId", "branchId", "startedAt", "finishedAt", "sessionId", "exitCode", "updatedAt")

// flatFlow is a manual spec, an agent, a review and a script: one step of each kind.
func flatFlow() string {
	return flatSpec + agentStep("implement", "") + rvStep("implementer") + scriptStep("release", "'exit 0'")
}

func entryOf(t *testing.T, f *runFixture) adewire.Task {
	t.Helper()
	return taskOf(t, board(t, f.app), f.taskID)
}

// pastSpec ends the manual first step so the task waits on Run.
func pastSpec(t *testing.T, f *runFixture) {
	t.Helper()
	if _, err := f.app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: f.taskID}); err != nil {
		t.Fatalf("StepDone: %v", err)
	}
}

func TestFlatSteps(t *testing.T) {
	t.Run("a task walks every step kind to done", func(t *testing.T) {
		f := newRunFixture(t, []fakeagent.Action{{Name: "done", Sh: rvImplement}}, flatFlow())
		created := entryOf(t, f)
		if created.EntryStepID != "spec" || len(created.Runs) != 0 {
			t.Fatalf("created task = %+v, want entry spec and no runs", created)
		}
		f.app.Contract(t, "ade-flat-steps", "AdeTaskService.Task#created", created, taskMask)

		launch, err := f.app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.taskID})
		if err != nil {
			t.Fatalf("LaunchStep on the manual step: %v", err)
		}
		if launch.SessionID == "" || launch.TerminalID == "" {
			t.Fatalf("manual launch = %+v, want a session and a terminal", launch)
		}
		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.taskID}); err == nil {
			t.Fatal("StartRun accepted while the task waits on a manual step")
		}
		manual := entryOf(t, f)
		f.app.Contract(t, "ade-flat-steps", "AdeTaskService.Task#manual", manual, taskMask)

		done, err := f.app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: f.taskID})
		if err != nil || done.EntryStepID != "implement" {
			t.Fatalf("StepDone = %+v, %v, want entry implement", done, err)
		}
		if runs := stepRuns(t, f, "implement"); len(runs) != 0 {
			t.Fatalf("implement started by itself: %+v", runs)
		}

		f.start(t, "feat/api-flat")
		impl := waitRun(t, f.app, f.taskID, "implement", "done")
		f.app.Contract(t, "ade-flat-steps", "AdeTaskService.Run#implement-done", impl, runMask)

		rev := waitRun(t, f.app, f.taskID, "review", "review")
		f.app.Contract(t, "ade-flat-steps", "AdeTaskService.Run#review-waiting", rev, runMask)
		if err := f.app.W.AdeTask.FinishReview(ctx, adewire.FinishReviewArgs{RunID: rev.ID, Result: "approved", Note: "lgtm"}); err != nil {
			t.Fatalf("FinishReview: %v", err)
		}
		release := waitRun(t, f.app, f.taskID, "release", "done")
		f.app.Contract(t, "ade-flat-steps", "AdeTaskService.Run#release-done", release, runMask)

		testx.WaitUntil(t, waitFor, func() bool { return entryOf(t, f).EntryStepID == "done" })
		final := entryOf(t, f)
		f.app.Contract(t, "ade-flat-steps", "AdeTaskService.Task#done", final, taskMask)
	})

	t.Run("a task moves back and forward and refuses while a run is live", func(t *testing.T) {
		hold := t.TempDir() + "/hold"
		f := newRunFixture(t, []fakeagent.Action{{Name: "done", Sh: rvImplement}, {Name: "done", WaitFile: hold}}, flatFlow())
		pastSpec(t, f)
		f.start(t, "feat/api-moves")
		waitRun(t, f.app, f.taskID, "review", "review")
		first, _ := latestRun(t, f.app, f.taskID, "implement")

		moved, err := f.app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: f.taskID, StepID: "implement"})
		if err != nil || moved.EntryStepID != "implement" {
			t.Fatalf("SetTaskStep back = %+v, %v, want entry implement", moved, err)
		}
		if moved.Since["implement/"+first.BranchID] != first.Attempt {
			t.Fatalf("since = %v, want the finished attempt %d of implement", moved.Since, first.Attempt)
		}
		f.app.Contract(t, "ade-flat-steps", "AdeTaskService.Task#moved", moved, taskMask)
		if runs := stepRuns(t, f, "implement"); len(runs) != 1 {
			t.Fatalf("moving back deleted or started runs: %+v", runs)
		}

		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.taskID}); err != nil {
			t.Fatalf("StartRun after the move: %v", err)
		}
		waitRun(t, f.app, f.taskID, "implement", "running")
		if _, err := f.app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: f.taskID, StepID: "release"}); err == nil {
			t.Fatal("SetTaskStep accepted while a run is live")
		}
		if err := f.app.W.AdeTask.StopRun(ctx, adewire.RunArgs{RunID: waitRun(t, f.app, f.taskID, "implement", "running").ID}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.taskID, "implement", "stuck")

		if got, err := f.app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: f.taskID, StepID: "release"}); err != nil || got.EntryStepID != "release" {
			t.Fatalf("SetTaskStep forward = %+v, %v", got, err)
		}
		if got, err := f.app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: f.taskID, StepID: "done"}); err != nil || got.EntryStepID != "done" {
			t.Fatalf("SetTaskStep to done = %+v, %v", got, err)
		}
		if _, err := f.app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: f.taskID, StepID: "nope"}); err == nil {
			t.Fatal("SetTaskStep accepted an unknown step")
		}
	})

	t.Run("a return before the entry step moves the entry back", func(t *testing.T) {
		f := newRunFixture(t, []fakeagent.Action{{Name: "done", Sh: rvImplement}, {Name: "done"}}, flatFlow())
		pastSpec(t, f)
		f.start(t, "feat/api-return")
		waitRun(t, f.app, f.taskID, "review", "review")
		for _, to := range []string{"release", "review"} {
			if got, err := f.app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: f.taskID, StepID: to}); err != nil || got.EntryStepID != to {
				t.Fatalf("SetTaskStep to %s = %+v, %v", to, got, err)
			}
		}
		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.taskID}); err != nil {
			t.Fatalf("StartRun at review: %v", err)
		}
		var rev adewire.Run
		testx.WaitUntil(t, waitFor, func() bool {
			rev, _ = latestRun(t, f.app, f.taskID, "review")
			return rev.Attempt == 2 && rev.State == "review"
		})
		if err := f.app.W.AdeTask.FinishReview(ctx, adewire.FinishReviewArgs{RunID: rev.ID, Result: "rework", Note: "rename foo"}); err != nil {
			t.Fatalf("FinishReview rework: %v", err)
		}
		testx.WaitUntil(t, waitFor, func() bool { return entryOf(t, f).EntryStepID == "implement" })
		again := waitRun(t, f.app, f.taskID, "implement", "done")
		if again.Attempt != 2 {
			t.Fatalf("implement = %+v, want attempt 2 after the return", again)
		}
		waitRun(t, f.app, f.taskID, "review", "review")
		returned := entryOf(t, f)
		f.app.Contract(t, "ade-flat-steps", "AdeTaskService.Task#return", returned, taskMask)
		var back adewire.Run
		for _, r := range returned.Runs {
			if r.StepID == "review" && r.State == "back" {
				back = r
			}
		}
		if back.ID == "" {
			t.Fatalf("runs = %+v, want the first review run sent back", returned.Runs)
		}
	})

	t.Run("a workflow saves one step per kind and reloads equal", func(t *testing.T) {
		f := newRunFixture(t, nil, flatFlow())
		list, err := f.app.W.AdeTask.Workflows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var entry adewire.WorkflowEntry
		for _, e := range list.Workflows {
			if e.FileName == "flow.yaml" {
				entry = e
			}
		}
		wf := *entry.Workflow
		kinds := []adewire.StepKind{adewire.KindManual, adewire.KindAgent, adewire.KindReview, adewire.KindScript}
		if len(wf.Steps) != len(kinds) {
			t.Fatalf("steps = %d, want %d", len(wf.Steps), len(kinds))
		}
		for i, k := range kinds {
			if wf.Steps[i].Kind != k {
				t.Fatalf("step %d kind = %q, want %q", i, wf.Steps[i].Kind, k)
			}
		}
		wf.Steps[1].Name = "Build it"
		args := adewire.SaveWorkflowArgs{FileName: "flow.yaml", Workflow: wf, BaseHash: entry.Hash}
		saved, err := f.app.W.AdeTask.SaveWorkflow(ctx, args)
		if err != nil || saved.Error != nil {
			t.Fatalf("SaveWorkflow = %+v, %v", saved, err)
		}
		y, err := f.app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "flow.yaml"})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"steps:", "kind: manual", "kind: agent", "kind: review", "kind: script", "name: Build it"} {
			mustContain(t, "saved yaml", y.Yaml, want)
		}
		for _, bad := range []string{"stages:", "on_failure", "id: done\n", "max_returns: 3"} {
			if strings.Contains(y.Yaml, bad) {
				t.Fatalf("saved yaml has %q, defaults must not be written:\n%s", bad, y.Yaml)
			}
		}
		f.app.Contract(t, "ade-flat-steps", "args:AdeTaskService.SaveWorkflow", args, flowharness.Mask("baseHash"))
		f.app.Contract(t, "ade-flat-steps", "AdeTaskService.SaveWorkflow#workflow", saved, flowharness.Mask("hash"))
		again, err := f.app.W.AdeTask.Workflows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range again.Workflows {
			if e.FileName == "flow.yaml" && e.Workflow.Steps[1].Name != "Build it" {
				t.Fatalf("reloaded workflow = %+v", e.Workflow.Steps)
			}
		}
	})
}
