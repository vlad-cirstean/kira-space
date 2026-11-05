package postgres

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// quoteIdent is read.ts's quoteIdent. The NUL check is unreachable with any real Postgres
// identifier — the server stores them as NUL-terminated C strings, so one physically cannot
// contain a NUL byte — but it stays, matching the TypeScript's own defensive check; panicking
// (rather than threading an error return through every quoteIdent call site) is fine specifically
// because this can only ever fire from Host.RunOp's own recover() boundary, which turns
// it into a failed op rather than a crash — the exact reason that boundary exists.
var quoteIdent = adapters.QuoteIdentDouble

var numericTypePrefix = regexp.MustCompile(`^(int2|int4|int8|smallint|integer|bigint|numeric|decimal|real|double precision|float4|float8|money)\b`)
var temporalTypePrefix = regexp.MustCompile(`^(date|time|timetz|timestamp|timestamptz|interval)\b`)

// typeClassFor is read.ts's typeClassFor — §5d's Postgres mapping. Array types (`_`-prefixed /
// `[]`-suffixed format_type() output) are checked first since they would otherwise match a
// base-type prefix (e.g. "integer[]").
func typeClassFor(dataType string) page.TypeClass {
	base := strings.ToLower(dataType)
	if strings.HasPrefix(base, "_") || strings.HasSuffix(base, "[]") {
		return page.TypeClassOther
	}
	if numericTypePrefix.MatchString(base) {
		return page.TypeClassNumber
	}
	if base == "boolean" || base == "bool" {
		return page.TypeClassBoolean
	}
	if temporalTypePrefix.MatchString(base) {
		return page.TypeClassTemporal
	}
	if base == "json" || base == "jsonb" {
		return page.TypeClassJSON
	}
	if base == "bytea" {
		return page.TypeClassBinary
	}
	return page.TypeClassText
}

// normalizeCellText is read.ts's normalizeCellText: bytea in text mode arrives as `\x…` (the hex
// bytea_output default since Postgres 9.0) — normalised to the app-wide `0x…` binary convention
// (D3, mirrored by MariaDB's blob handling). Reused by console.go for query results.
func normalizeCellText(value string, typeClass page.TypeClass) string {
	if typeClass == page.TypeClassBinary && strings.HasPrefix(value, `\x`) {
		return "0x" + value[2:]
	}
	return value
}

// readReq is adapter.ts's ReadRequest minus Path — the request shape readPage actually consumes.
type readReq = adapters.ReadReq

// assertReadOnlyFilterSortSafe is the guard, called from Read/Count before either
// touches a connection: a grid filter and a text-sort clause are raw SQL fragments concatenated
// straight into a WHERE/ORDER BY and run over pgx's simple protocol (query.go's TextMode) with no
// wrapping read-only transaction and no classification of their own — unlike a console statement,
// which AssertNoTransactionEscalation already covers. Postgres's simple protocol runs every
// statement found in a string it is handed, so on a read-only connection either input hiding a
// second top-level statement is rejected outright rather than let it run. Only gated on readOnly:
// a non-read-only connection's own MCP write-permission gate is a separate concern this function
// does not attempt to close.
func assertReadOnlyFilterSortSafe(readOnly bool, filter *string, sort *model.SortSpec) error {
	if !readOnly {
		return nil
	}
	if filter != nil {
		if err := adapters.AssertNoHiddenStatement(*filter, adapters.PostgresDialect); err != nil {
			return err
		}
	}
	if sort != nil && sort.Kind == "text" {
		if err := adapters.AssertNoHiddenStatement(sort.Text, adapters.PostgresDialect); err != nil {
			return err
		}
	}
	return nil
}

// readPage is read.ts's readPage — the densest function in the package.
func readPage(ctx context.Context, conn *trackedConn, op *adapters.OpCtx, track TrackQuery, target ReadTarget, req readReq) (page.TabularPage, error) {
	plan, err := adapters.PlanRelationalPage(adapters.RelationalPageArgs{
		Columns: target.Columns, Projection: req.Projection,
		PrimaryKey: target.PrimaryKey, UniqueKeys: target.UniqueKeys,
		Sort: req.Sort, CursorMode: req.Cursor.Mode,
		TypeClassFor: typeClassFor,
		// P36 D28: not detected here yet (definition.go's own attgenerated is the only place this
		// adapter currently reads it) — false rather than a guess.
		GeneratedFor: func(string) bool { return false },
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

	relationSQL := quoteIdent(target.QualifiedName.Schema) + "." + quoteIdent(target.QualifiedName.Relation)
	selectNames := make([]string, len(fetch.Columns))
	for i, c := range fetch.Columns {
		selectNames[i] = quoteIdent(c.Name)
	}
	selectList := strings.Join(selectNames, ", ")

	var params []any
	addParam := func(value any) int {
		params = append(params, value)
		return len(params)
	}

	whereSQL, err := adapters.ApplyConditions(adapters.WhereClause(req.Filter), req.Conditions, target.Columns, quoteIdent, dollarPlaceholder, addParam, typeClassFor)
	if err != nil {
		return page.TabularPage{}, err
	}
	if plan.WantsKeyset {
		whereSQL, err = adapters.BuildKeysetWhereSQL(req, order, plan.Fingerprint, whereSQL, quoteIdent, dollarPlaceholder, addParam, plan.IsBinaryColumn)
		if err != nil {
			return page.TabularPage{}, err
		}
	}

	sql := adapters.BuildPageSQL(relationSQL, selectList, whereSQL, plan.OrderBySQL, req, dollarPlaceholder, addParam)

	// Streamed straight into the builder (P2 R1) rather than materialized into a [][]*string and
	// transposed afterward: BuildKeysetPosition's CellAt is only ever called for the first and last
	// displayed row (sqltext.go), so those two full-width fetch rows are the only ones worth keeping
	// around past the row's own AppendRow call. The SQL's own "LIMIT pageSize+1" (D24) guarantees at
	// most one row ever arrives past req.PageSize, so probing it needs no early cancellation — the
	// callback just declines to push or track it.
	builder := page.NewTabularPageBuilder(columns)
	var collector adapters.KeysetPageCollector
	var firstRow, lastRow []*string
	err = streamArrayQuery(ctx, conn, sql, params, op, track, QueryOptions{TextMode: true, LogParams: true}, func(row []*string) error {
		if !collector.Track(req.PageSize) {
			return nil
		}

		visible := row[:len(projectedColumns)]
		values := make([]*string, len(visible))
		for i, v := range visible {
			// normalizeCellText only ever rewrites a TypeClassBinary cell (the `\x…` -> `0x…`
			// bytea_output rewrite) — every other type class returns its input string unchanged, so
			// reusing the already-scanned *string directly avoids a needless extra allocation +
			// pointer per cell for what is, on any non-binary column, every row of the page.
			if v == nil || columns[i].TypeClass != page.TypeClassBinary {
				values[i] = v
				continue
			}
			normalized := normalizeCellText(*v, columns[i].TypeClass)
			values[i] = &normalized
		}
		if err := builder.AppendRow(values); err != nil {
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
			raw := lastRow[col]
			if row == 0 {
				raw = firstRow[col]
			}
			if raw == nil {
				return nil
			}
			// Keyset tokens carry the app-wide 0x<hex> binary spelling, not bytea's \x<hex>.
			normalized := normalizeCellText(*raw, typeClassFor(fetch.Columns[col].DataType))
			return &normalized
		})
}

func dollarPlaceholder(i int) string { return "$" + strconv.Itoa(i) }

// countRows is read.ts's countRows.
func countRows(ctx context.Context, conn *trackedConn, op *adapters.OpCtx, track TrackQuery, target QualifiedName, filter *string) (adapters.CountResult, error) {
	relationSQL := quoteIdent(target.Schema) + "." + quoteIdent(target.Relation)
	sql := adapters.BuildCountSQL(relationSQL, filter)

	rows, err := runArrayQuery(ctx, conn, sql, nil, op, track, QueryOptions{TextMode: true})
	if err != nil {
		return adapters.CountResult{}, err
	}
	var raw any
	if len(rows) > 0 && len(rows[0]) > 0 && rows[0][0] != nil {
		raw = *rows[0][0]
	}
	value, err := adapters.ParseCountValue(raw)
	if err != nil {
		return adapters.CountResult{}, err
	}
	return adapters.CountResult{Value: value, Exact: true}, nil
}
