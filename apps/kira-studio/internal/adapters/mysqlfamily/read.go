package mysqlfamily

import (
	"context"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
)

// quoteIdent is read.ts's quoteIdent.
var quoteIdent = adapters.QuoteIdentBacktick

// binaryDatabaseTypes is B3's set: go-sql-driver's own DatabaseTypeName() already tells binary
// from text apart by the column's collation (fields.go's typeDatabaseName) — this package never
// needs to consult a collation table itself the way query.ts's typeCastString had to.
var binaryDatabaseTypes = map[string]bool{
	"BLOB": true, "TINYBLOB": true, "MEDIUMBLOB": true, "LONGBLOB": true,
	"BINARY": true, "VARBINARY": true, "GEOMETRY": true, "BIT": true,
}

// cellText is B3's cellText: a binary column's raw bytes render as 0x<hex> (D3, mirroring
// Postgres's bytea handling); everything else is the server's own text bytes, unmodified.
func cellText(raw []byte, dbType string) string {
	if binaryDatabaseTypes[dbType] {
		return "0x" + hex.EncodeToString(raw)
	}
	return string(raw)
}

var tinyint1 = regexp.MustCompile(`^tinyint\(1\)`)
var numberType = regexp.MustCompile(`^(tinyint|smallint|mediumint|int|integer|bigint|decimal|numeric|float|double)\b`)
var temporalType = regexp.MustCompile(`^(date|datetime|timestamp|time|year)\b`)

// binaryType classifies bit alongside binary/blob/geometry: binaryDatabaseTypes above
// already renders a BIT column's cellText as 0x<hex> (go-sql-driver's own DatabaseTypeName()
// reports it as "BIT"), but typeClassFor used to classify catalog type "bit(8)" as a number
// (matched by numberType above, which used to list bit too) — that mismatch let BinaryColumnsOf's
// isBinary lookup (sqlmutate.go) miss it, so NewParamRenderer bound the literal "0x05" display text
// straight into a BIT column instead of decoding it back to raw bytes, failing ("data too long") in
// strict mode or storing the wrong bits otherwise.
var binaryType = regexp.MustCompile(`^(binary|varbinary|tinyblob|blob|mediumblob|longblob|geometry|bit)\b`)

// typeClassFor is read.ts's typeClassFor — §5d's MariaDB/MySQL mapping. tinyint(1) is checked
// ahead of the general number match — it is how this family spells boolean.
func typeClassFor(dataType string) page.TypeClass {
	base := strings.ToLower(dataType)
	if tinyint1.MatchString(base) {
		return page.TypeClassBoolean
	}
	if numberType.MatchString(base) {
		return page.TypeClassNumber
	}
	if temporalType.MatchString(base) {
		return page.TypeClassTemporal
	}
	if strings.HasPrefix(base, "json") {
		return page.TypeClassJSON
	}
	if binaryType.MatchString(base) {
		return page.TypeClassBinary
	}
	return page.TypeClassText
}

// readReq is adapter.ts's ReadRequest minus Path.
type readReq = adapters.ReadReq

func questionPlaceholder(int) string { return "?" }

// readPage is read.ts's readPage — the same eleven-step shape as postgres/read.go's.
func readPage(ctx context.Context, conn Entry, op *adapters.OpCtx, track TrackQuery, target ReadTarget, req readReq) (page.TabularPage, error) {
	plan, err := adapters.PlanRelationalPage(adapters.RelationalPageArgs{
		Columns: target.Columns, Projection: req.Projection,
		PrimaryKey: target.PrimaryKey, UniqueKeys: target.UniqueKeys,
		Sort: req.Sort, CursorMode: req.Cursor.Mode,
		TypeClassFor: typeClassFor,
		// P36 D28: not detected here yet — false rather than a guess.
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

	relationSQL := quoteIdent(target.QualifiedName.Database) + "." + quoteIdent(target.QualifiedName.Table)
	selectNames := make([]string, len(fetch.Columns))
	for i, c := range fetch.Columns {
		selectNames[i] = quoteIdent(c.Name)
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

	// Streamed straight into the builder (P2 R1) rather than materialized into a [][]*string and
	// transposed afterward: BuildKeysetPosition's CellAt is only ever called for the first and last
	// displayed row (sqltext.go), so those two full-width fetch rows are the only ones worth keeping
	// around past the row's own AppendRow call. The SQL's own "LIMIT pageSize+1" (D24) guarantees at
	// most one row ever arrives past req.PageSize, so probing it needs no early cancellation — the
	// callback just declines to push or track it.
	builder := page.NewTabularPageBuilder(columns)
	var collector adapters.KeysetPageCollector
	var firstRow, lastRow []*string
	err = streamArrayQuery(ctx, conn, query, params, op, track, QueryOptions{LogParams: true}, func(row []*string) error {
		if !collector.Track(req.PageSize) {
			return nil
		}

		if err := builder.AppendRow(row[:len(projectedColumns)]); err != nil {
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
				return firstRow[col]
			}
			return lastRow[col]
		})
}

// countRows is read.ts's countRows.
func countRows(ctx context.Context, conn Entry, op *adapters.OpCtx, track TrackQuery, target QualifiedName, filter *string) (adapters.CountResult, error) {
	relationSQL := quoteIdent(target.Database) + "." + quoteIdent(target.Table)
	query := adapters.BuildCountSQL(relationSQL, filter)

	rows, err := runArrayQuery(ctx, conn, query, nil, op, track, QueryOptions{})
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
