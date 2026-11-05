package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/wasilibs/go-pgquery/parser"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
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
	w := &pgWalker{db: database, refs: map[tableref.Ref]bool{}, unq: map[string]bool{}, funcs: map[adapters.FuncRef]bool{}}
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
		switch key {
		case "ExplainStmt":
			if q, ok := body["query"].(map[string]any); ok {
				w.statement(q)
			}
			return
		case "DropStmt":
			if !pgRelationObjects[str(body["removeType"])] {
				w.refuse("statement DROP " + str(body["removeType"]))
				return
			}
			for _, o := range list(body["objects"]) {
				w.nameList(o, 0)
			}
			return
		case "CommentStmt":
			switch t := str(body["objtype"]); {
			case pgRelationObjects[t]:
				w.nameList(body["object"], 0)
			case t == "OBJECT_COLUMN":
				w.nameList(body["object"], 1)
			default:
				w.refuse("statement COMMENT ON " + t)
			}
			return
		case "AlterTableStmt":
			if t := str(body["objtype"]); !pgRelationObjects[t] {
				w.refuse("statement ALTER " + t)
				return
			}
		case "RenameStmt":
			t := str(body["renameType"])
			if !pgRelationObjects[t] && t != "OBJECT_COLUMN" && t != "OBJECT_TABCONSTRAINT" {
				w.refuse("statement RENAME " + t)
				return
			}
			if pgRelationObjects[t] {
				if rv, ok := body["relation"].(map[string]any); ok && str(body["newname"]) != "" {
					w.addRef(str(rv["catalogname"]), str(rv["schemaname"]), str(body["newname"]))
				}
			}
		case "GrantStmt":
			if str(body["objtype"]) != "OBJECT_TABLE" || str(body["targtype"]) != "ACL_TARGET_OBJECT" {
				w.refuse("statement GRANT on " + str(body["objtype"]))
				return
			}
		case "VacuumStmt":
			if len(list(body["rels"])) == 0 {
				w.refuse("VACUUM or ANALYZE without table names")
				return
			}
		}
		w.walkStruct(key, body, nil)
	}
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
		names := list(body["name"])
		if len(names) > 1 {
			if first, ok := node(names[0], "String"); !ok || str(first["sval"]) != "pg_catalog" {
				w.refuse("qualified operator")
			}
		}
	case "CallStmt", "DoStmt":
		w.refuse(typ)
		return
	}
	if wc, ok := body["withClause"].(map[string]any); ok {
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
		sc = all
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
		rows, err := runArrayQuery(ctx, conn, `SELECT DISTINCT p.proname FROM pg_catalog.pg_proc p
			JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
			WHERE n.nspname = 'pg_catalog' AND p.proname = ANY($1::text[])`, []any{names}, op, track, QueryOptions{})
		if err != nil {
			return nil, "", err
		}
		builtin := map[string]bool{}
		for _, r := range rows {
			if len(r) == 1 && r[0] != nil {
				builtin[*r[0]] = true
			}
		}
		var user []string
		for _, n := range names {
			if !builtin[n] {
				user = append(user, n)
			}
		}
		if len(user) > 0 {
			sort.Strings(user)
			return nil, "function " + user[0], nil
		}
	}
	return refs, "", nil
}
