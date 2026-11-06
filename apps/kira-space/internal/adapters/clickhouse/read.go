package clickhouse

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// quoteIdent is read.ts's own — ClickHouse quotes identifiers with backticks, the same
// character its own create_table_query output uses.
//
// ClickHouse's lexer reads a backtick-quoted identifier with the same backslash-escape
// rules as a string literal, not just SQL-style doubled-quote escaping — a raw backslash must be
// doubled too, or a name ending in one (or containing "\`") lets the following character,
// including the identifier's own closing backtick, be consumed as an escape sequence instead of
// terminating the identifier, corrupting the query or letting subsequent text run as SQL.
func quoteIdent(name string) string {
	if strings.ContainsRune(name, '\x00') {
		panic(adapters.Contract("identifier contains a NUL byte", nil))
	}
	escaped := strings.ReplaceAll(name, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "`", "``")
	return "`" + escaped + "`"
}

var (
	nullableRE      = regexp.MustCompile(`(?s)^Nullable\((.*)\)$`)
	lowCardinalityR = regexp.MustCompile(`(?s)^LowCardinality\((.*)\)$`)
	baseTypeNameRE  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*`)
)

// unwrapType is read.ts's own — D17: unwraps Nullable(...) and LowCardinality(...) recursively, in
// either nesting order, since LowCardinality(Nullable(String)) and Nullable(LowCardinality(String))
// are both real, common column types (F24).
func unwrapType(declared string) (inner string, nullable bool) {
	inner = strings.TrimSpace(declared)
	for {
		if m := nullableRE.FindStringSubmatch(inner); m != nil {
			nullable = true
			inner = strings.TrimSpace(m[1])
			continue
		}
		if m := lowCardinalityR.FindStringSubmatch(inner); m != nil {
			inner = strings.TrimSpace(m[1])
			continue
		}
		break
	}
	return inner, nullable
}

func baseTypeName(inner string) string {
	if m := baseTypeNameRE.FindString(inner); m != "" {
		return m
	}
	return inner
}

var (
	numberTypeRE  = regexp.MustCompile(`^(Int|UInt|Float|Decimal)`)
	textTypes     = map[string]bool{"String": true, "FixedString": true, "UUID": true, "IPv4": true, "IPv6": true, "Enum8": true, "Enum16": true}
	temporalTypes = map[string]bool{"Date": true, "Date32": true, "DateTime": true, "DateTime64": true, "Time": true, "Time64": true}
	jsonTypes     = map[string]bool{
		"JSON": true, "Dynamic": true, "Variant": true, "Array": true, "Tuple": true, "Map": true,
		"Nested": true, "Point": true, "Ring": true, "Polygon": true, "MultiPolygon": true,
	}
)

// typeClassFor is read.ts's own — D17: 'String' -> text, never 'binary'; the composite and
// semi-structured types -> 'json'; 'other' for AggregateFunction/SimpleAggregateFunction/Nothing/
// Interval/anything unrecognised, never guessed at.
func typeClassFor(declared string) page.TypeClass {
	inner, _ := unwrapType(declared)
	name := baseTypeName(inner)
	switch {
	case name == "Bool":
		return page.TypeClassBoolean
	case numberTypeRE.MatchString(name):
		return page.TypeClassNumber
	case temporalTypes[name]:
		return page.TypeClassTemporal
	case textTypes[name]:
		return page.TypeClassText
	case jsonTypes[name]:
		return page.TypeClassJSON
	default:
		return page.TypeClassOther
	}
}

// computeOrderBySql is read.ts's own — D21: a requested sort is honoured as given; with none, the
// table's own sorting key is used verbatim (F31), rather than re-deriving a column list, which
// would be a different, slower order the engine could no longer read in place.
func computeOrderBySql(sort *model.SortSpec, target ReadTarget) (string, error) {
	if sort != nil && sort.Kind == "text" {
		return sort.Text, nil
	}
	if sort != nil && sort.Kind == "structured" && len(sort.Terms) > 0 {
		terms := make([]adapters.OrderTerm, len(sort.Terms))
		for i, t := range sort.Terms {
			terms[i] = adapters.OrderTerm{Column: t.Column, Direction: t.Direction}
		}
		columnByName := make(map[string]model.ColumnMeta, len(target.Columns))
		for _, c := range target.Columns {
			columnByName[c.Name] = c
		}
		// F11: reject anything but the exact lowercase "asc"/"desc" — adapters.BuildOrderBy below
		// uppercases Direction straight into the ORDER BY text with no validation of its own, the
		// same gap adapters.ValidateRequestedTerms closes for the other three relational adapters.
		if err := adapters.ValidateRequestedTerms(terms, columnByName); err != nil {
			return "", err
		}
		return adapters.BuildOrderBy(terms, quoteIdent), nil
	}
	if key := strings.TrimSpace(target.SortingKey); key != "" {
		return key, nil
	}
	// SortingKey is "" for an `ORDER BY tuple()` MergeTree
	// table and for Log/TinyLog/Memory/Merge engines. readPage still pages with LIMIT/OFFSET
	// regardless, so with no ORDER BY at all two pages of the same request are two independent
	// unordered scans — with multiple parts and more than one reading thread, rows can repeat
	// across pages or never appear at all, even though caps.Pagination advertises "offset"
	// unconditionally. Ordering by every column is deterministic enough for the ordinary case (no
	// adapter here promises anything stronger for a table with no real key) and needs no virtual
	// column a non-MergeTree engine wouldn't have (`_part`/`_part_offset` only exist on MergeTree).
	if len(target.Columns) == 0 {
		return "", nil
	}
	fallbackTerms := make([]adapters.OrderTerm, len(target.Columns))
	for i, c := range target.Columns {
		fallbackTerms[i] = adapters.OrderTerm{Column: c.Name, Direction: "asc"}
	}
	return adapters.BuildOrderBy(fallbackTerms, quoteIdent), nil
}

const noKeysetMessage = "keyset pagination is unavailable for ClickHouse: a MergeTree PRIMARY KEY is a sparse index, " +
	"not a unique key, so there is no total order to build a keyset cursor on — use an offset cursor."

// readReq is adapter.ts's ReadRequest minus Path.
type readReq = adapters.ReadReq

// readPage is read.ts's own — caps.Pagination is offset, unconditionally; a cursor in
// after/before mode is refused outright rather than silently falling back to offset, since a
// silent strategy switch out from under a caller expecting keyset semantics would be its own kind
// of dishonesty.
func readPage(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, target ReadTarget, req readReq, settings map[string]string) (page.TabularPage, error) {
	if req.Cursor.Mode != "offset" {
		return page.TabularPage{}, adapters.New(adapters.CodeUnsupported, noKeysetMessage, nil)
	}

	projectedColumns, err := adapters.ResolveProjection(target.Columns, req.Projection)
	if err != nil {
		return page.TabularPage{}, err
	}
	columns := make([]page.ColumnDescriptor, len(projectedColumns))
	for i, c := range projectedColumns {
		columns[i] = page.ColumnDescriptor{
			Name: c.Name, DataType: c.DataType, TypeClass: typeClassFor(c.DataType),
			Nullable: c.Nullable, IsPrimaryKey: c.IsPrimaryKey,
			Generated: target.GeneratedColumns[c.Name],
		}
	}

	relationSQL := quoteIdent(target.QualifiedName.Database) + "." + quoteIdent(target.QualifiedName.Table)
	selectNames := make([]string, len(projectedColumns))
	for i, c := range projectedColumns {
		selectNames[i] = quoteIdent(c.Name)
	}
	// A Nullable column also selects isNull(col) after the projected ones: the *Strings format
	// renders NULL as a sentinel a real string can equal, the flag settles which it is.
	selectItems := make([]string, 0, 2*len(selectNames))
	selectItems = append(selectItems, selectNames...)
	nullableCols := make([]int, 0, len(projectedColumns))
	for i, c := range projectedColumns {
		if c.Nullable {
			nullableCols = append(nullableCols, i)
			selectItems = append(selectItems, "isNull("+quoteIdent(c.Name)+")")
		}
	}
	selectList := strings.Join(selectItems, ", ")
	whereSQL, params, err := conditionsWhere(adapters.WhereClause(req.Filter), req.Conditions, target.Columns)
	if err != nil {
		return page.TabularPage{}, err
	}
	orderBySQL, err := computeOrderBySql(req.Sort, target)
	if err != nil {
		return page.TabularPage{}, err
	}
	limit, err := adapters.SafeInt(req.PageSize+1, "page size")
	if err != nil {
		return page.TabularPage{}, err
	}
	offset, err := adapters.SafeInt(req.Cursor.Offset, "offset")
	if err != nil {
		return page.TabularPage{}, err
	}

	sqlParts := []string{"SELECT " + selectList, "FROM " + relationSQL}
	if whereSQL != "" {
		sqlParts = append(sqlParts, whereSQL)
	}
	if orderBySQL != "" {
		sqlParts = append(sqlParts, "ORDER BY "+orderBySQL)
	}
	sqlParts = append(sqlParts, "LIMIT "+strconv.Itoa(limit)+" OFFSET "+strconv.Itoa(offset))
	sql := strings.Join(sqlParts, "\n")

	op.SetCommand(sql)
	builder := page.NewTabularPageBuilder(columns)
	rowCount := 0
	hasMore := false
	err = StreamQuery(ctx, h, queryID, sql, op, track, settings, params, func(names, types []string) {}, func(values []*string) bool {
		if rowCount >= req.PageSize {
			hasMore = true
			return true
		}
		cells := values[:len(projectedColumns)]
		for k, i := range nullableCols {
			if flag := values[len(projectedColumns)+k]; flag != nil && *flag == "1" {
				cells[i] = nil
			} else if cells[i] == nil {
				literal := nullSentinel
				cells[i] = &literal
			}
		}
		_ = builder.AppendRow(cells)
		rowCount++
		return true
	})
	if err != nil {
		return page.TabularPage{}, err
	}

	offsetCopy := req.Cursor.Offset
	position := page.PagePosition{
		Offset: &offsetCopy, PageSize: req.PageSize, HasMore: hasMore,
		NextToken: nil, PrevToken: nil, Strategy: "offset",
	}
	return builder.Finish(position), nil
}

// conditionsWhere ANDs the structured conditions onto whereSQL. Values travel as `param_pN` query
// parameters typed after the column, never in the statement text.
func conditionsWhere(whereSQL string, conds []model.Condition, columns []model.ColumnMeta) (string, map[string]string, error) {
	if len(conds) == 0 {
		return whereSQL, nil, nil
	}
	byName := make(map[string]model.ColumnMeta, len(columns))
	for _, c := range columns {
		byName[c.Name] = c
	}
	params := map[string]string{}
	addParam := func(v any) int {
		n := len(params) + 1
		if b, ok := v.(bool); ok {
			params["p"+strconv.Itoa(n)] = strconv.FormatBool(b)
		} else {
			params["p"+strconv.Itoa(n)] = v.(string)
		}
		return n
	}
	placeholder := func(idx int, col model.ColumnMeta) string {
		return "{p" + strconv.Itoa(idx) + ":" + paramType(col.DataType) + "}"
	}
	extra, err := adapters.BuildConditionsSQL(conds, byName, quoteIdent, placeholder, addParam, func(string) bool { return false })
	if err != nil {
		return "", nil, err
	}
	return adapters.AppendWhere(whereSQL, extra), params, nil
}

// paramType is the declared type a bound value parses as: the column's own, wrappers removed. Enum and
// FixedString take String so a LIKE pattern or a short value still parses.
func paramType(declared string) string {
	inner, _ := unwrapType(declared)
	switch baseTypeName(inner) {
	case "Enum8", "Enum16", "FixedString":
		return "String"
	}
	return inner
}

// countRows is read.ts's own.
func countRows(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, target QualifiedName, filter *string) (adapters.CountResult, error) {
	relationSQL := quoteIdent(target.Database) + "." + quoteIdent(target.Table)
	sqlParts := []string{"SELECT count() AS n", "FROM " + relationSQL}
	if where := adapters.WhereClause(filter); where != "" {
		sqlParts = append(sqlParts, where)
	}
	sql := strings.Join(sqlParts, "\n")

	rows, err := RunCatalogQuery[struct {
		N string `json:"n"`
	}](ctx, h, queryID, sql, op, track, nil)
	if err != nil {
		return adapters.CountResult{}, err
	}
	n := "0"
	if len(rows) > 0 {
		n = rows[0].N
	}
	value, err := adapters.ParseCountValue(n)
	if err != nil {
		return adapters.CountResult{}, err
	}
	return adapters.CountResult{Value: value, Exact: true}, nil
}
