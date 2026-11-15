package ade

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// orchestrator.go: orchestrator tasks (P571). One orchestrator runs many passes over the same workflow; each
// pass starts from the first step and keeps every earlier run. Its tools list its children and read chains.

const (
	maxChildTasks     = 100
	lastHandoverBytes = 500
)

// isOrchestrator reports whether the task is a live orchestrator.
func (b *TaskBoard) isOrchestrator(taskID string) bool {
	t, err := b.deps.Tasks.GetTask(taskID)
	return err == nil && t.Kind == model.AdeTaskKindOrchestrator
}

// NextPass moves a finished orchestrator back to the first step of its workflow and starts it. The pass
// handover carries the optional message and the previous pass's final handover; every run stays history.
func (b *TaskBoard) NextPass(ctx context.Context, args adewire.NextPassArgs) (adewire.NextPassResult, error) {
	if err := checkNotes(args.Notes); err != nil {
		return adewire.NextPassResult{}, err
	}
	first, wasAuto, err := b.movePass(args)
	if err != nil {
		return adewire.NextPassResult{}, err
	}
	res := adewire.NextPassResult{StepID: first}
	if !wasAuto {
		return res, nil
	}
	if _, err := b.StartRun(ctx, adewire.StartRunArgs{TaskID: args.TaskID, FromStepID: first}); err != nil {
		return res, err
	}
	res.Started = true
	return res, nil
}

// movePass writes the pass move under the task lock; auto is true when the first step starts by itself.
func (b *TaskBoard) movePass(args adewire.NextPassArgs) (first string, auto bool, err error) {
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(args.TaskID)
	if err != nil {
		return "", false, err
	}
	if tc.task.Kind != model.AdeTaskKindOrchestrator {
		return "", false, repos.ErrOrchestratorKind
	}
	if err := b.staleStep(tc, args.FromStepID); err != nil {
		return "", false, err
	}
	if tc.wf == nil {
		return "", false, invalid("workflow %q is not available", tc.task.WorkflowID)
	}
	if b.currentStepID(tc) != entryDone {
		return "", false, invalid("a pass starts when the task is done")
	}
	running, err := b.deps.Tasks.HasRunning(args.TaskID)
	if err != nil {
		return "", false, err
	}
	if running {
		return "", false, invalid("stop its running agents first")
	}
	target, ok := firstStep(*tc.wf)
	if !ok {
		return "", false, invalid("workflow %q has no step to start on", tc.wf.Name)
	}
	first = tc.wf.Steps[target].ID
	if args.CloseSessions {
		if err := b.closeStageSessionsLocked(args.TaskID, entryDone, first); err != nil {
			return "", false, err
		}
	}
	if err := b.snapshotWorkflow(&tc.task); err != nil {
		return "", false, err
	}
	since, err := b.passSince(tc, target)
	if err != nil {
		return "", false, err
	}
	h := b.handoffFor(tc, entryDone, first, nil)
	h.Source = model.AdeHandoverSourcePass
	prev, err := b.deps.Tasks.LatestHandover(args.TaskID)
	if err != nil {
		return "", false, err
	}
	vars := llmprompts.PassHandoverVars{}
	if args.Notes != nil {
		vars.Message = strings.TrimSpace(*args.Notes)
	}
	if prev != nil && prev.ToStepID == entryDone {
		vars.Previous, vars.PreviousFrom = strings.TrimSpace(prev.Body), prev.FromStepName
		h.FromStepName = prev.FromStepName
	}
	if body := capBytes(strings.TrimSpace(llmprompts.AdePassHandover.Text(b.prompts(), vars)), maxHandoverBytes); body != "" {
		h.Notes = &body
	} else {
		h = nil
	}
	if err := b.deps.Tasks.SetEntryStep(args.TaskID, first, since, h); err != nil {
		return "", false, err
	}
	b.closeHandoverPrompt(args.TaskID)
	b.mirrorTask(args.TaskID, userEdit(false))
	b.syncStatus(args.TaskID)
	b.clearStepMessages(args.TaskID)
	b.notifyBoard()
	steps := workflowSteps(*tc.wf)
	return first, !steps[target].manual(), nil
}

// capBytes cuts s to n bytes on a rune boundary.
func capBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}
