package ade

import (
	"log/slog"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// runnote.go: the note an ADE run shows while one of its MCP calls waits on the user (branch requests,
// task creation). One hold per run counts every waiting call of any kind, so the first call's note stays
// until the last call ends.

// runWait counts the calls of one run waiting on the user. prev is the note before the first hold, set the
// note the hold wrote.
type runWait struct {
	prev, set string
	n         int
}

// runNotes tracks the held run notes.
type runNotes struct {
	mu    sync.Mutex
	holds map[string]*runWait
}

// holdRunNote shows note on the run while any of its calls waits; false when runID is no ADE run.
func (b *TaskBoard) holdRunNote(runID, note string) bool {
	if runID == "" {
		return false
	}
	q := &b.runNotes
	q.mu.Lock()
	defer q.mu.Unlock()
	if w := q.holds[runID]; w != nil {
		w.n++
		return true
	}
	prev, ok := b.setRunNote(runID, note)
	if !ok {
		return false
	}
	if q.holds == nil {
		q.holds = map[string]*runWait{}
	}
	q.holds[runID] = &runWait{prev: prev, set: note, n: 1}
	return true
}

// releaseRunNote puts the note back when the run's last waiting call ends.
func (b *TaskBoard) releaseRunNote(taskID, runID string) {
	q := &b.runNotes
	q.mu.Lock()
	defer q.mu.Unlock()
	w := q.holds[runID]
	if w == nil {
		return
	}
	if w.n--; w.n > 0 {
		return
	}
	delete(q.holds, runID)
	b.restoreRunNote(taskID, runID, w.set, w.prev)
}

// setRunNote sets the note of an ADE run and returns the old one; ok is false when runID is no ADE run.
func (b *TaskBoard) setRunNote(runID, note string) (prev string, ok bool) {
	if runID == "" {
		return "", false
	}
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil {
		return "", false
	}
	updated, err := b.deps.Tasks.UpdateRun(runID, model.AdeRunPatch{Note: &note})
	if err != nil {
		slog.Warn("ade: run note", "scope", "ade", "run", runID, "err", err)
		return "", false
	}
	b.emitRuns(updated)
	return run.Note, true
}

// restoreRunNote puts the pre-request note back only while the run still runs and still shows the note the
// hold set; a run that ended during the wait keeps the note its end path wrote.
func (b *TaskBoard) restoreRunNote(taskID, runID, set, note string) {
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil || run.State != model.AdeRunRunning || run.Note != set {
		return
	}
	updated, err := b.deps.Tasks.UpdateRun(runID, model.AdeRunPatch{Note: &note})
	if err != nil {
		slog.Warn("ade: run note", "scope", "ade", "run", runID, "err", err)
		return
	}
	b.emitRuns(updated)
}
