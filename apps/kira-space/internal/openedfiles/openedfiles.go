// Package openedfiles turns external file opens (Finder's Open With, a double-click once Kira
// Space is the default app) into a new window that shows those files read-only.
package openedfiles

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/filetypes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// coalesceDelay is how long after the last openFile: a batch waits for more.
const coalesceDelay = 300 * time.Millisecond

// Target is one file to show. RepoID and RelPath are set when the file lies in an imported
// repository (blame and git context stay available); otherwise only Path is, absolute.
type Target struct {
	Path    string `json:"path"`
	RepoID  string `json:"repoId"`
	RelPath string `json:"relPath"`
	// Line and Col place the cursor, 1-based; zero means none.
	Line int `json:"line,omitempty"`
	Col  int `json:"col,omitempty"`
}

// RepoLister is the imported-repository list.
type RepoLister interface {
	List() ([]model.CodeRepo, error)
}

// Resolve classifies path against the imported repositories. It requires an existing regular file
// whose extension is in the vocabulary. Hidden repositories count: hidden only filters the Git panel.
// A git repository that is not imported is never imported here.
func Resolve(path string, repos []model.CodeRepo) (Target, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Target{}, err
	}
	if !filetypes.Supports(abs) {
		return Target{}, fmt.Errorf("unsupported extension %q", filepath.Ext(abs))
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Target{}, err
	}
	info, err := os.Stat(real)
	if err != nil {
		return Target{}, err
	}
	if !info.Mode().IsRegular() {
		return Target{}, errors.New("not a regular file")
	}
	var best *model.CodeRepo
	var bestRoot string
	for i := range repos {
		root, err := filepath.EvalSymlinks(repos[i].Root)
		if err != nil || !within(root, real) {
			continue
		}
		if best == nil || len(root) > len(bestRoot) {
			best, bestRoot = &repos[i], root
		}
	}
	if best == nil {
		return Target{Path: abs}, nil
	}
	rel, err := filepath.Rel(bestRoot, real)
	if err != nil {
		return Target{}, err
	}
	if _, err := codeworkspace.ValidateRelPath(best.Root, rel); err != nil {
		return Target{}, err
	}
	return Target{RepoID: best.ID, RelPath: filepath.ToSlash(rel)}, nil
}

func within(root, p string) bool {
	return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, string(filepath.Separator))+string(filepath.Separator))
}

// Service owns the opened-files lifecycle: coalesce, resolve, open a keyed window, hand the
// targets to that window's page once.
type Service struct {
	Repos RepoLister
	// OpenWindow opens a new workbench window under key; BindShell sets it.
	OpenWindow func(key string)

	co      *Coalescer
	mu      sync.Mutex
	pending map[string][]Target
}

// New builds a Service whose Add batches with the production timer.
func New(repos RepoLister) *Service { return NewWithAfter(repos, RealAfter) }

// NewWithAfter is New with an injected timer.
func NewWithAfter(repos RepoLister, after After) *Service {
	s := &Service{Repos: repos, pending: map[string][]Target{}}
	s.co = NewCoalescer(coalesceDelay, after, s.Open)
	return s
}

// Add queues one externally opened path; the batch opens one window.
func (s *Service) Add(path string) {
	if path != "" {
		s.co.Add(path)
	}
}

// Close drops any batch still waiting; called at teardown.
func (s *Service) Close() { s.co.Close() }

// Open resolves paths and opens one window for the ones that qualify. A path that does not qualify
// is logged and skipped; none qualifying opens no window.
func (s *Service) Open(paths []string) {
	repos := s.repos()
	var targets []Target
	for _, p := range paths {
		t, err := Resolve(p, repos)
		if err != nil {
			slog.Warn("opened files: skip path", "scope", "openedfiles", "path", p, "err", err)
			continue
		}
		targets = append(targets, t)
	}
	s.deliver(targets)
}

// deliver is the one place that picks a window for targets.
func (s *Service) deliver(targets []Target) {
	if len(targets) == 0 || s.OpenWindow == nil {
		return
	}
	key := uuid.NewString()
	s.mu.Lock()
	s.pending[key] = targets
	s.mu.Unlock()
	s.OpenWindow(key)
}

// Pending returns windowKey's targets once; a second call returns none.
func (s *Service) Pending(windowKey string) []Target {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.pending[windowKey]
	delete(s.pending, windowKey)
	return slices.Clone(t)
}

// Forget drops windowKey's targets: the window closed before its page pulled them.
func (s *Service) Forget(windowKey string) {
	s.mu.Lock()
	delete(s.pending, windowKey)
	s.mu.Unlock()
}

// Read reads a standalone file for the viewer. path must be absolute, clean and carry a vocabulary
// extension.
//
// Wails-bound services are reachable only from this app's own webviews; the mobile server serves
// its own route table and the -tags server build is test-only. The renderer already holds broader
// powers (terminals, git writes), so the extension filter only keeps this call to what the feature
// opens. A persisted allow-list would add a table and guard nothing the renderer cannot already do.
func (s *Service) Read(path string) (codeworkspace.FileContent, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return codeworkspace.FileContent{}, errors.New("path must be absolute and clean")
	}
	if !filetypes.Supports(path) {
		return codeworkspace.FileContent{}, fmt.Errorf("unsupported extension %q", filepath.Ext(path))
	}
	return codeworkspace.ReadFile(path, filepath.Base(path))
}
