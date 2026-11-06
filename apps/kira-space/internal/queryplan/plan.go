// Package queryplan is the Go port of the console's EXPLAIN plan normalizer
// (frontend/src/views/console/plan.ts, planModel.ts, planIssues.ts, planParsers/*.ts).
// It is a port, not a bridge: every function
// involved is pure data transformation over the page each dialect's EXPLAIN returns, so this
// package imports internal/page and stdlib only — never internal/dbmcp, never internal/adapters.
//
// The two implementations are pinned to the same behaviour by a shared fixture set,
// tests/fixtures/explain-plans/*.{input,expected}.json, read by both this package's own
// parse_test.go and tests/unit/explain-plan.spec.ts's own parity block — a drift in either
// language fails a test in that language, on the same bytes.
package queryplan

// CostUnit is the unit a dialect's own native cost figure is reported in — never compared across
// dialects (planModel.ts's own note: MariaDB's and MySQL's identically-named `cost` field
// disagree by three orders of magnitude for a comparable scan). "none" covers SQLite and
// ClickHouse, which report no cost.
type CostUnit string

const (
	CostUnitPostgresPlanner CostUnit = "postgres-planner"
	CostUnitMysqlCost       CostUnit = "mysql-cost"
	CostUnitMariadbCost     CostUnit = "mariadb-cost"
	CostUnitNone            CostUnit = "none"
)

// Issue mirrors planModel.ts's PlanIssue.
type Issue struct {
	Severity string `json:"severity"` // "warn" | "info"
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// Metric mirrors planModel.ts's PlanNode.metrics entry — every field a dialect reported that this
// model has no typed slot for, projected as a label/value pair. Go decodes each node twice
// (once into a typed struct, once into a map) since a Go map has no insertion order the way
// Object.entries(raw) does on the TypeScript side; the untyped keys are emitted sorted by label
// instead, through parse.go's own shared helper. That order is deliberately non-contractual — see
// this field's own note below.
type Metric struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Cost mirrors planModel.ts's PlanNode.cost.
type Cost struct {
	Total   float64  `json:"total"`
	Startup *float64 `json:"startup,omitempty"`
}

// Node mirrors planModel.ts's PlanNode, field for field. EstimatedRows is float64, not int:
// Postgres's "Plan Rows" and MariaDB's "rows" are JSON numbers the TypeScript side keeps as
// `number`, and rounding them in Go would make the two implementations disagree on a fixture.
// Metrics, Issues and Children are always non-nil slices so they marshal as `[]`, never `null`.
type Node struct {
	Label         string   `json:"label"`
	Relation      string   `json:"relation,omitempty"`
	Detail        string   `json:"detail,omitempty"`
	EstimatedRows *float64 `json:"estimatedRows,omitempty"`
	Cost          *Cost    `json:"cost,omitempty"`
	// Metrics' own order is not contractual (§2.3) — sorted by label, not the server's own
	// response order. A client that needs the server's exact field order reads Raw instead, which
	// is verbatim.
	Metrics  []Metric `json:"metrics"`
	Issues   []Issue  `json:"issues"`
	Children []Node   `json:"children"`
}

// NativeCost mirrors planModel.ts's QueryPlan.nativeCost.
type NativeCost struct {
	Value float64  `json:"value"`
	Unit  CostUnit `json:"unit"`
}

// Plan mirrors planModel.ts's QueryPlan, field for field.
type Plan struct {
	Kind              string      `json:"kind"`
	Root              Node        `json:"root"`
	EstimatedRowsRead *float64    `json:"estimatedRowsRead,omitempty"`
	NativeCost        *NativeCost `json:"nativeCost,omitempty"`
	// Issues is the whole-plan roll-up of every node's own issues, deduplicated by code — see
	// issues.go's rollupIssues.
	Issues        []Issue `json:"issues"`
	OverThreshold bool    `json:"overThreshold"`
	Raw           string  `json:"raw,omitempty"`
}

// Unlike planModel.ts's ScanEstimate (a {node, rows} pair kept so a later pass can push the
// "wide scan" issue onto the exact node object reference — free in JS, where every object is a
// reference), each Go parser pushes that issue inline while a scan node is still the function's
// own local value, before it is copied into its parent's Children slice, and only threads the
// bare row counts ([]float64) through to compute the plan-level estimatedRowsRead/overThreshold
// (issues.go's maxEstimatedRows/isOverThreshold) — mutating through a stored *Node here would be
// mutating a copy already left behind once that Node's value is appended into its parent.
