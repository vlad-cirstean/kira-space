package clickhouse

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
)

func TestParseTablesClickHouse(t *testing.T) {
	cases := []struct {
		name, sql string
		refs      []string
		funcs     []string
		refusal   string
	}{
		{name: "plain", sql: "SELECT * FROM orders o JOIN other.items i ON o.id = i.id", refs: []string{"shop.orders", "other.items"}},
		{name: "subqueries", sql: "SELECT (SELECT max(x) FROM one) FROM two WHERE y IN (SELECT y FROM three)",
			refs: []string{"shop.one", "shop.two", "shop.three"}, funcs: []string{"max"}},
		{name: "union", sql: "SELECT a FROM t1 UNION ALL SELECT a FROM t2", refs: []string{"shop.t1", "shop.t2"}},
		{name: "cte shadows real table", sql: "WITH secret AS (SELECT 1) SELECT * FROM secret"},
		{name: "non recursive self reference is the real table", sql: "WITH secret AS (SELECT * FROM secret) SELECT * FROM secret", refs: []string{"shop.secret"}},
		{name: "later cte sees earlier", sql: "WITH a AS (SELECT 1), b AS (SELECT * FROM a) SELECT * FROM b"},
		{name: "earlier cte does not see later", sql: "WITH a AS (SELECT * FROM b), b AS (SELECT 1) SELECT * FROM a", refs: []string{"shop.b"}},
		{name: "scalar with is not a table", sql: "WITH 5 AS k SELECT k FROM secret", refs: []string{"shop.secret"}},
		{name: "qualified name ignores cte", sql: "WITH t AS (SELECT 1) SELECT * FROM shop.t", refs: []string{"shop.t"}},
		{name: "nested with", sql: "WITH a AS (WITH b AS (SELECT 1) SELECT * FROM b) SELECT * FROM a JOIN b ON 1", refs: []string{"shop.b"}},
		{name: "insert values", sql: "INSERT INTO t (a) VALUES (1)", refs: []string{"shop.t"}},
		{name: "insert select", sql: "INSERT INTO dst SELECT * FROM src", refs: []string{"shop.dst", "shop.src"}},
		{name: "insert into function", sql: "INSERT INTO FUNCTION remote('h', db.t) SELECT 1", refusal: "INSERT INTO FUNCTION"},
		{name: "alter delete subquery", sql: "ALTER TABLE a DELETE WHERE x IN (SELECT 1 FROM q)", refs: []string{"shop.a", "shop.q"}},
		{name: "alter attach from", sql: "ALTER TABLE a ATTACH PARTITION 1 FROM b", refs: []string{"shop.a", "shop.b"}},
		{name: "lightweight delete", sql: "DELETE FROM a WHERE 1", refs: []string{"shop.a"}},
		{name: "create table as", sql: "CREATE TABLE n AS o ENGINE = MergeTree ORDER BY tuple()", refs: []string{"shop.n", "shop.o"}, funcs: []string{"tuple"}},
		{name: "create table select", sql: "CREATE TABLE n ENGINE = MergeTree ORDER BY tuple() AS SELECT * FROM o", refs: []string{"shop.n", "shop.o"}, funcs: []string{"tuple"}},
		{name: "replicated mergetree", sql: "CREATE TABLE n (a Int32) ENGINE = ReplicatedMergeTree ORDER BY a", refs: []string{"shop.n"}},
		{name: "materialized view destination", sql: "CREATE MATERIALIZED VIEW v TO d.t AS SELECT * FROM s", refs: []string{"shop.v", "d.t", "shop.s"}},
		{name: "create view", sql: "CREATE VIEW v AS SELECT * FROM t", refs: []string{"shop.v", "shop.t"}},
		{name: "rename", sql: "RENAME TABLE a TO b", refs: []string{"shop.a", "shop.b"}},
		{name: "drop", sql: "DROP TABLE IF EXISTS a.b", refs: []string{"a.b"}},
		{name: "truncate", sql: "TRUNCATE TABLE a", refs: []string{"shop.a"}},
		{name: "optimize", sql: "OPTIMIZE TABLE a FINAL", refs: []string{"shop.a"}},
		{name: "explain", sql: "EXPLAIN SYNTAX SELECT * FROM a", refs: []string{"shop.a"}},
		{name: "multi statement one bad", sql: "SELECT 1 FROM a; SYSTEM FLUSH LOGS", refusal: "SystemStmt"},
		{name: "in table", sql: "SELECT * FROM a WHERE x IN secret", refusal: "IN with a table name"},
		{name: "in function table", sql: "SELECT count() FROM a WHERE in(id, secret)", refusal: "IN with a table name"},
		{name: "global not in function table", sql: "SELECT 1 FROM a WHERE globalNotIn(id, secret)", refusal: "IN with a table name"},
		{name: "in ignore set function table", sql: "SELECT 1 FROM a WHERE globalNullInIgnoreSet(id, secret)", refusal: "IN with a table name"},
		{name: "in parenthesised table", sql: "SELECT 1 FROM a WHERE id IN (secret)", refusal: "IN with a table name"},
		{name: "in function tuple", sql: "SELECT 1 FROM a WHERE in(id, (1, 2))", refs: []string{"shop.a"}, funcs: []string{"in"}},
		{name: "in function subquery", sql: "SELECT 1 FROM a WHERE in(id, (SELECT 1 FROM b))", refs: []string{"shop.a", "shop.b"}, funcs: []string{"in"}},
		{name: "numbers table function", sql: "SELECT * FROM numbers(3)"},
		{name: "remote table function", sql: "SELECT * FROM remote('h', db.t)", refusal: "table function remote"},
		{name: "merge table function", sql: "SELECT * FROM merge(currentDatabase(), '^t')", refusal: "table function merge"},
		{name: "dictGet", sql: "SELECT dictGet('d', 'a', 1)", refusal: "function dictget"},
		{name: "joinGet", sql: "SELECT joinGet('db.j', 'a', 1)", refusal: "function joinget"},
		{name: "udf probed", sql: "SELECT myudf(1) FROM a", refs: []string{"shop.a"}, funcs: []string{"myudf"}},
		{name: "merge engine", sql: "CREATE TABLE n AS o ENGINE = Merge(currentDatabase(), 'x')", refusal: "ENGINE Merge"},
		{name: "distributed engine", sql: "CREATE TABLE n (a Int32) ENGINE = Distributed(c, db, t)", refusal: "ENGINE Distributed"},
		{name: "set", sql: "SET max_threads = 1", refusal: "SetStmt"},
		{name: "use", sql: "USE other", refusal: "UseStmt"},
		{name: "show", sql: "SHOW TABLES", refusal: "ShowStmt"},
		{name: "describe", sql: "DESCRIBE a", refusal: "DescribeStmt"},
		{name: "drop dictionary", sql: "DROP DICTIONARY d", refusal: "DROP"},
		{name: "unparsable", sql: "SELEC nope", refusal: "parse error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseTablesClickHouse(c.sql, "shop")
			if c.refusal != "" {
				if !strings.Contains(got.Refusal, c.refusal) {
					t.Fatalf("refusal = %q; want %q", got.Refusal, c.refusal)
				}
				return
			}
			if got.Refusal != "" {
				t.Fatalf("unexpected refusal %q", got.Refusal)
			}
			if !sameRefs(got.Refs, c.refs) || !sameFuncs(got.Funcs, c.funcs) {
				t.Fatalf("got refs=%v funcs=%v; want refs=%v funcs=%v", got.Refs, got.Funcs, c.refs, c.funcs)
			}
		})
	}
}

func sameRefs(a []tableref.Ref, want []string) bool {
	got := map[string]int{}
	for _, r := range a {
		got[r.Database+"."+r.Name]++
	}
	for _, w := range want {
		got[w]--
	}
	for _, n := range got {
		if n != 0 {
			return false
		}
	}
	return true
}

func sameFuncs(a []adapters.FuncRef, want []string) bool {
	got := map[string]int{}
	for _, f := range a {
		got[f.Name]++
	}
	for _, w := range want {
		got[w]--
	}
	for _, n := range got {
		if n != 0 {
			return false
		}
	}
	return true
}
