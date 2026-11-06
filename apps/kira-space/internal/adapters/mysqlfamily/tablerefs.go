package mysqlfamily

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver" // value expression nodes the parser builds

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

var _ adapters.TableReferencer = (*Adapter)(nil)

// ParseTables is adapters.TableReferencer. MariaDB goes through the MySQL grammar: MariaDB-only syntax
// fails to parse and is refused.
func (a *Adapter) ParseTables(statement string, path model.NodePath) adapters.TableParse {
	db := ""
	if len(path.Segments) > 0 && path.Segments[0].Kind == "database" {
		db = path.Segments[0].Name
	}
	if db == "" {
		db = a.state.Load().PrimaryDatabase
	}
	return parseTablesMySQL(statement, db)
}

type myWalker struct {
	db      string
	out     adapters.TableParse
	refs    map[tableref.Ref]bool
	funcs   map[adapters.FuncRef]bool
	skip    map[*ast.TableName]bool
	refusal string
}

func (w *myWalker) refuse(msg string) {
	if w.refusal == "" {
		w.refusal = msg
	}
}

func (w *myWalker) addRef(schema, name string) {
	if schema == "" {
		schema = w.db
	}
	if schema == "" {
		w.refuse("no database selected for table " + name)
		return
	}
	r := tableref.Ref{Database: schema, Name: name}
	if !w.refs[r] {
		w.refs[r] = true
		w.out.Refs = append(w.out.Refs, r)
	}
}

func parseTablesMySQL(sql, database string) adapters.TableParse {
	stmts, _, err := parser.New().ParseSQL(sql)
	if err != nil {
		return adapters.TableParse{Refusal: "parse error: " + err.Error()}
	}
	w := &myWalker{db: database, refs: map[tableref.Ref]bool{}, funcs: map[adapters.FuncRef]bool{}, skip: map[*ast.TableName]bool{}}
	for _, s := range stmts {
		w.statement(s)
	}
	w.out.Refusal = w.refusal
	return w.out
}

func (w *myWalker) statement(s ast.StmtNode) {
	switch n := s.(type) {
	case *ast.ExplainStmt:
		w.statement(n.Stmt)
		return
	case *ast.SelectStmt, *ast.SetOprStmt, *ast.InsertStmt, *ast.UpdateStmt, *ast.DeleteStmt,
		*ast.BeginStmt, *ast.CommitStmt, *ast.RollbackStmt, *ast.SavepointStmt, *ast.ReleaseSavepointStmt,
		*ast.CreateViewStmt, *ast.AlterTableStmt, *ast.RenameTableStmt, *ast.DropTableStmt, *ast.TruncateTableStmt,
		*ast.CreateIndexStmt, *ast.DropIndexStmt, *ast.AnalyzeTableStmt, *ast.LockTablesStmt, *ast.UnlockTablesStmt:
	case *ast.CreateTableStmt:
		for _, o := range n.Options {
			if o.Tp == ast.TableOptionEngine {
				switch strings.ToUpper(o.StrValue) {
				case "MERGE", "MRG_MYISAM", "FEDERATED", "CONNECT":
					w.refuse("ENGINE " + o.StrValue)
					return
				}
			}
			if o.Tp == ast.TableOptionUnion {
				w.refuse("UNION table option")
				return
			}
		}
	default:
		w.refuse("statement " + fmt.Sprintf("%T", s))
		return
	}
	s.Accept(&myVisitor{w: w})
}

type myVisitor struct {
	w     *myWalker
	scope map[string]bool
}

func withOf(n ast.Node) **ast.WithClause {
	switch v := n.(type) {
	case *ast.SelectStmt:
		return &v.With
	case *ast.SetOprStmt:
		return &v.With
	case *ast.UpdateStmt:
		return &v.With
	case *ast.DeleteStmt:
		return &v.With
	}
	return nil
}

func (v *myVisitor) extend(names ...string) map[string]bool {
	out := make(map[string]bool, len(v.scope)+len(names))
	for k := range v.scope {
		out[k] = true
	}
	for _, n := range names {
		out[n] = true
	}
	return out
}

func (v *myVisitor) Enter(n ast.Node) (ast.Node, bool) {
	if p := withOf(n); p != nil && *p != nil {
		wc := *p
		*p = nil // visit the rest of the statement below, under the CTE scope
		v.enterWith(n, wc)
		return n, true
	}
	v.enterNode(n)
	return n, false
}

// enterWith visits the CTE queries under their scopes, then the rest of the statement.
func (v *myVisitor) enterWith(n ast.Node, wc *ast.WithClause) {
	names := make([]string, len(wc.CTEs))
	for i, c := range wc.CTEs {
		names[i] = c.Name.L
	}
	all := v.extend(names...)
	for i, c := range wc.CTEs {
		sc := all
		if !wc.IsRecursive {
			sc = v.extend(names[:i]...)
		}
		c.Query.Accept(&myVisitor{w: v.w, scope: sc})
	}
	rest := all
	switch n.(type) {
	case *ast.UpdateStmt, *ast.DeleteStmt:
		// The target of UPDATE/DELETE is a real table even when a CTE shares its name; a CTE used as a
		// source is then over-checked as a real table too, which only ever refuses more.
		rest = v.scope
	}
	n.Accept(&myVisitor{w: v.w, scope: rest})
}

func (v *myVisitor) enterNode(n ast.Node) {
	switch t := n.(type) {
	case *ast.TableName:
		if t.IsAlias || v.w.skip[t] {
			break
		}
		if t.Schema.O == "" && v.scope[t.Name.L] {
			break
		}
		v.w.addRef(t.Schema.O, t.Name.O)
	case *ast.SelectStmt:
		if t.SelectIntoOpt != nil && t.SelectIntoOpt.Tp != ast.SelectIntoVars {
			v.w.refuse("SELECT INTO file")
		}
	case *ast.FuncCallExpr:
		// The parser types most calls as keyword functions, user ones included, so every call is probed.
		f := adapters.FuncRef{Schema: t.Schema.O, Name: t.FnName.O}
		if f.Schema == "" {
			f.Schema = v.w.db
		}
		if !v.w.funcs[f] {
			v.w.funcs[f] = true
			v.w.out.Funcs = append(v.w.out.Funcs, f)
		}
	case *ast.DeleteStmt:
		// A multi-table DELETE target names a table or alias of the FROM list, which is checked there.
		if t.IsMultiTable && t.Tables != nil {
			for _, tn := range t.Tables.Tables {
				v.w.skip[tn] = true
			}
		}
	}
}

func (v *myVisitor) Leave(n ast.Node) (ast.Node, bool) { return n, true }

// ResolveTables finds stored functions: any call that matches a routine in information_schema is user code
// that can read tables the list cannot see. Loadable UDFs (mysql.func) are not visible there.
func (a *Adapter) ResolveTables(ctx context.Context, p adapters.TableParse, path model.NodePath, op *adapters.OpCtx) ([]tableref.Ref, string, error) {
	if len(p.Funcs) == 0 {
		return nil, "", nil
	}
	entry, release, err := a.requireEntry(ctx, "")
	if err != nil {
		return nil, "", err
	}
	defer release()
	exec := execFor(entry, op, a.trackerFor(op.OpID))
	var (
		conds []string
		args  []any
	)
	for _, f := range p.Funcs {
		conds = append(conds, "(ROUTINE_SCHEMA = ? AND LOWER(ROUTINE_NAME) = LOWER(?))")
		args = append(args, f.Schema, f.Name)
	}
	found := ""
	err = exec(ctx, `SELECT ROUTINE_SCHEMA, ROUTINE_NAME FROM information_schema.ROUTINES
		WHERE ROUTINE_TYPE = 'FUNCTION' AND (`+strings.Join(conds, " OR ")+`) LIMIT 1`, args, func(rows *sql.Rows) error {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			return err
		}
		found = schema + "." + name
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if found != "" {
		return nil, "function " + found, nil
	}
	return nil, "", nil
}
