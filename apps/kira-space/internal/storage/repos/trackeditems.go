package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// AdeTrackedItemsRepo reads and writes `ade_tracked_items`. Rows hold no SQL foreign key; the
// update watcher deletes the rows of a task or item that is gone.
type AdeTrackedItemsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

func scanTrackedItem(rows *sql.Rows) (model.AdeTrackedItem, bool, error) {
	var it model.AdeTrackedItem
	err := rows.Scan(&it.TaskID, &it.ItemKey, &it.Kind, &it.SeenJSON, &it.CurrentJSON, &it.SeenAt, &it.ObservedAt, &it.ChangedAt)
	return it, true, err
}

// List returns every tracked item.
func (r *AdeTrackedItemsRepo) List() ([]model.AdeTrackedItem, error) {
	c := kiraschema.AdeTrackedItems
	sb := r.Dialect.Select(c.TaskID, c.ItemKey, c.Kind, c.SeenJSON, c.CurrentJSON, c.SeenAt, c.ObservedAt, c.ChangedAt).From(c.Table).
		OrderBy(c.TaskID, c.ItemKey)
	rows, err := sqlb.Query(r.DB, sb)
	out, err := sqlitex.QueryAll(rows, err, scanTrackedItem)
	if err != nil {
		return nil, fmt.Errorf("repos: list tracked items: %w", err)
	}
	return out, nil
}

// Apply upserts and deletes in one transaction.
func (r *AdeTrackedItemsRepo) Apply(upserts []model.AdeTrackedItem, deletes []model.AdeTrackedKey) error {
	if len(upserts) == 0 && len(deletes) == 0 {
		return nil
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin apply tracked items: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	c := kiraschema.AdeTrackedItems
	for _, it := range upserts {
		ib := r.Dialect.InsertInto(c.Table).Cols(c.TaskID, c.ItemKey, c.Kind, c.SeenJSON, c.CurrentJSON, c.SeenAt, c.ObservedAt, c.ChangedAt).
			Values(it.TaskID, it.ItemKey, it.Kind, it.SeenJSON, it.CurrentJSON, it.SeenAt, it.ObservedAt, it.ChangedAt)
		sqlb.Upsert(ib, []string{c.TaskID, c.ItemKey}, []string{c.Kind, c.SeenJSON, c.CurrentJSON, c.SeenAt, c.ObservedAt, c.ChangedAt}, "")
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return fmt.Errorf("repos: upsert tracked item %s %s: %w", it.TaskID, it.ItemKey, err)
		}
	}
	for _, k := range deletes {
		del := r.Dialect.DeleteFrom(c.Table)
		del.Where(del.Equal(c.TaskID, k.TaskID), del.Equal(c.ItemKey, k.ItemKey))
		if _, err := sqlb.Exec(tx, del); err != nil {
			return fmt.Errorf("repos: delete tracked item %s %s: %w", k.TaskID, k.ItemKey, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit apply tracked items: %w", err)
	}
	return nil
}

// MarkSeen copies current to seen on the task's items, or on itemKeys when given, and reports
// whether any item changed.
func (r *AdeTrackedItemsRepo) MarkSeen(taskID string, itemKeys []string, now int64) (bool, error) {
	c := kiraschema.AdeTrackedItems
	ub := r.Dialect.Update(c.Table)
	ub.Set(c.SeenJSON+" = "+c.CurrentJSON, ub.Assign(c.SeenAt, now), ub.Assign(c.ChangedAt, 0))
	where := []string{ub.Equal(c.TaskID, taskID), c.SeenJSON + " <> " + c.CurrentJSON}
	if len(itemKeys) > 0 {
		where = append(where, ub.In(c.ItemKey, sqlb.Args(itemKeys)...))
	}
	ub.Where(where...)
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return false, fmt.Errorf("repos: mark tracked items seen for %s: %w", taskID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("repos: mark tracked items seen for %s: %w", taskID, err)
	}
	return n > 0, nil
}
