package dbmcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/queryplan"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Limit names. P529 and P530 implement them.
const (
	LimitStatementTimeout = "statement_timeout"
	LimitExpensiveQuery   = "expensive_query"
)

// Units of LimitHit.Measured and LimitHit.Max.
const (
	UnitMillis = "ms"
	UnitRows   = "rows"
)

// LimitHit is one limit a statement went over. Numbers only: never row data, never adapter text.
type LimitHit struct {
	Limit    string
	Measured float64
	Max      float64
	Unit     string
}

// LimitCall is what a limit sees of one run_query call. Exempt is true when the user approved lifting this
// limit for this call.
type LimitCall struct {
	Summary model.ConnectionSummary
	Mode    string
	Class   adapters.OpClass
	SQL     string
	Path    string
	Exempt  bool
	// Plan, PlanErr and Threshold are filled once per call by the server; limits never run statements.
	// Plan is nil when the call was not planned or the plan has no estimate source.
	Plan      *queryplan.Plan
	PlanErr   error
	Threshold int
}

// Limit is one enforced run_query limit. Implement PreLimit, ExecLimit or both.
type Limit interface{ Name() string }

// PreLimit checks before any approval or execution. The server never calls Check for an exempt limit.
// A nil hit passes. An error fails the call (toolError, masked on a masked connection).
type PreLimit interface {
	Limit
	Check(ctx context.Context, c LimitCall) (*LimitHit, error)
}

// ExecLimit acts at execution. Apply runs just before Execute/ExecuteRead, for exempt calls too (c.Exempt
// set), and may change req and derive ctx; done releases the derived ctx. Hit maps an Execute error to this
// limit; the server calls it only when the limit is not exempt.
type ExecLimit interface {
	Limit
	Apply(ctx context.Context, c LimitCall, req *adapterhost.ExecuteRequestWire) (context.Context, func())
	Hit(c LimitCall, err error, elapsed time.Duration) *LimitHit
}

// LimitError is a run_query call refused by limits. Error() is the agent-facing text.
type LimitError struct {
	ConnectionName string
	Hits           []LimitHit
	ExemptionID    string
	// Browse words the hints for a browse_table call.
	Browse bool
}

func (h LimitHit) describe() string {
	return fmt.Sprintf("measured %s %s, limit %s %s", fmtLimitNumber(h.Measured), h.Unit, fmtLimitNumber(h.Max), h.Unit)
}

func fmtLimitNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// Error words the refusal for the agent: no statement echo, no data. It names the `exemption` field so
// the agent learns the path exists.
func (e *LimitError) Error() string {
	names := make([]string, len(e.Hits))
	for i, h := range e.Hits {
		names[i] = h.Limit
	}
	var b strings.Builder
	if len(e.Hits) == 1 {
		fmt.Fprintf(&b, "refused by limit %s on %q: %s", names[0], e.ConnectionName, e.Hits[0].describe())
	} else {
		parts := make([]string, len(e.Hits))
		for i, h := range e.Hits {
			parts[i] = h.Limit + ": " + h.describe()
		}
		fmt.Fprintf(&b, "refused by limits %s on %q: %s", strings.Join(names, ", "), e.ConnectionName, strings.Join(parts, "; "))
	}
	if hasLimit(e.Hits, LimitExpensiveQuery) && e.Browse {
		b.WriteString(". expensive_query is the query plan's estimated rows read, not rows returned. Filter or order on an indexed column, or reduce scope")
	} else if hasLimit(e.Hits, LimitExpensiveQuery) {
		b.WriteString(". expensive_query is the query plan's estimated rows read, not rows returned; explain shows which scan. Filter on an indexed column or narrow the join")
	}
	if e.Browse {
		fmt.Fprintf(&b, `. Improve the call, or ask the user to lift the limit for this call only: repeat this exact browse_table call with exemption {"id":%q,"reason":"<one sentence why>"}. The id works once and expires in %s.`, e.ExemptionID, approvalWindow())
		return b.String()
	}
	fmt.Fprintf(&b, `. Improve the statement, or ask the user to lift the limit for this statement only: repeat this exact call with exemption {"id":%q,"reason":"<one sentence why>"}. The id works once and expires in %s.`, e.ExemptionID, approvalWindow())
	return b.String()
}

func hasLimit(hits []LimitHit, name string) bool {
	for _, h := range hits {
		if h.Limit == name {
			return true
		}
	}
	return false
}

// mergeHits is base plus the hits of add whose limit base lacks.
func mergeHits(base, add []LimitHit) []LimitHit {
	out := append([]LimitHit(nil), base...)
	for _, h := range add {
		if !hasLimit(out, h.Limit) {
			out = append(out, h)
		}
	}
	return out
}
