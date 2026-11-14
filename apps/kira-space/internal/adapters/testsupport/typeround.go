package testsupport

import (
	"context"
	"reflect"
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

			read := func(label string) [][]*string {
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
				var names []string
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
			show := func(rows [][]*string) [][]string {
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
			before := read("initial read")
			if !reflect.DeepEqual(show(before), show(c.Want)) {
				t.Fatalf("initial read:\n got %q\nwant %q", show(before), show(c.Want))
			}

			val := c.EditValue
			var expect model.RowValues
			for _, g := range c.Guard {
				for i, n := range c.Columns {
					if n != g {
						continue
					}
					for _, row := range before {
						if row[0] != nil && *row[0] == c.EditID {
							expect = append(expect, model.RowValue{Name: g, Value: row[i]})
						}
					}
				}
			}
			if _, err := a.Mutate(context.Background(), model.MutationPlan{Path: c.Table, Ops: []model.MutationRowOp{{
				Kind:    "update",
				Key:     model.RowValues{{Name: "id", Value: &c.EditID}},
				Changes: model.RowValues{{Name: c.EditColumn, Value: &val}},
				Expect:  expect,
			}}}, adapters.NewOpCtx("scenario-type-round-trip-edit")); err != nil {
				t.Fatalf("Mutate %s: %v", c.EditColumn, err)
			}
			after := read("read after edit")
			editCol := -1
			for i, n := range c.Columns {
				if n == c.EditColumn {
					editCol = i
				}
			}
			want := make([][]*string, len(c.Want))
			for r := range c.Want {
				want[r] = append([]*string(nil), c.Want[r]...)
			}
			for r := range want {
				if want[r][0] != nil && *want[r][0] == c.EditID {
					want[r][editCol] = &val
				}
			}
			if !reflect.DeepEqual(show(after), show(want)) {
				t.Fatalf("after editing %s:\n got %q\nwant %q", c.EditColumn, show(after), show(want))
			}
			if len(expect) > 0 {
				// The first edit changed `note`; a plan still guarding the old value is stale.
				stale := model.RowValues{{Name: c.EditColumn, Value: before[0][editCol]}}
				_, err := a.Mutate(context.Background(), model.MutationPlan{Path: c.Table, Ops: []model.MutationRowOp{{
					Kind:    "update",
					Key:     model.RowValues{{Name: "id", Value: &c.EditID}},
					Changes: model.RowValues{{Name: c.EditColumn, Value: &val}},
					Expect:  stale,
				}}}, adapters.NewOpCtx("scenario-type-round-trip-stale"))
				if code, _ := adapters.CodeOf(err); code != adapters.CodeConflict {
					t.Fatalf("stale guard: err = %v, want E_CONFLICT", err)
				}
			}
		},
	}
}
