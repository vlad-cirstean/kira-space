package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// maskTarget names the masked table and column per engine: the seeded customers table, or the fixture's
// notes table on SQLite.
func maskTarget(e engine) (table, column, reviewPrefix string, columns []string) {
	switch e.kind {
	case "sqlite":
		return "notes", "body", "notes.", []string{"id", "body"}
	case "postgres":
		return "customers", "name", "kira_test.app.customers.", []string{"id", "name", "region_id"}
	}
	return "customers", "name", "kira_test.customers.", []string{"id", "name", "region_id"}
}

func firstCell(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	var out struct {
		Rows [][]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(text(r)), &out); err != nil || len(out.Rows) == 0 || len(out.Rows[0]) == 0 {
		t.Fatalf("result %q: want a first cell (%v)", text(r), err)
	}
	s, _ := out.Rows[0][len(out.Rows[0])-1].(string)
	return s
}

// Masking, plan redaction, withheld errors and column review against every SQL engine. Mongo and Redis
// have no table columns to mask and stay on the SQLite-free adapter and render tests.
func TestDbMcpEngineMasking(t *testing.T) {
	engineTest(t, []string{"postgres", "mysql", "mariadb", "clickhouse", "sqlite"}, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		f.engineConn(t, e, nil)
		table, column, prefix, all := maskTarget(e)
		qualified := e.customers
		sel := "select " + column + " from " + qualified + " order by id limit 1"

		baseline := firstCell(t, e.run(t, sess, f.connID, sel, "read"))
		if baseline == "" || baseline == "[redacted]" {
			t.Fatalf("baseline %s = %q, want real data", e.kind, baseline)
		}
		if _, err := f.app.W.MaskRules.Upsert(bridge.MaskRulesUpsertArgs{ConnectionID: f.connID, Fields: model.MaskRuleFields{TableName: table, ColumnName: column, Kind: "redact"}}); err != nil {
			t.Fatal(err)
		}

		// run_query and browse_table both mask.
		r := e.run(t, sess, f.connID, sel, "read")
		if r.IsError || strings.Contains(text(r), baseline) || firstCell(t, r) != "[redacted]" {
			t.Fatalf("%s masked run_query = %q, want [redacted] and no %q", e.kind, text(r), baseline)
		}
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "browse_table", Arguments: map[string]any{"connectionId": f.connID, "path": e.customersPathOr()}})
		if err != nil {
			t.Fatal(err)
		}
		if out := decodeBrowse(t, res); out.Returned == 0 || strings.Contains(text(res), baseline) {
			t.Fatalf("%s masked browse = %q", e.kind, text(res))
		}

		// A failing statement keeps its engine message back.
		if r := e.run(t, sess, f.connID, e.castErr, "read"); !r.IsError || !strings.Contains(text(r), "query failed (message withheld") {
			t.Fatalf("%s castErr under mask = %q, want a withheld message", e.kind, text(r))
		}

		// The plan carries no statement text, no raw document and no row data.
		pr := call(t, sess, mergeArgs(e.args(f.connID, sel, "read"), map[string]any{"explain": true}))
		if pr.IsError {
			t.Fatalf("%s explain under mask = %q", e.kind, text(pr))
		}
		if strings.Contains(text(pr), baseline) {
			t.Fatalf("%s explain leaks %q: %q", e.kind, baseline, text(pr))
		}
		var plan map[string]any
		if err := json.Unmarshal([]byte(text(pr)), &plan); err != nil {
			t.Fatal(err)
		}
		if _, ok := plan["raw"]; ok {
			t.Fatalf("%s explain under mask kept raw: %q", e.kind, text(pr))
		}
		assertBlankDetail(t, e.kind, plan["root"])

		// Column review: only id reviewed, so every other column is withheld from select *.
		rules, err := f.app.W.MaskRules.List(bridge.MaskRulesListArgs{ConnectionID: f.connID})
		if err != nil || len(rules) != 1 {
			t.Fatalf("rules = %v (%v)", rules, err)
		}
		if err := f.app.W.MaskRules.Delete(bridge.MaskRulesDeleteArgs{ID: rules[0].ID}); err != nil {
			t.Fatal(err)
		}
		f.setReviewed(t, prefix+"id")
		f.refreshColumns(t)
		testx.WaitUntil(t, 10*time.Second, func() bool {
			_, w := f.columnReview(t, sess)
			return len(w) == len(all)-1
		})
		star := e.run(t, sess, f.connID, "select * from "+qualified, "read")
		cols, withheld := f.resultColumns(t, star)
		if star.IsError || len(cols) != 1 || len(withheld) != len(all)-1 || strings.Contains(text(star), baseline) {
			t.Fatalf("%s select * with review = %v withheld %v (%q), want id only", e.kind, cols, withheld, text(star))
		}
		reviewed := []string{prefix + "id", prefix + column}
		f.setReviewed(t, reviewed...)
		f.refreshColumns(t)
		star = e.run(t, sess, f.connID, "select * from "+qualified, "read")
		cols, _ = f.resultColumns(t, star)
		found := false
		for _, c := range cols {
			found = found || c == column
		}
		if !found {
			t.Fatalf("%s select * after reviewing %s = %v, want it returned", e.kind, column, cols)
		}
	})
}

func (e engine) customersPathOr() string {
	if e.customersPath != "" {
		return e.customersPath
	}
	return tablePath("main", "notes")
}

// assertBlankDetail walks a plan tree and fails on any node with statement text.
func assertBlankDetail(t *testing.T, kind string, node any) {
	t.Helper()
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	if d, _ := m["detail"].(string); d != "" {
		t.Fatalf("%s plan node detail = %q, want blank under masking", kind, d)
	}
	kids, _ := m["children"].([]any)
	for _, k := range kids {
		assertBlankDetail(t, kind, k)
	}
}
