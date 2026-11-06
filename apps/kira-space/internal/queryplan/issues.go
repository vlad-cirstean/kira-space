package queryplan

import (
	"fmt"
	"sort"
	"strings"
)

// commaFormat renders n the way planIssues.ts's toLocaleString() does for a plain integer-valued
// count: thousands-grouped, no decimal point (every row count this package formats is a whole
// number by construction — a JSON row estimate, never a fraction of a row).
func commaFormat(n float64) string {
	whole := int64(n)
	s := fmt.Sprintf("%d", whole)
	neg := false
	if whole < 0 {
		neg = true
		s = s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// pushWideScanIssueTo is pushWideScanIssue's own body, taking a bare *[]Issue and label rather
// than a *Node — clickhouse.go's own builder tree (not yet a Node at the point this must run,
// since the target node is picked after the whole tree is built) needs the same rule without a
// *Node to hang it on.
func pushWideScanIssueTo(issues *[]Issue, label string, rows float64, thresholdRows int) {
	if rows < float64(thresholdRows) {
		return
	}
	*issues = append(*issues, Issue{
		Severity: "warn",
		Code:     "wide-scan",
		Message:  fmt.Sprintf("%s is estimated to read %s %s.", label, commaFormat(rows), rowNoun(rows)),
	})
}

// pushWideScanIssue mirrors planIssues.ts's own — D14's "wide scan" rule: any scan-type node
// whose own row estimate meets/exceeds the threshold. Pushed onto that node directly (not the
// roll-up alone) so the tree view can point at exactly which node is the expensive one.
func pushWideScanIssue(node *Node, rows float64, thresholdRows int) {
	pushWideScanIssueTo(&node.Issues, node.Label, rows, thresholdRows)
}

// tableLabel mirrors mysqlTableLabel/mariadbTableLabel — MySQL and MariaDB's own EXPLAIN
// JSON name a table-level node identically ("Full scan on t" / "ref access on t" / bare name),
// even though the two engines' raw table structs otherwise stay separate (genuinely different
// schemas beyond this shared name/access_type pair).
func tableLabel(name, accessType *string) string {
	label := "?"
	if name != nil {
		label = *name
	}
	if accessType != nil && *accessType == "ALL" {
		return fmt.Sprintf("Full scan on %s", label)
	}
	if accessType != nil {
		return fmt.Sprintf("%s access on %s", *accessType, label)
	}
	return label
}

// pushIndexIssues mirrors the full-scan/unused-index issue pair MySQL and MariaDB's own
// per-table node builders push identically: relation is the resolved "?"-placeholder
// name already used for the node's other messages; accessType/possibleKeys/key are that engine's
// own raw table fields, read verbatim.
func pushIndexIssues(node *Node, relation string, accessType *string, possibleKeys []string, key *string) {
	if accessType != nil && *accessType == "ALL" {
		node.Issues = append(node.Issues, Issue{
			Severity: "warn", Code: "full-scan",
			Message: fmt.Sprintf("Full table scan on “%s”. No index was used.", relation),
		})
	}
	if len(possibleKeys) > 0 && key == nil {
		node.Issues = append(node.Issues, Issue{
			Severity: "warn", Code: "unused-index",
			Message: fmt.Sprintf("“%s” has an index (%s) the planner didn’t choose.", relation, strings.Join(possibleKeys, ", ")),
		})
	}
}

// maxEstimatedRows mirrors planIssues.ts's own — the widest single read among the dialect's own
// scan-type nodes. Takes bare row counts, not node references — see plan.go's own note on why.
func maxEstimatedRows(scans []float64) *float64 {
	if len(scans) == 0 {
		return nil
	}
	max := scans[0]
	for _, rows := range scans[1:] {
		if rows > max {
			max = rows
		}
	}
	return &max
}

// rollupIssues mirrors planIssues.ts's own — depth-first walk collecting every node's own issues
// into one deduplicated, order-preserving list (severity+code+message triple).
func rollupIssues(root *Node) []Issue {
	seen := map[string]bool{}
	out := make([]Issue, 0)
	var visit func(n *Node)
	visit = func(n *Node) {
		for _, issue := range n.Issues {
			key := issue.Severity + ":" + issue.Code + ":" + issue.Message
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, issue)
		}
		for i := range n.Children {
			visit(&n.Children[i])
		}
	}
	visit(root)
	return out
}

// isOverThreshold mirrors planIssues.ts's own — strictly the row-count threshold.
func isOverThreshold(estimatedRowsRead *float64, thresholdRows int) bool {
	return estimatedRowsRead != nil && *estimatedRowsRead >= float64(thresholdRows)
}

// sortMetricsByLabel is the shared helper: every parser emits its untyped/fallthrough metrics
// sorted by label, since a Go map has no insertion order to preserve the way
// Object.entries(raw) does on the TypeScript side. Declared non-contractual (plan.go's own doc
// comment) and sorted on both sides before the parity fixtures compare.
func sortMetricsByLabel(metrics []Metric) []Metric {
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].Label < metrics[j].Label })
	return metrics
}

// rowNoun matches the catalog's one/other plural split for "row".
func rowNoun(n float64) string {
	if n == 1 {
		return "row"
	}
	return "rows"
}
