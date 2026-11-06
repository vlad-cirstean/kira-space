package ade

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// handover.go: the text one stage leaves for the next (P407). A draft is what sessions write before the
// move; a stage move turns it into a handed row. Stage = workflow step.

// maxHandoverBytes bounds a handover body, whoever writes it.
const maxHandoverBytes = agentmcp.MaxHandoverBytes

// maxNoteBytes bounds a user note (state note); drafts and handed rows keep maxHandoverBytes.
const maxNoteBytes = 1 << 20

func noteTooLong() error {
	return invalid("%s", i18n.T("backend.space.ade.notesTooLong.body", nil))
}

// handoverTooLong is the refusal for a body over the limit.
func handoverTooLong() error {
	return invalid("%s", i18n.T("backend.space.ade.handoverTooLong.body", nil))
}

// stepName is the display name of a step id of the task's workflow; "" for done and unknown steps.
func stepName(tc *taskCtx, id string) string {
	if tc.wf == nil {
		return ""
	}
	for _, st := range tc.wf.Steps {
		if st.ID == id {
			return cmpNonEmpty(st.Name, st.ID)
		}
	}
	return ""
}

func handoverPromptID(taskID string) string { return prompts.ID(prompts.KindHandover, taskID) }

func (b *TaskBoard) openHandoverPrompt(tc *taskCtx) {
	if b.deps.PromptSink == nil {
		return
	}
	b.deps.PromptSink.Open(prompts.Prompt{
		ID: handoverPromptID(tc.task.ID), Kind: prompts.KindHandover, Ref: tc.task.ID,
		Title: i18n.T("backend.prompt.handover.title", map[string]string{"task": taskTitle(tc.task, tc.branches)}),
	})
}

func (b *TaskBoard) closeHandoverPrompt(taskID string) {
	if b.deps.PromptSink != nil {
		b.deps.PromptSink.Close(handoverPromptID(taskID))
	}
}

// RecordHandover stores a session's handover notes as the open draft of the task's current stage. stepID is the
// stage the session was opened for; a task that moved on since refuses. A finished draft on a live stage raises
// the "move to next stage" prompt.
func (b *TaskBoard) RecordHandover(_ context.Context, taskID, stepID, sessionID, notes string, finished bool) error {
	if len(notes) > maxHandoverBytes {
		return agentmcp.ToolError(fmt.Sprintf("notes is too long: at most %d bytes.", maxHandoverBytes))
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return err
	}
	cur := b.currentStepID(tc)
	if stepID != "" && stepID != cur {
		return agentmcp.ToolError(fmt.Sprintf("The task moved on to %s; nothing was recorded. Tell the user.", cmpNonEmpty(stepName(tc, cur), cur)))
	}
	if _, err := b.deps.Tasks.UpsertHandoverDraft(model.AdeHandover{
		ID: ids.NewID(), TaskID: taskID, FromStepID: cur, FromStepName: stepName(tc, cur),
		Body: notes, Finished: finished, SessionID: sessionID, UpdatedAt: b.deps.Now().UnixMilli(),
	}); err != nil {
		return err
	}
	if finished && cur != "" && cur != entryDone {
		b.openHandoverPrompt(tc)
	} else {
		b.closeHandoverPrompt(taskID)
	}
	b.notifyBoard()
	return nil
}

// handoffFor is the handover a stage move from -> to carries. notes nil keeps the stage's draft.
func (b *TaskBoard) handoffFor(tc *taskCtx, from, to string, notes *string) *model.AdeHandoff {
	return &model.AdeHandoff{
		ID: ids.NewID(), FromStepID: from, FromStepName: stepName(tc, from), ToStepID: to, ToStepName: stepName(tc, to),
		Notes: notes, Source: model.AdeHandoverSourceUser, Now: b.deps.Now().UnixMilli(),
	}
}

// checkNotes refuses notes over the limit.
func checkNotes(notes *string) error {
	if notes != nil && len(*notes) > maxHandoverBytes {
		return handoverTooLong()
	}
	return nil
}

// recordRunHandoverLocked stores the summaries of a step whose last target just finished as a handed `run` row
// into where the route path now ends. A draft of the step (an interactive branch session during an automatic
// stage) is folded in first. The task mutex is held.
func (b *TaskBoard) recordRunHandoverLocked(run model.AdeRun) {
	if run.Purpose != "" || run.State != model.AdeRunDone {
		return
	}
	tc, err := b.loadTaskCtx(run.TaskID)
	if err != nil || !tc.active() {
		return
	}
	plan, err := b.plan(tc)
	if err != nil {
		return
	}
	idx, entry := stepIndex(plan, run.StepID), tc.entryIndex(plan)
	if idx < 0 || entry < 0 || plan[idx].Def.manual() || plan[idx].state() != model.AdeRunDone {
		return
	}
	if path, _ := walkPath(plan, entry); !containsInt(path, idx) {
		return
	}
	to := entryDone
	if cur := currentStep(plan, entry); cur >= 0 {
		to = plan[cur].Def.ID
	}
	if to == run.StepID {
		return
	}
	draft, err := b.deps.Tasks.HandoverDraft(run.TaskID, run.StepID)
	if err != nil {
		slog.Warn("ade: read handover draft", "scope", "ade", "task", run.TaskID, "err", err)
		return
	}
	h := b.handoffFor(tc, run.StepID, to, nil)
	if summaries := b.runSummaries(tc, plan[idx]); summaries != "" {
		body, author := summaries, run.SessionID
		if draft != nil && strings.TrimSpace(draft.Body) != "" {
			body = draft.Body + "\n\n" + summaries
			if draft.SessionID != "" {
				author = draft.SessionID // the draft's text leads the body
			}
		}
		h.Notes, h.Source, h.SessionID = &body, model.AdeHandoverSourceRun, author
	} else if draft == nil {
		return
	}
	if err := b.deps.Tasks.RecordRunHandover(run.TaskID, *h); err != nil {
		slog.Warn("ade: record run handover", "scope", "ade", "task", run.TaskID, "err", err)
		return
	}
	b.closeHandoverPrompt(run.TaskID)
	b.notifyBoard()
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// runSummaries joins the summaries of a step's latest runs: the bare text for a one-branch step, else one
// "- <repo> · <branch>: <summary>" line per branch.
func (b *TaskBoard) runSummaries(tc *taskCtx, v stepView) string {
	var lines []string
	for _, id := range v.Targets {
		sum := strings.TrimSpace(v.Runs[id].Summary)
		if sum == "" {
			continue
		}
		if len(v.Targets) == 1 {
			return sum
		}
		sb, _ := tc.branch(id)
		lines = append(lines, fmt.Sprintf("- %s · %s: %s", tc.nick[sb.CodeRepoID], sb.Name, sum))
	}
	return strings.Join(lines, "\n")
}

// handoverText is a handover as a prompt carries it: the stage it came from and its body.
type handoverText struct{ Body, From string }

// handoverFor is the text the next session of stepID gets, empty when the latest stage move went elsewhere.
func (b *TaskBoard) handoverFor(tc *taskCtx, stepID string) handoverText {
	h := b.handoverInto(tc.task.ID, stepID)
	if h == nil {
		return handoverText{}
	}
	return handoverText{Body: h.Body, From: cmpNonEmpty(h.FromStepName, h.FromStepID)}
}

// handoverInto is the handover the task's latest stage move left for stepID; nil when the latest move
// went elsewhere. One rule for the prompts the server builds and the preview the client shows.
func (b *TaskBoard) handoverInto(taskID, stepID string) *model.AdeHandover {
	h, err := b.deps.Tasks.LatestHandover(taskID)
	if err != nil {
		slog.Warn("ade: read handover", "scope", "ade", "task", taskID, "err", err)
		return nil
	}
	if h == nil || h.ToStepID != stepID {
		return nil
	}
	return h
}

// DismissHandover hides the "move to next stage" prompt of the task's current stage; the draft stays.
func (b *TaskBoard) DismissHandover(_ context.Context, taskID string) error {
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return err
	}
	if _, err := b.deps.Tasks.DismissHandoverDraft(taskID, b.currentStepID(tc)); err != nil {
		return err
	}
	b.closeHandoverPrompt(taskID)
	b.notifyBoard()
	return nil
}

// AddNote appends a user note to the task's chain, stamped with the stage the task is in. Archived tasks refuse.
func (b *TaskBoard) AddNote(_ context.Context, taskID, body string) (adewire.Handover, error) {
	if strings.TrimSpace(body) == "" {
		return adewire.Handover{}, invalid("a note needs text")
	}
	if len(body) > maxNoteBytes {
		return adewire.Handover{}, noteTooLong()
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return adewire.Handover{}, err
	}
	cur := b.currentStepID(tc)
	now := b.deps.Now().UnixMilli()
	n, err := b.deps.Tasks.AddNote(model.AdeHandover{
		ID: ids.NewID(), TaskID: taskID, FromStepID: cur, FromStepName: stepName(tc, cur), Body: body, CreatedAt: now,
	})
	if err != nil {
		return adewire.Handover{}, err
	}
	b.recordHandover(n, "", n.Body)
	b.notifyBoard()
	return toWireHandover(n), nil
}

// userNotes joins the task's user notes, newest first, with a blank line between; the prompts' Notes input.
func (b *TaskBoard) userNotes(taskID string) string {
	rows, err := b.deps.Tasks.Notes(taskID)
	if err != nil {
		slog.Warn("ade: read notes", "scope", "ade", "task", taskID, "err", err)
		return ""
	}
	return joinNotes(rows)
}

func joinNotes(rows []model.AdeHandover) string {
	bodies := make([]string, len(rows))
	for i, n := range rows {
		bodies[i] = n.Body
	}
	return strings.Join(bodies, "\n\n")
}

// UpdateHandover replaces a row's body; an empty body deletes the row. A draft's deletion closes its prompt.
// A note row may reach maxNoteBytes; the others maxHandoverBytes.
func (b *TaskBoard) UpdateHandover(_ context.Context, id, body string) error {
	row, err := b.deps.Tasks.GetHandover(id)
	if err != nil {
		return err
	}
	if row == nil {
		return invalid("handover %s not found", id)
	}
	switch {
	case row.State == model.AdeHandoverNote && len(body) > maxNoteBytes:
		return noteTooLong()
	case row.State != model.AdeHandoverNote && len(body) > maxHandoverBytes:
		return handoverTooLong()
	}
	mu := b.taskMu(row.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if strings.TrimSpace(body) == "" {
		if err := b.deps.Tasks.DeleteHandover(id); err != nil {
			return err
		}
		if row.State == model.AdeHandoverDraft {
			b.closeHandoverPrompt(row.TaskID)
		}
	} else if _, err := b.deps.Tasks.UpdateHandoverBody(id, body, b.deps.Now().UnixMilli()); err != nil {
		return err
	}
	after := body
	if strings.TrimSpace(body) == "" {
		after = ""
	}
	b.recordHandover(*row, row.Body, after)
	b.notifyBoard()
	return nil
}

// Handovers lists a task's handover rows, newest first; archived tasks included.
func (b *TaskBoard) Handovers(_ context.Context, taskID string) ([]adewire.Handover, error) {
	if _, err := b.deps.Tasks.GetTask(taskID); err != nil {
		return nil, err
	}
	rows, err := b.deps.Tasks.Handovers(taskID)
	if err != nil {
		return nil, err
	}
	return toWireHandovers(rows), nil
}

// reopenHandoverPrompts raises the prompt again for every finished, undismissed draft of a live stage; the
// router keeps no prompts across a restart.
func (b *TaskBoard) reopenHandoverPrompts() {
	drafts, err := b.deps.Tasks.FinishedDrafts()
	if err != nil {
		slog.Warn("ade: list finished handovers", "scope", "ade", "err", err)
		return
	}
	for _, d := range drafts {
		func() {
			mu := b.taskMu(d.TaskID)
			mu.Lock()
			defer mu.Unlock()
			tc, err := b.loadTaskCtx(d.TaskID)
			if err != nil || d.FromStepID == "" || b.currentStepID(tc) != d.FromStepID {
				return
			}
			b.openHandoverPrompt(tc)
		}()
	}
}

func toWireHandover(h model.AdeHandover) adewire.Handover {
	return adewire.Handover{
		ID: h.ID, TaskID: h.TaskID, FromStepID: h.FromStepID, FromStepName: h.FromStepName, ToStepID: h.ToStepID,
		ToStepName: h.ToStepName, Body: h.Body, Source: h.Source, State: h.State, Finished: h.Finished,
		Dismissed: h.Dismissed, Edited: h.Edited, SessionID: h.SessionID, CreatedAt: h.CreatedAt, UpdatedAt: h.UpdatedAt,
	}
}

func toWireHandovers(rows []model.AdeHandover) []adewire.Handover {
	out := make([]adewire.Handover, len(rows))
	for i, h := range rows {
		out[i] = toWireHandover(h)
	}
	return out
}

// applyHandovers sets a wire task's handover facts.
func applyHandovers(t *adewire.Task, bh repos.BoardHandover) {
	if bh.In != nil {
		w := toWireHandover(*bh.In)
		t.HandoverIn = &w
	}
	t.HandoverDrafts = toWireHandovers(bh.Drafts)
	t.Notes = joinNotes(bh.Notes)
}
