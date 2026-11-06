// Package maskreview is the column review state of masking (P534): which live columns neither a mask
// rule covers nor the user reviewed, so the DB MCP server withholds them and the user is told.
package maskreview

import (
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

// Applies reports whether column review exists for kind. Kinds without fixed columns (mongodb, redis,
// kafka, s3, sqs) or without a column listing (kibana) are skipped.
func Applies(kind string) bool {
	switch kind {
	case "postgres", "mysql", "mariadb", "clickhouse", "sqlite":
		return true
	}
	return false
}

// LiveColumn is one column of the live schema.
type LiveColumn struct {
	Ref          tableref.ColumnRef
	DataType     string
	RelationKind string // table, view, matview
}

// Covered reports whether col is masked by a rule or reviewed as clear. A rule covers a column when its
// column name matches ignoring case and its table is * or the column's table name. reviewed holds
// tableref.ColumnKey values.
func Covered(kind string, col tableref.ColumnRef, rules []model.MaskRule, reviewed map[string]bool) bool {
	for _, r := range rules {
		if strings.EqualFold(strings.TrimSpace(r.ColumnName), col.Column) &&
			(r.TableName == "*" || strings.EqualFold(r.TableName, col.Name)) {
			return true
		}
	}
	return reviewed[tableref.ColumnKey(kind, col)]
}

// ReviewedKeys parses reviewed entries into the key set Covered reads; an entry that does not parse for
// kind is dropped (validation already refused it in the file).
func ReviewedKeys(kind string, entries []string) map[string]bool {
	out := make(map[string]bool, len(entries))
	for _, e := range entries {
		if c, err := tableref.ParseColumn(kind, e); err == nil {
			out[tableref.ColumnKey(kind, c)] = true
		}
	}
	return out
}

// Evaluation is the diff of the live columns against rules and the reviewed set.
type Evaluation struct {
	// Pending is the canonical refs of columns not covered, sorted.
	Pending []string
	// Names is the lowercased column names of Pending.
	Names map[string]bool
}

// Evaluate computes the pending columns among live. tables restricts them to the P532 allow list; nil
// allows every table.
func Evaluate(kind string, live []LiveColumn, rules []model.MaskRule, reviewed map[string]bool, tables *tableref.Set) Evaluation {
	out := Evaluation{Pending: []string{}, Names: map[string]bool{}}
	for _, c := range live {
		if !tables.Allows(c.Ref.Ref) || Covered(kind, c.Ref, rules, reviewed) {
			continue
		}
		out.Pending = append(out.Pending, tableref.FormatColumn(kind, c.Ref))
		out.Names[strings.ToLower(c.Ref.Column)] = true
	}
	sort.Strings(out.Pending)
	return out
}
