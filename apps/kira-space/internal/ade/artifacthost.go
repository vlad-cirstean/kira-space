package ade

import (
	"context"
	"log/slog"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// artifacthost.go keeps task Artifacts from waiting on the host (P758). The UI call answers from
// caches and the kept board snapshot; one background job per task then resolves what the call could
// not know and pushes the task only when the answer would differ.

// artifactHostRefreshTimeout bounds one confirm job: host lookups may block there, never in the call.
const artifactHostRefreshTimeout = 30 * time.Second

// artifactSlowLog is the Artifacts call time above which one Info line reports the per-source times.
const artifactSlowLog = 250 * time.Millisecond

// artifactHost is the last host lookup kind of each repo and the tasks with a confirm job running.
// Memory only: a repo shows no host gap until the first confirm after a start.
type artifactHost struct {
	mu      sync.Mutex
	kind    map[string]string // by code repo id: ok | disabled | unavailable
	running map[string]struct{}
	dirty   map[string]confirmJob // latest call that arrived while the task's job ran
}

func (h *artifactHost) lastKind(repoID string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.kind[repoID]
}

func (h *artifactHost) setKind(repoID, kind string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.kind == nil {
		h.kind = map[string]string{}
	}
	h.kind[repoID] = kind
}

// confirmJob is one Artifacts call's input to the confirm job: the scope, the repo set, the facts
// it read and the share it served.
type confirmJob struct {
	sc         ArtifactScope
	set        repoShareSet
	facts      *artifactFacts
	servedArts []adewire.Artifact
	servedGaps []adewire.ArtifactGap
}

// begin claims the task's confirm job. While one runs, it keeps job as the task's latest call
// instead (dirty) and returns false: end hands it back so the share a later call served is
// confirmed too.
func (h *artifactHost) begin(taskID string, job confirmJob) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, busy := h.running[taskID]; busy {
		h.dirty[taskID] = job
		return false
	}
	if h.running == nil {
		h.running, h.dirty = map[string]struct{}{}, map[string]confirmJob{}
	}
	h.running[taskID] = struct{}{}
	return true
}

// end releases the task's job, or returns the latest call kept while it ran (the job stays
// claimed for it).
func (h *artifactHost) end(taskID string) (confirmJob, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if job, dirty := h.dirty[taskID]; dirty {
		delete(h.dirty, taskID)
		return job, true
	}
	delete(h.running, taskID)
	return confirmJob{}, false
}

// repoShareSet is the repo part of one Artifacts answer.
type repoShareSet struct {
	repoIDs []string
	byRepo  map[string][]model.AdeTaskBranch
	nicks   map[string]string
}

// confirmArtifacts starts the task's background job: it resolves each repo's change requests with
// the host, waits for a board newer than the snapshot the call read, rebuilds the repo share and
// pushes {refs, task} when it differs from what the call served. No difference, no push, so a
// refetch never loops. A call that arrives while the job runs is not dropped: the job confirms once
// more against the latest share served when it ends.
func (b *TaskBoard) confirmArtifacts(sc ArtifactScope, set repoShareSet, facts *artifactFacts, servedArts []adewire.Artifact, servedGaps []adewire.ArtifactGap) {
	job := confirmJob{sc: sc, set: set, facts: facts, servedArts: servedArts, servedGaps: servedGaps}
	b.goTracked(func() {
		if !b.artHost.begin(sc.Task.ID, job) {
			return
		}
		for more := true; more; job, more = b.artHost.end(sc.Task.ID) {
			b.confirmOnce(job)
		}
	})
}

// confirmOnce is one confirm pass against the share job's call served.
func (b *TaskBoard) confirmOnce(job confirmJob) {
	taskID, facts := job.sc.Task.ID, job.facts
	ctx, cancel := context.WithTimeout(b.ctx, artifactHostRefreshTimeout)
	defer cancel()
	if facts != nil && !facts.current {
		board, err := b.Board(ctx)
		if err != nil {
			slog.Debug("ade artifacts: confirm board", "scope", "ade", "task", taskID, "err", err)
			return
		}
		facts = factsOfBoard(board, job.sc.Rows, true)
	}
	arts, gaps := b.repoShares(ctx, job.sc, job.set, facts, true)
	if ctx.Err() != nil || sameShare(job.servedArts, arts, job.servedGaps, gaps) {
		return
	}
	b.notifyBoardWith(adewire.BoardChangedEvent{Refs: true, TaskIDs: []string{taskID}})
}

// sameShare compares two repo shares. A deployment's CreatedAt is its last check time, which moves
// with every facts build and is no content, so it is ignored.
func sameShare(a, b []adewire.Artifact, ag, bg []adewire.ArtifactGap) bool {
	if !slices.Equal(ag, bg) {
		return false
	}
	return slices.EqualFunc(a, b, func(x, y adewire.Artifact) bool {
		if x.Kind == model.AdeArtifactDeployment {
			x.CreatedAt, y.CreatedAt = 0, 0
		}
		return reflect.DeepEqual(x, y)
	})
}
