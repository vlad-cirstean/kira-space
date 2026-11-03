package gitrpc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitaskpass"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/catfile"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpath"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/notify"
)

// Deps is everything a Router needs; nothing more reaches it (D10/D11 from G1, unchanged in shape
// — only the fields differ, D18).
type Deps struct {
	Discovery     *gitclient.Discovery
	Runner        gitclient.Runner
	Registry      *gitsession.Registry
	ServerVersion string
	// Askpass is G7's credential broker — nil when it failed to start (main.go's own D8 posture:
	// every remote op then runs with no askpass interposition at all, never a fatal boot error).
	Askpass *gitaskpass.Broker
	// DateFormat reads Kira Space's appearance.dateFormat; nil means "relative".
	DateFormat func() string
}

// Handlers is gitrpc's own two-function method table: gitrpc must not import internal/bridge (a
// domain package stays under the layering line, SPEC §7), so internal/bridge adapts these two
// functions onto GitService and the graph data plane.
type Handlers struct {
	Request func(ctx context.Context, method string, params json.RawMessage) (any, error)
	// GraphStream serves graph.stream for the graph data plane, calling emit per chunk.
	GraphStream func(ctx context.Context, params json.RawMessage, emit func(payload any, blob []byte) error) error
}

// repoSettingsQueueCap bounds each connection's own repoSettings.changed mailbox (ForConn) —
// generous for any realistic burst of concurrent repoSettings.set calls while staying tiny; a full
// queue drops its oldest entry rather than growing further, since every payload is a full snapshot
// anyway.
const repoSettingsQueueCap = 8

// Router builds a per-connection Handlers over one shared Deps: every method that touches a
// repository now needs to know which connection is asking, so it can route through that
// connection's own gitsession.Conn (its holds, its Emit) rather than a single global registry.
type Router struct {
	deps Deps

	// repoSettingsChanged is the fan-out: every currently connected client gets
	// repoSettings.set's own result, not only the connection that made the change (the
	// cross-connection regression guard) — the same internal/notify.Emitter[T] mechanism
	// dbmcp and gitcred use, one Router-wide instance rather than a new pub/sub of its own.
	repoSettingsChanged notify.Emitter[RepoSettingsChangedPayload]
}

// New constructs a Router over deps.
func New(deps Deps) *Router { return &Router{deps: deps} }

// ForConn returns the two-function Handlers internal/bridge serves for one connection — c is
// closed over by repo.open/repo.close (repo.close releases this connection's hold, it never evicts
// globally).
//
// G18 D7: also subscribes c to repoSettingsChanged for the life of the connection — unsubscribed
// via c.Done() rather than repo.close, since repoSettings.changed is not scoped to any one repo
// being held open. c.Emit (gitsession.Conn) is a
// mutex-guarded accessor, not a bare field (P108 Part 17 review F9) — c.SetEmit is not called until
// just after ForConn returns (bridge's GitWindows), so an event landing in that narrow window
// safely no-ops rather than racing gitsession.Conn's own unguarded field the way it used to: an
// event here is never this connection's OWN repoSettings.set (nothing can call it before this
// connection's own Handlers exist to dispatch it), but it CAN be a different, already-connected
// client's repoSettings.set landing in settingsQueue while this connection is still mid-construction
// — exactly the race emitMu closes.
//
// G32 round-3 architecture/security review, finding #5: notify.Emitter.Emit (notify.go) calls
// every subscriber SEQUENTIALLY, on the calling repoSettings.set request's own goroutine.
// A connection's emit can block on a bounded channel until the frame is accepted or THAT
// connection ends — a slow-reading or wedged OTHER client used to be
// able to stall the fan-out entirely, delaying the repoSettings.set RPC response for the client
// that actually made the change, and every subscriber snapshotted after the wedged one. Each
// connection now gets its own small, bounded mailbox (repoSettingsQueueCap) plus ONE dedicated
// forwarding goroutine — never one goroutine per event, which could reorder deliveries against
// each other — so the subscribe callback itself is a non-blocking channel send: a wedged
// connection can only ever stall its own forwarding goroutine, never the emitting caller or any
// other connection. Dropping the oldest queued entry when a connection falls behind is safe
// because RepoSettingsChangedPayload always carries a FULL snapshot (D4) — the next delivery makes
// the client current regardless of what was skipped in between.
func (r *Router) ForConn(c *gitsession.Conn) Handlers {
	settingsQueue := make(chan RepoSettingsChangedPayload, repoSettingsQueueCap)
	unsubscribeRepoSettings := r.repoSettingsChanged.Subscribe(func(payload RepoSettingsChangedPayload) {
		select {
		case settingsQueue <- payload:
		default:
			// The queue is full — this connection is badly behind. Drop the oldest pending
			// snapshot and enqueue the newest, non-blockingly either way; both selects have a
			// default case so a race with the forwarding goroutine draining concurrently can never
			// make this block.
			select {
			case <-settingsQueue:
			default:
			}
			select {
			case settingsQueue <- payload:
			default:
			}
		}
	})
	go func() {
		for {
			select {
			case payload := <-settingsQueue:
				c.Emit("repoSettings.changed", payload)
			case <-c.Done():
				return
			}
		}
	}()
	go func() {
		<-c.Done()
		unsubscribeRepoSettings()
	}()

	return Handlers{
		// Shape (b): the switch this used to be was itself the whole gocyclo score (one arm per
		// RPC method) — requestHandlers turns "which method" into a lookup instead of a branch.
		Request: func(ctx context.Context, method string, params json.RawMessage) (any, error) {
			h, ok := requestHandlers[method]
			if !ok {
				return nil, ipcerr.T(ipcerr.CodeUnknownMethod, "backend.space.gitrpc.unknownMethod.body", map[string]string{"method": method})
			}
			return h(r, oplog.WithOrigin(ctx, c.Origin()), c, params)
		},
		GraphStream: func(ctx context.Context, params json.RawMessage, emit func(payload any, blob []byte) error) error {
			return r.handleGraphStream(oplog.WithOrigin(ctx, c.Origin()), c, params, emit)
		},
	}
}

// requestHandler is one requestHandlers entry's own shape — uniform across every RPC method even
// though the underlying handleXxx methods are not (handleAppInit takes no c/params,
// handleRepoClose is a package func rather than a method) —
// each entry adapts its own handler to this one signature.
type requestHandler func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error)

// requestHandlers is ForConn's own dispatch table: one entry per RPC method Request accepts.
var requestHandlers = map[string]requestHandler{
	"app.init": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleAppInit(ctx), nil
	},
	"repo.open": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleRepoOpen(ctx, c, params)
	},
	"repo.close": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return handleRepoClose(c, params)
	},
	"graph.status": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleGraphStatus(ctx, c, params)
	},
	"graph.loadMore": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleGraphLoadMore(ctx, c, params)
	},
	"graph.refresh": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleGraphRefresh(ctx, c, params)
	},
	"graph.reportFailure": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleGraphReportFailure(ctx, c, params)
	},
	"commit.detail": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleCommitDetail(ctx, c, params)
	},
	"commit.fileDiff": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleCommitFileDiff(ctx, c, params)
	},
	"file.read": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleFileRead(ctx, c, params)
	},
	"file.goToTarget": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleFileGoToTarget(ctx, c, params)
	},
	"blame.line": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleBlameLine(ctx, c, params)
	},
	"working.detail": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleWorkingDetail(ctx, c, params)
	},
	"refs.list": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleRefsList(ctx, c, params)
	},
	"status.get": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleStatusGet(ctx, c, params)
	},
	"preflight.checkout": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handlePreflightCheckout(ctx, c, params)
	},
	"preflight.revert": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handlePreflightRevert(ctx, c, params)
	},
	"preflight.reset": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handlePreflightReset(ctx, c, params)
	},
	"preflight.cherryPick": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handlePreflightCherryPick(ctx, c, params)
	},
	"stash.list": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleStashList(ctx, c, params)
	},
	"stash.show": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleStashShow(ctx, c, params)
	},
	"preflight.stashPop": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handlePreflightStashPop(ctx, c, params)
	},
	"preflight.stashBranch": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handlePreflightStashBranch(ctx, c, params)
	},
	"globalStash.list": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleGlobalStashList(ctx, c, params)
	},
	"op.run": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleOpRun(ctx, c, params)
	},
	"undo.peek": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleUndoPeek(ctx, c, params)
	},
	"undo.run": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleUndoRun(ctx, c, params)
	},
	"review.resolveBase": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewResolveBase(ctx, c, params)
	},
	"remote.pullPreflight": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleRemotePullPreflight(ctx, c, params)
	},
	"remote.pushPreflight": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleRemotePushPreflight(ctx, c, params)
	},
	"remote.run": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleRemoteRun(ctx, c, params)
	},
	"remote.cancel": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleRemoteCancel(ctx, c, params)
	},
	"review.files": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewFiles(ctx, c, params)
	},
	"review.fileDiff": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewFileDiff(ctx, c, params)
	},
	"review.snapshot": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewSnapshot(ctx, c, params)
	},
	"review.mark": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewMark(ctx, c, params)
	},
	"review.markMany": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewMarkMany(ctx, c, params)
	},
	"review.comment.add": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewCommentAdd(ctx, c, params)
	},
	"review.comment.list": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewCommentList(ctx, c, params)
	},
	"review.comment.remove": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewCommentRemove(ctx, c, params)
	},
	"review.comment.setState": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewCommentSetState(ctx, c, params)
	},
	"review.comment.edit": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewCommentEdit(ctx, c, params)
	},
	"review.comment.clear": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewCommentClear(ctx, c, params)
	},
	"review.comment.export": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleReviewCommentExport(ctx, c, params)
	},
	"repoSettings.get": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleRepoSettingsGet(ctx, c, params)
	},
	"repoSettings.set": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleRepoSettingsSet(ctx, c, params)
	},
	"search.run": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleSearchRun(ctx, c, params)
	},
	"commit.resolveChangeRequests": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleCommitResolveChangeRequests(ctx, c, params)
	},
	"branch.resolveChangeRequest": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleBranchResolveChangeRequest(ctx, c, params)
	},
	"branch.changeRequestIndex": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleBranchChangeRequestIndex(ctx, c, params)
	},
	"branch.syncChangeRequests": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleBranchSyncChangeRequests(ctx, c, params)
	},
	"gitHost.info": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleGitHostInfo(ctx, c, params)
	},
	"gitHost.providers": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleGitHostProviders(ctx, c, params)
	},
	"changeRequest.browserUrl": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleChangeRequestBrowserUrl(ctx, c, params)
	},
	"changeRequest.reviewState": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleChangeRequestReviewState(ctx, c, params)
	},
	"changeRequest.submitReview": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleChangeRequestSubmitReview(ctx, c, params)
	},
	"changeRequest.lineComment": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleChangeRequestLineComment(ctx, c, params)
	},
	"changeRequest.comment": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleChangeRequestComment(ctx, c, params)
	},
	"worktree.list": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleWorktreeList(ctx, c, params)
	},
	"preflight.worktreeAdd": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handlePreflightWorktreeAdd(ctx, c, params)
	},
	"preflight.worktreeRemove": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handlePreflightWorktreeRemove(ctx, c, params)
	},
	"stack.list": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleStackList(ctx, c, params)
	},
	"preflight.restack": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handlePreflightRestack(ctx, c, params)
	},
	"stack.restack": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleStackRestack(ctx, c, params)
	},
	"stack.cancelRestack": func(r *Router, ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
		return r.handleStackCancelRestack(ctx, c, params)
	},
}

func (r *Router) handleAppInit(ctx context.Context) AppInitResult {
	return AppInitResult{
		ServerVersion: r.deps.ServerVersion,
		Git:           r.deps.Discovery.Status(ctx, gitPathFrom(r.deps.Registry)),
		DateFormat:    r.dateFormat(),
	}
}

func (r *Router) dateFormat() string {
	if r.deps.DateFormat == nil {
		return "relative"
	}
	return r.deps.DateFormat()
}

// gitPathFrom is G18 D15's own tiny helper: Registry.Settings' three-value destructure, named so
// it is not repeated at every Discovery.Status call site across handlers.go/graph.go.
// protectedBranches/autoFetchMinutes are unused here — Discovery.Status wants only the third
// value.
func gitPathFrom(reg *gitsession.Registry) string {
	_, _, gitPath := reg.Settings()
	return gitPath
}

func (r *Router) handleRepoOpen(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	var p RepoOpenParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ipcerr.Contract("gitrpc: repo.open: invalid params")
	}
	if p.Path == "" {
		return nil, ipcerr.Contract("gitrpc: repo.open: path is required")
	}
	p.Path = gitpath.CleanNFC(p.Path) // G27 D5d: a client-supplied directory param (D2 tier 1).

	status := r.deps.Discovery.Status(ctx, gitPathFrom(r.deps.Registry))
	if status.Kind != "ok" {
		return RepoOpenResult{Kind: "gitUnavailable", Git: &status}, nil
	}

	summary, err := c.Open(ctx, r.deps.Registry, status.Path, p.Path)
	if err != nil {
		if kind, ok := gitclient.KindOf(err); ok && kind == gitclient.KindNotARepository {
			return RepoOpenResult{Kind: "notARepository", Path: p.Path}, nil
		}
		return nil, mapGitError(err)
	}
	// Compose the summary with the entry's own LIVE head rather than the value Identify froze
	// at whichever window opened this repo first — a second window opening an already-open
	// repository must see HEAD as it stands now, not as it stood at that first open. Best-effort:
	// a failure here falls back to the frozen value rather than failing repo.open outright.
	if entry, ok := c.Entry(summary.RepoID); ok {
		if head, herr := entry.Head(ctx); herr == nil {
			summary.Head = head
		}
	}
	return RepoOpenResult{Kind: "ok", Repo: &summary}, nil
}

func handleRepoClose(c *gitsession.Conn, params json.RawMessage) (any, error) {
	var p RepoCloseParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ipcerr.Contract("gitrpc: repo.close: invalid params")
	}
	if p.RepoID == "" {
		return nil, ipcerr.Contract("gitrpc: repo.close: repoId is required")
	}
	// CloseRepo's own map is keyed by entry.Summary.RepoID (NFC, since Identify normalizes) — a
	// decomposed repoId for a non-ASCII repository path used to silently no-op here (CloseRepo
	// already answers {} either way, so nothing errored), leaking that connection's RepoEntry
	// refcount and its watcher subscription for the connection's whole life.
	c.CloseRepo(gitpath.CleanNFC(p.RepoID)) // idempotent either way (D15) — repo.close always answers {}.
	return struct{}{}, nil
}

// mapGitError turns gitclient's closed error vocabulary into ipcerr codes so the classification
// survives the wire. The bridge folds anything that is not an *ipcerr.Error into
// E_INTERNAL, which is the whole reason this exists — a git failure
// must cross as E_GIT_<KIND>, never as an anonymous internal error. Every code the server emits is
// listed in packages/git-ipc/src/contract.ts WireErrorCode; add new ones there.
func mapGitError(err error) error {
	switch {
	case errors.Is(err, gitsession.ErrRepoNotHeld), errors.Is(err, gitsession.ErrRepoTornDown):
		// Ordinary under concurrency: a request racing repo.close, teardown or a walk replaced by
		// another graph.loadMore. The client must tell this from a server fault.
		return ipcerr.T(ipcerr.CodeConflict, "backend.space.gitrpc.repoNotOpen.body", nil)
	case errors.Is(err, catfile.ErrInvalidRev):
		return ipcerr.BadRequest(err.Error())
	case errors.Is(err, gitreview.ErrStoreClosed):
		return ipcerr.T(ipcerr.CodeGitUnavailable, "backend.space.gitrpc.reviewStoreClosed.body", nil)
	}
	kind, ok := gitclient.KindOf(err)
	if !ok {
		return err
	}
	return ipcerr.New(string(gitErrorCode(kind)), err.Error())
}

// gitErrorCode maps gitclient's closed ErrorKind vocabulary to its E_GIT_* wire code.
func gitErrorCode(kind gitclient.ErrorKind) ipcerr.Code {
	switch kind {
	case gitclient.KindNotARepository:
		return ipcerr.CodeGitNotARepository
	case gitclient.KindPermissionDenied:
		return ipcerr.CodeGitPermissionDenied
	case gitclient.KindCancelled:
		return ipcerr.CodeGitCancelled
	case gitclient.KindTimeout:
		return ipcerr.CodeGitTimeout
	default:
		return ipcerr.CodeGitUnknown
	}
}
