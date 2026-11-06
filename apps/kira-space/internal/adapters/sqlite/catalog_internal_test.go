package sqlite

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// TestPkIsKeysetEligible covers: SQLite is the one dialect here where PRIMARY KEY does
// not imply NOT NULL, so a nullable PK column would otherwise be trusted as a keyset tiebreaker and
// silently drop a NULL-keyed row from every keyset page (or hard-fail keysetValueAt on one). Two
// shapes SQLite itself guarantees can never hold NULL are exempted even though
// table_xinfo's own notnull flag is not rewritten to say so.
func TestPkIsKeysetEligible(t *testing.T) {
	col := func(name, dataType string, nullable bool) model.ColumnMeta {
		return model.ColumnMeta{Name: name, DataType: dataType, Nullable: nullable}
	}

	tests := []struct {
		name         string
		primaryKey   []string
		columns      []model.ColumnMeta
		withoutRowid bool
		want         bool
	}{
		{"no primary key at all", nil, []model.ColumnMeta{col("id", "INTEGER", true)}, false, false},
		{
			"single-column INTEGER PK on a rowid table is the alias — eligible even though nullable",
			[]string{"id"}, []model.ColumnMeta{col("id", "INTEGER", true)}, false, true,
		},
		{
			"single-column INTEGER PK declared NOT NULL is still eligible",
			[]string{"id"}, []model.ColumnMeta{col("id", "INTEGER", false)}, false, true,
		},
		{
			"single-column TEXT PK on a rowid table, nullable — not eligible (the real F8 gap)",
			[]string{"id"}, []model.ColumnMeta{col("id", "TEXT", true)}, false, false,
		},
		{
			"single-column TEXT PK declared NOT NULL is eligible",
			[]string{"id"}, []model.ColumnMeta{col("id", "TEXT", false)}, false, true,
		},
		{
			"composite PK on a rowid table with one nullable column — not eligible",
			[]string{"a", "b"},
			[]model.ColumnMeta{col("a", "INTEGER", false), col("b", "TEXT", true)},
			false, false,
		},
		{
			"composite PK on a rowid table, every column NOT NULL — eligible",
			[]string{"a", "b"},
			[]model.ColumnMeta{col("a", "INTEGER", false), col("b", "TEXT", false)},
			false, true,
		},
		{
			"WITHOUT ROWID table's own PK is eligible even though notnull reports nullable",
			[]string{"a", "b"},
			[]model.ColumnMeta{col("a", "TEXT", true), col("b", "TEXT", true)},
			true, true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pkIsKeysetEligible(tt.primaryKey, tt.columns, tt.withoutRowid); got != tt.want {
				t.Fatalf("pkIsKeysetEligible(%v, %v, withoutRowid=%v) = %v, want %v",
					tt.primaryKey, tt.columns, tt.withoutRowid, got, tt.want)
			}
		})
	}
}
