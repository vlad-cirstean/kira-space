package gitclient

import (
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpath"
	"github.com/kirathecat/kira-studio/internal/fswatch"
)

// Signal is one coalesced, debounced repository-change notification (SPEC §6's repo.changed
// payload, minus the repo id gitsession attaches).
type Signal string

const (
	SignalRefsChanged     Signal = "refsChanged"
	SignalWorktreeChanged Signal = "worktreeChanged"
)

// debounceWindow is a leading-window debounce measured from the first event of a burst (G2 plan
// D11; SPEC §6): fires 200ms after the first event, not 200ms after the last, so a continuous
// `git fetch --prune` cannot starve the signal indefinitely.
const debounceWindow = 200 * time.Millisecond

// refIshNames are the basenames (after stripping a trailing .lock, F12) that mean "refs changed"
// when found directly inside commonDir or gitDir — SPEC §6's list plus BISECT_LOG, upstream's set
// verbatim (D9).
var refIshNames = map[string]bool{
	"HEAD":             true,
	"packed-refs":      true,
	"FETCH_HEAD":       true,
	"MERGE_HEAD":       true,
	"rebase-merge":     true,
	"rebase-apply":     true,
	"CHERRY_PICK_HEAD": true,
	"REVERT_HEAD":      true,
	"BISECT_LOG":       true,
	"sequencer":        true,
	// A hand-edited .git/config is the one legitimate out-of-app way to change a stack —
	// a human-readable store was deliberately chosen precisely so a user CAN edit it directly — and
	// without this, that edit produces no signal at all until something else touches a ref. Written
	// rarely enough (--set-upstream-to, remote add, and now this) that a spurious refsChanged costs
	// one debounced cache drop.
	"config": true,
}

// stripLockSuffix undoes git's own write-by-rename discipline (F12): every one of the paths above
// is written as "<name>.lock" and renamed onto "<name>", and the rename's source-side event names
// the .lock file — a classifier that doesn't strip it silently misses every ref/index write.
func stripLockSuffix(name string) string {
	return strings.TrimSuffix(name, ".lock")
}

// classify decides what path changing means for summary's repository, in upstream's own order:
// a path under commonDir/refs is always refsChanged; gitDir's own index is worktreeChanged;
// only then do the ref-ish basenames (checked in either commonDir or gitDir, so a linked
// worktree's own MERGE_HEAD/sequencer/rebase-* are covered too) apply.
//
// Both sides are normalized to NFC (not just Cleaned): without it, a repository at a non-ASCII
// path could see every rule below fall through on a form mismatch and the watcher would go
// permanently, silently dead rather than merely misclassifying one path.
func classify(summary RepoSummary, path string) (Signal, bool) {
	commonDir := gitpath.CleanNFC(summary.CommonDir)
	gitDir := gitpath.CleanNFC(summary.GitDir)
	path = gitpath.CleanNFC(path)

	if refsRoot := filepath.Join(commonDir, "refs"); path == refsRoot || strings.HasPrefix(path, refsRoot+string(filepath.Separator)) {
		return SignalRefsChanged, true
	}

	dir, base := filepath.Split(path)
	dir = filepath.Clean(dir)
	base = stripLockSuffix(base)

	if base == "index" && dir == gitDir {
		return SignalWorktreeChanged, true
	}
	if refIshNames[base] && (dir == commonDir || dir == gitDir) {
		return SignalRefsChanged, true
	}

	// A detached `worktree add`/`remove` writes only under commonDir/worktrees/<name>/ (its own
	// per-worktree HEAD, gitdir file, lock file) — for any worktree OTHER than this summary's own
	// gitDir, none of the two rules just above ever matches (dir is neither gitDir nor commonDir).
	// Checked LAST, after the more specific gitDir-scoped rules above, so this summary's OWN
	// worktree files (in particular its own index, which lives at exactly this same path) keep
	// resolving through the more specific rule that already names the right signal for them.
	// Treated as refsChanged, not a new third Signal kind: the same read (refs.list/worktree.list)
	// is what a client re-requests either way.
	if worktreesRoot := filepath.Join(commonDir, "worktrees"); path == worktreesRoot || strings.HasPrefix(path, worktreesRoot+string(filepath.Separator)) {
		return SignalRefsChanged, true
	}
	return "", false
}

// source is the event feed: *fswatch.Watcher, or a fake in tests.
type source interface {
	Events() <-chan fswatch.Event
	Close() error
}

// RepoWatcher watches one repository's .git directories and reports debounced, coalesced signals.
// Closing it stops its goroutine and closes Signals.
type RepoWatcher struct {
	summary RepoSummary
	src     source

	out  chan Signal
	stop chan struct{}
	done chan struct{}

	closeOnce sync.Once
}

// NewRepoWatcher starts watching summary's repository: commonDir's top, refs/** and worktrees,
// plus gitDir's top when it differs. fswatch reports paths in the spelling passed here (it
// resolves symlinks itself, which FSEvents needs), so classify compares against summary directly.
func NewRepoWatcher(summary RepoSummary) (*RepoWatcher, error) {
	commonDir := gitpath.CleanNFC(summary.CommonDir)
	gitDir := gitpath.CleanNFC(summary.GitDir)
	roots := []string{summary.CommonDir}
	if gitDir != commonDir {
		roots = append(roots, summary.GitDir)
	}
	src, err := fswatch.Watch(fswatch.Options{
		Roots:    roots,
		MaxDepth: fswatch.Unlimited,
		SkipDir: func(p string) bool {
			if gitDir != commonDir && (p == gitDir || strings.HasPrefix(p, gitDir+string(filepath.Separator))) {
				return true
			}
			rel, err := filepath.Rel(commonDir, p)
			if err != nil {
				return true
			}
			rel = filepath.ToSlash(rel)
			return !(rel == "worktrees" || rel == "refs" || strings.HasPrefix(rel, "refs/"))
		},
	})
	if err != nil {
		return nil, err
	}
	return newRepoWatcherWith(summary, src), nil
}

// newRepoWatcherWith is the shared constructor: NewRepoWatcher calls it with a real source, and
// watcher_test.go calls it with a fake to drive the debounce/Rescan rule without a filesystem.
func newRepoWatcherWith(summary RepoSummary, src source) *RepoWatcher {
	w := &RepoWatcher{
		summary: summary,
		src:     src,
		out:     make(chan Signal, 2), // at most one of each kind pending per debounce firing.
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}

	watchedPaths := []string{summary.CommonDir}
	if summary.GitDir != summary.CommonDir {
		watchedPaths = append(watchedPaths, summary.GitDir)
	}
	slog.Info("gitclient: repo watcher started", "scope", "watcher",
		"backend", fswatch.Backend(), "paths", watchedPaths)

	go w.run()
	return w
}

// Signals is the debounced, coalesced output — at most one refsChanged and one worktreeChanged per
// debounce firing (D11), closed once the watcher stops.
func (w *RepoWatcher) Signals() <-chan Signal { return w.out }

// Close stops the watcher goroutine and the underlying backend. Idempotent.
func (w *RepoWatcher) Close() error {
	w.closeOnce.Do(func() {
		close(w.stop)
		<-w.done
	})
	return w.src.Close()
}

func (w *RepoWatcher) emit(sig Signal) {
	select {
	case w.out <- sig:
	case <-w.stop:
	}
}

// run is the watcher's single goroutine: it owns the two pending flags and the leading-
// window debounce timer, and is the only place that reads w.src.Events() or writes pendingRefs/
// pendingWorktree, so neither needs a lock. Rescan means "something changed and we don't know
// what" — coalesced overflow/dropped events on either backend — and raises both signals.
func (w *RepoWatcher) run() {
	defer close(w.done)
	defer close(w.out)

	var timer *time.Timer
	var timerC <-chan time.Time
	var pendingRefs, pendingWorktree bool

	armIfNeeded := func() {
		if timer == nil && (pendingRefs || pendingWorktree) {
			timer = time.NewTimer(debounceWindow)
			timerC = timer.C
		}
	}
	fire := func() {
		if pendingRefs {
			w.emit(SignalRefsChanged)
		}
		if pendingWorktree {
			w.emit(SignalWorktreeChanged)
		}
		pendingRefs, pendingWorktree = false, false
		timer, timerC = nil, nil
	}

	for {
		select {
		case ev, ok := <-w.src.Events():
			if !ok {
				return
			}
			if ev.Rescan {
				pendingRefs, pendingWorktree = true, true
			} else {
				switch sig, matched := classify(w.summary, ev.Path); {
				case matched && sig == SignalRefsChanged:
					pendingRefs = true
				case matched && sig == SignalWorktreeChanged:
					pendingWorktree = true
				}
			}
			armIfNeeded()

		case <-timerC:
			fire()

		case <-w.stop:
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}
