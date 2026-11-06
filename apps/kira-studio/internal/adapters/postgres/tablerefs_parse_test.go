package postgres

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
)

func pgRefs(rs ...string) []tableref.Ref {
	out := make([]tableref.Ref, len(rs))
	for i, r := range rs {
		p := strings.Split(r, ".")
		out[i] = tableref.Ref{Database: "shop", Schema: p[0], Name: p[1]}
	}
	return out
}

func TestParseTablesPostgres(t *testing.T) {
	cases := []struct {
		name, sql string
		refs      []string
		unq       []string
		funcs     []string
		refusal   string
	}{
		{name: "plain", sql: `SELECT * FROM public.orders o JOIN sales.items i ON true`, refs: []string{"public.orders", "sales.items"}},
		{name: "unqualified", sql: `SELECT * FROM orders`, unq: []string{"orders"}},
		{name: "case folding", sql: `SELECT * FROM Public."Orders"`, refs: []string{"public.Orders"}},
		{name: "subqueries", sql: `SELECT (SELECT max(x) FROM a.one) FROM a.two WHERE EXISTS (SELECT 1 FROM a.three) AND y IN (SELECT y FROM a.four)`,
			refs: []string{"a.one", "a.two", "a.three", "a.four"}, funcs: []string{"max"}},
		{name: "lateral and union", sql: `SELECT * FROM a.t, LATERAL (SELECT * FROM a.u WHERE u.id = t.id) s UNION SELECT * FROM a.v`, refs: []string{"a.t", "a.u", "a.v"}},
		{name: "cte shadows real table", sql: `WITH secret AS (SELECT 1) SELECT * FROM secret`},
		{name: "non recursive self reference is the real table", sql: `WITH secret AS (SELECT * FROM secret) SELECT * FROM secret`, unq: []string{"secret"}},
		{name: "recursive self reference is the cte", sql: `WITH RECURSIVE r AS (SELECT 1 UNION ALL SELECT n FROM r) SELECT * FROM r`},
		{name: "later cte sees earlier", sql: `WITH a AS (SELECT 1), b AS (SELECT * FROM a) SELECT * FROM b`},
		{name: "earlier cte does not see later", sql: `WITH a AS (SELECT * FROM b), b AS (SELECT 1) SELECT * FROM a`, unq: []string{"b"}},
		{name: "qualified name ignores cte", sql: `WITH t AS (SELECT 1) SELECT * FROM public.t`, refs: []string{"public.t"}},
		{name: "nested with", sql: `WITH a AS (WITH b AS (SELECT 1) SELECT * FROM b) SELECT * FROM a JOIN b ON true`, unq: []string{"b"}},
		{name: "dml target is real even under same cte name", sql: `WITH secret AS (SELECT 1) DELETE FROM secret`, unq: []string{"secret"}},
		{name: "insert select", sql: `INSERT INTO s.dst SELECT * FROM s.src ON CONFLICT DO NOTHING RETURNING (SELECT count(*) FROM s.audit)`, refs: []string{"s.dst", "s.src", "s.audit"}, funcs: []string{"count"}},
		{name: "update from", sql: `UPDATE s.a SET x = b.x FROM s.b WHERE a.id = b.id`, refs: []string{"s.a", "s.b"}},
		{name: "delete using", sql: `DELETE FROM s.a USING s.b WHERE a.id = b.id`, refs: []string{"s.a", "s.b"}},
		{name: "merge", sql: `MERGE INTO s.a USING s.b ON a.id = b.id WHEN MATCHED THEN DELETE`, refs: []string{"s.a", "s.b"}},
		{name: "create table as", sql: `CREATE TABLE s.n AS SELECT * FROM s.o`, refs: []string{"s.n", "s.o"}},
		{name: "create table like inherits", sql: `CREATE TABLE s.n (LIKE s.o) INHERITS (s.p)`, refs: []string{"s.n", "s.o", "s.p"}},
		{name: "partition of", sql: `CREATE TABLE s.c PARTITION OF s.p FOR VALUES IN (1)`, refs: []string{"s.c", "s.p"}},
		{name: "foreign key", sql: `ALTER TABLE s.a ADD FOREIGN KEY (b) REFERENCES s.b (id)`, refs: []string{"s.a", "s.b"}},
		{name: "create view", sql: `CREATE VIEW s.v AS SELECT * FROM s.t`, refs: []string{"s.v", "s.t"}},
		{name: "rename table", sql: `ALTER TABLE s.a RENAME TO b`, refs: []string{"s.a", "s.b"}},
		{name: "drop list", sql: `DROP TABLE IF EXISTS s.a, b, shop.t.c`, unq: []string{"b"}, refs: []string{"s.a", "t.c"}},
		{name: "drop too many parts", sql: `DROP TABLE a.b.c.d`, refusal: "object name"},
		{name: "drop three parts", sql: `DROP TABLE shop.s.a`, refs: []string{"s.a"}},
		{name: "truncate", sql: `TRUNCATE s.a, s.b`, refs: []string{"s.a", "s.b"}},
		{name: "create index", sql: `CREATE INDEX ON s.a (x)`, refs: []string{"s.a"}},
		{name: "comment on column", sql: `COMMENT ON COLUMN s.a.x IS 'c'`, refs: []string{"s.a"}},
		{name: "comment on table", sql: `COMMENT ON TABLE s.a IS 'c'`, refs: []string{"s.a"}},
		{name: "explain", sql: `EXPLAIN SELECT * FROM s.a`, refs: []string{"s.a"}},
		{name: "explain of a call", sql: `EXPLAIN ANALYZE EXECUTE x`, refusal: "ExecuteStmt"},
		{name: "multi statement", sql: `SELECT 1 FROM s.a; SELECT 1 FROM s.b`, refs: []string{"s.a", "s.b"}},
		{name: "multi statement one bad", sql: `SELECT 1 FROM s.a; CALL p()`, refusal: "CallStmt"},
		{name: "builtin func needs probe", sql: `SELECT lower(x) FROM s.a`, refs: []string{"s.a"}, funcs: []string{"lower"}},
		{name: "pg_catalog func", sql: `SELECT pg_catalog.lower(x) FROM s.a`, refs: []string{"s.a"}},
		{name: "user schema func", sql: `SELECT audit.f(1)`, refusal: "function audit.f"},
		{name: "denied builtin", sql: `SELECT query_to_xml('select * from secret', true, false, '')`, refusal: "query_to_xml"},
		{name: "denied builtin qualified", sql: `SELECT pg_catalog.set_config('search_path', 'x', false)`, refusal: "set_config"},
		{name: "table function", sql: `SELECT * FROM generate_series(1, 3)`, funcs: []string{"generate_series"}},
		{name: "qualified operator", sql: `SELECT 1 OPERATOR(public.+) 2`, refusal: "qualified operator"},
		{name: "other database", sql: `SELECT * FROM other.s.a`, refusal: "database other"},
		{name: "call", sql: `CALL p()`, refusal: "CallStmt"},
		{name: "do", sql: `DO $$ BEGIN END $$`, refusal: "DoStmt"},
		{name: "set", sql: `SET search_path = x`, refusal: "VariableSetStmt"},
		{name: "copy", sql: `COPY a TO STDOUT`, refusal: "CopyStmt"},
		{name: "create function", sql: `CREATE FUNCTION f() RETURNS int AS 'select 1' LANGUAGE sql`, refusal: "CreateFunctionStmt"},
		{name: "create trigger", sql: `CREATE TRIGGER t BEFORE INSERT ON s.a EXECUTE FUNCTION f()`, refusal: "CreateTrigStmt"},
		{name: "drop index", sql: `DROP INDEX s.i`, refusal: "DROP"},
		{name: "grant on all tables", sql: `GRANT SELECT ON ALL TABLES IN SCHEMA s TO u`, refusal: "GRANT"},
		{name: "locking clause alias not a table", sql: `SELECT * FROM s.a x FOR UPDATE OF x`, refs: []string{"s.a"}},
		{name: "unparsable", sql: `SELEC nope`, refusal: "parse error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseTablesPostgres(c.sql, "shop")
			if c.refusal != "" {
				if !strings.Contains(got.Refusal, c.refusal) {
					t.Fatalf("refusal = %q; want %q", got.Refusal, c.refusal)
				}
				return
			}
			if got.Refusal != "" {
				t.Fatalf("unexpected refusal %q", got.Refusal)
			}
			if !sameRefs(got.Refs, pgRefs(c.refs...)) || !sameStrings(got.Unqualified, c.unq) || !sameFuncs(got.Funcs, c.funcs) {
				t.Fatalf("got refs=%v unq=%v funcs=%v; want refs=%v unq=%v funcs=%v", got.Refs, got.Unqualified, got.Funcs, c.refs, c.unq, c.funcs)
			}
		})
	}
}

func sameRefs(a, b []tableref.Ref) bool {
	m := map[tableref.Ref]int{}
	for _, r := range a {
		m[r]++
	}
	for _, r := range b {
		m[r]--
	}
	for _, n := range m {
		if n != 0 {
			return false
		}
	}
	return true
}

func sameStrings(a, b []string) bool {
	m := map[string]int{}
	for _, r := range a {
		m[r]++
	}
	for _, r := range b {
		m[r]--
	}
	for _, n := range m {
		if n != 0 {
			return false
		}
	}
	return true
}

func sameFuncs(a []adapters.FuncRef, b []string) bool {
	names := make([]string, len(a))
	for i, f := range a {
		names[i] = f.Name
	}
	return sameStrings(names, b)
}

// Unqualified operators go to the catalog probe: a user operator runs its backing function.
func TestParseTablesPostgresOperators(t *testing.T) {
	got := parseTablesPostgres(`SELECT 1 === 2 FROM s.a WHERE x = ANY (SELECT y FROM s.b) AND z ~~ 'a%' AND w OPERATOR(pg_catalog.+) 1 > 0`, "shop")
	if got.Refusal != "" || !sameStrings(got.Ops, []string{"===", "=", "~~", ">"}) {
		t.Fatalf("refusal=%q ops=%v", got.Refusal, got.Ops)
	}
}
