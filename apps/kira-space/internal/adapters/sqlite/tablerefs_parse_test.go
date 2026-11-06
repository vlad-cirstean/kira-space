package sqlite

import (
	"strings"
	"testing"
)

func TestParseTablesSQLite(t *testing.T) {
	cases := []struct {
		name, sql string
		refs      []string
		refusal   string
	}{
		{name: "plain", sql: "SELECT * FROM orders o JOIN main.items i ON o.id = i.id", refs: []string{"orders", "items"}},
		{name: "subqueries", sql: "SELECT (SELECT max(x) FROM one) FROM two WHERE EXISTS (SELECT 1 FROM three) AND y IN (SELECT y FROM four)",
			refs: []string{"one", "two", "three", "four"}},
		{name: "compound", sql: "SELECT a FROM t1 UNION SELECT a FROM t2", refs: []string{"t1", "t2"}},
		{name: "cte shadows real table", sql: "WITH secret AS (SELECT 1) SELECT * FROM secret"},
		{name: "cte shadow is case insensitive", sql: "WITH Secret AS (SELECT 1) SELECT * FROM SECRET"},
		{name: "non recursive self reference is the real table", sql: "WITH secret AS (SELECT * FROM secret) SELECT * FROM secret", refs: []string{"secret"}},
		{name: "recursive self reference is the cte", sql: "WITH RECURSIVE r(n) AS (SELECT 1 UNION ALL SELECT n FROM r) SELECT * FROM r"},
		{name: "later cte sees earlier", sql: "WITH a AS (SELECT 1), b AS (SELECT * FROM a) SELECT * FROM b"},
		{name: "earlier cte does not see later", sql: "WITH a AS (SELECT * FROM b), b AS (SELECT 1) SELECT * FROM a", refs: []string{"b"}},
		{name: "qualified name ignores cte", sql: "WITH t AS (SELECT 1) SELECT * FROM main.t", refs: []string{"t"}},
		{name: "nested with", sql: "WITH a AS (WITH b AS (SELECT 1) SELECT * FROM b) SELECT * FROM a JOIN b ON 1", refs: []string{"b"}},
		{name: "delete target is real under same cte name", sql: "WITH secret AS (SELECT 1) DELETE FROM secret", refs: []string{"secret"}},
		{name: "update target is real under same cte name", sql: "WITH secret AS (SELECT 1) UPDATE secret SET a = 1", refs: []string{"secret"}},
		{name: "insert select", sql: "INSERT INTO dst SELECT * FROM src", refs: []string{"dst", "src"}},
		{name: "insert target under same cte name", sql: "WITH secret AS (SELECT 1) INSERT INTO secret SELECT * FROM secret", refs: []string{"secret"}},
		{name: "update from", sql: "UPDATE a SET x = b.x FROM b WHERE a.id = b.id", refs: []string{"a", "b"}},
		{name: "create table as", sql: "CREATE TABLE n AS SELECT * FROM o", refs: []string{"n", "o"}},
		{name: "foreign key", sql: "CREATE TABLE n (id INT REFERENCES p (id))", refs: []string{"n", "p"}},
		{name: "create view", sql: "CREATE VIEW v AS SELECT * FROM t", refs: []string{"v", "t"}},
		{name: "alter rename", sql: "ALTER TABLE a RENAME TO b", refs: []string{"a", "b"}},
		{name: "drop", sql: "DROP TABLE IF EXISTS a", refs: []string{"a"}},
		{name: "create index", sql: "CREATE INDEX i ON a (x)", refs: []string{"a"}},
		{name: "explain query plan", sql: "EXPLAIN QUERY PLAN SELECT * FROM a", refs: []string{"a"}},
		{name: "multi statement", sql: "SELECT 1 FROM a; SELECT 1 FROM b", refs: []string{"a", "b"}},
		{name: "multi statement one bad", sql: "SELECT 1 FROM a; PRAGMA table_info(b)", refusal: "PragmaStatement"},
		{name: "transaction", sql: "BEGIN; SELECT 1 FROM a; COMMIT", refs: []string{"a"}},
		{name: "in table", sql: "SELECT * FROM a WHERE x IN secret", refusal: "IN with a table name"},
		{name: "temp schema", sql: "SELECT * FROM temp.t", refusal: "parse error"},
		{name: "attached schema", sql: "SELECT * FROM other.t", refusal: "schema other"},
		{name: "pragma function", sql: "SELECT * FROM pragma_table_info('t')", refusal: "table function pragma_table_info"},
		{name: "load_extension", sql: "SELECT load_extension('x')", refusal: "load_extension"},
		{name: "attach", sql: "ATTACH DATABASE 'x.db' AS o", refusal: "AttachStatement"},
		{name: "detach", sql: "DETACH DATABASE o", refusal: "DetachStatement"},
		{name: "vacuum", sql: "VACUUM", refusal: "VacuumStatement"},
		{name: "reindex", sql: "REINDEX a", refusal: "ReindexStatement"},
		{name: "create trigger", sql: "CREATE TRIGGER t AFTER INSERT ON a BEGIN SELECT 1; END", refusal: "CreateTriggerStatement"},
		{name: "drop index", sql: "DROP INDEX i", refusal: "DropIndexStatement"},
		{name: "unparsable", sql: "SELEC nope", refusal: "parse error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseTablesSQLite(c.sql)
			if c.refusal != "" {
				if !strings.Contains(got.Refusal, c.refusal) {
					t.Fatalf("refusal = %q; want %q", got.Refusal, c.refusal)
				}
				return
			}
			if got.Refusal != "" {
				t.Fatalf("unexpected refusal %q", got.Refusal)
			}
			have := map[string]int{}
			for _, r := range got.Refs {
				have[strings.ToLower(r.Name)]++
			}
			for _, w := range c.refs {
				have[w]--
			}
			for k, n := range have {
				if n != 0 {
					t.Fatalf("got refs %v; want %v (diff at %q)", got.Refs, c.refs, k)
				}
			}
		})
	}
}
