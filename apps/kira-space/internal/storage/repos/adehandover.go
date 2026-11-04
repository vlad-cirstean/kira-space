package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const handoverCols = `id, task_id, from_step_id, from_step_name, to_step_id, to_step_name, body, source, state,
	finished, dismissed, edited, created_at, updated_at, session_id`

func scanHandover(row rowScanner) (model.AdeHandover, error) {
	var h model.AdeHandover
	err := row.Scan(&h.ID, &h.TaskID, &h.FromStepID, &h.FromStepName, &h.ToStepID, &h.ToStepName, &h.Body,
		&h.Source, &h.State, &h.Finished, &h.Dismissed, &h.Edited, &h.CreatedAt, &h.UpdatedAt, &h.SessionID)
	return h, err
}

func (r *AdeTaskRepo) queryHandovers(query string, args ...any) ([]model.AdeHandover, error) {
	rows, err := r.DB.Query(query, args...)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeHandover, bool, error) {
		h, err := scanHandover(rows)
		return h, true, err
	})
}

// Handovers lists the task's handover rows, newest first (an open draft sorts by its last write).
func (r *AdeTaskRepo) Handovers(taskID string) ([]model.AdeHandover, error) {
	return r.queryHandovers(`SELECT `+handoverCols+` FROM ade_task_handovers WHERE task_id = ?
		ORDER BY CASE state WHEN 'draft' THEN updated_at ELSE created_at END DESC, rowid DESC`, taskID)
}

// GetHandover returns the row, (nil, nil) when it does not exist.
func (r *AdeTaskRepo) GetHandover(id string) (*model.AdeHandover, error) {
	return sqlitex.QueryOne(r.DB, func(row *sql.Row) (*model.AdeHandover, error) {
		h, err := scanHandover(row)
		return &h, err
	}, `SELECT `+handoverCols+` FROM ade_task_handovers WHERE id = ?`, id)
}

// HandoverDraft returns the task's open draft for a stage, (nil, nil) when none.
func (r *AdeTaskRepo) HandoverDraft(taskID, stepID string) (*model.AdeHandover, error) {
	return sqlitex.QueryOne(r.DB, func(row *sql.Row) (*model.AdeHandover, error) {
		h, err := scanHandover(row)
		return &h, err
	}, `SELECT `+handoverCols+` FROM ade_task_handovers WHERE task_id = ? AND from_step_id = ? AND state = 'draft'`,
		taskID, stepID)
}

// HandoverDrafts lists the task's open drafts.
func (r *AdeTaskRepo) HandoverDrafts(taskID string) ([]model.AdeHandover, error) {
	return r.queryHandovers(`SELECT `+handoverCols+` FROM ade_task_handovers WHERE task_id = ? AND state = 'draft'
		ORDER BY updated_at DESC`, taskID)
}

// LatestHandover returns the task's newest handed row, (nil, nil) when none.
func (r *AdeTaskRepo) LatestHandover(taskID string) (*model.AdeHandover, error) {
	return sqlitex.QueryOne(r.DB, func(row *sql.Row) (*model.AdeHandover, error) {
		h, err := scanHandover(row)
		return &h, err
	}, `SELECT `+handoverCols+` FROM ade_task_handovers WHERE task_id = ? AND state = 'handed'
		ORDER BY created_at DESC, rowid DESC LIMIT 1`, taskID)
}

// FinishedDrafts lists every open draft marked finished and not dismissed, across live tasks.
func (r *AdeTaskRepo) FinishedDrafts() ([]model.AdeHandover, error) {
	return r.queryHandovers(`SELECT ` + handoverCols + ` FROM ade_task_handovers
		WHERE state = 'draft' AND finished = 1 AND dismissed = 0 AND task_id IN (SELECT id FROM ade_tasks WHERE archived_at IS NULL)`)
}

// BoardHandover is one task's board facts: the newest handed row and its open drafts.
type BoardHandover struct {
	In     *model.AdeHandover
	Drafts []model.AdeHandover
}

// BoardHandovers reads the newest handed row and the open drafts of every live task in two queries.
func (r *AdeTaskRepo) BoardHandovers() (map[string]BoardHandover, error) {
	out := map[string]BoardHandover{}
	latest, err := r.queryHandovers(`SELECT ` + handoverCols + ` FROM ade_task_handovers h
		WHERE state = 'handed' AND NOT EXISTS (SELECT 1 FROM ade_task_handovers n WHERE n.task_id = h.task_id
			AND n.state = 'handed' AND (n.created_at > h.created_at OR (n.created_at = h.created_at AND n.rowid > h.rowid)))`)
	if err != nil {
		return nil, fmt.Errorf("repos: board handovers: %w", err)
	}
	for i := range latest {
		bh := out[latest[i].TaskID]
		bh.In = &latest[i]
		out[latest[i].TaskID] = bh
	}
	drafts, err := r.queryHandovers(`SELECT ` + handoverCols + ` FROM ade_task_handovers WHERE state = 'draft'
		ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("repos: board handover drafts: %w", err)
	}
	for _, d := range drafts {
		bh := out[d.TaskID]
		bh.Drafts = append(bh.Drafts, d)
		out[d.TaskID] = bh
	}
	return out, nil
}

// UpsertHandoverDraft writes the open draft of a stage: the body is replaced, the source is the agent, and
// a dismissal is cleared. A new draft takes h.ID; an existing one keeps its own.
func (r *AdeTaskRepo) UpsertHandoverDraft(h model.AdeHandover) (model.AdeHandover, error) {
	fin := 0
	if h.Finished {
		fin = 1
	}
	if _, err := r.DB.Exec(`INSERT INTO ade_task_handovers
		(id, task_id, from_step_id, from_step_name, body, source, state, finished, created_at, updated_at, session_id)
		VALUES (?, ?, ?, ?, ?, 'agent', 'draft', ?, ?, ?, ?)
		ON CONFLICT (task_id, from_step_id) WHERE state = 'draft' DO UPDATE SET
			body = excluded.body, source = 'agent', finished = excluded.finished, dismissed = 0, edited = 0,
			from_step_name = excluded.from_step_name, updated_at = excluded.updated_at, session_id = excluded.session_id`,
		h.ID, h.TaskID, h.FromStepID, h.FromStepName, h.Body, fin, h.UpdatedAt, h.UpdatedAt, h.SessionID); err != nil {
		return model.AdeHandover{}, fmt.Errorf("repos: upsert handover draft %s: %w", h.TaskID, err)
	}
	d, err := r.HandoverDraft(h.TaskID, h.FromStepID)
	if err != nil || d == nil {
		return model.AdeHandover{}, fmt.Errorf("repos: read handover draft %s: %w", h.TaskID, err)
	}
	return *d, nil
}

// UpdateHandoverBody replaces a row's body and marks it edited; the row it changed is returned.
func (r *AdeTaskRepo) UpdateHandoverBody(id, body string, now int64) (model.AdeHandover, error) {
	res, err := r.DB.Exec(`UPDATE ade_task_handovers SET body = ?, edited = 1, updated_at = ? WHERE id = ?`, body, now, id)
	if err != nil {
		return model.AdeHandover{}, fmt.Errorf("repos: update handover %s: %w", id, err)
	}
	if err := sqlitex.RequireOneRow(res, "handover "+id); err != nil {
		return model.AdeHandover{}, err
	}
	h, err := r.GetHandover(id)
	if err != nil || h == nil {
		return model.AdeHandover{}, fmt.Errorf("repos: read handover %s: %w", id, err)
	}
	return *h, nil
}

// DeleteHandover removes a row.
func (r *AdeTaskRepo) DeleteHandover(id string) error {
	res, err := r.DB.Exec(`DELETE FROM ade_task_handovers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("repos: delete handover %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "handover "+id)
}

// DismissHandoverDraft marks the stage's open draft dismissed; it reports whether a draft existed.
func (r *AdeTaskRepo) DismissHandoverDraft(taskID, stepID string) (bool, error) {
	res, err := r.DB.Exec(`UPDATE ade_task_handovers SET dismissed = 1 WHERE task_id = ? AND from_step_id = ? AND state = 'draft'`,
		taskID, stepID)
	if err != nil {
		return false, fmt.Errorf("repos: dismiss handover %s: %w", taskID, err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// RecordRunHandover stores a finished run's handover without moving the task.
func (r *AdeTaskRepo) RecordRunHandover(taskID string, h model.AdeHandoff) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin run handover %s: %w", taskID, err)
	}
	defer tx.Rollback() //nolint:errcheck
	if err := applyHandoff(tx, taskID, h); err != nil {
		return err
	}
	return tx.Commit()
}

func applyHandoff(tx *sql.Tx, taskID string, h model.AdeHandoff) error {
	var draft *model.AdeHandover
	row := tx.QueryRow(`SELECT `+handoverCols+` FROM ade_task_handovers WHERE task_id = ? AND from_step_id = ? AND state = 'draft'`,
		taskID, h.FromStepID)
	if d, err := scanHandover(row); err == nil {
		draft = &d
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("repos: read handover draft %s: %w", taskID, err)
	}
	insert := func(body, source, sessionID string) error {
		if _, err := tx.Exec(`INSERT INTO ade_task_handovers
			(id, task_id, from_step_id, from_step_name, to_step_id, to_step_name, body, source, state, created_at, updated_at, session_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'handed', ?, ?, ?)`,
			h.ID, taskID, h.FromStepID, h.FromStepName, h.ToStepID, h.ToStepName, body, source, h.Now, h.Now, sessionID); err != nil {
			return fmt.Errorf("repos: insert handover %s: %w", taskID, err)
		}
		return nil
	}
	if h.Source == model.AdeHandoverSourceRun {
		if draft != nil {
			if _, err := tx.Exec(`DELETE FROM ade_task_handovers WHERE id = ?`, draft.ID); err != nil {
				return fmt.Errorf("repos: replace handover draft %s: %w", taskID, err)
			}
		}
		if h.Notes == nil || *h.Notes == "" {
			return nil
		}
		return insert(*h.Notes, model.AdeHandoverSourceRun, h.SessionID)
	}
	if draft == nil {
		if h.Notes == nil || *h.Notes == "" {
			return nil
		}
		return insert(*h.Notes, model.AdeHandoverSourceUser, "")
	}
	if h.Notes != nil && *h.Notes == "" {
		if _, err := tx.Exec(`DELETE FROM ade_task_handovers WHERE id = ?`, draft.ID); err != nil {
			return fmt.Errorf("repos: drop handover draft %s: %w", taskID, err)
		}
		return nil
	}
	body, edited := draft.Body, draft.Edited
	if h.Notes != nil && *h.Notes != draft.Body {
		body, edited = *h.Notes, true
	}
	if _, err := tx.Exec(`UPDATE ade_task_handovers SET to_step_id = ?, to_step_name = ?, body = ?, edited = ?,
		state = 'handed', finished = 0, dismissed = 0, created_at = ?, updated_at = ? WHERE id = ?`,
		h.ToStepID, h.ToStepName, body, edited, h.Now, h.Now, draft.ID); err != nil {
		return fmt.Errorf("repos: hand over draft %s: %w", taskID, err)
	}
	return nil
}
