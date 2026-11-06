package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// AdeSessionSandbox is one row of `ade_session_sandbox`: the policy a manual step's interactive
// session started with. Fields mirror AdeRunSandbox, keyed by the session.
type AdeSessionSandbox struct {
	SessionID   string
	Access      string
	Enforced    bool
	Reason      string
	PolicyJSON  string
	Profile     string
	DenialCount int
	DenialsJSON string
}

// AdeSessionSandboxRepo reads and writes `ade_session_sandbox`.
type AdeSessionSandboxRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// Upsert stores a session's sandbox, replacing the row of an earlier start (a Take over). The
// session row exists already.
func (r *AdeSessionSandboxRepo) Upsert(s AdeSessionSandbox) error {
	enforced := 0
	if s.Enforced {
		enforced = 1
	}
	c := kiraschema.AdeSessionSandbox
	ib := r.Dialect.InsertInto(c.Table).Cols(c.SessionID, c.Access, c.Enforced, c.Reason, c.PolicyJSON, c.Profile).
		Values(s.SessionID, s.Access, enforced, s.Reason, s.PolicyJSON, s.Profile)
	sqlb.UpsertSet(ib, []string{c.SessionID}, []string{
		c.Access + " = excluded." + c.Access, c.Enforced + " = excluded." + c.Enforced, c.Reason + " = excluded." + c.Reason,
		c.PolicyJSON + " = excluded." + c.PolicyJSON, c.Profile + " = excluded." + c.Profile,
		c.DenialCount + " = 0", c.DenialsJSON + " = '[]'",
	}, "")
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: upsert session sandbox %s: %w", s.SessionID, err)
	}
	return nil
}

// Delete drops a session's sandbox row; a session restarted with the sandbox off keeps none.
func (r *AdeSessionSandboxRepo) Delete(sessionID string) error {
	c := kiraschema.AdeSessionSandbox
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(del.Equal(c.SessionID, sessionID))
	if _, err := sqlb.Exec(r.DB, del); err != nil {
		return fmt.Errorf("repos: delete session sandbox %s: %w", sessionID, err)
	}
	return nil
}

// Get returns a session's sandbox, (nil, nil) when it ran without one.
func (r *AdeSessionSandboxRepo) Get(sessionID string) (*AdeSessionSandbox, error) {
	c := kiraschema.AdeSessionSandbox
	sb := r.Dialect.Select(c.SessionID, c.Access, c.Enforced, c.Reason, c.PolicyJSON, c.Profile, c.DenialCount, c.DenialsJSON).From(c.Table)
	sb.Where(sb.Equal(c.SessionID, sessionID))
	return adeOne(r.DB, sb, func(row *sql.Row) (*AdeSessionSandbox, error) {
		var s AdeSessionSandbox
		var enforced int
		err := row.Scan(&s.SessionID, &s.Access, &enforced, &s.Reason, &s.PolicyJSON, &s.Profile, &s.DenialCount, &s.DenialsJSON)
		s.Enforced = enforced != 0
		return &s, err
	})
}

// Badges returns the light columns of the given sessions' rows, keyed by session id.
func (r *AdeSessionSandboxRepo) Badges(sessionIDs []string) (map[string]AdeSessionSandbox, error) {
	out := map[string]AdeSessionSandbox{}
	if len(sessionIDs) == 0 {
		return out, nil
	}
	c := kiraschema.AdeSessionSandbox
	sb := r.Dialect.Select(c.SessionID, c.Access, c.Enforced, c.Reason, c.DenialCount, c.DenialsJSON).From(c.Table)
	sb.Where(sb.In(c.SessionID, sqlb.Args(sessionIDs)...))
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: session sandbox badges: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s AdeSessionSandbox
		var enforced int
		if err := rows.Scan(&s.SessionID, &s.Access, &enforced, &s.Reason, &s.DenialCount, &s.DenialsJSON); err != nil {
			return nil, fmt.Errorf("repos: scan session sandbox badge: %w", err)
		}
		s.Enforced = enforced != 0
		out[s.SessionID] = s
	}
	return out, rows.Err()
}

// AddDenials counts delta more denials; denialsJSON, when not empty, replaces the stored list.
func (r *AdeSessionSandboxRepo) AddDenials(sessionID string, delta int, denialsJSON string) error {
	c := kiraschema.AdeSessionSandbox
	ub := r.Dialect.Update(c.Table)
	ub.Set(c.DenialCount + " = " + c.DenialCount + " + " + ub.Var(delta)).Where(ub.Equal(c.SessionID, sessionID))
	if denialsJSON != "" {
		ub.SetMore(ub.Assign(c.DenialsJSON, denialsJSON))
	}
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return fmt.Errorf("repos: count session sandbox denial %s: %w", sessionID, err)
	}
	return nil
}
