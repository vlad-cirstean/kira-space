package dbmcp

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/queryplan"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// expensiveQueryLimit is the built-in PreLimit LimitExpensiveQuery (P530): the plan's estimated rows read
// against advanced.expensiveQueryRows. The server runs the one EXPLAIN; this only compares.
type expensiveQueryLimit struct{}

func (expensiveQueryLimit) Name() string { return LimitExpensiveQuery }

// Check fails closed on c.PlanErr: a limit an agent passes by making EXPLAIN fail is no limit. A call
// with no plan, or a plan with no estimate, passes; the estimate at or above c.Threshold hits.
func (expensiveQueryLimit) Check(_ context.Context, c LimitCall) (*LimitHit, error) {
	if c.PlanErr != nil {
		return nil, c.PlanErr
	}
	if c.Plan == nil || c.Plan.EstimatedRowsRead == nil || *c.Plan.EstimatedRowsRead < float64(c.Threshold) {
		return nil, nil
	}
	return &LimitHit{
		Limit:    LimitExpensiveQuery,
		Measured: *c.Plan.EstimatedRowsRead,
		Max:      float64(c.Threshold),
		Unit:     UnitRows,
	}, nil
}

// planCheckError is an app-authored check failure: no adapter text, safe on a masked connection.
type planCheckError struct{ msg string }

func (e *planCheckError) Error() string { return e.msg }

// Check states of list_connections' expensiveQuery.check.
const (
	expensiveCheckPlan = "plan"
	expensiveCheckOff  = "off"
	expensiveCheckNone = "none"
)

// expensiveCheckOf is list_connections' expensiveQuery.check: none (no row estimate), off (switch off) or plan.
func expensiveCheckOf(c model.ConnectionSummary) string {
	switch {
	case !queryplan.HasRowEstimate(c.Kind):
		return expensiveCheckNone
	case !c.McpAutoExplain:
		return expensiveCheckOff
	}
	return expensiveCheckPlan
}

// expensiveQueryView is list_connections' expensiveQuery.
type expensiveQueryView struct {
	RowsThreshold int    `json:"rowsThreshold"`
	Check         string `json:"check"`
}
