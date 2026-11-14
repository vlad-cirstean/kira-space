package testsupport

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// TypeRoundTripCase drives TypeRoundTrip. Setup creates Table with an integer id primary key and the
// typed columns; Want is every row ordered by id, one cell per column in Columns order, nil for NULL.
// The edit changes EditColumn of the row with id EditID; every other cell must read back unchanged,
// which catches a rewrite of an untouched TIMESTAMP or a float round trip of a wide number.
type TypeRoundTripCase struct {
	Name       string
	Database   model.NodePath
	Table      model.NodePath
	Setup      []string
	Teardown   []string
	Columns    []string
	Want       [][]*string
	EditID     string
	EditColumn string
	EditValue  string
	// Guard names columns whose loaded text rides the edit as op.Expect: the edit must pass on an
	// untouched row (no false conflict for the type) and a stale guard must fail with E_CONFLICT.
	Guard []string
	// Unguarded names Guard columns the adapter deliberately leaves out of the comparison (MySQL
	// single-precision FLOAT, whose `=` is not stable). A stale guard on one must not conflict,
	// which pins the exclusion instead of leaving it unproven.
	Unguarded []string
}

// TypeRoundTrip asserts a typed table reads back as exact text, survives an edit of one column with
// every other cell untouched, and that the edited cell reads as the new value.
func TypeRoundTrip(c TypeRoundTripCase) Scenario {
	return Scenario{
		Name:     "types: round trip " + c.Name,
		Requires: func(caps adapters.Caps) bool { return caps.SQL && caps.Writable },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			execAll(t, a, c.Database, c.Setup)
			t.Cleanup(func() { execAll(t, a, c.Database, c.Teardown) })

			before := readTypedTable(t, a, c, "initial read")
			if !reflect.DeepEqual(showRows(before), showRows(c.Want)) {
				t.Fatalf("initial read:\n got %q\nwant %q", showRows(before), showRows(c.Want))
			}

			val := c.EditValue
			editCol := columnIndex(c.Columns, c.EditColumn)
			expect := guardValues(c, before)
			if _, err := a.Mutate(context.Background(), typeRoundTripEdit(c, &val, expect), adapters.NewOpCtx("scenario-type-round-trip-edit")); err != nil {
				t.Fatalf("Mutate %s: %v", c.EditColumn, err)
			}
			after := readTypedTable(t, a, c, "read after edit")
			want := make([][]*string, len(c.Want))
			for r := range c.Want {
				want[r] = append([]*string(nil), c.Want[r]...)
				if want[r][0] != nil && *want[r][0] == c.EditID {
					want[r][editCol] = &val
				}
			}
			if !reflect.DeepEqual(showRows(after), showRows(want)) {
				t.Fatalf("after editing %s:\n got %q\nwant %q", c.EditColumn, showRows(after), showRows(want))
			}
			if len(expect) > 0 {
				// The first edit changed the column; a plan still guarding the old value is stale.
				stale := model.RowValues{{Name: c.EditColumn, Value: before[0][editCol]}}
				assertStaleConflict(t, a, c, &val, stale, c.EditColumn)
			}
			assertEveryGuard(t, a, c, &val, before)
		},
	}
}

// assertEveryGuard checks each guarded column other than the edited one.
func assertEveryGuard(t *testing.T, a adapters.Adapter, c TypeRoundTripCase, val *string, before [][]*string) {
	t.Helper()
	// Every guarded column must detect a change, not only the edited one: a guard that always
	// matches would pass the untouched-row check above. The row's own text is swapped for the
	// text another row holds, which is valid for the type by construction.
	for _, g := range c.Guard {
		gi := columnIndex(c.Columns, g)
		if g == c.EditColumn || gi < 0 {
			continue
		}
		other, ok := otherRowText(before, c.EditID, gi)
		if !ok {
			t.Fatalf("guard %s: no other row holds a different value to test the stale guard with", g)
		}
		staleG := model.RowValues{{Name: g, Value: other}}
		if slices.Contains(c.Unguarded, g) {
			if _, err := a.Mutate(context.Background(), typeRoundTripEdit(c, val, staleG), adapters.NewOpCtx("scenario-type-round-trip-unguarded-"+g)); err != nil {
				t.Fatalf("%s is left out of the guard, so a stale value must not conflict: %v", g, err)
			}
			continue
		}
		assertStaleConflict(t, a, c, val, staleG, g)
	}
}

// assertStaleConflict applies the edit under a guard the row no longer matches and wants E_CONFLICT.
func assertStaleConflict(t *testing.T, a adapters.Adapter, c TypeRoundTripCase, val *string, stale model.RowValues, column string) {
	t.Helper()
	_, err := a.Mutate(context.Background(), typeRoundTripEdit(c, val, stale), adapters.NewOpCtx("scenario-type-round-trip-stale-"+column))
	if code, _ := adapters.CodeOf(err); code != adapters.CodeConflict {
		t.Fatalf("stale guard on %s: err = %v, want E_CONFLICT", column, err)
	}
}

// otherRowText is the text of column col in a row other than editID that differs from the edited
// row's own (NULL counts as a value).
func otherRowText(rows [][]*string, editID string, col int) (*string, bool) {
	var own *string
	for _, row := range rows {
		if row[0] != nil && *row[0] == editID {
			own = row[col]
		}
	}
	for _, row := range rows {
		if row[0] != nil && *row[0] == editID {
			continue
		}
		if (row[col] == nil) != (own == nil) || (row[col] != nil && own != nil && *row[col] != *own) {
			return row[col], true
		}
	}
	return nil, false
}

func typeRoundTripEdit(c TypeRoundTripCase, val *string, expect model.RowValues) model.MutationPlan {
	return model.MutationPlan{Path: c.Table, Ops: []model.MutationRowOp{{
		Kind:    "update",
		Key:     model.RowValues{{Name: "id", Value: &c.EditID}},
		Changes: model.RowValues{{Name: c.EditColumn, Value: val}},
		Expect:  expect,
	}}}
}

func columnIndex(columns []string, name string) int {
	idx := -1
	for i, n := range columns {
		if n == name {
			idx = i
		}
	}
	return idx
}

// guardValues is the loaded text of every Guard column on the edited row.
func guardValues(c TypeRoundTripCase, rows [][]*string) model.RowValues {
	var expect model.RowValues
	for _, g := range c.Guard {
		i := columnIndex(c.Columns, g)
		if i < 0 {
			continue
		}
		for _, row := range rows {
			if row[0] != nil && *row[0] == c.EditID {
				expect = append(expect, model.RowValue{Name: g, Value: row[i]})
			}
		}
	}
	return expect
}

func readTypedTable(t *testing.T, a adapters.Adapter, c TypeRoundTripCase, label string) [][]*string {
	t.Helper()
	p, err := a.Read(context.Background(), adapters.ReadRequest{
		Path: c.Table, PageSize: bigPageSize, Cursor: model.PageCursor{Mode: "offset"},
		Sort: &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}},
	}, adapters.NewOpCtx("scenario-type-round-trip"))
	if err != nil {
		t.Fatalf("%s: Read: %v", label, err)
	}
	tp, ok := p.(page.TabularPage)
	if !ok {
		t.Fatalf("%s: page kind = %T, want a TabularPage", label, p)
	}
	names := make([]string, 0, len(tp.Columns))
	for _, col := range tp.Columns {
		names = append(names, col.Name)
	}
	if !reflect.DeepEqual(names, c.Columns) {
		t.Fatalf("%s: columns = %v, want %v", label, names, c.Columns)
	}
	out := make([][]*string, tp.RowCount)
	for r := range out {
		out[r] = make([]*string, len(tp.Chunks))
		for i, ch := range tp.Chunks {
			if !page.IsNull(ch, r) {
				s := page.CellText(ch, r)
				out[r][i] = &s
			}
		}
	}
	return out
}

func showRows(rows [][]*string) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		for _, v := range r {
			if v == nil {
				out[i] = append(out[i], "<NULL>")
			} else {
				out[i] = append(out[i], *v)
			}
		}
	}
	return out
}
