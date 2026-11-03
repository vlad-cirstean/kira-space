package repocfg

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appsettings"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// migrate exports the legacy per-repo rows to files once, then drops the legacy tables.
func (s *Store) migrate() error {
	return configfs.MigrateOnce(s.o.DB, Folder, s.exportLegacy)
}

func (s *Store) exportLegacy() error {
	db := s.o.DB
	hasSettings, err := tableExists(db, "git_repo_settings")
	if err != nil {
		return err
	}
	if hasSettings {
		configs, err := readLegacy(db)
		if err != nil {
			return err
		}
		if err := s.writeLegacy(configs); err != nil {
			return err
		}
	}
	return dropLegacy(db)
}

func tableExists(db *sql.DB, name string) (bool, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n); err != nil {
		return false, fmt.Errorf("repocfg: look up table %s: %w", name, err)
	}
	return n > 0, nil
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		return false, fmt.Errorf("repocfg: look up column %s.%s: %w", table, column, err)
	}
	return n > 0, nil
}

// legacyRows collects the Config per repo id while the legacy tables are read.
type legacyRows struct {
	db        *sql.DB
	byPath    map[string]*Config
	codeRepos map[string]string // code repo id -> repo id
}

func (l *legacyRows) get(path string) *Config {
	key := Key(path)
	if c, ok := l.byPath[key]; ok {
		return c
	}
	c := Defaults(key)
	l.byPath[key] = &c
	return &c
}

// readLegacy builds one Config per repo id found in the legacy rows, sorted by path.
func readLegacy(db *sql.DB) ([]Config, error) {
	l := &legacyRows{db: db, byPath: map[string]*Config{}, codeRepos: map[string]string{}}
	steps := []func() error{l.readSettings, l.readCodeRepos, l.readNicknames, l.readIntegration, l.readEnvs}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	out := make([]Config, 0, len(l.byPath))
	for _, c := range l.byPath {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b Config) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

func (l *legacyRows) readSettings() error {
	stored := map[string]map[string]json.RawMessage{}
	err := eachRow(l.db, `SELECT repo_id, key, value FROM git_repo_settings`, func(sc func(...any) error) error {
		var repo, key, value string
		if err := sc(&repo, &key, &value); err != nil {
			return err
		}
		if stored[repo] == nil {
			stored[repo] = map[string]json.RawMessage{}
		}
		stored[repo][key] = json.RawMessage(value)
		return nil
	})
	if err != nil {
		return err
	}
	for repo, leaves := range stored {
		l.get(repo).Settings = legacySettings(leaves)
	}
	return nil
}

func (l *legacyRows) readCodeRepos() error {
	return eachRow(l.db, `SELECT id, repo_id FROM code_repos`, func(sc func(...any) error) error {
		var id, repo string
		if err := sc(&id, &repo); err != nil {
			return err
		}
		l.codeRepos[id] = repo
		return nil
	})
}

func (l *legacyRows) readNicknames() error {
	if ok, err := columnExists(l.db, "ade_repo_config", "nickname"); err != nil || !ok {
		return err
	}
	return eachRow(l.db, `SELECT code_repo_id, nickname FROM ade_repo_config WHERE nickname <> ''`, func(sc func(...any) error) error {
		var id, nick string
		if err := sc(&id, &nick); err != nil {
			return err
		}
		if repo, ok := l.codeRepos[id]; ok {
			l.get(repo).Nickname = strings.TrimSpace(nick)
		}
		return nil
	})
}

func (l *legacyRows) readIntegration() error {
	if ok, err := tableExists(l.db, "ade_repo_integration"); err != nil || !ok {
		return err
	}
	return eachRow(l.db, `SELECT code_repo_id, branch FROM ade_repo_integration ORDER BY code_repo_id, position`, func(sc func(...any) error) error {
		var id, branch string
		if err := sc(&id, &branch); err != nil {
			return err
		}
		branch = strings.TrimSpace(branch)
		repo, ok := l.codeRepos[id]
		if !ok || branch == "" {
			return nil
		}
		if c := l.get(repo); !slices.Contains(c.IntegrationBranches, branch) {
			c.IntegrationBranches = append(c.IntegrationBranches, branch)
		}
		return nil
	})
}

func (l *legacyRows) readEnvs() error {
	if ok, err := tableExists(l.db, "ade_repo_envs"); err != nil || !ok {
		return err
	}
	return eachRow(l.db, `SELECT code_repo_id, name, deployed_sha_script FROM ade_repo_envs ORDER BY code_repo_id, position`, func(sc func(...any) error) error {
		var id string
		var e model.AdeRepoEnv
		if err := sc(&id, &e.Name, &e.DeployedShaScript); err != nil {
			return err
		}
		if repo, ok := l.codeRepos[id]; ok {
			c := l.get(repo)
			c.Environments = append(c.Environments, e)
		}
		return nil
	})
}

func eachRow(db *sql.DB, query string, f func(scan func(...any) error) error) error {
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("repocfg: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := f(rows.Scan); err != nil {
			return fmt.Errorf("repocfg: scan: %w", err)
		}
	}
	return rows.Err()
}

// legacySettings reads the stored leaves with the same validity filters the old table repo used, plus
// the file schema's: a leaf the file could not hold stays at its default.
func legacySettings(stored map[string]json.RawMessage) model.GitRepoSettings {
	r := model.DefaultGitRepoSettings()
	appsettings.LeafValid(stored, "graphPageSize", &r.GraphPageSize, appsettings.InRange(model.GraphPageSizeMin, model.GraphPageSizeMax))
	appsettings.LeafValid(stored, "graphScope", &r.GraphScope, model.ValidGraphScope)
	appsettings.Leaf(stored, "stashShowInGraph", &r.StashShowInGraph)
	appsettings.Leaf(stored, "stashIncludeUntracked", &r.StashIncludeUntracked)
	appsettings.LeafValid(stored, "reviewBaseCandidates", &r.ReviewBaseCandidates, func(v []string) bool { return !slices.Contains(v, "") })
	appsettings.LeafValid(stored, "pullStrategy", &r.PullStrategy, model.ValidPullStrategy)
	appsettings.LeafValid(stored, "gitHostProvider", &r.GitHostProvider, func(v string) bool {
		return v == "auto" || v == "off" || appsettings.ValidProviderID(v)
	})
	appsettings.LeafValid(stored, "worktreePrepareScript", &r.WorktreePrepareScript, func(v string) bool { return len(v) <= MaxPrepareBytes })
	appsettings.LeafValid(stored, "worktreePrepareTimeout", &r.WorktreePrepareTimeout, func(v string) bool {
		_, err := model.ParsePrepareTimeout(v)
		return err == nil
	})
	appsettings.LeafValid(stored, "worktreeBasePath", &r.WorktreeBasePath, func(v string) bool { return v == "" || absolutePath(v) })
	appsettings.Leaf(stored, "checkoutAutoStash", &r.CheckoutAutoStash)
	return r
}

// writeLegacy writes a file per config that has anything beyond the defaults and no file yet.
func (s *Store) writeLegacy(configs []Config) error {
	have := map[string]bool{}
	for _, e := range s.folder.List() {
		if e.Value != nil {
			have[Key(e.Value.Path)] = true
		}
	}
	for _, c := range configs {
		f := FileOf(c)
		if have[c.Path] || f.isDefault() {
			continue
		}
		if !absolutePath(c.Path) {
			slog.Warn("repo config: skip legacy settings of a relative path", "scope", "config")
			continue
		}
		text, err := configfs.RenderYAML(f, header)
		if err != nil {
			return err
		}
		if _, prob := s.folder.Decode("", text); prob != nil {
			slog.Warn("repo config: exported file is invalid", "scope", "config", "line", prob.Line)
		}
		name, err := s.freeName(slug(c.Path))
		if err != nil {
			return err
		}
		if err := configfs.WriteAtomic(filepath.Join(s.folder.Dir, name), text, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func dropLegacy(db *sql.DB) error {
	hasNickname, err := columnExists(db, "ade_repo_config", "nickname")
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("repocfg: begin drop: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	for _, t := range []string{"git_repo_settings", "ade_repo_integration", "ade_repo_envs"} {
		if _, err := tx.Exec(`DROP TABLE IF EXISTS ` + t); err != nil {
			return fmt.Errorf("repocfg: drop %s: %w", t, err)
		}
	}
	if hasNickname {
		if _, err := tx.Exec(`ALTER TABLE ade_repo_config DROP COLUMN nickname`); err != nil {
			return fmt.Errorf("repocfg: drop nickname: %w", err)
		}
	}
	return tx.Commit()
}
