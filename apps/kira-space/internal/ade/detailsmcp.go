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
	res, err := b.SubmitFiles(ctx, call.TaskID, submitSession(call), in.Paths, in.Reason)
	if err != nil {
		return "", err
	}
	return submitReply(res), nil
}

// FinishSession is a session's step.finish: it submits the task files, then marks the stage's draft
// finished.
func (b *TaskBoard) FinishSession(ctx context.Context, call agentmcp.SpaceCall) (string, error) {
	res, err := b.SubmitFiles(ctx, call.TaskID, submitSession(call), nil, "")
	if err != nil {
		return "", err
	}
	note, err := b.finishDraft(call.TaskID)
	if err != nil {
		return "", err
	}
	if c := res.Counts(); c.Accepted+c.Pending+c.Invalid > 0 {
		return submitReply(res) + "\n" + note, nil
	}
	return note, nil
}

// finishDraft marks the current stage's draft finished and raises the move prompt. A missing draft is
// not an error: the note tells the agent to write one and finish again.
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
		return "The task has no open stage.", nil
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
		return fmt.Sprintf("Write %s first, then call state step.finish again.", draftRel(cur)), nil
	}
	// Board first: the prompt's dialog reads the draft from the board snapshot.
	b.notifyBoardNow(taskChanged(taskID))
	b.openHandoverPrompt(tc)
	return "The user was asked to move the task on.", nil
}

// SubmitForFinish is a run's step.finish implicit submit. An ok finish needs the current step's draft:
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
	msg := fmt.Sprintf("write %s, then call state step.finish", draftRel(cur))
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
		if f.Proof != nil && proofLine(&sb, f) {
			continue
		}
		fmt.Fprintf(&sb, "\n%s: %s", f.Path, f.Status)
		if f.Message != "" {
			sb.WriteString(": " + f.Message)
		}
	}
	return sb.String()
}

// proofLine writes the unit line of a proof result and, for an invalid unit, one line per problem. It
// reports false for a result with nothing to say beyond the plain per-file line.
func proofLine(sb *strings.Builder, f taskfiles.FileResult) bool {
	p := f.Proof
	folder := "proofs/" + p.Slug + "/"
	switch f.Status {
	case taskfiles.StatusInvalid:
		if len(p.Problems) == 0 {
			return false
		}
		fmt.Fprintf(sb, "\n%s: invalid, %d %s (lines follow)", folder, len(p.Problems), proofPlural(len(p.Problems), "problem"))
		for _, l := range p.Problems {
			sb.WriteString("\n" + l)
		}
		return true
	case taskfiles.StatusPending:
		if p.Changed != nil && len(p.Changed.Fields) == 1 && p.Changed.Fields[0] == "delete" {
			fmt.Fprintf(sb, "\n%s: pending user, delete of %q (%s)", folder, p.Name, p.Reason)
			return true
		}
		fmt.Fprintf(sb, "\n%s: pending user, change to %q: %s (%s)", folder, p.Name, changeText(p.Changed), p.Reason)
		return true
	case taskfiles.StatusAccepted:
		if p.New {
			steps := make([]string, len(p.Steps))
			for i, st := range p.Steps {
				steps[i] = fmt.Sprintf("%s (%s)", st.ID, st.View)
			}
			fmt.Fprintf(sb, "\n%s: accepted, new proof %q: %s; %d %s", folder, p.Name, strings.Join(steps, ", "), p.Counts.Steps, proofPlural(p.Counts.Steps, "step"))
			if p.Counts.Images > 0 {
				fmt.Fprintf(sb, ", %d %s", p.Counts.Images, proofPlural(p.Counts.Images, "image"))
			}
			if p.Counts.DataFiles > 0 {
				fmt.Fprintf(sb, ", %d data %s", p.Counts.DataFiles, proofPlural(p.Counts.DataFiles, "file"))
			}
			return true
		}
		if p.Changed != nil {
			fmt.Fprintf(sb, "\n%s: accepted, change to %q: %s", folder, p.Name, changeText(p.Changed))
			return true
		}
		fmt.Fprintf(sb, "\n%s: accepted, deleted %q", folder, p.Name)
		return true
	}
	return false
}

func proofPlural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// changeText lists what an edit changes by step id.
func changeText(c *taskfiles.ProofChanged) string {
	if c == nil {
		return "no change"
	}
	var parts []string
	for _, id := range c.Changed {
		parts = append(parts, id+" changed")
	}
	for _, id := range c.Added {
		parts = append(parts, id+" added")
	}
	for _, id := range c.Removed {
		parts = append(parts, id+" removed")
	}
	for _, f := range c.Fields {
		parts = append(parts, f+" changed")
	}
	return strings.Join(parts, ", ")
}
