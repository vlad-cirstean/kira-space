package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// MaxDbAuditCallsPerTask bounds one runaway agent; a normal run is tens of calls.
const MaxDbAuditCallsPerTask = 2000

// DbAuditRepo persists the Kira DB audit trail (migrations/0081_p547_db_audit.sql).
type DbAuditRepo struct {
	DB *sql.DB
}

// StartCall inserts a running call row and drops the task's oldest calls past the cap
// (their approvals follow by cascade).
func (r *DbAuditRepo) StartCall(c model.DbAuditCall) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin db audit call: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO ade_db_audit (id, task_id, kind, session_id, step_id, run_id, started_ms, status,
		tool, connection_id, connection_name, mode, target, statement, statement_len, statement_withheld,
		statement_truncated, summary)
		VALUES (?, ?, 'call', ?, ?, ?, ?, 'running', ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.TaskID, c.SessionID, c.StepID, c.RunID, c.StartedMs, c.Tool, c.ConnectionID, c.ConnectionName,
		c.Mode, c.Target, c.Statement, c.StatementLen, c.StatementWithheld, c.StatementTruncated, c.Summary); err != nil {
		return fmt.Errorf("repos: insert db audit call: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM ade_db_audit WHERE kind = 'call' AND task_id = ? AND id NOT IN (
		SELECT id FROM ade_db_audit WHERE kind = 'call' AND task_id = ? ORDER BY started_ms DESC, rowid DESC LIMIT ?)`,
		c.TaskID, c.TaskID, MaxDbAuditCallsPerTask); err != nil {
		return fmt.Errorf("repos: trim db audit: %w", err)
	}
	return tx.Commit()
}

// FinishCall sets a call's final status, end time and error text.
func (r *DbAuditRepo) FinishCall(id, status string, endedMs int64, errText string) error {
	if _, err := r.DB.Exec(`UPDATE ade_db_audit SET status = ?, ended_ms = ?, error = NULLIF(?, '') WHERE id = ? AND kind = 'call'`,
		status, endedMs, errText, id); err != nil {
		return fmt.Errorf("repos: finish db audit call %s: %w", id, err)
	}
	return nil
}

// SetStatement stores a call's redacted statement once it can be produced.
func (r *DbAuditRepo) SetStatement(id, statement string, length int, withheld, truncated bool) error {
	if _, err := r.DB.Exec(`UPDATE ade_db_audit SET statement = ?, statement_len = ?, statement_withheld = ?, statement_truncated = ?
		WHERE id = ? AND kind = 'call'`, statement, length, withheld, truncated, id); err != nil {
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
	if _, err := r.DB.Exec(`INSERT INTO ade_db_audit (id, task_id, call_id, kind, session_id, step_id, run_id, started_ms,
		ended_ms, status, subject, reason, stage, exemption_json)
		VALUES (?, ?, ?, 'approval', ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))`,
		a.ID, a.TaskID, a.CallID, a.SessionID, a.StepID, a.RunID, a.StartedMs, a.EndedMs, outcome,
		a.Subject, a.Reason, a.Stage, a.ExemptionJSON); err != nil {
		return fmt.Errorf("repos: insert db audit approval: %w", err)
	}
	return nil
}

// ResolveApproval sets a pending approval's outcome and end time.
func (r *DbAuditRepo) ResolveApproval(id, outcome string, endedMs int64) error {
	if _, err := r.DB.Exec(`UPDATE ade_db_audit SET status = ?, ended_ms = ? WHERE id = ? AND kind = 'approval'`,
		outcome, endedMs, id); err != nil {
		return fmt.Errorf("repos: resolve db audit approval %s: %w", id, err)
	}
	return nil
}

const dbAuditCols = `id, COALESCE(call_id, ''), task_id, session_id, step_id, run_id, started_ms, ended_ms, status, tool,
	COALESCE(connection_id, ''), connection_name, mode, target, COALESCE(statement, ''), statement_len,
	statement_withheld, statement_truncated, COALESCE(summary, ''), COALESCE(error, ''), subject, reason, stage,
	COALESCE(exemption_json, '')`

// ListForTask returns the task's calls newest first, each with its approvals oldest first.
func (r *DbAuditRepo) ListForTask(taskID string) ([]model.DbAuditCall, error) {
	rows, err := r.DB.Query(`SELECT `+dbAuditCols+` FROM ade_db_audit WHERE task_id = ? ORDER BY started_ms DESC, rowid DESC`, taskID)
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
	if _, err := r.DB.Exec(`UPDATE ade_db_audit SET status = 'error', error = ? WHERE kind = 'call' AND status = 'running'`, msg); err != nil {
		return fmt.Errorf("repos: reconcile db audit calls: %w", err)
	}
	if _, err := r.DB.Exec(`UPDATE ade_db_audit SET status = 'abandoned' WHERE kind = 'approval' AND status = 'pending'`); err != nil {
		return fmt.Errorf("repos: reconcile db audit approvals: %w", err)
	}
	return nil
}
