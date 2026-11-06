package maskreview

import (
	"context"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
)

type fakeTree struct {
	cols  map[string][]string // database -> columns of its one table "t"
	reads map[string]int      // live column reads per database
}

func (f *fakeTree) Children(_ context.Context, _, path string, _ bool) (tree.ChildrenResult, error) {
	var nodes []model.TreeNode
	if path == "" {
		for db := range f.cols {
			nodes = append(nodes, model.TreeNode{Kind: "database", Name: db, Path: model.EncodePath([]model.PathSegment{{Kind: "database", Name: db}})})
		}
	}
	return tree.ChildrenResult{Nodes: nodes}, nil
}

func (f *fakeTree) SchemaColumns(_ context.Context, _, path string, refresh bool) (tree.SchemaColumnsResult, error) {
	p, _ := model.DecodePath("c1", path)
	db := p.Segments[0].Name
	if refresh {
		f.reads[db]++
	}
	rel := model.RelationColumns{Name: "t", Kind: "table"}
	for _, c := range f.cols[db] {
		rel.Columns = append(rel.Columns, model.ColumnMeta{Name: c})
	}
	return tree.SchemaColumnsResult{Relations: []model.RelationColumns{rel}}, nil
}

type noRules struct{}

func (noRules) List(string) ([]model.MaskRule, error)      { return nil, nil }
func (noRules) ListForConnection(string) ([]string, error) { return nil, nil }

// Recheck reads live only the containers of the statement's relations, remembers an alias, and does not
// restart the freshness window.
func TestRecheckReadsOnlyStatementContainers(t *testing.T) {
	ft := &fakeTree{cols: map[string][]string{"db0": {"id"}, "db1": {"id"}, "db2": {"id"}}, reads: map[string]int{}}
	clock := time.Unix(1000, 0)
	s := New(Deps{Tree: ft, Rules: noRules{}, Reviewed: noRules{}, Now: func() time.Time { return clock }})
	summary := model.ConnectionSummary{ID: "c1", ConnectionFields: model.ConnectionFields{Kind: "mysql", MaskReviewOn: true}}
	s.For(context.Background(), summary)
	total := func() int { return ft.reads["db0"] + ft.reads["db1"] + ft.reads["db2"] }
	if total() != 3 {
		t.Fatalf("full check reads = %d, want 3", total())
	}
	clock = clock.Add(10 * time.Second)

	req := Rescan{Tables: []tableref.Ref{{Database: "db1", Name: "t"}}, Parsed: true, Names: []string{"total"}}
	w := s.Recheck(context.Background(), summary, req)
	if total() != 4 || ft.reads["db1"] != 2 || w.Incomplete != "" {
		t.Fatalf("reads = %v, want one more read of db1 only", ft.reads)
	}
	if got := w.Known.Unsettled(req.Tables, true, req.Names); got != nil {
		t.Fatalf("alias not remembered: %v", got)
	}

	ft.cols["db1"] = []string{"id", "ssn"}
	clock = clock.Add(10 * time.Second)
	w = s.Recheck(context.Background(), summary, Rescan{Tables: req.Tables, Parsed: true, Names: []string{"ssn"}})
	if total() != 5 || !w.Names["ssn"] {
		t.Fatalf("reads = %v names = %v; want db1 read again and ssn withheld", ft.reads, w.Names)
	}
	clock = clock.Add(11 * time.Second) // 31 s after the full check
	if s.fresh(summary) != nil {
		t.Fatal("a partial refresh must not extend the freshness window")
	}
}
