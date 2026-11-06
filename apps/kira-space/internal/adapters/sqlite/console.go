package sqlite

import (
	"context"
	"database/sql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// ClassifyStatement satisfies adapters.StatementClassifier (M2) over the shared SQL classifier.
func (a *Adapter) ClassifyStatement(_ context.Context, statement string) (adapters.OpClass, error) {
	return adapters.ClassifySQL(statement, adapters.SQLiteDialect), nil
}

// columnsFor is console.ts's own — a column's *declared* origin type ('INTEGER', 'TEXT', \"\" for
// an expression or an untyped column), the same vocabulary pragma_table_xinfo uses. typeClassFor is
// exactly the read path's function, reused unchanged. execute() never consults the catalog —
// nullability/PK-ness are unknowable here; console results are always read-only regardless.
func columnsFor(names, declTypes []string) []page.ColumnDescriptor {
	columns := make([]page.ColumnDescriptor, len(names))
	for i, name := range names {
		columns[i] = page.ColumnDescriptor{
			Name: name, DataType: declTypes[i], TypeClass: typeClassFor(declTypes[i]),
			Nullable: true, IsPrimaryKey: false, Generated: false,
		}
	}
	return columns
}

// runOneStatement is console.ts's own — a non-row-returning statement (INSERT/UPDATE/DELETE/
// DDL/pragma) is told apart from a SELECT by QueryContext's own zero-column result, the same signal
// mysqlfamily's console.go uses (confirmed there; verified independently for
// modernc.org/sqlite too, not assumed from that unrelated driver's behaviour).
func runOneStatement(ctx context.Context, conn *sql.Conn, sqlText string, limit page.ResultCap) (page.TabularPage, error) {
	if err := assertSingleStatement(sqlText); err != nil {
		return page.TabularPage{}, err
	}
	rows, err := conn.QueryContext(ctx, sqlText)
	if err != nil {
		return page.TabularPage{}, mapError(err)
	}
	defer rows.Close()

	types, err := rows.ColumnTypes()
	if err != nil {
		return page.TabularPage{}, mapError(err)
	}
	if len(types) == 0 {
		// A non-row-returning statement is read through QueryContext (never ExecContext) so the
		// zero-column signal above is what tells the two apart — but database/sql's *sql.Rows
		// exposes no RowsAffected count at all (that lives only on the driver.Result ExecContext
		// returns), so unlike node:sqlite's own `${result.changes} row(s) affected`, this reports a
		// generic "OK" — the same capability loss already documented for mysql-family, for the
		// same reason: this is a database/sql API shape, not a driver-specific gap.
		return adapters.SingleStatusPage("OK", "text"), nil
	}

	names := make([]string, len(types))
	declTypes := make([]string, len(types))
	for i, t := range types {
		names[i] = t.Name()
		declTypes[i] = t.DatabaseTypeName()
	}
	columns := columnsFor(names, declTypes)

	builder := page.NewTabularPageBuilder(columns)
	truncated := false
	for rows.Next() {
		// Next just proved another row exists; stopping here makes truncated mean "more existed".
		if limit.Reached(builder.RowCount(), builder.Bytes()) {
			truncated = true
			break
		}
		vals := make([]any, len(types))
		dest := make([]any, len(types))
		for i := range vals {
			dest[i] = &vals[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return page.TabularPage{}, mapError(err)
		}
		cells := make([]*string, len(vals))
		for i, v := range vals {
			cells[i] = toCellText(v)
		}
		if err := builder.AppendRow(cells); err != nil {
			return page.TabularPage{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return page.TabularPage{}, mapError(err)
	}
	return builder.Finish(page.CappedPosition(builder.RowCount(), truncated)), nil
}

// execute is console.ts's own execute.
func execute(ctx context.Context, conn *sql.Conn, op *adapters.OpCtx, statements []string, limit page.ResultCap) ([]page.Page, error) {
	if len(statements) == 0 {
		return nil, adapters.T(adapters.CodeQuery, "backend.studio.adapter.noStatements.body", nil, nil)
	}
	// One op-log row for the whole batch (P5 D9's precedent), not one per statement.
	op.SetCommand(adapters.JoinConsoleStatements(statements, "--"))

	pages := make([]page.Page, len(statements))
	for i, stmt := range statements {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return nil, err
		}
		p, err := runOneStatement(ctx, conn, stmt, limit)
		if err != nil {
			return nil, err
		}
		pages[i] = p
		limit = adapters.RemainingCap(limit, p.Size())
	}
	return pages, nil
}

// RedactStatement satisfies adapters.StatementRedactor: string literals masked, comments dropped.
func (a *Adapter) RedactStatement(statement string) (string, bool) {
	return adapters.MaskSQLLiterals(statement, adapters.SQLiteDialect)
}
