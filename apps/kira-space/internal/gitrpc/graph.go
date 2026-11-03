package gitrpc

import (
	"context"
	"encoding/json"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/logsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpath"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitstore"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// ChunkRows is upstream's own CHUNK_ROWS (repoService.ts:711) — how many rows one graph.stream
// wire chunk carries.
const ChunkRows = 500

// walkSpecFrom resolves entry's own stored kiraSpace.graph.* settings (G18 D6;
// RepoEntry.RepoSettings, itself falling back to the schema's own "all"/5000 defaults when entry
// is nil or storage has nothing stored) into a porcelain.WalkSpec and a concrete page size.
func walkSpecFrom(entry *gitsession.RepoEntry) (porcelain.WalkSpec, int) {
	return porcelain.WalkSpec{Scope: repoGraphScope(entry), ExcludeStash: excludeStashFor(entry)}, pageSizeFrom(entry)
}

// excludeStashFor is the settings-side wiring for the decidable "off" half of
// kiraSpace.stash.showInGraph — a server-side read from the repo's own stored settings, so no
// wire change at all. nil entry (no repo.open has happened yet) reads as false, the setting's own
// default ("on", today's de-facto behaviour).
func excludeStashFor(entry *gitsession.RepoEntry) bool {
	if entry == nil {
		return false
	}
	return !entry.RepoSettings().StashShowInGraph
}

func pageSizeFrom(entry *gitsession.RepoEntry) int {
	if entry != nil {
		if stored := entry.RepoSettings().GraphPageSize; stored > 0 {
			return stored
		}
	}
	return logsession.DefaultPageSize
}

// repoGraphScope is walkSpecFrom's own scope-side default (D6) — "all" when entry is nil (no
// repo.open has happened yet — never reached by a well-behaved client, since c.Walk itself
// requires a held repo) or the repo has never stored a scope of its own.
func repoGraphScope(entry *gitsession.RepoEntry) string {
	if entry != nil {
		if stored := entry.RepoSettings().GraphScope; stored != "" {
			return stored
		}
	}
	return "all"
}

// rangedWalkSpecFrom validates rng's base/branch (`merge-base` takes its revisions as
// separate argv tokens, so a leading "-" is refused rather than trusted to git's own argument
// parser) and returns the ranged WalkSpec. Scope is left empty and IncludeStash false,
// deliberately: RevSetArgs never reads Scope for a ranged walk and matchesSpec compares it, so
// filling it from the extension's injected graph.scope would churn a review walk that ignores
// scope entirely; a stash is not one of a branch's own commits either.
func rangedWalkSpecFrom(rng *CommitRangeParams) (porcelain.WalkSpec, error) {
	if err := validRefArg("range.base", rng.Base); err != nil {
		return porcelain.WalkSpec{}, err
	}
	if err := validRefArg("range.branch", rng.Branch); err != nil {
		return porcelain.WalkSpec{}, err
	}
	return porcelain.WalkSpec{Range: &porcelain.RangeSpec{Base: rng.Base, Branch: rng.Branch}}, nil
}

// graph.status/graph.loadMore/graph.refresh/graph.stream stay on their own hand-rolled dispatch:
// they resolve through c.Walk/c.WalkFor/c.ReviewWalkFor, not entryFor, and
// deliberately tolerate an unheld repo (an unopened walk answers a zero-value result, never
// ErrRepoNotHeld — see graph.status's own comment below) — handleRepoCall's entryFor gate would
// turn that into an error these handlers were written specifically to avoid. Only the plain
// "repoId is required" check is shared, via requireNonEmpty.
func (r *Router) handleGraphStatus(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	var p GraphStatusParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ipcerr.Contract("gitrpc: graph.status: invalid params")
	}
	if err := requireNonEmpty("graph.status", "repoId", p.RepoID); err != nil {
		return nil, err
	}
	// Normalized once, here, at wire ingestion — see this file's own resolveWalkRequest, which
	// already normalized ITS internal c.Entry lookup but left every c.Walk/WalkFor/ReviewWalkFor
	// call in this file using the raw p.RepoID. Conn's own maps are keyed by entry.Summary.RepoID
	// (NFC, since Identify normalizes), so a decomposed repoId for a non-ASCII repository path
	// failed ErrRepoNotHeld here even though commit.detail/file.read (entryFor, detail.go) already
	// normalize and work fine.
	p.RepoID = gitpath.CleanNFC(p.RepoID)

	// D10: `range` present -> the review walk's own counters, never the graph's.
	if p.Range != nil {
		if _, err := rangedWalkSpecFrom(p.Range); err != nil {
			return nil, err
		}
		w, ok := c.ReviewWalkFor(p.RepoID)
		if !ok {
			// The same zero-value answer G3 D14 gives for an unopened graph walk.
			return GraphStatusResult{}, nil
		}
		loaded, remaining, exhausted, err := w.Status(ctx)
		if err != nil {
			return nil, mapGitError(err)
		}
		return GraphStatusResult{Loaded: loaded, Remaining: remaining, Exhausted: exhausted}, nil
	}

	w, ok := c.WalkFor(p.RepoID)
	if !ok {
		// upstream's own answer for an unopened walk (repoService.ts:1005-1017).
		return GraphStatusResult{}, nil
	}
	loaded, remaining, exhausted, err := w.Status(ctx)
	if err != nil {
		return nil, mapGitError(err)
	}
	return GraphStatusResult{Loaded: loaded, Remaining: remaining, Exhausted: exhausted}, nil
}

func (r *Router) handleGraphLoadMore(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	var p GraphLoadMoreParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ipcerr.Contract("gitrpc: graph.loadMore: invalid params")
	}
	if err := requireNonEmpty("graph.loadMore", "repoId", p.RepoID); err != nil {
		return nil, err
	}
	p.RepoID = gitpath.CleanNFC(p.RepoID) // G31 round-2 architecture/security review, finding #4.

	status := r.deps.Discovery.Status(ctx, gitPathFrom(r.deps.Registry))
	if status.Kind != "ok" {
		return nil, ipcerr.T(ipcerr.CodeGitUnavailable, "backend.space.gitrpc.gitUnavailable.body", nil)
	}

	// D10: `range` present -> pages the review walk, never the graph's.
	spec, pageSize, precomputedTotal, err := resolveWalkRequest(c, p.RepoID, p.Range)
	if err != nil {
		return nil, err
	}
	w, err := c.Walk(p.RepoID, status.Path, spec, pageSize, precomputedTotal)
	if err != nil {
		return nil, mapGitError(err)
	}

	pages := 1
	if p.Pages != nil && *p.Pages > 0 {
		pages = *p.Pages
	}
	started, err := w.ReadPage(ctx, pages)
	if err != nil {
		return nil, mapGitError(err)
	}
	return GraphLoadMoreResult{Started: started}, nil
}

func (r *Router) handleGraphRefresh(_ context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	var p GraphRefreshParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ipcerr.Contract("gitrpc: graph.refresh: invalid params")
	}
	if err := requireNonEmpty("graph.refresh", "repoId", p.RepoID); err != nil {
		return nil, err
	}
	p.RepoID = gitpath.CleanNFC(p.RepoID) // G31 round-2 architecture/security review, finding #4.

	// graph.refresh has no `range` — the review view has no refresh affordance (D10).
	w, ok := c.WalkFor(p.RepoID)
	if !ok {
		return GraphRefreshResult{Restarted: false}, nil
	}
	w.MarkRefresh()
	return GraphRefreshResult{Restarted: true}, nil
}

// resolveWalkRequest is graph.loadMore/graph.stream's shared range-vs-scope resolution: with a
// `range`, it validates the two ref fields and peeks the per-repo range-count slot (a
// non-blocking best-effort optimisation — a miss passes nil and logsession runs its own count);
// without one, it resolves the stored scope/pageSize. entry is looked up once, here, and
// threaded into both branches — the range branch already needed it for the range-count peek; the
// non-ranged branch now needs it too, for the per-repo scope/pageSize defaults.
func resolveWalkRequest(c *gitsession.Conn, repoID string, rng *CommitRangeParams) (porcelain.WalkSpec, int, *int, error) {
	entry, _ := c.Entry(gitpath.CleanNFC(repoID)) // G27 D6; nil, ok=false when unheld — c.Walk below rejects that case itself.

	if rng == nil {
		spec, size := walkSpecFrom(entry)
		return spec, size, nil, nil
	}
	spec, err := rangedWalkSpecFrom(rng)
	if err != nil {
		return porcelain.WalkSpec{}, 0, nil, err
	}
	var precomputedTotal *int
	if entry != nil {
		precomputedTotal = entry.TakeRangeCount(rng.Base, rng.Branch)
	}
	return spec, pageSizeFrom(entry), precomputedTotal, nil
}

// handleGraphReportFailure records a client-detected graph failure. The message is composed here,
// never taken from the client.
func (r *Router) handleGraphReportFailure(_ context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	var p GraphReportFailureParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ipcerr.Contract("gitrpc: graph.reportFailure: invalid params")
	}
	if err := requireNonEmpty("graph.reportFailure", "repoId", p.RepoID); err != nil {
		return nil, err
	}
	if p.Reason != "corrupted" {
		return nil, ipcerr.Contract("gitrpc: graph.reportFailure: unknown reason")
	}
	c.RecordFailure(gitpath.CleanNFC(p.RepoID), "graph.load", "graph data stream was corrupted; the graph was reset")
	return struct{}{}, nil
}

// handleGraphStream serves graph.stream: upstream's streamGraph, via gitsession.Walk.Stream
// — every emitted StreamChunk is packed to a FlatBuffer (gitstore.EncodeChunkFrame) and
// handed to the plane's emit as the chunk envelope's out-of-band blob. `range` present
// streams the review walk instead of the graph's.
//
// resumeThroughRow used to be forced nil on this path (the "a ranged walk has no persisted
// cache to resume from" — true, but conflated "nothing PERSISTED across a cold reopen" with
// "nothing the CLIENT already has in memory"). Conn.Walk reuses the SAME *Walk (and so the same
// w.store/w.marks cache) across every graph.stream call for one range spec, exactly like the
// graph's own non-ranged walk — review.ts's own loadMore does a graph.loadMore (pages the walk)
// followed immediately by a graph.stream re-open, and that re-open used to always start at row 0,
// re-sending and re-decoding every row already applied from the walk's own cache, THROWN AWAY AND
// REBUILT client-side each time (packedStream.ts's own from===0-with-existing-rows reset rule) —
// quadratic in the number of pages loaded. resumeThroughRow is now honored exactly like the
// non-ranged path: Walk.Stream's own clamping (a value past what is cached, or with no matching
// dictionary mark, safely falls back to 0) is what makes trusting it safe, not whether
// the walk happens to be ranged.
func (r *Router) handleGraphStream(ctx context.Context, c *gitsession.Conn, params json.RawMessage, emit func(payload any, blob []byte) error) error {
	var p GraphStreamParams
	if err := json.Unmarshal(params, &p); err != nil {
		return ipcerr.Contract("gitrpc: graph.stream: invalid params")
	}
	if err := requireNonEmpty("graph.stream", "repoId", p.RepoID); err != nil {
		return err
	}
	p.RepoID = gitpath.CleanNFC(p.RepoID) // G31 round-2 architecture/security review, finding #4:
	// also fixes graph.stream's own chunk echo (RepoID: p.RepoID below) carrying the client's raw
	// spelling while repo.changed events carry the NFC one — a client keying state on repoId used
	// to see two spellings for one repository.

	spec, pageSize, precomputedTotal, err := resolveWalkRequest(c, p.RepoID, p.Range)
	if err != nil {
		return err
	}
	status := r.deps.Discovery.Status(ctx, gitPathFrom(r.deps.Registry))
	if status.Kind != "ok" {
		return ipcerr.T(ipcerr.CodeGitUnavailable, "backend.space.gitrpc.gitUnavailable.body", nil)
	}
	w, err := c.Walk(p.RepoID, status.Path, spec, pageSize, precomputedTotal)
	if err != nil {
		recordGraphFailure(ctx, c, p.RepoID, err)
		return mapGitError(err)
	}

	err = w.Stream(ctx, p.ResumeThroughRow, ChunkRows, func(chunk gitsession.StreamChunk) error {
		blob := gitstore.EncodeChunkFrame(chunk.Packed)
		payload := graphChunk{
			RepoID: p.RepoID, Seq: chunk.Seq, From: chunk.From, To: chunk.To,
			Source: chunk.Source, Remaining: chunk.Remaining, Exhausted: chunk.Exhausted,
		}
		return emit(payload, blob)
	})
	if err != nil {
		recordGraphFailure(ctx, c, p.RepoID, err)
		return mapGitError(err)
	}
	return nil
}

// recordGraphFailure logs a server-side graph load failure; cancellation and teardown log nothing.
func recordGraphFailure(ctx context.Context, c *gitsession.Conn, repoID string, err error) {
	if ctx.Err() == nil {
		c.RecordFailure(repoID, "graph.load", "graph load failed: "+err.Error())
	}
}
