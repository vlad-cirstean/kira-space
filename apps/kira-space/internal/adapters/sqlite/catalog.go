package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// QueryExecutor is catalog.ts's own QueryExecutor: every catalog query is routed through it so it
// stays command-logged and cancellation-checked like any other query (F17's own comment). scan is
// called once per row.
type QueryExecutor func(query string, params []any, scan func(*sql.Rows) error) error

// execFor binds one conn/op pair — ctx is always the adapter-owned driverCtx (adapter.go's
// runOnConn), never the op's own context (B8).
func execFor(ctx context.Context, conn *sql.Conn, op *adapters.OpCtx) QueryExecutor {
	return func(query string, params []any, scan func(*sql.Rows) error) error {
		return runRows(ctx, conn, query, params, op, false, scan)
	}
}

// QualifiedName is the sqlite analogue of mysqlfamily's own QualifiedName.
type QualifiedName struct {
	Database string `json:"database"`
	Table    string `json:"table"`
}

// listDatabases is catalog.ts's listDatabases — D19: the tree's one "schema" level, read from
// `PRAGMA database_list` rather than hardcoded. `temp` is always present and never has anything a
// user put there; Kira never issues ATTACH, so in practice this is always exactly one `main` row.
func listDatabases(exec QueryExecutor) ([]model.TreeNode, error) {
	var nodes []model.TreeNode
	err := exec("PRAGMA database_list", nil, func(rows *sql.Rows) error {
		var seq int
		var name string
		var file sql.NullString
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return err
		}
		if name == "temp" {
			return nil
		}
		var detail *string
		if file.Valid && file.String != "" {
			base := filepath.Base(file.String)
			detail = &base
		}
		nodes = append(nodes, model.TreeNode{
			Kind: "database", Name: name,
			Path:        model.EncodePath([]model.PathSegment{{Kind: "database", Name: name}}),
			HasChildren: true, Detail: detail,
		})
		return nil
	})
	return nodes, err
}

type tableListRow struct {
	schema string
	name   string
	typ    string
}

// relevantTables is catalog.ts's own — a `shadow` row is FTS5/RTREE/etc.'s own internal
// bookkeeping table, never shown, the same discipline mysql-family/catalog.go applies to
// information_schema/performance_schema/mysql/sys. `sqlite_`-prefixed names are SQLite's own,
// hidden the same way. A `virtual` table (FTS5, RTREE, ...) reads through SELECT like any other and
// is shown as a plain table.
func relevantTables(exec QueryExecutor, schema string) ([]tableListRow, error) {
	var rows []tableListRow
	err := exec("PRAGMA table_list", nil, func(r *sql.Rows) error {
		var rowSchema, name, typ string
		var ncol, wr, strict int
		if err := r.Scan(&rowSchema, &name, &typ, &ncol, &wr, &strict); err != nil {
			return err
		}
		if rowSchema != schema || typ == "shadow" || strings.HasPrefix(name, "sqlite_") {
			return nil
		}
		rows = append(rows, tableListRow{schema: rowSchema, name: name, typ: typ})
		return nil
	})
	return rows, err
}

// statsQueryFailed is catalog.ts's own — a never-ANALYZEd database has no sqlite_stat1 *table* at
// all, not merely an empty one; the fixed-literal, no-params query below (the only way it can fail)
// throws E_QUERY for it. That is a real, not hypothetical, hole: it took tree enumeration itself
// down before this check existed. Absent estimates is exactly what the
// empty-map/nil return already means for every table stat1 has no row for, so the missing-table
// case is handled the same way, not treated as an error.
func statsQueryFailed(err error) bool {
	code, ok := adapters.CodeOf(err)
	return ok && code == adapters.CodeQuery
}

// loadRowEstimates is catalog.ts's own bulk fetch — F20: taking the max across every stat1 row for
// a table is correct whether the row came from a bare-table stat or a full/partial index's stat (a
// partial index's row can only ever be an undercount, never an overcount).
func loadRowEstimates(exec QueryExecutor) (map[string]int64, error) {
	byTable := make(map[string]int64)
	err := exec("SELECT tbl, stat FROM sqlite_stat1", nil, func(r *sql.Rows) error {
		var tbl, stat string
		if err := r.Scan(&tbl, &stat); err != nil {
			return err
		}
		n, err := strconv.ParseInt(strings.SplitN(stat, " ", 2)[0], 10, 64)
		if err != nil {
			return nil
		}
		if prev, ok := byTable[tbl]; !ok || n > prev {
			byTable[tbl] = n
		}
		return nil
	})
	if err != nil {
		if statsQueryFailed(err) {
			return map[string]int64{}, nil
		}
		return nil, err
	}
	return byTable, nil
}

// getRowEstimateFor is describe()'s single-table counterpart to loadRowEstimates' bulk fetch — same
// max-across-stat1-rows logic (F20), scoped to one table.
func getRowEstimateFor(exec QueryExecutor, table string) (*int, error) {
	var best *int64
	err := exec("SELECT tbl, stat FROM sqlite_stat1 WHERE tbl = ?", []any{table}, func(r *sql.Rows) error {
		var tbl, stat string
		if err := r.Scan(&tbl, &stat); err != nil {
			return err
		}
		n, perr := strconv.ParseInt(strings.SplitN(stat, " ", 2)[0], 10, 64)
		if perr != nil {
			return nil
		}
		if best == nil || n > *best {
			best = &n
		}
		return nil
	})
	if err != nil {
		if statsQueryFailed(err) {
			return nil, nil
		}
		return nil, err
	}
	if best == nil {
		return nil, nil
	}
	v := int(*best)
	return &v, nil
}

// listTablesAndViews is catalog.ts's own — §5.1: database -> tables/views -> column. No routine
// level (SQLite has no stored routines) and no sequence kind (SQLite has no SEQUENCE engine).
func listTablesAndViews(exec QueryExecutor, schema string) ([]model.TreeNode, error) {
	tables, err := relevantTables(exec, schema)
	if err != nil {
		return nil, err
	}
	estimates, err := loadRowEstimates(exec)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(tables, func(i, j int) bool {
		rank := func(t string) int {
			if t == "view" {
				return 1
			}
			return 0
		}
		ri, rj := rank(tables[i].typ), rank(tables[j].typ)
		if ri != rj {
			return ri < rj
		}
		return tables[i].name < tables[j].name
	})

	nodes := make([]model.TreeNode, len(tables))
	for i, row := range tables {
		kind := "table"
		if row.typ == "view" {
			kind = "view"
		}
		var detail *string
		if kind == "table" {
			if n, ok := estimates[row.name]; ok {
				d := "~" + adapters.AbbreviateCount(n)
				detail = &d
			}
		}
		nodes[i] = model.TreeNode{
			Kind: kind, Name: row.name,
			Path: model.EncodePath([]model.PathSegment{
				{Kind: "database", Name: schema}, {Kind: kind, Name: row.name},
			}),
			// P19 D5: every relation is a leaf — a table/view's columns live in the definition view.
			HasChildren: false, Detail: detail,
		}
	}
	return nodes, nil
}

type tableXInfoRow struct {
	cid     int
	name    string
	typ     string
	notnull int
	dflt    sql.NullString
	pk      int
	hidden  int
}

func scanXInfoRow(r *sql.Rows) (tableXInfoRow, error) {
	var row tableXInfoRow
	if err := r.Scan(&row.cid, &row.name, &row.typ, &row.notnull, &row.dflt, &row.pk, &row.hidden); err != nil {
		return tableXInfoRow{}, err
	}
	return row, nil
}

// listColumns is catalog.ts's own — `table_xinfo`, not `table_info` — the latter omits
// generated columns that `SELECT *` still returns. hidden 1 marks a virtual table's shadow-only
// column (excluded); hidden 0/2/3 (ordinary / VIRTUAL generated / STORED generated) are all real,
// selectable columns. schema is passed as the pragma's own second argument — without
// it, an ATTACHed or TEMP table can shadow a main-schema table of the same name (SQLite's own
// default resolution order for an unqualified table-valued pragma call, confirmed empirically).
func listColumns(exec QueryExecutor, schema, table string) ([]model.ColumnMeta, []tableXInfoRow, error) {
	var raw []tableXInfoRow
	err := exec("SELECT * FROM pragma_table_xinfo(?, ?)", []any{table, schema}, func(r *sql.Rows) error {
		row, err := scanXInfoRow(r)
		if err != nil {
			return err
		}
		raw = append(raw, row)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	var columns []model.ColumnMeta
	for _, r := range raw {
		if r.hidden == 1 {
			continue
		}
		var def *string
		if r.dflt.Valid {
			def = &r.dflt.String
		}
		columns = append(columns, model.ColumnMeta{
			Name: r.name, Position: r.cid, DataType: r.typ, Nullable: r.notnull == 0,
			DefaultExpr: def, IsPrimaryKey: r.pk > 0,
		})
	}
	return columns, raw, nil
}

// listSchemaColumns is the SchemaColumns, the schema-wide widening of listColumns above:
// relevantTables' own table/view listing (already excludes shadow/sqlite_-prefixed tables), then
// one pragma_table_xinfo(m.name) join across every one of those tables at once instead of a
// per-relation call. IsPrimaryKey comes straight off table_xinfo's own `pk` ordinal, the same as
// listColumns above — byte-identical to what Describe reports for the same column.
func listSchemaColumns(exec QueryExecutor, schema string) ([]model.RelationColumns, error) {
	tables, err := relevantTables(exec, schema)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return []model.RelationColumns{}, nil
	}
	byName := make(map[string]*model.RelationColumns, len(tables))
	order := make([]string, 0, len(tables))
	placeholders := make([]string, len(tables))
	args := make([]any, len(tables))
	for i, t := range tables {
		kind := "table"
		if t.typ == "view" {
			kind = "view"
		}
		byName[t.name] = &model.RelationColumns{Name: t.name, Kind: kind, Columns: []model.ColumnMeta{}}
		order = append(order, t.name)
		placeholders[i] = "?"
		args[i] = t.name
	}
	// Quoted "<schema>".sqlite_master and pragma_table_xinfo's own schema argument — an
	// unqualified sqlite_master always resolves to "main", and an unqualified pragma call
	// follows SQLite's own default resolution order, either of which can shadow a same-named
	// ATTACHed/TEMP table when schema isn't "main".
	query := `SELECT m.name AS table_name, p.cid, p.name, p.type, p."notnull", p.dflt_value, p.pk, p.hidden
	 FROM ` + quoteIdent(schema) + `.sqlite_master m, pragma_table_xinfo(m.name, ?) p
	 WHERE m.name IN (` + strings.Join(placeholders, ",") + `)
	 ORDER BY m.name, p.cid`
	err = exec(query, append([]any{schema}, args...), func(r *sql.Rows) error {
		var tableName string
		var row tableXInfoRow
		if err := r.Scan(&tableName, &row.cid, &row.name, &row.typ, &row.notnull, &row.dflt, &row.pk, &row.hidden); err != nil {
			return err
		}
		if row.hidden == 1 {
			return nil
		}
		rc, ok := byName[tableName]
		if !ok {
			return nil
		}
		var def *string
		if row.dflt.Valid {
			def = &row.dflt.String
		}
		rc.Columns = append(rc.Columns, model.ColumnMeta{
			Name: row.name, Position: row.cid, DataType: row.typ, Nullable: row.notnull == 0,
			DefaultExpr: def, IsPrimaryKey: row.pk > 0,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.RelationColumns, len(order))
	for i, name := range order {
		out[i] = *byName[name]
	}
	return out, nil
}

// primaryKeyFromColumns is catalog.ts's own — the `pk` ordinal is 1-based and always present on
// table_xinfo, unlike the other SQL adapters: a single-column INTEGER PRIMARY KEY (the rowid alias)
// has no backing index at all, so this is read directly rather than derived from listIndexes.
func primaryKeyFromColumns(columns []model.ColumnMeta, raw []tableXInfoRow) []string {
	known := make(map[string]bool, len(columns))
	for _, c := range columns {
		known[c.Name] = true
	}
	pkRows := make([]tableXInfoRow, 0)
	for _, r := range raw {
		if r.pk > 0 {
			pkRows = append(pkRows, r)
		}
	}
	if len(pkRows) == 0 {
		return nil
	}
	sort.SliceStable(pkRows, func(i, j int) bool { return pkRows[i].pk < pkRows[j].pk })
	var names []string
	for _, r := range pkRows {
		if known[r.name] {
			names = append(names, r.name)
		}
	}
	return names
}

// pkIsKeysetEligible reports whether primaryKey is safe to use as a keyset tiebreaker.
// SQLite is the one dialect here where PRIMARY KEY does not imply NOT NULL: per SQLite's own
// documented quirk, a composite PK or a non-INTEGER single-column PK on a rowid table can genuinely
// hold NULL regardless of whether NOT NULL was declared — nothing but a real NOT NULL constraint
// (table_xinfo's own notnull flag, already resolved into columns' Nullable) ever prevents it there.
// Two shapes SQLite itself guarantees can never store NULL are exempted even when
// table_xinfo's notnull flag reports false (it is not rewritten to reflect either guarantee): a
// WITHOUT ROWID table's own declared primary key (SQLite refuses a NULL there at INSERT time,
// independent of any declared NOT NULL), and a rowid table's single-column INTEGER PRIMARY KEY (the
// rowid alias — inserting NULL there auto-assigns the next rowid instead of ever storing NULL).
func pkIsKeysetEligible(primaryKey []string, columns []model.ColumnMeta, withoutRowid bool) bool {
	if len(primaryKey) == 0 {
		return false
	}
	if withoutRowid {
		return true
	}
	byName := make(map[string]model.ColumnMeta, len(columns))
	for _, c := range columns {
		byName[c.Name] = c
	}
	if len(primaryKey) == 1 {
		c, ok := byName[primaryKey[0]]
		if !ok {
			return false
		}
		if strings.EqualFold(strings.TrimSpace(c.DataType), "INTEGER") {
			return true
		}
		return !c.Nullable
	}
	for _, name := range primaryKey {
		if c, ok := byName[name]; !ok || c.Nullable {
			return false
		}
	}
	return true
}

// listIndexes is catalog.ts's own. schema is passed as each pragma's own second argument (finding
// F9) — see listColumns' identical reasoning.
func listIndexes(exec QueryExecutor, schema, table string) ([]model.IndexMeta, error) {
	type idxRow struct {
		name   string
		unique bool
		origin string
	}
	var indexes []idxRow
	err := exec("SELECT * FROM pragma_index_list(?, ?)", []any{table, schema}, func(r *sql.Rows) error {
		var seq int
		var name string
		var unique, partial int
		var origin string
		if err := r.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return err
		}
		indexes = append(indexes, idxRow{name: name, unique: unique == 1, origin: origin})
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]model.IndexMeta, len(indexes))
	for i, idx := range indexes {
		var columns []string
		err := exec("SELECT * FROM pragma_index_info(?, ?)", []any{idx.name, schema}, func(r *sql.Rows) error {
			var seqno, cid int
			var name sql.NullString
			if err := r.Scan(&seqno, &cid, &name); err != nil {
				return err
			}
			if name.Valid {
				columns = append(columns, name.String)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		result[i] = model.IndexMeta{
			Name: idx.name, Columns: columns, Unique: idx.unique, Primary: idx.origin == "pk",
			// SQLite reports no index method via any pragma — always a B-tree internally, but
			// there is nothing honest to put here beyond nil (unlike MariaDB's INDEX_TYPE).
			Method: nil,
		}
	}
	return result, nil
}

type foreignKeyListRow struct {
	id       int
	seq      int
	table    string
	from     string
	to       string
	onUpdate string
	onDelete string
}

// fetchForeignKeyList's schema arg is finding F9's fix — see listColumns' identical reasoning.
func fetchForeignKeyList(exec QueryExecutor, schema, table string) ([]foreignKeyListRow, error) {
	var rows []foreignKeyListRow
	err := exec("SELECT * FROM pragma_foreign_key_list(?, ?)", []any{table, schema}, func(r *sql.Rows) error {
		var row foreignKeyListRow
		var match string
		if err := r.Scan(&row.id, &row.seq, &row.table, &row.from, &row.to, &row.onUpdate, &row.onDelete, &match); err != nil {
			return err
		}
		rows = append(rows, row)
		return nil
	})
	return rows, err
}

// synthesizeFkName is catalog.ts's own — `foreign_key_list` never reports the constraint's own
// name, even when the SQL declared one with `CONSTRAINT name FOREIGN KEY (...)`. Postgres's own
// convention for an unnamed FK (`<table>_<column>_fkey`).
func synthesizeFkName(table, firstColumn string) string {
	return table + "_" + firstColumn + "_fkey"
}

func groupByID(rows []foreignKeyListRow) [][]foreignKeyListRow {
	byID := make(map[int][]foreignKeyListRow)
	var order []int
	for _, r := range rows {
		if _, ok := byID[r.id]; !ok {
			order = append(order, r.id)
		}
		byID[r.id] = append(byID[r.id], r)
	}
	groups := make([][]foreignKeyListRow, len(order))
	for i, id := range order {
		group := append([]foreignKeyListRow{}, byID[id]...)
		sort.SliceStable(group, func(a, b int) bool { return group[a].seq < group[b].seq })
		groups[i] = group
	}
	return groups
}

// listForeignKeys is catalog.ts's own.
func listForeignKeys(exec QueryExecutor, schema, table string) ([]model.ForeignKeyMeta, error) {
	rows, err := fetchForeignKeyList(exec, schema, table)
	if err != nil {
		return nil, err
	}
	groups := groupByID(rows)
	result := make([]model.ForeignKeyMeta, len(groups))
	for i, group := range groups {
		columns := make([]string, len(group))
		refColumns := make([]string, len(group))
		for j, r := range group {
			columns[j] = r.from
			refColumns[j] = r.to
		}
		onDelete, onUpdate := group[0].onDelete, group[0].onUpdate
		result[i] = model.ForeignKeyMeta{
			Name:    synthesizeFkName(table, group[0].from),
			Columns: columns,
			ReferencedPath: model.EncodePath([]model.PathSegment{
				{Kind: "database", Name: schema}, {Kind: "table", Name: group[0].table},
			}),
			ReferencedColumns: refColumns, OnDelete: &onDelete, OnUpdate: &onUpdate,
		}
	}
	return result, nil
}

// referencingFKRow is one row of fetchReferencingForeignKeys — foreignKeyListRow plus which table
// it came from (src), since a single joined query mixes rows from every table together.
type referencingFKRow struct {
	src string
	foreignKeyListRow
}

// fetchReferencingForeignKeys is one query joining every table's own pragma_foreign_key_list
// against sqlite_master, filtered to the rows that reference target — supported since SQLite 3.16
// (table-valued functions), collapsing what used to be N separate round trips (one per table in
// the database) into one. The `match` column fetchForeignKeyList also scans is omitted here too —
// nothing downstream ever read it.
// schema is quoted onto sqlite_master and passed as pragma_foreign_key_list's own second argument
// see listColumns' identical reasoning.
func fetchReferencingForeignKeys(exec QueryExecutor, schema, target string) ([]referencingFKRow, error) {
	var rows []referencingFKRow
	err := exec(`
		SELECT m.name AS src, f.id, f.seq, f."table", f."from", f."to", f.on_update, f.on_delete
		  FROM `+quoteIdent(schema)+`.sqlite_master m JOIN pragma_foreign_key_list(m.name, ?) f
		 WHERE m.type = 'table' AND f."table" = ?
	`, []any{schema, target}, func(r *sql.Rows) error {
		var row referencingFKRow
		if err := r.Scan(&row.src, &row.id, &row.seq, &row.table, &row.from, &row.to, &row.onUpdate, &row.onDelete); err != nil {
			return err
		}
		rows = append(rows, row)
		return nil
	})
	return rows, err
}

// listReferencedBy is catalog.ts's own — SQLite has no reverse-FK index, so this used to
// scan every other relevant table's own foreign_key_list, one query per table, looking for one
// that points back at `table`. Describing one table in a 500-table database was ~501 serialised
// round trips on the single pinned connection, blocking every other op on it for the duration —
// and views/grid/state.ts calls this on every data-tab load, so opening twenty tables paid it
// twenty times over. allTables includes `table` itself on purpose: a self-referencing FK must
// appear here too — fetchReferencingForeignKeys's own query already scans every table via the
// join, so allTables here is used only to filter to relevant tables (never a view, which
// `foreign_key_list` reports as if it had no FKs of its own regardless) and to order the result the
// same way the old per-table loop did (each source table's own group of FKs, in that table's own
// position in allTables, first-seen id order within it).
func listReferencedBy(exec QueryExecutor, schema, table string, allTables []string) ([]model.ForeignKeyMeta, error) {
	rows, err := fetchReferencingForeignKeys(exec, schema, table)
	if err != nil {
		return nil, err
	}

	tablePos := make(map[string]int, len(allTables))
	for i, t := range allTables {
		tablePos[t] = i
	}

	type groupKey struct {
		src string
		id  int
	}
	byKey := make(map[groupKey][]foreignKeyListRow)
	var order []groupKey
	for _, r := range rows {
		if _, ok := tablePos[r.src]; !ok {
			continue // a view or anything else outside allTables' own scope
		}
		key := groupKey{src: r.src, id: r.id}
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], r.foreignKeyListRow)
	}
	// Re-sort by each source table's own position in allTables — the join's row order is not
	// guaranteed to match it, and callers/tests may depend on the same table-by-table grouping the
	// old per-table loop produced. Stable, so first-seen id order within one table survives.
	sort.SliceStable(order, func(i, j int) bool { return tablePos[order[i].src] < tablePos[order[j].src] })

	var result []model.ForeignKeyMeta
	for _, key := range order {
		group := append([]foreignKeyListRow{}, byKey[key]...)
		sort.SliceStable(group, func(a, b int) bool { return group[a].seq < group[b].seq })
		columns := make([]string, len(group))
		refColumns := make([]string, len(group))
		for j, r := range group {
			columns[j] = r.to
			refColumns[j] = r.from
		}
		onDelete, onUpdate := group[0].onDelete, group[0].onUpdate
		result = append(result, model.ForeignKeyMeta{
			Name:    synthesizeFkName(key.src, group[0].from),
			Columns: columns,
			ReferencedPath: model.EncodePath([]model.PathSegment{
				{Kind: "database", Name: schema}, {Kind: "table", Name: key.src},
			}),
			ReferencedColumns: refColumns, OnDelete: &onDelete, OnUpdate: &onUpdate,
		})
	}
	return result, nil
}

func listAllTableNames(exec QueryExecutor, schema string) ([]string, error) {
	tables, err := relevantTables(exec, schema)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, t := range tables {
		if t.typ != "view" {
			names = append(names, t.name)
		}
	}
	return names, nil
}

// ReadTarget is the sqlite analogue of catalog.ts's ReadTarget.
type ReadTarget struct {
	QualifiedName QualifiedName
	Columns       []model.ColumnMeta
	PrimaryKey    []string
	// UniqueKeys are unique indexes whose columns are all NOT NULL — keyset tiebreaker candidates.
	UniqueKeys [][]string
	// RowidColumn is 'rowid' (or one of its two aliases, whichever isn't shadowed by a real column)
	// for a rowid table with no explicit primary key candidate of its own; nil for a view, a
	// WITHOUT ROWID table, or the rare table that shadows all three rowid aliases. Never
	// mutation identity — purely an internal keyset tiebreaker.
	RowidColumn *string
	// GeneratedColumns is table_xinfo.hidden 2 (VIRTUAL) or 3 (STORED) — a GENERATED ALWAYS AS
	// column (P36 D28).
	GeneratedColumns map[string]bool
}

var rowidAliases = []string{"rowid", "_rowid_", "oid"}

func pickRowidColumn(columns []model.ColumnMeta, isRowidTable bool) *string {
	if !isRowidTable {
		return nil
	}
	used := make(map[string]bool, len(columns))
	for _, c := range columns {
		used[strings.ToLower(c.Name)] = true
	}
	for _, alias := range rowidAliases {
		if !used[alias] {
			a := alias
			return &a
		}
	}
	return nil
}

// getReadTarget is catalog.ts's own — the read path needs the relation's columns/PK/unique-index/
// rowid shape in one shot, resolved fresh on every uncached read (same discipline as the other SQL
// adapters' getReadTarget).
func getReadTarget(exec QueryExecutor, schema, table string) (ReadTarget, error) {
	columns, raw, err := listColumns(exec, schema, table)
	if err != nil {
		return ReadTarget{}, err
	}
	primaryKey := primaryKeyFromColumns(columns, raw)

	indexes, err := listIndexes(exec, schema, table)
	if err != nil {
		return ReadTarget{}, err
	}
	nullableByName := make(map[string]bool, len(columns))
	for _, c := range columns {
		nullableByName[c.Name] = c.Nullable
	}
	var uniqueKeys [][]string
	for _, idx := range indexes {
		if !idx.Unique {
			continue
		}
		allNotNull := true
		for _, c := range idx.Columns {
			if nullableByName[c] {
				allNotNull = false
				break
			}
		}
		if allNotNull {
			uniqueKeys = append(uniqueKeys, idx.Columns)
		}
	}

	// Independent of whether a primary key exists — readPage's own fallback order (PK, else a
	// unique index, else rowid, D22) decides when this is actually consulted; a WITHOUT ROWID table
	// (wr === 1) always has its own declared PK by SQLite's own rule, so this is simply nil there.
	// `type` has to be checked too: pragma_table_list reports wr:0 for a *view* as well (the field
	// is meaningless there, not "false") — found empirically, reading a view crashed with
	// "no such column: rowid" before this check existed.
	//
	// pragma_table_list takes a single table-name argument, filtering by name only (confirmed
	// empirically — no second, schema-scoping argument exists for it, unlike table_xinfo/index_list/
	// foreign_key_list above), so an ATTACHed or TEMP table sharing table's own bare name would
	// otherwise return more than one row here with no way to ask the pragma itself to pick one.
	// Filtering by its own `schema` output column instead (finding F9) disambiguates correctly.
	var tableType string
	var wr int
	hasRow := false
	err = exec("SELECT type, wr FROM pragma_table_list(?) WHERE schema = ?", []any{table, schema}, func(r *sql.Rows) error {
		hasRow = true
		return r.Scan(&tableType, &wr)
	})
	if err != nil {
		return ReadTarget{}, err
	}
	isRowidTable := hasRow && tableType != "view" && wr == 0
	rowidColumn := pickRowidColumn(columns, isRowidTable)

	// A PK carrying a nullable column is not a safe keyset tiebreaker — ComputeEffectiveOrder
	// downstream has no per-row way to know a page's boundary row happened to have a NULL key, so
	// keysetValueAt hard-fails and a NULL-keyed row silently drops from every keyset page. Excluded
	// here (before readPage/relationalpage.go's own selectTiebreaker ever sees it) rather than left
	// to fall through to a unique key or rowid instead.
	withoutRowid := hasRow && tableType != "view" && wr == 1
	if !pkIsKeysetEligible(primaryKey, columns, withoutRowid) {
		primaryKey = nil
	}

	generated := make(map[string]bool)
	for _, r := range raw {
		if r.hidden == 2 || r.hidden == 3 {
			generated[r.name] = true
		}
	}

	return ReadTarget{
		QualifiedName: QualifiedName{Database: schema, Table: table},
		Columns:       columns, PrimaryKey: primaryKey, UniqueKeys: uniqueKeys,
		RowidColumn: rowidColumn, GeneratedColumns: generated,
	}, nil
}
