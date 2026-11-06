package postgres

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// queryExec is catalog.ts's QueryExecutor: every catalog query is routed through it so it stays
// cancellable and command-logged like any other query (large-database catalog lookups must be
// cancellable even for a lightweight tree expand). scan is called once per row; the callback
// pattern replaces TS's typed generic return, since Go has no equivalent way to say "give me T[]"
// without repeating a struct definition per call site pgx's own reflection-based scanning would
// still need a `db:"..."` tag for anyway (these queries alias columns as snake_case).
type queryExec func(ctx context.Context, sql string, params []any, scan func(pgx.Rows) error) error

// execFor is adapter.go's own execFor, binding one conn/op/track triple. Like every other query it
// runs through adapters.RunWithAbortRace on a ctx detached from the op's own: handing the op's ctx
// to pgx made a cancel close the pinned connection under every later op (F5). The server-side
// cancel stays Cancel's pg_cancel_backend. scan runs on the query's own goroutine, and never after execFor has returned.
func execFor(conn *trackedConn, op *adapters.OpCtx, track TrackQuery) queryExec {
	return func(ctx context.Context, sql string, params []any, scan func(pgx.Rows) error) error {
		op.SetCommand(sql)
		if err := adapters.CheckNotStarted(ctx); err != nil {
			return err
		}
		release := track(conn.running())
		done := conn.track()

		var mu sync.Mutex
		abandoned := false
		_, err := adapters.RunWithAbortRace(ctx, func() { release(); done() }, func(queryCtx context.Context) (struct{}, error) {
			rows, err := conn.Query(queryCtx, sql, params...)
			if err != nil {
				return struct{}{}, mapError(err)
			}
			defer rows.Close()
			for rows.Next() {
				mu.Lock()
				if abandoned {
					mu.Unlock()
					return struct{}{}, adapters.CheckCancelled(ctx)
				}
				err := scan(rows)
				mu.Unlock()
				if err != nil {
					return struct{}{}, mapError(err)
				}
			}
			if err := rows.Err(); err != nil {
				return struct{}{}, mapError(err)
			}
			return struct{}{}, nil
		})
		mu.Lock()
		abandoned = true
		mu.Unlock()
		return err
	}
}

func databasePath(segments ...model.PathSegment) string {
	return model.EncodePath(segments)
}

// listDatabases is catalog.ts's listDatabases.
func listDatabases(ctx context.Context, exec queryExec, currentDatabase string) ([]model.TreeNode, error) {
	var nodes []model.TreeNode
	err := exec(ctx, `SELECT datname AS name,
	        pg_catalog.shobj_description(oid, 'pg_database') AS comment
	 FROM pg_database
	 WHERE NOT datistemplate AND datallowconn
	 ORDER BY datname`, nil, func(rows pgx.Rows) error {
		var name string
		var comment *string
		if err := rows.Scan(&name, &comment); err != nil {
			return err
		}
		var detail *string
		if name == currentDatabase {
			d := "connected"
			detail = &d
		}
		nodes = append(nodes, model.TreeNode{
			Kind: "database", Name: name,
			Path:        databasePath(model.PathSegment{Kind: "database", Name: name}),
			HasChildren: true, Detail: detail,
		})
		return nil
	})
	return nodes, err
}

// listSchemas is catalog.ts's listSchemas. D15: system schemas are hidden.
func listSchemas(ctx context.Context, exec queryExec, databaseSegment string) ([]model.TreeNode, error) {
	var nodes []model.TreeNode
	err := exec(ctx, `SELECT nspname AS name
	 FROM pg_namespace
	 WHERE nspname NOT IN ('pg_catalog', 'information_schema')
	   AND nspname NOT LIKE 'pg\_toast%' AND nspname NOT LIKE 'pg\_temp%'
	 ORDER BY nspname`, nil, func(rows pgx.Rows) error {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		nodes = append(nodes, model.TreeNode{
			Kind: "schema", Name: name,
			Path: databasePath(
				model.PathSegment{Kind: "database", Name: databaseSegment},
				model.PathSegment{Kind: "schema", Name: name},
			),
			HasChildren: true,
		})
		return nil
	})
	return nodes, err
}

var relkindToNodeKind = map[string]string{
	"r": "table", "p": "table", "v": "view", "m": "matview", "S": "sequence",
}

// listRelationsAndFunctions is catalog.ts's listRelationsAndFunctions. D15: a schema's children
// are the objects themselves, no Tables/Views folder nodes; functions share the level.
func listRelationsAndFunctions(ctx context.Context, exec queryExec, databaseSegment, schema string) ([]model.TreeNode, error) {
	var nodes []model.TreeNode
	err := exec(ctx, `SELECT c.relname AS name, c.relkind,
	        c.reltuples::bigint AS row_estimate,
	        obj_description(c.oid, 'pg_class') AS comment
	 FROM pg_class c
	 JOIN pg_namespace n ON n.oid = c.relnamespace
	 WHERE n.nspname = $1 AND c.relkind = ANY('{r,p,v,m,S}') AND NOT c.relispartition
	 ORDER BY CASE c.relkind WHEN 'r' THEN 0 WHEN 'p' THEN 0 WHEN 'v' THEN 1
	                         WHEN 'm' THEN 2 WHEN 'S' THEN 3 END, c.relname`,
		[]any{schema}, func(rows pgx.Rows) error {
			var name, relkind string
			var rowEstimate *int64
			var comment *string
			if err := rows.Scan(&name, &relkind, &rowEstimate, &comment); err != nil {
				return err
			}
			kind, ok := relkindToNodeKind[relkind]
			if !ok {
				kind = "table"
			}
			var detail *string
			// Postgres uses -1 for "never analysed" — render nothing, never the raw -1.
			if (kind == "table" || kind == "matview") && rowEstimate != nil && *rowEstimate >= 0 {
				d := "~" + adapters.AbbreviateCount(*rowEstimate)
				detail = &d
			}
			nodes = append(nodes, model.TreeNode{
				Kind: kind, Name: name,
				Path: databasePath(
					model.PathSegment{Kind: "database", Name: databaseSegment},
					model.PathSegment{Kind: "schema", Name: schema},
					model.PathSegment{Kind: kind, Name: name},
				),
				// P19 D5: a relation is a leaf — columns moved into the definition view — except
				// a partitioned table, whose partitions nest under it.
				HasChildren: relkind == "p", Detail: detail,
			})
			return nil
		})
	if err != nil {
		return nil, err
	}

	err = exec(ctx, `SELECT p.proname AS name,
	        pg_get_function_identity_arguments(p.oid) AS args
	 FROM pg_proc p
	 JOIN pg_namespace n ON n.oid = p.pronamespace
	 WHERE n.nspname = $1 AND p.prokind IN ('f', 'p')
	 ORDER BY p.proname`, []any{schema}, func(rows pgx.Rows) error {
		var name, args string
		if err := rows.Scan(&name, &args); err != nil {
			return err
		}
		detail := "(" + args + ")"
		nodes = append(nodes, model.TreeNode{
			Kind: "function", Name: name,
			Path: databasePath(
				model.PathSegment{Kind: "database", Name: databaseSegment},
				model.PathSegment{Kind: "schema", Name: schema},
				model.PathSegment{Kind: "function", Name: name},
			),
			HasChildren: false, Detail: &detail,
		})
		return nil
	})
	return nodes, err
}

// listPartitions lists a table's declarative partitions. A partition keeps its own schema in its
// path so Describe/Read resolve it like any table; legacy inheritance children stay top-level.
func listPartitions(ctx context.Context, exec queryExec, databaseSegment, schema, table string) ([]model.TreeNode, error) {
	nodes := []model.TreeNode{}
	err := exec(ctx, `SELECT c.relname, n.nspname, c.relkind, c.reltuples::bigint
	 FROM pg_inherits i
	 JOIN pg_class c ON c.oid = i.inhrelid
	 JOIN pg_namespace n ON n.oid = c.relnamespace
	 WHERE c.relispartition
	   AND i.inhparent = (SELECT pc.oid FROM pg_class pc
	                        JOIN pg_namespace pn ON pn.oid = pc.relnamespace
	                       WHERE pn.nspname = $1 AND pc.relname = $2)
	 ORDER BY c.relname`, []any{schema, table}, func(rows pgx.Rows) error {
		var name, partSchema, relkind string
		var rowEstimate *int64
		if err := rows.Scan(&name, &partSchema, &relkind, &rowEstimate); err != nil {
			return err
		}
		var detail *string
		if rowEstimate != nil && *rowEstimate >= 0 {
			d := "~" + adapters.AbbreviateCount(*rowEstimate)
			detail = &d
		}
		nodes = append(nodes, model.TreeNode{
			Kind: "table", Name: name,
			Path: databasePath(
				model.PathSegment{Kind: "database", Name: databaseSegment},
				model.PathSegment{Kind: "schema", Name: partSchema},
				model.PathSegment{Kind: "table", Name: name},
			),
			HasChildren: relkind == "p", Detail: detail,
		})
		return nil
	})
	return nodes, err
}

// RelationInfo is catalog.ts's RelationInfo.
type RelationInfo struct {
	OID         string
	Comment     *string
	RowEstimate *int64
}

// getRelationInfo is catalog.ts's getRelationInfo — describe()'s own OID lookup, also pulling the
// comment and row estimate so ObjectMeta doesn't need a second round trip.
func getRelationInfo(ctx context.Context, exec queryExec, schema, table string) (RelationInfo, error) {
	var info RelationInfo
	found := false
	err := exec(ctx, `SELECT c.oid::text AS oid, obj_description(c.oid, 'pg_class') AS comment,
	        c.reltuples::bigint AS row_estimate
	 FROM pg_class c
	 JOIN pg_namespace n ON n.oid = c.relnamespace
	 WHERE n.nspname = $1 AND c.relname = $2`, []any{schema, table}, func(rows pgx.Rows) error {
		found = true
		var rowEstimate *int64
		if err := rows.Scan(&info.OID, &info.Comment, &rowEstimate); err != nil {
			return err
		}
		// Postgres uses -1 for "never analysed" — surface nil, never the raw -1.
		if rowEstimate != nil && *rowEstimate >= 0 {
			info.RowEstimate = rowEstimate
		}
		return nil
	})
	if err != nil {
		return RelationInfo{}, err
	}
	if !found {
		return RelationInfo{}, adapters.New(adapters.CodeNotFound, "relation \""+schema+"\".\""+table+"\" not found", nil)
	}
	return info, nil
}

// listColumns is catalog.ts's listColumns. Raw columns, IsPrimaryKey left false — callers fold in
// the primary-key column list from listIndexes before exposing this as ColumnMeta.
func listColumns(ctx context.Context, exec queryExec, schema, table string) ([]model.ColumnMeta, error) {
	var columns []model.ColumnMeta
	err := exec(ctx, `SELECT a.attname AS name, a.attnum AS position,
	        format_type(a.atttypid, a.atttypmod) AS data_type,
	        NOT a.attnotnull AS nullable,
	        pg_get_expr(d.adbin, d.adrelid) AS default_expr,
	        col_description(a.attrelid, a.attnum) AS comment
	 FROM pg_attribute a
	 LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
	 WHERE a.attrelid = (
	         SELECT c.oid FROM pg_class c
	         JOIN pg_namespace n ON n.oid = c.relnamespace
	         WHERE n.nspname = $1 AND c.relname = $2)
	   AND a.attnum > 0 AND NOT a.attisdropped
	 ORDER BY a.attnum`, []any{schema, table}, func(rows pgx.Rows) error {
		var col model.ColumnMeta
		var position int32
		if err := rows.Scan(&col.Name, &position, &col.DataType, &col.Nullable, &col.DefaultExpr, &col.Comment); err != nil {
			return err
		}
		col.Position = int(position)
		columns = append(columns, col)
		return nil
	})
	return columns, err
}

// listSchemaColumns is the SchemaColumns, the schema-wide widening of listColumns above:
// the same join, minus the relname filter, plus one LEFT JOIN against pg_index to fold in
// IsPrimaryKey (cheap — a single index lookup, not the full listIndexes/listForeignKeys graph
// Describe pays for) so a completion source built from this can say "not null, pk" without a
// second per-relation round trip. Every column here is byte-identical to what listColumns/Describe
// reports for the same column.
func listSchemaColumns(ctx context.Context, exec queryExec, schema string) ([]model.RelationColumns, error) {
	byName := map[string]*model.RelationColumns{}
	var order []string
	err := exec(ctx, `SELECT c.relname AS table_name, c.relkind,
	        a.attname AS name, a.attnum AS position,
	        format_type(a.atttypid, a.atttypmod) AS data_type,
	        NOT a.attnotnull AS nullable,
	        pg_get_expr(d.adbin, d.adrelid) AS default_expr,
	        col_description(a.attrelid, a.attnum) AS comment,
	        (ix.indexrelid IS NOT NULL) AS is_primary_key
	 FROM pg_class c
	 JOIN pg_namespace n ON n.oid = c.relnamespace
	 JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
	 LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
	 LEFT JOIN pg_index ix ON ix.indrelid = c.oid AND ix.indisprimary AND a.attnum = ANY(ix.indkey)
	 WHERE n.nspname = $1 AND c.relkind = ANY('{r,p,v,m}')
	 ORDER BY c.relname, a.attnum`, []any{schema}, func(rows pgx.Rows) error {
		var tableName, relkind string
		var col model.ColumnMeta
		var position int32
		if err := rows.Scan(&tableName, &relkind, &col.Name, &position, &col.DataType, &col.Nullable,
			&col.DefaultExpr, &col.Comment, &col.IsPrimaryKey); err != nil {
			return err
		}
		col.Position = int(position)
		rc, ok := byName[tableName]
		if !ok {
			kind, ok2 := relkindToNodeKind[relkind]
			if !ok2 {
				kind = "table"
			}
			rc = &model.RelationColumns{Name: tableName, Kind: kind, Columns: []model.ColumnMeta{}}
			byName[tableName] = rc
			order = append(order, tableName)
		}
		rc.Columns = append(rc.Columns, col)
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

// listIndexes is catalog.ts's listIndexes.
func listIndexes(ctx context.Context, exec queryExec, relOID string) ([]model.IndexMeta, error) {
	var indexes []model.IndexMeta
	err := exec(ctx, `SELECT i.relname AS name, ix.indisunique AS unique, ix.indisprimary AS primary,
	        am.amname AS method,
	        ARRAY(SELECT pg_get_indexdef(ix.indexrelid, k.i + 1, true)
	              FROM generate_subscripts(ix.indkey, 1) AS k(i) ORDER BY k.i) AS columns
	 FROM pg_index ix
	 JOIN pg_class i ON i.oid = ix.indexrelid
	 JOIN pg_am am ON am.oid = i.relam
	 WHERE ix.indrelid = $1::oid`, []any{relOID}, func(rows pgx.Rows) error {
		var idx model.IndexMeta
		if err := rows.Scan(&idx.Name, &idx.Unique, &idx.Primary, &idx.Method, &idx.Columns); err != nil {
			return err
		}
		indexes = append(indexes, idx)
		return nil
	})
	return indexes, err
}

// ReadTarget is catalog.ts's ReadTarget.
type ReadTarget struct {
	OID           string
	QualifiedName QualifiedName
	Columns       []model.ColumnMeta
	// PrimaryKey is nil when the relation has none.
	PrimaryKey []string
	// UniqueKeys are unique indexes whose columns are all NOT NULL — keyset tiebreaker
	// candidates (D7).
	UniqueKeys [][]string
}

// QualifiedName is the schema/relation pair every SQL statement this package builds quotes and
// joins itself — never interpolated raw.
type QualifiedName struct {
	Schema, Relation string
}

// getReadTarget is catalog.ts's getReadTarget — the read path's relation shape in one shot,
// resolved fresh on every uncached read, in the same op, right before the data statement.
// Sequential, not concurrent: pgx's *Conn, like node-postgres's Client, does not support
// concurrent queries on one connection.
func getReadTarget(ctx context.Context, exec queryExec, schema, relation string) (ReadTarget, error) {
	info, err := getRelationInfo(ctx, exec, schema, relation)
	if err != nil {
		return ReadTarget{}, err
	}
	rawColumns, err := listColumns(ctx, exec, schema, relation)
	if err != nil {
		return ReadTarget{}, err
	}
	indexes, err := listIndexes(ctx, exec, info.OID)
	if err != nil {
		return ReadTarget{}, err
	}
	shape := adapters.ResolveKeyShape(rawColumns, indexes)
	return ReadTarget{
		OID: info.OID, QualifiedName: QualifiedName{Schema: schema, Relation: relation},
		Columns: shape.Columns, PrimaryKey: shape.PrimaryKey, UniqueKeys: shape.UniqueKeys,
	}, nil
}

var constraintAction = map[string]string{
	"a": "NO ACTION", "r": "RESTRICT", "c": "CASCADE", "n": "SET NULL", "d": "SET DEFAULT",
}

func mapConstraintAction(code *string) *string {
	if code == nil {
		return nil
	}
	action, ok := constraintAction[*code]
	if !ok {
		return nil
	}
	return &action
}

// foreignKeyEdge pairs one FK row's rendered meta with the raw schema/table of the *other* table
// in the edge — model.ForeignKeyMeta only has room for the already-encoded ReferencedPath, and
// building that string needs databaseSegment, which queryForeignKeyEdges itself does not have.
type foreignKeyEdge struct {
	meta                              model.ForeignKeyMeta
	referencedSchema, referencedTable string
}

func queryForeignKeyEdges(ctx context.Context, exec queryExec, relOID, direction string) ([]foreignKeyEdge, error) {
	whereClause := "con.conrelid = $1::oid"
	if direction == "inbound" {
		whereClause = "con.confrelid = $1::oid"
	}
	var edges []foreignKeyEdge
	err := exec(ctx, `SELECT con.conname AS name,
	        con.confdeltype AS on_delete, con.confupdtype AS on_update,
	        (SELECT array_agg(att.attname::text ORDER BY u.ord)
	           FROM unnest(con.conkey) WITH ORDINALITY u(attnum, ord)
	           JOIN pg_attribute att ON att.attrelid = con.conrelid AND att.attnum = u.attnum) AS columns,
	        fn.nspname AS ref_schema, fc.relname AS ref_table,
	        (SELECT array_agg(att.attname::text ORDER BY u.ord)
	           FROM unnest(con.confkey) WITH ORDINALITY u(attnum, ord)
	           JOIN pg_attribute att ON att.attrelid = con.confrelid AND att.attnum = u.attnum) AS ref_columns,
	        sn.nspname AS src_schema, sc.relname AS src_table
	 FROM pg_constraint con
	 JOIN pg_class fc ON fc.oid = con.confrelid JOIN pg_namespace fn ON fn.oid = fc.relnamespace
	 JOIN pg_class sc ON sc.oid = con.conrelid  JOIN pg_namespace sn ON sn.oid = sc.relnamespace
	 WHERE con.contype = 'f' AND `+whereClause,
		[]any{relOID}, func(r pgx.Rows) error {
			var name string
			var onDelete, onUpdate *string
			var columns, refColumns []string
			var refSchema, refTable, srcSchema, srcTable string
			if err := r.Scan(&name, &onDelete, &onUpdate, &columns,
				&refSchema, &refTable, &refColumns, &srcSchema, &srcTable); err != nil {
				return err
			}
			meta := model.ForeignKeyMeta{Name: name, OnDelete: mapConstraintAction(onDelete), OnUpdate: mapConstraintAction(onUpdate)}
			edge := foreignKeyEdge{meta: meta}
			// Outbound: I am conrelid, so my own columns are conkey (columns) and the other
			// table is confrelid (ref_*). Inbound: I am confrelid, so my own columns are
			// confkey (ref_columns) and the other (referencing) table is conrelid (src_*) — the
			// mirror image, keeping meta.Columns = mine, referencedSchema/Table = the other table,
			// for both directions.
			if direction == "inbound" {
				edge.meta.Columns = refColumns
				edge.meta.ReferencedColumns = columns
				edge.referencedSchema, edge.referencedTable = srcSchema, srcTable
			} else {
				edge.meta.Columns = columns
				edge.meta.ReferencedColumns = refColumns
				edge.referencedSchema, edge.referencedTable = refSchema, refTable
			}
			edges = append(edges, edge)
			return nil
		})
	return edges, err
}

func edgesToForeignKeys(edges []foreignKeyEdge, databaseSegment string) []model.ForeignKeyMeta {
	metas := make([]model.ForeignKeyMeta, len(edges))
	for i, e := range edges {
		metas[i] = e.meta
		metas[i].ReferencedPath = databasePath(
			model.PathSegment{Kind: "database", Name: databaseSegment},
			model.PathSegment{Kind: "schema", Name: e.referencedSchema},
			model.PathSegment{Kind: "table", Name: e.referencedTable},
		)
	}
	return metas
}

// listForeignKeys is catalog.ts's listForeignKeys — outbound (this table's own FKs): columns are
// mine (conkey, since conrelid = me); referencedPath/referencedColumns describe the other table.
func listForeignKeys(ctx context.Context, exec queryExec, relOID, databaseSegment string) ([]model.ForeignKeyMeta, error) {
	edges, err := queryForeignKeyEdges(ctx, exec, relOID, "outbound")
	if err != nil {
		return nil, err
	}
	return edgesToForeignKeys(edges, databaseSegment), nil
}

// listReferencedBy is catalog.ts's listReferencedBy (D17): see queryForeignKeyEdges' own comment
// for the inbound/outbound mirror-image rule.
func listReferencedBy(ctx context.Context, exec queryExec, relOID, databaseSegment string) ([]model.ForeignKeyMeta, error) {
	edges, err := queryForeignKeyEdges(ctx, exec, relOID, "inbound")
	if err != nil {
		return nil, err
	}
	return edgesToForeignKeys(edges, databaseSegment), nil
}
