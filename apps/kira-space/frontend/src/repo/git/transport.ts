/**
 * The native `Transport` git-ui's `mount()` is handed, and the per-repo-
 * workspace cache over it ("one transport per repo workspace, not per mount").
 *
 * This implements `Transport` directly, dispatching per method to `hostHandlers.ts`'s own map
 * (`editor.openDiff`, `clipboard.write`, `app.init`'s host half and `repo.list` are host concerns
 * the Go server has no case for) or forwarding to the bound `GitService` (`boundGit.ts`); graph
 * rows ride the `gitgraph` data plane (`graphStream.ts`). A transport holds no socket: it is a
 * lease on repo holds and event subscriptions.
 *
 * This adds a small local event bus alongside the remote one: `review.target` and
 * `ui.action` are composed host-side (`review.open`'s handler, and the repaint choreography below)
 * and Go never emits either — the honest native analogue of what `KiraReviewViewProvider` does
 * with `this.#server?.emit(...)`, not a workaround. The repaint choreography lives here too,
 * as a post-processing step over `request()` (`reviewRepaint.ts`'s `afterReviewWrite`, shared with
 * direct `gitCall` writes).
 */
import type {
  EventKey,
  EventPayload,
  ParamsOf,
  RequestKey,
  ResultOf,
  StreamChunkOf,
  StreamKey,
  StreamParamsOf,
  Transport,
} from '@kira/git-ipc';
import { callBound, onGitEvent } from './boundGit';
import { streamGraph } from './graphStream';
import { clearPendingTargets, createHostHandlers } from './hostHandlers';
import { type GitRepoHold, holdGitRepo } from './repoHolds';
import { afterReviewWrite, registerCommentsRefresh } from './reviewRepaint';

/** The two `EventKey`s this host answers itself rather than forwarding to `remote.on` — never
 *  emitted by Go (§8.1). */
const LOCAL_EVENT_KEYS: ReadonlySet<EventKey> = new Set(['review.target', 'ui.action']);

/** A minimal typed pub-sub — all `emitLocal`/`on` need for the two events above. One instance per
 *  transport (per repo workspace), matching `remote.on`'s own scoping. */
function createLocalEmitter(): {
  on<K extends EventKey>(method: K, handler: (payload: EventPayload<K>) => void): () => void;
  /** Returns whether any listener was actually registered for `method` — a caller (e.g.
   *  `graph.revealCommit`'s handler, Group 6, P68 review) needs to tell "delivered live" apart
   *  from "nothing was listening" to decide whether a cold-mount stash is still needed. */
  emit<K extends EventKey>(method: K, payload: EventPayload<K>): boolean;
} {
  const handlersByMethod = new Map<EventKey, Set<(payload: unknown) => void>>();
  return {
    on(method, handler) {
      let handlers = handlersByMethod.get(method);
      if (!handlers) {
        handlers = new Set();
        handlersByMethod.set(method, handlers);
      }
      const asUnknown = handler as (payload: unknown) => void;
      handlers.add(asUnknown);
      return () => {
        handlersByMethod.get(method)?.delete(asUnknown);
      };
    },
    emit(method, payload) {
      const handlers = handlersByMethod.get(method);
      if (!handlers || handlers.size === 0) return false;
      for (const handler of handlers) handler(payload);
      return true;
    },
  };
}

/** Events pushed per repo; each payload names its `repoId`. `repoSettings.changed` is not scoped to
 *  a held repo (a viewer decides for itself), so it passes unfiltered. */
const REPO_SCOPED_EVENTS: ReadonlySet<EventKey> = new Set([
  'repo.changed',
  'autoFetch.changed',
  'remote.progress',
  'stack.progress',
]);

/** Every request goes through the window's shared Go connection (`bridge.GitWindows`) as a bound
 *  call; `graph.stream` rides the data plane. The transport tracks the repos it opened so its
 *  dispose releases their holds. */
function createNativeGitTransport(codeRepoId: string): Transport {
  const inflight = new Set<() => void>();
  const eventOffs = new Set<() => void>();
  const heldRepoIds = new Set<string>();
  let unregisterRefresh = () => {};
  // Every consumer shares the window's connection, but only the graph tab ever sent `repo.open`; a
  // reloaded Review pane or diff tab would otherwise compare against a connection holding no repo.
  const opened = new Map<string, Promise<unknown>>();
  // One window-level hold per opened path (repoHolds.ts).
  const holds = new Map<string, GitRepoHold>();
  const pathByRepoId = new Map<string, string>();
  const send = (method: RequestKey, params: unknown, signal?: AbortSignal): Promise<unknown> =>
    callBound(method, params, signal, inflight);
  const openHold = async (path: string): Promise<unknown> => {
    let hold = holds.get(path);
    if (!hold) {
      hold = holdGitRepo(path);
      holds.set(path, hold);
    }
    try {
      const result = await hold.ready;
      // A test double may answer null.
      const opened = result as ResultOf<'repo.open'> | null;
      const repoId = opened?.kind === 'ok' ? opened.repo.repoId : undefined;
      if (repoId) {
        heldRepoIds.add(repoId);
        pathByRepoId.set(repoId, path);
      }
      return result;
    } catch (err) {
      if (holds.get(path) === hold) holds.delete(path);
      hold.release();
      throw err;
    }
  };
  const releaseHolds = () => {
    for (const hold of holds.values()) hold.release();
    holds.clear();
    pathByRepoId.clear();
  };
  const ensureOpen = (params: unknown): Promise<unknown> | undefined => {
    const id = (params as { repoId?: unknown } | undefined)?.repoId;
    if (typeof id !== 'string' || id === '') return undefined;
    let held = opened.get(id);
    if (!held) {
      held = openHold(id).then(
        (result) => {
          heldRepoIds.add(id);
          return result;
        },
        (err: unknown) => {
          opened.delete(id);
          throw err;
        },
      );
      opened.set(id, held);
    }
    return held;
  };
  const remote = {
    async request(method: RequestKey, params: unknown, signal?: AbortSignal): Promise<unknown> {
      if (method === 'repo.close') {
        const id = (params as { repoId: string }).repoId;
        opened.delete(id);
        heldRepoIds.delete(id);
        const path = pathByRepoId.get(id) ?? id;
        holds.get(path)?.release();
        holds.delete(path);
        pathByRepoId.delete(id);
        await send(method, params, signal);
        return {};
      }
      if (method === 'repo.open') return openHold((params as { path: string }).path);
      await ensureOpen(params);
      return send(method, params, signal);
    },
    async stream(
      params: unknown,
      onChunk: (chunk: never) => void,
      signal?: AbortSignal,
    ): Promise<void> {
      await ensureOpen(params);
      return streamGraph(params as never, onChunk as never, signal);
    },
    on(method: EventKey, handler: (payload: never) => void): () => void {
      const scoped = REPO_SCOPED_EVENTS.has(method);
      const off = onGitEvent(method, (payload) => {
        if (scoped && !heldRepoIds.has((payload as { repoId: string }).repoId)) return;
        (handler as (payload: unknown) => void)(payload);
      });
      eventOffs.add(off);
      return () => {
        eventOffs.delete(off);
        off();
      };
    },
    dispose(): void {
      for (const off of eventOffs) off();
      eventOffs.clear();
      for (const cancel of [...inflight]) cancel();
      // Releases the holds a closed repo tab would otherwise keep until the window closes.
      releaseHolds();
      heldRepoIds.clear();
      unregisterRefresh();
    },
  };
  const local = createLocalEmitter();

  unregisterRefresh = registerCommentsRefresh(
    () => local.emit('ui.action', { action: 'refreshReviewComments' }),
    (repoId) => heldRepoIds.has(repoId),
  );

  const host = createHostHandlers({
    remoteRequest: remote.request as Transport['request'],
    codeRepoId,
    emitLocal: local.emit,
  });

  const transport: Transport = {
    request<K extends RequestKey>(
      method: K,
      params: ParamsOf<K>,
      signal?: AbortSignal,
    ): Promise<ResultOf<K>> {
      const handler = host[method];
      const result = handler
        ? handler(params, signal)
        : (remote.request(method, params, signal) as Promise<ResultOf<K>>);
      // Repaint every open review surface once a review write succeeds, never on a rejection.
      return result.then((value) => {
        afterReviewWrite(method, params);
        return value;
      });
    },
    // Every event Go emits (repo.changed, repoSettings.changed) and the one stream (graph.stream)
    // still forward to remote — only review.target/ui.action are ever local (§8.1).
    on<K extends EventKey>(method: K, handler: (payload: EventPayload<K>) => void): () => void {
      return LOCAL_EVENT_KEYS.has(method) ? local.on(method, handler) : remote.on(method, handler);
    },
    stream<K extends StreamKey>(
      _method: K,
      params: StreamParamsOf<K>,
      onChunk: (chunk: StreamChunkOf<K>) => void,
      signal?: AbortSignal,
    ): Promise<void> {
      return remote.stream(params, onChunk, signal);
    },
    dispose(): void {
      remote.dispose();
    },
  };

  return transport;
}

/** An `AbortController` that aborts as soon as either input signal does. Not `AbortSignal.any` —
 *  this repo does not rely on it anywhere else.
 *
 * `{once:true}` alone only removes the listener on the signal that fired — `b`'s own long-lived
 * signal is `leaseOf`'s `streams.signal`, which outlives this one `stream()` call for the whole
 * mount, so its listener (closing over `controller`) leaked for the mount's own lifetime every time
 * `a` (the caller's per-call signal) fired first, the normal case.
 * `onAbort` now removes both listeners itself the first time either fires, so `once:true` is no
 * longer needed on either registration. */
function linkAbort(a: AbortSignal, b?: AbortSignal): AbortSignal {
  if (!b) return a;
  const controller = new AbortController();
  if (a.aborted || b.aborted) {
    controller.abort();
    return controller.signal;
  }
  const onAbort = () => {
    a.removeEventListener('abort', onAbort);
    b.removeEventListener('abort', onAbort);
    controller.abort();
  };
  a.addEventListener('abort', onAbort);
  b.addEventListener('abort', onAbort);
  return controller.signal;
}

/** One real client per repo workspace, plus every outstanding lease on it (P67b §2.1). */
interface SharedClient {
  readonly transport: Transport;
  readonly leases: Set<Transport>;
}

/** A fresh view onto `shared.transport` per `gitTransportFor` call. `request` is deliberately
 *  untracked — an orphaned response from a departed mount resolves into a dead closure, exactly
 *  as it already does today for call sites (`RepoDiffView.vue`, `RepoFileView.vue`) that never
 *  dispose anything. `on` and `stream` ARE tracked, so a departed mount's subscriptions and
 *  long-lived streams (`graph.stream`) stop instead of quietly leaking. `dispose()`
 *  releases only this lease's own registrations and NEVER closes the underlying socket — that is
 *  `disposeGitTransport`'s job alone. */
function leaseOf(shared: SharedClient): Transport {
  const unsubs = new Set<() => void>();
  const streams = new AbortController();
  let released = false;
  const lease: Transport = {
    request(method, params, signal) {
      return shared.transport.request(method, params, signal);
    },
    on(method, handler) {
      if (released) return () => {};
      const off = shared.transport.on(method, handler);
      unsubs.add(off);
      return () => {
        unsubs.delete(off);
        off();
      };
    },
    stream(method, params, onChunk, signal) {
      return shared.transport.stream(method, params, onChunk, linkAbort(streams.signal, signal));
    },
    dispose(): void {
      if (released) return;
      released = true;
      for (const off of unsubs) off();
      unsubs.clear();
      streams.abort();
      shared.leases.delete(lease);
    },
  };
  shared.leases.add(lease);
  return lease;
}

const sharedClientsByCodeRepoId = new Map<string, SharedClient>();
/** Everything tied to one shared client's life, whether it ended on purpose or not. */
function evictSharedClient(codeRepoId: string, shared: SharedClient): void {
  sharedClientsByCodeRepoId.delete(codeRepoId);
  for (const lease of [...shared.leases]) lease.dispose();
  shared.transport.dispose();
}

/** One transport LEASE per call, over one shared client per repo workspace — cached across
 *  mount/unmount of the pinned graph tab, so a cold remount reuses the same underlying stream
 *  rather than opening a second one and losing whatever repo hold the first took. Never returns
 *  the same object twice: each caller (`RepoGraphView.vue`, `RepoReviewView.vue`,
 *  `RepoDiffView.vue`, `RepoFileView.vue`) gets its own lease, so one mount tearing its lease down
 *  on unmount no longer kills every other mount sharing the workspace. The shared client itself is
 *  disposed only when the workspace closes (`state/workspace.ts`), never on a mere tab-hide. */
export function gitTransportFor(codeRepoId: string): Transport {
  let shared = sharedClientsByCodeRepoId.get(codeRepoId);
  if (!shared) {
    shared = { transport: createNativeGitTransport(codeRepoId), leases: new Set() };
    sharedClientsByCodeRepoId.set(codeRepoId, shared);
  }
  return leaseOf(shared);
}

/** S17: called from the repo workspace's own close path. A no-op for a workspace whose graph tab
 *  was never mounted (no shared client was ever created). Disposes every outstanding lease first,
 *  then the shared client — so a workspace close still ends in exactly one `channel.close()`. */
export function disposeGitTransport(codeRepoId: string): void {
  const shared = sharedClientsByCodeRepoId.get(codeRepoId);
  if (!shared) return;
  clearPendingTargets(codeRepoId);
  evictSharedClient(codeRepoId, shared);
}
