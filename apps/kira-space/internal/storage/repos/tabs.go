package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const tabsSelectAllSQL = `SELECT id, path, kind, state_json, "order", active, workspace_id FROM tabs WHERE window_key = ? ORDER BY "order" ASC`

// TabsRepo is Kira Studio's own TabsRepo, minus its `connection_id` column — this app has no
// connections concept, and WorkspaceID is written NOT NULL (0002_p100_tabs_layout.sql), never
// scanned as sql.NullString.
type TabsRepo struct {
	DB *sql.DB

	selectAll *sql.Stmt
}

func (r *TabsRepo) List(windowKey string) ([]model.TabRecord, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if r.selectAll != nil {
		rows, err = r.selectAll.Query(windowKey)
	} else {
		rows, err = r.DB.Query(tabsSelectAllSQL, windowKey)
	}
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.TabRecord, bool, error) {
		var (
			id, path, kind, stateJSON, workspaceID string
			order                                  int
			active                                 bool
		)
		if err := rows.Scan(&id, &path, &kind, &stateJSON, &order, &active, &workspaceID); err != nil {
			return model.TabRecord{}, false, err
		}
		if !model.IsJSONObject([]byte(stateJSON)) {
			slog.Warn("dropping tab row: state_json is not a JSON object", "scope", "storage/tabs", "id", id)
			return model.TabRecord{}, false, nil
		}
		if !model.IsRenderableTabKind(kind) {
			slog.Warn("dropping tab row: unrecognised kind", "scope", "storage/tabs", "id", id, "kind", kind)
			return model.TabRecord{}, false, nil
		}
		return model.TabRecord{
			ID: id, Path: path, Kind: kind, State: json.RawMessage(stateJSON),
			Order: order, Active: active, WorkspaceID: &workspaceID,
		}, true, nil
	})
}

// Save replaces windowKey's own tab set in one transaction — Kira Studio's own TabsRepo.Save,
// unchanged in shape (validate up front, upsert-then-prune by id, scoped to windowKey — F6's fix),
// minus the connection_id column.
func (r *TabsRepo) Save(windowKey string, records []model.TabRecord) error {
	for i, rec := range records {
		if err := rec.Validate(); err != nil {
			return fmt.Errorf("repos/tabs: record %d: %w", i, err)
		}
	}

	keep := make([]string, 0, len(records))
	for _, rec := range records {
		keep = append(keep, rec.ID)
	}

	return appstorage.ReplaceKeyed(r.DB, "tabs", "window_key", windowKey, keep, func(tx *sql.Tx) error {
		for i, rec := range records {
			res, err := tx.Exec(
				`INSERT INTO tabs (id, path, kind, state_json, "order", active, window_key, workspace_id)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(id) DO UPDATE SET
				   path          = excluded.path,
				   kind          = excluded.kind,
				   state_json    = excluded.state_json,
				   "order"       = excluded."order",
				   active        = excluded.active,
				   workspace_id  = excluded.workspace_id
				 WHERE tabs.window_key = excluded.window_key`,
				rec.ID, rec.Path, rec.Kind, string(rec.State), i, rec.Active, windowKey, rec.WorkspaceID,
			)
			if err != nil {
				return fmt.Errorf("repos/tabs: upsert %s: %w", rec.ID, err)
			}
			// 0 rows: the id conflicts with a row of another window, which the WHERE leaves untouched.
			if n, err := res.RowsAffected(); err != nil {
				return fmt.Errorf("repos/tabs: upsert %s: %w", rec.ID, err)
			} else if n == 0 {
				return fmt.Errorf("repos/tabs: tab %s: %w", rec.ID, appstorage.ErrTabOwnedElsewhere)
			}
		}
		return nil
	})
}
