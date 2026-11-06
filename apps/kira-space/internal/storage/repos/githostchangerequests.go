package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/huandu/go-sqlbuilder"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// GitHostChangeRequestsRepo reads and writes `git_host_change_requests` and `git_host_sync_state`:
// the persisted change request index of each code host repository, keyed by repo_key so two clones
// of one repository share rows.
type GitHostChangeRequestsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

var gitHostCRColumns = []string{
	kiraschema.GitHostChangeRequests.Number, kiraschema.GitHostChangeRequests.NodeID, kiraschema.GitHostChangeRequests.HeadRef,
	kiraschema.GitHostChangeRequests.HeadOwner, kiraschema.GitHostChangeRequests.HeadSha, kiraschema.GitHostChangeRequests.BaseRef,
	kiraschema.GitHostChangeRequests.State, kiraschema.GitHostChangeRequests.Draft, kiraschema.GitHostChangeRequests.Title,
	kiraschema.GitHostChangeRequests.URL, kiraschema.GitHostChangeRequests.ReviewDecision, kiraschema.GitHostChangeRequests.ChecksState,
	kiraschema.GitHostChangeRequests.CommentCount, kiraschema.GitHostChangeRequests.Assignees, kiraschema.GitHostChangeRequests.UpdatedAt,
	kiraschema.GitHostChangeRequests.SyncedAt,
}

// upsertChangeRequestSQL builds the row upsert once per call; the statement is prepared and run per row.
func (r *GitHostChangeRequestsRepo) upsertChangeRequestSQL() string {
	c := kiraschema.GitHostChangeRequests
	cols := append([]string{c.RepoKey}, gitHostCRColumns...)
	ib := r.Dialect.InsertInto(c.Table).Cols(cols...).Values(make([]any, len(cols))...)
	update := gitHostCRColumns[1:]
	changed := make([]string, 0, len(update))
	for _, col := range update {
		if col != c.SyncedAt {
			changed = append(changed, c.Table+"."+col+" IS NOT excluded."+col)
		}
	}
	sqlb.Upsert(ib, []string{c.RepoKey, c.Number}, update, strings.Join(changed, " OR "))
	q, _ := ib.Build()
	return q
}

func scanGitHostCR(rows *sql.Rows) (model.GitHostChangeRequest, bool, error) {
	var c model.GitHostChangeRequest
	var draft int
	var comments sql.NullInt64
	var assignees sql.NullString
	err := rows.Scan(&c.Number, &c.NodeID, &c.HeadRef, &c.HeadOwner, &c.HeadSha, &c.BaseRef, &c.State, &draft, &c.Title, &c.URL,
		&c.ReviewDecision, &c.ChecksState, &comments, &assignees, &c.UpdatedAt, &c.SyncedAt)
	c.Draft = draft != 0
	if comments.Valid {
		n := int(comments.Int64)
		c.CommentCount = &n
	}
	if assignees.Valid {
		if jerr := json.Unmarshal([]byte(assignees.String), &c.Assignees); jerr != nil && err == nil {
			err = fmt.Errorf("repos: decode assignees of change request %d: %w", c.Number, jerr)
		}
	}
	return c, true, err
}

// UpsertChangeRequests writes rows in one transaction and returns how many were new or differed
// from the stored row (synced_at aside); an identical row is left untouched.
func (r *GitHostChangeRequestsRepo) UpsertChangeRequests(repoKey string, rows []model.GitHostChangeRequest) (changed int, err error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return 0, fmt.Errorf("repos: begin upsert change requests: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.Prepare(r.upsertChangeRequestSQL())
	if err != nil {
		return 0, fmt.Errorf("repos: prepare upsert change requests: %w", err)
	}
	defer stmt.Close()
	for _, c := range rows {
		var comments any
		if c.CommentCount != nil {
			comments = *c.CommentCount
		}
		var assignees any
		if c.Assignees != nil {
			b, jerr := json.Marshal(c.Assignees)
			if jerr != nil {
				return 0, fmt.Errorf("repos: encode assignees of change request %s#%d: %w", repoKey, c.Number, jerr)
			}
			assignees = string(b)
		}
		res, err := stmt.Exec(repoKey, c.Number, c.NodeID, c.HeadRef, c.HeadOwner, c.HeadSha, c.BaseRef, c.State, c.Draft,
			c.Title, c.URL, c.ReviewDecision, c.ChecksState, comments, assignees, c.UpdatedAt, c.SyncedAt)
		if err != nil {
			return 0, fmt.Errorf("repos: upsert change request %s#%d: %w", repoKey, c.Number, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("repos: upsert change request %s#%d: %w", repoKey, c.Number, err)
		}
		changed += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("repos: commit upsert change requests: %w", err)
	}
	return changed, nil
}

// DeleteChangeRequest drops one row and reports whether it existed.
func (r *GitHostChangeRequestsRepo) DeleteChangeRequest(repoKey string, number int) (bool, error) {
	c := kiraschema.GitHostChangeRequests
	db := r.Dialect.DeleteFrom(c.Table)
	db.Where(db.Equal(c.RepoKey, repoKey), db.Equal(c.Number, number))
	res, err := sqlb.Exec(r.DB, db)
	if err != nil {
		return false, fmt.Errorf("repos: delete change request %s#%d: %w", repoKey, number, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("repos: delete change request %s#%d: %w", repoKey, number, err)
	}
	return n > 0, nil
}

// NewestPerBranch returns each head branch's newest row among those whose head lives in
// headOwner's repository, newest first.
func (r *GitHostChangeRequestsRepo) NewestPerBranch(repoKey, headOwner string) ([]model.GitHostChangeRequest, error) {
	c := kiraschema.GitHostChangeRequests
	inner := r.Dialect.Select(append(append([]string{}, gitHostCRColumns...),
		"row_number() OVER (PARTITION BY "+c.HeadRef+" ORDER BY "+c.UpdatedAt+" DESC, "+c.Number+" DESC) AS rn")...).From(c.Table)
	inner.Where(inner.Equal(c.RepoKey, repoKey), inner.Equal(c.HeadOwner, headOwner))
	outer := r.Dialect.Select(gitHostCRColumns...)
	outer.From(outer.BuilderAs(inner, "ranked")).Where(outer.Equal("rn", 1)).OrderBy(c.UpdatedAt+" DESC", c.Number+" DESC")
	rows, err := sqlb.Query(r.DB, outer)
	out, err := sqlitex.QueryAll(rows, err, scanGitHostCR)
	if err != nil {
		return nil, fmt.Errorf("repos: list change requests of %s: %w", repoKey, err)
	}
	return out, nil
}

// ChangeRequestsForBranch returns one head branch's rows, newest first.
func (r *GitHostChangeRequestsRepo) ChangeRequestsForBranch(repoKey, headOwner, headRef string) ([]model.GitHostChangeRequest, error) {
	c := kiraschema.GitHostChangeRequests
	sb := r.Dialect.Select(gitHostCRColumns...).From(c.Table)
	sb.Where(sb.Equal(c.RepoKey, repoKey), sb.Equal(c.HeadOwner, headOwner), sb.Equal(c.HeadRef, headRef)).
		OrderBy(c.UpdatedAt+" DESC", c.Number+" DESC")
	rows, err := sqlb.Query(r.DB, sb)
	out, err := sqlitex.QueryAll(rows, err, scanGitHostCR)
	if err != nil {
		return nil, fmt.Errorf("repos: list change requests of %s %s: %w", repoKey, headRef, err)
	}
	return out, nil
}

// OpenChangeRequests returns every open row of the repository, any head owner.
func (r *GitHostChangeRequestsRepo) OpenChangeRequests(repoKey string) ([]model.GitHostChangeRequest, error) {
	c := kiraschema.GitHostChangeRequests
	sb := r.Dialect.Select(gitHostCRColumns...).From(c.Table)
	sb.Where(sb.Equal(c.RepoKey, repoKey), sb.Equal(c.State, "open")).OrderBy(c.UpdatedAt+" DESC", c.Number+" DESC")
	rows, err := sqlb.Query(r.DB, sb)
	out, err := sqlitex.QueryAll(rows, err, scanGitHostCR)
	if err != nil {
		return nil, fmt.Errorf("repos: list open change requests of %s: %w", repoKey, err)
	}
	return out, nil
}

// SyncState returns the repository's sync state; false when never synced.
func (r *GitHostChangeRequestsRepo) SyncState(repoKey string) (model.GitHostSyncState, bool, error) {
	var s model.GitHostSyncState
	var complete int
	c := kiraschema.GitHostSyncState
	sb := r.Dialect.Select(c.Account, c.WatermarkUpdatedAt, c.Etag, c.LastFullSyncAt, c.LastDeltaAt, c.LastChecksAt,
		c.BackfillCursor, c.BackfillComplete, c.SchemaVersion, c.DeltaFloor).From(c.Table)
	sb.Where(sb.Equal(c.RepoKey, repoKey))
	err := sqlb.QueryRow(r.DB, sb).
		Scan(&s.Account, &s.WatermarkUpdatedAt, &s.ETag, &s.LastFullSyncAt, &s.LastDeltaAt, &s.LastChecksAt,
			&s.BackfillCursor, &complete, &s.SchemaVersion, &s.DeltaFloor)
	if err == sql.ErrNoRows {
		return model.GitHostSyncState{}, false, nil
	}
	if err != nil {
		return model.GitHostSyncState{}, false, fmt.Errorf("repos: read sync state of %s: %w", repoKey, err)
	}
	s.BackfillComplete = complete != 0
	return s, true, nil
}

// PutSyncState writes the repository's sync state.
func (r *GitHostChangeRequestsRepo) PutSyncState(repoKey string, s model.GitHostSyncState) error {
	c := kiraschema.GitHostSyncState
	ib := r.Dialect.InsertInto(c.Table).Cols(c.RepoKey, c.Account, c.WatermarkUpdatedAt, c.Etag, c.LastFullSyncAt, c.LastDeltaAt,
		c.LastChecksAt, c.BackfillCursor, c.BackfillComplete, c.SchemaVersion, c.UpdatedAt, c.DeltaFloor).
		Values(repoKey, s.Account, s.WatermarkUpdatedAt, s.ETag, s.LastFullSyncAt, s.LastDeltaAt, s.LastChecksAt,
			s.BackfillCursor, s.BackfillComplete, s.SchemaVersion, time.Now().UnixMilli(), s.DeltaFloor)
	sqlb.Upsert(ib, []string{c.RepoKey}, []string{c.Account, c.WatermarkUpdatedAt, c.Etag, c.LastFullSyncAt, c.LastDeltaAt,
		c.LastChecksAt, c.BackfillCursor, c.BackfillComplete, c.SchemaVersion, c.UpdatedAt, c.DeltaFloor}, "")
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: write sync state of %s: %w", repoKey, err)
	}
	return nil
}

// DropRepo removes the repository's rows and sync state in one transaction.
func (r *GitHostChangeRequestsRepo) DropRepo(repoKey string) error {
	return r.dropWhere(func(cond *sqlbuilder.Cond) string {
		return cond.Equal(kiraschema.GitHostChangeRequests.RepoKey, repoKey)
	})
}

// PruneIdle drops every repository with no sync state write since before.
func (r *GitHostChangeRequestsRepo) PruneIdle(before int64) error {
	return r.dropWhere(func(cond *sqlbuilder.Cond) string {
		s := kiraschema.GitHostSyncState
		idle := r.Dialect.Select(s.RepoKey).From(s.Table)
		idle.Where(idle.LessThan("max("+s.LastFullSyncAt+", "+s.LastDeltaAt+", "+s.LastChecksAt+", "+s.UpdatedAt+")", before))
		return cond.In(s.RepoKey, idle)
	})
}

// dropWhere runs both deletes with the same repo_key condition; cond builds it against each
// builder's own placeholders.
func (r *GitHostChangeRequestsRepo) dropWhere(cond func(*sqlbuilder.Cond) string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin drop change requests: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	// Rows first: the state table is what selects the repositories in the PruneIdle condition.
	cr := r.Dialect.DeleteFrom(kiraschema.GitHostChangeRequests.Table)
	cr.Where(cond(&cr.Cond))
	if _, err := sqlb.Exec(tx, cr); err != nil {
		return fmt.Errorf("repos: drop change requests: %w", err)
	}
	st := r.Dialect.DeleteFrom(kiraschema.GitHostSyncState.Table)
	st.Where(cond(&st.Cond))
	if _, err := sqlb.Exec(tx, st); err != nil {
		return fmt.Errorf("repos: drop sync state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit drop change requests: %w", err)
	}
	return nil
}
