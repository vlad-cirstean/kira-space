package repos

import (
	"database/sql"
	"fmt"

	"github.com/huandu/go-sqlbuilder"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

var handoverCols = []string{
	kiraschema.AdeTaskHandovers.ID, kiraschema.AdeTaskHandovers.TaskID, kiraschema.AdeTaskHandovers.FromStepID,
	kiraschema.AdeTaskHandovers.FromStepName, kiraschema.AdeTaskHandovers.ToStepID, kiraschema.AdeTaskHandovers.ToStepName,
	kiraschema.AdeTaskHandovers.Body, kiraschema.AdeTaskHandovers.Source, kiraschema.AdeTaskHandovers.State,
	kiraschema.AdeTaskHandovers.Finished, kiraschema.AdeTaskHandovers.Dismissed, kiraschema.AdeTaskHandovers.Edited,
	kiraschema.AdeTaskHandovers.CreatedAt, kiraschema.AdeTaskHandovers.UpdatedAt, kiraschema.AdeTaskHandovers.SessionID,
}

func (r *AdeTaskRepo) selectHandovers() *sqlbuilder.SelectBuilder {
	return r.Dialect.Select(handoverCols...).From(kiraschema.AdeTaskHandovers.Table)
}

func scanHandover(row rowScanner) (model.AdeHandover, error) {
	var h model.AdeHandover
	err := row.Scan(&h.ID, &h.TaskID, &h.FromStepID, &h.FromStepName, &h.ToStepID, &h.ToStepName, &h.Body,
		&h.Source, &h.State, &h.Finished, &h.Dismissed, &h.Edited, &h.CreatedAt, &h.UpdatedAt, &h.SessionID)
	return h, err
}

func (r *AdeTaskRepo) queryHandovers(b sqlb.Builder) ([]model.AdeHandover, error) {
	rows, err := sqlb.Query(r.DB, b)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeHandover, bool, error) {
		h, err := scanHandover(rows)
		return h, true, err
	})
}

// Handovers lists the task's handover rows, newest first (an open draft sorts by its last write).
func (r *AdeTaskRepo) Handovers(taskID string) ([]model.AdeHandover, error) {
	c := kiraschema.AdeTaskHandovers
	sb := r.selectHandovers()
	sb.Where(sb.Equal(c.TaskID, taskID)).
		OrderBy("CASE "+c.State+" WHEN "+sb.Var(handoverDraft)+" THEN "+c.UpdatedAt+" ELSE "+c.CreatedAt+" END DESC", sqlb.RowID+" DESC")
	return r.queryHandovers(sb)
}

// Notes lists the task's user notes, newest first.
func (r *AdeTaskRepo) Notes(taskID string) ([]model.AdeHandover, error) {
	c := kiraschema.AdeTaskHandovers
	sb := r.selectHandovers()
	sb.Where(sb.Equal(c.TaskID, taskID), sb.Equal(c.State, handoverNote)).
		OrderBy(c.CreatedAt+" DESC", sqlb.RowID+" DESC")
	return r.queryHandovers(sb)
}

func insertNote(tx *sql.Tx, d sqlb.Dialect, h model.AdeHandover) error {
	c := kiraschema.AdeTaskHandovers
	ib := d.InsertInto(c.Table).Cols(c.ID, c.TaskID, c.FromStepID, c.FromStepName, c.Body, c.Source, c.State, c.CreatedAt, c.UpdatedAt).
		Values(h.ID, h.TaskID, h.FromStepID, h.FromStepName, h.Body, model.AdeHandoverSourceUser, handoverNote,
			h.CreatedAt, h.CreatedAt)
	if _, err := sqlb.Exec(tx, ib); err != nil {
		return fmt.Errorf("repos: insert note %s: %w", h.TaskID, err)
	}
	return nil
}

// AddNote stores a user note; h.FromStep* snapshot the task's stage.
func (r *AdeTaskRepo) AddNote(h model.AdeHandover) (model.AdeHandover, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return model.AdeHandover{}, fmt.Errorf("repos: begin add note %s: %w", h.TaskID, err)
	}
	defer tx.Rollback() //nolint:errcheck
	if err := insertNote(tx, r.Dialect, h); err != nil {
		return model.AdeHandover{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.AdeHandover{}, fmt.Errorf("repos: commit add note %s: %w", h.TaskID, err)
	}
	n, err := r.GetHandover(h.ID)
	if err != nil || n == nil {
		return model.AdeHandover{}, fmt.Errorf("repos: read note %s: %w", h.ID, err)
	}
	return *n, nil
}

// GetHandover returns the row, (nil, nil) when it does not exist.
func (r *AdeTaskRepo) GetHandover(id string) (*model.AdeHandover, error) {
	sb := r.selectHandovers()
	sb.Where(sb.Equal(kiraschema.AdeTaskHandovers.ID, id))
	return adeOne(r.DB, sb, func(row *sql.Row) (*model.AdeHandover, error) {
		h, err := scanHandover(row)
		return &h, err
	})
}

// HandoverDraft returns the task's open draft for a stage, (nil, nil) when none.
func (r *AdeTaskRepo) HandoverDraft(taskID, stepID string) (*model.AdeHandover, error) {
	c := kiraschema.AdeTaskHandovers
	sb := r.selectHandovers()
	sb.Where(sb.Equal(c.TaskID, taskID), sb.Equal(c.FromStepID, stepID), sb.Equal(c.State, handoverDraft))
	return adeOne(r.DB, sb, func(row *sql.Row) (*model.AdeHandover, error) {
		h, err := scanHandover(row)
		return &h, err
	})
}

// HandoverDrafts lists the task's open drafts.
func (r *AdeTaskRepo) HandoverDrafts(taskID string) ([]model.AdeHandover, error) {
	c := kiraschema.AdeTaskHandovers
	sb := r.selectHandovers()
	sb.Where(sb.Equal(c.TaskID, taskID), sb.Equal(c.State, handoverDraft)).OrderBy(c.UpdatedAt + " DESC")
	return r.queryHandovers(sb)
}

// LatestHandover returns the task's newest handed row, (nil, nil) when none.
func (r *AdeTaskRepo) LatestHandover(taskID string) (*model.AdeHandover, error) {
	c := kiraschema.AdeTaskHandovers
	sb := r.selectHandovers()
	sb.Where(sb.Equal(c.TaskID, taskID), sb.Equal(c.State, handoverHanded)).
		OrderBy(c.CreatedAt+" DESC", sqlb.RowID+" DESC").Limit(1)
	return adeOne(r.DB, sb, func(row *sql.Row) (*model.AdeHandover, error) {
		h, err := scanHandover(row)
		return &h, err
	})
}

// FinishedDrafts lists every open draft marked finished and not dismissed, across live tasks.
func (r *AdeTaskRepo) FinishedDrafts() ([]model.AdeHandover, error) {
	c, t := kiraschema.AdeTaskHandovers, kiraschema.AdeTasks
	live := r.Dialect.Select(t.ID).From(t.Table)
	live.Where(live.IsNull(t.ArchivedAt))
	sb := r.selectHandovers()
	sb.Where(sb.Equal(c.State, handoverDraft), sb.Equal(c.Finished, true), sb.Equal(c.Dismissed, false),
		sb.In(c.TaskID, live))
	return r.queryHandovers(sb)
}

// BoardHandover is one task's board facts: the newest handed row, its open drafts and its user notes
// (newest first).
type BoardHandover struct {
	In     *model.AdeHandover
	Drafts []model.AdeHandover
	Notes  []model.AdeHandover
}

// BoardHandovers reads the newest handed row the open drafts and the user notes of every live task in three queries.
func (r *AdeTaskRepo) BoardHandovers() (map[string]BoardHandover, error) {
	out := map[string]BoardHandover{}
	c := kiraschema.AdeTaskHandovers
	newer := r.Dialect.Select("1").From(c.Table + " n")
	newer.Where("n."+c.TaskID+" = h."+c.TaskID, newer.Equal("n."+c.State, handoverHanded),
		newer.Or("n."+c.CreatedAt+" > h."+c.CreatedAt,
			newer.And("n."+c.CreatedAt+" = h."+c.CreatedAt, "n."+sqlb.RowID+" > h."+sqlb.RowID)))
	sbLatest := r.Dialect.Select(handoverCols...).From(c.Table + " h")
	sbLatest.Where(sbLatest.Equal(c.State, handoverHanded), sbLatest.NotExists(newer))
	latest, err := r.queryHandovers(sbLatest)
	if err != nil {
		return nil, fmt.Errorf("repos: board handovers: %w", err)
	}
	for i := range latest {
		bh := out[latest[i].TaskID]
		bh.In = &latest[i]
		out[latest[i].TaskID] = bh
	}
	sbDrafts := r.selectHandovers()
	sbDrafts.Where(sbDrafts.Equal(c.State, handoverDraft)).OrderBy(c.UpdatedAt + " DESC")
	drafts, err := r.queryHandovers(sbDrafts)
	if err != nil {
		return nil, fmt.Errorf("repos: board handover drafts: %w", err)
	}
	for _, d := range drafts {
		bh := out[d.TaskID]
		bh.Drafts = append(bh.Drafts, d)
		out[d.TaskID] = bh
	}
	sbNotes := r.selectHandovers()
	sbNotes.Where(sbNotes.Equal(c.State, handoverNote)).OrderBy(c.CreatedAt+" DESC", sqlb.RowID+" DESC")
	notes, err := r.queryHandovers(sbNotes)
	if err != nil {
		return nil, fmt.Errorf("repos: board notes: %w", err)
	}
	for _, n := range notes {
		bh := out[n.TaskID]
		bh.Notes = append(bh.Notes, n)
		out[n.TaskID] = bh
	}
	return out, nil
}

// UpsertHandoverDraft writes the open draft of a stage: the body is replaced, the source is the agent, and
// a dismissal is cleared. A new draft takes h.ID; an existing one keeps its own.
func (r *AdeTaskRepo) UpsertHandoverDraft(h model.AdeHandover) (model.AdeHandover, error) {
	c := kiraschema.AdeTaskHandovers
	ib := r.Dialect.InsertInto(c.Table).Cols(c.ID, c.TaskID, c.FromStepID, c.FromStepName, c.Body, c.Source, c.State,
		c.Finished, c.CreatedAt, c.UpdatedAt, c.SessionID).
		Values(h.ID, h.TaskID, h.FromStepID, h.FromStepName, h.Body, model.AdeHandoverSourceAgent, handoverDraft,
			h.Finished, h.UpdatedAt, h.UpdatedAt, h.SessionID)
	ex := func(col string) string { return col + " = excluded." + col }
	sqlb.AdeUpsertPartial(ib, []string{c.TaskID, c.FromStepID}, c.State+" = '"+handoverDraft+"'", []string{
		ex(c.Body), c.Source + " = '" + model.AdeHandoverSourceAgent + "'", ex(c.Finished), c.Dismissed + " = 0", c.Edited + " = 0",
		ex(c.FromStepName), ex(c.UpdatedAt), ex(c.SessionID),
	})
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
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
	c := kiraschema.AdeTaskHandovers
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Body, body), ub.Assign(c.Edited, true), ub.Assign(c.UpdatedAt, now)).Where(ub.Equal(c.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
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
	del := r.Dialect.DeleteFrom(kiraschema.AdeTaskHandovers.Table)
	del.Where(del.Equal(kiraschema.AdeTaskHandovers.ID, id))
	res, err := sqlb.Exec(r.DB, del)
	if err != nil {
		return fmt.Errorf("repos: delete handover %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "handover "+id)
}

// DismissHandoverDraft marks the stage's open draft dismissed; it reports whether a draft existed.
func (r *AdeTaskRepo) DismissHandoverDraft(taskID, stepID string) (bool, error) {
	c := kiraschema.AdeTaskHandovers
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Dismissed, true)).
		Where(ub.Equal(c.TaskID, taskID), ub.Equal(c.FromStepID, stepID), ub.Equal(c.State, handoverDraft))
	res, err := sqlb.Exec(r.DB, ub)
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
	if err := applyHandoff(tx, r.Dialect, taskID, h); err != nil {
		return err
	}
	return tx.Commit()
}

func applyHandoff(tx *sql.Tx, d sqlb.Dialect, taskID string, h model.AdeHandoff) error {
	var draft *model.AdeHandover
	c := kiraschema.AdeTaskHandovers
	find := d.Select(handoverCols...).From(c.Table)
	find.Where(find.Equal(c.TaskID, taskID), find.Equal(c.FromStepID, h.FromStepID), find.Equal(c.State, handoverDraft))
	row := sqlb.QueryRow(tx, find)
	if d, err := scanHandover(row); err == nil {
		draft = &d
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("repos: read handover draft %s: %w", taskID, err)
	}
	insert := func(body, source, sessionID string) error {
		ib := d.InsertInto(c.Table).Cols(c.ID, c.TaskID, c.FromStepID, c.FromStepName, c.ToStepID, c.ToStepName, c.Body, c.Source,
			c.State, c.CreatedAt, c.UpdatedAt, c.SessionID).
			Values(h.ID, taskID, h.FromStepID, h.FromStepName, h.ToStepID, h.ToStepName, body, source,
				handoverHanded, h.Now, h.Now, sessionID)
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return fmt.Errorf("repos: insert handover %s: %w", taskID, err)
		}
		return nil
	}
	if h.Source == model.AdeHandoverSourceRun {
		if draft != nil {
			if _, err := sqlb.Exec(tx, deleteHandoverByID(d, draft.ID)); err != nil {
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
		if _, err := sqlb.Exec(tx, deleteHandoverByID(d, draft.ID)); err != nil {
			return fmt.Errorf("repos: drop handover draft %s: %w", taskID, err)
		}
		return nil
	}
	body, edited := draft.Body, draft.Edited
	if h.Notes != nil && *h.Notes != draft.Body {
		body, edited = *h.Notes, true
	}
	ub := d.Update(c.Table)
	ub.Set(ub.Assign(c.ToStepID, h.ToStepID), ub.Assign(c.ToStepName, h.ToStepName), ub.Assign(c.Body, body),
		ub.Assign(c.Edited, edited), ub.Assign(c.State, handoverHanded), ub.Assign(c.Finished, false),
		ub.Assign(c.Dismissed, false), ub.Assign(c.CreatedAt, h.Now), ub.Assign(c.UpdatedAt, h.Now)).
		Where(ub.Equal(c.ID, draft.ID))
	if _, err := sqlb.Exec(tx, ub); err != nil {
		return fmt.Errorf("repos: hand over draft %s: %w", taskID, err)
	}
	return nil
}

func deleteHandoverByID(d sqlb.Dialect, id string) *sqlbuilder.DeleteBuilder {
	del := d.DeleteFrom(kiraschema.AdeTaskHandovers.Table)
	return del.Where(del.Equal(kiraschema.AdeTaskHandovers.ID, id))
}

const (
	handoverDraft  = "draft"
	handoverNote   = "note"
	handoverHanded = "handed"
)
