package ade

import (
	"context"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestRunEngine_SetTaskStepMovesBothWaysAndGuardsLiveRuns(t *testing.T) {
	e := newEngine(t, map[string][]string{"*": {"sleep"}})
	e.repo("api")
	e.workflow(flowYAML(agentStep("one", "") + manualStep))
	ctx := context.Background()
	id := e.task("api")

	e.start(id)
	e.waitRun(id, "one/api", model.AdeRunRunning)
	if _, err := e.board.SetTaskStep(ctx, id, "review", "", nil, false); err == nil {
		t.Fatal("step moved while a run is live")
	}
	if err := e.board.StopRun(ctx, e.runs(id)["one/api"].ID); err != nil {
		t.Fatal(err)
	}
	e.waitRun(id, "one/api", model.AdeRunStuck)

	for _, want := range []string{"review", "one", "done", "one"} {
		task, err := e.board.SetTaskStep(ctx, id, want, "", nil, false)
		if err != nil || task.EntryStepID != want {
			t.Fatalf("SetTaskStep(%q) = %+v, %v", want, task.EntryStepID, err)
		}
	}
	if len(e.runs(id)) != 1 {
		t.Fatal("moving steps deleted runs")
	}
	if _, err := e.board.SetTaskStep(ctx, id, "nope", "", nil, false); err == nil {
		t.Fatal("unknown step accepted")
	}
}

func TestRunEngine_StartedTaskKeepsItsWorkflowAfterAnEdit(t *testing.T) {
	e := newEngine(t, map[string][]string{"*": {"done"}})
	e.repo("api")
	e.workflow(flowYAML(agentStep("one", "") + manualStep))
	ctx := context.Background()
	started := e.task("api")
	idle := e.task("api")

	e.start(started)
	e.waitRun(started, "one/api", model.AdeRunDone)

	qa := "  - id: qa\n    name: QA\n    kind: manual\n    status: In review\n"
	e.workflow(flowYAML(agentStep("one", "") + qa + manualStep))

	board, err := e.board.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var startedWire, idleWire *adewire.Task
	for i := range board.Tasks {
		switch board.Tasks[i].ID {
		case started:
			startedWire = &board.Tasks[i]
		case idle:
			idleWire = &board.Tasks[i]
		}
	}
	if startedWire == nil || startedWire.Workflow == nil || !startedWire.WorkflowOutdated || len(startedWire.Workflow.Steps) != 2 {
		t.Fatalf("started task = %+v, want its 2-step snapshot flagged outdated", startedWire)
	}
	if idleWire.Workflow != nil || idleWire.WorkflowOutdated {
		t.Fatalf("idle task = %+v, want no snapshot and no flag", idleWire)
	}

	if _, err := e.board.SetTaskStep(ctx, started, "qa", "", nil, false); err == nil {
		t.Fatal("started task moved to a step only the edited file has")
	}
	task, err := e.board.StepDone(ctx, started, nil, false)
	if err != nil || task.EntryStepID != "done" {
		t.Fatalf("StepDone = %+v, %v; want done from the snapshot, not qa", task.EntryStepID, err)
	}

	task, err = e.board.SetTaskStep(ctx, idle, "qa", "", nil, false)
	if err != nil || task.EntryStepID != "qa" {
		t.Fatalf("idle SetTaskStep = %+v, %v; want qa from the live file", task.EntryStepID, err)
	}
	if task.Workflow == nil || len(task.Workflow.Steps) != 3 || task.WorkflowOutdated {
		t.Fatalf("a first step move snapshots the live file: %+v", task.Workflow)
	}
}
