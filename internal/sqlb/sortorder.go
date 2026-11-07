package sqlb

import (
	"database/sql"
	"fmt"

	"github.com/huandu/go-sqlbuilder"
)

// NextSortOrder returns the append position for table's sort column: 0 for an empty (or empty-scope)
// set, one past the max otherwise. where, when non-nil, adds a condition built from sb's Cond methods.
func NextSortOrder(q Querier, d Dialect, table, sortCol string, where func(sb *sqlbuilder.SelectBuilder) string) (int, error) {
	sb := d.Select("COALESCE(MAX(" + sortCol + "), -1) + 1").From(table)
	if where != nil {
		sb.Where(where(sb))
	}
	var order int
	if err := QueryRow(q, sb).Scan(&order); err != nil {
		return 0, fmt.Errorf("sqlb: next sort order: %w", err)
	}
	return order, nil
}

// SortSet names one sibling set whose sort column is rewritten dense, 0..n-1. Select must select
// exactly Key, ordered the way the new order follows.
type SortSet struct {
	Table, Key, Order string
	Select            *sqlbuilder.SelectBuilder
}

func (s SortSet) readKeys(tx *sql.Tx) ([]string, error) {
	rows, err := Query(tx, s.Select)
	if err != nil {
		return nil, fmt.Errorf("read siblings: %w", err)
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scan sibling: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sibling rows: %w", err)
	}
	return keys, nil
}

func (s SortSet) write(tx *sql.Tx, d Dialect, keys []string) error {
	for order, k := range keys {
		ub := d.Update(s.Table)
		ub.Set(ub.Assign(s.Order, order)).Where(ub.Equal(s.Key, k))
		if _, err := Exec(tx, ub); err != nil {
			return fmt.Errorf("write %s: %w", k, err)
		}
	}
	return nil
}

// ReindexSortOrder rewrites a sibling set's sort column dense in the order the rows already have.
func ReindexSortOrder(tx *sql.Tx, d Dialect, set SortSet) error {
	keys, err := set.readKeys(tx)
	if err != nil {
		return fmt.Errorf("sqlb: reindex: %w", err)
	}
	if err := set.write(tx, d, keys); err != nil {
		return fmt.Errorf("sqlb: reindex: %w", err)
	}
	return nil
}

// ReorderSortOrder applies a caller order to one sibling set: keys first, in the given order, then
// every row keys leaves out in its existing order. A key outside the set, or listed twice, is an error.
func ReorderSortOrder(tx *sql.Tx, d Dialect, set SortSet, keys []string) error {
	existing, err := set.readKeys(tx)
	if err != nil {
		return fmt.Errorf("sqlb: reorder: %w", err)
	}
	inSet := make(map[string]bool, len(existing))
	for _, k := range existing {
		inSet[k] = true
	}
	ordered := make([]string, 0, len(existing))
	placed := make(map[string]bool, len(keys))
	for _, k := range keys {
		if !inSet[k] {
			return fmt.Errorf("sqlb: reorder: %s is not in this set", k)
		}
		if placed[k] {
			return fmt.Errorf("sqlb: reorder: %s listed twice", k)
		}
		placed[k] = true
		ordered = append(ordered, k)
	}
	for _, k := range existing {
		if !placed[k] {
			ordered = append(ordered, k)
		}
	}
	if err := set.write(tx, d, ordered); err != nil {
		return fmt.Errorf("sqlb: reorder: %w", err)
	}
	return nil
}
