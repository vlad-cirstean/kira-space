package repos

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// RepoFiles is the repo config files: nickname, integration branches and environments of a repo.
type RepoFiles interface {
	Repo(repoID string) repocfg.Config
}

// AdeRepoConfigRepo reads the per-repo ade config over code_repos and writes its watched folders and
// import source. The rest of a repo's config (nickname, integration branches, environments, prepare
// script) lives in the repo files.
type AdeRepoConfigRepo struct {
	DB    *sql.DB
	Files RepoFiles
}

// List returns every code repo (sort order) joined with its import source and its repo file's
// nickname, integration branches and environments. A repo with no file carries the defaults.
func (r *AdeRepoConfigRepo) List() ([]model.AdeRepoConfig, error) {
	rows, err := r.DB.Query(`SELECT c.id, c.repo_id, c.name, c.root, COALESCE(a.source, 'added')
		FROM code_repos c LEFT JOIN ade_repo_config a ON a.code_repo_id = c.id ORDER BY c.sort_order, c.id`)
	if err != nil {
		return nil, fmt.Errorf("repos: query ade repo config: %w", err)
	}
	defer rows.Close()
	out := make([]model.AdeRepoConfig, 0)
	for rows.Next() {
		var c model.AdeRepoConfig
		if err := rows.Scan(&c.CodeRepoID, &c.RepoID, &c.Name, &c.Root, &c.Source); err != nil {
			return nil, fmt.Errorf("repos: scan ade repo config: %w", err)
		}
		cfg := repocfg.Defaults(c.RepoID)
		if r.Files != nil {
			cfg = r.Files.Repo(c.RepoID)
		}
		c.Nickname, c.IntegrationBranches, c.Environments = cfg.Nickname, cfg.IntegrationBranches, cfg.Environments
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade repo config rows: %w", err)
	}
	return out, nil
}

// Folders returns every watched folder with its imported repo count (repos whose source is the path).
func (r *AdeRepoConfigRepo) Folders() ([]model.AdeFolder, error) {
	rows, err := r.DB.Query(`SELECT f.path, f.watch, f.hidden, (SELECT COUNT(*) FROM ade_repo_config a WHERE a.source = f.path),
		(SELECT COUNT(*) FROM ade_repo_config a JOIN code_repos c ON c.id = a.code_repo_id
			WHERE a.source = f.path AND c.hidden = 1)
		FROM ade_folders f ORDER BY f.path`)
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
	if _, err := r.DB.Exec(`INSERT INTO ade_folders (path, watch) VALUES (?, ?)
		ON CONFLICT(path) DO UPDATE SET watch = excluded.watch`, path, boolInt(watch)); err != nil {
		return fmt.Errorf("repos: add ade folder: %w", err)
	}
	return nil
}

// SetFolderWatch flips a folder's watch flag; false when the folder is unknown.
func (r *AdeRepoConfigRepo) SetFolderWatch(path string, watch bool) (bool, error) {
	res, err := r.DB.Exec(`UPDATE ade_folders SET watch = ? WHERE path = ?`, boolInt(watch), path)
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
	res, err := tx.Exec(`UPDATE ade_folders SET hidden = ? WHERE path = ?`, boolInt(hidden), path)
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
	if _, err := tx.Exec(`UPDATE code_repos SET hidden = ?, hidden_rev = hidden_rev + 1 WHERE id IN
		(SELECT code_repo_id FROM ade_repo_config WHERE source = ?)`, boolInt(hidden), path); err != nil {
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
	for _, in := range imported {
		if _, err := tx.Exec(`UPDATE code_repos SET hidden = COALESCE((SELECT hidden FROM ade_folders WHERE path = ?), hidden),
			hidden_rev = hidden_rev + 1 WHERE id = ? AND hidden_rev = ?`, path, in.ID, in.Rev); err != nil {
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
	err := r.DB.QueryRow(`SELECT hidden FROM ade_folders WHERE path = ?`, path).Scan(&hidden)
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
	res, err := tx.Exec(`DELETE FROM ade_folders WHERE path = ?`, path)
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
	if _, err := tx.Exec(`UPDATE ade_repo_config SET source = 'added' WHERE source = ?`, path); err != nil {
		return false, fmt.Errorf("repos: release folder repos: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("repos: commit remove ade folder: %w", err)
	}
	return true, nil
}

// SetSource records where a repo was imported from ('added' or a folder path).
func (r *AdeRepoConfigRepo) SetSource(codeRepoID, source string) error {
	if _, err := r.DB.Exec(`INSERT INTO ade_repo_config (code_repo_id, source) VALUES (?, ?)
		ON CONFLICT(code_repo_id) DO UPDATE SET source = excluded.source`, codeRepoID, source); err != nil {
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
