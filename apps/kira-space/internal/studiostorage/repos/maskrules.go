package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const maskRulesSelectColumns = `
	id, connection_id, table_name, column_name, mask_kind, keep_hint, correlate, created_at, updated_at
`

// MaskRulesRepo reads connection_mask_rules, the projection of each connection file's masks: a
// selectColumns const, a scan*Row(rowScanner) helper, each query ordered deterministically.
type MaskRulesRepo struct {
	DB *sql.DB
}

func scanMaskRuleRow(row rowScanner) (model.MaskRule, error) {
	var (
		r                   model.MaskRule
		kind                string
		keepHint, correlate int
	)
	if err := row.Scan(
		&r.ID, &r.ConnectionID, &r.TableName, &r.ColumnName, &kind, &keepHint, &correlate,
		&r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return model.MaskRule{}, err
	}
	r.Kind = model.MaskKind(kind)
	r.KeepHint = keepHint != 0
	r.Correlate = correlate != 0
	return r, nil
}

// queryer is satisfied by both *sql.DB and *sql.Tx — listForConnection (below) runs the identical
// query over either, so CopyForConnection's own transaction (F8) can share it with ListForConnection
// rather than keeping a second, drifting copy of the same query/scan loop.
type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// ListForConnection orders by lower(table_name), lower(column_name) — deterministic and matching
// the unique index's own ordering, so two rules that would collide under the case-insensitive
// constraint never come back in arbitrary order relative to each other.
func (r *MaskRulesRepo) ListForConnection(connectionID string) ([]model.MaskRule, error) {
	return listForConnection(r.DB, connectionID)
}

func listForConnection(q queryer, connectionID string) ([]model.MaskRule, error) {
	rows, err := q.Query(
		`SELECT `+maskRulesSelectColumns+` FROM connection_mask_rules WHERE connection_id = ? ORDER BY lower(table_name), lower(column_name)`,
		connectionID,
	)
	if err != nil {
		return nil, fmt.Errorf("repos/maskrules: list %s: %w", connectionID, err)
	}
	defer rows.Close()

	out := []model.MaskRule{}
	for rows.Next() {
		rec, err := scanMaskRuleRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repos/maskrules: scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/maskrules: rows: %w", err)
	}
	return out, nil
}

// Get reads one rule by id, (nil, nil) when not found.
func (r *MaskRulesRepo) Get(id string) (*model.MaskRule, error) {
	rec, err := sqlitex.QueryOne(r.DB, func(row *sql.Row) (*model.MaskRule, error) {
		m, err := scanMaskRuleRow(row)
		if err != nil {
			return nil, err
		}
		return &m, nil
	}, `SELECT `+maskRulesSelectColumns+` FROM connection_mask_rules WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("repos/maskrules: get %s: %w", id, err)
	}
	return rec, nil
}

// CountByConnection returns every connection id that has at least one rule, mapped to its rule
// count — the Settings glance's own backend (§7.5), and the "does this connection have any rules
// at all" check §4.4's document/stream refusal and §6.2's toolbar-button visibility both need.
func (r *MaskRulesRepo) CountByConnection() (map[string]int, error) {
	rows, err := r.DB.Query(`SELECT connection_id, COUNT(*) FROM connection_mask_rules GROUP BY connection_id`)
	if err != nil {
		return nil, fmt.Errorf("repos/maskrules: count by connection: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var connID string
		var count int
		if err := rows.Scan(&connID, &count); err != nil {
			return nil, fmt.Errorf("repos/maskrules: scan count: %w", err)
		}
		out[connID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/maskrules: rows: %w", err)
	}
	return out, nil
}
