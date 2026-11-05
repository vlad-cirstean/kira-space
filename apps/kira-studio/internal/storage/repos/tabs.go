package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const tabsSelectAllSQL = `SELECT id, connection_id, path, kind, state_json, "order", active, workspace_id FROM tabs WHERE window_key = ? ORDER BY "order" ASC`

type TabsRepo struct {
	DB *sql.DB

	// selectAll is prepared once by repos.New (P52 §5.4); nil when constructed directly, which
	// falls back to an ad-hoc query with identical SQL.
	selectAll *sql.Stmt
}

// List returns windowKey's own tab set — every window keeps an independent list (P8 F6), scoped
// by the `tabs_window` index (windowKey, "order").
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
			id, path, kind, stateJSON string
			connectionID              sql.NullString
			workspaceID               sql.NullString
			order                     int
			active                    bool
		)
		if err := rows.Scan(&id, &connectionID, &path, &kind, &stateJSON, &order, &active, &workspaceID); err != nil {
			return model.TabRecord{}, false, err
		}
		if !model.IsJSONObject([]byte(stateJSON)) {
			slog.Warn("dropping tab row: state_json is not a JSON object", "scope", "storage/tabs", "id", id)
			return model.TabRecord{}, false, nil
		}
		// No 'ddl'->'definition' coercion here (P53 §3.1: dropped alongside ops.go's, not
		// ported): the renderer has not written 'ddl' since P19, and a fresh kira.db cannot
		// contain one, so an unrecognised kind is simply dropped like any other.
		if !model.IsRenderableTabKind(kind) {
			slog.Warn("dropping tab row: unrecognised kind", "scope", "storage/tabs", "id", id, "kind", kind)
			return model.TabRecord{}, false, nil
		}
		rec := model.TabRecord{ID: id, Path: path, Kind: kind, State: json.RawMessage(stateJSON), Order: order, Active: active}
		if connectionID.Valid {
			v := connectionID.String
			rec.ConnectionID = &v
		}
		if workspaceID.Valid {
			v := workspaceID.String
			rec.WorkspaceID = &v
		}
		return rec, true, nil
	})
}

// Save replaces windowKey's own tab set in one transaction, rewriting `order` as the array index
// so the stored order is always dense (tabs.ts's replaceTabs). Every record is validated up front,
// the same envelope List() enforces on read: unlike List, which must tolerate legacy
// rows already on disk by dropping and logging, Save is the boundary where bad data should be
// refused outright rather than silently written and then vanish on the next restore.
//
// Scoping the DELETE by window_key is the fix: this used to be `DELETE FROM tabs` with no
// scope at all, so whichever window saved last erased every other open window's tabs outright —
// reproduced against a real two-client `-tags server` binary before this fix existed.
//
// This used to DELETE every row for the window and re-INSERT
// every record regardless of what changed — tabs.ts's saveDebounced() calls this once a second
// while someone types in a console tab, so an unchanged 1 MB `state_json` for nine untouched tabs
// was rewritten every second purely because a tenth tab's own text changed. `id` is the table's
// own PRIMARY KEY (globally unique — every tab id is a UUID, and the original code
// already assumed as much: an INSERT colliding with a row under a *different* window_key would
// have failed outright before this fix, same as it would now), so `ON CONFLICT(id) DO UPDATE`
// rewrites only the row for the record it was asked to write, and SQLite skips
// re-writing a page whose column values are byte-identical to what's already stored. The prune
// (appstorage.ReplaceKeyed) removes only what fell out of `records` (a closed tab), never a row
// that survived.
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
				`INSERT INTO tabs (id, connection_id, path, kind, state_json, "order", active, window_key, workspace_id)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(id) DO UPDATE SET
				   connection_id = excluded.connection_id,
				   path          = excluded.path,
				   kind          = excluded.kind,
				   state_json    = excluded.state_json,
				   "order"       = excluded."order",
				   active        = excluded.active,
				   workspace_id  = excluded.workspace_id
				 WHERE tabs.window_key = excluded.window_key`,
				rec.ID, rec.ConnectionID, rec.Path, rec.Kind, string(rec.State), i, rec.Active, windowKey, rec.WorkspaceID,
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
