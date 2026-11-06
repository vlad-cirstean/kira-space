package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/huandu/go-sqlbuilder"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// OpLogRepo persists operation log records (migrations/0060_p415_op_log.sql).
type OpLogRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect

	upsertSQL sqlb.Cached
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
	// Shape never varies, so the statement builds once; args follow opColumns order.
	query := r.upsertSQL.SQL(func() sqlb.Builder {
		c := kiraschema.OpLog
		ib := r.Dialect.InsertInto(c.Table).Cols(opColumns...).Values(make([]any, len(opColumns))...)
		sqlb.Upsert(ib, []string{c.ID}, []string{c.DurationMs, c.Status, c.Source, c.Subject, c.Command, c.Error, c.StepsJSON,
			c.StepsDropped, c.RepeatCount, c.Rows, c.CommandTruncated, c.Path}, "")
		return ib
	})
	if _, err = r.DB.Exec(query,
		rec.ID, started.UnixMilli(), rec.DurationMs, rec.Category, rec.Kind, rec.Status, rec.Module, rec.Trigger, rec.Source,
		rec.Subject, rec.RepoRoot, rec.RepoName, rec.TaskID, rec.ParentID, rec.Command, rec.Error, string(steps),
		rec.StepsDropped, rec.Background, rec.Repeat, rec.ConnectionID, rec.TabID, rec.Rows, rec.CommandTruncated, rec.Path); err != nil {
		return fmt.Errorf("repos: upsert op %s: %w", rec.ID, err)
	}
	return nil
}

// Recent returns up to limit rows, newest first.
func (r *OpLogRepo) Recent(limit int) ([]oplog.Record, error) {
	rows, err := sqlb.Query(r.DB, r.recentQuery(limit))
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

func (r *OpLogRepo) recentQuery(limit int) *sqlbuilder.SelectBuilder {
	sb := r.Dialect.Select(opColumns...).From(kiraschema.OpLog.Table)
	return sb.OrderBy(kiraschema.OpLog.StartedMs+" DESC", kiraschema.OpLog.ID+" DESC").Limit(limit)
}

// opColumns is the column list scanOp reads, in scan order.
var opColumns = []string{
	kiraschema.OpLog.ID, kiraschema.OpLog.StartedMs, kiraschema.OpLog.DurationMs, kiraschema.OpLog.Category,
	kiraschema.OpLog.Kind, kiraschema.OpLog.Status, kiraschema.OpLog.Module, kiraschema.OpLog.Trigger,
	kiraschema.OpLog.Source, kiraschema.OpLog.Subject, kiraschema.OpLog.RepoRoot, kiraschema.OpLog.RepoName,
	kiraschema.OpLog.TaskID, kiraschema.OpLog.ParentID, kiraschema.OpLog.Command, kiraschema.OpLog.Error,
	kiraschema.OpLog.StepsJSON, kiraschema.OpLog.StepsDropped, kiraschema.OpLog.Background,
	kiraschema.OpLog.RepeatCount, kiraschema.OpLog.ConnectionID, kiraschema.OpLog.TabID, kiraschema.OpLog.Rows,
	kiraschema.OpLog.CommandTruncated, kiraschema.OpLog.Path,
}

// scanOp reads one row of opColumns; withRowID means the query selected rowid first.
func scanOp(rows *sql.Rows, withRowID bool) (oplog.Record, int64, error) {
	var rec oplog.Record
	var steps string
	var bg, truncated bool
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
	rec.Background = bg
	rec.CommandTruncated = truncated
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
	notDB := func(c *sqlbuilder.Cond) string { return c.NotEqual(kiraschema.OpLog.Category, oplog.CategoryDB) }
	if err := r.pruneAge(notDB, cutoff); err != nil {
		return fmt.Errorf("repos: prune ops by age: %w", err)
	}
	if err := r.pruneCount(notDB, maxRows); err != nil {
		return fmt.Errorf("repos: prune ops by count: %w", err)
	}
	return nil
}

// PruneCategory deletes one category's rows older than cutoff, then its oldest beyond maxRows.
func (r *OpLogRepo) PruneCategory(category string, cutoff time.Time, maxRows int) error {
	inCategory := func(c *sqlbuilder.Cond) string { return c.Equal(kiraschema.OpLog.Category, category) }
	if err := r.pruneAge(inCategory, cutoff); err != nil {
		return fmt.Errorf("repos: prune %s ops by age: %w", category, err)
	}
	if err := r.pruneCount(inCategory, maxRows); err != nil {
		return fmt.Errorf("repos: prune %s ops by count: %w", category, err)
	}
	return nil
}

func (r *OpLogRepo) pruneAge(scope func(*sqlbuilder.Cond) string, cutoff time.Time) error {
	c := kiraschema.OpLog
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(scope(&del.Cond), del.LessThan(c.StartedMs, cutoff.UnixMilli()))
	_, err := sqlb.Exec(r.DB, del)
	return err
}

// pruneCount keeps the newest maxRows rows of scope.
func (r *OpLogRepo) pruneCount(scope func(*sqlbuilder.Cond) string, maxRows int) error {
	_, err := sqlb.Exec(r.DB, r.pruneCountQuery(scope, maxRows))
	return err
}

func (r *OpLogRepo) pruneCountQuery(scope func(*sqlbuilder.Cond) string, maxRows int) *sqlbuilder.DeleteBuilder {
	c := kiraschema.OpLog
	old := r.Dialect.Select(c.ID).From(c.Table)
	old.Where(scope(&old.Cond)).OrderBy(c.StartedMs+" DESC", c.ID+" DESC")
	sqlb.OffsetOnly(r.Dialect, old, maxRows)
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(del.In(c.ID, old))
	return del
}

// ReconcileInterrupted turns rows left running by a quit into errors carrying msg. Foreign rows
// (id holds a '.') belong to a process that may outlive the app; the owner sweep handles them.
func (r *OpLogRepo) ReconcileInterrupted(msg string) error {
	c := kiraschema.OpLog
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Status, oplog.StatusError), ub.Assign(c.Error, msg)).
		Where(ub.Equal(c.Status, oplog.StatusRunning), ub.NotLike(c.ID, foreignIDLike))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: reconcile ops: %w", err)
	}
	return nil
}
