package repos

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// AdeFactsRepo stores the ade integration/deployment memory: branch marks (tip last seen fully
// contained in a target or environment) and each environment's last deploy-script result.
type AdeFactsRepo struct {
	DB *sql.DB
}

// Marks returns every mark for the given branch ids, keyed by branch id.
func (r *AdeFactsRepo) Marks(branchIDs []string) (map[string][]model.AdeBranchMark, error) {
	out := make(map[string][]model.AdeBranchMark)
	if len(branchIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(branchIDs))
	for i, id := range branchIDs {
		args[i] = id
	}
	rows, err := r.DB.Query(`SELECT branch_id, kind, name, merged_tip, recorded, updated_at FROM ade_branch_marks
		WHERE branch_id IN (?`+strings.Repeat(",?", len(branchIDs)-1)+`) ORDER BY branch_id, kind, name`, args...)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade marks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m model.AdeBranchMark
		var rec int
		if err := rows.Scan(&m.BranchID, &m.Kind, &m.Name, &m.MergedTip, &rec, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("repos: scan ade mark: %w", err)
		}
		m.Recorded = rec != 0
		out[m.BranchID] = append(out[m.BranchID], m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade mark rows: %w", err)
	}
	return out, nil
}

// UpsertMark records the contained tip. recorded only ever rises: an automatic mark never clears a
// recorded one, and recorded=false leaves an existing flag alone.
func (r *AdeFactsRepo) UpsertMark(m model.AdeBranchMark) error {
	if _, err := r.DB.Exec(`INSERT INTO ade_branch_marks (branch_id, kind, name, merged_tip, recorded, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(branch_id, kind, name) DO UPDATE SET merged_tip = excluded.merged_tip,
		  recorded = MAX(recorded, excluded.recorded), updated_at = excluded.updated_at`,
		m.BranchID, m.Kind, m.Name, m.MergedTip, boolInt(m.Recorded), m.UpdatedAt); err != nil {
		return fmt.Errorf("repos: upsert ade mark: %w", err)
	}
	return nil
}

// DeleteMarksNotIn drops the marks of kind for codeRepoID's branches whose name is not in names
// (a target or environment removed from config).
func (r *AdeFactsRepo) DeleteMarksNotIn(kind, codeRepoID string, names []string) error {
	q := `DELETE FROM ade_branch_marks WHERE kind = ? AND branch_id IN (SELECT id FROM ade_task_branches WHERE code_repo_id = ?)`
	args := []any{kind, codeRepoID}
	if len(names) > 0 {
		q += ` AND name NOT IN (?` + strings.Repeat(",?", len(names)-1) + `)`
		for _, n := range names {
			args = append(args, n)
		}
	}
	if _, err := r.DB.Exec(q, args...); err != nil {
		return fmt.Errorf("repos: delete ade marks: %w", err)
	}
	return nil
}

// DeleteEnvStateNotIn drops codeRepoID's environment states whose environment is not in names (an
// environment removed from config).
func (r *AdeFactsRepo) DeleteEnvStateNotIn(codeRepoID string, names []string) error {
	q := `DELETE FROM ade_env_state WHERE code_repo_id = ?`
	args := []any{codeRepoID}
	if len(names) > 0 {
		q += ` AND env NOT IN (?` + strings.Repeat(",?", len(names)-1) + `)`
		for _, n := range names {
			args = append(args, n)
		}
	}
	if _, err := r.DB.Exec(q, args...); err != nil {
		return fmt.Errorf("repos: delete ade env state: %w", err)
	}
	return nil
}

// EnvStates returns codeRepoID's environment states keyed by environment name.
func (r *AdeFactsRepo) EnvStates(codeRepoID string) (map[string]model.AdeEnvState, error) {
	rows, err := r.DB.Query(`SELECT code_repo_id, env, sha, prev_sha, error, checked_at FROM ade_env_state WHERE code_repo_id = ?`, codeRepoID)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade env state: %w", err)
	}
	defer rows.Close()
	out := make(map[string]model.AdeEnvState)
	for rows.Next() {
		var s model.AdeEnvState
		if err := rows.Scan(&s.CodeRepoID, &s.Env, &s.Sha, &s.PrevSha, &s.Error, &s.CheckedAt); err != nil {
			return nil, fmt.Errorf("repos: scan ade env state: %w", err)
		}
		out[s.Env] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade env state rows: %w", err)
	}
	return out, nil
}

// SetEnvState stores a script result. On success (sha non-empty, no error) prev_sha takes the old
// sha when it changed; on failure the last good sha is kept and only error/checked_at move.
func (r *AdeFactsRepo) SetEnvState(s model.AdeEnvState) error {
	if _, err := r.DB.Exec(`INSERT INTO ade_env_state (code_repo_id, env, sha, prev_sha, error, checked_at)
		VALUES (?, ?, ?, '', ?, ?)
		ON CONFLICT(code_repo_id, env) DO UPDATE SET
		  prev_sha = CASE WHEN excluded.error = '' AND excluded.sha <> '' AND sha <> '' AND sha <> excluded.sha THEN sha ELSE prev_sha END,
		  sha = CASE WHEN excluded.error = '' AND excluded.sha <> '' THEN excluded.sha ELSE sha END,
		  error = excluded.error, checked_at = excluded.checked_at`,
		s.CodeRepoID, s.Env, s.Sha, s.Error, s.CheckedAt); err != nil {
		return fmt.Errorf("repos: set ade env state: %w", err)
	}
	return nil
}
