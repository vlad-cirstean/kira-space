package ade

import (
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/internal/fileservice"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpath"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
)

// idleWatch watches every task folder under the files root. A save in a task no agent watcher covers
// pushes the board for that task, so the Details panel restats; nothing is applied. The per-task
// taskWatch stays the only applier.
type idleWatch struct {
	b     *TaskBoard
	root  string
	nroot string // root, Clean and NFC: the spelling fswatch reports paths in
	w     *fileservice.Watcher
	deb   *fileservice.Debouncer

	// archived holds the folder names left unwatched at start: archived tasks. Read-only after start.
	archived map[string]bool

	mu      sync.Mutex
	tasks   map[string]map[string]struct{} // task id -> slash paths saved since the last push
	rescan  bool
	stopped bool
	done    chan struct{}
}

// idleWatchDepth is the deepest watched dir below the files root: <task>/proofs/<slug>.
const idleWatchDepth = taskWatchDepth + 1

// startIdleWatch starts the board-wide watcher once; Close stops it. The initial walk runs outside
// watchMu so an agent watchTask never waits for it. Folders of tasks archived at start stay unwatched
// (one inotify watch per directory otherwise grows with every archived task); RestoreTask restarts the
// watcher through rewatchRestored. A task archived mid-session stays watched until the next restart.
func (b *TaskBoard) startIdleWatch() {
	df := b.files
	if df == nil {
		return
	}
	b.watchMu.Lock()
	skip := b.ctx.Err() != nil || b.idle != nil
	b.watchMu.Unlock()
	if skip {
		return
	}
	if err := fileservice.Host().MkdirAll(df.root); err != nil {
		slog.Warn(taskWatchEvents, "scope", "ade", "err", err)
		return
	}
	iw := &idleWatch{b: b, root: df.root, nroot: gitpath.CleanNFC(df.root), archived: b.archivedFolders(), tasks: map[string]map[string]struct{}{}, done: make(chan struct{})}
	w, err := fileservice.Host().Watch(fileservice.WatchOptions{Names: []string{df.root}, MaxDepth: idleWatchDepth, SkipDir: iw.skipDir})
	if err != nil {
		slog.Warn(taskWatchEvents, "scope", "ade", "err", err)
		return // the banner still refreshes on focus and Refresh
	}
	iw.w = w
	iw.deb = fileservice.NewDebouncer(taskWatchQuiet, taskWatchMaxWait, iw.fire)
	b.watchMu.Lock()
	if b.ctx.Err() != nil || b.idle != nil { // closed, or another start won, while walking
		b.watchMu.Unlock()
		_ = w.Close()
		return
	}
	b.idle = iw
	b.watchMu.Unlock()
	slog.Info(taskWatchEvents, "scope", "ade", "idle", true, "backend", fileservice.WatchBackend())
	go iw.loop()
}

// archivedFolders is the set of task folder names of archived tasks.
func (b *TaskBoard) archivedFolders() map[string]bool {
	out := map[string]bool{}
	if b.deps.Tasks == nil {
		return out
	}
	ts, err := b.deps.Tasks.ListArchived()
	if err != nil {
		slog.Warn(taskWatchEvents, "scope", "ade", "err", err)
		return out
	}
	for _, t := range ts {
		out[pathSegment(t.ID)] = true
	}
	return out
}

// rewatchRestored restarts the idle watcher when the restored task's folder was left unwatched.
func (b *TaskBoard) rewatchRestored(taskID string) {
	b.watchMu.Lock()
	old := b.idle
	if old == nil || !old.archived[pathSegment(taskID)] {
		b.watchMu.Unlock()
		return
	}
	b.idle = nil
	b.watchMu.Unlock()
	old.stop()
	b.startIdleWatch()
}

// slashRel is the root-relative slash path of a reported path, "" outside the root or for the root itself.
func (iw *idleWatch) slashRel(name string) string {
	rel, err := filepath.Rel(iw.nroot, name)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// skipDir watches the root, each non-hidden task folder and the task-file folders below it.
func (iw *idleWatch) skipDir(p string) bool {
	rel := iw.slashRel(p)
	if rel == "" {
		return false
	}
	segs := strings.Split(rel, "/")
	if len(segs) == 1 {
		return strings.HasPrefix(segs[0], ".") || iw.archived[segs[0]]
	}
	return !watchedTopDirs[segs[1]] || hasHiddenSegment(rel)
}

func (iw *idleWatch) loop() {
	defer close(iw.done)
	for ev := range iw.w.Events() {
		iw.handle(ev)
	}
}

func (iw *idleWatch) handle(ev fileservice.Event) {
	if ev.Rescan {
		iw.mu.Lock()
		if iw.stopped {
			iw.mu.Unlock()
			return
		}
		iw.rescan = true
		iw.mu.Unlock()
		iw.deb.Schedule()
		return
	}
	if ev.Op == fileservice.Meta {
		return
	}
	seg, rest, ok := strings.Cut(iw.slashRel(ev.Path), "/")
	if !ok || seg == "" || strings.HasPrefix(seg, ".") {
		return
	}
	// A new dir under a watched top dir counts as well: the push restats the whole folder.
	if keepTaskFile(rest) == "" && !(watchedTopDirs[strings.SplitN(rest, "/", 2)[0]] && !hasHiddenSegment(rest) && iw.isNewDir(ev)) {
		return
	}
	iw.queue(seg, rest)
}

func (iw *idleWatch) isNewDir(ev fileservice.Event) bool {
	if !ev.Op.Has(fileservice.Create) && !ev.Op.Has(fileservice.Rename) {
		return false
	}
	if ev.Dir {
		return true
	}
	fi, err := fileservice.Host().Lstat(ev.Path)
	return err == nil && fi.Dir
}

func (iw *idleWatch) queue(taskID, rel string) {
	iw.mu.Lock()
	if iw.stopped {
		iw.mu.Unlock()
		return
	}
	if iw.tasks[taskID] == nil {
		iw.tasks[taskID] = map[string]struct{}{}
	}
	iw.tasks[taskID][rel] = struct{}{}
	iw.mu.Unlock()
	iw.deb.Schedule()
}

// fire pushes the board for tasks saved since the last push, leaving out tasks an agent covers: its
// watcher pushes after its own pass, and an earlier push would show the agent's unapplied save as a
// banner row. A task whose watcher failed to start still has its grant, so grants count too. Saves
// that match the stored index hash are Kira's own writes, which push the board themselves.
func (iw *idleWatch) fire() {
	iw.mu.Lock()
	saved := iw.tasks
	rescan := iw.rescan
	iw.tasks, iw.rescan = map[string]map[string]struct{}{}, false
	iw.mu.Unlock()
	if rescan {
		if ev, ok := iw.rescanEvent(); ok {
			iw.b.notifyBoardNow(ev)
		}
		return
	}
	candidates := make([]string, 0, len(saved))
	for id := range saved {
		candidates = append(candidates, id)
	}
	idle := iw.b.unwatched(candidates)
	out := idle[:0]
	for _, id := range idle {
		if iw.external(id, saved[id]) {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return
	}
	sort.Strings(out)
	iw.b.notifyBoardNow(taskChanged(out...))
}

// rescanEvent names every live task the idle watcher covers; ok is false when none is covered. A store
// failure leaves the unscoped event (every task may have changed).
func (iw *idleWatch) rescanEvent() (ev adewire.BoardChangedEvent, ok bool) {
	tasks, err := iw.b.deps.Tasks.ListLive()
	if err != nil {
		slog.Warn(taskWatchEvents, "scope", "ade", "err", err)
		return adewire.BoardChangedEvent{}, true
	}
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	idle := iw.b.unwatched(ids)
	if len(idle) == 0 {
		return adewire.BoardChangedEvent{}, false
	}
	sort.Strings(idle)
	return taskChanged(idle...), true
}

// unwatched keeps the ids with no agent watcher and no Space grant: the tasks the idle watcher covers.
func (b *TaskBoard) unwatched(ids []string) []string {
	b.watchMu.Lock()
	defer b.watchMu.Unlock()
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if b.watches[id] == nil && b.grants[id] == 0 {
			out = append(out, id)
		}
	}
	return out
}

// external reports whether any saved path differs from the index: a folder, a file the index does not
// hold or holds under another hash, or a file gone that the index still lists. Unreadable counts as external.
func (iw *idleWatch) external(taskID string, rels map[string]struct{}) bool {
	rows, err := iw.b.files.idx.Files(taskID)
	if err != nil {
		return true
	}
	hash := make(map[string]string, len(rows))
	for _, r := range rows {
		hash[r.RelPath] = r.SHA256
	}
	for rel := range rels {
		want, indexed := hash[rel]
		data, err := fileservice.Host().ReadFile(filepath.Join(iw.root, taskID, filepath.FromSlash(rel)))
		switch {
		case err != nil && !indexed && errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil, !indexed, taskfiles.Sum(data) != want:
			return true
		}
	}
	return false
}

func (iw *idleWatch) stop() {
	iw.mu.Lock()
	iw.stopped = true
	iw.mu.Unlock()
	iw.deb.Stop() // waits for an in-flight push
	_ = iw.w.Close()
	<-iw.done
}
