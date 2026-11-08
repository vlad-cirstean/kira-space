package claude

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// TestAdeOrchestrator runs a headless step of an orchestrator task. The agent must ask for a child task
// through create_task, wait for the approval, then read its own chain with task_chain.
func TestAdeOrchestrator(t *testing.T) {
	f := newAdeFixture(t, nil)
	const brief = "Create hello.txt containing hi, commit it and hand back the file name."
	f.saveWorkflow(t, "flow", agentStepYAML+manualStepYAML)
	f.saveWorkflow(t, "orch", "  - id: plan\n    name: Plan\n    kind: agent\n    runs_on: once\n    timeout: 6m\n"+
		"    prompt: \"You orchestrate. Call create_task once with title Hello child, workflow flow, repos [{repo: api, base: main}] and brief: "+brief+" "+
		"Wait for the answer. Then call task_chain with no arguments; an empty chain is fine, do not wait for the child. Then call finish_step with status done and summary ok.\"\n")
	orch, err := f.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Real orchestrator", WorkflowID: "orch", Kind: "orchestrator"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := f.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: orch.ID}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	var item approvals.Item
	testx.WaitUntil(t, 3*time.Minute, func() bool {
		items, err := f.W.ApprovalsSvc.List()
		if err != nil {
			return false
		}
		for _, it := range items {
			if it.Kind == approvals.KindTaskCreate {
				item = it
				return true
			}
		}
		return false
	})
	var pl struct {
		ParentTaskID string `json:"parentTaskId"`
		Title        string `json:"title"`
		Workflow     struct {
			ID string `json:"id"`
		} `json:"workflow"`
		Repos []struct {
			Name string `json:"name"`
			Base string `json:"base"`
		} `json:"repos"`
		Brief string `json:"brief"`
	}
	if err := json.Unmarshal(item.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	if pl.ParentTaskID != orch.ID || pl.Title != "Hello child" || pl.Workflow.ID != "flow" || len(pl.Repos) != 1 || pl.Repos[0].Name != "api" || pl.Repos[0].Base != "main" || !strings.Contains(pl.Brief, "hello.txt") {
		t.Fatalf("payload = %+v, want the child the prompt describes", pl)
	}
	res, err := f.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{item.ID}, Approve: true})
	if err != nil || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("Approve = %+v, %v", res, err)
	}

	var run adewire.Run
	testx.WaitUntil(t, 4*time.Minute, func() bool {
		b, err := f.W.AdeTask.Board(ctx)
		if err != nil {
			return false
		}
		for _, tk := range b.Tasks {
			for _, r := range tk.Runs {
				if tk.ID == orch.ID && r.StepID == "plan" && r.State != "running" && r.State != "queued" {
					run = r
					return true
				}
			}
		}
		return false
	})
	log := f.runLog(t, run.ID)
	if run.State != "done" {
		t.Fatalf("run = %+v, want done; log:\n%s", run, tail(log, 2500))
	}
	for _, tool := range []string{"mcp__kira-space__create_task", "mcp__kira-space__task_chain"} {
		if !strings.Contains(log, tool) {
			t.Fatalf("run log lacks %s:\n%s", tool, tail(log, 2500))
		}
	}

	b, err := f.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var children []adewire.Task
	for _, tk := range b.Tasks {
		if tk.ParentID == orch.ID {
			children = append(children, tk)
		}
	}
	if len(children) != 1 || children[0].Title != "Hello child" || children[0].Kind != "task" {
		t.Fatalf("children = %+v, want one task Hello child", children)
	}
	hs, err := f.W.AdeTask.Handovers(ctx, adewire.TaskArgs{TaskID: children[0].ID})
	if err != nil || len(hs.Handovers) != 1 || hs.Handovers[0].Source != "orchestrator" || !strings.Contains(hs.Handovers[0].Body, "hello.txt") {
		t.Fatalf("child handovers = %+v, %v, want the brief", hs, err)
	}
	f.requireCostUnder(t)
}
