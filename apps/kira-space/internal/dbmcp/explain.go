package dbmcp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/queryplan"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// approvalPlanIssuesCap bounds ApprovalPlan.Issues on the wire — a rolled-up plan can carry many,
// the dialog renders every one, and an uncapped list is the same unbounded-render hazard
// bridge/dbmcp.go's own capApprovalStatement already guards for the statement text (M3 §6.2).
const approvalPlanIssuesCap = 10

// approvalPlanFrom projects plan into the evidence the approval dialog shows: warn-severity
// issues first (the ones a decision turns on), capped at approvalPlanIssuesCap with
// IssuesOmitted carrying the remainder. nil in, nil out — an ordinary permission prompt with no
// plan attached (auto-force-explain off, or the statement was not explainable) carries no plan.
func approvalPlanFrom(plan *queryplan.Plan, thresholdRows int) *ApprovalPlan {
	if plan == nil {
		return nil
	}
	sorted := make([]queryplan.Issue, len(plan.Issues))
	copy(sorted, plan.Issues)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Severity == "warn" && sorted[j].Severity != "warn"
	})
	omitted := 0
	if len(sorted) > approvalPlanIssuesCap {
		omitted = len(sorted) - approvalPlanIssuesCap
		sorted = sorted[:approvalPlanIssuesCap]
	}
	issues := make([]ApprovalPlanIssue, len(sorted))
	for i, iss := range sorted {
		issues[i] = ApprovalPlanIssue(iss)
	}
	return &ApprovalPlan{
		EstimatedRowsRead: plan.EstimatedRowsRead,
		ThresholdRows:     thresholdRows,
		OverThreshold:     plan.OverThreshold,
		Issues:            issues,
		IssuesOmitted:     omitted,
	}
}

// planSummary is what an AI client gets beside a run_query result, and what the approval dialog
// renders: the numbers a decision turns on, never the whole tree (that is explain's job).
type planSummary struct {
	EstimatedRowsRead *float64          `json:"estimatedRowsRead,omitempty"`
	ThresholdRows     int               `json:"thresholdRows"`
	OverThreshold     bool              `json:"overThreshold"`
	Issues            []queryplan.Issue `json:"issues"`
}

func summaryOf(plan *queryplan.Plan, thresholdRows int) *planSummary {
	if plan == nil {
		return nil
	}
	return &planSummary{
		EstimatedRowsRead: plan.EstimatedRowsRead,
		ThresholdRows:     thresholdRows,
		OverThreshold:     plan.OverThreshold,
		Issues:            plan.Issues,
	}
}

// maskPlanForMaskedConnection strips whatever an EXPLAIN plan can carry of real row data when the
// connection has active mask rules: MySQL/MariaDB substitute real column values from a
// const-evaluated table into another table's own attached_condition (surfaced as Node.Detail) and
// index_condition (a metric), and ClickHouse's index metrics carry the same kind of evaluated
// condition text (" condition"); only allowlisted metric labels survive. Column-name masking cannot
// see through either — they are plan prose, not a result
// set row — so both are blanked outright rather than filtered by column name. Raw is dropped
// unconditionally too, regardless of IncludeRaw: it is the server's own EXPLAIN text verbatim, the
// same data by construction.
func maskPlanForMaskedConnection(plan queryplan.Plan) queryplan.Plan {
	plan.Raw = ""
	plan.Root = maskPlanNode(plan.Root)
	return plan
}

// planMetricLabels are metric labels known to hold no evaluated row data. An allowlist, not a
// blocklist: a suffix match already missed MySQL/MariaDB's `index_condition` (P168 Part 6 F6),
// which holds the same const-table-substituted values as `attached_condition`.
var planMetricLabels = map[string]bool{
	"filtered": true, "key": true, "key_length": true, "used_key_parts": true, "access_type": true,
	"possible_keys": true, "used_columns": true,
	"Join Type": true, "Index Name": true, "Scan Direction": true, "Parallel Aware": true,
	"Async Capable": true, "Plan Width": true, "Strategy": true, "Parent Relationship": true,
	"Alias": true, "Schema": true,
}

func planMetricCarriesNoRowData(label string) bool {
	if planMetricLabels[label] || strings.HasPrefix(label, "cost_info.") ||
		strings.HasPrefix(label, "rows") || strings.HasPrefix(label, "using_") {
		return true
	}
	for _, suffix := range []string{" keys", " parts", " granules", " search"} {
		if strings.HasSuffix(label, suffix) {
			return true
		}
	}
	return false
}

func maskPlanNode(n queryplan.Node) queryplan.Node {
	n.Detail = ""
	if len(n.Metrics) > 0 {
		kept := make([]queryplan.Metric, 0, len(n.Metrics))
		for _, m := range n.Metrics {
			if planMetricCarriesNoRowData(m.Label) {
				kept = append(kept, m)
			}
		}
		n.Metrics = kept
	}
	if len(n.Children) > 0 {
		children := make([]queryplan.Node, len(n.Children))
		for i, c := range n.Children {
			children[i] = maskPlanNode(c)
		}
		n.Children = children
	}
	return n
}

// assertComposedStatementsAreReads is the checkable defense in depth: before executing a
// composed EXPLAIN batch, every statement statements.go composed is passed through
// Query.ClassifyStatement and must report ClassRead. This is the one property the whole "EXPLAIN
// never bypasses the permission gate" argument rests on — if a future dialect's EXPLAIN
// spelling ever classifies as anything but a read, it must not reach the server through this path.
// A non-read verdict (or a classification failure) is an internal refusal, not a caller error —
// the caller did nothing wrong; this app's own composer produced something it does not trust.
func (s *Server) assertComposedStatementsAreReads(ctx context.Context, connectionID string, statements []string) error {
	for _, stmt := range statements {
		class, err := s.cfg.Query.ClassifyStatement(ctx, connectionID, stmt)
		if err != nil {
			s.log.Warn("dbmcp: explain: classification of composed EXPLAIN failed", "connectionId", connectionID, "err", err)
			return fmt.Errorf("dbmcp: classify composed EXPLAIN statement: %w", err)
		}
		if class != adapters.ClassRead {
			s.log.Error("dbmcp: explain: composed EXPLAIN did not classify as a read", "connectionId", connectionID, "class", class, "statement", stmt)
			return fmt.Errorf("dbmcp: composed EXPLAIN statement classified as %s, not a read — refusing", class)
		}
	}
	return nil
}

// planFor composes, verifies and runs this connection's own EXPLAIN for sql, and parses the
// result against threshold. Returns (nil, nil) when this kind has no EXPLAIN this app can parse or sql is
// not explainable — a no-op, not a failure. Failures are the caller's to treat as a refusal: a
// *planCheckError (app text, no adapter data) when the plan cannot be composed, trusted or read, else the
// adapter's own error wrapped.
func (s *Server) planFor(ctx context.Context, summary model.ConnectionSummary, sql, path string, threshold int) (*queryplan.Plan, error) {
	if !queryplan.Supported(summary.Kind) || !queryplan.Explainable(sql) {
		return nil, nil
	}
	return s.planStatements(ctx, summary, queryplan.StatementsFor(summary.Kind, sql), path, threshold)
}

// planStatements runs and parses already-composed EXPLAIN statements. The caller owns the composition and
// the Explainable check: browse_table composes from a spec whose literals may hold ";".
func (s *Server) planStatements(ctx context.Context, summary model.ConnectionSummary, statements []string, path string, threshold int) (*queryplan.Plan, error) {
	if err := s.assertComposedStatementsAreReads(ctx, summary.ID, statements); err != nil {
		s.log.Warn("dbmcp: expensive_query: composed EXPLAIN refused", "connectionId", summary.ID, "err", err)
		return nil, &planCheckError{msg: fmt.Sprintf("expensive_query check could not plan this statement on %q; it did not run.", summary.Name)}
	}

	req := adapterhost.ExecuteRequestWire{
		OpID: ids.NewID(), ConnectionID: summary.ID, Path: path, Statements: statements,
	}
	applyStatementTimeout(&req, summary, modeRead)
	resp, err := s.cfg.Query.ExecuteRead(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("expensive_query check: EXPLAIN failed: %w", err)
	}

	plan, err := queryplan.FromPages(summary.Kind, resp.Pages, threshold)
	if err != nil {
		why := "unreadable plan"
		if errors.Is(err, queryplan.ErrTruncated) {
			why = "plan too large to parse"
		}
		s.log.Warn("dbmcp: expensive_query: plan unreadable", "connectionId", summary.ID, "err", err)
		return nil, &planCheckError{msg: fmt.Sprintf("expensive_query check could not read the query plan's row estimate on %q (%s); it did not run. Simplify the statement. The user can turn the check off in the connection's MCP tab.", summary.Name, why)}
	}
	return &plan, nil
}
