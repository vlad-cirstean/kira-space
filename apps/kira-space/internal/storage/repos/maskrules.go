package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

var maskRulesSelectColumns = []string{
	kiraschema.ConnectionMaskRules.ID, kiraschema.ConnectionMaskRules.ConnectionID, kiraschema.ConnectionMaskRules.TableName,
	kiraschema.ConnectionMaskRules.ColumnName, kiraschema.ConnectionMaskRules.MaskKind, kiraschema.ConnectionMaskRules.KeepHint,
	kiraschema.ConnectionMaskRules.Correlate, kiraschema.ConnectionMaskRules.CreatedAt, kiraschema.ConnectionMaskRules.UpdatedAt,
}

// MaskRulesRepo reads connection_mask_rules, the projection of each connection file's masks: a
// selectColumns const, a scan*Row(rowScanner) helper, each query ordered deterministically.
type MaskRulesRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

func scanMaskRuleRow(row rowScanner) (model.MaskRule, error) {
	var (
		r                    model.MaskRule
		kind                 string
		keepHint, correlate  int
		createdMs, updatedMs int64
	)
	if err := row.Scan(
		&r.ID, &r.ConnectionID, &r.TableName, &r.ColumnName, &kind, &keepHint, &correlate,
		&createdMs, &updatedMs,
	); err != nil {
		return model.MaskRule{}, err
	}
	r.CreatedAt, r.UpdatedAt = kiratime.MsToISO(createdMs), kiratime.MsToISO(updatedMs)
	r.Kind = model.MaskKind(kind)
	r.KeepHint = keepHint != 0
	r.Correlate = correlate != 0
	return r, nil
}

// ListForConnection orders by lower(table_name), lower(column_name) — deterministic and matching
// the unique index's own ordering, so two rules that would collide under the case-insensitive
// constraint never come back in arbitrary order relative to each other.
func (r *MaskRulesRepo) ListForConnection(connectionID string) ([]model.MaskRule, error) {
	c := kiraschema.ConnectionMaskRules
	sb := r.Dialect.Select(maskRulesSelectColumns...).From(c.Table)
	sb.Where(sb.Equal(c.ConnectionID, connectionID)).OrderBy("lower("+c.TableName+")", "lower("+c.ColumnName+")")
	rows, err := sqlb.Query(r.DB, sb)
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
	sb := r.Dialect.Select(maskRulesSelectColumns...).From(kiraschema.ConnectionMaskRules.Table)
	sb.Where(sb.Equal(kiraschema.ConnectionMaskRules.ID, id))
	query, args := sb.Build()
	rec, err := sqlitex.QueryOne(r.DB, func(row *sql.Row) (*model.MaskRule, error) {
		m, err := scanMaskRuleRow(row)
		if err != nil {
			return nil, err
		}
		return &m, nil
	}, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repos/maskrules: get %s: %w", id, err)
	}
	return rec, nil
}

// CountByConnection returns every connection id that has at least one rule, mapped to its rule
// count — the Settings glance's own backend (§7.5), and the "does this connection have any rules
// at all" check §4.4's document/stream refusal and §6.2's toolbar-button visibility both need.
func (r *MaskRulesRepo) CountByConnection() (map[string]int, error) {
	c := kiraschema.ConnectionMaskRules
	sb := r.Dialect.Select(c.ConnectionID, "COUNT(*)").From(c.Table).GroupBy(c.ConnectionID)
	rows, err := sqlb.Query(r.DB, sb)
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
