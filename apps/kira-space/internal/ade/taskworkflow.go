package ade

import (
	"context"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// SwitchLosesWorkError refuses an unconfirmed switch and names what it would delete.
type SwitchLosesWorkError struct{ Runs, Logs int }

func (e *SwitchLosesWorkError) Error() string {
	return fmt.Sprintf("ade: switching the workflow deletes %d runs and %d logs", e.Runs, e.Logs)
}

// SetTaskWorkflow switches the task's workflow and restarts it at the first step: the task's runs
// and run logs go; without Confirmed a task holding any is refused with SwitchLosesWorkError.
// The same id is a no-op; "" clears workflow and position.
func (b *TaskBoard) SetTaskWorkflow(_ context.Context, args adewire.SetTaskWorkflowArgs) (adewire.Task, error) {
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	task, err := b.deps.Tasks.GetTask(args.TaskID)
	if err != nil {
		return adewire.Task{}, err
	}
	if task.WorkflowID == args.WorkflowID {
		return b.wireTask(task)
	}
	var entry string
	if args.WorkflowID != "" {
		if b.deps.Workflows == nil {
			return adewire.Task{}, invalid("workflow %q not found", args.WorkflowID)
		}
		wf, ok := b.deps.Workflows.Get(args.WorkflowID)
		if !ok {
			return adewire.Task{}, invalid("workflow %q not found", args.WorkflowID)
		}
		entry = firstStepID(wf)
	}
	running, err := b.deps.Tasks.HasRunning(args.TaskID)
	if err != nil {
		return adewire.Task{}, err
	}
	if running {
		return adewire.Task{}, invalid("stop its running agents first")
	}
	if !args.Confirmed {
		runs, logs, err := b.deps.Tasks.RunsAndLogs(args.TaskID)
		if err != nil {
			return adewire.Task{}, err
		}
		if runs > 0 || logs > 0 {
			return adewire.Task{}, &SwitchLosesWorkError{Runs: runs, Logs: logs}
		}
	}
	if err := b.deps.Tasks.ResetWorkflow(args.TaskID, args.WorkflowID, entry); err != nil {
		return adewire.Task{}, err
	}
	b.syncStatus(args.TaskID)
	b.clearStepMessages(args.TaskID)
	b.notifyBoard()
	task, err = b.deps.Tasks.GetTask(args.TaskID)
	if err != nil {
		return adewire.Task{}, err
	}
	return b.wireTask(task)
}

func (b *TaskBoard) clearStepMessages(taskID string) {
	prefix := taskID + "|"
	b.runMu.Lock()
	defer b.runMu.Unlock()
	for k := range b.stepMsgs {
		if strings.HasPrefix(k, prefix) {
			delete(b.stepMsgs, k)
		}
	}
}
