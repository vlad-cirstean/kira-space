package queryplan

import (
	"encoding/json"
	"fmt"
)

// postgres.go ports planParsers/postgres.ts verbatim: Postgres's `EXPLAIN (FORMAT JSON, COSTS
// TRUE, ...)` returns one row, one column QUERY PLAN, whose text is a JSON array of one
// `{ Plan: {...} }`. Only fields available *without* ANALYZE are read — this app never issues
// ANALYZE.

type pgRawNode struct {
	NodeType     *string           `json:"Node Type"`
	RelationName *string           `json:"Relation Name"`
	Alias        *string           `json:"Alias"`
	StartupCost  *float64          `json:"Startup Cost"`
	TotalCost    *float64          `json:"Total Cost"`
	PlanRows     *float64          `json:"Plan Rows"`
	Filter       *string           `json:"Filter"`
	IndexCond    *string           `json:"Index Cond"`
	HashCond     *string           `json:"Hash Cond"`
	Plans        []json.RawMessage `json:"Plans"`
}

// pgScanNodeTypes are the node types a full/partial *read* actually happens at — the ones whose
// own Plan Rows is what estimatedRowsRead is the max over (D14).
var pgScanNodeTypes = map[string]bool{
	"Seq Scan": true, "Index Scan": true, "Index Only Scan": true, "Bitmap Heap Scan": true,
	"CTE Scan": true, "Function Scan": true, "Foreign Scan": true,
}

// pgTypedKeys are every raw field this parser reads into a typed slot — the rest fall through to
// Metrics verbatim, so a field Postgres adds later still shows up rather than vanishing silently.
var pgTypedKeys = map[string]bool{
	"Node Type": true, "Relation Name": true, "Startup Cost": true, "Total Cost": true,
	"Plan Rows": true, "Filter": true, "Index Cond": true, "Hash Cond": true, "Plans": true,
}

func pgBuildNode(raw json.RawMessage, thresholdRows int, scans *[]float64) Node {
	var typed pgRawNode
	_ = json.Unmarshal(raw, &typed)
	rawMap := decodeRawObject(raw)

	nodeType := "Node"
	if typed.NodeType != nil {
		nodeType = *typed.NodeType
	}
	label := nodeType
	if typed.RelationName != nil {
		relation := *typed.RelationName
		if typed.Alias != nil && *typed.Alias != relation {
			label = fmt.Sprintf("%s on %s %s", nodeType, relation, *typed.Alias)
		} else {
			label = fmt.Sprintf("%s on %s", nodeType, relation)
		}
	}

	var detail *string
	switch {
	case typed.Filter != nil:
		detail = typed.Filter
	case typed.IndexCond != nil:
		detail = typed.IndexCond
	case typed.HashCond != nil:
		detail = typed.HashCond
	}

	var cost *Cost
	if typed.TotalCost != nil {
		cost = &Cost{Total: *typed.TotalCost, Startup: typed.StartupCost}
	}

	children := make([]Node, 0, len(typed.Plans))
	for _, childRaw := range typed.Plans {
		children = append(children, pgBuildNode(childRaw, thresholdRows, scans))
	}

	node := Node{
		Label:         label,
		EstimatedRows: typed.PlanRows,
		Cost:          cost,
		Metrics:       collectMetrics(rawMap, pgTypedKeys),
		Issues:        []Issue{},
		Children:      children,
	}
	if typed.RelationName != nil {
		node.Relation = *typed.RelationName
	}
	if detail != nil {
		node.Detail = *detail
	}

	relationOrPlaceholder := "?"
	if typed.RelationName != nil {
		relationOrPlaceholder = *typed.RelationName
	}

	// D15: full scan with a predicate — a Seq Scan that still carries a Filter means the planner
	// read every row and then discarded most of them client-side.
	if nodeType == "Seq Scan" && typed.Filter != nil {
		node.Issues = append(node.Issues, Issue{
			Severity: "warn", Code: "full-scan",
			Message: fmt.Sprintf("Full table scan on “%s” with a filter. No index was used.", relationOrPlaceholder),
		})
	}
	// D15 (info): index-only scan is a good sign, not a problem.
	if nodeType == "Index Only Scan" {
		node.Issues = append(node.Issues, Issue{
			Severity: "info", Code: "index-only-scan",
			Message: fmt.Sprintf("“%s” is read entirely from its index. No heap access needed.", relationOrPlaceholder),
		})
	}
	// D15: a nested loop whose inner side is wide re-executes that inner scan once per outer row.
	if nodeType == "Nested Loop" {
		for _, c := range node.Children {
			rows := 0.0
			if c.EstimatedRows != nil {
				rows = *c.EstimatedRows
			}
			if rows >= 10_000 {
				node.Issues = append(node.Issues, Issue{
					Severity: "warn", Code: "nested-loop-wide-inner",
					Message: fmt.Sprintf("Nested loop repeats a scan estimated at %s %s for every outer row.", commaFormat(*c.EstimatedRows), rowNoun(*c.EstimatedRows)),
				})
				break
			}
		}
	}

	// D15/D14: the "wide scan" issue is pushed here, before node's value is copied into its
	// parent's Children slice (plan.go's own note on why this cannot be deferred to a later pass
	// the way the TypeScript port's own buildNode+pushWideScanIssue split does it).
	if pgScanNodeTypes[nodeType] && typed.PlanRows != nil {
		*scans = append(*scans, *typed.PlanRows)
		pushWideScanIssue(&node, *typed.PlanRows, thresholdRows)
	}
	return node
}

func parsePostgresPlan(rawText string, thresholdRows int) (Plan, error) {
	var parsed []struct {
		Plan json.RawMessage `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(rawText), &parsed); err != nil {
		return Plan{}, fmt.Errorf("queryplan: parse postgres plan: %w", err)
	}
	var planRaw json.RawMessage
	if len(parsed) > 0 {
		planRaw = parsed[0].Plan
	} else {
		planRaw = json.RawMessage("{}")
	}

	scans := make([]float64, 0)
	root := pgBuildNode(planRaw, thresholdRows, &scans)

	estimatedRowsRead := maxEstimatedRows(scans)
	var nativeCost *NativeCost
	if root.Cost != nil {
		nativeCost = &NativeCost{Value: root.Cost.Total, Unit: CostUnitPostgresPlanner}
	}
	return Plan{
		Kind: "postgres", Root: root, EstimatedRowsRead: estimatedRowsRead, NativeCost: nativeCost,
		Issues: rollupIssues(&root), OverThreshold: isOverThreshold(estimatedRowsRead, thresholdRows),
		Raw: rawText,
	}, nil
}
