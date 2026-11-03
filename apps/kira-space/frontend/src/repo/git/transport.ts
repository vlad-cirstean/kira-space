/**
 * The native `Transport` git-ui's `mount()` is handed, and the per-repo-
 * workspace cache over it ("one transport per repo workspace, not per mount").
 *
 * This implements `Transport` directly, dispatching per method to `hostHandlers.ts`'s own map
 * (`editor.openDiff`, `clipboard.write`, `app.init`'s host half and `repo.list` are host concerns
 * the Go server has no case for) or forwarding to the git stream.
 *
 * No length prefix, no pairing, no trust store: `streamChannel.ts` is message-framed already,
 * and `internal/bridge/gitstream.go`'s own doc comment states why there is no handshake — the peer
 * is this process's own webview, not an external client a trust store exists to gate
 * (`docs/HISTORY.md (v1.5 C10)`).
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
import { createRpcClient, createStreamChannel } from '@kira/git-ipc';
// tsconfig.json maps this specifier onto @wailsio/runtime's real types (see bridge/port.ts's own
// identical comment) and resolves it cleanly under typecheck:web; tests/unit's own project has no
// such mapping (breaks Bun's mock.module interception for every transitive importer), so this
// stays the suppress-if-present directive that file already established, not the require-an-error
// kind (which would itself fail where this resolves fine).
// biome-ignore lint/suspicious/noTsIgnore: an "unused directive" kind fails where this resolves fine (see comment above)
// @ts-ignore
import { Stream } from '/wails/runtime.js';
import { BOUND_METHODS, callBound, onGitEvent } from './boundGit';
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

/** One named stream per repo workspace for the methods `GitService` does not carry yet; bound
 *  methods and every event go through the window's shared Go connection (`bridge.GitWindows`).
 *  Repo opens go to both: in a native window they reach the same connection, and the server build
 *  can key the stream differently from the page. */
function createNativeGitTransport(codeRepoId: string): Transport {
  const channel = createStreamChannel(Stream('git'));
  const rpc = createRpcClient(channel);
  const inflight = new Set<() => void>();
  const eventOffs = new Set<() => void>();
  const heldRepoIds = new Set<string>();
  let unregisterRefresh = () => {};
  // Every consumer shares this connection, but only the graph tab ever sent `repo.open`; a reloaded
  // Review pane or diff tab would otherwise compare against a connection holding no repo.
  const opened = new Map<string, Promise<unknown>>();
  // One window-level hold per opened path (repoHolds.ts); the stream half stays per transport.
  const holds = new Map<string, GitRepoHold>();
  const pathByRepoId = new Map<string, string>();
  const send = (method: RequestKey, params: unknown, signal?: AbortSignal): Promise<unknown> =>
    method in BOUND_METHODS
      ? callBound(method, params, signal, inflight)
      : rpc.request(method, params as never, signal);
  const openBoth = async (path: string, signal?: AbortSignal): Promise<unknown> => {
    let hold = holds.get(path);
    if (!hold) {
      hold = holdGitRepo(path);
      holds.set(path, hold);
    }
    try {
      const result = await hold.ready;
      await rpc.request('repo.open', { path }, signal);
      const repoId = result.kind === 'ok' ? result.repo.repoId : undefined;
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
      held = openBoth(id).then(
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
        await rpc.request(method, params as never, signal);
        return {};
      }
      if (method === 'repo.open') return openBoth((params as { path: string }).path, signal);
      await ensureOpen(params);
      return send(method, params, signal);
    },
    async stream(
      method: StreamKey,
      params: unknown,
      onChunk: (chunk: never) => void,
      signal?: AbortSignal,
    ): Promise<void> {
      await ensureOpen(params);
      return rpc.stream(method, params as never, onChunk as never, signal);
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
      rpc.dispose();
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
      method: K,
      params: StreamParamsOf<K>,
      onChunk: (chunk: StreamChunkOf<K>) => void,
      signal?: AbortSignal,
    ): Promise<void> {
      return remote.stream(method, params, onChunk, signal);
    },
    dispose(): void {
      remote.dispose();
    },
  };

  // The channel closing (including a self-inflicted close from a decodeFrame failure) used to
  // go unnoticed entirely — nothing ever subscribed to onClose, so every already-in-flight
  // request/stream on this client hung forever, and a stale entry in sharedClientsByCodeRepoId
  // kept handing the now-dead transport to every later gitTransportFor call. remote.dispose()
  // rejects every pending request/stream immediately; evicting the map entry makes the next
  // gitTransportFor call dial a fresh connection instead of reusing this one.
  //
  // The identity check guards a narrower race this fix adds: `onClose` fires
  // asynchronously, from the socket's own close event, strictly after `channel.close()` returns —
  // so `disposeGitTransport(id)` followed by a same-tick `gitTransportFor(id)` (collapse/re-expand
  // a worktree, or close/reopen a workspace) can install a NEW shared client for `codeRepoId`
  // before this stale handler runs. Deleting unconditionally would evict that new, live entry
  // instead of this dead one, orphaning it: still open (socket, Go `gitsession.Conn`, repo hold),
  // but unreachable from `sharedClientsByCodeRepoId` for the rest of the window's life.
  channel.onClose(() => {
    remote.dispose();
    const shared = sharedClientsByCodeRepoId.get(codeRepoId);
    if (shared?.transport !== transport) return;
    evictSharedClient(codeRepoId, shared);
    // Leases died with the client; their holders re-lease on demand (a self-inflicted close is the
    // only path here — an intentional one already evicted the entry).
    for (const fn of [...(evictionListeners.get(codeRepoId) ?? [])]) fn();
  });

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
/** Pause before a lease holder re-leases after an unexpected close, so a client that keeps dying
 *  cannot spin a redial loop. */
export const RELEASE_RETRY_MS = 1000;
const evictionListeners = new Map<string, Set<() => void>>();

/** Runs `fn` when `codeRepoId`'s shared client closes on its own, not through
 *  `disposeGitTransport`. Long-lived lease holders use it to re-lease; returns the unsubscribe. */
export function onTransportEvicted(codeRepoId: string, fn: () => void): () => void {
  let set = evictionListeners.get(codeRepoId);
  if (!set) {
    set = new Set();
    evictionListeners.set(codeRepoId, set);
  }
  set.add(fn);
  return () => {
    set.delete(fn);
    if (set.size === 0 && evictionListeners.get(codeRepoId) === set)
      evictionListeners.delete(codeRepoId);
  };
}

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
