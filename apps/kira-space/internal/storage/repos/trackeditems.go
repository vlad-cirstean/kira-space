package repos

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// AdeTrackedItemsRepo reads and writes `ade_tracked_items`. Rows hold no SQL foreign key; the
// update watcher deletes the rows of a task or item that is gone.
type AdeTrackedItemsRepo struct {
	DB *sql.DB
}

func scanTrackedItem(rows *sql.Rows) (model.AdeTrackedItem, bool, error) {
	var it model.AdeTrackedItem
	err := rows.Scan(&it.TaskID, &it.ItemKey, &it.Kind, &it.SeenJSON, &it.CurrentJSON, &it.SeenAt, &it.ObservedAt, &it.ChangedAt)
	return it, true, err
}

// List returns every tracked item.
func (r *AdeTrackedItemsRepo) List() ([]model.AdeTrackedItem, error) {
	rows, err := r.DB.Query(`SELECT task_id, item_key, kind, seen_json, current_json, seen_at, observed_at, changed_at
		FROM ade_tracked_items ORDER BY task_id, item_key`)
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
	for _, it := range upserts {
		if _, err := tx.Exec(`INSERT INTO ade_tracked_items (task_id, item_key, kind, seen_json, current_json, seen_at, observed_at, changed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (task_id, item_key) DO UPDATE SET kind = excluded.kind, seen_json = excluded.seen_json,
				current_json = excluded.current_json, seen_at = excluded.seen_at, observed_at = excluded.observed_at,
				changed_at = excluded.changed_at`,
			it.TaskID, it.ItemKey, it.Kind, it.SeenJSON, it.CurrentJSON, it.SeenAt, it.ObservedAt, it.ChangedAt); err != nil {
			return fmt.Errorf("repos: upsert tracked item %s %s: %w", it.TaskID, it.ItemKey, err)
		}
	}
	for _, k := range deletes {
		if _, err := tx.Exec(`DELETE FROM ade_tracked_items WHERE task_id = ? AND item_key = ?`, k.TaskID, k.ItemKey); err != nil {
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
	q := `UPDATE ade_tracked_items SET seen_json = current_json, seen_at = ?, changed_at = 0
		WHERE task_id = ? AND seen_json != current_json`
	args := []any{now, taskID}
	if len(itemKeys) > 0 {
		q += ` AND item_key IN (?` + strings.Repeat(`, ?`, len(itemKeys)-1) + `)`
		for _, k := range itemKeys {
			args = append(args, k)
		}
	}
	res, err := r.DB.Exec(q, args...)
	if err != nil {
		return false, fmt.Errorf("repos: mark tracked items seen for %s: %w", taskID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("repos: mark tracked items seen for %s: %w", taskID, err)
	}
	return n > 0, nil
}
