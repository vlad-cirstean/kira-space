package dbmcp

import (
	"context"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

type recheckReview struct {
	fakeColumnReviewNone
	calls int
	got   maskreview.Rescan
	forFn maskreview.Withheld
	next  maskreview.Withheld
}

func (r *recheckReview) For(context.Context, model.ConnectionSummary) maskreview.Withheld {
	return r.forFn
}

func (r *recheckReview) Recheck(_ context.Context, _ model.ConnectionSummary, req maskreview.Rescan) maskreview.Withheld {
	r.calls++
	r.got = req
	return r.next
}

// tablerQuery adds StatementTables to a QueryRunner; only that method is called.
type tablerQuery struct {
	QueryRunner
	tr adapterhost.TablesResult
}

func (q tablerQuery) StatementTables(context.Context, string, string, string) (adapterhost.TablesResult, error) {
	return q.tr, nil
}

func cols(names ...string) []page.ColumnDescriptor {
	out := make([]page.ColumnDescriptor, len(names))
	for i, n := range names {
		out[i] = page.ColumnDescriptor{Name: n}
	}
	return out
}

func liveOf(table string, names ...string) []maskreview.LiveColumn {
	out := make([]maskreview.LiveColumn, len(names))
	for i, n := range names {
		out[i] = maskreview.LiveColumn{Ref: tableref.ColumnRef{Ref: tableref.Ref{Name: table}, Column: n}}
	}
	return out
}

func sqliteKnown(live ...[]maskreview.LiveColumn) *maskreview.Known {
	var all []maskreview.LiveColumn
	for _, l := range live {
		all = append(all, l...)
	}
	return maskreview.NewKnown("sqlite", all)
}

func TestSettleResult(t *testing.T) {
	known := sqliteKnown(liveOf("customers", "id", "email"), liveOf("users", "id"))
	orders := tableref.Ref{Name: "customers"}
	users := tableref.Ref{Name: "users"}
	cases := []struct {
		name      string
		known     *maskreview.Known
		tr        adapterhost.TablesResult
		columns   []page.ColumnDescriptor
		next      maskreview.Withheld
		wantCalls int
		wantDrop  []string
		refused   bool
	}{
		{name: "review off never refreshes", known: nil, columns: cols("anything")},
		{name: "columns of the statement's table skip the refresh", known: known,
			tr: adapterhost.TablesResult{Refs: []tableref.Ref{orders}}, columns: cols("ID", "Email")},
		{name: "a name known only on another table refreshes", known: known,
			tr: adapterhost.TablesResult{Refs: []tableref.Ref{users}}, columns: cols("id", "email"),
			next:      maskreview.Withheld{On: true, Names: map[string]bool{"email": true}, Known: known},
			wantCalls: 1, wantDrop: []string{"email"}},
		{name: "alias stays after the refresh", known: known,
			tr: adapterhost.TablesResult{Refs: []tableref.Ref{orders}}, columns: cols("id", "count(*)"),
			next: maskreview.Withheld{On: true, Names: map[string]bool{}, Known: known}, wantCalls: 1},
		{name: "no relation, nothing to refresh", known: known,
			tr: adapterhost.TablesResult{}, columns: cols("1")},
		{name: "unparsed statement falls back to every name", known: known,
			tr: adapterhost.TablesResult{Refusal: "parse error"}, columns: cols("id", "email")},
		{name: "unparsed statement refreshes an unseen name", known: known,
			tr: adapterhost.TablesResult{Refusal: "parse error"}, columns: cols("ssn"),
			next:      maskreview.Withheld{On: true, Names: map[string]bool{"ssn": true}, Known: known},
			wantCalls: 1, wantDrop: []string{"ssn"}},
		{name: "incomplete refresh refuses", known: known,
			tr: adapterhost.TablesResult{Refs: []tableref.Ref{orders}}, columns: cols("x"),
			next: maskreview.Withheld{On: true, Incomplete: "the column check failed"}, wantCalls: 1, refused: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rv := &recheckReview{next: c.next}
			s := &Server{cfg: Config{ColumnReview: rv, Query: tablerQuery{tr: c.tr}}}
			got, refusal := s.settleResult(context.Background(), queryResolution{class: adapters.ClassWrite}, runQueryArgs{}, maskset{known: c.known}, c.columns)
			if rv.calls != c.wantCalls {
				t.Fatalf("refreshes = %d, want %d", rv.calls, c.wantCalls)
			}
			if (refusal != nil) != c.refused {
				t.Fatalf("refusal = %v, want %v", refusal != nil, c.refused)
			}
			if c.refused {
				if text := resultText(refusal); !strings.Contains(text, "ran") || !strings.Contains(text, "Do not rerun it") {
					t.Fatalf("refusal after a write must say it ran and not to rerun: %q", text)
				}
				return
			}
			keep, withheld := keptColumns(c.columns, &got)
			if len(withheld) != len(c.wantDrop) || len(keep) != len(c.columns)-len(c.wantDrop) {
				t.Fatalf("kept %d withheld %v, want drop %v", len(keep), withheld, c.wantDrop)
			}
		})
	}
}

// A column that shows up while a result waits for review changes the withheld set: recheckHeld withholds the
// result and, for a write, tells the agent not to rerun.
func TestRecheckHeld(t *testing.T) {
	known := sqliteKnown(liveOf("customers", "id"))
	tr := adapterhost.TablesResult{Refs: []tableref.Ref{{Name: "customers"}}}
	summary := model.ConnectionSummary{ID: "c1", Name: "conn", McpEnabled: true}
	for _, class := range []adapters.OpClass{adapters.ClassRead, adapters.ClassWrite} {
		rv := &recheckReview{
			forFn: maskreview.Withheld{On: true, Names: map[string]bool{}, Known: known},
			next:  maskreview.Withheld{On: true, Names: map[string]bool{"ssn": true}, Known: known},
		}
		s := &Server{cfg: Config{
			Conns: &fakeConnsMutable{summary: summary}, MaskRules: fakeMaskRulesNone{}, ColumnReview: rv, Query: tablerQuery{tr: tr},
		}}
		rendered := maskset{known: known, withheld: map[string]bool{}, resultColumns: cols("id", "ssn"), stmt: &stmtTables{refs: tr.Refs, parsed: true}}
		msg := s.recheckHeld(context.Background(), queryResolution{summary: summary, class: class}, runQueryArgs{ConnectionID: "c1"}, rendered)
		if rv.calls != 1 || !strings.Contains(msg, "ran") || !strings.Contains(msg, "changed") {
			t.Fatalf("class %v: calls=%d msg=%q; want one refresh and a withheld result", class, rv.calls, msg)
		}
		if rerun := strings.Contains(msg, "Do not rerun it"); rerun != (class == adapters.ClassWrite) {
			t.Fatalf("class %v: do-not-rerun = %v in %q", class, rerun, msg)
		}
	}
}

// The allow list can shrink while a statement waits for approval: the approved statement is refused.
func TestRecheckAfterApprovalRefusesUnlistedTable(t *testing.T) {
	summary := model.ConnectionSummary{ID: "c1", Name: "conn"}
	summary.Kind, summary.McpEnabled, summary.McpWriteMode, summary.McpTables = "sqlite", true, "prompt", []string{"other"}
	s := &Server{cfg: Config{
		Conns: &fakeConnsMutable{summary: summary},
		Query: tablerQuery{tr: adapterhost.TablesResult{Refs: []tableref.Ref{{Name: "notes"}}}},
	}}
	rv := queryResolution{summary: summary, mode: modeWrite}
	res, _, _, done := s.recheckAfterApproval(context.Background(), runQueryArgs{ConnectionID: "c1", SQL: "update notes set x = 1"}, &rv)
	if !done || res == nil || !res.IsError || !strings.Contains(resultText(res), "not listed: notes") {
		t.Fatalf("done=%v res=%v; want a refusal naming the unlisted table", done, res)
	}
}
