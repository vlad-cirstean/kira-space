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

// begin claims the task's confirm job; false when one is already running.
func (h *artifactHost) begin(taskID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, busy := h.running[taskID]; busy {
		return false
	}
	if h.running == nil {
		h.running = map[string]struct{}{}
	}
	h.running[taskID] = struct{}{}
	return true
}

func (h *artifactHost) end(taskID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.running, taskID)
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
// refetch never loops. A job already running for the task makes this a no-op.
func (b *TaskBoard) confirmArtifacts(sc ArtifactScope, set repoShareSet, facts *artifactFacts, servedArts []adewire.Artifact, servedGaps []adewire.ArtifactGap) {
	taskID := sc.Task.ID
	b.goTracked(func() {
		if !b.artHost.begin(taskID) {
			return
		}
		defer b.artHost.end(taskID)
		ctx, cancel := context.WithTimeout(b.ctx, artifactHostRefreshTimeout)
		defer cancel()
		if facts != nil && !facts.current {
			board, err := b.Board(ctx)
			if err != nil {
				slog.Debug("ade artifacts: confirm board", "scope", "ade", "task", taskID, "err", err)
				return
			}
			facts = factsOfBoard(board, sc.Rows, true)
		}
		arts, gaps := b.repoShares(ctx, sc, set, facts, true)
		if ctx.Err() != nil || sameShare(servedArts, arts, servedGaps, gaps) {
			return
		}
		b.notifyBoardWith(adewire.BoardChangedEvent{Refs: true, TaskIDs: []string{taskID}})
	})
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
