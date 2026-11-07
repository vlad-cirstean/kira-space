// Package configsync pulls shared config files from one git repository into <home>/config. Pull
// only: Kira never commits or pushes to the shared repo.
package configsync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// ValidateFunc runs a module's own decoder over shared text; "" when valid, else a message that
// never echoes a value.
type ValidateFunc func(file string, data []byte) string

// AuthFunc runs fn with the git env that routes credential prompts to the window of windowKey.
type AuthFunc func(ctx context.Context, windowKey string, fn func(env []string) error) error

// Options wires the service.
type Options struct {
	Home    string
	Runner  gitclient.Runner
	GitPath func(ctx context.Context) (string, error)
	// Auth is used by explicit fetches only; a background fetch never prompts.
	Auth AuthFunc
	// Validate maps a module to its decoder, for shared files and merged combined files.
	Validate map[string]ValidateFunc
	// SecretUnset reports whether a ${secret:name} of module has no local value.
	SecretUnset func(module, name string) bool
	// Emit receives the status after every change.
	Emit func(Status)
	Now  func() time.Time
	// Write puts a config file on disk during an apply; a test replaces it to inject a failure.
	Write func(path string, data []byte) error
	// StartDelay is the wait before the first background fetch; default one minute.
	StartDelay time.Duration
}

// ModuleInfo is one manifest module in the Settings table.
type ModuleInfo struct {
	Name        string   `json:"name"`
	Mode        string   `json:"mode"`
	Modes       []string `json:"modes"`
	Units       int      `json:"units"`
	Deactivated int      `json:"deactivated"`
}

// Status is what Settings and the toast show.
type Status struct {
	Configured   bool         `json:"configured"`
	URL          string       `json:"url"`
	Branch       string       `json:"branch"`
	FetchMinutes int          `json:"fetchMinutes"`
	Fetched      string       `json:"fetched"`
	Applied      string       `json:"applied"`
	FetchedAt    string       `json:"fetchedAt"`
	AppliedAt    string       `json:"appliedAt"`
	Fetching     bool         `json:"fetching"`
	Problem      *Problem     `json:"problem"`
	Changes      int          `json:"changes"`
	Available    bool         `json:"available"`
	ShowToast    bool         `json:"showToast"`
	Modules      []ModuleInfo `json:"modules"`
}

// ErrKind classifies a service error for the bridge.
type ErrKind int

const (
	ErrBadRequest ErrKind = iota
	ErrNotFound
	ErrConflict
	ErrInternal
)

// Error carries a catalog key; the bridge turns it into a wire error.
type Error struct {
	Kind   ErrKind
	Key    string
	Params map[string]string
	Cause  error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Key + ": " + e.Cause.Error()
	}
	return e.Key
}

func (e *Error) Unwrap() error { return e.Cause }

func errf(kind ErrKind, key string) *Error { return &Error{Kind: kind, Key: key} }

func internal(err error) *Error {
	return &Error{Kind: ErrInternal, Key: "backend.space.configSync.internal.body", Cause: err}
}

const (
	previewTTL   = 10 * time.Minute
	fetchTimeout = 2 * time.Minute
	// promptTimeout leaves time for the user to answer a credential prompt.
	promptTimeout = 10 * time.Minute
	// maxSessions bounds open previews; the oldest is dropped past it.
	maxSessions     = 8
	backoffCap      = 60 * time.Minute
	defaultStartLag = time.Minute
	keepBackups     = 10
)

type moduleCount struct{ units, deactivated int }

type session struct {
	token   string
	expires time.Time
	plan    *plan
}

// Service owns the sync state, the mirror and the background fetch.
type Service struct {
	o      Options
	mirror *mirror

	// opMu serializes everything that reads or writes config files and .sync.
	opMu sync.Mutex
	// mu guards the fields below; held only briefly.
	mu       sync.Mutex
	loaded   bool
	set      Settings
	st       State
	changes  int
	counts   map[string]moduleCount
	fetching bool
	sessions map[string]*session
	// interrupted is true while .sync/apply.json holds an apply no recovery settled.
	interrupted bool

	fetchMu sync.Mutex

	timerMu sync.Mutex
	timer   *time.Timer
	bo      *backoff.ExponentialBackOff
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// New builds a Service; it touches nothing on disk until used.
func New(o Options) *Service {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Write == nil {
		o.Write = func(path string, data []byte) error { return configfs.WriteAtomic(path, data, 0o644) }
	}
	if o.StartDelay <= 0 {
		o.StartDelay = defaultStartLag
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		o: o, ctx: ctx, cancel: cancel, counts: map[string]moduleCount{}, sessions: map[string]*session{},
		mirror: &mirror{dir: filepath.Join(syncDir(o.Home), mirrorDir), runner: o.Runner, gitPath: o.GitPath},
	}
}

func (s *Service) now() time.Time { return s.o.Now() }

// load reads settings and state once. mu is held.
func (s *Service) load() error {
	if s.loaded {
		return nil
	}
	set, err := loadSettings(s.o.Home)
	if err != nil {
		s.set, s.st = Settings{}, State{Problem: &Problem{Kind: ProblemSettings, Message: err.Error()}}
		s.loaded = true
		return err
	}
	s.set, s.st, s.loaded = set, loadState(s.o.Home), true
	return nil
}

// Start arms the background fetch and recomputes what is waiting.
func (s *Service) Start() {
	s.opMu.Lock()
	s.mu.Lock()
	_ = s.load()
	configured := s.set.Repo != nil
	s.mu.Unlock()
	s.checkJournal()
	if configured {
		s.refreshLocked(s.ctx)
	}
	s.opMu.Unlock()
	if configured {
		s.arm(s.o.StartDelay)
	}
}

// Stop cancels the timer and any running fetch.
func (s *Service) Stop() {
	s.timerMu.Lock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timerMu.Unlock()
	s.cancel()
}

// statusLocked builds the status. mu is held.
func (s *Service) statusLocked() Status {
	st := Status{Problem: s.st.Problem, Fetching: s.fetching, Modules: []ModuleInfo{}}
	if s.interrupted {
		st.Problem = &Problem{Kind: ProblemInterrupted, Message: i18n.T("backend.space.configSync.interrupted.body", nil)}
	}
	if s.set.Repo == nil {
		return st
	}
	st.Configured, st.URL, st.Branch, st.FetchMinutes = true, s.set.Repo.URL, s.set.branch(), s.set.minutes()
	st.Fetched, st.Applied, st.FetchedAt, st.AppliedAt = s.st.Fetched, s.st.Applied, s.st.FetchedAt, s.st.AppliedAt
	st.Changes = s.changes
	st.Available = s.st.Fetched != "" && s.st.Fetched != s.st.Applied && s.changes > 0 && (s.st.Problem == nil || s.st.Problem.Kind != ProblemManifest)
	st.ShowToast = st.Available && s.st.Dismissed != s.st.Fetched
	for _, m := range syncModules {
		if !slices.Contains(s.st.Manifest, m) {
			continue
		}
		c := s.counts[m]
		st.Modules = append(st.Modules, ModuleInfo{Name: m, Mode: s.set.mode(m), Modes: modesOf(m), Units: c.units, Deactivated: c.deactivated})
	}
	return st
}

// Status is the current status.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load()
	return s.statusLocked()
}

func (s *Service) emit() {
	if s.o.Emit != nil {
		s.o.Emit(s.Status())
	}
}

// refreshLocked recomputes the change count and per-module counts from the fetched commit. opMu is held.
func (s *Service) refreshLocked(ctx context.Context) {
	s.mu.Lock()
	set, fetched := s.set, s.st.Fetched
	s.mu.Unlock()
	if fetched == "" || set.Repo == nil {
		return
	}
	tree, se := s.mirror.readTree(ctx, fetched)
	if se != nil {
		s.setProblem(se.problem())
		return
	}
	p, err := s.buildPlan(tree, set)
	if err != nil {
		s.setProblem(&Problem{Kind: ProblemGit, Message: err.Error()})
		return
	}
	counts := map[string]moduleCount{}
	for _, r := range p.rows() {
		c := counts[r.Module]
		c.units++
		if r.Deactivated {
			c.deactivated++
		}
		counts[r.Module] = c
	}
	s.mu.Lock()
	s.changes, s.counts, s.st.Manifest = p.changes(), counts, tree.modules
	_ = saveState(s.o.Home, s.st)
	s.mu.Unlock()
}

func (s *Service) setProblem(p *Problem) {
	s.mu.Lock()
	s.st.Problem = p
	if p != nil && p.Kind == ProblemManifest {
		s.changes = 0
	}
	_ = saveState(s.o.Home, s.st)
	s.mu.Unlock()
}

// SetRepo stores the repository, forgets the previous one's mirror and state, and fetches.
func (s *Service) SetRepo(ctx context.Context, url, branch, windowKey string) (Status, error) {
	url = strings.TrimSpace(url)
	if branch = strings.TrimSpace(branch); branch == "" {
		branch = defaultBranch
	}
	if validRepoURL(url) != nil {
		return Status{}, errf(ErrBadRequest, "backend.space.configSync.url.invalid.body")
	}
	if !validBranch(branch) {
		return Status{}, errf(ErrBadRequest, "backend.space.configSync.branch.invalid.body")
	}
	s.fetchMu.Lock()
	s.opMu.Lock()
	s.mu.Lock()
	_ = s.load()
	same := s.set.Repo != nil && s.set.Repo.URL == url && s.set.branch() == branch
	if !same {
		if err := os.RemoveAll(syncDir(s.o.Home)); err != nil {
			s.mu.Unlock()
			s.opMu.Unlock()
			s.fetchMu.Unlock()
			return Status{}, internal(err)
		}
		s.set = Settings{Repo: &Repo{URL: url, Branch: branch}}
		s.st, s.changes, s.counts, s.sessions = State{}, 0, map[string]moduleCount{}, map[string]*session{}
		if err := saveSettings(s.o.Home, s.set); err != nil {
			s.mu.Unlock()
			s.opMu.Unlock()
			s.fetchMu.Unlock()
			return Status{}, internal(err)
		}
	}
	s.mu.Unlock()
	s.opMu.Unlock()
	s.fetchMu.Unlock()
	return s.FetchNow(ctx, windowKey)
}

// RemoveRepo deletes the mirror, base, state and settings. Applied files stay as local files.
func (s *Service) RemoveRepo() (Status, error) {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.timerMu.Lock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timerMu.Unlock()
	s.mu.Lock()
	if err := os.RemoveAll(syncDir(s.o.Home)); err != nil {
		s.mu.Unlock()
		return Status{}, internal(err)
	}
	s.set, s.st, s.changes, s.counts, s.sessions, s.loaded = Settings{}, State{}, 0, map[string]moduleCount{}, map[string]*session{}, true
	st := s.statusLocked()
	s.mu.Unlock()
	if s.o.Emit != nil {
		s.o.Emit(st)
	}
	return st, nil
}

// mutate changes the settings under both locks, saves them and recomputes the counts.
func (s *Service) mutate(fn func(*Settings) *Error) (Status, error) {
	s.opMu.Lock()
	s.mu.Lock()
	_ = s.load()
	if s.set.Repo == nil {
		s.mu.Unlock()
		s.opMu.Unlock()
		return Status{}, errf(ErrNotFound, "backend.space.configSync.notConfigured.body")
	}
	next := s.set
	next.Modules = cloneMap(next.Modules)
	next.Deactivated = cloneLists(next.Deactivated)
	if e := fn(&next); e != nil {
		s.mu.Unlock()
		s.opMu.Unlock()
		return Status{}, e
	}
	if err := saveSettings(s.o.Home, next); err != nil {
		s.mu.Unlock()
		s.opMu.Unlock()
		return Status{}, internal(err)
	}
	s.set, s.sessions = next, map[string]*session{}
	s.mu.Unlock()
	s.refreshLocked(s.ctx)
	s.opMu.Unlock()
	s.emit()
	return s.Status(), nil
}

func cloneMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneLists(m map[string][]string) map[string][]string {
	out := make(map[string][]string, len(m)+1)
	for k, v := range m {
		out[k] = slices.Clone(v)
	}
	return out
}

// SetFetchMinutes sets the background interval: 0 is off, else 5 to 1440.
func (s *Service) SetFetchMinutes(n int) (Status, error) {
	if n != 0 && (n < minFetchMinutes || n > 1440) {
		return Status{}, errf(ErrBadRequest, "backend.space.configSync.interval.invalid.body")
	}
	st, err := s.mutate(func(set *Settings) *Error { set.FetchMinutes = &n; return nil })
	if err == nil {
		if n == 0 {
			s.disarm()
		} else {
			s.arm(time.Duration(n) * time.Minute)
		}
	}
	return st, err
}

// SetMode sets how module follows the repository.
func (s *Service) SetMode(module, mode string) (Status, error) {
	if !validMode(module, mode) {
		return Status{}, errf(ErrBadRequest, "backend.space.configSync.mode.invalid.body")
	}
	return s.mutate(func(set *Settings) *Error { set.Modules[module] = mode; return nil })
}

// SetDeactivated marks one shared unit (file, or file#entry) as never applied here, or clears it.
func (s *Service) SetDeactivated(module, unit string, off bool) (Status, error) {
	file, _, _ := strings.Cut(unit, "#")
	if !slices.Contains(syncModules, module) || !syncableFile(module, file) {
		return Status{}, errf(ErrBadRequest, "backend.space.configSync.unit.invalid.body")
	}
	return s.mutate(func(set *Settings) *Error {
		list := slices.DeleteFunc(set.Deactivated[module], func(u string) bool { return u == unit })
		if off {
			list = append(list, unit)
		}
		if set.Deactivated == nil {
			set.Deactivated = map[string][]string{}
		}
		set.Deactivated[module] = list
		return nil
	})
}

// Dismiss hides the toast for commit sha; the next new commit shows it again.
func (s *Service) Dismiss(sha string) (Status, error) {
	s.mu.Lock()
	_ = s.load()
	if s.set.Repo == nil {
		s.mu.Unlock()
		return Status{}, errf(ErrNotFound, "backend.space.configSync.notConfigured.body")
	}
	s.st.Dismissed = sha
	err := saveState(s.o.Home, s.st)
	s.mu.Unlock()
	if err != nil {
		return Status{}, internal(err)
	}
	s.emit()
	return s.Status(), nil
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// modeOf is the permission of the file at path, else of fallback, else 0644.
func modeOf(path, fallback string) os.FileMode {
	for _, p := range []string{path, fallback} {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil {
			return fi.Mode().Perm()
		}
	}
	return 0o644
}

// checkJournal refreshes interrupted from the journal file and reports it.
func (s *Service) checkJournal() bool {
	_, err := os.Stat(s.journalPath())
	s.mu.Lock()
	s.interrupted = err == nil
	s.mu.Unlock()
	return err == nil
}
