package dbflow

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// J-DEF-010 / J-DEF-001 (contract studio-definition): the DDL the Definition tab shows recreates
// the table. Run in an empty database, it yields a table that Describe reports identically:
// columns, defaults, primary key, foreign keys, unique and check constraints, indexes, comments.
// The UI half (definition.spec.ts) replays the definition and compares what Copy writes.

// describeOf returns a table's metadata and definition with the fields that name the database
// (paths, row estimates) blanked, so two databases compare equal.
func describeOf(t *testing.T, app *flowharness.App, connID, name string) (model.ObjectMeta, model.ObjectDefinition) {
	t.Helper()
	path := tableNodePath(t, app, connID, name)
	d, err := app.W.Tree.Describe(bridge.TreeDescribeArgs{ConnectionID: connID, Path: path, Refresh: true})
	if err != nil {
		t.Fatalf("Describe %s: %v", name, err)
	}
	def, err := app.W.Tree.Definition(bridge.TreeDescribeArgs{ConnectionID: connID, Path: path, Refresh: true})
	if err != nil {
		t.Fatalf("Definition %s: %v", name, err)
	}
	m := d.Meta
	m.Path, m.RowEstimate = "", nil
	for i := range m.ForeignKeys {
		m.ForeignKeys[i].ReferencedPath = ""
	}
	m.ReferencedBy = nil
	// Indexes list in creation order, which a clone built from the DDL does not share.
	sort.Slice(m.Indexes, func(i, j int) bool { return m.Indexes[i].Name < m.Indexes[j].Name })
	df := def.Definition
	df.Path, df.GeneratedAt = "", ""
	return m, df
}

func dump(v any) string { b, _ := json.MarshalIndent(v, "", " "); return string(b) }

// diffLines lists the lines that differ between two dumps, for a readable failure.
func diffLines(a, b any) string {
	x, y := strings.Split(dump(a), "\n"), strings.Split(dump(b), "\n")
	var out []string
	for i := 0; i < len(x) || i < len(y); i++ {
		var l, r string
		if i < len(x) {
			l = x[i]
		}
		if i < len(y) {
			r = y[i]
		}
		if l != r {
			out = append(out, "line "+itoa(i)+": source "+strings.TrimSpace(l)+" | clone "+strings.TrimSpace(r))
		}
	}
	return strings.Join(out, "\n")
}

const sqliteDefDDL = `
create table def_parent (id integer primary key, code text not null unique);
create table def_child (
  id integer primary key autoincrement,
  parent_id integer not null references def_parent(id) on delete cascade,
  qty integer not null default 1 check (qty > 0),
  label text default 'n/a' collate nocase,
  price real default 0.5,
  slug text generated always as (lower(label)) stored,
  unique (parent_id, label),
  constraint qty_cap check (qty < 1000)
);
create index def_child_label_idx on def_child (label desc);
create index def_child_part_idx on def_child (qty) where qty > 10;
create index def_child_expr_idx on def_child (lower(label));
`

func TestDefinitionDDLRecreatesTableSQLite(t *testing.T) {
	app := flowharness.New(t)
	src := filepath.Join(t.TempDir(), "src.db")
	sqliteExec(t, src, strings.Split(strings.TrimSpace(sqliteDefDDL), ";\n")...)
	srcConn := createConn(t, app, sqliteInput("src", src))
	connect(t, app, srcConn.ID)
	parentMeta, parentDef := describeOf(t, app, srcConn.ID, "def_parent")
	childMeta, childDef := describeOf(t, app, srcConn.ID, "def_child")

	clone := filepath.Join(t.TempDir(), "clone.db")
	sqliteExec(t, clone, `create table seed (id integer)`)
	sqliteExec(t, clone, append(append([]string{}, parentDef.Statements...), childDef.Statements...)...)
	cloneConn := createConn(t, app, sqliteInput("clone", clone))
	connect(t, app, cloneConn.ID)
	cParentMeta, cParentDef := describeOf(t, app, cloneConn.ID, "def_parent")
	cChildMeta, cChildDef := describeOf(t, app, cloneConn.ID, "def_child")

	for name, pair := range map[string][2]any{
		"parent meta": {parentMeta, cParentMeta}, "child meta": {childMeta, cChildMeta},
		"parent definition": {parentDef, cParentDef}, "child definition": {childDef, cChildDef},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s differs:\n%s", name, diffLines(pair[0], pair[1]))
		}
	}
	statements := childDef.Statements
	if len(statements) != 4 || !strings.HasPrefix(statements[0], "CREATE TABLE def_child") {
		t.Fatalf("child statements = %q, want the table then its three indexes", statements)
	}
	childDef.Path, childDef.QualifiedName = "<path>", "main.def_child"
	app.Contract(t, "studio-definition", "sqlite#definition", childDef)
	app.Contract(t, "studio-definition", "sqlite#copy", definitionCopy(childDef))
}

// definitionCopy is what the tab's Copy button writes: each statement ends with a semicolon,
// separated by a blank line.
func definitionCopy(d model.ObjectDefinition) string {
	out := make([]string, len(d.Statements))
	for i, s := range d.Statements {
		out[i] = s + ";"
	}
	return strings.Join(out, "\n\n")
}

const pgDefDDL = `
create table app.def_parent (id integer primary key, code text not null unique);
comment on table app.def_parent is 'parents';
create table app.def_child (
  id integer generated always as identity primary key,
  parent_id integer not null references app.def_parent(id) on delete cascade,
  qty integer not null default 1 check (qty > 0),
  label text default 'n/a',
  price numeric(10,2) default 0.00,
  created timestamptz default now(),
  tags text[] default '{}',
  slug text generated always as (lower(label)) stored,
  unique (parent_id, label),
  constraint qty_cap check (qty < 1000)
);
create index def_child_label_idx on app.def_child (label desc);
create index def_child_part_idx on app.def_child (qty) where qty > 10;
create index def_child_expr_idx on app.def_child (lower(label));
comment on table app.def_child is 'children';
comment on column app.def_child.label is 'display label';
`

func TestDefinitionDDLRecreatesTablePostgres(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartPostgres(t)
	pgExec(t, fx.URI, `drop table if exists app.def_child`, `drop table if exists app.def_parent`)
	pgExec(t, fx.URI, strings.Split(strings.TrimSpace(pgDefDDL), ";\n")...)
	t.Cleanup(func() { pgExec(t, fx.URI, `drop table if exists app.def_child`, `drop table if exists app.def_parent`) })
	app := flowharness.New(t)
	srcConn := pgFixtureConn(t, app, fx)
	parentMeta, parentDef := describeOf(t, app, srcConn.ID, "def_parent")
	childMeta, childDef := describeOf(t, app, srcConn.ID, "def_child")

	// An empty database on the same server, with the schema the statements name.
	pgExec(t, fx.URI, `drop database if exists def_clone`, `create database def_clone`)
	cloneURI := strings.Replace(fx.URI, "/kira_test", "/def_clone", 1)
	pgExec(t, cloneURI, `create schema app`)
	pgExec(t, cloneURI, append(append([]string{}, parentDef.Statements...), childDef.Statements...)...)
	cfg := fx.Config
	cfg.Database = ptr("def_clone")
	cloneConn := createConn(t, app, inputFromConfig(cfg))
	connect(t, app, cloneConn.ID)
	cParentMeta, cParentDef := describeOf(t, app, cloneConn.ID, "def_parent")
	cChildMeta, cChildDef := describeOf(t, app, cloneConn.ID, "def_child")

	for name, pair := range map[string][2]any{
		"parent meta": {parentMeta, cParentMeta}, "child meta": {childMeta, cChildMeta},
		"parent definition": {parentDef, cParentDef}, "child definition": {childDef, cChildDef},
	} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s differs:\n%s", name, diffLines(pair[0], pair[1]))
		}
	}
	if len(childDef.Statements) != 11 {
		t.Fatalf("child statements = %d, want the table, five constraints, three indexes and two comments", len(childDef.Statements))
	}
	childDef.Path = "<path>"
	app.Contract(t, "studio-definition", "pg#definition", childDef)
	app.Contract(t, "studio-definition", "pg#copy", definitionCopy(childDef))
}
