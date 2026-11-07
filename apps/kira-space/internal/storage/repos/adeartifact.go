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

// artifactCols is in scan order.
var artifactCols = []string{
	kiraschema.AdeTaskArtifacts.ID, kiraschema.AdeTaskArtifacts.TaskID, kiraschema.AdeTaskArtifacts.Kind,
	kiraschema.AdeTaskArtifacts.Title, kiraschema.AdeTaskArtifacts.URL, kiraschema.AdeTaskArtifacts.Ref,
	kiraschema.AdeTaskArtifacts.Source, kiraschema.AdeTaskArtifacts.StepID, kiraschema.AdeTaskArtifacts.RunID,
	kiraschema.AdeTaskArtifacts.CreatedAt, kiraschema.AdeTaskArtifacts.UpdatedAt, kiraschema.AdeTaskArtifacts.SessionID,
}

func (r *AdeTaskRepo) selectArtifacts() *sqlbuilder.SelectBuilder {
	return r.Dialect.Select(artifactCols...).From(kiraschema.AdeTaskArtifacts.Table)
}

func scanArtifact(row rowScanner) (model.AdeTaskArtifact, error) {
	var a model.AdeTaskArtifact
	err := row.Scan(&a.ID, &a.TaskID, &a.Kind, &a.Title, &a.URL, &a.Ref, &a.Source, &a.StepID, &a.RunID,
		&a.CreatedAt, &a.UpdatedAt, &a.SessionID)
	return a, err
}

// Artifacts lists the task's reported artifacts, oldest first.
func (r *AdeTaskRepo) Artifacts(taskID string) ([]model.AdeTaskArtifact, error) {
	c := kiraschema.AdeTaskArtifacts
	sb := r.selectArtifacts()
	sb.Where(sb.Equal(c.TaskID, taskID)).OrderBy(c.CreatedAt, sqlb.RowID)
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.AdeTaskArtifact, bool, error) {
		a, err := scanArtifact(rows)
		return a, true, err
	})
}

// GetArtifact returns the row, (nil, nil) when it does not exist.
func (r *AdeTaskRepo) GetArtifact(id string) (*model.AdeTaskArtifact, error) {
	sb := r.selectArtifacts()
	sb.Where(sb.Equal(kiraschema.AdeTaskArtifacts.ID, id))
	return adeOne(r.DB, sb, func(row *sql.Row) (*model.AdeTaskArtifact, error) {
		a, err := scanArtifact(row)
		return &a, err
	})
}

// CountArtifacts returns how many artifacts the task has stored.
func (r *AdeTaskRepo) CountArtifacts(taskID string) (int, error) {
	c := kiraschema.AdeTaskArtifacts
	n, err := adeCount(r.DB, r.Dialect, c.Table, func(sb *sqlbuilder.SelectBuilder) []string { return []string{sb.Equal(c.TaskID, taskID)} })
	if err != nil {
		return 0, fmt.Errorf("repos: count artifacts %s: %w", taskID, err)
	}
	return n, nil
}

// UpsertArtifact stores an artifact. A row with the same (task, url) updates kind, title, ref and
// updated_at in place; an empty URL always inserts. A new row takes a.ID.
func (r *AdeTaskRepo) UpsertArtifact(a model.AdeTaskArtifact) (model.AdeTaskArtifact, error) {
	c := kiraschema.AdeTaskArtifacts
	ib := r.Dialect.InsertInto(c.Table).Cols(artifactCols...).
		Values(a.ID, a.TaskID, a.Kind, a.Title, a.URL, a.Ref, a.Source, a.StepID, a.RunID, a.CreatedAt, a.UpdatedAt, a.SessionID)
	if a.URL == "" {
		if _, err := sqlb.Exec(r.DB, ib); err != nil {
			return model.AdeTaskArtifact{}, fmt.Errorf("repos: insert artifact %s: %w", a.TaskID, err)
		}
		return r.readArtifact(a.ID)
	}
	sqlb.AdeUpsertPartial(ib, []string{c.TaskID, c.URL}, c.URL+" <> ''", []string{
		c.Kind + " = excluded." + c.Kind, c.Title + " = excluded." + c.Title, c.Ref + " = excluded." + c.Ref,
		c.UpdatedAt + " = excluded." + c.UpdatedAt,
		c.SessionID + " = CASE WHEN excluded." + c.SessionID + " <> '' THEN excluded." + c.SessionID + " ELSE " + c.Table + "." + c.SessionID + " END",
	})
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return model.AdeTaskArtifact{}, fmt.Errorf("repos: upsert artifact %s: %w", a.TaskID, err)
	}
	var id string
	idQ := r.Dialect.Select(c.ID).From(c.Table)
	idQ.Where(idQ.Equal(c.TaskID, a.TaskID), idQ.Equal(c.URL, a.URL))
	if err := sqlb.QueryRow(r.DB, idQ).Scan(&id); err != nil {
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
	c := kiraschema.AdeTaskArtifacts
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Kind, kind), ub.Assign(c.Title, title), ub.Assign(c.URL, url), ub.Assign(c.Ref, ref), ub.Assign(c.UpdatedAt, now)).
		Where(ub.Equal(c.ID, id), ub.Equal(c.TaskID, taskID))
	res, err := sqlb.Exec(r.DB, ub)
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
	c := kiraschema.AdeTaskArtifacts
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(del.Equal(c.ID, id))
	res, err := sqlb.Exec(r.DB, del)
	if err != nil {
		return fmt.Errorf("repos: delete artifact %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "artifact "+id)
}
