package repos

import (
	"fmt"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

var _ oplog.ForeignStore = (*OpLogRepo)(nil)

// DataVersion moves only when another connection commits; the app's handle is one connection.
func (r *OpLogRepo) DataVersion() (int64, error) {
	var v int64
	if err := r.DB.QueryRow(`PRAGMA data_version`).Scan(&v); err != nil {
		return 0, fmt.Errorf("repos: data_version: %w", err)
	}
	return v, nil
}

// foreignIDLike matches ids written by another process (the id holds a '.').
const foreignIDLike = "%.%"

// MaxRowID returns the newest op_log rowid, 0 for an empty table.
func (r *OpLogRepo) MaxRowID() (int64, error) {
	var v int64
	sb := r.Dialect.Select("COALESCE(MAX(" + sqlb.RowID + "), 0)").From(kiraschema.OpLog.Table)
	if err := sqlb.QueryRow(r.DB, sb).Scan(&v); err != nil {
		return 0, fmt.Errorf("repos: max op rowid: %w", err)
	}
	return v, nil
}

// ForeignAfter returns rows written by another process (their id holds a '.') with a rowid past
// rowid or an id in ids, oldest first, and the newest rowid seen (rowid when none is newer).
func (r *OpLogRepo) ForeignAfter(rowid int64, ids []string) ([]oplog.Record, int64, error) {
	c := kiraschema.OpLog
	sb := r.Dialect.Select(append([]string{sqlb.RowID}, opColumns...)...).From(c.Table)
	newer := []string{sb.GreaterThan(sqlb.RowID, rowid)}
	if len(ids) > 0 {
		newer = append(newer, sb.In(c.ID, sqlb.Args(ids)...))
	}
	sb.Where(sb.Like(c.ID, foreignIDLike), sb.Or(newer...)).OrderBy(sqlb.RowID)
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, rowid, fmt.Errorf("repos: list foreign ops: %w", err)
	}
	defer rows.Close()
	out, top := []oplog.Record{}, rowid
	for rows.Next() {
		rec, rid, err := scanOp(rows, true)
		if err != nil {
			return nil, rowid, err
		}
		top = max(top, rid)
		out = append(out, rec)
	}
	return out, top, rows.Err()
}

// RunningForeignIDs lists the ids of foreign rows still marked running.
func (r *OpLogRepo) RunningForeignIDs() ([]string, error) {
	c := kiraschema.OpLog
	sb := r.Dialect.Select(c.ID).From(c.Table)
	sb.Where(sb.Equal(c.Status, oplog.StatusRunning), sb.Like(c.ID, foreignIDLike))
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: list running foreign ops: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("repos: scan running foreign op: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FailRunning turns the rows among ids that are still running into errors carrying msg.
func (r *OpLogRepo) FailRunning(ids []string, msg string) error {
	if len(ids) == 0 {
		return nil
	}
	c := kiraschema.OpLog
	ub := r.Dialect.Update(c.Table)
	ub.Set(
		ub.Assign(c.Status, oplog.StatusError),
		ub.Assign(c.Error, msg),
		c.DurationMs+" = MAX(0, "+ub.Var(time.Now().UnixMilli())+" - "+c.StartedMs+")",
	).Where(ub.Equal(c.Status, oplog.StatusRunning), ub.In(c.ID, sqlb.Args(ids)...))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: fail running ops: %w", err)
	}
	return nil
}
