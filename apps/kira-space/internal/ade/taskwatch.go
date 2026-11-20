package ade

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/kirathecat/kira-studio/internal/agentmcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
)

// taskwatch.go: while an agent holds a Space grant on a task, its folder is watched and each edit the
// agent saves is applied after a short quiet, as the agent's own submit with no reason. Only the event
// paths go in, so an earlier edit of the user stays on the disk banner. The pass is a Watch pass: it
// applies free changes only. An ask-first change is deferred untouched (no row, no fileErrors entry,
// no word to the agent), because the save may be the user's own in an editor; the banner shows it.

const (
	taskWatchQuiet = 300 * time.Millisecond
	// taskWatchMaxWait caps how long a steady stream of saves can defer the pass.
	taskWatchMaxWait = 2 * time.Second
	taskWatchEvents  = "ade task watch"
	// taskWatchDepth is the deepest folder under the task folder that is watched: proofs/<slug>.
	taskWatchDepth = 2
)

// maxTaskWatches caps the live task watchers, 0 for none. kqueue (macOS) holds one fd per watched
// folder and per file in it, so many busy tasks could reach kern.maxfilesperproc; inotify has no such cost.
// A task over the cap is unwatched and its edits apply at the finish flush.
func maxTaskWatches() int {
	if runtime.GOOS == "darwin" {
		return 8
	}
	return 0
}

// watchedTopDirs are the folders of the task folder that hold task files.
var watchedTopDirs = map[string]bool{"notes": true, "handovers": true, "proofs": true}

// callerRef is one live agent grant on the task; a session that registers twice counts twice.
type callerRef struct {
	call agentmcp.SpaceCall
	n    int
}

type taskWatch struct {
	b      *TaskBoard
	taskID string
	root   string
	w      *fsnotify.Watcher

	mu      sync.Mutex
	callers map[string]*callerRef
	paths   map[string]struct{} // slash paths relative to root since the last pass
	first   time.Time           // when the first path of the current batch was queued
	timer   *time.Timer
	stopped bool
	passes  sync.WaitGroup // in-flight pass; stop waits for it
	done    chan struct{}
}

func spaceCallOf(g agentmcp.Grant) agentmcp.SpaceCall {
	return agentmcp.SpaceCall{TaskID: g.TaskID, RunID: g.RunID, StepID: g.StepID, SessionID: g.SessionID, Orchestrator: g.Orchestrator}
}

func callerKey(c agentmcp.SpaceCall) string { return cmpNonEmpty(c.SessionID, c.RunID) }

// registerSpaceGrant registers an agent grant and, for a Space grant on a task, starts watching the
// task folder until the grant is released.
func (b *TaskBoard) registerSpaceGrant(g agentmcp.Grant) (string, func(), error) {
	cfg, release, err := b.agent.Register(g)
	if err != nil || !g.Space || g.TaskID == "" || b.files == nil {
		return cfg, release, err
	}
	call := spaceCallOf(g)
	b.watchTask(call)
	return cfg, func() {
		b.unwatchTask(call)
		release()
	}, nil
}

func (b *TaskBoard) watchTask(call agentmcp.SpaceCall) {
	b.watchMu.Lock()
	defer b.watchMu.Unlock()
	if b.grants == nil {
		b.grants = map[string]int{}
	}
	b.grants[call.TaskID]++
	tw := b.watches[call.TaskID]
	if tw == nil {
		if limit := maxTaskWatches(); limit > 0 && len(b.watches) >= limit {
			slog.Warn(taskWatchEvents, "scope", "ade", "task", call.TaskID, "err", "watcher cap reached")
			return // the finish flush still applies the edits
		}
		var err error
		if tw, err = b.newTaskWatch(call.TaskID); err != nil {
			slog.Warn(taskWatchEvents, "scope", "ade", "task", call.TaskID, "err", err)
			return // the finish flush still applies the edits
		}
		if b.watches == nil {
			b.watches = map[string]*taskWatch{}
		}
		b.watches[call.TaskID] = tw
	}
	tw.mu.Lock()
	ref := tw.callers[callerKey(call)]
	if ref == nil {
		ref = &callerRef{call: call}
		tw.callers[callerKey(call)] = ref
	}
	ref.n++
	tw.mu.Unlock()
}

func (b *TaskBoard) newTaskWatch(taskID string) (*taskWatch, error) {
	root, err := b.TaskFolder(taskID)
	if err != nil || root == "" {
		return nil, errors.Join(err, errors.New("no task folder"))
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	tw := &taskWatch{b: b, taskID: taskID, root: root, w: w, callers: map[string]*callerRef{}, paths: map[string]struct{}{}, done: make(chan struct{})}
	tw.addTree(root)
	go tw.loop()
	return tw, nil
}

func (b *TaskBoard) unwatchTask(call agentmcp.SpaceCall) {
	b.watchMu.Lock()
	if b.grants[call.TaskID]--; b.grants[call.TaskID] <= 0 {
		delete(b.grants, call.TaskID)
	}
	live := b.grants[call.TaskID]
	tw := b.watches[call.TaskID]
	if tw == nil {
		b.watchMu.Unlock()
		if live == 0 { // never watched: errors of explicit submits and the finish flush still clear
			b.clearFileErrors(call.TaskID)
		}
		return
	}
	tw.mu.Lock()
	if ref := tw.callers[callerKey(call)]; ref != nil {
		if ref.n--; ref.n <= 0 {
			delete(tw.callers, callerKey(call))
		}
	}
	left := len(tw.callers)
	tw.mu.Unlock()
	if left > 0 {
		b.watchMu.Unlock()
		return
	}
	delete(b.watches, call.TaskID)
	b.watchWG.Add(1)
	b.watchMu.Unlock()
	// A release can run under the task lock (CloseSession), which the final pass takes: run it aside.
	go func() {
		defer b.watchWG.Done()
		tw.stop(&call)
		b.watchMu.Lock()
		_, again := b.watches[call.TaskID]
		b.watchMu.Unlock()
		if !again {
			b.clearFileErrors(call.TaskID)
		}
	}()
}

// stopTaskWatches ends every watcher at board close; their last edits are not applied.
func (b *TaskBoard) stopTaskWatches() {
	b.watchMu.Lock()
	all := b.watches
	b.watches = nil
	b.watchMu.Unlock()
	for _, tw := range all {
		tw.stop(nil)
	}
	b.watchWG.Wait()
}

// addTree watches dir and every task-file folder below it, and returns the files found there.
func (t *taskWatch) addTree(dir string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rerr := filepath.Rel(t.root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !d.IsDir() {
			if d.Type()&fs.ModeSymlink == 0 && rel != "." {
				files = append(files, rel)
			}
			return nil
		}
		segs := strings.Split(rel, "/")
		switch {
		case rel == ".":
		case strings.HasPrefix(d.Name(), "."), len(segs) > taskWatchDepth, !watchedTopDirs[segs[0]]:
			return filepath.SkipDir
		}
		if err := t.w.Add(p); err != nil {
			slog.Debug(taskWatchEvents, "scope", "ade", "task", t.taskID, "dir", rel, "err", err)
		}
		return nil
	})
	return files
}

// relOf is the task-folder-relative slash path of an event, "" when the event is not about a task file.
func (t *taskWatch) relOf(name string) string {
	rel, err := filepath.Rel(t.root, name)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ""
	}
	rel = filepath.ToSlash(rel)
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, ".") {
			return ""
		}
	}
	switch rel {
	case taskfiles.ReadmeFile, taskfiles.TaskFile, taskfiles.StateFile: // Kira's own; finish still restores tampering
		return ""
	}
	if taskfiles.Classify(rel).Kind == taskfiles.KindIgnored && taskfiles.ProofFolderHint(rel) == "" {
		return ""
	}
	return rel
}

func (t *taskWatch) loop() {
	defer close(t.done)
	for {
		select {
		case ev, ok := <-t.w.Events:
			if !ok {
				return
			}
			t.handle(ev)
		case err, ok := <-t.w.Errors:
			if !ok {
				return
			}
			slog.Warn(taskWatchEvents, "scope", "ade", "task", t.taskID, "err", err)
		}
	}
}

func (t *taskWatch) handle(ev fsnotify.Event) {
	if ev.Op == fsnotify.Chmod {
		return
	}
	if ev.Has(fsnotify.Create) {
		if fi, err := os.Lstat(ev.Name); err == nil && fi.IsDir() {
			// Files written before the watch was added raise no event.
			for _, rel := range t.addTree(ev.Name) {
				t.queue(t.relOf(filepath.Join(t.root, filepath.FromSlash(rel))))
			}
			return
		}
	}
	t.queue(t.relOf(ev.Name))
}

func (t *taskWatch) queue(rel string) {
	if rel == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	now := time.Now()
	if len(t.paths) == 0 {
		t.first = now
	}
	t.paths[rel] = struct{}{}
	wait := min(taskWatchQuiet, max(t.first.Add(taskWatchMaxWait).Sub(now), 0))
	if t.timer == nil {
		t.timer = time.AfterFunc(wait, t.fire)
		return
	}
	t.timer.Reset(wait)
}

func (t *taskWatch) fire() {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.passes.Add(1)
	t.mu.Unlock()
	defer t.passes.Done()
	rels, call, one := t.take()
	t.run(rels, call, one)
}

// take swaps out the queued paths and names the caller when there is exactly one.
func (t *taskWatch) take() (rels []string, call agentmcp.SpaceCall, one bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	rels = make([]string, 0, len(t.paths))
	for p := range t.paths {
		rels = append(rels, p)
	}
	sort.Strings(rels)
	t.paths = map[string]struct{}{}
	if len(t.callers) == 1 {
		for _, ref := range t.callers {
			call, one = ref.call, true
		}
	}
	return rels, call, one
}

// run applies the paths as the one live caller. With none or several the edits wait for an explicit
// submit or the finish flush, since a write cannot be attributed; the board push refreshes the banner.
func (t *taskWatch) run(rels []string, call agentmcp.SpaceCall, one bool) {
	if len(rels) == 0 {
		return
	}
	if !one {
		t.b.notifyBoardNow(taskChanged(t.taskID))
		return
	}
	if _, err := t.b.SyncTaskFiles(t.b.ctx, call, rels); err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn(taskWatchEvents, "scope", "ade", "task", t.taskID, "err", err)
	}
}

// stop ends the watcher. With last set, edits still queued are applied as that caller first.
func (t *taskWatch) stop(last *agentmcp.SpaceCall) {
	t.mu.Lock()
	t.stopped = true
	if t.timer != nil {
		t.timer.Stop()
	}
	t.mu.Unlock()
	t.w.Close()
	<-t.done
	t.passes.Wait()
	if last == nil {
		return
	}
	t.mu.Lock()
	rels := make([]string, 0, len(t.paths))
	for p := range t.paths {
		rels = append(rels, p)
	}
	t.paths = map[string]struct{}{}
	t.mu.Unlock()
	sort.Strings(rels)
	t.run(rels, *last, true)
}
