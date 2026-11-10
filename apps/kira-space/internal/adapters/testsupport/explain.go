package testsupport

import (
	"context"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/queryplan"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ExplainPlanCase drives ExplainPlan. Path is the console context the statements run under.
// FullScan must estimate at least MinRows; Point must estimate under MaxPointRows (engines whose
// estimate is block-granular pass a looser bound).
type ExplainPlanCase struct {
	Path         model.NodePath
	FullScan     string
	Point        string
	MinRows      int64
	MaxPointRows int64
}

// ExplainPlan asserts the EXPLAIN statements the DB MCP expensive-query check composes
// (queryplan.StatementsFor) run on the read lane's read-only session and parse into a plan whose
// row estimate separates a full scan from a point read. Engines without a row estimate (sqlite)
// must still parse to a plan.
func ExplainPlan(c ExplainPlanCase) Scenario {
	return Scenario{
		Name:     "explain: the composed EXPLAIN runs on the lane and parses",
		Requires: func(caps adapters.Caps) bool { return caps.SQL },
		Run: func(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig) {
			t.Helper()
			kind := a.Kind()
			if !queryplan.Supported(kind) {
				t.Skipf("%s has no EXPLAIN this app parses", kind)
			}
			lane := laneAdapter(t, a, cfg)
			explain := func(sql string) queryplan.Plan {
				t.Helper()
				pages, err := lane.Execute(context.Background(), model.ConsoleRequest{
					Path: c.Path, Statements: queryplan.StatementsFor(kind, sql), ReadOnly: true,
				}, adapters.NewOpCtx("lane-explain"))
				if err != nil {
					t.Fatalf("lane EXPLAIN of %s: %v", sql, err)
				}
				plan, err := queryplan.FromPages(kind, pages, 100000)
				if err != nil {
					t.Fatalf("FromPages(%s, %s): %v", kind, sql, err)
				}
				return plan
			}
			full, point := explain(c.FullScan), explain(c.Point)
			if !queryplan.HasRowEstimate(kind) {
				if full.EstimatedRowsRead != nil {
					t.Errorf("%s plan carries a row estimate %v, want none", kind, *full.EstimatedRowsRead)
				}
				return
			}
			if full.EstimatedRowsRead == nil || point.EstimatedRowsRead == nil {
				t.Fatalf("%s plan has no row estimate: full %v, point %v", kind, full.EstimatedRowsRead, point.EstimatedRowsRead)
			}
			if got := int64(*full.EstimatedRowsRead); got < c.MinRows {
				t.Errorf("full scan estimate = %d, want >= %d", got, c.MinRows)
			}
			if got := int64(*point.EstimatedRowsRead); got >= c.MaxPointRows {
				t.Errorf("point read estimate = %d, want < %d", got, c.MaxPointRows)
			}
		},
	}
}
