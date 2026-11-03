/**
 * Every git request method, served by the bound `GitService` (plus `GitHostService.Providers`).
 * Graph rows are the one bulk path and ride the `gitgraph` data plane (`graphStream.ts`).
 *
 * Imports the bindings directly, not through `bridge/index.ts`: `unwrap` returns a plain promise
 * and loses `.cancel()`, and git errors must surface as `RpcError` (git-ui branches on its code),
 * not `CodedError`.
 */
import * as GitHostService from '@bindings/githostservice.js';
import * as GitService from '@bindings/gitservice.js';
import type { RequestKey } from '@kira/git-ipc';
import { RpcError, TransportError } from '@kira/git-ipc';
import { toCodedError } from '@workbench/bridge/codedError';
import { on, windowKey } from '@workbench/bridge/rpc';

interface Cancellable extends Promise<unknown> {
  cancel(): void;
}

type BoundCall = (windowKey: string, params: never) => Cancellable;

const BOUND_METHODS = {
  'app.init': (w) => GitService.AppInit({ windowKey: w }),
  'repo.open': (w, p) => GitService.RepoOpen({ windowKey: w, params: p }),
  'repo.close': (w, p) => GitService.RepoClose({ windowKey: w, params: p }),
  'graph.status': (w, p) => GitService.GraphStatus({ windowKey: w, params: p }),
  'graph.loadMore': (w, p) => GitService.GraphLoadMore({ windowKey: w, params: p }),
  'graph.refresh': (w, p) => GitService.GraphRefresh({ windowKey: w, params: p }),
  'graph.reportFailure': (w, p) => GitService.GraphReportFailure({ windowKey: w, params: p }),
  'status.get': (w, p) => GitService.StatusGet({ windowKey: w, params: p }),
  'refs.list': (w, p) => GitService.RefsList({ windowKey: w, params: p }),
  'commit.detail': (w, p) => GitService.CommitDetail({ windowKey: w, params: p }),
  'commit.fileDiff': (w, p) => GitService.CommitFileDiff({ windowKey: w, params: p }),
  'working.detail': (w, p) => GitService.WorkingDetail({ windowKey: w, params: p }),
  'file.read': (w, p) => GitService.FileRead({ windowKey: w, params: p }),
  'file.goToTarget': (w, p) => GitService.FileGoToTarget({ windowKey: w, params: p }),
  'blame.line': (w, p) => GitService.BlameLine({ windowKey: w, params: p }),
  'search.run': (w, p) => GitService.SearchRun({ windowKey: w, params: p }),
  'repoSettings.get': (w, p) => GitService.RepoSettingsGet({ windowKey: w, params: p }),
  'repoSettings.set': (w, p) => GitService.RepoSettingsSet({ windowKey: w, params: p }),
  'preflight.checkout': (w, p) => GitService.PreflightCheckout({ windowKey: w, params: p }),
  'preflight.revert': (w, p) => GitService.PreflightRevert({ windowKey: w, params: p }),
  'preflight.reset': (w, p) => GitService.PreflightReset({ windowKey: w, params: p }),
  'preflight.cherryPick': (w, p) => GitService.PreflightCherryPick({ windowKey: w, params: p }),
  'stash.list': (w, p) => GitService.StashList({ windowKey: w, params: p }),
  'stash.show': (w, p) => GitService.StashShow({ windowKey: w, params: p }),
  'preflight.stashPop': (w, p) => GitService.PreflightStashPop({ windowKey: w, params: p }),
  'preflight.stashBranch': (w, p) => GitService.PreflightStashBranch({ windowKey: w, params: p }),
  'globalStash.list': (w, p) => GitService.GlobalStashList({ windowKey: w, params: p }),
  'op.run': (w, p) => GitService.OpRun({ windowKey: w, params: p }),
  'undo.peek': (w, p) => GitService.UndoPeek({ windowKey: w, params: p }),
  'undo.run': (w, p) => GitService.UndoRun({ windowKey: w, params: p }),
  'remote.pullPreflight': (w, p) => GitService.RemotePullPreflight({ windowKey: w, params: p }),
  'remote.pushPreflight': (w, p) => GitService.RemotePushPreflight({ windowKey: w, params: p }),
  'remote.run': (w, p) => GitService.RemoteRun({ windowKey: w, params: p }),
  'remote.cancel': (w, p) => GitService.RemoteCancel({ windowKey: w, params: p }),
  'worktree.list': (w, p) => GitService.WorktreeList({ windowKey: w, params: p }),
  'preflight.worktreeAdd': (w, p) => GitService.PreflightWorktreeAdd({ windowKey: w, params: p }),
  'preflight.worktreeRemove': (w, p) =>
    GitService.PreflightWorktreeRemove({ windowKey: w, params: p }),
  'stack.list': (w, p) => GitService.StackList({ windowKey: w, params: p }),
  'preflight.restack': (w, p) => GitService.PreflightRestack({ windowKey: w, params: p }),
  'stack.restack': (w, p) => GitService.StackRestack({ windowKey: w, params: p }),
  'stack.cancelRestack': (w, p) => GitService.StackCancelRestack({ windowKey: w, params: p }),
  'review.resolveBase': (w, p) => GitService.ReviewResolveBase({ windowKey: w, params: p }),
  'review.files': (w, p) => GitService.ReviewFiles({ windowKey: w, params: p }),
  'review.removed.list': (w, p) => GitService.ReviewRemovedList({ windowKey: w, params: p }),
  'review.removed.dismiss': (w, p) => GitService.ReviewRemovedDismiss({ windowKey: w, params: p }),
  'review.fileDiff': (w, p) => GitService.ReviewFileDiff({ windowKey: w, params: p }),
  'review.snapshot': (w, p) => GitService.ReviewSnapshot({ windowKey: w, params: p }),
  'review.mark': (w, p) => GitService.ReviewMark({ windowKey: w, params: p }),
  'review.markMany': (w, p) => GitService.ReviewMarkMany({ windowKey: w, params: p }),
  'review.comment.add': (w, p) => GitService.ReviewCommentAdd({ windowKey: w, params: p }),
  'review.comment.list': (w, p) => GitService.ReviewCommentList({ windowKey: w, params: p }),
  'review.comment.remove': (w, p) => GitService.ReviewCommentRemove({ windowKey: w, params: p }),
  'review.comment.setState': (w, p) =>
    GitService.ReviewCommentSetState({ windowKey: w, params: p }),
  'review.comment.edit': (w, p) => GitService.ReviewCommentEdit({ windowKey: w, params: p }),
  'review.comment.clear': (w, p) => GitService.ReviewCommentClear({ windowKey: w, params: p }),
  'review.comment.export': (w, p) => GitService.ReviewCommentExport({ windowKey: w, params: p }),
  'commit.resolveChangeRequests': (w, p) =>
    GitService.CommitResolveChangeRequests({ windowKey: w, params: p }),
  'branch.resolveChangeRequest': (w, p) =>
    GitService.BranchResolveChangeRequest({ windowKey: w, params: p }),
  'branch.changeRequestIndex': (w, p) =>
    GitService.BranchChangeRequestIndex({ windowKey: w, params: p }),
  'branch.syncChangeRequests': (w, p) =>
    GitService.BranchSyncChangeRequests({ windowKey: w, params: p }),
  'gitHost.info': (w, p) => GitService.GitHostInfo({ windowKey: w, params: p }),
  'changeRequest.browserUrl': (w, p) =>
    GitService.ChangeRequestBrowserUrl({ windowKey: w, params: p }),
  'changeRequest.reviewState': (w, p) =>
    GitService.ChangeRequestReviewState({ windowKey: w, params: p }),
  'changeRequest.submitReview': (w, p) =>
    GitService.ChangeRequestSubmitReview({ windowKey: w, params: p }),
  'changeRequest.comment': (w, p) => GitService.ChangeRequestComment({ windowKey: w, params: p }),
  'changeRequest.lineComment': (w, p) =>
    GitService.ChangeRequestLineComment({ windowKey: w, params: p }),
  'gitHost.providers': () => GitHostService.Providers(),
} satisfies Partial<Record<RequestKey, BoundCall>>;

/** Every git method a bound call carries. */
export type BoundKey = keyof typeof BOUND_METHODS;

const boundTable: Partial<Record<RequestKey, BoundCall>> = BOUND_METHODS;

/** Every git event the Go side pushes to a window, as `kira:git:<event>` (EmitTo). */
const GIT_EVENT_PREFIX = 'kira:git:';

let windowReady: Promise<void> | undefined;

/** Forgets the memoized reset so the next call sends it again; unit specs share one module graph. */
export function forgetGitWindowReset(): void {
  windowReady = undefined;
}

/** Resets this window's Go connection once per page load, before any other git call: a reload
 *  keeps its window key, so the previous page's holds and walks are released at once instead of
 *  at window close. A failure clears the memo so the next caller retries. */
export function gitWindowReady(): Promise<void> {
  if (windowReady) return windowReady;
  const ready: Promise<void> = GitService.ResetWindow({ windowKey }).then(
    () => undefined,
    (err: unknown) => {
      windowReady = undefined;
      throw err;
    },
  );
  windowReady = ready;
  return ready;
}

/** Runs a bound call once the window reset settled. `signal` abort cancels the call (or the wait
 *  for the reset); `inflight` lets the owning transport cancel everything on dispose. */
export function callBound(
  method: RequestKey,
  params: unknown,
  signal: AbortSignal | undefined,
  inflight: Set<() => void>,
): Promise<unknown> {
  const call = boundTable[method];
  if (!call) return Promise.reject(new Error(`git method '${method}' is not bound`));
  if (signal?.aborted) {
    return Promise.reject(
      new TransportError('cancelled', `request '${method}' was already cancelled`),
    );
  }
  return new Promise((resolve, reject) => {
    let promise: Cancellable | undefined;
    let settled = false;
    const finish = () => {
      settled = true;
      inflight.delete(closeOut);
      signal?.removeEventListener('abort', onAbort);
    };
    const stop = (code: 'cancelled' | 'transport-closed') => {
      if (settled) return;
      finish();
      promise?.cancel();
      reject(new TransportError(code, `request '${method}' was ${code}`));
    };
    const fail = (err: unknown) => {
      if (settled) return;
      finish();
      const coded = toCodedError(err);
      reject(new RpcError({ code: coded.code ?? 'E_INTERNAL', message: coded.message }));
    };
    const closeOut = () => stop('transport-closed');
    const onAbort = () => stop('cancelled');
    inflight.add(closeOut);
    signal?.addEventListener('abort', onAbort);
    gitWindowReady().then(() => {
      if (settled) return;
      promise = call(windowKey, params as never);
      promise.then((value) => {
        if (settled) return;
        finish();
        resolve(value);
      }, fail);
    }, fail);
  });
}

/** Subscribes to a Go-pushed git event for this window; returns the unsubscribe. */
export function onGitEvent(event: string, handler: (payload: unknown) => void): () => void {
  return on<unknown>(GIT_EVENT_PREFIX + event, handler);
}
