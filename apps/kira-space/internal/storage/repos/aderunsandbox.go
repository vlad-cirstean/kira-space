package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// AdeRunSandbox is one row of `ade_run_sandbox`: the policy a headless run started with. DenialsJSON
// is a JSON array of the first denials; DenialCount counts every one.
type AdeRunSandbox struct {
	RunID       string
	Access      string
	Enforced    bool
	Reason      string
	PolicyJSON  string
	Profile     string
	DenialCount int
	DenialsJSON string
}

// AdeRunSandboxRepo reads and writes `ade_run_sandbox`.
type AdeRunSandboxRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// Insert stores a run's sandbox. The run row exists already.
func (r *AdeRunSandboxRepo) Insert(s AdeRunSandbox) error {
	c := kiraschema.AdeRunSandbox
	ib := r.Dialect.InsertInto(c.Table).Cols(c.RunID, c.Access, c.Enforced, c.Reason, c.PolicyJSON, c.Profile).
		Values(s.RunID, s.Access, s.Enforced, s.Reason, s.PolicyJSON, s.Profile)
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: insert run sandbox %s: %w", s.RunID, err)
	}
	return nil
}

// Get returns a run's sandbox, (nil, nil) when it ran without one.
func (r *AdeRunSandboxRepo) Get(runID string) (*AdeRunSandbox, error) {
	c := kiraschema.AdeRunSandbox
	sb := r.Dialect.Select(c.RunID, c.Access, c.Enforced, c.Reason, c.PolicyJSON, c.Profile, c.DenialCount, c.DenialsJSON).From(c.Table)
	sb.Where(sb.Equal(c.RunID, runID))
	return adeOne(r.DB, sb, func(row *sql.Row) (*AdeRunSandbox, error) {
		var s AdeRunSandbox
		var enforced int
		err := row.Scan(&s.RunID, &s.Access, &enforced, &s.Reason, &s.PolicyJSON, &s.Profile, &s.DenialCount, &s.DenialsJSON)
		s.Enforced = enforced != 0
		return &s, err
	})
}

// Badges returns the light columns (no profile or policy) of the given runs' rows, keyed by run id.
func (r *AdeRunSandboxRepo) Badges(runIDs []string) (map[string]AdeRunSandbox, error) {
	out := map[string]AdeRunSandbox{}
	if len(runIDs) == 0 {
		return out, nil
	}
	c := kiraschema.AdeRunSandbox
	sb := r.Dialect.Select(c.RunID, c.Access, c.Enforced, c.DenialCount, c.DenialsJSON).From(c.Table)
	sb.Where(sb.In(c.RunID, sqlb.Args(runIDs)...))
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: run sandbox badges: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s AdeRunSandbox
		var enforced int
		if err := rows.Scan(&s.RunID, &s.Access, &enforced, &s.DenialCount, &s.DenialsJSON); err != nil {
			return nil, fmt.Errorf("repos: scan run sandbox badge: %w", err)
		}
		s.Enforced = enforced != 0
		out[s.RunID] = s
	}
	return out, rows.Err()
}

// AddDenials counts delta more denials; denialsJSON, when not empty, replaces the stored list.
func (r *AdeRunSandboxRepo) AddDenials(runID string, delta int, denialsJSON string) error {
	c := kiraschema.AdeRunSandbox
	ub := r.Dialect.Update(c.Table)
	sets := []string{c.DenialCount + " = " + c.DenialCount + " + " + ub.Var(delta)}
	if denialsJSON != "" {
		sets = append(sets, ub.Assign(c.DenialsJSON, denialsJSON))
	}
	ub.Set(sets...).Where(ub.Equal(c.RunID, runID))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: count sandbox denial %s: %w", runID, err)
	}
	return nil
}
