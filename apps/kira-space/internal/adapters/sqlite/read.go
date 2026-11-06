package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// quoteIdent is read.ts's quoteIdent.
var quoteIdent = adapters.QuoteIdentDouble

var (
	boolType     = regexp.MustCompile(`^BOOL`)
	temporalType = regexp.MustCompile(`^(DATE|DATETIME|TIMESTAMP)$`)
)

// typeClassFor is read.ts's typeClassFor — SQLite's own five type-affinity rules over
// the *declared* type string, plus three sugar cases no affinity rule covers on its own (BOOLEAN,
// the DATE family, JSON). 'other' for an undeclared or STRICT-table ANY column: the declared type
// is a hint at best.
func typeClassFor(declaredType string) page.TypeClass {
	base := strings.ToUpper(strings.TrimSpace(declaredType))
	switch {
	case base == "" || base == "ANY":
		return page.TypeClassOther
	case boolType.MatchString(base):
		return page.TypeClassBoolean
	case temporalType.MatchString(base):
		return page.TypeClassTemporal
	case base == "JSON":
		return page.TypeClassJSON
	case strings.Contains(base, "INT"):
		return page.TypeClassNumber
	case strings.Contains(base, "CHAR") || strings.Contains(base, "CLOB") || strings.Contains(base, "TEXT"):
		return page.TypeClassText
	case strings.Contains(base, "BLOB"):
		return page.TypeClassBinary
	case strings.Contains(base, "REAL") || strings.Contains(base, "FLOA") || strings.Contains(base, "DOUB"):
		return page.TypeClassNumber
	default:
		return page.TypeClassNumber // NUMERIC affinity catch-all: DECIMAL, NUMERIC, anything else undeclared
	}
}

// resolveKeysetColumnMeta is read.ts's own inline helper: a rowid table's own rowid is not a
// declared column, so it needs a synthetic ColumnMeta only for fetch purposes when it is the
// chosen tiebreaker (D22/F23) — never added to target.Columns, never shown as a page column (D23).
func resolveKeysetColumnMeta(target ReadTarget, name string) (model.ColumnMeta, error) {
	for _, c := range target.Columns {
		if c.Name == name {
			return c, nil
		}
	}
	if target.RowidColumn != nil && name == *target.RowidColumn {
		return model.ColumnMeta{Name: name, Position: -1, DataType: "INTEGER", Nullable: false}, nil
	}
	return model.ColumnMeta{}, adapters.New(adapters.CodeQuery, "keyset tiebreaker column not found: "+name, nil)
}

// selectExpr wraps a quoted identifier the way readPage's own SELECT list must, to route around a
// real modernc.org/sqlite driver behaviour that has nothing to do with SQLite itself: unlike
// node:sqlite (the whole "nothing switches on the declared type" claim held there), this driver's
// own Next() unconditionally re-parses a TEXT value into a Go time.Time whenever the column's
// declared type is DATE/DATETIME/TIMESTAMP and the stored text happens to look like a date —
// verified directly against the driver's own rows.go, then confirmed against a live query: a
// DATETIME column holding '2024-01-01 12:34:56' comes back as time.Time, silently, with no DSN
// option to disable it (unlike the unrelated, genuinely opt-in _inttotime/_texttotime flags). A
// CASE expression is not a "simple column reference" in SQLite's own sqlite3_column_decltype()
// terms, so wrapping every fetched column in one defeats the coercion — a NULL stays NULL, an
// INTEGER/REAL/BLOB value stored in a temporally-declared column still comes back as
// int64/float64/[]byte untouched, and a TEXT value comes back byte for byte, valid-looking date
// string included. Confirmed empirically (see CLAUDE.md's findings) against all six combinations,
// not assumed from reading the driver's source.
func selectExpr(ident string) string {
	return "CASE WHEN typeof(" + ident + ") = 'text' THEN " + ident + " || '' ELSE " + ident + " END"
}

// readReq is adapter.ts's ReadRequest minus Path.
type readReq = adapters.ReadReq

func questionPlaceholder(int) string { return "?" }

// readPage is read.ts's readPage.
func readPage(ctx context.Context, conn *sql.Conn, op *adapters.OpCtx, target ReadTarget, req readReq) (page.TabularPage, error) {
	// D22: the fallback chain is primary key, else a unique (all-NOT-NULL) index, else — a step
	// further than the other SQL adapters can offer — the table's own implicit rowid (F23), which
	// every rowid table has for free regardless of whether it declares a primary key at all.
	var extraTiebreaker []string
	if target.RowidColumn != nil {
		extraTiebreaker = []string{*target.RowidColumn}
	}
	plan, err := adapters.PlanRelationalPage(adapters.RelationalPageArgs{
		Columns: target.Columns, Projection: req.Projection,
		PrimaryKey: target.PrimaryKey, UniqueKeys: target.UniqueKeys,
		ExtraTiebreaker: extraTiebreaker,
		Sort:            req.Sort, CursorMode: req.Cursor.Mode,
		ResolveHidden: func(name string) (model.ColumnMeta, error) {
			return resolveKeysetColumnMeta(target, name)
		},
		TypeClassFor: typeClassFor,
		GeneratedFor: func(name string) bool { return target.GeneratedColumns[name] },
		QuoteIdent:   quoteIdent,
		Fingerprint: adapters.RelationalFingerprint[QualifiedName]{
			Path: target.QualifiedName, Projection: req.Projection, Filter: req.Filter,
			Sort: req.Sort, PageSize: req.PageSize, Conditions: req.Conditions,
		},
	})
	if err != nil {
		return page.TabularPage{}, err
	}
	projectedColumns, order, fetch := plan.ProjectedColumns, plan.Order, plan.Fetch
	columns := plan.Columns

	relationSQL := quoteIdent(target.QualifiedName.Database) + "." + quoteIdent(target.QualifiedName.Table)
	selectNames := make([]string, len(fetch.Columns))
	for i, c := range fetch.Columns {
		selectNames[i] = selectExpr(quoteIdent(c.Name))
	}
	selectList := strings.Join(selectNames, ", ")

	addParam, paramsPtr := adapters.NewParamAccumulator()
	whereSQL, err := adapters.ApplyConditions(adapters.WhereClause(req.Filter), req.Conditions, target.Columns, quoteIdent, questionPlaceholder, addParam, typeClassFor)
	if err != nil {
		return page.TabularPage{}, err
	}
	if plan.WantsKeyset {
		whereSQL, err = adapters.BuildKeysetWhereSQL(req, order, plan.Fingerprint, whereSQL, quoteIdent, questionPlaceholder, addParam, plan.IsBinaryColumn)
		if err != nil {
			return page.TabularPage{}, err
		}
	}

	query := adapters.BuildPageSQL(relationSQL, selectList, whereSQL, plan.OrderBySQL, req, questionPlaceholder, addParam)
	params := *paramsPtr

	// Streamed straight into the builder rather than materialized into a [][]any and
	// transposed afterward: BuildKeysetPosition's CellAt is only ever called for the first and last
	// displayed row (sqltext.go), so those two full-width fetch rows are the only ones worth
	// keeping around past the row's own AppendRow call. The SQL's own "LIMIT pageSize+1" guarantees
	// at most one row ever arrives past req.PageSize, so probing it needs no early cancellation —
	// the callback declines to push or track it.
	builder := page.NewTabularPageBuilder(columns)
	var collector adapters.KeysetPageCollector
	var firstRow, lastRow []any
	// cells is pure AppendRow scratch — unlike row (retained below via firstRow/lastRow), AppendRow
	// copies every cell into the builder's own scratch buffers synchronously and never keeps cells
	// itself, so one allocation for the whole page is enough instead of one per row (P2 R2, #95).
	cells := make([]*string, len(projectedColumns))
	err = streamArrayQuery(ctx, conn, query, params, op, true, func(row []any) error {
		if !collector.Track(req.PageSize) {
			return nil
		}

		for i := range cells {
			cells[i] = toCellText(row[i])
		}
		if err := builder.AppendRow(cells); err != nil {
			return err
		}

		if firstRow == nil {
			firstRow = row
		}
		lastRow = row
		return nil
	})
	if err != nil {
		return page.TabularPage{}, err
	}

	return collector.Finish(builder, plan, req, fetch, order,
		func() { firstRow, lastRow = lastRow, firstRow },
		func(row, col int) *string {
			if row == 0 {
				return toCellText(firstRow[col])
			}
			return toCellText(lastRow[col])
		})
}

// countRows is read.ts's countRows.
func countRows(ctx context.Context, conn *sql.Conn, op *adapters.OpCtx, target QualifiedName, filter *string) (adapters.CountResult, error) {
	relationSQL := quoteIdent(target.Database) + "." + quoteIdent(target.Table)
	query := adapters.BuildCountSQL(relationSQL, filter)

	rows, err := runArrayQuery(ctx, conn, query, nil, op, false)
	if err != nil {
		return adapters.CountResult{}, err
	}
	var raw any
	if len(rows) > 0 && len(rows[0]) > 0 {
		raw = rows[0][0]
	}
	value, err := adapters.ParseCountValue(raw)
	if err != nil {
		return adapters.CountResult{}, err
	}
	return adapters.CountResult{Value: value, Exact: true}, nil
}

// toCellText is read.ts's value->text codec: it switches on the *value's* own Go type,
// never the column's declared type — SQLite is dynamically typed, so a TEXT-declared column
// is free to hold a BLOB value and vice versa. The `0x<hex>` spelling is mysql-family's own
// convention, not a new one. The time.Time case has no TypeScript counterpart: it exists
// only because of the modernc.org/sqlite decltype coercion selectExpr's own comment documents —
// readPage's SELECT list already routes around it for the data-grid path, so this branch is reached
// only via the query console running a user's own raw "SELECT dt FROM t" against a
// DATE/DATETIME/TIMESTAMP column, which the adapter cannot rewrite (it is the user's own SQL text,
// not adapter-composed). Reformatted to SQLite's own canonical strftime('%Y-%m-%d %H:%M:%f')
// shape rather than passed through byte for byte in that one case — a narrow, documented
// capability trade (docs/ARCHITECTURE.md), not a silent gap.
func toCellText(value any) *string {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []byte:
		s := "0x" + hex.EncodeToString(v)
		return &s
	case int64:
		s := strconv.FormatInt(v, 10)
		return &s
	case float64:
		s := strconv.FormatFloat(v, 'g', -1, 64)
		return &s
	case string:
		return &v
	case time.Time:
		var s string
		if v.Nanosecond() == 0 {
			s = v.UTC().Format("2006-01-02 15:04:05")
		} else {
			s = v.UTC().Format("2006-01-02 15:04:05.999999999")
		}
		return &s
	default:
		s := ""
		return &s
	}
}
