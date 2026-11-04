package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const artifactCols = `id, task_id, kind, title, url, ref, source, step_id, run_id, created_at, updated_at, session_id`

func scanArtifact(row rowScanner) (model.AdeTaskArtifact, error) {
	var a model.AdeTaskArtifact
	err := row.Scan(&a.ID, &a.TaskID, &a.Kind, &a.Title, &a.URL, &a.Ref, &a.Source, &a.StepID, &a.RunID,
		&a.CreatedAt, &a.UpdatedAt, &a.SessionID)
	return a, err
}

// Artifacts lists the task's reported artifacts, oldest first.
func (r *AdeTaskRepo) Artifacts(taskID string) ([]model.AdeTaskArtifact, error) {
	rows, err := r.DB.Query(`SELECT `+artifactCols+` FROM ade_task_artifacts WHERE task_id = ?
		ORDER BY created_at, rowid`, taskID)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeTaskArtifact, bool, error) {
		a, err := scanArtifact(rows)
		return a, true, err
	})
}

// GetArtifact returns the row, (nil, nil) when it does not exist.
func (r *AdeTaskRepo) GetArtifact(id string) (*model.AdeTaskArtifact, error) {
	return sqlitex.QueryOne(r.DB, func(row *sql.Row) (*model.AdeTaskArtifact, error) {
		a, err := scanArtifact(row)
		return &a, err
	}, `SELECT `+artifactCols+` FROM ade_task_artifacts WHERE id = ?`, id)
}

// CountArtifacts returns how many artifacts the task has stored.
func (r *AdeTaskRepo) CountArtifacts(taskID string) (int, error) {
	var n int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM ade_task_artifacts WHERE task_id = ?`, taskID).Scan(&n); err != nil {
		return 0, fmt.Errorf("repos: count artifacts %s: %w", taskID, err)
	}
	return n, nil
}

// UpsertArtifact stores an artifact. A row with the same (task, url) updates kind, title, ref and
// updated_at in place; an empty URL always inserts. A new row takes a.ID.
func (r *AdeTaskRepo) UpsertArtifact(a model.AdeTaskArtifact) (model.AdeTaskArtifact, error) {
	if a.URL == "" {
		if _, err := r.DB.Exec(`INSERT INTO ade_task_artifacts
			(id, task_id, kind, title, url, ref, source, step_id, run_id, created_at, updated_at, session_id)
			VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, a.TaskID, a.Kind, a.Title, a.Ref, a.Source, a.StepID, a.RunID, a.CreatedAt, a.UpdatedAt, a.SessionID); err != nil {
			return model.AdeTaskArtifact{}, fmt.Errorf("repos: insert artifact %s: %w", a.TaskID, err)
		}
		return r.readArtifact(a.ID)
	}
	if _, err := r.DB.Exec(`INSERT INTO ade_task_artifacts
		(id, task_id, kind, title, url, ref, source, step_id, run_id, created_at, updated_at, session_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (task_id, url) WHERE url <> '' DO UPDATE SET
			kind = excluded.kind, title = excluded.title, ref = excluded.ref, updated_at = excluded.updated_at,
			session_id = CASE WHEN excluded.session_id <> '' THEN excluded.session_id ELSE ade_task_artifacts.session_id END`,
		a.ID, a.TaskID, a.Kind, a.Title, a.URL, a.Ref, a.Source, a.StepID, a.RunID, a.CreatedAt, a.UpdatedAt, a.SessionID); err != nil {
		return model.AdeTaskArtifact{}, fmt.Errorf("repos: upsert artifact %s: %w", a.TaskID, err)
	}
	var id string
	if err := r.DB.QueryRow(`SELECT id FROM ade_task_artifacts WHERE task_id = ? AND url = ?`, a.TaskID, a.URL).Scan(&id); err != nil {
		return model.AdeTaskArtifact{}, fmt.Errorf("repos: read artifact %s: %w", a.TaskID, err)
	}
	return r.readArtifact(id)
}

func (r *AdeTaskRepo) readArtifact(id string) (model.AdeTaskArtifact, error) {
	a, err := r.GetArtifact(id)
	if err != nil || a == nil {
		return model.AdeTaskArtifact{}, fmt.Errorf("repos: read artifact %s: %w", id, err)
	}
	return *a, nil
}

// UpdateArtifact rewrites a row's kind, title, url and ref and returns it.
func (r *AdeTaskRepo) UpdateArtifact(taskID, id, kind, title, url, ref string, now int64) (model.AdeTaskArtifact, error) {
	res, err := r.DB.Exec(`UPDATE ade_task_artifacts SET kind = ?, title = ?, url = ?, ref = ?, updated_at = ? WHERE id = ? AND task_id = ?`,
		kind, title, url, ref, now, id, taskID)
	if err != nil {
		return model.AdeTaskArtifact{}, fmt.Errorf("repos: update artifact %s: %w", id, err)
	}
	if err := sqlitex.RequireOneRow(res, "artifact "+id); err != nil {
		return model.AdeTaskArtifact{}, err
	}
	return r.readArtifact(id)
}

// DeleteArtifact removes a row.
func (r *AdeTaskRepo) DeleteArtifact(id string) error {
	res, err := r.DB.Exec(`DELETE FROM ade_task_artifacts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("repos: delete artifact %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "artifact "+id)
}
