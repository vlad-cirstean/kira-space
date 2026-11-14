package dbmcpflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mask"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// J-MASK-001 / J-BC-007 (contract studio-mask). One rule on `name`, one table. The agent reads the
// table through browse_table and run_query and never sees a real name, through an alias, a quoted
// alias or an expression. The same values masked here are what the grid must copy: the UI half
// (tests/ui/studio/mask-copy.spec.ts) replays the page and the rules and compares the clipboard
// text, with the preview on and again from a stale (disconnected) grid. The rule sits in the
// connection YAML and survives a restart.
func TestMaskRuleHoldsAcrossGridCopyAndAgentRead(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.execSQL(t,
		"create table composite_pk (tenant_id integer not null, entity_id integer not null, name text, primary key (tenant_id, entity_id))",
		"insert into composite_pk values (1, 1, 'Ada Lovelace'), (1, 2, 'Pat O''Brien'), (1, 3, null), (2, 1, 'Ada Lovelace')",
	)
	rule, err := f.app.W.MaskRules.Upsert(bridge.MaskRulesUpsertArgs{ConnectionID: f.connID, Fields: model.MaskRuleFields{
		TableName: "*", ColumnName: "name", Kind: model.MaskKindName, KeepHint: true,
	}})
	if err != nil {
		t.Fatal(err)
	}

	rules, err := f.app.W.MaskRules.List(bridge.MaskRulesListArgs{ConnectionID: f.connID})
	if err != nil || len(rules) != 1 {
		t.Fatalf("MaskRules.List = %+v (%v), want the one rule", rules, err)
	}
	f.app.Contract(t, "studio-mask", "rules", rules, flowharness.Mask("id", "connectionId", "createdAt", "updatedAt"))

	// The rule is in the connection YAML, so a second window and a restart both see it.
	yamls, _ := filepath.Glob(filepath.Join(f.app.ConfigDir("connections"), "*"))
	var inYAML bool
	for _, p := range yamls {
		if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), "name") && strings.Contains(string(b), string(model.MaskKindName)) {
			inYAML = true
		}
	}
	if !inYAML {
		t.Fatalf("no connection file under %v holds the mask rule", yamls)
	}

	// The grid page the UI half replays.
	if st, err := f.app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: f.connID}); err != nil || st.Status != "connected" {
		t.Fatalf("Connect = %+v (%v)", st, err)
	}
	ds := f.app.DataStream(t)
	page, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: f.connID, Path: tablePath("main", "composite_pk")})
	if err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, "studio-mask", "rows", page.Rows)

	// What the grid copies with the preview on: tab-separated rows, names masked, NULL empty.
	masker := mask.New(nil)
	mr := mask.Rule{Kind: mask.Kind(rule.Kind), KeepHint: rule.KeepHint, Correlate: rule.Correlate}
	var tsv []string
	var names []string
	for _, row := range page.Rows {
		cell := ""
		if s, ok := row[2].(string); ok {
			cell = masker.Apply(mr, s)
			names = append(names, cell)
		}
		tsv = append(tsv, row[0].(string)+"\t"+row[1].(string)+"\t"+cell)
	}
	for _, n := range names {
		if strings.Contains(n, "Lovelace") || strings.Contains(n, "Brien") {
			t.Fatalf("masked name %q still holds a real name", n)
		}
	}
	if names[0] != names[len(names)-1] {
		t.Fatalf("same name masked to %q and %q, want one output", names[0], names[len(names)-1])
	}
	f.app.Contract(t, "studio-mask", "copy:tsv", strings.Join(tsv, "\n"))

	// The agent gets the same masked values through browse_table.
	res := browse(t, sess, f.connID, map[string]any{"path": tablePath("main", "composite_pk")})
	var out browseOut
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatalf("browse_table %q: %v", text(res), err)
	}
	if got := out.names("name"); !reflect.DeepEqual(got, names) {
		t.Fatalf("browse_table names = %v, want the grid's masked %v", got, names)
	}
	f.app.Contract(t, "studio-mask", "mcp:browse", out.Rows)

	// No spelling of the column leaks a real value: an alias, a quoted alias, an expression.
	for _, stmt := range []string{
		`select name as n from composite_pk`,
		`select "name", name as "n2" from composite_pk`,
		`select lower(name), upper(name), substr(name, 1, 3) from composite_pk`,
		`select * from composite_pk`,
	} {
		r := runQuery(t, sess, f.connID, stmt)
		if body := strings.ToLower(text(r)); strings.Contains(body, "lovelace") || strings.Contains(body, "brien") || strings.Contains(body, "ada ") {
			t.Fatalf("%q leaked a real name: %s", stmt, text(r))
		}
	}

	// The rule survives a restart.
	f.app.Restart()
	again, err := f.app.W.MaskRules.List(bridge.MaskRulesListArgs{ConnectionID: f.connID})
	if err != nil || len(again) != 1 || again[0].ColumnName != "name" {
		t.Fatalf("rules after restart = %+v (%v), want the one rule", again, err)
	}
}
