package repos

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/huandu/go-sqlbuilder"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// RepoFiles is the repo config files: nickname, integration branches and environments of a repo.
type RepoFiles interface {
	Repo(repoID string) repocfg.Config
}

// AdeRepoConfigRepo reads the per-repo ade config over code_repos and writes its watched folders and
// import source. The rest of a repo's config (nickname, integration branches, environments, prepare
// script) lives in the repo files.
type AdeRepoConfigRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
	Files   RepoFiles
}

// List returns every code repo (sort order) joined with its import source and its repo file's
// nickname, integration branches and environments. A repo with no file carries the defaults.
func (r *AdeRepoConfigRepo) List() ([]model.AdeRepoConfig, error) {
	c, a := kiraschema.CodeRepos, kiraschema.AdeRepoConfig
	sb := r.Dialect.Select()
	sb.Select("c."+c.ID, "c."+c.RepoID, "c."+c.Name, "c."+c.Root, "COALESCE(a."+a.Source+", "+sb.Var("added")+")").
		From(c.Table+" c").JoinWithOption(sqlbuilder.LeftJoin, a.Table+" a", "a."+a.CodeRepoID+" = c."+c.ID).
		OrderBy("c."+c.SortOrder, "c."+c.ID)
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade repo config: %w", err)
	}
	defer rows.Close()
	out := make([]model.AdeRepoConfig, 0)
	for rows.Next() {
		var rc model.AdeRepoConfig
		if err := rows.Scan(&rc.CodeRepoID, &rc.RepoID, &rc.Name, &rc.Root, &rc.Source); err != nil {
			return nil, fmt.Errorf("repos: scan ade repo config: %w", err)
		}
		cfg := repocfg.Defaults(rc.RepoID)
		if r.Files != nil {
			cfg = r.Files.Repo(rc.RepoID)
		}
		rc.Nickname, rc.IntegrationBranches, rc.Environments = cfg.Nickname, cfg.IntegrationBranches, cfg.Environments
		out = append(out, rc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade repo config rows: %w", err)
	}
	return out, nil
}

// Folders returns every watched folder with its imported repo count (repos whose source is the path).
func (r *AdeRepoConfigRepo) Folders() ([]model.AdeFolder, error) {
	f, a, c := kiraschema.AdeFolders, kiraschema.AdeRepoConfig, kiraschema.CodeRepos
	imported := r.Dialect.Select("COUNT(*)").From(a.Table + " a")
	imported.Where("a." + a.Source + " = f." + f.Path)
	hiddenN := r.Dialect.Select("COUNT(*)").From(a.Table + " a")
	hiddenN.Join(c.Table+" c", "c."+c.ID+" = a."+a.CodeRepoID).
		Where("a."+a.Source+" = f."+f.Path, hiddenN.Equal("c."+c.Hidden, 1))
	sb := r.Dialect.Select()
	sb.Select("f."+f.Path, "f."+f.Watch, "f."+f.Hidden, sb.BuilderAs(imported, "repo_count"), sb.BuilderAs(hiddenN, "hidden_count")).
		From(f.Table + " f").OrderBy("f." + f.Path)
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade folders: %w", err)
	}
	defer rows.Close()
	out := make([]model.AdeFolder, 0)
	for rows.Next() {
		var f model.AdeFolder
		var watch, hidden int
		if err := rows.Scan(&f.Path, &watch, &hidden, &f.RepoCount, &f.HiddenCount); err != nil {
			return nil, fmt.Errorf("repos: scan ade folder: %w", err)
		}
		f.Watch = watch != 0
		f.Hidden = hidden != 0
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade folders rows: %w", err)
	}
	return out, nil
}

// AddFolder inserts a watched folder; an existing path keeps its row and updates its watch flag.
func (r *AdeRepoConfigRepo) AddFolder(path string, watch bool) error {
	f := kiraschema.AdeFolders
	ib := r.Dialect.InsertInto(f.Table).Cols(f.Path, f.Watch).Values(path, boolInt(watch))
	sqlb.Upsert(ib, []string{f.Path}, []string{f.Watch}, "")
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: add ade folder: %w", err)
	}
	return nil
}

// SetFolderWatch flips a folder's watch flag; false when the folder is unknown.
func (r *AdeRepoConfigRepo) SetFolderWatch(path string, watch bool) (bool, error) {
	f := kiraschema.AdeFolders
	ub := r.Dialect.Update(f.Table)
	ub.Set(ub.Assign(f.Watch, boolInt(watch))).Where(ub.Equal(f.Path, path))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return false, fmt.Errorf("repos: set ade folder watch: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("repos: ade folder rows affected: %w", err)
	}
	return n > 0, nil
}

// SetFolderHidden sets the folder's hidden flag and writes it onto every repo it imported, in one
// transaction. False when the folder is unknown.
func (r *AdeRepoConfigRepo) SetFolderHidden(path string, hidden bool) (bool, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return false, fmt.Errorf("repos: begin set ade folder hidden: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	f, a, c := kiraschema.AdeFolders, kiraschema.AdeRepoConfig, kiraschema.CodeRepos
	folderUp := r.Dialect.Update(f.Table)
	folderUp.Set(folderUp.Assign(f.Hidden, boolInt(hidden))).Where(folderUp.Equal(f.Path, path))
	res, err := sqlb.Exec(tx, folderUp)
	if err != nil {
		return false, fmt.Errorf("repos: set ade folder hidden: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("repos: ade folder rows affected: %w", err)
	}
	if n == 0 {
		return false, nil
	}
	imported := r.Dialect.Select(a.CodeRepoID).From(a.Table)
	imported.Where(imported.Equal(a.Source, path))
	repoUp := r.Dialect.Update(c.Table)
	repoUp.Set(repoUp.Assign(c.Hidden, boolInt(hidden)), c.HiddenRev+" = "+c.HiddenRev+" + 1").Where(repoUp.In(c.ID, imported))
	if _, err := sqlb.Exec(tx, repoUp); err != nil {
		return false, fmt.Errorf("repos: hide folder repos: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("repos: commit set ade folder hidden: %w", err)
	}
	return true, nil
}

// ImportedRepo is a repo a folder scan imported and the hidden_rev it was stored with.
type ImportedRepo struct {
	ID  string
	Rev int
}

// ApplyFolderHidden writes the folder's stored hidden flag onto the imported repos whose hidden_rev
// still equals the one at import, in one transaction. A repo written since (the folder flip's own
// bulk UPDATE, a per-repo toggle) is skipped. It never writes ade_folders, so a flag changed since
// the caller read it is not overwritten.
func (r *AdeRepoConfigRepo) ApplyFolderHidden(path string, imported []ImportedRepo) error {
	if len(imported) == 0 {
		return nil
	}
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin apply ade folder hidden: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	f, c := kiraschema.AdeFolders, kiraschema.CodeRepos
	for _, in := range imported {
		stored := r.Dialect.Select(f.Hidden).From(f.Table)
		stored.Where(stored.Equal(f.Path, path))
		ub := r.Dialect.Update(c.Table)
		ub.Set(c.Hidden+" = COALESCE(("+ub.Var(stored)+"), "+c.Hidden+")", c.HiddenRev+" = "+c.HiddenRev+" + 1").
			Where(ub.Equal(c.ID, in.ID), ub.Equal(c.HiddenRev, in.Rev))
		if _, err := sqlb.Exec(tx, ub); err != nil {
			return fmt.Errorf("repos: apply ade folder hidden: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit apply ade folder hidden: %w", err)
	}
	return nil
}

// FolderHidden reports a folder's hidden flag; false for an unknown folder.
func (r *AdeRepoConfigRepo) FolderHidden(path string) (bool, error) {
	var hidden int
	f := kiraschema.AdeFolders
	sb := r.Dialect.Select(f.Hidden).From(f.Table)
	sb.Where(sb.Equal(f.Path, path))
	err := sqlb.QueryRow(r.DB, sb).Scan(&hidden)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("repos: ade folder hidden: %w", err)
	}
	return hidden != 0, nil
}

// RemoveFolder deletes the folder row and moves its imported repos to source 'added', in one
// transaction. False when the folder is unknown.
func (r *AdeRepoConfigRepo) RemoveFolder(path string) (bool, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return false, fmt.Errorf("repos: begin remove ade folder: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	f, a := kiraschema.AdeFolders, kiraschema.AdeRepoConfig
	del := r.Dialect.DeleteFrom(f.Table)
	del.Where(del.Equal(f.Path, path))
	res, err := sqlb.Exec(tx, del)
	if err != nil {
		return false, fmt.Errorf("repos: delete ade folder: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("repos: ade folder rows affected: %w", err)
	}
	if n == 0 {
		return false, nil
	}
	ub := r.Dialect.Update(a.Table)
	ub.Set(ub.Assign(a.Source, "added")).Where(ub.Equal(a.Source, path))
	if _, err := sqlb.Exec(tx, ub); err != nil {
		return false, fmt.Errorf("repos: release folder repos: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("repos: commit remove ade folder: %w", err)
	}
	return true, nil
}

// SetSource records where a repo was imported from ('added' or a folder path).
func (r *AdeRepoConfigRepo) SetSource(codeRepoID, source string) error {
	a := kiraschema.AdeRepoConfig
	ib := r.Dialect.InsertInto(a.Table).Cols(a.CodeRepoID, a.Source).Values(codeRepoID, source)
	sqlb.Upsert(ib, []string{a.CodeRepoID}, []string{a.Source}, "")
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: set ade repo source: %w", err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
