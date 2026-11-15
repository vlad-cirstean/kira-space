package repos

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// DetailsCache is the SQLite cache of the Details files (P643): the handler of a file kind writes it
// inside the transaction of its file, through the Querier the file index hands out.
type DetailsCache struct{ Dialect sqlb.Dialect }

// HandoverByRel returns the handover row a file caches, (nil, nil) when none.
func (c DetailsCache) HandoverByRel(q sqlb.Querier, taskID, rel string) (*model.AdeHandover, error) {
	k := kiraschema.AdeTaskHandovers
	sb := c.Dialect.Select(handoverCols...).From(k.Table)
	sb.Where(sb.Equal(k.TaskID, taskID), sb.Equal(k.RelPath, rel))
	return oneHandover(q, sb)
}

// DraftOf returns the open draft of a stage, (nil, nil) when none.
func (c DetailsCache) DraftOf(q sqlb.Querier, taskID, stepID string) (*model.AdeHandover, error) {
	k := kiraschema.AdeTaskHandovers
	sb := c.Dialect.Select(handoverCols...).From(k.Table)
	sb.Where(sb.Equal(k.TaskID, taskID), sb.Equal(k.FromStepID, stepID), sb.Equal(k.State, handoverDraft))
	return oneHandover(q, sb)
}

func oneHandover(q sqlb.Querier, b sqlb.Builder) (*model.AdeHandover, error) {
	h, err := scanHandover(sqlb.QueryRow(q, b))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// InsertHandover writes a row with every cache field, rel_path included.
func (c DetailsCache) InsertHandover(q sqlb.Querier, h model.AdeHandover) error {
	k := kiraschema.AdeTaskHandovers
	ib := c.Dialect.InsertInto(k.Table).Cols(k.ID, k.TaskID, k.FromStepID, k.FromStepName, k.ToStepID, k.ToStepName, k.Body, k.Source,
		k.State, k.Finished, k.Edited, k.CreatedAt, k.UpdatedAt, k.SessionID, k.FromTaskID, k.RelPath).
		Values(h.ID, h.TaskID, h.FromStepID, h.FromStepName, h.ToStepID, h.ToStepName, h.Body, h.Source,
			h.State, h.Finished, h.Edited, h.CreatedAt, h.UpdatedAt, h.SessionID, nilIfEmpty(h.FromTaskID), h.RelPath)
	if _, err := sqlb.Exec(q, ib); err != nil {
		return fmt.Errorf("repos: insert handover %s: %w", h.TaskID, err)
	}
	return nil
}

// SetHandoverBody replaces a row's body; edited marks it changed by hand.
func (c DetailsCache) SetHandoverBody(q sqlb.Querier, id, body string, edited bool, now int64) error {
	k := kiraschema.AdeTaskHandovers
	ub := c.Dialect.Update(k.Table)
	ub.Set(ub.Assign(k.Body, body), ub.Assign(k.Edited, edited), ub.Assign(k.UpdatedAt, now)).Where(ub.Equal(k.ID, id))
	res, err := sqlb.Exec(q, ub)
	if err != nil {
		return fmt.Errorf("repos: update handover %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "handover "+id)
}

// SetHandoverRel records the file of a row.
func (c DetailsCache) SetHandoverRel(q sqlb.Querier, id, rel string) error {
	k := kiraschema.AdeTaskHandovers
	ub := c.Dialect.Update(k.Table)
	ub.Set(ub.Assign(k.RelPath, rel)).Where(ub.Equal(k.ID, id))
	_, err := sqlb.Exec(q, ub)
	return err
}

// DeleteHandover removes a row; a missing row is not an error.
func (c DetailsCache) DeleteHandover(q sqlb.Querier, id string) error {
	_, err := sqlb.Exec(q, deleteHandoverByID(c.Dialect, id))
	return err
}

// ClearHandoverRel detaches a row from its file (the file is gone, the row stays).
func (c DetailsCache) ClearHandoverRel(q sqlb.Querier, taskID, rel string) error {
	k := kiraschema.AdeTaskHandovers
	ub := c.Dialect.Update(k.Table)
	ub.Set(ub.Assign(k.RelPath, "")).Where(ub.Equal(k.TaskID, taskID), ub.Equal(k.RelPath, rel))
	_, err := sqlb.Exec(q, ub)
	return err
}

// TaskHandoversWithFiles lists the rows of a task: notes (user), drafts and handed, oldest first.
func (c DetailsCache) TaskHandoversWithFiles(q sqlb.Querier, taskID string) ([]model.AdeHandover, error) {
	k := kiraschema.AdeTaskHandovers
	sb := c.Dialect.Select(handoverCols...).From(k.Table)
	sb.Where(sb.Equal(k.TaskID, taskID)).OrderBy(k.CreatedAt, sqlb.RowID)
	rows, err := sqlb.Query(q, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeHandover, bool, error) {
		h, err := scanHandover(rows)
		return h, true, err
	})
}

// Artifacts lists the task's stored artifacts, oldest first.
func (c DetailsCache) Artifacts(q sqlb.Querier, taskID string) ([]model.AdeTaskArtifact, error) {
	k := kiraschema.AdeTaskArtifacts
	sb := c.Dialect.Select(artifactCols...).From(k.Table)
	sb.Where(sb.Equal(k.TaskID, taskID)).OrderBy(k.CreatedAt, sqlb.RowID)
	rows, err := sqlb.Query(q, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeTaskArtifact, bool, error) {
		a, err := scanArtifact(rows)
		return a, true, err
	})
}

// InsertArtifact writes a stored artifact row.
func (c DetailsCache) InsertArtifact(q sqlb.Querier, a model.AdeTaskArtifact) error {
	k := kiraschema.AdeTaskArtifacts
	ib := c.Dialect.InsertInto(k.Table).Cols(artifactCols...).
		Values(a.ID, a.TaskID, a.Kind, a.Title, a.URL, a.Ref, a.Source, a.StepID, a.RunID, a.CreatedAt, a.UpdatedAt, a.SessionID)
	if _, err := sqlb.Exec(q, ib); err != nil {
		return fmt.Errorf("repos: insert artifact %s: %w", a.TaskID, err)
	}
	return nil
}

// UpdateArtifact rewrites a row's kind, title, url and ref.
func (c DetailsCache) UpdateArtifact(q sqlb.Querier, id, kind, title, url, ref string, now int64) error {
	k := kiraschema.AdeTaskArtifacts
	ub := c.Dialect.Update(k.Table)
	ub.Set(ub.Assign(k.Kind, kind), ub.Assign(k.Title, title), ub.Assign(k.URL, url), ub.Assign(k.Ref, ref), ub.Assign(k.UpdatedAt, now)).
		Where(ub.Equal(k.ID, id))
	_, err := sqlb.Exec(q, ub)
	return err
}

// DeleteArtifact removes a stored artifact row.
func (c DetailsCache) DeleteArtifact(q sqlb.Querier, id string) error {
	k := kiraschema.AdeTaskArtifacts
	del := c.Dialect.DeleteFrom(k.Table)
	del.Where(del.Equal(k.ID, id))
	_, err := sqlb.Exec(q, del)
	return err
}

// TaskSlots is the Jira and pull request link slots of a task.
type TaskSlots struct{ JiraKey, JiraURL, HostURL string }

// Slots reads the slots of a task.
func (c DetailsCache) Slots(q sqlb.Querier, taskID string) (TaskSlots, error) {
	k := kiraschema.AdeTasks
	sb := c.Dialect.Select(k.JiraKey, k.JiraURL, k.HostURL).From(k.Table)
	sb.Where(sb.Equal(k.ID, taskID))
	var s TaskSlots
	if err := sqlb.QueryRow(q, sb).Scan(&s.JiraKey, &s.JiraURL, &s.HostURL); err != nil {
		return s, fmt.Errorf("repos: read slots %s: %w", taskID, err)
	}
	return s, nil
}

// SetSlots writes the slots of a task.
func (c DetailsCache) SetSlots(q sqlb.Querier, taskID string, s TaskSlots) error {
	k := kiraschema.AdeTasks
	ub := c.Dialect.Update(k.Table)
	ub.Set(ub.Assign(k.JiraKey, s.JiraKey), ub.Assign(k.JiraURL, s.JiraURL), ub.Assign(k.HostURL, s.HostURL)).Where(ub.Equal(k.ID, taskID))
	_, err := sqlb.Exec(q, ub)
	return err
}
