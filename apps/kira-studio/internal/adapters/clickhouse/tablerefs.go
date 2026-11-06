package clickhouse

import (
	"context"
	"fmt"
	"strings"

	chparser "github.com/AfterShip/clickhouse-sql-parser/parser"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
)

var _ adapters.TableReferencer = (*Adapter)(nil)

// chTableFunctions are the only table functions an allow list lets through; the rest read other servers,
// files or tables by name.
var chTableFunctions = map[string]bool{
	"numbers": true, "numbers_mt": true, "zeros": true, "zeros_mt": true, "generaterandom": true,
	"values": true, "null": true, "generate_series": true,
}

// chEngines are the engines whose tables hold their own data. Merge, Distributed, Buffer, Dictionary, MySQL, URL
// and the like read other tables or servers.
func chEngineAllowed(name string) bool {
	switch strings.ToLower(name) {
	case "memory", "log", "tinylog", "stripelog", "null", "set":
		return true
	}
	return strings.HasSuffix(strings.ToLower(name), "mergetree")
}

func chDeniedFunc(name string) bool {
	return strings.HasPrefix(name, "dictget") || name == "dicthas" || name == "dictisin" ||
		strings.HasPrefix(name, "joinget")
}

// ParseTables is adapters.TableReferencer.
func (a *Adapter) ParseTables(statement string, path model.NodePath) adapters.TableParse {
	db := ""
	if len(path.Segments) > 0 && path.Segments[0].Kind == "database" {
		db = path.Segments[0].Name
	}
	if db == "" {
		if h, err := a.requireHandle(); err == nil {
			db = h.DefaultDatabase
		}
	}
	if db == "" {
		db = "default"
	}
	return parseTablesClickHouse(statement, db)
}

type chWalker struct {
	db      string
	out     adapters.TableParse
	refs    map[tableref.Ref]bool
	funcs   map[adapters.FuncRef]bool
	refusal string
}

func (w *chWalker) refuse(msg string) {
	if w.refusal == "" {
		w.refusal = msg
	}
}

func identName(i *chparser.Ident) string {
	if i == nil {
		return ""
	}
	return i.Name
}

func (w *chWalker) addTable(t *chparser.TableIdentifier, scope map[string]bool) {
	if t == nil || t.Table == nil {
		return
	}
	if t.Database == nil && scope[t.Table.Name] {
		return
	}
	db := identName(t.Database)
	if db == "" {
		db = w.db
	}
	r := tableref.Ref{Database: db, Name: t.Table.Name}
	if !w.refs[r] {
		w.refs[r] = true
		w.out.Refs = append(w.out.Refs, r)
	}
}

func parseTablesClickHouse(sql, database string) (out adapters.TableParse) {
	defer func() {
		// The parser is third-party; a panic on odd input must refuse, never crash the app.
		if r := recover(); r != nil {
			out = adapters.TableParse{Refusal: fmt.Sprintf("parse error: %v", r)}
		}
	}()
	stmts, err := chparser.NewParser(sql).ParseStmts()
	if err != nil {
		return adapters.TableParse{Refusal: "parse error: " + err.Error()}
	}
	w := &chWalker{db: database, refs: map[tableref.Ref]bool{}, funcs: map[adapters.FuncRef]bool{}}
	for _, s := range stmts {
		w.statement(s)
	}
	w.out.Refusal = w.refusal
	return w.out
}

func (w *chWalker) statement(s chparser.Expr) {
	switch n := s.(type) {
	case *chparser.ExplainStmt:
		w.statement(n.Statement)
		return
	case *chparser.SelectQuery, *chparser.AlterTable, *chparser.CreateView, *chparser.TruncateTable,
		*chparser.OptimizeStmt, *chparser.DeleteClause:
	case *chparser.InsertStmt:
		if _, ok := n.Table.(*chparser.TableIdentifier); !ok {
			w.refuse("INSERT INTO FUNCTION")
			return
		}
	case *chparser.CreateTable:
		if n.Engine != nil && !chEngineAllowed(n.Engine.Name) {
			w.refuse("ENGINE " + n.Engine.Name)
			return
		}
		if n.TableFunction != nil {
			w.refuse("CREATE TABLE AS table function")
			return
		}
	case *chparser.CreateMaterializedView:
		if n.Engine != nil && !chEngineAllowed(n.Engine.Name) {
			w.refuse("ENGINE " + n.Engine.Name)
			return
		}
	case *chparser.DropStmt:
		if t := strings.ToUpper(n.DropTarget); t != "TABLE" && t != "VIEW" {
			w.refuse("statement DROP " + n.DropTarget)
			return
		}
	case *chparser.RenameStmt:
		if !strings.EqualFold(n.RenameTarget, "TABLE") {
			w.refuse("statement RENAME " + n.RenameTarget)
			return
		}
	default:
		w.refuse("statement " + strings.TrimPrefix(fmt.Sprintf("%T", s), "*parser."))
		return
	}
	w.walk(s, nil)
}

func (w *chWalker) walk(n chparser.Expr, scope map[string]bool) {
	chparser.Walk(n, func(node chparser.Expr) bool {
		switch t := node.(type) {
		case *chparser.SelectQuery:
			if t.With != nil {
				w.withScope(t, scope)
				return false
			}
		case *chparser.TableIdentifier:
			w.addTable(t, scope)
		case *chparser.TableFunctionExpr:
			name := ""
			if id, ok := t.Name.(*chparser.Ident); ok {
				name = id.Name
			}
			if !chTableFunctions[strings.ToLower(name)] {
				w.refuse("table function " + name)
			}
		case *chparser.FunctionExpr:
			name := strings.ToLower(identName(t.Name))
			if chDeniedFunc(name) {
				w.refuse("function " + name)
				break
			}
			if chInFuncs[name] && t.Params != nil && t.Params.Items != nil && len(t.Params.Items.Items) > 1 && isNameExpr(t.Params.Items.Items[1]) {
				w.refuse("IN with a table name")
				break
			}
			f := adapters.FuncRef{Name: name}
			if !w.funcs[f] {
				w.funcs[f] = true
				w.out.Funcs = append(w.out.Funcs, f)
			}
		case *chparser.BinaryOperation:
			// "x IN secret" reads a table, but parses as a plain identifier.
			if strings.Contains(strings.ToUpper(string(t.Operation)), "IN") {
				if isNameExpr(t.RightExpr) {
					w.refuse("IN with a table name")
				}
			}
		}
		return true
	})
}

// chInFuncs are the function forms of IN; a bare name as the second argument is a table.
var chInFuncs = map[string]bool{
	"in": true, "notin": true, "globalin": true, "globalnotin": true,
	"nullin": true, "notnullin": true, "globalnullin": true, "globalnotnullin": true,
	"innullin": true, "notinnullin": true,
}

// isNameExpr reports a bare (possibly parenthesised) identifier, which ClickHouse reads as a table after IN.
func isNameExpr(e chparser.Expr) bool {
	for {
		switch t := e.(type) {
		case *chparser.Ident, *chparser.NestedIdentifier, *chparser.Path:
			return true
		case *chparser.ColumnExpr:
			e = t.Expr
		case *chparser.ParamExprList:
			if t.Items == nil || len(t.Items.Items) != 1 {
				return false
			}
			e = t.Items.Items[0]
		case *chparser.ColumnExprList:
			if len(t.Items) != 1 {
				return false
			}
			e = t.Items[0]
		default:
			return false
		}
	}
}

// withScope walks q with its WITH clause taken off, so each CTE body and the rest see the right names.
// A "name AS (subquery)" entry is a table-like CTE; "expr AS alias" is a scalar and names no table.
func (w *chWalker) withScope(q *chparser.SelectQuery, scope map[string]bool) {
	with := q.With
	q.With = nil
	defer func() { q.With = with }()
	all := map[string]bool{}
	for k := range scope {
		all[k] = true
	}
	for _, c := range with.CTEs {
		if id, ok := c.Expr.(*chparser.Ident); ok && isSubquery(c.Alias) {
			all[id.Name] = true
		}
	}
	// Each body sees the CTEs before it (a ClickHouse CTE cannot reference a later one).
	seen := map[string]bool{}
	for k := range scope {
		seen[k] = true
	}
	for _, c := range with.CTEs {
		if id, ok := c.Expr.(*chparser.Ident); ok && isSubquery(c.Alias) {
			w.walk(c.Alias, copyScope(seen))
			seen[id.Name] = true
			continue
		}
		w.walk(c.Expr, copyScope(seen))
	}
	w.walk(q, all)
}

func copyScope(s map[string]bool) map[string]bool {
	out := make(map[string]bool, len(s))
	for k := range s {
		out[k] = true
	}
	return out
}

func isSubquery(e chparser.Expr) bool {
	switch e.(type) {
	case *chparser.SelectQuery, *chparser.SubQuery:
		return true
	}
	return false
}

// ResolveTables finds user-defined functions (SQL or executable): origin is not System.
func (a *Adapter) ResolveTables(ctx context.Context, p adapters.TableParse, _ model.NodePath, op *adapters.OpCtx) ([]tableref.Ref, string, error) {
	if len(p.Funcs) == 0 {
		return nil, "", nil
	}
	handle, err := a.requireHandle()
	if err != nil {
		return nil, "", err
	}
	seq := a.newOpSeq(op.OpID)
	params := map[string]string{}
	var holders []string
	for i, f := range p.Funcs {
		k := fmt.Sprintf("f%d", i)
		params[k] = f.Name
		holders = append(holders, "{"+k+":String}")
	}
	rows, err := RunCatalogQuery[struct {
		Name string `json:"name"`
	}](ctx, handle, seq.next(a), `SELECT name FROM system.functions
		WHERE origin != 'System' AND lower(name) IN (`+strings.Join(holders, ", ")+`) LIMIT 1`, op, a.trackerFor(op.OpID), params)
	if err != nil {
		return nil, "", err
	}
	if len(rows) > 0 {
		return nil, "function " + rows[0].Name, nil
	}
	return nil, "", nil
}
