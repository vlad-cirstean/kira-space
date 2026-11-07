package repos

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/palette"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

var codeReposSelectColumns = []string{
	kiraschema.CodeRepos.ID, kiraschema.CodeRepos.Name, kiraschema.CodeRepos.Root, kiraschema.CodeRepos.RepoID,
	kiraschema.CodeRepos.SortOrder, kiraschema.CodeRepos.Color, kiraschema.CodeRepos.CreatedAt,
	kiraschema.CodeRepos.Hidden, kiraschema.CodeRepos.HiddenRev,
}

// CodeReposRepo reads and writes the `code_repos` table (C5 §3.1) — the repo-import store.
type CodeReposRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

func scanCodeRepoRow(row rowScanner) (model.CodeRepo, error) {
	var r model.CodeRepo
	var hidden int
	if err := row.Scan(&r.ID, &r.Name, &r.Root, &r.RepoID, &r.SortOrder, &r.Color, msISO(&r.CreatedAt), &hidden, &r.HiddenRev); err != nil {
		return model.CodeRepo{}, err
	}
	r.Hidden = hidden != 0
	return r, nil
}

// List orders by sort_order ASC, name ASC.
func (r *CodeReposRepo) List() ([]model.CodeRepo, error) {
	c := kiraschema.CodeRepos
	sb := r.Dialect.Select(codeReposSelectColumns...).From(c.Table).OrderBy(c.SortOrder+" ASC", c.Name+" ASC")
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.CodeRepo, bool, error) {
		rec, err := scanCodeRepoRow(rows)
		return rec, true, err
	})
}

// Get reads one row by id, (nil, nil) when not found — CodeWorkspaceService's own callers
// distinguish "not found" from a real error rather than getting sql.ErrNoRows leaking upward.
func (r *CodeReposRepo) Get(id string) (*model.CodeRepo, error) {
	c := kiraschema.CodeRepos
	sb := r.Dialect.Select(codeReposSelectColumns...).From(c.Table)
	sb.Where(sb.Equal(c.ID, id))
	cr, err := scanCodeRepoRow(sqlb.QueryRow(r.DB, sb))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repos: get code repo %s: %w", id, err)
	}
	return &cr, nil
}

// ErrCodeRepoExists reports a Create that hit the unique repo_id index (a concurrent import).
var ErrCodeRepoExists = errors.New("repos: code repo already exists")

// Create inserts a new row, sort_order set to one past the current max — repo_id's UNIQUE index is
// what actually refuses importing the same checkout twice; the caller
// (CodeWorkspaceService.ImportRepo) checks first only to return a friendlier error than a raw
// constraint violation.
func (r *CodeReposRepo) Create(rec model.CodeRepo) (model.CodeRepo, error) {
	c := kiraschema.CodeRepos
	sortOrder, err := sqlb.NextSortOrder(r.DB, r.Dialect, c.Table, c.SortOrder, nil)
	if err != nil {
		return model.CodeRepo{}, fmt.Errorf("repos: code repo next sort order: %w", err)
	}
	rec.SortOrder = sortOrder
	if rec.Color == "" {
		rec.Color = palette.AutoRepoColor(sortOrder)
	}
	if err := rec.Validate(); err != nil {
		return model.CodeRepo{}, fmt.Errorf("repos: %w", err)
	}
	ib := r.Dialect.InsertInto(c.Table).Cols(c.ID, c.Name, c.Root, c.RepoID, c.SortOrder, c.Color, c.CreatedAt, c.Hidden).
		Values(rec.ID, rec.Name, rec.Root, rec.RepoID, rec.SortOrder, rec.Color, kiratime.ISOOrZero(rec.CreatedAt), rec.Hidden)
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		if isUniqueViolation(err) {
			return model.CodeRepo{}, ErrCodeRepoExists
		}
		return model.CodeRepo{}, fmt.Errorf("repos: insert code repo: %w", err)
	}
	return rec, nil
}

// Rename updates only the label — root/repoId are the checkout's own identity, never user-edited.
func (r *CodeReposRepo) Rename(id, name string) (model.CodeRepo, error) {
	if name == "" {
		return model.CodeRepo{}, fmt.Errorf("repos: rename code repo %s: name is required", id)
	}
	c := kiraschema.CodeRepos
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Name, name)).Where(ub.Equal(c.ID, id))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return model.CodeRepo{}, fmt.Errorf("repos: rename code repo %s: %w", id, err)
	}
	rec, err := r.Get(id)
	if err != nil {
		return model.CodeRepo{}, err
	}
	if rec == nil {
		return model.CodeRepo{}, fmt.Errorf("repos: rename code repo %s: not found", id)
	}
	return *rec, nil
}

// ErrCodeRepoNotFound means a code repo id has no row (removed in another window).
var ErrCodeRepoNotFound = errors.New("code repo not found")

// SetColor updates only the palette colour.
func (r *CodeReposRepo) SetColor(id, color string) (model.CodeRepo, error) {
	if !palette.Valid(color) {
		return model.CodeRepo{}, fmt.Errorf("repos: set code repo %s colour: invalid colour %q", id, color)
	}
	c := kiraschema.CodeRepos
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Color, color)).Where(ub.Equal(c.ID, id))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return model.CodeRepo{}, fmt.Errorf("repos: set code repo %s colour: %w", id, err)
	}
	rec, err := r.Get(id)
	if err != nil {
		return model.CodeRepo{}, err
	}
	if rec == nil {
		return model.CodeRepo{}, fmt.Errorf("repos: set code repo %s colour: %w", id, ErrCodeRepoNotFound)
	}
	return *rec, nil
}

// SetHidden updates only the hidden flag.
func (r *CodeReposRepo) SetHidden(id string, hidden bool) (model.CodeRepo, error) {
	c := kiraschema.CodeRepos
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Hidden, hidden), c.HiddenRev+" = "+c.HiddenRev+" + 1").Where(ub.Equal(c.ID, id))
	if _, err := sqlb.Exec(r.DB, ub); err != nil {
		return model.CodeRepo{}, fmt.Errorf("repos: set code repo %s hidden: %w", id, err)
	}
	rec, err := r.Get(id)
	if err != nil {
		return model.CodeRepo{}, err
	}
	if rec == nil {
		return model.CodeRepo{}, fmt.Errorf("repos: set code repo %s hidden: %w", id, ErrCodeRepoNotFound)
	}
	return *rec, nil
}

// Reorder rewrites sort_order dense in the order ids gives, in one transaction. An id with no row
// (removed in another window) is skipped; a row ids omits (imported in another window) keeps its
// relative order after the listed ones. Returns List().
func (r *CodeReposRepo) Reorder(ids []string) ([]model.CodeRepo, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("repos: reorder code repos: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	c := kiraschema.CodeRepos
	sb := r.Dialect.Select(c.ID).From(c.Table).OrderBy(c.SortOrder+" ASC", c.Name+" ASC")
	rows, err := sqlb.Query(tx, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: reorder code repos: %w", err)
	}
	existing, err := sqlitex.QueryAll(rows, nil, func(rows *sql.Rows) (string, bool, error) {
		var id string
		err := rows.Scan(&id)
		return id, true, err
	})
	if err != nil {
		return nil, fmt.Errorf("repos: reorder code repos: %w", err)
	}

	known := make(map[string]bool, len(existing))
	for _, id := range existing {
		known[id] = true
	}
	final := make([]string, 0, len(existing))
	listed := make(map[string]bool, len(ids))
	for _, id := range ids {
		if known[id] && !listed[id] {
			listed[id] = true
			final = append(final, id)
		}
	}
	for _, id := range existing {
		if !listed[id] {
			final = append(final, id)
		}
	}

	for i, id := range final {
		ub := r.Dialect.Update(c.Table)
		ub.Set(ub.Assign(c.SortOrder, i)).Where(ub.Equal(c.ID, id))
		if _, err := sqlb.Exec(tx, ub); err != nil {
			return nil, fmt.Errorf("repos: reorder code repo %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("repos: reorder code repos: %w", err)
	}
	return r.List()
}

// Remove deletes the repo row. Kira Studio's own Remove also deletes every `tabs` row scoped to
// the repo's workspace in the same transaction — Kira Space has no `tabs` table yet (this
// package's own migrations/0001_init.sql header comment: deferred; this method picks that
// cascade back up once the table exists to cascade into).
func (r *CodeReposRepo) Remove(id string) error {
	c := kiraschema.CodeRepos
	db := r.Dialect.DeleteFrom(c.Table)
	db.Where(db.Equal(c.ID, id))
	if _, err := sqlb.Exec(r.DB, db); err != nil {
		return fmt.Errorf("repos: remove code repo %s: %w", id, err)
	}
	return nil
}
