package ade

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/fileservice"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpath"
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
	nroot  string // root, Clean and NFC: the spelling fswatch reports paths in
	w      *fileservice.Watcher
	deb    *fileservice.Debouncer

	mu      sync.Mutex
	callers map[string]*callerRef
	paths   map[string]struct{} // slash paths relative to root since the last pass
	stopped bool
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
	tw := &taskWatch{b: b, taskID: taskID, root: root, nroot: gitpath.CleanNFC(root), callers: map[string]*callerRef{}, paths: map[string]struct{}{}, done: make(chan struct{})}
	w, err := fileservice.Host().Watch(fileservice.WatchOptions{Names: []string{root}, MaxDepth: taskWatchDepth, SkipDir: tw.skipDir})
	if err != nil {
		return nil, err
	}
	tw.w = w
	tw.deb = fileservice.NewDebouncer(taskWatchQuiet, taskWatchMaxWait, tw.fire)
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

// skipDir is the watch's SkipDir: only task-file folders (notes, handovers, proofs and below) are watched.
func (t *taskWatch) skipDir(p string) bool {
	rel := t.slashRel(p)
	if rel == "" || !watchedTopDirs[strings.SplitN(rel, "/", 2)[0]] {
		return true
	}
	return hasHiddenSegment(rel)
}

func hasHiddenSegment(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// walkFiles returns the task-file rels found at and below dir, using the watch's own folder rules.
func (t *taskWatch) walkFiles(dir string) []string {
	base, err := filepath.Rel(t.root, dir)
	if err != nil {
		return nil
	}
	base = filepath.ToSlash(base)
	if base != "." && t.skipRel(base) {
		return nil
	}
	var files []string
	_ = fileservice.Host().Walk(dir, fileservice.WalkOptions{
		SkipDir: func(rel string) bool {
			if base != "." {
				rel = base + "/" + rel
			}
			return t.skipRel(rel)
		},
	}, func(e fileservice.Entry) error {
		if e.Dir || e.Mode&fs.ModeSymlink != 0 {
			return nil
		}
		rel := e.Name
		if base != "." {
			rel = base + "/" + rel
		}
		files = append(files, rel)
		return nil
	})
	return files
}

// skipRel is the walk's folder rule for a slash dir path under the task root.
func (t *taskWatch) skipRel(rel string) bool {
	segs := strings.Split(rel, "/")
	return strings.HasPrefix(segs[len(segs)-1], ".") || len(segs) > taskWatchDepth || !watchedTopDirs[segs[0]]
}

// slashRel is the task-folder-relative slash path of a reported path, "" outside the folder or for the folder itself.
func (t *taskWatch) slashRel(name string) string {
	rel, err := filepath.Rel(t.nroot, name)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// relOf is the slash path of an event, "" when the event is not about a task file.
func (t *taskWatch) relOf(name string) string { return keepTaskFile(t.slashRel(name)) }

// keepTaskFile returns rel when it names a task file, else "".
func keepTaskFile(rel string) string {
	if rel == "" || hasHiddenSegment(rel) {
		return ""
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
	for ev := range t.w.Events() {
		t.handle(ev)
	}
}

func (t *taskWatch) handle(ev fileservice.Event) {
	if ev.Rescan {
		// No path pass: a full walk would apply a user's earlier external edit as the agent's. The banner
		// refreshes; an explicit submit or the finish flush applies the edits.
		t.b.notifyBoardNow(taskChanged(t.taskID))
		return
	}
	if ev.Op == fileservice.Meta {
		return
	}
	if ev.Op.Has(fileservice.Create) || ev.Op.Has(fileservice.Rename) {
		if rel := t.slashRel(ev.Path); rel != "" {
			dir := filepath.Join(t.root, filepath.FromSlash(rel))
			if fi, err := fileservice.Host().Lstat(dir); ev.Dir || (err == nil && fi.Dir) {
				// Files written before the watch covered the dir raise no event.
				for _, f := range t.walkFiles(dir) {
					t.queue(keepTaskFile(f))
				}
				return
			}
		}
	}
	t.queue(t.relOf(ev.Path))
}

func (t *taskWatch) queue(rel string) {
	if rel == "" {
		return
	}
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.paths[rel] = struct{}{}
	t.mu.Unlock()
	t.deb.Schedule()
}

func (t *taskWatch) fire() {
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
	t.mu.Unlock()
	t.deb.Stop() // waits for an in-flight pass
	_ = t.w.Close()
	<-t.done
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
