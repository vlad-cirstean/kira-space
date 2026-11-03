package repocfg

import (
	"database/sql"
	"log/slog"
	"reflect"
	"regexp"
	"slices"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Options builds a Store.
type Options struct {
	DB       *sql.DB  // config_last_valid and the legacy tables
	Dir      string   // <home>/repos
	GitHosts []string // registered Git host ids, listed in AGENTS.md
	Now      func() time.Time
}

// Change is one repo whose effective config moved. FromPatch marks a change the app itself wrote
// through Patch, so its writer already told the Git clients.
type Change struct {
	Path      string
	Old, New  Config
	FromPatch bool
}

// FileState is one file of the folder as the last load saw it.
type FileState struct {
	File string
	Hash string
	// Path is the repo the file names; "" when the file is invalid and has no last good value.
	Path     string
	Problem  *configfs.Problem
	LastGood bool
	// Duplicate: an earlier file names the same path, so this one applies nothing.
	Duplicate bool
}

// Store holds the repo files, an in-memory index of the effective config per repo, and the writes.
type Store struct {
	// OnChange gets each batch of changed repos after a load. OnReload fires after a load that
	// changed any file, value or problem. Both are set before Watch.
	OnChange func([]Change)
	OnReload func()

	o      Options
	folder *configfs.Folder[File]
	doc    configfs.DocModel

	op sync.Mutex // serializes loads and writes

	mu    sync.RWMutex
	index map[string]Config
	owner map[string]string // path -> file applying to it
	files []FileState

	stop func()
}

// New builds a Store with the schema assembled from the registered Git hosts; it creates nothing on disk.
func New(o Options) (*Store, error) {
	schema, err := assembleSchema(o.GitHosts)
	if err != nil {
		return nil, err
	}
	decode, doc, err := configfs.YAMLSchema[File](schema, rules()...)
	if err != nil {
		return nil, err
	}
	doc.Pattern, doc.Format, doc.KeyedBy, doc.Intro, doc.App, doc.Title = "<name>.yaml", "YAML", "path", intro, "Space", "Repos"
	s := &Store{
		o: o, doc: doc, index: map[string]Config{}, owner: map[string]string{},
		folder: &configfs.Folder[File]{
			Dir: o.Dir, Ext: ".yaml", Name: regexp.MustCompile(`^[^.].*\.yaml$`), Decode: decode,
			Projection: configfs.TableProjection{DB: o.DB, Folder: Folder}, Now: o.Now,
		},
	}
	return s, nil
}

// Start moves the legacy rows into files once, loads the folder and writes AGENTS.md.
func (s *Store) Start() error {
	if err := s.migrate(); err != nil {
		return err
	}
	s.Reload()
	if err := s.folder.WriteDocs(s.doc); err != nil {
		slog.Warn("repo config: write docs", "scope", "config", "err", err)
	}
	return nil
}

// Watch reloads when the folder changes. A failure is a warning: the files then apply at next start.
func (s *Store) Watch() {
	stop, err := s.folder.Watch(s.Reload)
	if err != nil {
		slog.Warn("repo config: watch", "scope", "config", "err", err)
		return
	}
	s.stop = stop
}

// Close stops the watch.
func (s *Store) Close() {
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
}

// Dir is the config folder.
func (s *Store) Dir() string { return s.folder.Dir }

// Repo is the effective config of a repo: its file's values, or the defaults.
func (s *Store) Repo(repoID string) Config {
	key := Key(repoID)
	s.mu.RLock()
	c, ok := s.index[key]
	s.mu.RUnlock()
	if !ok {
		return Defaults(key)
	}
	return c.clone()
}

// Settings is the git settings of a repo.
func (s *Store) Settings(repoID string) (model.GitRepoSettings, error) {
	return s.Repo(repoID).Settings, nil
}

// Files lists the folder's files as the last load saw them.
func (s *Store) Files() []FileState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.files)
}

// Value is a file as last loaded: its content, or the last good one while it is invalid.
func (s *Store) Value(file string) (File, bool) {
	e, ok := s.folder.Get(file)
	if !ok || e.Value == nil {
		return File{}, false
	}
	return *e.Value, true
}

// FileFor is the file applying to a repo, or "".
func (s *Store) FileFor(repoID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.owner[Key(repoID)]
}

// FileInfo is the file applying to a repo and its error, if any.
func (s *Store) FileInfo(repoID string) (string, *configfs.Problem) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	file := s.owner[Key(repoID)]
	for _, f := range s.files {
		if f.File == file {
			return file, f.Problem
		}
	}
	return "", nil
}

// Reload re-reads the folder.
func (s *Store) Reload() {
	s.op.Lock()
	l := s.load("")
	s.op.Unlock()
	s.finish(l)
}

type loaded struct {
	changes []Change
	moved   bool
}

func (s *Store) finish(l loaded) {
	if len(l.changes) > 0 && s.OnChange != nil {
		s.OnChange(l.changes)
	}
	if l.moved && s.OnReload != nil {
		s.OnReload()
	}
}

// load rebuilds the index from the folder; op is held. patched is the path a Patch just wrote.
func (s *Store) load(patched string) loaded {
	idx := map[string]Config{}
	owner := map[string]string{}
	var files []FileState
	for _, e := range s.folder.List() {
		st := FileState{File: e.File, Hash: e.Hash, Problem: e.Problem, LastGood: e.Problem != nil && e.Value != nil}
		if e.Value == nil {
			files = append(files, st)
			continue
		}
		cfg := e.Value.Config()
		st.Path = cfg.Path
		if first, taken := owner[cfg.Path]; taken {
			st.Duplicate = true
			if st.Problem == nil {
				st.Problem = s.duplicateProblem(e.File, first)
			}
			files = append(files, st)
			continue
		}
		owner[cfg.Path], idx[cfg.Path] = e.File, cfg
		files = append(files, st)
	}
	s.mu.Lock()
	oldIdx, oldFiles := s.index, s.files
	s.index, s.owner, s.files = idx, owner, files
	s.mu.Unlock()

	var changes []Change
	for path, cfg := range idx {
		old, had := oldIdx[path]
		if !had {
			old = Defaults(path)
		}
		if !reflect.DeepEqual(old, cfg) {
			changes = append(changes, Change{Path: path, Old: old, New: cfg, FromPatch: path == patched})
		}
	}
	for path, old := range oldIdx {
		if _, ok := idx[path]; !ok {
			changes = append(changes, Change{Path: path, Old: old, New: Defaults(path), FromPatch: path == patched})
		}
	}
	slices.SortFunc(changes, func(a, b Change) int {
		switch {
		case a.Path < b.Path:
			return -1
		case a.Path > b.Path:
			return 1
		}
		return 0
	})
	return loaded{changes, len(changes) > 0 || !reflect.DeepEqual(oldFiles, files)}
}

// duplicateProblem points at the path line of the later file.
func (s *Store) duplicateProblem(file, winner string) *configfs.Problem {
	p := &configfs.Problem{Path: "/path", Message: i18n.T("backend.space.repoFiles.duplicatePath.body", map[string]string{"file": winner})}
	if src, _, err := s.folder.Read(file); err == nil {
		var doc yaml.Node
		if yaml.Unmarshal(src, &doc) == nil && len(doc.Content) == 1 {
			m := doc.Content[0]
			for i := 0; i+1 < len(m.Content); i += 2 {
				if m.Content[i].Value == "path" {
					p.Line, p.Col = m.Content[i].Line, m.Content[i].Column
				}
			}
		}
	}
	return p
}

func (c Config) clone() Config {
	c.IntegrationBranches = slices.Clone(c.IntegrationBranches)
	c.Environments = slices.Clone(c.Environments)
	c.Settings.ReviewBaseCandidates = slices.Clone(c.Settings.ReviewBaseCandidates)
	return c
}
