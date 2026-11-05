package mysqlfamily

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
)

func TestParseTablesMySQL(t *testing.T) {
	cases := []struct {
		name, sql string
		refs      []string // db.name
		funcs     []string // db.name
		refusal   string
	}{
		{name: "plain", sql: "SELECT * FROM orders o JOIN other.items i ON 1", refs: []string{"shop.orders", "other.items"}},
		{name: "subqueries", sql: "SELECT (SELECT MAX(x) FROM one) FROM two WHERE EXISTS (SELECT 1 FROM three) AND y IN (SELECT y FROM four)",
			refs: []string{"shop.one", "shop.two", "shop.three", "shop.four"}},
		{name: "union", sql: "SELECT a FROM t1 UNION SELECT a FROM t2", refs: []string{"shop.t1", "shop.t2"}},
		{name: "derived table", sql: "SELECT * FROM (SELECT * FROM inner_t) d", refs: []string{"shop.inner_t"}},
		{name: "cte shadows real table", sql: "WITH secret AS (SELECT 1) SELECT * FROM secret"},
		{name: "cte shadow is case insensitive", sql: "WITH Secret AS (SELECT 1) SELECT * FROM secret"},
		{name: "non recursive self reference is the real table", sql: "WITH secret AS (SELECT * FROM secret) SELECT * FROM secret", refs: []string{"shop.secret"}},
		{name: "recursive self reference is the cte", sql: "WITH RECURSIVE r AS (SELECT 1 AS n UNION ALL SELECT n FROM r) SELECT * FROM r"},
		{name: "later cte sees earlier", sql: "WITH a AS (SELECT 1), b AS (SELECT * FROM a) SELECT * FROM b"},
		{name: "earlier cte does not see later", sql: "WITH a AS (SELECT * FROM b), b AS (SELECT 1) SELECT * FROM a", refs: []string{"shop.b"}},
		{name: "qualified name ignores cte", sql: "WITH t AS (SELECT 1) SELECT * FROM shop.t", refs: []string{"shop.t"}},
		{name: "nested with", sql: "WITH a AS (WITH b AS (SELECT 1) SELECT * FROM b) SELECT * FROM a JOIN b ON 1", refs: []string{"shop.b"}},
		{name: "delete target is real under same cte name", sql: "WITH secret AS (SELECT 1) DELETE FROM secret", refs: []string{"shop.secret"}},
		{name: "update target is real under same cte name", sql: "WITH secret AS (SELECT 1) UPDATE secret SET a = 1", refs: []string{"shop.secret"}},
		{name: "insert select", sql: "INSERT INTO dst SELECT * FROM src", refs: []string{"shop.dst", "shop.src"}},
		{name: "insert target under cte of same name", sql: "INSERT INTO secret WITH secret AS (SELECT 1) SELECT * FROM secret", refs: []string{"shop.secret"}},
		{name: "update join", sql: "UPDATE a JOIN b ON a.id = b.id SET a.x = b.x", refs: []string{"shop.a", "shop.b"}},
		{name: "multi table delete", sql: "DELETE x FROM a AS x JOIN b ON x.id = b.id", refs: []string{"shop.a", "shop.b"}},
		{name: "replace", sql: "REPLACE INTO t VALUES (1)", refs: []string{"shop.t"}},
		{name: "create table like", sql: "CREATE TABLE n LIKE o", refs: []string{"shop.n", "shop.o"}},
		{name: "create table as", sql: "CREATE TABLE n AS SELECT * FROM o", refs: []string{"shop.n", "shop.o"}},
		{name: "foreign key", sql: "CREATE TABLE n (id INT, FOREIGN KEY (id) REFERENCES other.p (id))", refs: []string{"shop.n", "other.p"}},
		{name: "create view", sql: "CREATE VIEW v AS SELECT * FROM t", refs: []string{"shop.v", "shop.t"}},
		{name: "alter rename", sql: "ALTER TABLE a RENAME TO b", refs: []string{"shop.a", "shop.b"}},
		{name: "rename table", sql: "RENAME TABLE a TO b, c TO d", refs: []string{"shop.a", "shop.b", "shop.c", "shop.d"}},
		{name: "drop list", sql: "DROP TABLE IF EXISTS a, other.b", refs: []string{"shop.a", "other.b"}},
		{name: "truncate", sql: "TRUNCATE TABLE a", refs: []string{"shop.a"}},
		{name: "create index", sql: "CREATE INDEX i ON a (x)", refs: []string{"shop.a"}},
		{name: "explain", sql: "EXPLAIN SELECT * FROM a", refs: []string{"shop.a"}},
		{name: "multi statement", sql: "SELECT 1 FROM a; SELECT 1 FROM b", refs: []string{"shop.a", "shop.b"}},
		{name: "multi statement one bad", sql: "SELECT 1 FROM a; CALL p()", refusal: "CallStmt"},
		{name: "executable comment is code", sql: "/*!50000 SELECT * FROM secret */", refs: []string{"shop.secret"}},
		{name: "stored function", sql: "SELECT audit.f(1), g(2) FROM a", refs: []string{"shop.a"}, funcs: []string{"audit.f", "shop.g"}},
		{name: "builtin functions are probed too", sql: "SELECT LOWER(x), COUNT(*) FROM a", refs: []string{"shop.a"}, funcs: []string{"shop.LOWER"}},
		{name: "call", sql: "CALL p()", refusal: "CallStmt"},
		{name: "set", sql: "SET @a = 1", refusal: "SetStmt"},
		{name: "use", sql: "USE other", refusal: "UseStmt"},
		{name: "show", sql: "SHOW TABLES", refusal: "ShowStmt"},
		{name: "describe", sql: "DESCRIBE a", refusal: "ShowStmt"},
		{name: "into outfile", sql: "SELECT * FROM a INTO OUTFILE '/tmp/x'", refusal: "INTO file"},
		{name: "load data", sql: "LOAD DATA INFILE '/tmp/x' INTO TABLE a", refusal: "LoadDataStmt"},
		{name: "create function", sql: "CREATE FUNCTION f() RETURNS INT RETURN 1", refusal: "parse error"},
		{name: "create trigger", sql: "CREATE TRIGGER t BEFORE INSERT ON a FOR EACH ROW SET NEW.x = 1", refusal: "parse error"},
		{name: "merge engine", sql: "CREATE TABLE m (a INT) ENGINE=MERGE UNION=(t1,t2)", refusal: "ENGINE"},
		{name: "federated engine", sql: "CREATE TABLE m (a INT) ENGINE=FEDERATED", refusal: "ENGINE"},
		{name: "unparsable", sql: "SELEC nope", refusal: "parse error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseTablesMySQL(c.sql, "shop")
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

func TestParseTablesMySQLNoDatabase(t *testing.T) {
	if got := parseTablesMySQL("SELECT * FROM a", ""); !strings.Contains(got.Refusal, "no database") {
		t.Fatalf("refusal = %q", got.Refusal)
	}
	if got := parseTablesMySQL("SELECT * FROM x.a", ""); got.Refusal != "" || len(got.Refs) != 1 {
		t.Fatalf("qualified name needs no default: %+v", got)
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
		got[f.Schema+"."+f.Name]++
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
