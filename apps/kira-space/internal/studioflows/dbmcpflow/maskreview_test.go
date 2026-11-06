package dbmcpflow

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P534 column review. UI half: tests/ui/connection-mask-review.spec.ts, contract dbmcp-mask-review.

func (f *fixture) maskingPrompt(t *testing.T) bool {
	t.Helper()
	all, err := f.app.W.PromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.Kind == prompts.KindMasking && p.Ref == f.connID {
			return true
		}
	}
	return false
}

func (f *fixture) execSQL(t *testing.T, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *fixture) setReviewed(t *testing.T, cols ...string) {
	t.Helper()
	if err := f.app.W.ConnectionFiles.Patch(f.connID, configfs.NodeSet{Path: []string{"mask_review"}, Value: map[string]any{"columns": cols}}); err != nil {
		t.Fatalf("Patch mask_review: %v", err)
	}
	f.app.W.Deps.MaskReview.Wait()
}

// refreshColumns is the tree Refresh: a background check that bypasses the metadata cache.
func (f *fixture) refreshColumns(t *testing.T) {
	t.Helper()
	if err := f.app.W.Tree.Invalidate(bridge.TreeInvalidateArgs{ConnectionID: f.connID}); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	f.app.W.Deps.MaskReview.Wait()
}

func (f *fixture) resultColumns(t *testing.T, r *mcp.CallToolResult) (cols, withheld []string) {
	t.Helper()
	var out struct {
		Columns         []struct{ Name string } `json:"columns"`
		WithheldColumns []string                `json:"withheldColumns"`
	}
	if err := json.Unmarshal([]byte(text(r)), &out); err != nil {
		t.Fatalf("result %q: %v", text(r), err)
	}
	for _, c := range out.Columns {
		cols = append(cols, c.Name)
	}
	return cols, out.WithheldColumns
}

func (f *fixture) columnReview(t *testing.T, sess *mcp.ClientSession) (on bool, withheld []string) {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_connections"})
	if err != nil {
		t.Fatal(err)
	}
	var listed []struct {
		ID           string `json:"id"`
		ColumnReview *struct {
			On       bool     `json:"on"`
			Withheld []string `json:"withheld"`
		} `json:"columnReview"`
	}
	if err := json.Unmarshal([]byte(text(res)), &listed); err != nil {
		t.Fatalf("list_connections %q: %v", text(res), err)
	}
	for _, c := range listed {
		if c.ID == f.connID && c.ColumnReview != nil {
			return c.ColumnReview.On, c.ColumnReview.Withheld
		}
	}
	t.Fatalf("list_connections %q has no columnReview for the connection", text(res))
	return false, nil
}

func TestDbMcpColumnReview(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	const hush = "hush-value"

	// Off by default: every column reaches the agent, nobody is told anything.
	if on, _ := f.columnReview(t, sess); on {
		t.Fatal("columnReview.on = true without a mask_review key")
	}
	if cols, w := f.resultColumns(t, runQuery(t, sess, f.connID, "select * from notes")); len(cols) != 2 || len(w) != 0 {
		t.Fatalf("select * with review off = %v withheld %v, want both columns", cols, w)
	}
	if f.maskingPrompt(t) {
		t.Fatal("masking prompt open with review off")
	}

	// Every current column reviewed: nothing withheld.
	f.setReviewed(t, "notes.id", "notes.body")
	if on, w := f.columnReview(t, sess); !on || len(w) != 0 {
		t.Fatalf("columnReview = %v %v, want on with nothing withheld", on, w)
	}
	if cols, w := f.resultColumns(t, runQuery(t, sess, f.connID, "select * from notes")); len(cols) != 2 || len(w) != 0 {
		t.Fatalf("select * all reviewed = %v withheld %v", cols, w)
	}

	// A column appears out of band; Refresh finds it, withholds it and opens the prompt.
	f.execSQL(t, "alter table notes add column secret text", "update notes set secret = '"+hush+"'")
	f.refreshColumns(t)
	testx.WaitUntil(t, 5*time.Second, func() bool { return f.maskingPrompt(t) })
	if _, w := f.columnReview(t, sess); len(w) != 1 || w[0] != "notes.secret" {
		t.Fatalf("columnReview.withheld = %v, want [notes.secret]", w)
	}
	r := runQuery(t, sess, f.connID, "select * from notes")
	cols, w := f.resultColumns(t, r)
	if r.IsError || len(cols) != 2 || len(w) != 1 || w[0] != "secret" || strings.Contains(text(r), hush) {
		t.Fatalf("select * = %q, want secret dropped and named", text(r))
	}
	for _, stmt := range []string{
		"select secret as s from notes",
		"select upper(secret) from notes",
		"select id from notes where secret = 'x'",
		"select id from notes order by secret",
	} {
		r := runQuery(t, sess, f.connID, stmt)
		if !r.IsError || !strings.Contains(text(r), "secret") || strings.Contains(text(r), hush) {
			t.Fatalf("%q = %q (error %v), want a refusal naming secret and no value", stmt, text(r), r.IsError)
		}
	}
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("a refused statement queued %d approvals", n)
	}

	// A mask rule covers the column: not pending, prompt closed, value masked.
	f.maskColumn(t, "secret")
	f.app.W.Deps.MaskReview.Wait()
	testx.WaitUntil(t, 5*time.Second, func() bool { return !f.maskingPrompt(t) })
	r = runQuery(t, sess, f.connID, "select * from notes")
	if cols, w := f.resultColumns(t, r); len(cols) != 3 || len(w) != 0 || strings.Contains(text(r), hush) {
		t.Fatalf("select * with a rule = %q, want secret returned masked", text(r))
	}

	// Without the rule it is pending again; confirming all reviews it.
	rules, err := f.app.W.MaskRules.List(bridge.MaskRulesListArgs{ConnectionID: f.connID})
	if err != nil || len(rules) != 1 {
		t.Fatalf("rules = %v (%v)", rules, err)
	}
	if err := f.app.W.MaskRules.Delete(bridge.MaskRulesDeleteArgs{ID: rules[0].ID}); err != nil {
		t.Fatal(err)
	}
	f.app.W.Deps.MaskReview.Wait()
	testx.WaitUntil(t, 5*time.Second, func() bool { return f.maskingPrompt(t) })
	f.setReviewed(t, "notes.id", "notes.body", "notes.secret")
	testx.WaitUntil(t, 5*time.Second, func() bool { return !f.maskingPrompt(t) })
	if cols, w := f.resultColumns(t, runQuery(t, sess, f.connID, "select * from notes")); len(cols) != 3 || len(w) != 0 {
		t.Fatalf("select * after confirm = %v withheld %v", cols, w)
	}

	// The Privacy tab lists the live columns.
	listing, err := f.app.W.MaskRules.ReviewColumns(bridge.MaskReviewColumnsArgs{ID: f.connID})
	if err != nil || !listing.Applicable || len(listing.Columns) != 3 {
		t.Fatalf("ReviewColumns = %+v (%v), want 3 columns", listing, err)
	}
	f.app.Contract(t, "dbmcp-mask-review", "MaskRulesService.ReviewColumns", listing, studioflowharness.Mask("checkedAt"))
	pending, err := f.app.W.MaskRules.ReviewPending(bridge.MaskReviewPendingArgs{ID: f.connID})
	if err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, "dbmcp-mask-review", "MaskRulesService.ReviewPending", pending)
}

// A column added outside MCP is withheld on the first result that carries it, without waiting for a tree
// Refresh or the 30 s freshness to lapse (real maskreview.Service, sqlite).
func TestDbMcpColumnReviewNewColumnWithinFreshness(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	const leak = "leak-value"
	f.setReviewed(t, "notes.id", "notes.body")
	if cols, w := f.resultColumns(t, runQuery(t, sess, f.connID, "select * from notes")); len(cols) != 2 || len(w) != 0 {
		t.Fatalf("select * all reviewed = %v withheld %v", cols, w)
	}

	f.execSQL(t, "alter table notes add column secret text", "update notes set secret = '"+leak+"'")
	r := runQuery(t, sess, f.connID, "select * from notes")
	if cols, w := f.resultColumns(t, r); r.IsError || len(cols) != 2 || len(w) != 1 || w[0] != "secret" || strings.Contains(text(r), leak) {
		t.Fatalf("select * after an out-of-band column = %q, want secret withheld", text(r))
	}

	// A new table whose column shares the name of a reviewed one on another table.
	f.execSQL(t, "create table other (id integer)", "alter table other add column body text", "insert into other (id, body) values (1, '"+leak+"')")
	r = runQuery(t, sess, f.connID, "select body from other")
	if _, w := f.resultColumns(t, r); r.IsError || len(w) != 1 || w[0] != "body" || strings.Contains(text(r), leak) {
		t.Fatalf("select body from other = %q, want the unreviewed body withheld", text(r))
	}
}

func TestDbMcpColumnReviewOutsideTableList(t *testing.T) {
	f := newFixture(t)
	f.setReviewed(t, "notes.id", "notes.body")
	if err := f.app.W.ConnectionFiles.Patch(f.connID, configfs.NodeSet{Path: []string{"mcp", "tables"}, Value: []string{"notes"}}); err != nil {
		t.Fatal(err)
	}
	f.execSQL(t, "create table other (id integer primary key, token text)")
	f.refreshColumns(t)
	if f.maskingPrompt(t) {
		t.Fatal("a column of an unlisted table opened the masking prompt")
	}
	if n := f.app.W.Deps.MaskReview.PendingCount(f.connID); n != 0 {
		t.Fatalf("pending = %d, want 0 for an unlisted table", n)
	}
}

func TestDbMcpColumnReviewValidation(t *testing.T) {
	f := newFixture(t)
	for _, v := range []any{map[string]any{"columns": []string{"a.b.c.d.e"}}, map[string]any{"columns": []string{""}}} {
		if err := f.app.W.ConnectionFiles.Patch(f.connID, configfs.NodeSet{Path: []string{"mask_review"}, Value: v}); err == nil {
			t.Fatalf("Patch mask_review %v = nil, want a validation error", v)
		}
	}
}
