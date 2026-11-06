package clickhouse

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// QualifiedName is the sqlite/mysqlfamily analogue's own shape, reused here.
type QualifiedName struct {
	Database string `json:"database"`
	Table    string `json:"table"`
}

// listDatabases is catalog.ts's own — D15: keeps `system` unlike mysql-family's own SYSTEM_SCHEMAS
// exclusion, since it is genuinely browsable and hiding it would be hiding the thing the app is
// built on.
func listDatabases(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery) ([]model.TreeNode, error) {
	rows, err := RunCatalogQuery[struct {
		Name string `json:"name"`
	}](ctx, h, queryID, `SELECT name FROM system.databases
	 WHERE name NOT IN ('INFORMATION_SCHEMA', 'information_schema')
	 ORDER BY name`, op, track, nil)
	if err != nil {
		return nil, err
	}
	nodes := make([]model.TreeNode, len(rows))
	for i, row := range rows {
		nodes[i] = model.TreeNode{
			Kind: "database", Name: row.Name,
			Path:        model.EncodePath([]model.PathSegment{{Kind: "database", Name: row.Name}}),
			HasChildren: true,
		}
	}
	return nodes, nil
}

type systemTableRow struct {
	Database         string  `json:"database"`
	Name             string  `json:"name"`
	Engine           string  `json:"engine"`
	Comment          string  `json:"comment"`
	TotalRows        *string `json:"total_rows"`
	SortingKey       string  `json:"sorting_key"`
	PrimaryKey       string  `json:"primary_key"`
	PartitionKey     string  `json:"partition_key"`
	CreateTableQuery string  `json:"create_table_query"`
}

const tableColumns = "database, name, engine, comment, total_rows, sorting_key, primary_key, partition_key, create_table_query"

func relevantTables(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, schema string) ([]systemTableRow, error) {
	return RunCatalogQuery[systemTableRow](ctx, h, queryID, "SELECT "+tableColumns+` FROM system.tables
	 WHERE database = {db:String} AND is_temporary = 0
	 ORDER BY name`, op, track, map[string]string{"db": schema})
}

// kindForEngine is catalog.ts's own — F33: engine name is the object's kind; everything else
// (MergeTree family, Dictionary, Log, Memory, ...) reads through SELECT like any other table.
func kindForEngine(engine string) string {
	switch engine {
	case "View":
		return "view"
	case "MaterializedView":
		return "matview"
	default:
		return "table"
	}
}

// listTablesAndViews is catalog.ts's own — §5.1: database -> tables/views/materialized views
// (ungrouped here; the renderer's own GROUPED_KINDS folders view/matview) -> column. No sequence
// or routine level — ClickHouse has neither.
func listTablesAndViews(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, schema string) ([]model.TreeNode, error) {
	tables, err := relevantTables(ctx, h, queryID, op, track, schema)
	if err != nil {
		return nil, err
	}
	// This used to be a hand-rolled insertion sort over an
	// index slice, justified only by "sort.SliceStable would need the `sort` import" — a premise
	// that was already wrong (sqlite/catalog.go imports `sort` for exactly this). The cost was
	// real: lessTable below allocates two strings.ToLower copies per comparison, and the rank
	// split (every view moves behind every table that follows it) makes insertion sort O(n²) in
	// the ordinary interleaved case — thousands of tables/views is seconds of CPU and hundreds of
	// MB of garbage on a plain tree-node expand. sort.SliceStable is O(n log n), and precomputing
	// each row's lowercased name once removes the allocation from the comparator entirely.
	lowerNames := make([]string, len(tables))
	for i, row := range tables {
		lowerNames[i] = strings.ToLower(row.Name)
	}
	sortIdx := make([]int, len(tables))
	for i := range sortIdx {
		sortIdx[i] = i
	}
	sort.SliceStable(sortIdx, func(i, j int) bool {
		return lessTableIdx(tables, lowerNames, sortIdx[i], sortIdx[j])
	})

	nodes := make([]model.TreeNode, len(tables))
	for i, idx := range sortIdx {
		row := tables[idx]
		kind := kindForEngine(row.Engine)
		var detail *string
		if kind == "table" && row.TotalRows != nil {
			if n, perr := strconv.ParseInt(*row.TotalRows, 10, 64); perr == nil {
				d := "~" + adapters.AbbreviateCount(n)
				detail = &d
			}
		}
		nodes[i] = model.TreeNode{
			Kind: kind, Name: row.Name,
			Path: model.EncodePath([]model.PathSegment{
				{Kind: "database", Name: schema}, {Kind: kind, Name: row.Name},
			}),
			// P19 D5: every relation is a leaf — a table/view's columns live in describe()/definition().
			HasChildren: false, Detail: detail,
		}
	}
	return nodes, nil
}

func tableRank(t systemTableRow) int {
	if kindForEngine(t.Engine) == "table" {
		return 0
	}
	return 1
}

// lessTableIdx is lessTable's own rule (tables before views/matviews, then a case-insensitive name
// compare), rewritten to compare by index against a lowercased-name slice computed once up front
// (P21 round 3 performance finding 7) rather than allocating two strings.ToLower copies per call.
func lessTableIdx(tables []systemTableRow, lowerNames []string, i, j int) bool {
	ra, rb := tableRank(tables[i]), tableRank(tables[j])
	if ra != rb {
		return ra < rb
	}
	// catalog.ts's own listing sorted with String.prototype.localeCompare, which (for this driver's
	// plain-ASCII table names) is case-insensitive — plain byte comparison would instead sort every
	// uppercase-leading name before every lowercase one (caught by fixture regeneration reordering
	// "Order Items" ahead of "big_rows" instead of next to "order_items").
	return lowerNames[i] < lowerNames[j]
}

type systemColumnRow struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Position is UInt64 in system.columns — like total_rows/count(), FORMAT JSON renders it as a
	// quoted string once output_format_json_quote_64bit_integers is on (client.go's own comment),
	// so this is string, not int, and toColumnMeta parses it.
	Position          string `json:"position"`
	DefaultKind       string `json:"default_kind"`
	DefaultExpression string `json:"default_expression"`
	Comment           string `json:"comment"`
}

func listColumnsRaw(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, schema, table string) ([]systemColumnRow, error) {
	return RunCatalogQuery[systemColumnRow](ctx, h, queryID, `SELECT name, type, position, default_kind, default_expression, comment
	 FROM system.columns
	 WHERE database = {db:String} AND table = {tbl:String}
	 ORDER BY position`, op, track, map[string]string{"db": schema, "tbl": table})
}

// toColumnMeta is catalog.ts's own — nullability lives inside the type string, not a
// separate column. isPrimaryKey is always false — a MergeTree PRIMARY KEY is a sparse
// index, not a unique row identity, and showing a PK badge here would claim exactly the
// uniqueness that does not exist.
func toColumnMeta(row systemColumnRow) model.ColumnMeta {
	_, nullable := unwrapType(row.Type)
	var def *string
	if row.DefaultExpression != "" {
		def = &row.DefaultExpression
	}
	var comment *string
	if row.Comment != "" {
		comment = &row.Comment
	}
	position, _ := strconv.Atoi(row.Position)
	return model.ColumnMeta{
		Name: row.Name, Position: position, DataType: row.Type, Nullable: nullable,
		DefaultExpr: def, IsPrimaryKey: false, Comment: comment,
	}
}

// listSchemaColumns is the SchemaColumns, the schema-wide widening: relevantTables' own
// table/view/matview listing (already excludes system engines, kindForEngine) joined against one
// schema-wide system.columns query, rather than one listColumnsRaw call per relation.
// IsPrimaryKey is always false, mirroring toColumnMeta's own rule — a MergeTree PRIMARY
// KEY is a sparse index, not a uniqueness constraint.
func listSchemaColumns(ctx context.Context, h *Handle, tablesQueryID, columnsQueryID string, op *adapters.OpCtx, track TrackQuery, schema string) ([]model.RelationColumns, error) {
	tables, err := relevantTables(ctx, h, tablesQueryID, op, track, schema)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*model.RelationColumns, len(tables))
	order := make([]string, 0, len(tables))
	for _, t := range tables {
		byName[t.Name] = &model.RelationColumns{Name: t.Name, Kind: kindForEngine(t.Engine), Columns: []model.ColumnMeta{}}
		order = append(order, t.Name)
	}

	type schemaColumnRow struct {
		Table string `json:"table"`
		systemColumnRow
	}
	rows, err := RunCatalogQuery[schemaColumnRow](ctx, h, columnsQueryID,
		`SELECT table, name, type, position, default_kind, default_expression, comment
	 FROM system.columns
	 WHERE database = {db:String}
	 ORDER BY table, position`, op, track, map[string]string{"db": schema})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		rc, ok := byName[row.Table]
		if !ok {
			// A shadow/.inner table system.columns still lists (e.g. a materialized view's own
			// backing table) that relevantTables' own is_temporary/engine filter excluded — not a
			// relation this container's tree shows, so its columns are skipped, not surfaced.
			continue
		}
		rc.Columns = append(rc.Columns, toColumnMeta(row.systemColumnRow))
	}
	out := make([]model.RelationColumns, len(order))
	for i, name := range order {
		out[i] = *byName[name]
	}
	return out, nil
}

// splitTopLevelCommas is catalog.ts's own — parenthesis-aware split for a key expression such as
// "toYYYYMM(d), id".
func splitTopLevelCommas(expr string) []string {
	var parts []string
	depth := 0
	var current strings.Builder
	for _, ch := range expr {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		}
		if ch == ',' && depth == 0 {
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
		} else {
			current.WriteRune(ch)
		}
	}
	if strings.TrimSpace(current.String()) != "" {
		parts = append(parts, strings.TrimSpace(current.String()))
	}
	return parts
}

// listIndexes is catalog.ts's own — system.data_skipping_indices plus one synthetic entry
// for the sparse primary index — without it the definition view would show nothing at all about
// the one thing that most determines how a MergeTree table behaves. unique: false throughout: a
// skipping index is a pruning aid, and the primary index is a sparse index, neither a uniqueness
// constraint.
func listIndexes(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, schema, table, primaryKeyExpression string) ([]model.IndexMeta, error) {
	skipping, err := RunCatalogQuery[struct {
		Name string `json:"name"`
		Expr string `json:"expr"`
		Type string `json:"type"`
	}](ctx, h, queryID, `SELECT name, expr, type FROM system.data_skipping_indices
	 WHERE database = {db:String} AND table = {tbl:String}`, op, track, map[string]string{"db": schema, "tbl": table})
	if err != nil {
		return nil, err
	}
	indexes := make([]model.IndexMeta, len(skipping))
	for i, idx := range skipping {
		method := idx.Type
		indexes[i] = model.IndexMeta{Name: idx.Name, Columns: []string{idx.Expr}, Unique: false, Primary: false, Method: &method}
	}
	if strings.TrimSpace(primaryKeyExpression) != "" {
		method := "sparse (primary index)"
		primary := model.IndexMeta{
			Name: table + "_primary_idx", Columns: splitTopLevelCommas(primaryKeyExpression),
			Unique: false, Primary: true, Method: &method,
		}
		indexes = append([]model.IndexMeta{primary}, indexes...)
	}
	return indexes, nil
}

type constraintRow struct {
	Name       string
	Type       string
	Expression string
}

// checkConstraintRE is catalog.ts's own — 'ASSUME' is a query-optimizer hint, not a constraint a
// user would recognise as one, so only CONSTRAINT ... CHECK ... clauses are matched.
var checkConstraintRE = regexp.MustCompile(`(?is)^CONSTRAINT\s+(` + "`" + `(?:[^` + "`" + `]|` + "``" + `)+` + "`" + `|\S+)\s+CHECK\s+([\s\S]+)$`)

// listCheckConstraints is catalog.ts's own — system.constraints is documented but
// does not exist on the server this adapter is built/tested against (checked against
// clickhouse/clickhouse-server:26.3), so this parses CHECK constraints out of the CREATE TABLE DDL
// text itself instead of querying a catalog table. This is the one thing in this package that
// clears CLAUDE.md's unit-test bar on its own: a small parenthesis-aware parser with several
// interacting lexical rules.
func listCheckConstraints(createTableQuery string) []constraintRow {
	start := strings.IndexByte(createTableQuery, '(')
	if start == -1 {
		return nil
	}
	depth := 0
	end := -1
	for i := start; i < len(createTableQuery); i++ {
		switch createTableQuery[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end != -1 {
			break
		}
	}
	if end == -1 {
		return nil
	}

	var constraints []constraintRow
	for _, part := range splitTopLevelCommas(createTableQuery[start+1 : end]) {
		m := checkConstraintRE.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			continue
		}
		rawName := m[1]
		name := rawName
		if strings.HasPrefix(rawName, "`") && strings.HasSuffix(rawName, "`") {
			name = strings.ReplaceAll(rawName[1:len(rawName)-1], "``", "`")
		}
		constraints = append(constraints, constraintRow{Name: name, Type: "CHECK", Expression: strings.TrimSpace(m[2])})
	}
	return constraints
}

// ReadTarget is catalog.ts's own.
type ReadTarget struct {
	QualifiedName QualifiedName
	Columns       []model.ColumnMeta
	// GeneratedColumns: MATERIALIZED/ALIAS columns are readable (this adapter never emits
	// SELECT *) but refuse an INSERT (F15).
	GeneratedColumns map[string]bool
	Engine           string
	// SortingKey is F14/F31: ClickHouse's own sorting-key expression text, used verbatim as the
	// default ORDER BY when the request asks for no sort (D21). "" for an engine with no sorting key.
	SortingKey           string
	PrimaryKeyExpression string
	PartitionKey         string
	// TotalRows is F32: exact when ClickHouse can answer cheaply from part metadata, else nil —
	// never 0.
	TotalRows        *int64
	Comment          *string
	CreateTableQuery string
}

func getTableRow(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, schema, table string) (*systemTableRow, error) {
	rows, err := RunCatalogQuery[systemTableRow](ctx, h, queryID, "SELECT "+tableColumns+` FROM system.tables
	 WHERE database = {db:String} AND name = {tbl:String}`, op, track, map[string]string{"db": schema, "tbl": table})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// getReadTarget is catalog.ts's own — the read/mutate path's one-shot answer for a relation's
// columns/engine/keys, resolved fresh on every op (same discipline as every other SQL adapter's
// getReadTarget).
func getReadTarget(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, schema, table string) (ReadTarget, error) {
	tableRow, err := getTableRow(ctx, h, queryID, op, track, schema, table)
	if err != nil {
		return ReadTarget{}, err
	}
	if tableRow == nil {
		return ReadTarget{}, adapters.New(adapters.CodeNotFound, `no such table: "`+schema+`"."`+table+`"`, nil)
	}
	columnRows, err := listColumnsRaw(ctx, h, queryID, op, track, schema, table)
	if err != nil {
		return ReadTarget{}, err
	}
	columns := make([]model.ColumnMeta, len(columnRows))
	generated := make(map[string]bool)
	for i, r := range columnRows {
		columns[i] = toColumnMeta(r)
		if r.DefaultKind == "MATERIALIZED" || r.DefaultKind == "ALIAS" {
			generated[r.Name] = true
		}
	}
	var comment *string
	if tableRow.Comment != "" {
		comment = &tableRow.Comment
	}
	var totalRows *int64
	if tableRow.TotalRows != nil {
		if n, perr := strconv.ParseInt(*tableRow.TotalRows, 10, 64); perr == nil {
			totalRows = &n
		}
	}
	return ReadTarget{
		QualifiedName: QualifiedName{Database: schema, Table: table}, Columns: columns,
		GeneratedColumns: generated, Engine: tableRow.Engine, SortingKey: tableRow.SortingKey,
		PrimaryKeyExpression: tableRow.PrimaryKey, PartitionKey: tableRow.PartitionKey,
		TotalRows: totalRows, Comment: comment, CreateTableQuery: tableRow.CreateTableQuery,
	}, nil
}
