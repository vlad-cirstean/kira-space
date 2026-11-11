package queryplan

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// mysql.go ports planParsers/mysql.ts verbatim: MySQL's `EXPLAIN FORMAT=JSON`. Two schemas: schema 1
// (8.0, 8.4 default) roots at query_block, schema 2.0 (9.x default explain_json_format_version=2)
// at query_plan, an iterator tree of operation nodes under inputs (mysqlV2Node). Schema 1: one row, one
// column, a query_block whose real content sits under `table` (single-table), `nested_loop` (a
// join — same key MariaDB's own JSON uses, but a genuinely different schema underneath), or a
// wrapping grouping_operation/ordering_operation/duplicates_removal block around either.

type mysqlRawTable struct {
	TableName                *string            `json:"table_name"`
	AccessType               *string            `json:"access_type"`
	PossibleKeys             []string           `json:"possible_keys"`
	Key                      *string            `json:"key"`
	RowsExaminedPerScan      *float64           `json:"rows_examined_per_scan"`
	AttachedCondition        *string            `json:"attached_condition"`
	UsingTemporaryTable      *bool              `json:"using_temporary_table"`
	UsingFilesort            *bool              `json:"using_filesort"`
	CostInfo                 json.RawMessage    `json:"cost_info"`
	MaterializedFromSubquery *mysqlMaterialized `json:"materialized_from_subquery"`
}

type mysqlMaterialized struct {
	UsingTemporaryTable *bool           `json:"using_temporary_table"`
	QueryBlock          json.RawMessage `json:"query_block"`
}

type mysqlRawBlock struct {
	CostInfo          *mysqlCostInfo   `json:"cost_info"`
	Table             json.RawMessage  `json:"table"`
	NestedLoop        []mysqlLoopEntry `json:"nested_loop"`
	GroupingOperation json.RawMessage  `json:"grouping_operation"`
	OrderingOperation json.RawMessage  `json:"ordering_operation"`
	DuplicatesRemoval json.RawMessage  `json:"duplicates_removal"`
}

type mysqlLoopEntry struct {
	Table json.RawMessage `json:"table"`
}

type mysqlCostInfo struct {
	QueryCost *string `json:"query_cost"`
}

var mysqlTableTypedKeys = map[string]bool{
	"table_name": true, "access_type": true, "possible_keys": true, "key": true,
	"rows_examined_per_scan": true, "attached_condition": true, "materialized_from_subquery": true,
	// cost_info is special-cased below (expanded into cost_info.<subkey> metrics), never emitted
	// as a single metric of its own.
	"cost_info": true,
}

func mysqlTableMetrics(rawMap map[string]json.RawMessage) []Metric {
	out := make([]Metric, 0, len(rawMap))
	for key, value := range rawMap {
		if key == "cost_info" {
			for subKey, subValue := range decodeRawObject(value) {
				out = append(out, Metric{Label: "cost_info." + subKey, Value: formatMetricValue(subValue)})
			}
			continue
		}
		if mysqlTableTypedKeys[key] {
			continue
		}
		out = append(out, Metric{Label: key, Value: formatMetricValue(value)})
	}
	return sortMetricsByLabel(out)
}

func mysqlTableNode(raw json.RawMessage, thresholdRows int, scans *[]float64) Node {
	var table mysqlRawTable
	_ = json.Unmarshal(raw, &table)
	rawMap := decodeRawObject(raw)

	var cost *Cost
	if table.CostInfo != nil {
		var costInfo struct {
			PrefixCost *string `json:"prefix_cost"`
		}
		_ = json.Unmarshal(table.CostInfo, &costInfo)
		if costInfo.PrefixCost != nil {
			cost = &Cost{Total: parseFloatCell(*costInfo.PrefixCost)}
		}
	}

	node := Node{
		Label:         tableLabel(table.TableName, table.AccessType),
		EstimatedRows: table.RowsExaminedPerScan,
		Cost:          cost,
		Metrics:       mysqlTableMetrics(rawMap),
		Issues:        []Issue{},
		Children:      []Node{},
	}
	if table.TableName != nil {
		node.Relation = *table.TableName
	}
	if table.AttachedCondition != nil {
		node.Detail = *table.AttachedCondition
	}

	relationOrPlaceholder := "?"
	if table.TableName != nil {
		relationOrPlaceholder = *table.TableName
	}

	// D15: full scan / unused index.
	pushIndexIssues(&node, relationOrPlaceholder, table.AccessType, table.PossibleKeys, table.Key)
	// D15: filesort, when the flag sits on the table itself rather than an ordering_operation
	// wrapper (both are real shapes).
	if table.UsingFilesort != nil && *table.UsingFilesort {
		node.Issues = append(node.Issues, Issue{
			Severity: "warn", Code: "filesort",
			Message: fmt.Sprintf("“%s” was sorted with a temporary file (filesort).", relationOrPlaceholder),
		})
	}
	// D15: a derived table materialized into a temp table.
	materializedTemp := table.MaterializedFromSubquery != nil && table.MaterializedFromSubquery.UsingTemporaryTable != nil && *table.MaterializedFromSubquery.UsingTemporaryTable
	if (table.UsingTemporaryTable != nil && *table.UsingTemporaryTable) || materializedTemp {
		node.Issues = append(node.Issues, Issue{
			Severity: "warn", Code: "temp-table",
			Message: fmt.Sprintf("“%s” was materialised into a temporary table.", relationOrPlaceholder),
		})
	}

	if table.MaterializedFromSubquery != nil && table.MaterializedFromSubquery.QueryBlock != nil {
		node.Children = append(node.Children, mysqlBlockNodes(table.MaterializedFromSubquery.QueryBlock, thresholdRows, scans)...)
	}

	if table.RowsExaminedPerScan != nil {
		*scans = append(*scans, *table.RowsExaminedPerScan)
		pushWideScanIssue(&node, *table.RowsExaminedPerScan, thresholdRows)
	}
	return node
}

func mysqlWrap(label string, children []Node, issues []Issue) Node {
	if issues == nil {
		issues = []Issue{}
	}
	if children == nil {
		children = []Node{}
	}
	return Node{Label: label, Metrics: []Metric{}, Issues: issues, Children: children}
}

func mysqlBlockNodes(raw json.RawMessage, thresholdRows int, scans *[]float64) []Node {
	if raw == nil {
		return []Node{}
	}
	var block mysqlRawBlock
	_ = json.Unmarshal(raw, &block)

	if block.OrderingOperation != nil {
		var op struct {
			UsingFilesort *bool `json:"using_filesort"`
		}
		_ = json.Unmarshal(block.OrderingOperation, &op)
		var issues []Issue
		if op.UsingFilesort != nil && *op.UsingFilesort {
			issues = []Issue{{Severity: "warn", Code: "filesort", Message: "A temporary sort file (filesort) was used."}}
		}
		return []Node{mysqlWrap("Sort", mysqlBlockNodes(block.OrderingOperation, thresholdRows, scans), issues)}
	}
	if block.GroupingOperation != nil {
		return []Node{mysqlWrap("Group by", mysqlBlockNodes(block.GroupingOperation, thresholdRows, scans), nil)}
	}
	if block.DuplicatesRemoval != nil {
		return []Node{mysqlWrap("Duplicates removal", mysqlBlockNodes(block.DuplicatesRemoval, thresholdRows, scans), nil)}
	}
	if block.NestedLoop != nil {
		children := make([]Node, 0, len(block.NestedLoop))
		for _, entry := range block.NestedLoop {
			if entry.Table != nil {
				children = append(children, mysqlTableNode(entry.Table, thresholdRows, scans))
			}
		}
		return []Node{mysqlWrap("Nested loop join", children, nil)}
	}
	if block.Table != nil {
		return []Node{mysqlTableNode(block.Table, thresholdRows, scans)}
	}
	return []Node{}
}

var mysqlV2TypedKeys = map[string]bool{
	"operation": true, "table_name": true, "condition": true, "lookup_condition": true,
	"estimated_rows": true, "estimated_total_cost": true, "inputs": true,
}

type mysqlV2Raw struct {
	Operation          *string           `json:"operation"`
	TableName          *string           `json:"table_name"`
	AccessType         *string           `json:"access_type"`
	Condition          *string           `json:"condition"`
	LookupCondition    *string           `json:"lookup_condition"`
	EstimatedRows      *float64          `json:"estimated_rows"`
	EstimatedTotalCost *float64          `json:"estimated_total_cost"`
	Inputs             []json.RawMessage `json:"inputs"`
}

// mysqlV2Node builds a schema 2.0 node. Every leaf with an estimate is a scan (the access paths);
// wrappers such as Sort or Filter may carry no estimate.
func mysqlV2Node(raw json.RawMessage, thresholdRows int, scans *[]float64) Node {
	var n mysqlV2Raw
	_ = json.Unmarshal(raw, &n)

	metrics := make([]Metric, 0)
	for key, value := range decodeRawObject(raw) {
		if !mysqlV2TypedKeys[key] {
			metrics = append(metrics, Metric{Label: key, Value: formatMetricValue(value)})
		}
	}
	node := Node{
		Label: "?", EstimatedRows: n.EstimatedRows, Metrics: sortMetricsByLabel(metrics),
		Issues: []Issue{}, Children: []Node{},
	}
	if n.Operation != nil {
		node.Label = *n.Operation
	}
	if n.TableName != nil {
		node.Relation = *n.TableName
	}
	if n.Condition != nil {
		node.Detail = *n.Condition
	} else if n.LookupCondition != nil {
		node.Detail = *n.LookupCondition
	}
	if n.EstimatedTotalCost != nil {
		node.Cost = &Cost{Total: *n.EstimatedTotalCost}
	}

	access := ""
	if n.AccessType != nil {
		access = *n.AccessType
	}
	relation := "?"
	if n.TableName != nil {
		relation = *n.TableName
	}
	// A base-table scan is a leaf; a scan with inputs reads a materialised derived table or CTE.
	if access == "table" && len(n.Inputs) == 0 && n.TableName != nil && *n.TableName != "" && !strings.HasPrefix(*n.TableName, "<") {
		node.Issues = append(node.Issues, Issue{
			Severity: "warn", Code: "full-scan",
			Message: fmt.Sprintf("Full table scan on “%s”. No index was used.", relation),
		})
	}
	if access == "sort" {
		node.Issues = append(node.Issues, Issue{Severity: "warn", Code: "filesort", Message: "A temporary sort file (filesort) was used."})
	}
	if access == "temp_table_aggregate" || access == "materialize" {
		node.Issues = append(node.Issues, Issue{
			Severity: "warn", Code: "temp-table",
			Message: fmt.Sprintf("“%s” was materialised into a temporary table.", relation),
		})
	}

	for _, input := range n.Inputs {
		node.Children = append(node.Children, mysqlV2Node(input, thresholdRows, scans))
	}
	if len(n.Inputs) == 0 && n.EstimatedRows != nil {
		*scans = append(*scans, *n.EstimatedRows)
		pushWideScanIssue(&node, *n.EstimatedRows, thresholdRows)
	}
	return node
}

// mysqlV2TopCost returns the cost of the topmost node carrying one, breadth-first: MySQL 9 omits
// estimated_total_cost on Sort and temp-table aggregate nodes.
func mysqlV2TopCost(top *Node) *float64 {
	queue := []*Node{top}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n.Cost != nil {
			return &n.Cost.Total
		}
		for i := range n.Children {
			queue = append(queue, &n.Children[i])
		}
	}
	return nil
}

func parseMysqlV2(rawText string, plan json.RawMessage, thresholdRows int) Plan {
	scans := make([]float64, 0)
	child := mysqlV2Node(plan, thresholdRows, &scans)

	var rootCost *Cost
	var nativeCost *NativeCost
	if cost := mysqlV2TopCost(&child); cost != nil {
		rootCost = &Cost{Total: *cost}
		nativeCost = &NativeCost{Value: *cost, Unit: CostUnitMysqlCost}
	}
	root := Node{
		Label: i18n.T("backend.queryPlan.root.query.label", nil), Cost: rootCost,
		Metrics: []Metric{}, Issues: []Issue{}, Children: []Node{child},
	}
	estimatedRowsRead := maxEstimatedRows(scans)
	return Plan{
		Kind: "mysql", Root: root, EstimatedRowsRead: estimatedRowsRead, NativeCost: nativeCost,
		Issues: rollupIssues(&root), OverThreshold: isOverThreshold(estimatedRowsRead, thresholdRows),
		Raw: rawText,
	}
}

func parseMysqlPlan(rawText string, thresholdRows int) (Plan, error) {
	var parsed struct {
		QueryBlock json.RawMessage `json:"query_block"`
		QueryPlan  json.RawMessage `json:"query_plan"`
	}
	if err := json.Unmarshal([]byte(rawText), &parsed); err != nil {
		return Plan{}, fmt.Errorf("queryplan: parse mysql plan: %w", err)
	}
	if len(parsed.QueryPlan) > 0 && parsed.QueryPlan[0] == '{' {
		return parseMysqlV2(rawText, parsed.QueryPlan, thresholdRows), nil
	}

	var block mysqlRawBlock
	if parsed.QueryBlock != nil {
		_ = json.Unmarshal(parsed.QueryBlock, &block)
	}

	scans := make([]float64, 0)
	children := mysqlBlockNodes(parsed.QueryBlock, thresholdRows, &scans)

	var queryCost *float64
	if block.CostInfo != nil && block.CostInfo.QueryCost != nil {
		v := parseFloatCell(*block.CostInfo.QueryCost)
		queryCost = &v
	}
	var rootCost *Cost
	if queryCost != nil {
		rootCost = &Cost{Total: *queryCost}
	}
	root := Node{Label: i18n.T("backend.queryPlan.root.query.label", nil), Cost: rootCost, Metrics: []Metric{}, Issues: []Issue{}, Children: children}

	estimatedRowsRead := maxEstimatedRows(scans)
	var nativeCost *NativeCost
	if queryCost != nil {
		nativeCost = &NativeCost{Value: *queryCost, Unit: CostUnitMysqlCost}
	}
	return Plan{
		Kind: "mysql", Root: root, EstimatedRowsRead: estimatedRowsRead, NativeCost: nativeCost,
		Issues: rollupIssues(&root), OverThreshold: isOverThreshold(estimatedRowsRead, thresholdRows),
		Raw: rawText,
	}, nil
}
