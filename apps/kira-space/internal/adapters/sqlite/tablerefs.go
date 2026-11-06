package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/rqlite/sql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

var _ adapters.TableReferencer = (*Adapter)(nil)

// ParseTables is adapters.TableReferencer. SQLite has no SQL-defined functions and the app registers none,
// so no probe is needed.
func (a *Adapter) ParseTables(statement string, _ model.NodePath) adapters.TableParse {
	return parseTablesSQLite(statement)
}

// ResolveTables is never called: ParseTables never asks for a probe.
func (a *Adapter) ResolveTables(context.Context, adapters.TableParse, model.NodePath, *adapters.OpCtx) ([]tableref.Ref, string, error) {
	return nil, "", nil
}

type sqWalker struct {
	out     adapters.TableParse
	refs    map[string]bool
	refusal string
}

func (w *sqWalker) refuse(msg string) {
	if w.refusal == "" {
		w.refusal = msg
	}
}

// add records a table; schema other than main is refused (temp tables and attached files).
func (w *sqWalker) add(schema, name *sql.Ident) {
	if name == nil {
		return
	}
	if schema != nil && !strings.EqualFold(schema.Name, "main") {
		w.refuse("schema " + schema.Name)
		return
	}
	key := strings.ToLower(name.Name)
	if !w.refs[key] {
		w.refs[key] = true
		w.out.Refs = append(w.out.Refs, tableref.Ref{Name: name.Name})
	}
}

func parseTablesSQLite(statement string) (out adapters.TableParse) {
	defer func() {
		if r := recover(); r != nil {
			out = adapters.TableParse{Refusal: fmt.Sprintf("parse error: %v", r)}
		}
	}()
	stmts, err := sql.NewParser(strings.NewReader(statement)).ParseStatements()
	if err != nil {
		return adapters.TableParse{Refusal: "parse error: " + err.Error()}
	}
	w := &sqWalker{refs: map[string]bool{}}
	for _, s := range stmts {
		w.statement(s)
	}
	w.out.Refusal = w.refusal
	return w.out
}

func (w *sqWalker) statement(s sql.Statement) {
	switch n := s.(type) {
	case *sql.ExplainStatement:
		w.statement(n.Stmt)
	case *sql.BeginStatement, *sql.CommitStatement, *sql.RollbackStatement, *sql.SavepointStatement, *sql.ReleaseStatement:
	case *sql.SelectStatement:
		w.walk(n, nil, nil)
	case *sql.InsertStatement:
		// The target is a real table even when a CTE shares its name.
		w.add(n.Schema, n.Table)
		w.walk(n, nil, nil)
	case *sql.UpdateStatement:
		w.target(n.Table)
		w.walk(n, nil, n.Table)
	case *sql.DeleteStatement:
		w.target(n.Table)
		w.walk(n, nil, n.Table)
	case *sql.CreateTableStatement:
		w.add(n.Schema, n.Name)
		w.walk(n, nil, nil)
	case *sql.CreateViewStatement:
		w.add(n.Schema, n.Name)
		w.walk(n, nil, nil)
	case *sql.CreateIndexStatement:
		w.add(n.Schema, n.Table)
		w.walk(n, nil, nil)
	case *sql.AlterTableStatement:
		w.add(n.Schema, n.Name)
		w.add(n.Schema, n.NewName)
		w.walk(n, nil, nil)
	case *sql.DropTableStatement:
		w.add(n.Schema, n.Name)
	case *sql.DropViewStatement:
		w.add(n.Schema, n.Name)
	case *sql.AnalyzeStatement:
		if n.Name == nil {
			w.refuse("ANALYZE without a table name")
			return
		}
		w.add(n.Schema, n.Name)
	default:
		w.refuse("statement " + strings.TrimPrefix(fmt.Sprintf("%T", s), "*sql."))
	}
}

func (w *sqWalker) target(t *sql.QualifiedTableName) {
	if t != nil {
		w.add(t.Schema, t.Name)
	}
}

func (w *sqWalker) walk(n sql.Node, scope map[string]bool, skip *sql.QualifiedTableName) {
	_, _ = sql.Walk(&sqVisitor{w: w, scope: scope, skip: skip}, n)
}

type sqVisitor struct {
	w     *sqWalker
	scope map[string]bool
	skip  *sql.QualifiedTableName
}

func (v *sqVisitor) extend(names ...string) map[string]bool {
	out := make(map[string]bool, len(v.scope)+len(names))
	for k := range v.scope {
		out[k] = true
	}
	for _, n := range names {
		out[n] = true
	}
	return out
}

// withScope sets the CTEs of wc up: each body sees the earlier CTEs (all of them for WITH RECURSIVE), and
// the rest of the statement sees all. wc is detached by the caller so Walk does not revisit it.
func (v *sqVisitor) withScope(wc *sql.WithClause) map[string]bool {
	names := make([]string, len(wc.CTEs))
	for i, c := range wc.CTEs {
		names[i] = strings.ToLower(c.TableName.Name)
	}
	all := v.extend(names...)
	for i, c := range wc.CTEs {
		sc := v.extend(names[:i]...)
		if wc.Recursive.IsValid() {
			sc = all
		}
		v.w.walk(c.Select, sc, nil)
	}
	return all
}

func (v *sqVisitor) Visit(n sql.Node) (sql.Visitor, sql.Node, error) {
	scope := v.scope
	switch t := n.(type) {
	case *sql.SelectStatement:
		if t.WithClause != nil {
			wc := t.WithClause
			t.WithClause = nil
			scope = v.withScope(wc)
		}
	case *sql.InsertStatement:
		if t.WithClause != nil {
			wc := t.WithClause
			t.WithClause = nil
			scope = v.withScope(wc)
		}
	case *sql.UpdateStatement:
		if t.WithClause != nil {
			wc := t.WithClause
			t.WithClause = nil
			v.withScope(wc) // sources keep the outer scope: the target is real, so a shadowing CTE name is over-checked
		}
	case *sql.DeleteStatement:
		if t.WithClause != nil {
			wc := t.WithClause
			t.WithClause = nil
			v.withScope(wc)
		}
	case *sql.QualifiedTableName:
		if t == v.skip {
			break
		}
		if t.Schema == nil && t.Name != nil && scope[strings.ToLower(t.Name.Name)] {
			break
		}
		v.w.add(t.Schema, t.Name)
	case *sql.QualifiedTableFunctionName:
		if t.Name != nil && strings.HasPrefix(strings.ToLower(t.Name.Name), "pragma_") {
			v.w.refuse("table function " + t.Name.Name)
		}
	case *sql.Call:
		if t.Name != nil && strings.EqualFold(t.Name.Name, "load_extension") {
			v.w.refuse("function load_extension")
		}
	case *sql.ForeignKeyConstraint:
		v.w.add(nil, t.ForeignTable)
	case *sql.BinaryExpr:
		// "x IN secret" reads a table, but parses as a plain identifier.
		if t.Op == sql.IN || t.Op == sql.NOTIN {
			switch t.Y.(type) {
			case *sql.Ident, *sql.QualifiedRef:
				v.w.refuse("IN with a table name")
			}
		}
	}
	if scope == nil {
		scope = map[string]bool{}
	}
	return &sqVisitor{w: v.w, scope: scope, skip: v.skip}, n, nil
}

func (v *sqVisitor) VisitEnd(n sql.Node) (sql.Node, error) { return n, nil }
