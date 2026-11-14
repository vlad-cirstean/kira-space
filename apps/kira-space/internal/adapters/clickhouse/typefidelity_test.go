package clickhouse_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func chExec(t *testing.T, a adapters.Adapter, path model.NodePath, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := a.Execute(context.Background(), model.ConsoleRequest{Path: path, Statements: []string{s}}, opCtx(t, "p629-setup")); err != nil {
			t.Fatalf("Execute(%s): %v", s, err)
		}
	}
}

func cellStrings(tp page.TabularPage) [][]string {
	out := make([][]string, tp.RowCount)
	for r := range out {
		for _, ch := range tp.Chunks {
			if page.IsNull(ch, r) {
				out[r] = append(out[r], "<NULL>")
			} else {
				out[r] = append(out[r], page.CellText(ch, r))
			}
		}
	}
	return out
}

// chAnalyticTypes is J-GRID-033: the analytic column types read back as exact text, and an offset
// read of a table with no unique key (duplicate sorting-key values) returns every row exactly once
// across pages (OQ-GRID-4: the order is the sorting key plus the projected columns).
func chAnalyticTypes(cfg model.ResolvedConnectionConfig) testsupport.Scenario {
	db := testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test"))
	tbl := func(name string) model.NodePath {
		return testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test"), testsupport.Seg("table", name))
	}
	return testsupport.Scenario{
		Name:     "types: analytic types and paging tiebreak",
		Requires: func(c adapters.Caps) bool { return c.SQL },
		Run: func(t *testing.T, a adapters.Adapter, _ model.ResolvedConnectionConfig) {
			chExec(t, a, db,
				`DROP TABLE IF EXISTS kira_test.p629_types`, `DROP TABLE IF EXISTS kira_test.p629_ties`,
				`CREATE TABLE kira_test.p629_types (id UInt32, dec Decimal256(10), u64 UInt64, lc LowCardinality(String), nl Nullable(String),
					arr Array(UInt8), mp Map(String, UInt32), dt DateTime64(3, 'UTC')) ENGINE = MergeTree ORDER BY id`,
				`INSERT INTO kira_test.p629_types VALUES
					(1, '12345678901234567890123456789012345678901234567890.1234567891', 18446744073709551615, 'low', 'x', [1,2,3], {'a': 1, 'b': 2}, '2024-03-10 12:34:56.789'),
					(2, 0, 0, '', NULL, [], {}, '1970-01-01 00:00:00.000')`,
				`CREATE TABLE kira_test.p629_ties (k UInt8, v String) ENGINE = MergeTree ORDER BY k`,
				`INSERT INTO kira_test.p629_ties VALUES (1,'a'),(1,'b'),(1,'c'),(2,'d'),(2,'e'),(2,'f'),(3,'g')`,
			)
			t.Cleanup(func() {
				chExec(t, a, db, `DROP TABLE IF EXISTS kira_test.p629_types`, `DROP TABLE IF EXISTS kira_test.p629_ties`)
			})

			p, err := a.Read(context.Background(), adapters.ReadRequest{
				Path: tbl("p629_types"), PageSize: 100, Cursor: model.PageCursor{Mode: "offset"},
				Sort: &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}},
			}, opCtx(t, "p629-read-types"))
			if err != nil {
				t.Fatalf("Read types: %v", err)
			}
			got := cellStrings(p.(page.TabularPage))
			want := [][]string{
				{"1", "12345678901234567890123456789012345678901234567890.1234567891", "18446744073709551615", "low", "x", "[1,2,3]", `{'a':1,'b':2}`, "2024-03-10 12:34:56.789"},
				{"2", "0", "0", "", "<NULL>", "[]", "{}", "1970-01-01 00:00:00.000"},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("types read:\n got %q\nwant %q", got, want)
			}

			var seen []string
			for offset := 0; offset < 14; offset += 3 {
				pg, err := a.Read(context.Background(), adapters.ReadRequest{
					Path: tbl("p629_ties"), PageSize: 3, Cursor: model.PageCursor{Mode: "offset", Offset: offset},
				}, opCtx(t, "p629-read-ties"))
				if err != nil {
					t.Fatalf("Read ties offset %d: %v", offset, err)
				}
				for _, row := range cellStrings(pg.(page.TabularPage)) {
					seen = append(seen, row[1])
				}
			}
			if joined := strings.Join(seen, ""); joined != "abcdefg" {
				t.Fatalf("paged rows = %q, want each of a..g once in key order", joined)
			}
		},
	}
}

// chConsoleFormat is J-CONS-023: UInt64 above 2^53 and a wide Decimal reach the console exact, and a
// user FORMAT clause is refused with a message that names it (OQ-CONS-2).
func chConsoleFormat(cfg model.ResolvedConnectionConfig) testsupport.Scenario {
	db := testsupport.NodePath(cfg.ID, testsupport.Seg("database", "kira_test"))
	return testsupport.Scenario{
		Name:     "console: numeric fidelity and FORMAT refused",
		Requires: func(c adapters.Caps) bool { return c.SQL },
		Run: func(t *testing.T, a adapters.Adapter, _ model.ResolvedConnectionConfig) {
			if got := testsupport.FirstCell(t, a, db, `SELECT toUInt64('18446744073709551615')`); got != "18446744073709551615" {
				t.Errorf("UInt64 max = %q, want 18446744073709551615", got)
			}
			if got := testsupport.FirstCell(t, a, db, `SELECT toDecimal256('12345678901234567890123456789012345678901234567890.1234567891', 10)`); got != "12345678901234567890123456789012345678901234567890.1234567891" {
				t.Errorf("Decimal256 = %q", got)
			}
			for _, sql := range []string{`SELECT 1 FORMAT JSON`, "SELECT 1\nformat CSV;", `SELECT 'FORMAT JSON' AS s FORMAT TabSeparated`} {
				_, err := a.Execute(context.Background(), model.ConsoleRequest{Path: db, Statements: []string{sql}}, opCtx(t, "p629-format"))
				if err == nil {
					t.Fatalf("%q: want a refusal, got nil", sql)
				}
				if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
					t.Errorf("%q: code = %v, want %v", sql, code, adapters.CodeQuery)
				}
				if !strings.Contains(strings.ToLower(err.Error()), "format ") {
					t.Errorf("%q: message %q does not name the FORMAT clause", sql, err.Error())
				}
			}
			// A string that only mentions FORMAT, and a column called format, stay legal.
			if got := testsupport.FirstCell(t, a, db, `SELECT 'FORMAT JSON'`); got != "FORMAT JSON" {
				t.Errorf("literal mentioning FORMAT = %q", got)
			}
		},
	}
}
