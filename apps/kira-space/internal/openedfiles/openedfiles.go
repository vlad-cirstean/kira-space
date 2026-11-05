// Package openedfiles turns external file opens (Finder's Open With, a double-click once Kira
// Space is the default app, a terminal path click) into tabs of one files window.
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

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/filetypes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// WindowKeyPrefix marks the files window's key; its row is ephemeral.
const WindowKeyPrefix = "files-"

// ChannelArrived signals the files window that targets wait in Pending.
const ChannelArrived = "kira:openedFiles:arrived"

// IsWindowKey reports whether key names the files window.
func IsWindowKey(key string) bool { return strings.HasPrefix(key, WindowKeyPrefix) }

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

// Service owns the opened-files lifecycle: coalesce, resolve, then raise the files window (or open
// one) and hand it the targets.
type Service struct {
	Repos RepoLister
	// OpenWindow opens a new window under key; BindShell sets it.
	OpenWindow func(key string)
	// FocusWindow raises key's window; false when no live window has the key.
	FocusWindow func(key string) bool
	// SetWindowTitle titles key's window.
	SetWindowTitle func(key, title string)
	// Events signals a live files window that targets arrived.
	Events interface {
		EmitTo(windowKey, name string, data any)
	}

	co      *Coalescer
	openMu  sync.Mutex
	mu      sync.Mutex
	pending map[string][]Target
	// filesKey is the live files window's key, "" when none.
	filesKey string
}

// New builds a Service whose Add batches with the production timer.
func New(repos RepoLister) *Service { return NewWithAfter(repos, RealAfter) }

// NewWithAfter is New with an injected timer.
func NewWithAfter(repos RepoLister, after After) *Service {
	s := &Service{Repos: repos, pending: map[string][]Target{}}
	s.co = NewCoalescer(coalesceDelay, after, s.Open)
	return s
}

// Add queues one externally opened path; the batch lands in the files window.
func (s *Service) Add(path string) {
	if path != "" {
		s.co.Add(path)
	}
}

// Close drops any batch still waiting; called at teardown.
func (s *Service) Close() { s.co.Close() }

// Open resolves paths and shows the ones that qualify in the files window. A path that does not
// qualify is logged and skipped; none qualifying touches no window.
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

// deliver is the one place that picks a window for targets: the live files window, else a new one.
// Focus, open, title and emit never run under mu (Focus runs on the main thread; Forget runs from
// WindowClosing and takes mu). A window closing between Focus and its drain drops these targets.
func (s *Service) deliver(targets []Target) {
	if len(targets) == 0 || s.OpenWindow == nil {
		return
	}
	s.openMu.Lock()
	defer s.openMu.Unlock()
	s.mu.Lock()
	key := s.filesKey
	if key != "" {
		s.pending[key] = append(s.pending[key], targets...)
	}
	s.mu.Unlock()
	if key != "" && s.FocusWindow != nil && s.FocusWindow(key) {
		if s.Events != nil {
			s.Events.EmitTo(key, ChannelArrived, nil)
		}
		return
	}
	s.mu.Lock()
	fresh := targets
	if key != "" {
		fresh = s.pending[key]
		delete(s.pending, key)
	}
	key = WindowKeyPrefix + ids.NewID()
	s.pending[key] = fresh
	s.filesKey = key
	s.mu.Unlock()
	s.OpenWindow(key)
	if s.SetWindowTitle != nil {
		s.SetWindowTitle(key, i18n.T("backend.space.openedFiles.window.title", nil))
	}
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
	if s.filesKey == windowKey {
		s.filesKey = ""
	}
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
