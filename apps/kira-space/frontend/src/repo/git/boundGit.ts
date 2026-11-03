/**
 * Git methods served by the bound `GitService` instead of the git stream. `transport.ts` routes a
 * request here when its method has an entry; the rest still go over the stream.
 *
 * Imports the bindings directly, not through `bridge/index.ts`: `unwrap` returns a plain promise
 * and loses `.cancel()`, and git errors must surface as `RpcError` (git-ui branches on its code),
 * not `CodedError`.
 */
import * as GitService from '@bindings/gitservice.js';
import type { RequestKey } from '@kira/git-ipc';
import { RpcError, TransportError } from '@kira/git-ipc';
import { toCodedError } from '@workbench/bridge/codedError';
import { on, windowKey } from '@workbench/bridge/rpc';

interface Cancellable extends Promise<unknown> {
  cancel(): void;
}

type BoundCall = (windowKey: string, params: never) => Cancellable;

export const BOUND_METHODS: Partial<Record<RequestKey, BoundCall>> = {
  'app.init': (w) => GitService.AppInit({ windowKey: w }),
  'repo.open': (w, p) => GitService.RepoOpen({ windowKey: w, params: p }),
  'repo.close': (w, p) => GitService.RepoClose({ windowKey: w, params: p }),
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
};

/** Every git event the Go side pushes to a window, as `kira:git:<event>` (EmitTo). */
const GIT_EVENT_PREFIX = 'kira:git:';

/** Runs a bound call. `signal` abort cancels the binding promise; `inflight` lets the owning
 *  transport cancel everything on dispose. */
export function callBound(
  method: RequestKey,
  params: unknown,
  signal: AbortSignal | undefined,
  inflight: Set<() => void>,
): Promise<unknown> {
  const call = BOUND_METHODS[method];
  if (!call) return Promise.reject(new Error(`git method '${method}' is not bound`));
  if (signal?.aborted) {
    return Promise.reject(
      new TransportError('cancelled', `request '${method}' was already cancelled`),
    );
  }
  return new Promise((resolve, reject) => {
    const promise = call(windowKey, params as never);
    let settled = false;
    const finish = () => {
      settled = true;
      inflight.delete(closeOut);
      signal?.removeEventListener('abort', onAbort);
    };
    const stop = (code: 'cancelled' | 'transport-closed') => {
      if (settled) return;
      finish();
      promise.cancel();
      reject(new TransportError(code, `request '${method}' was ${code}`));
    };
    const closeOut = () => stop('transport-closed');
    const onAbort = () => stop('cancelled');
    inflight.add(closeOut);
    signal?.addEventListener('abort', onAbort);
    promise.then(
      (value) => {
        if (settled) return;
        finish();
        resolve(value);
      },
      (err: unknown) => {
        if (settled) return;
        finish();
        const coded = toCodedError(err);
        reject(new RpcError({ code: coded.code ?? 'E_INTERNAL', message: coded.message }));
      },
    );
  });
}

/** Subscribes to a Go-pushed git event for this window; returns the unsubscribe. */
export function onGitEvent(event: string, handler: (payload: unknown) => void): () => void {
  return on<unknown>(GIT_EVENT_PREFIX + event, handler);
}
