package sqlb

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// LeafTable names a key/value JSON table (ui_layout, settings).
type LeafTable struct{ Table, Key, Value string }

// Leaf is one key/value pair UpsertLeafList writes; Value is json.Marshal-ed.
type Leaf struct {
	Key   string
	Value any
}

// ScanLeafRows drains rows (or reports queryErr) into a key -> raw JSON map.
func ScanLeafRows(rows *sql.Rows, queryErr error) (map[string]json.RawMessage, error) {
	if queryErr != nil {
		return nil, fmt.Errorf("sqlb: leaf query: %w", queryErr)
	}
	defer rows.Close()
	stored := map[string]json.RawMessage{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("sqlb: leaf scan: %w", err)
		}
		stored[key] = json.RawMessage(value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlb: leaf rows: %w", err)
	}
	return stored, nil
}

// QueryLeaves reads every leaf of t. selectAll, when non-nil, is the caller's prepared form of the
// same select.
func QueryLeaves(db *sql.DB, d Dialect, selectAll *sql.Stmt, t LeafTable) (map[string]json.RawMessage, error) {
	if selectAll != nil {
		rows, err := selectAll.Query()
		return ScanLeafRows(rows, err)
	}
	rows, err := Query(db, d.Select(t.Key, t.Value).From(t.Table))
	return ScanLeafRows(rows, err)
}

// UpsertLeafList upserts every leaf into t, in order.
func UpsertLeafList(tx *sql.Tx, d Dialect, t LeafTable, leaves []Leaf) error {
	for _, l := range leaves {
		encoded, err := json.Marshal(l.Value)
		if err != nil {
			return fmt.Errorf("sqlb: encode %s: %w", l.Key, err)
		}
		ib := d.InsertInto(t.Table).Cols(t.Key, t.Value).Values(l.Key, string(encoded))
		Upsert(ib, []string{t.Key}, []string{t.Value}, "")
		if _, err := Exec(tx, ib); err != nil {
			return fmt.Errorf("sqlb: upsert %s: %w", l.Key, err)
		}
	}
	return nil
}

// UpdateLeaves runs begin, read leaves, apply, commit. selectAll, when non-nil, is the prepared
// select; it runs inside the transaction.
func UpdateLeaves(db *sql.DB, d Dialect, selectAll *sql.Stmt, t LeafTable, apply func(tx *sql.Tx, stored map[string]json.RawMessage) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("sqlb: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var rows *sql.Rows
	if selectAll != nil {
		rows, err = tx.Stmt(selectAll).Query()
	} else {
		rows, err = Query(tx, d.Select(t.Key, t.Value).From(t.Table))
	}
	stored, err := ScanLeafRows(rows, err)
	if err != nil {
		return err
	}
	if err := apply(tx, stored); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlb: commit: %w", err)
	}
	return nil
}

// ReplaceKeyed runs begin, upsert, delete rows of scope whose id is not in keys, commit. Empty
// keys deletes every row of the scope.
func ReplaceKeyed(db *sql.DB, d Dialect, table, scopeCol, idCol, scope string, keys []string, upsert func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("sqlb: replace %s: begin: %w", table, err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := upsert(tx); err != nil {
		return err
	}
	del := d.DeleteFrom(table)
	conds := []string{del.Equal(scopeCol, scope)}
	if len(keys) > 0 {
		ids := make([]any, len(keys))
		for i, k := range keys {
			ids[i] = k
		}
		conds = append(conds, del.NotIn(idCol, ids...))
	}
	del.Where(conds...)
	if _, err := Exec(tx, del); err != nil {
		return fmt.Errorf("sqlb: replace %s: prune: %w", table, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlb: replace %s: commit: %w", table, err)
	}
	return nil
}
