package ade

import (
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/internal/fileservice"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpath"
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

	mu      sync.Mutex
	tasks   map[string]struct{} // task ids with a save since the last push
	rescan  bool
	stopped bool
	done    chan struct{}
}

// idleWatchDepth is the deepest watched dir below the files root: <task>/proofs/<slug>.
const idleWatchDepth = taskWatchDepth + 1

// startIdleWatch starts the board-wide watcher once; Close stops it.
func (b *TaskBoard) startIdleWatch() {
	df := b.files
	if df == nil {
		return
	}
	b.watchMu.Lock()
	defer b.watchMu.Unlock()
	if b.ctx.Err() != nil || b.idle != nil {
		return
	}
	if err := fileservice.Host().MkdirAll(df.root); err != nil {
		slog.Warn(taskWatchEvents, "scope", "ade", "err", err)
		return
	}
	iw := &idleWatch{b: b, root: df.root, nroot: gitpath.CleanNFC(df.root), tasks: map[string]struct{}{}, done: make(chan struct{})}
	w, err := fileservice.Host().Watch(fileservice.WatchOptions{Names: []string{df.root}, MaxDepth: idleWatchDepth, SkipDir: iw.skipDir})
	if err != nil {
		slog.Warn(taskWatchEvents, "scope", "ade", "err", err)
		return // the banner still refreshes on focus and Refresh
	}
	slog.Info(taskWatchEvents, "scope", "ade", "idle", true, "backend", fileservice.WatchBackend())
	iw.w = w
	iw.deb = fileservice.NewDebouncer(taskWatchQuiet, taskWatchMaxWait, iw.fire)
	b.idle = iw
	go iw.loop()
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
		return strings.HasPrefix(segs[0], ".")
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
	iw.queue(seg)
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

func (iw *idleWatch) queue(taskID string) {
	iw.mu.Lock()
	if iw.stopped {
		iw.mu.Unlock()
		return
	}
	iw.tasks[taskID] = struct{}{}
	iw.mu.Unlock()
	iw.deb.Schedule()
}

// fire pushes the board for tasks saved since the last push, leaving out tasks an agent watcher
// covers: it pushes after its own pass, and an earlier push would show the agent's unapplied save
// as a banner row.
func (iw *idleWatch) fire() {
	iw.mu.Lock()
	ids := make([]string, 0, len(iw.tasks))
	for id := range iw.tasks {
		ids = append(ids, id)
	}
	rescan := iw.rescan
	iw.tasks, iw.rescan = map[string]struct{}{}, false
	iw.mu.Unlock()
	if rescan {
		iw.b.notifyBoardNow(adewire.BoardChangedEvent{})
		return
	}
	iw.b.watchMu.Lock()
	idle := ids[:0]
	for _, id := range ids {
		if iw.b.watches[id] == nil {
			idle = append(idle, id)
		}
	}
	iw.b.watchMu.Unlock()
	if len(idle) == 0 {
		return
	}
	sort.Strings(idle)
	iw.b.notifyBoardNow(taskChanged(idle...))
}

func (iw *idleWatch) stop() {
	iw.mu.Lock()
	iw.stopped = true
	iw.mu.Unlock()
	iw.deb.Stop() // waits for an in-flight push
	_ = iw.w.Close()
	<-iw.done
}
