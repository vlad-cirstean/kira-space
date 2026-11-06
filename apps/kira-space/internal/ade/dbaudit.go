package ade

import (
	"context"
	"encoding/json"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// DbAuditReader is the audit store as the board reads it.
type DbAuditReader interface {
	ListForTask(taskID string) ([]model.DbAuditCall, error)
}

// DbAudit lists the Kira DB calls of a task, newest first, each with its approvals. Archived tasks work too.
func (b *TaskBoard) DbAudit(_ context.Context, taskID string) ([]adewire.DbAuditEntry, error) {
	task, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	out := []adewire.DbAuditEntry{}
	if b.deps.DbAudit == nil {
		return out, nil
	}
	rows, err := b.deps.DbAudit.ListForTask(taskID)
	if err != nil {
		return nil, err
	}
	tc := &taskCtx{}
	if wf, ok := b.taskWorkflow(task); ok {
		tc.wf = &wf
	}
	for _, c := range rows {
		e := adewire.DbAuditEntry{
			ID: c.ID, SessionID: c.SessionID, StepID: c.StepID, StepName: stepName(tc, c.StepID), Tool: c.Tool,
			ConnectionName: c.ConnectionName, Mode: c.Mode, Target: c.Target, Statement: c.Statement,
			StatementLen: c.StatementLen, StatementWithheld: c.StatementWithheld, StatementTruncated: c.StatementTruncated,
			Summary: c.Summary, Status: c.Status, Error: c.Error, StartedAt: c.StartedMs,
			Approvals: make([]adewire.DbAuditApproval, 0, len(c.Approvals)),
		}
		if c.EndedMs != nil {
			d := *c.EndedMs - c.StartedMs
			e.DurationMs = &d
		}
		for _, a := range c.Approvals {
			wa := adewire.DbAuditApproval{
				ID: a.ID, Subject: a.Subject, Reason: a.Reason, Stage: a.Stage, Outcome: a.Outcome,
				RaisedAt: a.StartedMs, ResolvedAt: a.EndedMs, ExemptionLimits: []adewire.DbAuditLimit{},
			}
			if a.ExemptionJSON != "" {
				var ex struct {
					Limits []adewire.DbAuditLimit `json:"limits"`
					Reason string                 `json:"reason"`
				}
				if json.Unmarshal([]byte(a.ExemptionJSON), &ex) == nil {
					if ex.Limits != nil {
						wa.ExemptionLimits = ex.Limits
					}
					wa.ExemptionReason = ex.Reason
				}
			}
			e.Approvals = append(e.Approvals, wa)
		}
		out = append(out, e)
	}
	return out, nil
}
