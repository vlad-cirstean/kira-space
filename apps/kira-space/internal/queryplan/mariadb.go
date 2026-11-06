package queryplan

import (
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// mariadb.go ports planParsers/mariadb.ts verbatim: MariaDB's `EXPLAIN FORMAT=JSON` — same
// statement spelling as MySQL's, a genuinely different response schema: every table sits
// inside a nested_loop array (even a single-table query), wrapped in read_sorted_file/filesort for
// a sort. The scalar cost field is named `cost` (≈ seconds under MariaDB 11.x's model) rather than
// MySQL's cost_info.query_cost, and `filtered` is a plain number, not MySQL's string.

type mariadbRawTable struct {
	TableName         *string  `json:"table_name"`
	AccessType        *string  `json:"access_type"`
	PossibleKeys      []string `json:"possible_keys"`
	Key               *string  `json:"key"`
	Rows              *float64 `json:"rows"`
	Cost              *float64 `json:"cost"`
	AttachedCondition *string  `json:"attached_condition"`
}

type mariadbLoopEntry struct {
	Table          json.RawMessage `json:"table"`
	ReadSortedFile *struct {
		Filesort *mariadbFilesort `json:"filesort"`
	} `json:"read_sorted_file"`
	BlockNLJoin *struct {
		NestedLoop []mariadbLoopEntry `json:"nested_loop"`
	} `json:"block-nl-join"`
}

type mariadbFilesort struct {
	SortKey    *string            `json:"sort_key"`
	Table      json.RawMessage    `json:"table"`
	NestedLoop []mariadbLoopEntry `json:"nested_loop"`
}

type mariadbRawBlock struct {
	Cost       *float64           `json:"cost"`
	NestedLoop []mariadbLoopEntry `json:"nested_loop"`
}

var mariadbTableTypedKeys = map[string]bool{
	"table_name": true, "access_type": true, "possible_keys": true, "key": true, "rows": true,
	"attached_condition": true,
}

func mariadbTableMetrics(rawMap map[string]json.RawMessage) []Metric {
	return collectMetrics(rawMap, mariadbTableTypedKeys)
}

func mariadbTableNode(raw json.RawMessage, thresholdRows int, scans *[]float64) Node {
	var table mariadbRawTable
	_ = json.Unmarshal(raw, &table)
	rawMap := decodeRawObject(raw)

	var cost *Cost
	if table.Cost != nil {
		cost = &Cost{Total: *table.Cost}
	}

	node := Node{
		Label:         tableLabel(table.TableName, table.AccessType),
		EstimatedRows: table.Rows,
		Cost:          cost,
		Metrics:       mariadbTableMetrics(rawMap),
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

	if table.Rows != nil {
		*scans = append(*scans, *table.Rows)
		pushWideScanIssue(&node, *table.Rows, thresholdRows)
	}
	return node
}

func mariadbEntryNode(entry mariadbLoopEntry, scans *[]float64, thresholdRows int) *Node {
	if entry.Table != nil {
		n := mariadbTableNode(entry.Table, thresholdRows, scans)
		return &n
	}

	// D15: filesort — a read_sorted_file/filesort wrapper node is present.
	if entry.ReadSortedFile != nil && entry.ReadSortedFile.Filesort != nil {
		fs := entry.ReadSortedFile.Filesort
		var child *Node
		switch {
		case fs.Table != nil:
			n := mariadbTableNode(fs.Table, thresholdRows, scans)
			child = &n
		case fs.NestedLoop != nil:
			n := mysqlWrap("Nested loop join", mariadbEntriesToNodes(fs.NestedLoop, scans, thresholdRows), nil)
			child = &n
		}
		children := []Node{}
		if child != nil {
			children = []Node{*child}
		}
		message := "A temporary sort file (filesort) was used."
		if fs.SortKey != nil {
			message = fmt.Sprintf("A temporary sort file (filesort) was used. Sort key %s.", *fs.SortKey)
		}
		n := mysqlWrap("Sort (filesort)", children, []Issue{{Severity: "warn", Code: "filesort", Message: message}})
		return &n
	}

	if entry.BlockNLJoin != nil && entry.BlockNLJoin.NestedLoop != nil {
		n := mysqlWrap("Block nested loop join", mariadbEntriesToNodes(entry.BlockNLJoin.NestedLoop, scans, thresholdRows), nil)
		return &n
	}

	return nil
}

func mariadbEntriesToNodes(entries []mariadbLoopEntry, scans *[]float64, thresholdRows int) []Node {
	out := make([]Node, 0, len(entries))
	for _, e := range entries {
		if n := mariadbEntryNode(e, scans, thresholdRows); n != nil {
			out = append(out, *n)
		}
	}
	return out
}

func parseMariadbPlan(rawText string, thresholdRows int) (Plan, error) {
	var parsed struct {
		QueryBlock *mariadbRawBlock `json:"query_block"`
	}
	if err := json.Unmarshal([]byte(rawText), &parsed); err != nil {
		return Plan{}, fmt.Errorf("queryplan: parse mariadb plan: %w", err)
	}
	block := mariadbRawBlock{}
	if parsed.QueryBlock != nil {
		block = *parsed.QueryBlock
	}

	scans := make([]float64, 0)
	var children []Node
	if block.NestedLoop != nil {
		children = mariadbEntriesToNodes(block.NestedLoop, &scans, thresholdRows)
	} else {
		children = []Node{}
	}

	var rootCost *Cost
	if block.Cost != nil {
		rootCost = &Cost{Total: *block.Cost}
	}
	root := Node{Label: i18n.T("backend.queryPlan.root.query.label", nil), Cost: rootCost, Metrics: []Metric{}, Issues: []Issue{}, Children: children}

	estimatedRowsRead := maxEstimatedRows(scans)
	var nativeCost *NativeCost
	if block.Cost != nil {
		nativeCost = &NativeCost{Value: *block.Cost, Unit: CostUnitMariadbCost}
	}
	return Plan{
		Kind: "mariadb", Root: root, EstimatedRowsRead: estimatedRowsRead, NativeCost: nativeCost,
		Issues: rollupIssues(&root), OverThreshold: isOverThreshold(estimatedRowsRead, thresholdRows),
		Raw: rawText,
	}, nil
}
