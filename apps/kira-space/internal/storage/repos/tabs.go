package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// tabsColumns is the select list in Scan order, window_key excluded.
var tabsColumns = []string{
	kiraschema.Tabs.ID, kiraschema.Tabs.ConnectionID, kiraschema.Tabs.Path, kiraschema.Tabs.Kind,
	kiraschema.Tabs.StateJSON, kiraschema.Tabs.SortOrder, kiraschema.Tabs.Active, kiraschema.Tabs.WorkspaceID,
}

// tabsSelectAllSQL is the per-window list query repos.New prepares once.
var tabsSelectAllSQL = func() string {
	sb := sqlb.SQLite.Select(tabsColumns...).From(kiraschema.Tabs.Table)
	sb.Where(sb.Equal(kiraschema.Tabs.WindowKey, "")).OrderBy(kiraschema.Tabs.SortOrder + " ASC")
	q, _ := sb.Build()
	return q
}()

// TabsRepo reads and writes the `tabs` table: repository tabs carry a workspace, database-module
// tabs a connection, so both columns are nullable.
type TabsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect

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
		c := kiraschema.Tabs
		sb := r.Dialect.Select(tabsColumns...).From(c.Table)
		sb.Where(sb.Equal(c.WindowKey, windowKey)).OrderBy(c.SortOrder + " ASC")
		rows, err = sqlb.Query(r.DB, sb)
	}
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.TabRecord, bool, error) {
		var (
			id, path, kind, stateJSON string
			connectionID, workspaceID sql.NullString
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
		if !model.IsRenderableTabKind(kind) {
			slog.Warn("dropping tab row: unrecognised kind", "scope", "storage/tabs", "id", id, "kind", kind)
			return model.TabRecord{}, false, nil
		}
		rec := model.TabRecord{ID: id, Path: path, Kind: kind, State: json.RawMessage(stateJSON), Order: order, Active: active}
		if connectionID.Valid {
			rec.ConnectionID = &connectionID.String
		}
		if workspaceID.Valid {
			rec.WorkspaceID = &workspaceID.String
		}
		return rec, true, nil
	})
}

// Save replaces windowKey's own tab set in one transaction: validate up front, upsert-then-prune
// by id, scoped to windowKey so one window never erases another's tabs.
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

	c := kiraschema.Tabs
	return sqlb.ReplaceKeyed(r.DB, r.Dialect, c.Table, c.WindowKey, c.ID, windowKey, keep, func(tx *sql.Tx) error {
		for i, rec := range records {
			ib := r.Dialect.InsertInto(c.Table).Cols(append(append([]string{}, tabsColumns...), c.WindowKey)...).
				Values(rec.ID, rec.ConnectionID, rec.Path, rec.Kind, string(rec.State), i, rec.Active, rec.WorkspaceID, windowKey)
			sqlb.Upsert(ib, []string{c.ID},
				[]string{c.ConnectionID, c.Path, c.Kind, c.StateJSON, c.SortOrder, c.Active, c.WorkspaceID},
				c.Table+"."+c.WindowKey+" = excluded."+c.WindowKey)
			res, err := sqlb.Exec(tx, ib)
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
