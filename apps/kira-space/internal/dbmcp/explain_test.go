package dbmcp

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/queryplan"
)

// TestMaskPlanForMaskedConnectionStripsRealValues: MySQL/MariaDB's attached_condition
// (Node.Detail) and ClickHouse's index condition metrics substitute real row values evaluated from
// a const table, and Raw is the server's own EXPLAIN text verbatim — none of it is column-name
// maskable, so a masked connection's plan must come back with all of it stripped, at every depth.
func TestMaskPlanForMaskedConnectionStripsRealValues(t *testing.T) {
	plan := queryplan.Plan{
		Kind: "mysql",
		Root: queryplan.Node{
			Label:  "Query",
			Detail: "top-level detail",
			Metrics: []queryplan.Metric{
				{Label: "primary key condition", Value: "email = 'real@example.com'"},
				{Label: "rows_examined_per_scan", Value: "10"},
				{Label: "index_condition", Value: "((`d`.`o`.`status` > 'alice@secret.example'))"},
				{Label: "index_condition", Value: "o.status > 'alice@secret.example'"},
			},
			Children: []queryplan.Node{
				{
					Label:  "child",
					Detail: "customers.email = 'real@example.com'",
					Metrics: []queryplan.Metric{
						{Label: "minmax condition", Value: "id = 42"},
					},
				},
			},
		},
		Raw: "-- verbatim server EXPLAIN output, may embed real values --",
	}

	masked := maskPlanForMaskedConnection(plan)

	if masked.Raw != "" {
		t.Fatalf("Raw = %q, want empty on a masked connection", masked.Raw)
	}
	if masked.Root.Detail != "" {
		t.Fatalf("Root.Detail = %q, want empty", masked.Root.Detail)
	}
	if masked.Root.Children[0].Detail != "" {
		t.Fatalf("Children[0].Detail = %q, want empty", masked.Root.Children[0].Detail)
	}
	for _, m := range masked.Root.Metrics {
		if strings.HasSuffix(m.Label, "condition") {
			t.Fatalf("Root.Metrics still carries a condition metric: %+v", m)
		}
	}
	if len(masked.Root.Metrics) != 1 || masked.Root.Metrics[0].Label != "rows_examined_per_scan" {
		t.Fatalf("Root.Metrics = %+v, want only the non-condition metric kept", masked.Root.Metrics)
	}
	if len(masked.Root.Children[0].Metrics) != 0 {
		t.Fatalf("Children[0].Metrics = %+v, want the condition metric dropped", masked.Root.Children[0].Metrics)
	}
}
