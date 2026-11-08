package ade

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// orchestrator.go: orchestrator tasks (P571). One orchestrator runs many passes over the same workflow; each
// pass starts from the first step and keeps every earlier run. Its tools list its children and read chains.

const (
	maxChildTasks      = 100
	lastHandoverBytes  = 500
	taskNotACaller     = "That task is not one of your child tasks."
	handoverNotInChain = "That row is not in the chain."
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

// ListTasks implements agentmcp.OrchestratorTools: the caller's children, newest first.
func (b *TaskBoard) ListTasks(_ context.Context, call agentmcp.SpaceCall, args agentmcp.ListTasksArgs) (agentmcp.ListTasksResult, error) {
	state := cmpNonEmpty(args.State, "all")
	tasks, more, err := b.deps.Tasks.ChildTasks(call.TaskID, state, maxChildTasks)
	if err != nil {
		return agentmcp.ListTasksResult{}, toolErr(err)
	}
	statuses, err := b.deps.Statuses.List()
	if err != nil {
		return agentmcp.ListTasksResult{}, err
	}
	statusName := make(map[string]string, len(statuses))
	for _, s := range statuses {
		statusName[s.ID] = s.Name
	}
	out := agentmcp.ListTasksResult{Tasks: make([]agentmcp.ChildTaskInfo, 0, len(tasks)), More: more}
	for _, t := range tasks {
		info := agentmcp.ChildTaskInfo{
			ID: t.ID, Title: t.Title, Kind: t.Kind, Workflow: t.WorkflowID, Status: cmpNonEmpty(statusName[t.StatusID], t.StatusID),
			Archived: t.ArchivedAt != nil, CreatedAt: kiratime.MsToISO(t.CreatedAt),
		}
		if info.Title == "" {
			info.Title = cmpNonEmpty(t.JiraKey, "New task")
		}
		if wf, ok := b.taskWorkflow(t); ok {
			info.Workflow = cmpNonEmpty(wf.Name, wf.ID)
		}
		b.fillChildStep(&info, t)
		last, err := b.deps.Tasks.LatestHandover(t.ID)
		if err != nil {
			return agentmcp.ListTasksResult{}, err
		}
		if last != nil {
			info.LastHandover = capBytes(last.Body, lastHandoverBytes)
		}
		out.Tasks = append(out.Tasks, info)
	}
	return out, nil
}

// fillChildStep sets the child's current step name and whether it is done.
func (b *TaskBoard) fillChildStep(info *agentmcp.ChildTaskInfo, t model.AdeTask) {
	if t.ArchivedAt != nil {
		info.Finished = t.EntryStepID == entryDone
		if !info.Finished {
			info.Step = t.EntryStepID
		}
		return
	}
	tc, err := b.loadTaskCtx(t.ID)
	if err != nil {
		return
	}
	cur := b.currentStepID(tc)
	if info.Finished = cur == entryDone; !info.Finished {
		info.Step = cmpNonEmpty(stepName(tc, cur), cur)
	}
}

// TaskChain implements agentmcp.OrchestratorTools: a handover chain page of the caller or one child.
func (b *TaskBoard) TaskChain(_ context.Context, call agentmcp.SpaceCall, args agentmcp.TaskChainArgs) (agentmcp.TaskChainResult, error) {
	taskID := cmpNonEmpty(args.TaskID, call.TaskID)
	if taskID != call.TaskID {
		child, err := b.deps.Tasks.GetTask(taskID)
		if err != nil {
			if errors.Is(err, repos.ErrTaskNotFound) {
				return agentmcp.TaskChainResult{}, agentmcp.ToolError(taskNotACaller)
			}
			return agentmcp.TaskChainResult{}, err
		}
		if child.ParentID != call.TaskID {
			return agentmcp.TaskChainResult{}, agentmcp.ToolError(taskNotACaller)
		}
	}
	rows, more, err := b.deps.Tasks.HandoverPage(taskID, args.Before, args.Limit)
	switch {
	case errors.Is(err, repos.ErrHandoverNotFound):
		return agentmcp.TaskChainResult{}, agentmcp.ToolError(handoverNotInChain)
	case err != nil:
		return agentmcp.TaskChainResult{}, err
	}
	out := agentmcp.TaskChainResult{Rows: make([]agentmcp.ChainRow, 0, len(rows))}
	total := 0
	for i, h := range rows {
		body := agentmcp.ChainBodyCap(h.Body)
		if i > 0 && total+len(body) > agentmcp.ChainTotalMaxBytes {
			more = true
			break
		}
		total += len(body)
		from := h.FromStepName
		if h.Source == model.AdeHandoverSourceChild {
			from = cmpNonEmpty(h.FromTaskTitle, h.FromStepName)
		}
		out.Rows = append(out.Rows, agentmcp.ChainRow{
			ID: h.ID, From: from, To: cmpNonEmpty(h.ToStepName, h.ToStepID), Source: h.Source, State: h.State, Body: body,
			CreatedAt: kiratime.MsToISO(h.CreatedAt), FromTaskID: h.FromTaskID,
		})
	}
	if more && len(out.Rows) > 0 {
		out.More, out.Next = true, out.Rows[len(out.Rows)-1].ID
	}
	return out, nil
}
