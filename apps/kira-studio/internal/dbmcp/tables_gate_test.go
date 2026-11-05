package dbmcp

import (
	"context"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
)

// tablerRunner names a fixed table for every statement and counts executions.
type tablerRunner struct {
	fakeQueryRunner
	ref tableref.Ref
}

func (r *tablerRunner) StatementTables(context.Context, string, string, string) (adapterhost.TablesResult, error) {
	return adapterhost.TablesResult{Refs: []tableref.Ref{r.ref}}, nil
}

// Every statement path reaches the allow list before any execution: run_query and explain_query refuse an
// unlisted table, and the same call on a listed one gets past the gate.
func TestStatementToolsCheckTheAllowList(t *testing.T) {
	summary := model.ConnectionSummary{
		ID: "c1", Name: "conn", Kind: "postgres", McpEnabled: true,
		McpReadMode: "allow", McpWriteMode: "deny", McpDdlMode: "deny",
		McpTables: []string{"shop.public.orders"},
	}
	conns := &fakeConnsReader{state: model.ConnectionState{ConnectionID: "c1", Status: "connected"}}
	conns2 := &fakeConnsListState{fakeConnsReader: conns, summaries: []model.ConnectionSummary{summary}}
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "shop"}})
	for _, tc := range []struct {
		name  string
		ref   tableref.Ref
		block bool
	}{
		{"unlisted", tableref.Ref{Database: "shop", Schema: "public", Name: "payroll"}, true},
		{"listed", tableref.Ref{Database: "shop", Schema: "public", Name: "orders"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qr := &tablerRunner{fakeQueryRunner: fakeQueryRunner{class: adapters.ClassRead}, ref: tc.ref}
			s := &Server{cfg: Config{Conns: conns2, Query: qr, MaskRules: fakeMaskRulesNone{}, ColumnReview: fakeColumnReviewNone{}, ExplainThreshold: func() int { return 1000 }}}
			res, _, err := s.explainQuery(context.Background(), nil, explainQueryArgs{ConnectionID: "c1", SQL: "SELECT 1", Path: path})
			if tc.block {
				if err != nil || res == nil || !res.IsError || !strings.Contains(resultText(res), "not listed: shop.public.payroll") {
					t.Fatalf("explain_query = %+v (%v), want a not-listed refusal", res, err)
				}
			} else if res != nil && strings.Contains(resultText(res), "not listed") {
				t.Fatalf("explain_query on a listed table was refused: %s", resultText(res))
			}
			res, _, err = s.checkStatementTables(context.Background(), summary, path, "SELECT 1")
			if (res != nil || err != nil) != tc.block {
				t.Fatalf("checkStatementTables blocked = %v, want %v", res != nil || err != nil, tc.block)
			}
		})
	}
}

type fakeConnsListState struct {
	*fakeConnsReader
	summaries []model.ConnectionSummary
}

func (f *fakeConnsListState) List() ([]model.ConnectionSummary, error) { return f.summaries, nil }
