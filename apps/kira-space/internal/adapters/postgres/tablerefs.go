package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wasilibs/go-pgquery/parser"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

var _ adapters.TableReferencer = (*Adapter)(nil)

// pgStatements are the top-level statements an allow list lets through; anything else is refused.
var pgStatements = map[string]bool{
	"SelectStmt": true, "InsertStmt": true, "UpdateStmt": true, "DeleteStmt": true, "MergeStmt": true,
	"ExplainStmt": true, "TransactionStmt": true, "CreateStmt": true, "CreateTableAsStmt": true,
	"ViewStmt": true, "AlterTableStmt": true, "RenameStmt": true, "IndexStmt": true, "DropStmt": true,
	"TruncateStmt": true, "CommentStmt": true, "GrantStmt": true, "RefreshMatViewStmt": true,
	"VacuumStmt": true, "LockStmt": true,
}

var pgRelationObjects = map[string]bool{"OBJECT_TABLE": true, "OBJECT_VIEW": true, "OBJECT_MATVIEW": true}

// pgDeniedFuncs run SQL text or read a relation by name.
var pgDeniedFuncs = map[string]bool{
	"query_to_xml": true, "query_to_xmlschema": true, "query_to_xml_and_xmlschema": true,
	"cursor_to_xml": true, "cursor_to_xmlschema": true, "table_to_xml": true, "table_to_xmlschema": true,
	"table_to_xml_and_xmlschema": true, "ts_stat": true, "ts_rewrite": true, "set_config": true,
}

func pgDeniedFunc(name string) bool {
	return pgDeniedFuncs[name] || strings.HasPrefix(name, "schema_to_xml") || strings.HasPrefix(name, "database_to_xml")
}

func pathDatabase(path model.NodePath) string {
	if len(path.Segments) > 0 && path.Segments[0].Kind == "database" {
		return path.Segments[0].Name
	}
	return ""
}

// ParseTables is adapters.TableReferencer.
func (a *Adapter) ParseTables(statement string, path model.NodePath) adapters.TableParse {
	return parseTablesPostgres(statement, pathDatabase(path))
}

type pgWalker struct {
	db      string
	out     adapters.TableParse
	refs    map[tableref.Ref]bool
	unq     map[string]bool
	funcs   map[adapters.FuncRef]bool
	ops     map[string]bool
	refusal string
}

func (w *pgWalker) refuse(msg string) {
	if w.refusal == "" {
		w.refusal = msg
	}
}

func (w *pgWalker) addRef(catalog, schema, name string) {
	if catalog != "" && catalog != w.db {
		w.refuse("database " + catalog)
		return
	}
	if schema == "" {
		if !w.unq[name] {
			w.unq[name] = true
			w.out.Unqualified = append(w.out.Unqualified, name)
		}
		return
	}
	r := tableref.Ref{Database: w.db, Schema: schema, Name: name}
	if !w.refs[r] {
		w.refs[r] = true
		w.out.Refs = append(w.out.Refs, r)
	}
}

func parseTablesPostgres(sql, database string) adapters.TableParse {
	if database == "" {
		return adapters.TableParse{Refusal: "no database in the path"}
	}
	js, err := parser.ParseToJSON(sql)
	if err != nil {
		return adapters.TableParse{Refusal: "parse error: " + err.Error()}
	}
	var root struct {
		Stmts []struct {
			Stmt map[string]any `json:"stmt"`
		} `json:"stmts"`
	}
	if err := json.Unmarshal([]byte(js), &root); err != nil {
		return adapters.TableParse{Refusal: "parse error: " + err.Error()}
	}
	w := &pgWalker{db: database, refs: map[tableref.Ref]bool{}, unq: map[string]bool{}, funcs: map[adapters.FuncRef]bool{}, ops: map[string]bool{}}
	for _, s := range root.Stmts {
		w.statement(s.Stmt)
	}
	w.out.Refusal = w.refusal
	return w.out
}

// statement checks the whitelist and the statement kinds whose relations are not RangeVars.
func (w *pgWalker) statement(stmt map[string]any) {
	for key, val := range stmt {
		if !pgStatements[key] {
			w.refuse("statement " + key)
			return
		}
		body, _ := val.(map[string]any)
		if w.statementKind(key, body) {
			return
		}
		w.walkStruct(key, body, nil)
	}
}

// statementKind handles the kinds with relations outside RangeVars. It reports true once the statement is
// fully handled or refused.
func (w *pgWalker) statementKind(key string, body map[string]any) bool {
	switch key {
	case "ExplainStmt":
		if q, ok := body["query"].(map[string]any); ok {
			w.statement(q)
		}
		return true
	case "DropStmt":
		if !pgRelationObjects[str(body["removeType"])] {
			w.refuse("statement DROP " + str(body["removeType"]))
			return true
		}
		for _, o := range list(body["objects"]) {
			w.nameList(o, 0)
		}
		return true
	case "CommentStmt":
		switch t := str(body["objtype"]); {
		case pgRelationObjects[t]:
			w.nameList(body["object"], 0)
		case t == "OBJECT_COLUMN":
			w.nameList(body["object"], 1)
		default:
			w.refuse("statement COMMENT ON " + t)
		}
		return true
	case "AlterTableStmt":
		if t := str(body["objtype"]); !pgRelationObjects[t] {
			w.refuse("statement ALTER " + t)
			return true
		}
	case "RenameStmt":
		t := str(body["renameType"])
		if !pgRelationObjects[t] && t != "OBJECT_COLUMN" && t != "OBJECT_TABCONSTRAINT" {
			w.refuse("statement RENAME " + t)
			return true
		}
		if pgRelationObjects[t] {
			if rv, ok := body["relation"].(map[string]any); ok && str(body["newname"]) != "" {
				w.addRef(str(rv["catalogname"]), str(rv["schemaname"]), str(body["newname"]))
			}
		}
	case "GrantStmt":
		if str(body["objtype"]) != "OBJECT_TABLE" || str(body["targtype"]) != "ACL_TARGET_OBJECT" {
			w.refuse("statement GRANT on " + str(body["objtype"]))
			return true
		}
	case "VacuumStmt":
		if len(list(body["rels"])) == 0 {
			w.refuse("VACUUM or ANALYZE without table names")
			return true
		}
	}
	return false
}

// nameList reads a DropStmt/CommentStmt object: a List of String parts, the last dropTail of which are not
// part of the relation name (the column).
func (w *pgWalker) nameList(n any, dropTail int) {
	m, _ := n.(map[string]any)
	inner, _ := m["List"].(map[string]any)
	var parts []string
	for _, it := range list(inner["items"]) {
		if s, ok := node(it, "String"); ok {
			parts = append(parts, str(s["sval"]))
		}
	}
	if dropTail > 0 && len(parts) > dropTail {
		parts = parts[:len(parts)-dropTail]
	}
	switch len(parts) {
	case 1:
		w.addRef("", "", parts[0])
	case 2:
		w.addRef("", parts[0], parts[1])
	case 3:
		w.addRef(parts[0], parts[1], parts[2])
	default:
		w.refuse("object name with " + fmt.Sprint(len(parts)) + " parts")
	}
}

func str(v any) string { s, _ := v.(string); return s }

func list(v any) []any { l, _ := v.([]any); return l }

// node unwraps {"Type": {...}}.
func node(v any, typ string) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	inner, ok := m[typ].(map[string]any)
	return inner, ok
}

type scope map[string]bool

func (s scope) with(names ...string) scope {
	out := make(scope, len(s)+len(names))
	for k := range s {
		out[k] = true
	}
	for _, n := range names {
		out[n] = true
	}
	return out
}

// walk visits every node below n. sc holds the CTE names visible there.
func (w *pgWalker) walk(n any, sc scope) {
	switch v := n.(type) {
	case []any:
		for _, e := range v {
			w.walk(e, sc)
		}
	case map[string]any:
		if _, ok := v["relname"].(string); ok {
			w.walkStruct("RangeVar", v, sc)
			return
		}
		for key, val := range v {
			body, isMap := val.(map[string]any)
			if !isMap {
				w.walk(val, sc)
				continue
			}
			w.walkStruct(key, body, sc)
		}
	}
}

// walkStruct handles one typed node. Inside the node, "withClause" opens a CTE scope.
func (w *pgWalker) walkStruct(typ string, body map[string]any, sc scope) {
	// Typed fields (relation, into.rel, pktable) hold a RangeVar body without the {"RangeVar": ...} wrapper.
	if _, ok := body["relname"].(string); ok {
		typ = "RangeVar"
	}
	switch typ {
	case "RangeVar":
		name := str(body["relname"])
		if str(body["schemaname"]) == "" && sc[name] {
			return
		}
		w.addRef(str(body["catalogname"]), str(body["schemaname"]), name)
		return
	case "FuncCall":
		w.funcCall(body)
	case "A_Expr":
		w.operator(list(body["name"]))
	case "SubLink":
		w.operator(list(body["operName"]))
	case "CallStmt", "DoStmt":
		w.refuse(typ)
		return
	}
	if wc, ok := body["withClause"].(map[string]any); ok {
		sc = w.withScope(wc, sc)
	}
	dml := typ == "InsertStmt" || typ == "UpdateStmt" || typ == "DeleteStmt" || typ == "MergeStmt"
	for k, val := range body {
		switch {
		case k == "withClause", typ == "LockingClause" && k == "lockedRels":
			continue
		case dml && k == "relation":
			// A DML target is always a real table, even when a CTE shares its name.
			w.walk(val, nil)
		default:
			w.walk(val, sc)
		}
	}
}

// withScope walks the CTE queries of a WITH clause and returns the scope the rest of the statement sees.
func (w *pgWalker) withScope(wc map[string]any, sc scope) scope {
	ctes := list(wc["ctes"])
	recursive, _ := wc["recursive"].(bool)
	var names []string
	for _, c := range ctes {
		if cte, ok := node(c, "CommonTableExpr"); ok {
			names = append(names, str(cte["ctename"]))
		}
	}
	all := sc.with(names...)
	for i, c := range ctes {
		cte, ok := node(c, "CommonTableExpr")
		if !ok {
			continue
		}
		bodySc := all
		if !recursive {
			bodySc = sc.with(names[:i]...)
		}
		w.walk(cte["ctequery"], bodySc)
	}
	return all
}

// operator refuses a qualified operator outside pg_catalog and collects an unqualified name for the catalog
// probe: a user operator in a search_path schema runs its backing function.
func (w *pgWalker) operator(names []any) {
	switch len(names) {
	case 0:
	case 1:
		if s, ok := node(names[0], "String"); ok && !w.ops[str(s["sval"])] {
			w.ops[str(s["sval"])] = true
			w.out.Ops = append(w.out.Ops, str(s["sval"]))
		}
	default:
		if first, ok := node(names[0], "String"); !ok || str(first["sval"]) != "pg_catalog" {
			w.refuse("qualified operator")
		}
	}
}

func (w *pgWalker) funcCall(body map[string]any) {
	var parts []string
	for _, p := range list(body["funcname"]) {
		if s, ok := node(p, "String"); ok {
			parts = append(parts, str(s["sval"]))
		}
	}
	if len(parts) == 0 {
		w.refuse("function call without a name")
		return
	}
	name := strings.ToLower(parts[len(parts)-1])
	switch len(parts) {
	case 1:
	case 2:
		if parts[0] != "pg_catalog" {
			w.refuse("function " + parts[0] + "." + parts[1])
			return
		}
	default:
		w.refuse("function " + strings.Join(parts, "."))
		return
	}
	if pgDeniedFunc(name) {
		w.refuse("function " + name)
		return
	}
	if len(parts) == 2 {
		return // pg_catalog-qualified: built in
	}
	f := adapters.FuncRef{Name: parts[0]}
	if !w.funcs[f] {
		w.funcs[f] = true
		w.out.Funcs = append(w.out.Funcs, f)
	}
}

// ResolveTables resolves unqualified relation names through the server's search_path and finds user-defined
// functions. Names travel as bound parameters, never inside the SQL text.
func (a *Adapter) ResolveTables(ctx context.Context, p adapters.TableParse, path model.NodePath, op *adapters.OpCtx) ([]tableref.Ref, string, error) {
	database := pathDatabase(path)
	conn, release, err := a.requireClient(ctx, database)
	if err != nil {
		return nil, "", err
	}
	defer release()
	track := a.trackerFor(op.OpID)
	var refs []tableref.Ref
	if len(p.Unqualified) > 0 {
		rows, err := runArrayQuery(ctx, conn, `SELECT t.name,
			COALESCE((SELECT n.nspname FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
				WHERE c.oid = pg_catalog.to_regclass(pg_catalog.quote_ident(t.name))), pg_catalog.current_schema(), '')
			FROM pg_catalog.unnest($1::text[]) AS t(name)`, []any{p.Unqualified}, op, track, QueryOptions{})
		if err != nil {
			return nil, "", err
		}
		for _, r := range rows {
			if len(r) != 2 || r[0] == nil || r[1] == nil {
				return nil, "", adapters.Contract("table probe returned a malformed row", nil)
			}
			refs = append(refs, tableref.Ref{Database: database, Schema: *r[1], Name: *r[0]})
		}
		if len(rows) != len(p.Unqualified) {
			return nil, "", adapters.Contract("table probe returned a different number of rows", nil)
		}
	}
	if len(p.Funcs) > 0 {
		names := make([]string, len(p.Funcs))
		for i, f := range p.Funcs {
			names[i] = f.Name
		}
		// builtin: pg_catalog has a function of this name, which pg_catalog.<name> calls.
		rows, err := runArrayQuery(ctx, conn, `SELECT p.proname, MIN(n.nspname),
			pg_catalog.bool_or(EXISTS (SELECT 1 FROM pg_catalog.pg_proc b WHERE b.pronamespace = 'pg_catalog'::pg_catalog.regnamespace AND b.proname = p.proname))::text
			FROM pg_catalog.pg_proc p
			JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
			WHERE n.nspname <> 'pg_catalog' AND p.proname = ANY($1::text[])
			GROUP BY p.proname ORDER BY p.proname LIMIT 1`, []any{names}, op, track, QueryOptions{})
		if err != nil {
			return nil, "", err
		}
		if len(rows) == 1 && len(rows[0]) == 3 && rows[0][0] != nil && rows[0][1] != nil {
			msg := "function " + *rows[0][0]
			if rows[0][2] != nil && *rows[0][2] == "true" {
				msg += " (a function of this name also exists in schema " + *rows[0][1] + "; call pg_catalog." + *rows[0][0] + " to use the built-in)"
			}
			return nil, msg, nil
		}
	}
	if len(p.Ops) > 0 {
		// An extension's operators (citext, pg_trgm, ...) are installed by the server's owner and stay allowed.
		rows, err := runArrayQuery(ctx, conn, `SELECT DISTINCT o.oprname FROM pg_catalog.pg_operator o
			JOIN pg_catalog.pg_namespace n ON n.oid = o.oprnamespace
			WHERE n.nspname <> 'pg_catalog' AND o.oprname = ANY($1::text[])
			AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_depend d WHERE d.classid = 'pg_catalog.pg_operator'::pg_catalog.regclass AND d.objid = o.oid AND d.deptype = 'e')
			ORDER BY o.oprname LIMIT 1`, []any{p.Ops}, op, track, QueryOptions{})
		if err != nil {
			return nil, "", err
		}
		if len(rows) == 1 && len(rows[0]) == 1 && rows[0][0] != nil {
			return nil, "operator " + *rows[0][0], nil
		}
	}
	return refs, "", nil
}
