package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/kiratime"
)

// OpLogRepo persists operation log records (migrations/0060_p415_op_log.sql).
type OpLogRepo struct {
	DB *sql.DB
}

// Upsert inserts or replaces the row for rec.ID.
func (r *OpLogRepo) Upsert(rec oplog.Record) error {
	steps, err := json.Marshal(rec.Steps)
	if err != nil {
		return fmt.Errorf("repos: marshal op steps: %w", err)
	}
	started, err := kiratime.ParseISO(rec.StartedAt)
	if err != nil {
		return fmt.Errorf("repos: parse op start %q: %w", rec.StartedAt, err)
	}
	bg := 0
	if rec.Background {
		bg = 1
	}
	_, err = r.DB.Exec(`INSERT INTO op_log (id, started_ms, duration_ms, category, kind, status, module, trigger_, source,
		subject, repo_root, repo_name, task_id, parent_id, command, error, steps_json, steps_dropped, background, repeat_count,
		connection_id, tab_id, rows, command_truncated, path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET duration_ms = excluded.duration_ms, status = excluded.status, source = excluded.source,
		subject = excluded.subject, command = excluded.command, error = excluded.error, steps_json = excluded.steps_json,
		steps_dropped = excluded.steps_dropped, repeat_count = excluded.repeat_count, rows = excluded.rows,
		command_truncated = excluded.command_truncated, path = excluded.path`,
		rec.ID, started.UnixMilli(), rec.DurationMs, rec.Category, rec.Kind, rec.Status, rec.Module, rec.Trigger, rec.Source,
		rec.Subject, rec.RepoRoot, rec.RepoName, rec.TaskID, rec.ParentID, rec.Command, rec.Error, string(steps),
		rec.StepsDropped, bg, rec.Repeat, rec.ConnectionID, rec.TabID, rec.Rows, rec.CommandTruncated, rec.Path)
	if err != nil {
		return fmt.Errorf("repos: upsert op %s: %w", rec.ID, err)
	}
	return nil
}

// Recent returns up to limit rows, newest first.
func (r *OpLogRepo) Recent(limit int) ([]oplog.Record, error) {
	rows, err := r.DB.Query(`SELECT `+opColumns+` FROM op_log ORDER BY started_ms DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("repos: list ops: %w", err)
	}
	defer rows.Close()
	out := []oplog.Record{}
	for rows.Next() {
		rec, _, err := scanOp(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// opColumns is the column list scanOp reads.
const opColumns = `id, started_ms, duration_ms, category, kind, status, module, trigger_, source, subject,
	repo_root, repo_name, task_id, parent_id, command, error, steps_json, steps_dropped, background, repeat_count,
	connection_id, tab_id, rows, command_truncated, path`

// scanOp reads one row of opColumns; withRowID means the query selected rowid first.
func scanOp(rows *sql.Rows, withRowID bool) (oplog.Record, int64, error) {
	var rec oplog.Record
	var steps string
	var bg, truncated int
	var startedMs, rid int64
	dest := []any{&rec.ID, &startedMs, &rec.DurationMs, &rec.Category, &rec.Kind, &rec.Status, &rec.Module,
		&rec.Trigger, &rec.Source, &rec.Subject, &rec.RepoRoot, &rec.RepoName, &rec.TaskID, &rec.ParentID,
		&rec.Command, &rec.Error, &steps, &rec.StepsDropped, &bg, &rec.Repeat,
		&rec.ConnectionID, &rec.TabID, &rec.Rows, &truncated, &rec.Path}
	if withRowID {
		dest = append([]any{&rid}, dest...)
	}
	if err := rows.Scan(dest...); err != nil {
		return rec, 0, fmt.Errorf("repos: scan op: %w", err)
	}
	rec.StartedAt = kiratime.FormatISO(time.UnixMilli(startedMs))
	rec.Background = bg != 0
	rec.CommandTruncated = truncated != 0
	rec.Steps = []oplog.Step{}
	if err := json.Unmarshal([]byte(steps), &rec.Steps); err != nil {
		return rec, 0, fmt.Errorf("repos: decode op steps %s: %w", rec.ID, err)
	}
	if rec.Steps == nil {
		rec.Steps = []oplog.Step{}
	}
	return rec, rid, nil
}

// Prune deletes rows older than cutoff, then the oldest beyond maxRows. Database rows have their own
// retention and cap (PruneCategory).
func (r *OpLogRepo) Prune(cutoff time.Time, maxRows int) error {
	if _, err := r.DB.Exec(`DELETE FROM op_log WHERE category <> 'db' AND started_ms < ?`, cutoff.UnixMilli()); err != nil {
		return fmt.Errorf("repos: prune ops by age: %w", err)
	}
	if _, err := r.DB.Exec(`DELETE FROM op_log WHERE id IN (SELECT id FROM op_log WHERE category <> 'db' ORDER BY started_ms DESC, id DESC LIMIT -1 OFFSET ?)`, maxRows); err != nil {
		return fmt.Errorf("repos: prune ops by count: %w", err)
	}
	return nil
}

// PruneCategory deletes one category's rows older than cutoff, then its oldest beyond maxRows.
func (r *OpLogRepo) PruneCategory(category string, cutoff time.Time, maxRows int) error {
	if _, err := r.DB.Exec(`DELETE FROM op_log WHERE category = ? AND started_ms < ?`, category, cutoff.UnixMilli()); err != nil {
		return fmt.Errorf("repos: prune %s ops by age: %w", category, err)
	}
	if _, err := r.DB.Exec(`DELETE FROM op_log WHERE id IN (SELECT id FROM op_log WHERE category = ? ORDER BY started_ms DESC, id DESC LIMIT -1 OFFSET ?)`, category, maxRows); err != nil {
		return fmt.Errorf("repos: prune %s ops by count: %w", category, err)
	}
	return nil
}

// ReconcileInterrupted turns rows left running by a quit into errors carrying msg. Foreign rows
// (id holds a '.') belong to a process that may outlive the app; the owner sweep handles them.
func (r *OpLogRepo) ReconcileInterrupted(msg string) error {
	if _, err := r.DB.Exec(`UPDATE op_log SET status = 'error', error = ? WHERE status = 'running' AND instr(id, '.') = 0`, msg); err != nil {
		return fmt.Errorf("repos: reconcile ops: %w", err)
	}
	return nil
}
