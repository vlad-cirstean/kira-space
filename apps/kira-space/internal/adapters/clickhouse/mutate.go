package clickhouse

import (
	"context"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// assertInsertOnly is mutate.ts's own — D24/D25: MergeTree's PRIMARY KEY is a sparse index, not a
// unique key (F16) — there is no way to address "this one row" for an UPDATE or DELETE, so
// canUpdate/canDelete are permanently false (caps.go) and every non-insert op is refused.
func assertInsertOnly(op model.MutationRowOp) error {
	if op.Kind != "insert" {
		return adapters.New(adapters.CodeUnsupported,
			"ClickHouse only supports adding new rows (insert): a MergeTree PRIMARY KEY is a sparse "+
				"index, not a unique key, so there is no addressable row to update or delete", nil)
	}
	return nil
}

// literalFor is mutate.ts's own — D6/F27: backslash-then-quote, ClickHouse's own string-literal
// escaping (not SQL's standard doubled-quote convention every other adapter in this codebase
// renders with).
func literalFor(value *string) string {
	if value == nil {
		return "NULL"
	}
	escaped := strings.ReplaceAll(*value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}

// renderInsert is mutate.ts's own — every insert op in one plan renders as a single multi-row
// INSERT statement, cheaper than one round trip per row, and what mutate() executes, so
// preview() must render the same shape it would run. Columns are the union across every op, not
// assumed uniform: an op missing a given column pads that row's tuple with NULL rather than
// silently misaligning.
func renderInsert(relationSQL string, ops []model.MutationRowOp, render func(*string) string) string {
	var columns []string
	seen := make(map[string]bool)
	for _, op := range ops {
		for _, kv := range op.Values {
			if !seen[kv.Name] {
				seen[kv.Name] = true
				columns = append(columns, kv.Name)
			}
		}
	}
	quotedColumns := make([]string, len(columns))
	for i, c := range columns {
		quotedColumns[i] = quoteIdent(c)
	}
	rows := make([]string, len(ops))
	for i, op := range ops {
		byName := make(map[string]*string, len(op.Values))
		for _, kv := range op.Values {
			byName[kv.Name] = kv.Value
		}
		tuple := make([]string, len(columns))
		for j, c := range columns {
			tuple[j] = render(byName[c])
		}
		rows[i] = "(" + strings.Join(tuple, ", ") + ")"
	}
	return "INSERT INTO " + relationSQL + " (" + strings.Join(quotedColumns, ", ") + ") VALUES " + strings.Join(rows, ", ")
}

// preview is mutate.ts's own — synchronous (Adapter rule 3/§8.13): no catalog lookup, no network.
func preview(plan model.MutationPlan) ([]string, error) {
	database, table, err := adapters.ResolveDatabaseTablePath(plan.Path)
	if err != nil {
		return nil, err
	}
	relationSQL := quoteIdent(database) + "." + quoteIdent(table)
	for _, op := range plan.Ops {
		if err := assertInsertOnly(op); err != nil {
			return nil, err
		}
	}
	if len(plan.Ops) == 0 {
		return []string{}, nil
	}
	return []string{renderInsert(relationSQL, plan.Ops, literalFor)}, nil
}

// mutate is mutate.ts's own.
func mutate(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, readOnly bool, plan model.MutationPlan) (model.MutationResult, error) {
	if err := adapters.AssertWritable(readOnly); err != nil {
		return model.MutationResult{}, err
	}

	database, table, err := adapters.ResolveDatabaseTablePath(plan.Path)
	if err != nil {
		return model.MutationResult{}, err
	}
	for _, rowOp := range plan.Ops {
		if err := assertInsertOnly(rowOp); err != nil {
			return model.MutationResult{}, err
		}
	}
	if len(plan.Ops) == 0 {
		return model.MutationResult{AffectedRows: 0}, nil
	}

	// Fresh in this same op (D7, mirrors resolveProjection's own discipline) — never trusts a
	// column name the renderer sent without re-checking it against the catalog right now.
	target, err := getReadTarget(ctx, h, queryID, op, track, database, table)
	if err != nil {
		return model.MutationResult{}, err
	}
	for _, rowOp := range plan.Ops {
		if err := adapters.AssertColumnsKnown(target.Columns, rowOp.Values.Names()); err != nil {
			return model.MutationResult{}, err
		}
	}

	relationSQL := quoteIdent(database) + "." + quoteIdent(table)
	sql := renderInsert(relationSQL, plan.Ops, literalFor)
	op.SetCommand(sql)
	written, err := RunCommand(ctx, h, queryID, sql, op, track, nil)
	if err != nil {
		return model.MutationResult{}, err
	}
	if written > 0 {
		return model.MutationResult{AffectedRows: int(written)}, nil
	}
	return model.MutationResult{AffectedRows: len(plan.Ops)}, nil
}
