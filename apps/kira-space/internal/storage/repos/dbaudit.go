package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// MaxDbAuditCallsPerTask bounds one runaway agent; a normal run is tens of calls.
const MaxDbAuditCallsPerTask = 2000

// DbAuditRepo persists the Kira DB audit trail (migrations/0081_p547_db_audit.sql).
type DbAuditRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// dbAuditAbandoned is the outcome of an approval left pending by a quit.
const dbAuditAbandoned = "abandoned"

// StartCall inserts a running call row and drops the task's oldest calls past the cap
// (their approvals follow by cascade).
func (r *DbAuditRepo) StartCall(c model.DbAuditCall) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin db audit call: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	a := kiraschema.AdeDbAudit
	ib := r.Dialect.InsertInto(a.Table).Cols(a.ID, a.TaskID, a.Kind, a.SessionID, a.StepID, a.RunID, a.StartedMs, a.Status,
		a.Tool, a.ConnectionID, a.ConnectionName, a.Mode, a.Target, a.Statement, a.StatementLen, a.StatementWithheld,
		a.StatementTruncated, a.Summary).
		Values(c.ID, c.TaskID, model.DbAuditKindCall, c.SessionID, c.StepID, c.RunID, c.StartedMs, model.DbAuditRunning,
			c.Tool, nullableString(c.ConnectionID), c.ConnectionName, c.Mode, c.Target, c.Statement, c.StatementLen,
			c.StatementWithheld, c.StatementTruncated, c.Summary)
	if _, err := sqlb.Exec(tx, ib); err != nil {
		return fmt.Errorf("repos: insert db audit call: %w", err)
	}
	keep := r.Dialect.Select(a.ID).From(a.Table)
	keep.Where(keep.Equal(a.Kind, model.DbAuditKindCall), keep.Equal(a.TaskID, c.TaskID)).
		OrderBy(a.StartedMs+" DESC", sqlb.RowID+" DESC").Limit(MaxDbAuditCallsPerTask)
	trim := r.Dialect.DeleteFrom(a.Table)
	trim.Where(trim.Equal(a.Kind, model.DbAuditKindCall), trim.Equal(a.TaskID, c.TaskID), trim.NotIn(a.ID, keep))
	if _, err := sqlb.Exec(tx, trim); err != nil {
		return fmt.Errorf("repos: trim db audit: %w", err)
	}
	return tx.Commit()
}

// FinishCall sets a call's final status, end time and error text.
func (r *DbAuditRepo) FinishCall(id, status string, endedMs int64, errText string) error {
	a := kiraschema.AdeDbAudit
	ub := r.Dialect.Update(a.Table)
	ub.Set(ub.Assign(a.Status, status), ub.Assign(a.EndedMs, endedMs), ub.Assign(a.Error, nullableString(errText))).
		Where(ub.Equal(a.ID, id), ub.Equal(a.Kind, model.DbAuditKindCall))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: finish db audit call %s: %w", id, err)
	}
	return nil
}

// SetStatement stores a call's redacted statement once it can be produced.
func (r *DbAuditRepo) SetStatement(id, statement string, length int, withheld, truncated bool) error {
	a := kiraschema.AdeDbAudit
	ub := r.Dialect.Update(a.Table)
	ub.Set(ub.Assign(a.Statement, statement), ub.Assign(a.StatementLen, length), ub.Assign(a.StatementWithheld, withheld),
		ub.Assign(a.StatementTruncated, truncated)).Where(ub.Equal(a.ID, id), ub.Equal(a.Kind, model.DbAuditKindCall))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: set db audit statement %s: %w", id, err)
	}
	return nil
}

// RaiseApproval inserts an approval row; ResolvedMs nil leaves it pending.
func (r *DbAuditRepo) RaiseApproval(a model.DbAuditApproval) error {
	outcome := a.Outcome
	if outcome == "" {
		outcome = model.DbAuditPending
	}
	c := kiraschema.AdeDbAudit
	ib := r.Dialect.InsertInto(c.Table).Cols(c.ID, c.TaskID, c.CallID, c.Kind, c.SessionID, c.StepID, c.RunID, c.StartedMs,
		c.EndedMs, c.Status, c.Subject, c.Reason, c.Stage, c.ExemptionJSON).
		Values(a.ID, a.TaskID, a.CallID, model.DbAuditKindApproval, a.SessionID, a.StepID, a.RunID, a.StartedMs, a.EndedMs, outcome,
			a.Subject, a.Reason, a.Stage, nullableString(a.ExemptionJSON))
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: insert db audit approval: %w", err)
	}
	return nil
}

// ResolveApproval sets a pending approval's outcome and end time.
func (r *DbAuditRepo) ResolveApproval(id, outcome string, endedMs int64) error {
	a := kiraschema.AdeDbAudit
	ub := r.Dialect.Update(a.Table)
	ub.Set(ub.Assign(a.Status, outcome), ub.Assign(a.EndedMs, endedMs)).Where(ub.Equal(a.ID, id), ub.Equal(a.Kind, model.DbAuditKindApproval))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: resolve db audit approval %s: %w", id, err)
	}
	return nil
}

func dbAuditCoalesce(col string) string { return "COALESCE(" + col + ", '')" }

// dbAuditCols is in scan order.
var dbAuditCols = []string{
	kiraschema.AdeDbAudit.ID, dbAuditCoalesce(kiraschema.AdeDbAudit.CallID), kiraschema.AdeDbAudit.TaskID,
	kiraschema.AdeDbAudit.SessionID, kiraschema.AdeDbAudit.StepID, kiraschema.AdeDbAudit.RunID, kiraschema.AdeDbAudit.StartedMs,
	kiraschema.AdeDbAudit.EndedMs, kiraschema.AdeDbAudit.Status, kiraschema.AdeDbAudit.Tool,
	dbAuditCoalesce(kiraschema.AdeDbAudit.ConnectionID), kiraschema.AdeDbAudit.ConnectionName, kiraschema.AdeDbAudit.Mode,
	kiraschema.AdeDbAudit.Target, dbAuditCoalesce(kiraschema.AdeDbAudit.Statement), kiraschema.AdeDbAudit.StatementLen,
	kiraschema.AdeDbAudit.StatementWithheld, kiraschema.AdeDbAudit.StatementTruncated,
	dbAuditCoalesce(kiraschema.AdeDbAudit.Summary), dbAuditCoalesce(kiraschema.AdeDbAudit.Error), kiraschema.AdeDbAudit.Subject,
	kiraschema.AdeDbAudit.Reason, kiraschema.AdeDbAudit.Stage, dbAuditCoalesce(kiraschema.AdeDbAudit.ExemptionJSON),
}

// ListForTask returns the task's calls newest first, each with its approvals oldest first.
func (r *DbAuditRepo) ListForTask(taskID string) ([]model.DbAuditCall, error) {
	a := kiraschema.AdeDbAudit
	sb := r.Dialect.Select(dbAuditCols...).From(a.Table)
	sb.Where(sb.Equal(a.TaskID, taskID)).OrderBy(a.StartedMs+" DESC", sqlb.RowID+" DESC")
	rows, err := sqlb.Query(r.DB, sb)
	all, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (dbAuditRow, bool, error) {
		var x dbAuditRow
		var withheld, truncated bool
		err := rows.Scan(&x.id, &x.callID, &x.taskID, &x.sessionID, &x.stepID, &x.runID, &x.startedMs, &x.endedMs,
			&x.status, &x.tool, &x.connID, &x.connName, &x.mode, &x.target, &x.statement, &x.stmtLen,
			&withheld, &truncated, &x.summary, &x.errText, &x.subject, &x.reason, &x.stage, &x.exemption)
		x.withheld, x.truncated = withheld, truncated
		return x, true, err
	})
	if err != nil {
		return nil, fmt.Errorf("repos: list db audit: %w", err)
	}
	var calls []model.DbAuditCall
	approvals := map[string][]model.DbAuditApproval{}
	for _, x := range all {
		if x.callID != "" {
			approvals[x.callID] = append(approvals[x.callID], model.DbAuditApproval{
				ID: x.id, CallID: x.callID, TaskID: x.taskID, SessionID: x.sessionID, StepID: x.stepID, RunID: x.runID,
				StartedMs: x.startedMs, EndedMs: x.endedMs, Outcome: x.status, Subject: x.subject, Reason: x.reason,
				Stage: x.stage, ExemptionJSON: x.exemption,
			})
			continue
		}
		calls = append(calls, model.DbAuditCall{
			ID: x.id, TaskID: x.taskID, SessionID: x.sessionID, StepID: x.stepID, RunID: x.runID,
			StartedMs: x.startedMs, EndedMs: x.endedMs, Status: x.status, Tool: x.tool, ConnectionID: x.connID,
			ConnectionName: x.connName, Mode: x.mode, Target: x.target, Statement: x.statement,
			StatementLen: x.stmtLen, StatementWithheld: x.withheld, StatementTruncated: x.truncated,
			Summary: x.summary, Error: x.errText,
		})
	}
	for i := range calls {
		a := approvals[calls[i].ID]
		for l, rr := 0, len(a)-1; l < rr; l, rr = l+1, rr-1 { // rows came newest first
			a[l], a[rr] = a[rr], a[l]
		}
		calls[i].Approvals = a
	}
	return calls, nil
}

type dbAuditRow struct {
	id, callID, taskID, sessionID, stepID, runID string
	startedMs                                    int64
	endedMs                                      *int64
	status, tool, connID, connName, mode, target string
	statement                                    string
	stmtLen                                      int
	withheld, truncated                          bool
	summary, errText, subject, reason, stage     string
	exemption                                    string
}

// ReconcileRunning turns calls left running by a quit into errors carrying msg and their pending
// approvals into abandoned.
func (r *DbAuditRepo) ReconcileRunning(msg string) error {
	a := kiraschema.AdeDbAudit
	calls := r.Dialect.Update(a.Table)
	calls.Set(calls.Assign(a.Status, model.DbAuditError), calls.Assign(a.Error, msg)).
		Where(calls.Equal(a.Kind, model.DbAuditKindCall), calls.Equal(a.Status, model.DbAuditRunning))
	if _, err := sqlb.Exec(r.DB, calls); err != nil {
		return fmt.Errorf("repos: reconcile db audit calls: %w", err)
	}
	approvals := r.Dialect.Update(a.Table)
	approvals.Set(approvals.Assign(a.Status, dbAuditAbandoned)).
		Where(approvals.Equal(a.Kind, model.DbAuditKindApproval), approvals.Equal(a.Status, model.DbAuditPending))
	if _, err := sqlb.Exec(r.DB, approvals); err != nil {
		return fmt.Errorf("repos: reconcile db audit approvals: %w", err)
	}
	return nil
}
