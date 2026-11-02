package repos

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// GitHostChangeRequestsRepo reads and writes `git_host_change_requests` and `git_host_sync_state`:
// the persisted change request index of each code host repository, keyed by repo_key so two clones
// of one repository share rows.
type GitHostChangeRequestsRepo struct {
	DB *sql.DB
}

const gitHostCRColumns = `number, node_id, head_ref, head_owner, head_sha, base_ref, state, draft, title, url,
	review_decision, checks_state, updated_at, synced_at`

func scanGitHostCR(rows *sql.Rows) (model.GitHostChangeRequest, bool, error) {
	var c model.GitHostChangeRequest
	var draft int
	err := rows.Scan(&c.Number, &c.NodeID, &c.HeadRef, &c.HeadOwner, &c.HeadSha, &c.BaseRef, &c.State, &draft, &c.Title, &c.URL,
		&c.ReviewDecision, &c.ChecksState, &c.UpdatedAt, &c.SyncedAt)
	c.Draft = draft != 0
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
	stmt, err := tx.Prepare(`INSERT INTO git_host_change_requests (repo_key, ` + gitHostCRColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (repo_key, number) DO UPDATE SET
			node_id = excluded.node_id, head_ref = excluded.head_ref, head_owner = excluded.head_owner,
			head_sha = excluded.head_sha, base_ref = excluded.base_ref, state = excluded.state, draft = excluded.draft,
			title = excluded.title, url = excluded.url, review_decision = excluded.review_decision,
			checks_state = excluded.checks_state, updated_at = excluded.updated_at, synced_at = excluded.synced_at
		WHERE git_host_change_requests.node_id IS NOT excluded.node_id OR git_host_change_requests.head_ref IS NOT excluded.head_ref
			OR git_host_change_requests.head_owner IS NOT excluded.head_owner OR git_host_change_requests.head_sha IS NOT excluded.head_sha
			OR git_host_change_requests.base_ref IS NOT excluded.base_ref OR git_host_change_requests.state IS NOT excluded.state
			OR git_host_change_requests.draft IS NOT excluded.draft OR git_host_change_requests.title IS NOT excluded.title
			OR git_host_change_requests.url IS NOT excluded.url OR git_host_change_requests.review_decision IS NOT excluded.review_decision
			OR git_host_change_requests.checks_state IS NOT excluded.checks_state OR git_host_change_requests.updated_at IS NOT excluded.updated_at`)
	if err != nil {
		return 0, fmt.Errorf("repos: prepare upsert change requests: %w", err)
	}
	defer stmt.Close()
	for _, c := range rows {
		res, err := stmt.Exec(repoKey, c.Number, c.NodeID, c.HeadRef, c.HeadOwner, c.HeadSha, c.BaseRef, c.State, boolInt(c.Draft),
			c.Title, c.URL, c.ReviewDecision, c.ChecksState, c.UpdatedAt, c.SyncedAt)
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
	res, err := r.DB.Exec(`DELETE FROM git_host_change_requests WHERE repo_key = ? AND number = ?`, repoKey, number)
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
	rows, err := r.DB.Query(`SELECT `+gitHostCRColumns+` FROM (
			SELECT `+gitHostCRColumns+`, row_number() OVER (PARTITION BY head_ref ORDER BY updated_at DESC, number DESC) AS rn
			FROM git_host_change_requests WHERE repo_key = ? AND head_owner = ?)
		WHERE rn = 1 ORDER BY updated_at DESC, number DESC`, repoKey, headOwner)
	out, err := sqlitex.QueryAll(rows, err, scanGitHostCR)
	if err != nil {
		return nil, fmt.Errorf("repos: list change requests of %s: %w", repoKey, err)
	}
	return out, nil
}

// ChangeRequestsForBranch returns one head branch's rows, newest first.
func (r *GitHostChangeRequestsRepo) ChangeRequestsForBranch(repoKey, headOwner, headRef string) ([]model.GitHostChangeRequest, error) {
	rows, err := r.DB.Query(`SELECT `+gitHostCRColumns+` FROM git_host_change_requests
		WHERE repo_key = ? AND head_owner = ? AND head_ref = ? ORDER BY updated_at DESC, number DESC`, repoKey, headOwner, headRef)
	out, err := sqlitex.QueryAll(rows, err, scanGitHostCR)
	if err != nil {
		return nil, fmt.Errorf("repos: list change requests of %s %s: %w", repoKey, headRef, err)
	}
	return out, nil
}

// OpenChangeRequests returns every open row of the repository, any head owner.
func (r *GitHostChangeRequestsRepo) OpenChangeRequests(repoKey string) ([]model.GitHostChangeRequest, error) {
	rows, err := r.DB.Query(`SELECT `+gitHostCRColumns+` FROM git_host_change_requests
		WHERE repo_key = ? AND state = 'open' ORDER BY updated_at DESC, number DESC`, repoKey)
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
	err := r.DB.QueryRow(`SELECT account, watermark_updated_at, etag, last_full_sync_at, last_delta_at, last_checks_at,
		backfill_cursor, backfill_complete, schema_version, delta_floor FROM git_host_sync_state WHERE repo_key = ?`, repoKey).
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
	if _, err := r.DB.Exec(`INSERT INTO git_host_sync_state (repo_key, account, watermark_updated_at, etag, last_full_sync_at,
		last_delta_at, last_checks_at, backfill_cursor, backfill_complete, schema_version, updated_at, delta_floor) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (repo_key) DO UPDATE SET account = excluded.account, watermark_updated_at = excluded.watermark_updated_at,
			etag = excluded.etag, last_full_sync_at = excluded.last_full_sync_at, last_delta_at = excluded.last_delta_at,
			last_checks_at = excluded.last_checks_at, backfill_cursor = excluded.backfill_cursor,
			backfill_complete = excluded.backfill_complete, schema_version = excluded.schema_version,
			updated_at = excluded.updated_at, delta_floor = excluded.delta_floor`,
		repoKey, s.Account, s.WatermarkUpdatedAt, s.ETag, s.LastFullSyncAt, s.LastDeltaAt, s.LastChecksAt,
		s.BackfillCursor, boolInt(s.BackfillComplete), s.SchemaVersion, time.Now().UnixMilli(), s.DeltaFloor); err != nil {
		return fmt.Errorf("repos: write sync state of %s: %w", repoKey, err)
	}
	return nil
}

// DropRepo removes the repository's rows and sync state in one transaction.
func (r *GitHostChangeRequestsRepo) DropRepo(repoKey string) error {
	return r.dropWhere(`repo_key = ?`, repoKey)
}

// PruneIdle drops every repository with no sync state write since before.
func (r *GitHostChangeRequestsRepo) PruneIdle(before int64) error {
	return r.dropWhere(`repo_key IN (SELECT repo_key FROM git_host_sync_state
		WHERE max(last_full_sync_at, last_delta_at, last_checks_at, updated_at) < ?)`, before)
}

func (r *GitHostChangeRequestsRepo) dropWhere(cond string, args ...any) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin drop change requests: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	// Rows first: the state table is what selects the repositories in the PruneIdle condition.
	if _, err := tx.Exec(`DELETE FROM git_host_change_requests WHERE `+cond, args...); err != nil {
		return fmt.Errorf("repos: drop change requests: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM git_host_sync_state WHERE `+cond, args...); err != nil {
		return fmt.Errorf("repos: drop sync state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit drop change requests: %w", err)
	}
	return nil
}
