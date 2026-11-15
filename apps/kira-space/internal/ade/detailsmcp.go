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
	if !ok {
		return agentmcp.SubmitFinish{Reply: reply}, nil
	}
	tc, err := b.loadTaskCtx(call.TaskID)
	if err != nil {
		return agentmcp.SubmitFinish{}, err
	}
	cur := b.currentStepID(tc)
	has, err := b.hasDraft(call.TaskID, cur)
	if err != nil {
		return agentmcp.SubmitFinish{}, err
	}
	if has {
		return agentmcp.SubmitFinish{Reply: reply}, nil
	}
	msg := fmt.Sprintf("write %s, then call finish_step", draftRel(cur))
	if c.Invalid > 0 {
		msg += "\n" + reply
	}
	return agentmcp.SubmitFinish{Refuse: msg}, nil
}

func (b *TaskBoard) hasDraft(taskID, stepID string) (bool, error) {
	if b.files == nil {
		return true, nil
	}
	has := false
	err := b.files.idx.InTx(func(tx taskfiles.Tx) error {
		d, err := b.files.cache.DraftOf(tx.Querier(), taskID, stepID)
		has = d != nil && strings.TrimSpace(d.Body) != ""
		return err
	})
	return has, err
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
