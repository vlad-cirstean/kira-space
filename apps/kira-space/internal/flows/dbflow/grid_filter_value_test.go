package dbflow

import (
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// J-BC-032 (contract studio-filter-by-value): the cell menu's "Filter by this value" builds a
// predicate from the cell's raw value. Each engine stores hostile values and reads them back with
// exactly the filter the grid must send, and each filter returns the rows holding that value and
// no others. The UI half (grid-filter-by-value.spec.ts) replays the rows and compares the filter
// text it sends with this contract. Quoting follows the dialect: SQLite and Postgres double the
// quote only; MySQL, MariaDB and ClickHouse also double the backslash and quote names with backticks.

type filterCase struct {
	label string
	value *string
}

func fvCases() []filterCase {
	s := func(v string) *string { return &v }
	return []filterCase{
		{"quote", s("O'Brien")},
		{"decoy-quote", s("OxBrien")},
		{"backslash", s(`back\slash`)},
		{"trailing-backslash", s(`ends\`)},
		{"percent", s("100%")},
		{"decoy-percent", s("100 percent")},
		{"underscore", s("a_b")},
		{"decoy-underscore", s("aXb")},
		{"unicode", s("ünï ☃ 日本")},
		{"hostile", s(`it's 100% "x"\y`)},
		{"empty", s("")},
		{"null", nil},
	}
}

// filterFor is the predicate the grid builds, written independently of the TypeScript.
func filterFor(backticks bool, v *string) string {
	col := `"v"`
	if backticks {
		col = "`v`"
	}
	if v == nil {
		return col + " IS NULL"
	}
	lit := *v
	if backticks {
		lit = strings.ReplaceAll(lit, `\`, `\\`)
	}
	return col + " = '" + strings.ReplaceAll(lit, "'", "''") + "'"
}

func TestGridFilterByValue(t *testing.T) {
	type engine struct {
		name      string
		backticks bool
		create    string
		open      func(t *testing.T) (*flowharness.App, connections.Input)
	}
	engines := []engine{
		{"sqlite", false, `create table fbv (id integer primary key, v text)`, func(t *testing.T) (*flowharness.App, connections.Input) {
			file := filepath.Join(t.TempDir(), "fbv.db")
			sqliteExec(t, file, `create table seed (id integer primary key)`)
			return flowharness.New(t), sqliteInput("fbv", file)
		}},
		{"postgres", false, `create table app.fbv (id integer primary key, v text)`, func(t *testing.T) (*flowharness.App, connections.Input) {
			flowharness.RequireDocker(t)
			fx := testsupport.StartPostgres(t)
			pgExec(t, fx.URI, `drop table if exists app.fbv`)
			return flowharness.New(t), startPgInput(fx)
		}},
		{"mariadb", true, "create table fbv (id int primary key, v varchar(100) character set utf8mb4)", func(t *testing.T) (*flowharness.App, connections.Input) {
			flowharness.RequireDocker(t)
			fx := testsupport.StartMariadb(t)
			return flowharness.New(t), inputFromConfig(fx.Config)
		}},
		{"clickhouse", true, "create table fbv (id Int32, v Nullable(String)) engine = MergeTree order by id", func(t *testing.T) (*flowharness.App, connections.Input) {
			flowharness.RequireDocker(t)
			fx := testsupport.StartClickHouse(t)
			return flowharness.New(t), inputFromConfig(fx.Config)
		}},
	}
	filters := map[string]map[string]string{}
	matches := map[string]map[string][][]any{}
	cases := fvCases()
	for _, eng := range engines {
		t.Run(eng.name, func(t *testing.T) {
			app, in := eng.open(t)
			conn := createConn(t, app, in)
			connect(t, app, conn.ID)
			ds := app.DataStream(t)
			if eng.name == "mariadb" || eng.name == "clickhouse" {
				t.Cleanup(func() { _, _ = ds.Execute(conn.ID, "", "drop table if exists fbv") })
				_, _ = ds.Execute(conn.ID, "", "drop table if exists fbv")
			}
			if _, err := ds.Execute(conn.ID, "", eng.create); err != nil {
				t.Fatal(err)
			}
			if err := app.W.Tree.Invalidate(bridge.TreeInvalidateArgs{ConnectionID: conn.ID}); err != nil {
				t.Fatal(err)
			}
			path := tableNodePath(t, app, conn.ID, "fbv")

			var ops []model.MutationRowOp
			for i, c := range cases {
				var v any
				if c.value != nil {
					v = *c.value
				}
				ops = append(ops, ins(vals("id", strconv.Itoa(i+1), "v", v)))
			}
			if _, err := ds.Mutate(conn.ID, path, ops); err != nil {
				t.Fatalf("insert: %v", err)
			}

			filters[eng.name], matches[eng.name] = map[string]string{}, map[string][][]any{}
			for i, c := range cases {
				f := filterFor(eng.backticks, c.value)
				page, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: path, Filter: &f})
				if err != nil {
					t.Fatalf("%s: filter %s: %v", c.label, f, err)
				}
				want := [][]any{{strconv.Itoa(i + 1), nil}}
				if c.value != nil {
					want[0][1] = *c.value
				}
				// Rows holding the same value are the match; decoys never are.
				wantRows(t, c.label+" via "+f, page.Rows, want)
				filters[eng.name][c.label] = f
				matches[eng.name][c.label] = page.Rows
			}
		})
	}
	var names []string
	for n := range filters {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) == len(engines) {
		app := flowharness.New(t)
		app.Contract(t, "studio-filter-by-value", "cases", labelsOf(cases))
		app.Contract(t, "studio-filter-by-value", "filters", filters)
		app.Contract(t, "studio-filter-by-value", "matches", matches)
		if !reflect.DeepEqual(names, []string{"clickhouse", "mariadb", "postgres", "sqlite"}) {
			t.Fatalf("engines = %v", names)
		}
	}
}

func labelsOf(cases []filterCase) []map[string]any {
	out := make([]map[string]any, len(cases))
	for i, c := range cases {
		out[i] = map[string]any{"label": c.label, "value": c.value}
	}
	return out
}
