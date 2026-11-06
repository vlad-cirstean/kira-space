package dbmcp

import (
	"context"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

type recheckReview struct {
	fakeColumnReviewNone
	calls int
	next  maskreview.Withheld
}

func (r *recheckReview) Recheck(context.Context, model.ConnectionSummary) maskreview.Withheld {
	r.calls++
	return r.next
}

func cols(names ...string) []page.ColumnDescriptor {
	out := make([]page.ColumnDescriptor, len(names))
	for i, n := range names {
		out[i] = page.ColumnDescriptor{Name: n}
	}
	return out
}

func TestSettleResult(t *testing.T) {
	known := map[string]bool{"id": true, "email": true}
	live := func(names ...string) map[string]bool {
		m := map[string]bool{}
		for _, n := range names {
			m[n] = true
		}
		return m
	}
	cases := []struct {
		name      string
		known     map[string]bool
		columns   []page.ColumnDescriptor
		next      maskreview.Withheld
		wantCalls int
		wantDrop  []string
		refused   bool
	}{
		{name: "review off never refreshes", known: nil, columns: cols("anything"), wantCalls: 0},
		{name: "all names known skips the refresh", known: known, columns: cols("ID", "Email"), wantCalls: 0},
		{name: "alias stays after the refresh", known: known, columns: cols("id", "count(*)"),
			next: maskreview.Withheld{On: true, Names: map[string]bool{}, Known: known}, wantCalls: 1},
		{name: "column added since the check is withheld", known: known, columns: cols("id", "SSN"),
			next:      maskreview.Withheld{On: true, Names: map[string]bool{"ssn": true}, Known: live("id", "email", "ssn")},
			wantCalls: 1, wantDrop: []string{"ssn"}},
		{name: "several unknown names refresh once", known: known, columns: cols("a", "b", "c"),
			next: maskreview.Withheld{On: true, Names: map[string]bool{"b": true}, Known: live("id", "b")}, wantCalls: 1, wantDrop: []string{"b"}},
		{name: "incomplete refresh refuses", known: known, columns: cols("x"),
			next: maskreview.Withheld{On: true, Incomplete: "the column check failed"}, wantCalls: 1, refused: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rv := &recheckReview{next: c.next}
			s := &Server{cfg: Config{ColumnReview: rv}}
			got, refusal := s.settleResult(context.Background(), model.ConnectionSummary{}, maskset{known: c.known}, c.columns)
			if rv.calls != c.wantCalls {
				t.Fatalf("refreshes = %d, want %d", rv.calls, c.wantCalls)
			}
			if (refusal != nil) != c.refused {
				t.Fatalf("refusal = %v, want %v", refusal != nil, c.refused)
			}
			if c.refused {
				return
			}
			keep, withheld := keptColumns(c.columns, &got)
			if len(withheld) != len(c.wantDrop) || len(keep) != len(c.columns)-len(c.wantDrop) {
				t.Fatalf("kept %d withheld %v, want drop %v", len(keep), withheld, c.wantDrop)
			}
		})
	}
}

// A column that shows up while a result waits for review changes the withheld set, so the held-result
// recheck (same settleResult over the stored result columns) sees a mismatch and withholds the result.
func TestSettleResultHeldRecheck(t *testing.T) {
	rendered := maskset{known: map[string]bool{"id": true}, withheld: map[string]bool{}, resultColumns: cols("id", "ssn")}
	rv := &recheckReview{next: maskreview.Withheld{On: true, Names: map[string]bool{"ssn": true}, Known: map[string]bool{"id": true, "ssn": true}}}
	s := &Server{cfg: Config{ColumnReview: rv}}
	now, refusal := s.settleResult(context.Background(), model.ConnectionSummary{}, maskset{known: rendered.known, withheld: map[string]bool{}}, rendered.resultColumns)
	if refusal != nil || rv.calls != 1 || now.Equal(rendered) {
		t.Fatalf("refusal=%v calls=%d equal=%v; want one refresh and a changed set", refusal != nil, rv.calls, now.Equal(rendered))
	}
}
