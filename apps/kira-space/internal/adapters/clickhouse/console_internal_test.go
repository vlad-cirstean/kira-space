// console_internal_test.go guards: run_query classified any bare (non-ANALYZE)
// EXPLAIN as ClassRead via the shared, dialect-agnostic adapters.ClassifySQL — safe on
// Postgres/MySQL/MariaDB/SQLite, whose plain EXPLAIN never executes its target, but not confirmed
// safe on ClickHouse (docs/HISTORY.md "the one it does not clear"). A connection with
// write:deny could otherwise run a write through `EXPLAIN <DELETE ...>` typed straight into
// run_query's raw SQL argument.
package clickhouse

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/queryplan"
)

func TestClassifyClickHouseSQLReclassifiesBareExplainByTarget(t *testing.T) {
	cases := []struct {
		name      string
		statement string
		want      adapters.OpClass
	}{
		{"explain select stays read", "EXPLAIN SELECT * FROM t", adapters.ClassRead},
		{"explain delete is a write, not a read", "EXPLAIN DELETE FROM t WHERE id = 1", adapters.ClassWrite},
		{"explain insert is a write", "EXPLAIN INSERT INTO t VALUES (1)", adapters.ClassWrite},
		{"explain alter is ddl", "EXPLAIN ALTER TABLE t DELETE WHERE id = 1", adapters.ClassDDL},
		{"explain analyze select still classifies its target", "EXPLAIN ANALYZE SELECT * FROM t", adapters.ClassRead},
		{
			"explain analyze delete classifies as write, same as without analyze",
			"EXPLAIN ANALYZE DELETE FROM t WHERE id = 1",
			adapters.ClassWrite,
		},
		{"plain select is unaffected", "SELECT * FROM t", adapters.ClassRead},
		{"plain delete is unaffected", "DELETE FROM t WHERE id = 1", adapters.ClassWrite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyClickHouseSQL(tc.statement); got != tc.want {
				t.Fatalf("classifyClickHouseSQL(%q) = %s, want %s", tc.statement, got, tc.want)
			}
		})
	}
}

// TestClassifyClickHouseSQLHandlesComposedExplainStatements guards: every statement
// queryplan.StatementsFor("clickhouse", ...) composes must classify by its real target,
// not ClassUnknown, so explain_query and run_query's auto-explain never silently fail to
// classify. Built from the composer itself (not a hand-copied string) so the two can't drift
// apart again.
func TestClassifyClickHouseSQLHandlesComposedExplainStatements(t *testing.T) {
	targets := []struct {
		name string
		sql  string
		want adapters.OpClass
	}{
		{"select target", "SELECT * FROM t", adapters.ClassRead},
		{"delete target is still a write under EXPLAIN PLAN/ESTIMATE", "DELETE FROM t WHERE id = 1", adapters.ClassWrite},
	}
	for _, target := range targets {
		for _, composed := range queryplan.StatementsFor("clickhouse", target.sql) {
			t.Run(composed, func(t *testing.T) {
				if got := classifyClickHouseSQL(composed); got != target.want {
					t.Fatalf("classifyClickHouseSQL(%q) = %s, want %s", composed, got, target.want)
				}
			})
		}
	}
}

// TestClassifyClickHouseSQLOtherExplainKinds covers the EXPLAIN kinds StatementsFor doesn't
// compose (AST/SYNTAX/QUERY TREE/PIPELINE) so a future composer addition isn't the only thing
// exercising them.
func TestClassifyClickHouseSQLOtherExplainKinds(t *testing.T) {
	cases := []struct {
		name      string
		statement string
		want      adapters.OpClass
	}{
		{"explain ast select", "EXPLAIN AST SELECT * FROM t", adapters.ClassRead},
		{"explain syntax delete", "EXPLAIN SYNTAX DELETE FROM t WHERE id = 1", adapters.ClassWrite},
		{"explain query tree select", "EXPLAIN QUERY TREE SELECT * FROM t", adapters.ClassRead},
		{"explain pipeline select", "EXPLAIN PIPELINE SELECT * FROM t", adapters.ClassRead},
		{"explain estimate select", "EXPLAIN ESTIMATE SELECT * FROM t", adapters.ClassRead},
		{"explain plan with settings and insert target", "EXPLAIN PLAN json = 1, indexes = 1, description = 1 INSERT INTO t VALUES (1)", adapters.ClassWrite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyClickHouseSQL(tc.statement); got != tc.want {
				t.Fatalf("classifyClickHouseSQL(%q) = %s, want %s", tc.statement, got, tc.want)
			}
		})
	}
}

// isRowReturning decides streamed query vs command: a misroute buffers a whole result and shows
// "0 row(s) written" (P168 Part 3 F12).
func TestIsRowReturning(t *testing.T) {
	cases := []struct {
		sql  string
		want bool
	}{
		{"SELECT 1", true},
		{"-- c\nSELECT 1", true},
		{"# c\nSELECT 1", true},
		{"# a\n-- b\n/* c */ SELECT 1", true},
		{"(SELECT 1) UNION ALL (SELECT 2)", true},
		{"FROM t SELECT x", true},
		{"INSERT INTO t VALUES (1)", false},
		{"# c\nINSERT INTO t VALUES (1)", false},
		{"CREATE TABLE t (x UInt8) ENGINE = Memory", false},
	}
	for _, tc := range cases {
		if got := isRowReturning(tc.sql); got != tc.want {
			t.Errorf("isRowReturning(%q) = %v, want %v", tc.sql, got, tc.want)
		}
	}
}

// decodeRow reads the sentinel as NULL only where the column can be NULL (P168 Part 3 F11).
func TestDecodeRowNullSentinel(t *testing.T) {
	got := decodeRow([]string{nullSentinel, nullSentinel, "x"}, nullableColumns([]string{"Nullable(String)", "String", "LowCardinality(Nullable(String))"}))
	if got[0] != nil || got[1] == nil || *got[1] != nullSentinel || got[2] == nil {
		t.Fatalf("decodeRow = %v %v %v, want NULL, the literal text, a value", got[0], got[1], got[2])
	}
}

func TestEscapeParamValue(t *testing.T) {
	if got, want := escapeParamValue("a\\b\tc\nd\re"), `a\\b\tc\nd\re`; got != want {
		t.Fatalf("escapeParamValue = %q, want %q", got, want)
	}
}
