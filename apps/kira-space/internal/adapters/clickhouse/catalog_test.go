package clickhouse

import "testing"

// TestListCheckConstraints is the one thing in this package that clears CLAUDE.md's unit-test bar
// on its own: a small parenthesis-aware
// parser over CREATE TABLE text with several interacting lexical rules — backtick-quoted names
// with doubled backticks, nested parentheses inside the CHECK expression, and 'ASSUME' deliberately
// excluded (a query-optimizer hint, not a constraint a user would recognise as one).
func TestListCheckConstraints(t *testing.T) {
	cases := []struct {
		name string
		ddl  string
		want []constraintRow
	}{
		{
			name: "no constraints at all",
			ddl:  "CREATE TABLE t (id UInt32, name String) ENGINE = MergeTree ORDER BY id",
			want: nil,
		},
		{
			name: "a single unnamed-looking CHECK constraint",
			ddl: "CREATE TABLE order_items (\n" +
				"    id UInt32,\n" +
				"    order_id UInt32,\n" +
				"    quantity Int32,\n" +
				"    CONSTRAINT order_items_quantity_positive CHECK quantity > 0\n" +
				") ENGINE = MergeTree ORDER BY id",
			want: []constraintRow{
				{Name: "order_items_quantity_positive", Type: "CHECK", Expression: "quantity > 0"},
			},
		},
		{
			name: "a backtick-quoted constraint name with a doubled backtick",
			ddl: "CREATE TABLE t (\n" +
				"    id UInt32,\n" +
				"    CONSTRAINT `weird``check` CHECK id > 0\n" +
				") ENGINE = MergeTree ORDER BY id",
			want: []constraintRow{
				{Name: "weird`check", Type: "CHECK", Expression: "id > 0"},
			},
		},
		{
			name: "a CHECK expression itself containing nested parentheses and a comma",
			ddl: "CREATE TABLE t (\n" +
				"    id UInt32,\n" +
				"    a UInt32,\n" +
				"    b UInt32,\n" +
				"    CONSTRAINT c1 CHECK greatest(a, b) > 0\n" +
				") ENGINE = MergeTree ORDER BY id",
			want: []constraintRow{
				{Name: "c1", Type: "CHECK", Expression: "greatest(a, b) > 0"},
			},
		},
		{
			name: "two CHECK constraints, comma-separated at the top level",
			ddl: "CREATE TABLE t (\n" +
				"    id UInt32,\n" +
				"    a UInt32,\n" +
				"    CONSTRAINT c1 CHECK a > 0,\n" +
				"    CONSTRAINT c2 CHECK a < 100\n" +
				") ENGINE = MergeTree ORDER BY id",
			want: []constraintRow{
				{Name: "c1", Type: "CHECK", Expression: "a > 0"},
				{Name: "c2", Type: "CHECK", Expression: "a < 100"},
			},
		},
		{
			name: "ASSUME is a query-optimizer hint, never matched as a constraint",
			ddl: "CREATE TABLE t (\n" +
				"    id UInt32,\n" +
				"    a UInt32,\n" +
				"    CONSTRAINT c1 ASSUME a > 0\n" +
				") ENGINE = MergeTree ORDER BY id",
			want: nil,
		},
		{
			name: "no opening parenthesis at all",
			ddl:  "CREATE TABLE t ENGINE = MergeTree",
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := listCheckConstraints(tc.ddl)
			if len(got) != len(tc.want) {
				t.Fatalf("listCheckConstraints() = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("constraint[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSplitTopLevelCommas(t *testing.T) {
	cases := []struct {
		expr string
		want []string
	}{
		{"", nil},
		{"id", []string{"id"}},
		{"tenant_id, entity_id", []string{"tenant_id", "entity_id"}},
		{"toYYYYMM(d), id", []string{"toYYYYMM(d)", "id"}},
		{"greatest(a, b), c", []string{"greatest(a, b)", "c"}},
	}
	for _, tc := range cases {
		got := splitTopLevelCommas(tc.expr)
		if len(got) != len(tc.want) {
			t.Fatalf("splitTopLevelCommas(%q) = %v, want %v", tc.expr, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitTopLevelCommas(%q)[%d] = %q, want %q", tc.expr, i, got[i], tc.want[i])
			}
		}
	}
}
