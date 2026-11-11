package testsupport

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ConditionsCase drives ReadConditions. The scratch table holds ids 1..4: Alice, O'Brien (note NULL),
// Bob, alina (note NULL); SQL for it is dialect text the caller supplies. Binary is set when the
// engine types a column as binary and bin of id 2 is 0x0304.
type ConditionsCase struct {
	Name     string
	Database model.NodePath
	Table    model.NodePath
	Setup    []string
	Teardown []string
	Binary   bool
}

type conditionCheck struct {
	name  string
	conds []model.Condition
	sort  []model.SortTerm
	want  []string
}

func cond(column, op string, values ...any) model.Condition {
	return model.Condition{Column: column, Op: op, Values: values}
}

// ReadConditions asserts structured conditions bind their values as data on a real engine: every
// comparison op, in, like, null tests, an apostrophe and an injection-shaped value, a binary value, and
// an order term. Each check also runs Count with the same conditions and expects the row total (P609).
// It also asserts a read and a count with a statement timeout set succeed; the timeout trip itself and
// its reset belong to the StatementTimeout scenario, which runs the same session helper.
func ReadConditions(c ConditionsCase) Scenario {
	checks := []conditionCheck{
		{"eq", []model.Condition{cond("name", model.CondEq, "Alice")}, nil, []string{"1"}},
		{"apostrophe is data", []model.Condition{cond("name", model.CondEq, "O'Brien")}, nil, []string{"2"}},
		{"injection-shaped value matches nothing", []model.Condition{cond("name", model.CondEq, "x' OR '1'='1")}, nil, nil},
		{"ne", []model.Condition{cond("name", model.CondNe, "Alice")}, nil, []string{"2", "3", "4"}},
		{"number as exact text", []model.Condition{cond("id", model.CondEq, json.Number("3"))}, nil, []string{"3"}},
		{"gt", []model.Condition{cond("id", model.CondGt, json.Number("2"))}, nil, []string{"3", "4"}},
		{"lte", []model.Condition{cond("id", model.CondLte, json.Number("2"))}, nil, []string{"1", "2"}},
		{"in", []model.Condition{cond("id", model.CondIn, json.Number("1"), json.Number("3"))}, nil, []string{"1", "3"}},
		{"like", []model.Condition{cond("name", model.CondLike, "Bo%")}, nil, []string{"3"}},
		{"like with apostrophe", []model.Condition{cond("name", model.CondLike, "%'B%")}, nil, []string{"2"}},
		{"isNull", []model.Condition{cond("note", model.CondIsNull)}, nil, []string{"2", "4"}},
		{"notNull", []model.Condition{cond("note", model.CondNotNull)}, nil, []string{"1", "3"}},
		{"conditions are ANDed", []model.Condition{cond("name", model.CondNe, "Alice"), cond("note", model.CondIsNull)}, nil, []string{"2", "4"}},
		{"order desc", nil, []model.SortTerm{{Column: "id", Direction: "desc"}}, []string{"4", "3", "2", "1"}},
	}
	if c.Binary {
		checks = append(checks, conditionCheck{"binary value", []model.Condition{cond("bin", model.CondEq, "0x0304")}, nil, []string{"2"}})
	}
	return Scenario{
		Name:     "read: structured conditions " + c.Name,
		Requires: func(caps adapters.Caps) bool { return caps.SQL && caps.ServerFilter },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			execAll(t, a, c.Database, c.Setup)
			t.Cleanup(func() { execAll(t, a, c.Database, c.Teardown) })
			for _, k := range checks {
				req := adapters.ReadRequest{
					Path: c.Table, PageSize: bigPageSize, Cursor: model.PageCursor{Mode: "offset"}, Conditions: k.conds,
					Projection: []string{"id"},
				}
				// ids are the tiebreak order for every unsorted check
				req.Sort = &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "id", Direction: "asc"}}}
				if k.sort != nil {
					req.Sort.Terms = k.sort
				}
				got := readIDs(t, a, req, k.name)
				if strings.Join(got, ",") != strings.Join(k.want, ",") {
					t.Errorf("%s: ids = %v, want %v", k.name, got, k.want)
				}
				countRows(t, a, adapters.CountRequest{Path: c.Table, Conditions: k.conds}, int64(len(k.want)), k.name)
			}
			req := adapters.ReadRequest{
				Path: c.Table, PageSize: bigPageSize, Cursor: model.PageCursor{Mode: "offset"},
				Conditions: []model.Condition{cond("id", model.CondEq, json.Number("1"))}, StatementTimeout: 30 * time.Second,
			}
			if got := readIDs(t, a, req, "with statement timeout"); fmt.Sprint(got) != "[1]" {
				t.Errorf("with statement timeout: ids = %v, want [1]", got)
			}
			countRows(t, a, adapters.CountRequest{Path: c.Table, Conditions: req.Conditions, StatementTimeout: 30 * time.Second}, 1, "count with statement timeout")
		},
	}
}

func countRows(t *testing.T, a adapters.Adapter, req adapters.CountRequest, want int64, name string) {
	t.Helper()
	res, err := a.Count(context.Background(), req, adapters.NewOpCtx("scenario-count-conditions"))
	if err != nil {
		t.Fatalf("%s: Count: %v", name, err)
	}
	if res.Value != want || !res.Exact {
		t.Errorf("%s: count = %d exact=%v, want %d exact", name, res.Value, res.Exact, want)
	}
}

func readIDs(t *testing.T, a adapters.Adapter, req adapters.ReadRequest, name string) []string {
	t.Helper()
	p, err := a.Read(context.Background(), req, adapters.NewOpCtx("scenario-read-conditions"))
	if err != nil {
		t.Fatalf("%s: Read: %v", name, err)
	}
	tp, ok := p.(page.TabularPage)
	if !ok {
		t.Fatalf("%s: page kind = %T, want a TabularPage", name, p)
	}
	idx := -1
	for i, col := range tp.Columns {
		if col.Name == "id" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("%s: no id column in %+v", name, tp.Columns)
	}
	out := make([]string, 0, tp.RowCount)
	for r := 0; r < tp.RowCount; r++ {
		out = append(out, page.CellText(tp.Chunks[idx], r))
	}
	return out
}
