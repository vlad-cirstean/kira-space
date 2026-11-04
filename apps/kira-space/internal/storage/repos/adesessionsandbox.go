package repos

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/internal/sqlitex"
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
	DB *sql.DB
}

// Upsert stores a session's sandbox, replacing the row of an earlier start (a Take over). The
// session row exists already.
func (r *AdeSessionSandboxRepo) Upsert(s AdeSessionSandbox) error {
	enforced := 0
	if s.Enforced {
		enforced = 1
	}
	if _, err := r.DB.Exec(
		`INSERT INTO ade_session_sandbox (session_id, access, enforced, reason, policy_json, profile) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (session_id) DO UPDATE SET access = excluded.access, enforced = excluded.enforced, reason = excluded.reason,
		   policy_json = excluded.policy_json, profile = excluded.profile, denial_count = 0, denials_json = '[]'`,
		s.SessionID, s.Access, enforced, s.Reason, s.PolicyJSON, s.Profile,
	); err != nil {
		return fmt.Errorf("repos: upsert session sandbox %s: %w", s.SessionID, err)
	}
	return nil
}

// Delete drops a session's sandbox row; a session restarted with the sandbox off keeps none.
func (r *AdeSessionSandboxRepo) Delete(sessionID string) error {
	if _, err := r.DB.Exec(`DELETE FROM ade_session_sandbox WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("repos: delete session sandbox %s: %w", sessionID, err)
	}
	return nil
}

// Get returns a session's sandbox, (nil, nil) when it ran without one.
func (r *AdeSessionSandboxRepo) Get(sessionID string) (*AdeSessionSandbox, error) {
	return sqlitex.QueryOne(r.DB, func(row *sql.Row) (*AdeSessionSandbox, error) {
		var s AdeSessionSandbox
		var enforced int
		err := row.Scan(&s.SessionID, &s.Access, &enforced, &s.Reason, &s.PolicyJSON, &s.Profile, &s.DenialCount, &s.DenialsJSON)
		s.Enforced = enforced != 0
		return &s, err
	}, `SELECT session_id, access, enforced, reason, policy_json, profile, denial_count, denials_json FROM ade_session_sandbox WHERE session_id = ?`, sessionID)
}

// Badges returns the light columns of the given sessions' rows, keyed by session id.
func (r *AdeSessionSandboxRepo) Badges(sessionIDs []string) (map[string]AdeSessionSandbox, error) {
	out := map[string]AdeSessionSandbox{}
	if len(sessionIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(sessionIDs))
	marks := make([]string, len(sessionIDs))
	for i, id := range sessionIDs {
		args[i], marks[i] = id, "?"
	}
	rows, err := r.DB.Query(`SELECT session_id, access, enforced, reason, denial_count, denials_json FROM ade_session_sandbox WHERE session_id IN (`+strings.Join(marks, ",")+`)`, args...)
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
	var err error
	if denialsJSON == "" {
		_, err = r.DB.Exec(`UPDATE ade_session_sandbox SET denial_count = denial_count + ? WHERE session_id = ?`, delta, sessionID)
	} else {
		_, err = r.DB.Exec(`UPDATE ade_session_sandbox SET denial_count = denial_count + ?, denials_json = ? WHERE session_id = ?`, delta, denialsJSON, sessionID)
	}
	if err != nil {
		return fmt.Errorf("repos: count session sandbox denial %s: %w", sessionID, err)
	}
	return nil
}
