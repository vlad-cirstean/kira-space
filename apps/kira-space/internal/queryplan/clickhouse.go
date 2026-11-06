package queryplan

import (
	"encoding/json"
	"fmt"
	"strings"
)

// clickhouse.go ports planParsers/clickhouse.ts verbatim: ClickHouse's `EXPLAIN PLAN json = 1,
// indexes = 1, description = 1` plus a second `EXPLAIN ESTIMATE` call. Neither reports a cost, so
// NativeCost stays unset; ESTIMATE's own row count is the only size figure available, and the
// plan's own Indexes[] entries are the only index-usage signal.

type chRawIndex struct {
	Type             *string  `json:"Type"`
	Keys             []string `json:"Keys"`
	Condition        *string  `json:"Condition"`
	SearchAlgorithm  *string  `json:"Search Algorithm"`
	InitialParts     *int     `json:"Initial Parts"`
	SelectedParts    *int     `json:"Selected Parts"`
	InitialGranules  *int     `json:"Initial Granules"`
	SelectedGranules *int     `json:"Selected Granules"`
}

type chRawNode struct {
	NodeType    *string           `json:"Node Type"`
	Description *string           `json:"Description"`
	Indexes     []chRawIndex      `json:"Indexes"`
	Plans       []json.RawMessage `json:"Plans"`
}

type estimateRow struct {
	rows float64
}

func chIndexMetrics(indexes []chRawIndex) []Metric {
	out := make([]Metric, 0)
	for _, idx := range indexes {
		typ := "Index"
		if idx.Type != nil {
			typ = *idx.Type
		}
		if idx.Keys != nil {
			out = append(out, Metric{Label: typ + " keys", Value: strings.Join(idx.Keys, ", ")})
		}
		if idx.Condition != nil {
			out = append(out, Metric{Label: typ + " condition", Value: *idx.Condition})
		}
		if idx.SearchAlgorithm != nil {
			out = append(out, Metric{Label: typ + " search", Value: *idx.SearchAlgorithm})
		}
		if idx.InitialParts != nil || idx.SelectedParts != nil {
			out = append(out, Metric{Label: typ + " parts", Value: fmt.Sprintf("%s / %s selected", intOrPlaceholder(idx.SelectedParts), intOrPlaceholder(idx.InitialParts))})
		}
		if idx.InitialGranules != nil || idx.SelectedGranules != nil {
			out = append(out, Metric{Label: typ + " granules", Value: fmt.Sprintf("%s / %s selected", intOrPlaceholder(idx.SelectedGranules), intOrPlaceholder(idx.InitialGranules))})
		}
	}
	return out
}

func intOrPlaceholder(v *int) string {
	if v == nil {
		return "?"
	}
	return fmt.Sprintf("%d", *v)
}

// chBuilderNode is a pointer-based scratch tree, the same trick sqlite.go's own sqliteBuilderNode
// uses and for the same reason: the "wide scan" issue is decided only after the whole tree is
// built (mergeTreeNodes[0] is picked once every ReadFromMergeTree call has run) and from a figure
// (EXPLAIN ESTIMATE's own row sum) no single node's own JSON carries, so it cannot be pushed
// inline the way postgres.go/mysql.go/mariadb.go push their own row-threshold issue. Building via
// pointers and converting to the value-typed Node tree exactly once, after that push, is what
// keeps the mutation from landing on an already-discarded copy.
type chBuilderNode struct {
	label    string
	relation string
	metrics  []Metric
	issues   []Issue
	children []*chBuilderNode
}

func finalizeChNode(b *chBuilderNode) Node {
	children := make([]Node, 0, len(b.children))
	for _, c := range b.children {
		children = append(children, finalizeChNode(c))
	}
	return Node{Label: b.label, Relation: b.relation, Metrics: b.metrics, Issues: b.issues, Children: children}
}

func chBuildNode(raw json.RawMessage, mergeTreeNodes *[]*chBuilderNode) *chBuilderNode {
	var typed chRawNode
	_ = json.Unmarshal(raw, &typed)

	nodeType := "Node"
	if typed.NodeType != nil {
		nodeType = *typed.NodeType
	}
	label := nodeType
	if typed.Description != nil {
		label = fmt.Sprintf("%s: %s", nodeType, *typed.Description)
	}

	var metrics []Metric
	if typed.Indexes != nil {
		metrics = chIndexMetrics(typed.Indexes)
	} else {
		metrics = []Metric{}
	}

	children := make([]*chBuilderNode, 0, len(typed.Plans))
	for _, childRaw := range typed.Plans {
		children = append(children, chBuildNode(childRaw, mergeTreeNodes))
	}

	b := &chBuilderNode{label: label, metrics: metrics, issues: []Issue{}, children: children}
	if nodeType == "ReadFromMergeTree" && typed.Description != nil {
		b.relation = *typed.Description
	}

	if nodeType == "ReadFromMergeTree" {
		chMergeTreeIssues(b, typed)
		*mergeTreeNodes = append(*mergeTreeNodes, b)
	}

	return b
}

// chMergeTreeIssues appends D15's own two ReadFromMergeTree issues to b.issues — a primary key
// that did not narrow the read, and every part of the table read — based on typed's own
// PrimaryKey index entry, when one is present.
func chMergeTreeIssues(b *chBuilderNode, typed chRawNode) {
	var pk *chRawIndex
	for i := range typed.Indexes {
		if typed.Indexes[i].Type != nil && *typed.Indexes[i].Type == "PrimaryKey" {
			pk = &typed.Indexes[i]
			break
		}
	}
	if pk == nil {
		return
	}
	relationOrPlaceholder := "?"
	if typed.Description != nil {
		relationOrPlaceholder = *typed.Description
	}
	// D15: the primary key did not narrow the read.
	if pk.SelectedGranules != nil && pk.InitialGranules != nil && *pk.SelectedGranules == *pk.InitialGranules {
		b.issues = append(b.issues, Issue{
			Severity: "warn", Code: "pk-not-narrowed",
			Message: fmt.Sprintf("The primary key on “%s” didn’t narrow the read. Every granule was selected.", relationOrPlaceholder),
		})
	}
	// D15: every part read.
	initialParts := 0
	if pk.InitialParts != nil {
		initialParts = *pk.InitialParts
	}
	if pk.SelectedParts != nil && pk.InitialParts != nil && *pk.SelectedParts == *pk.InitialParts && initialParts > 1 {
		b.issues = append(b.issues, Issue{
			Severity: "warn", Code: "all-parts-read",
			Message: fmt.Sprintf("Every part of “%s” was read (%d parts).", relationOrPlaceholder, initialParts),
		})
	}
}

func parseClickhousePlan(planRawText string, estimateRows []estimateRow, thresholdRows int) (Plan, error) {
	var parsed []struct {
		Plan json.RawMessage `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(planRawText), &parsed); err != nil {
		return Plan{}, fmt.Errorf("queryplan: parse clickhouse plan: %w", err)
	}
	var planRaw json.RawMessage
	if len(parsed) > 0 {
		planRaw = parsed[0].Plan
	} else {
		planRaw = json.RawMessage("{}")
	}

	mergeTreeNodes := make([]*chBuilderNode, 0)
	rootBuilder := chBuildNode(planRaw, &mergeTreeNodes)

	// D14: ClickHouse has no per-node row estimate at all — EXPLAIN ESTIMATE's own summed `rows`
	// is the only figure available, so the "wide read" issue (D15) is attached to the
	// ReadFromMergeTree node(s) the plan actually found rather than derived from the tree itself.
	estimatedRowsRead := 0.0
	for _, r := range estimateRows {
		estimatedRowsRead += r.rows
	}
	target := rootBuilder
	if len(mergeTreeNodes) > 0 {
		target = mergeTreeNodes[0]
	}
	pushWideScanIssueTo(&target.issues, target.label, estimatedRowsRead, thresholdRows)

	root := finalizeChNode(rootBuilder)

	raw := fmt.Sprintf("%s\n\n-- EXPLAIN ESTIMATE --\n%s", planRawText, mustMarshalEstimateRows(estimateRows))
	var estimatedRowsReadPtr *float64
	if len(estimateRows) > 0 {
		v := estimatedRowsRead
		estimatedRowsReadPtr = &v
	}
	overThreshold := len(estimateRows) > 0 && estimatedRowsRead >= float64(thresholdRows)
	return Plan{
		Kind: "clickhouse", Root: root, EstimatedRowsRead: estimatedRowsReadPtr, NativeCost: nil,
		Issues: rollupIssues(&root), OverThreshold: overThreshold, Raw: raw,
	}, nil
}

func mustMarshalEstimateRows(rows []estimateRow) string {
	type wire struct {
		Rows float64 `json:"rows"`
	}
	out := make([]wire, len(rows))
	for i, r := range rows {
		out[i] = wire{Rows: r.rows}
	}
	b, _ := json.Marshal(out)
	return string(b)
}
