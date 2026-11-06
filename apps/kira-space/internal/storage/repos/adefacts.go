package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// AdeFactsRepo stores the ade integration/deployment memory: branch marks (tip last seen fully
// contained in a target or environment) and each environment's last deploy-script result.
type AdeFactsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// Marks returns every mark for the given branch ids, keyed by branch id.
func (r *AdeFactsRepo) Marks(branchIDs []string) (map[string][]model.AdeBranchMark, error) {
	out := make(map[string][]model.AdeBranchMark)
	if len(branchIDs) == 0 {
		return out, nil
	}
	c := kiraschema.AdeBranchMarks
	sb := r.Dialect.Select(c.BranchID, c.Kind, c.Name, c.MergedTip, c.Recorded, c.UpdatedAt).From(c.Table)
	sb.Where(sb.In(c.BranchID, sqlb.Args(branchIDs)...)).OrderBy(c.BranchID, c.Kind, c.Name)
	rows, err := sqlb.Query(r.DB, sb)
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
	c := kiraschema.AdeBranchMarks
	ib := r.Dialect.InsertInto(c.Table).Cols(c.BranchID, c.Kind, c.Name, c.MergedTip, c.Recorded, c.UpdatedAt).
		Values(m.BranchID, m.Kind, m.Name, m.MergedTip, m.Recorded, m.UpdatedAt)
	sqlb.UpsertSet(ib, []string{c.BranchID, c.Kind, c.Name}, []string{
		c.MergedTip + " = excluded." + c.MergedTip,
		c.Recorded + " = MAX(" + c.Recorded + ", excluded." + c.Recorded + ")",
		c.UpdatedAt + " = excluded." + c.UpdatedAt,
	}, "")
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: upsert ade mark: %w", err)
	}
	return nil
}

// DeleteMarksNotIn drops the marks of kind for codeRepoID's branches whose name is not in names
// (a target or environment removed from config).
func (r *AdeFactsRepo) DeleteMarksNotIn(kind, codeRepoID string, names []string) error {
	c, b := kiraschema.AdeBranchMarks, kiraschema.AdeTaskBranches
	branches := r.Dialect.Select(b.ID).From(b.Table)
	branches.Where(branches.Equal(b.CodeRepoID, codeRepoID))
	del := r.Dialect.DeleteFrom(c.Table)
	conds := []string{del.Equal(c.Kind, kind), del.In(c.BranchID, branches)}
	if len(names) > 0 {
		conds = append(conds, del.NotIn(c.Name, sqlb.Args(names)...))
	}
	del.Where(conds...)
	if _, err := sqlb.Exec(r.DB, del); err != nil {
		return fmt.Errorf("repos: delete ade marks: %w", err)
	}
	return nil
}

// DeleteEnvStateNotIn drops codeRepoID's environment states whose environment is not in names (an
// environment removed from config).
func (r *AdeFactsRepo) DeleteEnvStateNotIn(codeRepoID string, names []string) error {
	c := kiraschema.AdeEnvState
	del := r.Dialect.DeleteFrom(c.Table)
	conds := []string{del.Equal(c.CodeRepoID, codeRepoID)}
	if len(names) > 0 {
		conds = append(conds, del.NotIn(c.Env, sqlb.Args(names)...))
	}
	del.Where(conds...)
	if _, err := sqlb.Exec(r.DB, del); err != nil {
		return fmt.Errorf("repos: delete ade env state: %w", err)
	}
	return nil
}

// EnvStates returns codeRepoID's environment states keyed by environment name.
func (r *AdeFactsRepo) EnvStates(codeRepoID string) (map[string]model.AdeEnvState, error) {
	c := kiraschema.AdeEnvState
	sb := r.Dialect.Select(c.CodeRepoID, c.Env, c.Sha, c.PrevSha, c.Error, c.CheckedAt).From(c.Table)
	sb.Where(sb.Equal(c.CodeRepoID, codeRepoID))
	rows, err := sqlb.Query(r.DB, sb)
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
	c := kiraschema.AdeEnvState
	ib := r.Dialect.InsertInto(c.Table).Cols(c.CodeRepoID, c.Env, c.Sha, c.PrevSha, c.Error, c.CheckedAt).
		Values(s.CodeRepoID, s.Env, s.Sha, "", s.Error, s.CheckedAt)
	ex := func(col string) string { return "excluded." + col }
	sqlb.UpsertSet(ib, []string{c.CodeRepoID, c.Env}, []string{
		c.PrevSha + " = CASE WHEN " + ex(c.Error) + " = '' AND " + ex(c.Sha) + " <> '' AND " + c.Sha + " <> '' AND " + c.Sha +
			" <> " + ex(c.Sha) + " THEN " + c.Sha + " ELSE " + c.PrevSha + " END",
		c.Sha + " = CASE WHEN " + ex(c.Error) + " = '' AND " + ex(c.Sha) + " <> '' THEN " + ex(c.Sha) + " ELSE " + c.Sha + " END",
		c.Error + " = " + ex(c.Error),
		c.CheckedAt + " = " + ex(c.CheckedAt),
	}, "")
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: set ade env state: %w", err)
	}
	return nil
}
