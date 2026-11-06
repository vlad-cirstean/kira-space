package adapters_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

func TestClassifySQLMySQLLexer(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		want      adapters.OpClass
	}{
		// `--` without trailing whitespace is arithmetic, not a comment: the INTO stays visible.
		{"dash dash without space is not a comment", "SELECT 1--1 INTO OUTFILE '/tmp/x'", adapters.ClassWrite},
		{"dash dash with space is a comment", "SELECT 1 -- INTO OUTFILE '/tmp/x'", adapters.ClassRead},
		{"dash dash at end of input is a comment", "SELECT 1 --", adapters.ClassRead},
		// `#` opens a line comment; the following /* does not open a block comment.
		{"hash comment hides a block opener", "SELECT 1 # /*\nINTO OUTFILE '/tmp/x' -- */", adapters.ClassWrite},
		{"hash comment hides into", "SELECT 1 # INTO OUTFILE '/tmp/x'", adapters.ClassRead},
		{"hash comment before a drop", "# c\nDROP TABLE t", adapters.ClassDDL},
		// Block comments do not nest: the first */ ends the comment, DROP is live.
		{"block comments do not nest", "/* /* */ DROP TABLE t -- */", adapters.ClassDDL},
		{"executable comment body is live", "SELECT 1 /*! INTO OUTFILE '/tmp/x' */", adapters.ClassWrite},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adapters.ClassifySQL(tt.statement, adapters.MySQLDialect); got != tt.want {
				t.Fatalf("ClassifySQL(%q) = %q, want %q", tt.statement, got, tt.want)
			}
		})
	}
}

func TestClassifySQL(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		want      adapters.OpClass
	}{
		{"plain select", "SELECT * FROM t", adapters.ClassRead},
		{"lowercase select", "select * from t", adapters.ClassRead},
		{"select with leading line comment", "-- note\nSELECT 1", adapters.ClassRead},
		{"select with leading block comment", "/* c */ SELECT 1", adapters.ClassRead},
		{"line comment before a drop is still a drop", "-- c\nDROP TABLE t", adapters.ClassDDL},
		{"block comment before an update is still an update", "/* c */ UPDATE t SET x = 1", adapters.ClassWrite},
		{"nested block comments", "SELECT /* a /* b */ c */ 1", adapters.ClassRead},

		{"one trailing semicolon is stripped", "SELECT 1;", adapters.ClassRead},
		{"trailing semicolon with whitespace", "SELECT 1;   ", adapters.ClassRead},
		{"embedded semicolon is unknown", "SELECT 1; DROP TABLE t", adapters.ClassUnknown},
		{"embedded semicolon mid-statement", "DROP TABLE t; SELECT 1", adapters.ClassUnknown},

		// M6 finding #1 (critical): a comment marker hidden inside a string literal must not be
		// read as a real comment, swallowing the real embedded ";" that follows and letting the
		// statement misclassify as a plain read.
		{"block comment marker inside a string is not a real comment", "SELECT '/*' AS a; CREATE TABLE pwned(x int)", adapters.ClassUnknown},
		{"line comment marker inside a string is not a real comment", "SELECT '--' ; DROP TABLE users", adapters.ClassUnknown},
		{"dollar-quoted string hides no comment marker either", "SELECT $$/*$$ ; DROP TABLE users", adapters.ClassUnknown},

		// M7 finding #2 (high): MySQL/MariaDB executes the body of a `/*! ... */` (MariaDB's own
		// `/*M! ... */`) executable comment as real SQL — it must not disappear like an ordinary
		// comment, or the MCP write gate's own INTO special-case never sees it.
		{"mysql executable comment reveals into outfile as a write", "SELECT * FROM users /*! INTO OUTFILE '/tmp/x.csv' */", adapters.ClassWrite},
		{"mysql version-gated executable comment reveals into outfile", "SELECT * FROM users /*!50000 INTO OUTFILE '/tmp/x.csv' */", adapters.ClassWrite},
		{"mariadb executable comment reveals into outfile as a write", "SELECT * FROM users /*M! INTO OUTFILE '/tmp/x.csv' */", adapters.ClassWrite},
		{"ordinary block comment containing into is still just a comment", "SELECT * FROM users /* INTO OUTFILE '/tmp/x.csv' */", adapters.ClassRead},

		{"with select body reads", "WITH x AS (SELECT * FROM t) SELECT * FROM x", adapters.ClassRead},
		{"with a data-modifying cte writes", "WITH x AS (DELETE FROM t RETURNING *) SELECT * FROM x", adapters.ClassWrite},
		{"with followed by insert writes", "WITH x AS (SELECT 1) INSERT INTO t SELECT * FROM x", adapters.ClassWrite},
		{"with followed by ddl is ddl", "WITH x AS (SELECT 1) CREATE TABLE t AS SELECT * FROM x", adapters.ClassDDL},

		{"select into writes", "SELECT * INTO newtable FROM t", adapters.ClassWrite},
		{"select with into as a column name substring is not into", "SELECT intoxicated FROM t", adapters.ClassRead},

		{"show is read", "SHOW TABLES", adapters.ClassRead},
		{"describe is read", "DESCRIBE t", adapters.ClassRead},
		{"desc is read", "DESC t", adapters.ClassRead},
		{"describe analyze delete is a write", "DESCRIBE ANALYZE DELETE o FROM orders o JOIN users u ON u.id = o.user_id", adapters.ClassWrite},
		{"desc analyze select is a read", "DESC ANALYZE SELECT * FROM t", adapters.ClassRead},
		{"mariadb analyze delete is a write", "ANALYZE DELETE FROM t", adapters.ClassWrite},
		{"mariadb analyze format json update is a write", "ANALYZE FORMAT=JSON UPDATE t SET x = 1", adapters.ClassWrite},
		{"mariadb analyze select is a read", "ANALYZE SELECT * FROM t", adapters.ClassRead},
		{"analyze table stays ddl", "ANALYZE TABLE t", adapters.ClassDDL},
		{"analyze bare table stays ddl", "ANALYZE t", adapters.ClassDDL},
		{"values is read", "VALUES (1, 2)", adapters.ClassRead},
		{"table is read", "TABLE t", adapters.ClassRead},

		{"explain select is read", "EXPLAIN SELECT * FROM t", adapters.ClassRead},
		{"explain analyze delete is a write", "EXPLAIN ANALYZE DELETE FROM t", adapters.ClassWrite},
		{"explain analyze select is a read", "EXPLAIN ANALYZE SELECT * FROM t", adapters.ClassRead},
		{"explain analyze create is ddl", "EXPLAIN ANALYZE CREATE TABLE t (id int)", adapters.ClassDDL},

		// M6 finding #2 (high): Postgres's parenthesised EXPLAIN option-list form must also be
		// recognized as running the target statement when ANALYZE is one of the options — the
		// token right after EXPLAIN there is "(ANALYZE)"/"(ANALYZE,", never bare "ANALYZE".
		{"explain parenthesized analyze delete is a write", "EXPLAIN (ANALYZE, BUFFERS) DELETE FROM users", adapters.ClassWrite},
		{"explain parenthesized bare analyze delete is a write", "EXPLAIN (ANALYZE) DELETE FROM users", adapters.ClassWrite},
		{"explain parenthesized analyze select is a read", "EXPLAIN (ANALYZE) SELECT * FROM t", adapters.ClassRead},
		{"explain parenthesized options without analyze is read", "EXPLAIN (BUFFERS, COSTS FALSE) DELETE FROM users", adapters.ClassRead},
		{"explain unbalanced parenthesized options is unknown", "EXPLAIN (ANALYZE DELETE FROM users", adapters.ClassUnknown},

		{"insert writes", "INSERT INTO t VALUES (1)", adapters.ClassWrite},
		{"update writes", "UPDATE t SET x = 1", adapters.ClassWrite},
		{"delete writes", "DELETE FROM t", adapters.ClassWrite},
		{"merge writes", "MERGE INTO t USING s ON t.id = s.id WHEN MATCHED THEN UPDATE SET x = 1", adapters.ClassWrite},
		{"replace writes", "REPLACE INTO t VALUES (1)", adapters.ClassWrite},
		{"copy writes", "COPY t FROM STDIN", adapters.ClassWrite},
		{"load writes", "LOAD DATA INFILE 'x' INTO TABLE t", adapters.ClassWrite},

		{"create is ddl", "CREATE TABLE t (id int)", adapters.ClassDDL},
		{"alter is ddl", "ALTER TABLE t ADD COLUMN x int", adapters.ClassDDL},
		{"drop is ddl", "DROP TABLE t", adapters.ClassDDL},
		{"truncate is ddl, not write", "TRUNCATE TABLE t", adapters.ClassDDL},
		{"grant is ddl", "GRANT SELECT ON t TO u", adapters.ClassDDL},
		{"vacuum is ddl", "VACUUM t", adapters.ClassDDL},
		{"standalone analyze is ddl", "ANALYZE t", adapters.ClassDDL},

		{"pragma is unknown", "PRAGMA journal_mode=WAL", adapters.ClassUnknown},
		{"set is unknown", "SET SESSION x = 1", adapters.ClassUnknown},
		{"use is unknown", "USE mydb", adapters.ClassUnknown},
		{"begin is unknown", "BEGIN", adapters.ClassUnknown},
		{"commit is unknown", "COMMIT", adapters.ClassUnknown},
		{"rollback is unknown", "ROLLBACK", adapters.ClassUnknown},
		{"call is unknown", "CALL my_proc()", adapters.ClassUnknown},
		{"empty statement is unknown", "", adapters.ClassUnknown},
		{"whitespace-only statement is unknown", "   \n\t  ", adapters.ClassUnknown},
		{"comment-only statement is unknown", "-- just a comment", adapters.ClassUnknown},

		// A Postgres E'...' string's backslash-escaped quote was never
		// recognized as an escape, so the scanner read the string as closing right after it —
		// exposing the genuine trailing `-- '` as if it opened a real line comment, which then
		// swallowed the real `;` and the smuggled DELETE along with it. Pre-fix this classified
		// ClassRead (confirmed against the pre-fix scanner); the embedded-semicolon guard now runs
		// against the raw statement, so it classifies ClassUnknown regardless of whether the
		// E-string scanning fix alone would also have exposed the real `;`.
		{"postgres E-string backslash-escaped quote hides a real semicolon and delete", `SELECT E'\' -- ' ; DELETE FROM t`, adapters.ClassUnknown},
		{"lowercase e-string backslash-escaped quote hides a real semicolon", `SELECT e'\' -- ' ; DELETE FROM t`, adapters.ClassUnknown},

		// P168 Part 3 F3: ANALYSE executes the target, and WITH ... SELECT INTO creates a table.
		{"explain analyse delete is a write", "EXPLAIN ANALYSE DELETE FROM t", adapters.ClassWrite},
		{"explain parenthesized analyse delete is a write", "EXPLAIN (ANALYSE) DELETE FROM t", adapters.ClassWrite},
		{"explain parenthesized analyse with options", "EXPLAIN (BUFFERS, ANALYSE) DELETE FROM t", adapters.ClassWrite},
		{"with select into writes", "WITH a AS (SELECT 1) SELECT * INTO newt FROM a", adapters.ClassWrite},
		{"with select without into reads", "WITH a AS (SELECT 1) SELECT * FROM a", adapters.ClassRead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adapters.ClassifySQL(tt.statement, adapters.PostgresDialect); got != tt.want {
				t.Fatalf("ClassifySQL(%q) = %q, want %q", tt.statement, got, tt.want)
			}
		})
	}
}
