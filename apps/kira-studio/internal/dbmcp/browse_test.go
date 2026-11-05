package dbmcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/mask"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

func browseMeta(kind string, cols ...string) model.ObjectMeta {
	m := model.ObjectMeta{Kind: kind}
	for _, c := range cols {
		m.Columns = append(m.Columns, model.ColumnMeta{Name: c})
	}
	return m
}

func scalar(v any) *browseScalar { return &browseScalar{v: v} }

// planBrowse decides what masked and withheld columns may do: projection, filter and order.
func TestPlanBrowseMaskingRules(t *testing.T) {
	summary := model.ConnectionSummary{Kind: "postgres"}
	sh := browseShape{limit: 50, relation: `"app"."customers"`}
	meta := browseMeta("table", "id", "email", "note", "ssn")
	masked := maskset{Set: mask.Set{Rules: map[string]mask.Rule{"email": {Kind: mask.KindRedact}}}}
	withheld := maskset{withheld: map[string]bool{"ssn": true}}
	both := maskset{Set: masked.Set, withheld: withheld.withheld}

	cases := []struct {
		name    string
		meta    model.ObjectMeta
		args    browseTableArgs
		mk      maskset
		refusal string
		check   func(t *testing.T, p browsePlan)
	}{
		{name: "plain", meta: meta, mk: maskset{}, check: func(t *testing.T, p browsePlan) {
			if len(p.projection) != 4 {
				t.Fatalf("projection %v", p.projection)
			}
		}},
		{name: "withheld column left out and named", meta: meta, mk: withheld, check: func(t *testing.T, p browsePlan) {
			if len(p.projection) != 3 || len(p.withheld) != 1 || p.withheld[0] != "ssn" {
				t.Fatalf("projection %v withheld %v", p.projection, p.withheld)
			}
		}},
		{name: "every requested column withheld", meta: meta, args: browseTableArgs{Columns: []string{"ssn"}}, mk: withheld, refusal: "every requested column"},
		{name: "unknown column lists the table's", meta: meta, args: browseTableArgs{Columns: []string{"nope"}}, refusal: "Columns: id, email, note, ssn"},
		{name: "case differs from catalog", meta: meta, args: browseTableArgs{Columns: []string{"ID"}}, refusal: "not on"},
		{name: "duplicate column", meta: meta, args: browseTableArgs{Columns: []string{"id", "id"}}, refusal: "twice"},
		{name: "masked eq refused", meta: meta, args: browseTableArgs{Filters: []browseFilter{{Column: "email", Op: "eq", Value: scalar("a")}}}, mk: masked, refusal: "is masked"},
		{name: "masked like refused", meta: meta, args: browseTableArgs{Filters: []browseFilter{{Column: "email", Op: "like", Value: scalar("a%")}}}, mk: masked, refusal: "is masked"},
		{name: "masked in refused", meta: meta, args: browseTableArgs{Filters: []browseFilter{{Column: "email", Op: "in", Values: []browseScalar{*scalar("a")}}}}, mk: masked, refusal: "is masked"},
		{name: "masked isNull allowed", meta: meta, args: browseTableArgs{Filters: []browseFilter{{Column: "email", Op: "isNull"}}}, mk: masked, check: func(t *testing.T, p browsePlan) {
			if len(p.conditions) != 1 || p.conditions[0].Op != "isNull" {
				t.Fatalf("conditions %v", p.conditions)
			}
		}},
		{name: "masked order refused", meta: meta, args: browseTableArgs{Order: []browseOrder{{Column: "email", Direction: "asc"}}}, mk: masked, refusal: "cannot be sorted on"},
		{name: "withheld filter refused", meta: meta, args: browseTableArgs{Filters: []browseFilter{{Column: "ssn", Op: "isNull"}}}, mk: withheld, refusal: "not reviewed"},
		{name: "withheld order refused", meta: meta, args: browseTableArgs{Order: []browseOrder{{Column: "ssn", Direction: "asc"}}}, mk: withheld, refusal: "not reviewed"},
		{name: "unmasked column filters and sorts", meta: meta, args: browseTableArgs{
			Filters: []browseFilter{{Column: "note", Op: "eq", Value: scalar("x")}},
			Order:   []browseOrder{{Column: "id", Direction: "desc"}},
		}, mk: both, check: func(t *testing.T, p browsePlan) {
			if p.sort == nil || len(p.conditions) != 1 {
				t.Fatalf("plan %+v", p)
			}
		}},
		{name: "view refused under masking", meta: browseMeta("view", "id"), mk: masked, refusal: "base tables only"},
		{name: "view allowed without masking", meta: browseMeta("view", "id"), mk: maskset{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, refusal := planBrowse(summary, sh, tc.meta, tc.args, tc.mk)
			if tc.refusal != "" {
				if !strings.Contains(refusal, tc.refusal) {
					t.Fatalf("refusal %q, want %q", refusal, tc.refusal)
				}
				return
			}
			if refusal != "" {
				t.Fatalf("unexpected refusal %q", refusal)
			}
			if tc.check != nil {
				tc.check(t, p)
			}
		})
	}
}

// A string value cannot end its quotes early under any engine's escape rule.
func TestBrowsePreviewLiterals(t *testing.T) {
	var num json.Number = "12"
	cond := func(v any) browsePlan {
		return browsePlan{relation: "t", projection: []string{"a"}, limit: 5, conditions: []model.Condition{{Column: "a", Op: "eq", Values: []any{v}}}}
	}
	cases := []struct {
		kind string
		v    any
		want string
	}{
		{"postgres", `a\'; DROP TABLE t; --`, `WHERE "a" = 'a\\''; DROP TABLE t; --' LIMIT`},
		{"mysql", `x\`, "WHERE `a` = 'x\\\\' LIMIT"},
		{"sqlite", `it's`, `WHERE "a" = 'it''s' LIMIT`},
		{"clickhouse", true, "WHERE `a` = TRUE LIMIT"},
		{"sqlite", true, `WHERE "a" = 1 LIMIT`},
		{"postgres", num, `WHERE "a" = 12 LIMIT`},
	}
	for _, tc := range cases {
		if got := cond(tc.v).preview(tc.kind); !strings.Contains(got, tc.want) {
			t.Errorf("%s: %s, want to contain %s", tc.kind, got, tc.want)
		}
	}
}
