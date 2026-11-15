package ade

import (
	"context"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/internal/agentmcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
)

func submitSession(call agentmcp.SpaceCall) detailsSession {
	s := detailsSession{key: cmpNonEmpty(call.SessionID, call.RunID), run: call.RunID, role: taskfiles.RoleManual}
	switch {
	case call.Orchestrator:
		s.role = taskfiles.RoleOrch
	case call.RunID != "":
		s.role = taskfiles.RoleRun
	}
	return s
}

// Submit is the submit tool: it reads the named task files as the calling session.
func (b *TaskBoard) Submit(ctx context.Context, call agentmcp.SpaceCall, in agentmcp.SubmitArgs) (string, error) {
	s := submitSession(call)
	res, err := b.SubmitFiles(ctx, call.TaskID, s, in.Paths, in.Reason)
	if err != nil {
		return "", err
	}
	reply := submitReply(res)
	if in.Finished && s.role != taskfiles.RoleRun {
		note, err := b.finishDraft(call.TaskID)
		if err != nil {
			return "", err
		}
		reply += note
	}
	return reply, nil
}

// finishDraft marks the current stage's draft finished and raises the move prompt. The note is
// appended to the reply: a missing draft is not an error, the agent writes one and submits again.
func (b *TaskBoard) finishDraft(taskID string) (string, error) {
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return "", err
	}
	cur := b.currentStepID(tc)
	if cur == "" || cur == entryDone {
		return "\nfinished: the task has no open stage", nil
	}
	var id string
	err = b.files.idx.InTx(func(tx taskfiles.Tx) error {
		d, err := b.files.cache.DraftOf(tx.Querier(), taskID, cur)
		if err != nil || d == nil || strings.TrimSpace(d.Body) == "" {
			return err
		}
		id = d.ID
		return b.files.cache.SetDraftFinished(tx.Querier(), d.ID, true, b.deps.Now().UnixMilli())
	})
	if err != nil {
		return "", err
	}
	if id == "" {
		return fmt.Sprintf("\nfinished: write %s first, then call submit with finished", draftRel(cur)), nil
	}
	// Board first: the prompt's dialog reads the draft from the board snapshot.
	b.notifyBoardNow(taskChanged(taskID))
	b.openHandoverPrompt(tc)
	return "\nfinished: the user was asked to move the task on", nil
}

// SubmitForFinish is finish_step's implicit submit. An ok finish needs the current step's draft:
// without one the finish is refused.
func (b *TaskBoard) SubmitForFinish(ctx context.Context, call agentmcp.SpaceCall, ok bool) (agentmcp.SubmitFinish, error) {
	res, err := b.SubmitFiles(ctx, call.TaskID, submitSession(call), nil, "")
	if err != nil {
		return agentmcp.SubmitFinish{}, err
	}
	reply := submitReply(res)
	c := res.Counts()
	if c.Accepted+c.Pending+c.Invalid == 0 {
		reply = ""
	}
	tc, err := b.loadTaskCtx(call.TaskID)
	if err != nil {
		return agentmcp.SubmitFinish{}, err
	}
	cur := b.currentStepID(tc)
	draft, err := b.draftBody(call.TaskID, cur)
	if err != nil {
		return agentmcp.SubmitFinish{}, err
	}
	if draft != "" || !ok || b.noStageRun(call.RunID) {
		return agentmcp.SubmitFinish{Reply: reply, Summary: draft}, nil
	}
	msg := fmt.Sprintf("write %s, then call finish_step", draftRel(cur))
	if c.Invalid > 0 {
		msg += "\n" + reply
	}
	return agentmcp.SubmitFinish{Refuse: msg}, nil
}

// noStageRun reports a run that is no stage of the task (a rebase, a check, a script run on the
// task): it hands nothing over.
func (b *TaskBoard) noStageRun(runID string) bool {
	run, err := b.deps.Tasks.GetRun(runID)
	return err != nil || run.Purpose != ""
}

// draftBody is the trimmed body of a stage's draft, empty when there is none.
func (b *TaskBoard) draftBody(taskID, stepID string) (string, error) {
	if b.files == nil {
		return "", nil
	}
	var body string
	err := b.files.idx.InTx(func(tx taskfiles.Tx) error {
		d, err := b.files.cache.DraftOf(tx.Querier(), taskID, stepID)
		if d != nil {
			body = strings.TrimSpace(d.Body)
		}
		return err
	})
	return body, err
}

// submitReply is the count line, then one line per file that changed or was refused.
func submitReply(res taskfiles.Result) string {
	c := res.Counts()
	var sb strings.Builder
	fmt.Fprintf(&sb, "accepted %d, pending user %d, invalid %d", c.Accepted, c.Pending, c.Invalid)
	for _, f := range res.Files {
		if f.Status == taskfiles.StatusUnchanged || f.Status == taskfiles.StatusIgnored {
			continue
		}
		fmt.Fprintf(&sb, "\n%s: %s", f.Path, f.Status)
		if f.Message != "" {
			sb.WriteString(": " + f.Message)
		}
	}
	return sb.String()
}
