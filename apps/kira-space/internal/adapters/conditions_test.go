package adapters

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestBuildConditionsSQL(t *testing.T) {
	columns := map[string]model.ColumnMeta{
		"id": {Name: "id", DataType: "integer"}, "name": {Name: "name", DataType: "text"},
		"bin": {Name: "bin", DataType: "bytea"},
	}
	quote := QuoteIdentDouble
	isBinary := func(n string) bool { return n == "bin" }
	dollar := func(i int, _ model.ColumnMeta) string { return "$" + strconv.Itoa(i) }
	question := func(int, model.ColumnMeta) string { return "?" }
	typed := func(i int, c model.ColumnMeta) string { return "{p" + strconv.Itoa(i) + ":" + c.DataType + "}" }

	cases := []struct {
		name        string
		conds       []model.Condition
		placeholder func(int, model.ColumnMeta) string
		wantSQL     string
		wantParams  []any
		wantErr     string
	}{
		{"numbered placeholders follow value order", []model.Condition{
			{Column: "name", Op: model.CondEq, Values: []any{"a"}},
			{Column: "id", Op: model.CondIn, Values: []any{json.Number("1"), json.Number("2")}},
			{Column: "name", Op: model.CondLike, Values: []any{"b%"}},
		}, dollar, `("name" = $1) AND ("id" IN ($2, $3)) AND ("name" LIKE $4)`, []any{"a", "1", "2", "b%"}, ""},
		{"null ops bind nothing", []model.Condition{
			{Column: "name", Op: model.CondIsNull}, {Column: "id", Op: model.CondNotNull},
		}, question, `("name" IS NULL) AND ("id" IS NOT NULL)`, nil, ""},
		{"binary value decodes and a bool stays a bool", []model.Condition{
			{Column: "bin", Op: model.CondEq, Values: []any{"0x0a0B"}},
			{Column: "id", Op: model.CondNe, Values: []any{true}},
		}, question, `("bin" = ?) AND ("id" <> ?)`, []any{[]byte{0x0a, 0x0b}, true}, ""},
		{"typed placeholder sees the column", []model.Condition{
			{Column: "id", Op: model.CondGte, Values: []any{json.Number("7")}},
		}, typed, `("id" >= {p1:integer})`, []any{"7"}, ""},
		{"unknown column", []model.Condition{{Column: "nope", Op: model.CondEq, Values: []any{"x"}}}, question, "", nil, "unknown column"},
		{"value is never spelled into the text", []model.Condition{
			{Column: "name", Op: model.CondEq, Values: []any{"x'; DROP TABLE t; --"}},
		}, question, `("name" = ?)`, []any{"x'; DROP TABLE t; --"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addParam, params := NewParamAccumulator()
			got, err := BuildConditionsSQL(tc.conds, columns, quote, tc.placeholder, addParam, isBinary)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.wantSQL {
				t.Errorf("sql = %s, want %s", got, tc.wantSQL)
			}
			if len(*params) != len(tc.wantParams) {
				t.Fatalf("params = %v, want %v", *params, tc.wantParams)
			}
			for i, w := range tc.wantParams {
				if !equalParam((*params)[i], w) {
					t.Errorf("param %d = %v, want %v", i, (*params)[i], w)
				}
			}
		})
	}

	t.Run("conditions AND onto a free filter", func(t *testing.T) {
		filter := "id > 1"
		got := AppendWhere(WhereClause(&filter), `("name" = ?)`)
		if want := "WHERE (id > 1\n) AND (\"name\" = ?)"; got != want {
			t.Errorf("where = %q, want %q", got, want)
		}
	})
}

func equalParam(a, b any) bool {
	ab, ok := a.([]byte)
	if !ok {
		return a == b
	}
	bb, _ := b.([]byte)
	return bytes.Equal(ab, bb)
}
