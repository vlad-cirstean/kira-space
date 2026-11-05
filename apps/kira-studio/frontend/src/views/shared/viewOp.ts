import { newUuid } from '@shared/ids';
import { errorText } from '@workbench/bridge/codedError';
import { fireAndForget } from '@workbench/bridge/fireAndForget';
import { reactive } from 'vue';
import { control } from '../../bridge/control';
import { useTabsStore } from '../../state/tabs';

// grid/state.ts, documents/state.ts, keyvalue/state.ts, stream/state.ts and
// console/state.ts each declared the same DISCONNECTED_CODES set and the same code/message
// extraction, and a byte-identical stop(). The reaction genuinely differs per caller (console's
// disconnected branch additionally sets `status = 'idle'` before unmarkHydrated, for a reason its
// own comment records) — classifyLoadError() returns a classification rather than performing the
// reaction, so every caller keeps its own exact lines.
//
// E_NOT_FOUND used to sit in this set alongside E_ENGINE_DOWN, on the
// theory that "not found" meant "the connection is gone". It doesn't — several adapters also throw
// E_NOT_FOUND for an ordinary query-time condition against a perfectly live connection (an unknown
// column in a stale projection/sort, a dropped table, a bad path segment), and reload()/reload-
// after-refresh code paths were hitting exactly that, unmarking hydration and popping the
// Reconnect & load gate over a table whose connection had never dropped. data.ts's/
// control.ts's own "no active adapter" throw is the one true "connection is gone" signal, and now
// carries E_ENGINE_DOWN, not E_NOT_FOUND, so this set no longer needs to (and must not) treat every
// not-found as a disconnect.
const DISCONNECTED_CODES = new Set(['E_ENGINE_DOWN', 'E_CONNECT']);

export interface LoadFailure {
  kind: 'cancelled' | 'disconnected' | 'error';
  code: string;
  message: string;
}

export function classifyLoadError(err: unknown): LoadFailure {
  const code = (err as { code?: string } | undefined)?.code ?? 'E_QUERY';
  const message = errorText(err);
  if (code === 'E_CANCELLED') return { kind: 'cancelled', code, message };
  if (DISCONNECTED_CODES.has(code)) return { kind: 'disconnected', code, message };
  return { kind: 'error', code, message };
}

/** `control.opsCancel(rt.opId)` when there is one — the body all five `stop()` functions share. */
export function stopOp(rt: { opId: string | null } | undefined): void {
  if (rt?.opId) fireAndForget(control.opsCancel(rt.opId), 'cancelOp');
}

/** The per-tab runtime record every view state module keeps, plus the one accessor that creates
 *  it. `runtime` is reactive() — Vue only wraps a nested plain object in its
 *  own reactive proxy when it's read back out through the parent proxy. `ensureRuntime` therefore
 *  re-reads `runtime[tabId]` rather than returning the object it just created: handing a caller
 *  that unwrapped local reference directly would mean every mutation made afterward (status,
 *  hasMore, tokens, ...) bypasses the proxy's `set` trap entirely, so no dependent render (e.g. a
 *  pager button's `disabled` binding) would ever be notified. Each view keeps its own
 *  `registerTabRuntimeCleanup` call — the cleanup body genuinely differs (console's also clears
 *  `results`) — and its own `defaultRuntime()`, since the six runtime shapes differ. */
type HasActionError = { actionError: string | null };
type HasSearchOpen = { searchOpen: boolean };

// setActionError (five identical four-line functions) and the search-open toggle
// (five onToggleSearch, four onCloseSearch copies) both reduce to "read/write one field on this
// tab's runtime" — the same shape ensureRuntime's own record already owns. The three setters below
// are conditionally present on the returned object, keyed off whether R carries the
// field: browse's runtime has actionError but no searchOpen (no find toolbar), console's has
// searchOpen but no actionError (nothing to mutate) — a store built over either type only ever
// exposes the setters that field supports, so calling the wrong one is a type error, not a no-op.
export function createRuntimeStore<R>(makeDefault: () => R): {
  runtime: Record<string, R>;
  ensureRuntime(tabId: string): R;
} & (R extends HasActionError
  ? { setActionError(tabId: string, message: string | null): void }
  : object) &
  (R extends HasSearchOpen
    ? { toggleSearchOpen(tabId: string): void; setSearchOpen(tabId: string, open: boolean): void }
    : object) {
  const runtime = reactive({} as Record<string, R>);

  function ensureRuntime(tabId: string): R {
    if (!runtime[tabId]) {
      runtime[tabId] = makeDefault();
    }
    return runtime[tabId];
  }

  function setActionError(tabId: string, message: string | null): void {
    const rt = runtime[tabId] as unknown as HasActionError | undefined;
    if (rt) rt.actionError = message;
  }

  function toggleSearchOpen(tabId: string): void {
    const rt = runtime[tabId] as unknown as HasSearchOpen | undefined;
    if (rt) rt.searchOpen = !rt.searchOpen;
  }

  function setSearchOpen(tabId: string, open: boolean): void {
    const rt = runtime[tabId] as unknown as HasSearchOpen | undefined;
    if (rt) rt.searchOpen = open;
  }

  return { runtime, ensureRuntime, setActionError, toggleSearchOpen, setSearchOpen } as never;
}

interface OpPreambleRuntime {
  status: string;
  opId: string | null;
  error: { code: string; message: string } | null;
  actionError: string | null;
}

// P48 F14: the op-start preamble — status/opId/error/actionError, after a newUuid() —
// byte-identical across grid/documents/keyvalue/stream's state.ts. Returns the op id it stamped,
// for the caller's own request and its `if (rt.opId !== opId) return` supersession checks.
export function beginOp(rt: OpPreambleRuntime): string {
  const opId = newUuid();
  rt.status = 'loading';
  rt.opId = opId;
  rt.error = null;
  rt.actionError = null;
  return opId;
}

interface LoadFailureRuntime {
  status: string;
  opId: string | null;
  error: { code: string; message: string } | null;
}

// The four-view failure tail, accrued one level past the classifyLoadError
// extraction — cancelled/disconnected/error react identically in grid/documents/keyvalue/stream's
// state.ts, and now in console/state.ts too via `onDisconnected`, the one extra line
// (`rt.status = 'idle'`) its own disconnected branch needs before unmarkHydrated (its own comment
// explains why: left at 'running', the Stop button would come back permanently red).
export function applyLoadFailure(
  rt: LoadFailureRuntime,
  opId: string,
  err: unknown,
  tabId: string,
  opts?: { onDisconnected?(): void },
): void {
  if (rt.opId !== opId) return;
  rt.opId = null;
  const failure = classifyLoadError(err);
  if (failure.kind === 'cancelled') {
    rt.status = 'cancelled';
    return;
  }
  if (failure.kind === 'disconnected') {
    // Left at 'loading' here for four of five views (only console's own
    // onDisconnected passed 'idle' along) — the ReconnectGate sits behind whatever status was
    // last set, so a tab that disconnects mid-load stayed on its loading UI behind the gate
    // instead of the same idle/error state a reconnect-then-load cycle leaves it in anyway. It
    // self-corrected once onReconnectAndLoad -> load() -> beginOp() ran, but that made the
    // invariant hold by luck at a distance rather than by construction.
    rt.status = 'idle';
    opts?.onDisconnected?.();
    useTabsStore().unmarkHydrated(tabId);
    return;
  }
  rt.status = 'error';
  rt.error = { code: failure.code, message: failure.message };
}

// The fields every paged view's own `*ViewRuntime` interface and `defaultRuntime()`
// repeated verbatim — documents/state.ts, keyvalue/state.ts, grid/state.ts, stream/state.ts. Each
// view's own interface `extends PagedViewRuntime` and adds only what genuinely differs: grid has
// no `rowCount` (it reads the page store directly instead), stream has no `prevToken` (Kafka/SQS
// browsing is forward-only) — neither field belongs in the shared base, so it stays out
// rather than being forced onto a runtime that never used it.
export interface PagedViewRuntime {
  status: 'idle' | 'loading' | 'error' | 'cancelled';
  error: { code: string; message: string } | null;
  actionError: string | null;
  opId: string | null;
  count: { value: number; exact: boolean; stale: boolean } | null;
  countOpId: string | null;
  countError: string | null;
  hasMore: boolean;
  nextToken: string | null;
  searchOpen: boolean;
}

export function defaultPagedRuntime(): PagedViewRuntime {
  return {
    status: 'idle',
    error: null,
    actionError: null,
    opId: null,
    count: null,
    countOpId: null,
    countError: null,
    hasMore: false,
    nextToken: null,
    searchOpen: false,
  };
}

interface ApplyPagePositionRuntime {
  status: PagedViewRuntime['status'];
  opId: string | null;
  rowCount: number;
  hasMore: boolean;
  nextToken: string | null;
  prevToken: string | null;
}

// documents/state.ts's and keyvalue/state.ts's own `load()` apply callback opened with this exact
// six-line block (only `setPage`/`rt` differ, which stay with each caller). grid/state.ts's and
// stream/state.ts's own apply blocks interleave the identical status/opId/hasMore/nextToken
// assignments with genuinely per-view lines (pending-changes clearing, `lastStrategy`, `polled`'s
// own reset) and don't track both `rowCount` and `prevToken` on their runtime — left as their own
// hand-written blocks rather than forced through this helper for two fields neither one needs.
export function applyPagePosition(
  rt: ApplyPagePositionRuntime,
  page: {
    rowCount: number;
    position: { hasMore: boolean; nextToken: string | null; prevToken: string | null };
  },
): void {
  rt.status = 'idle';
  rt.opId = null;
  rt.rowCount = page.rowCount;
  rt.hasMore = page.position.hasMore;
  rt.nextToken = page.position.nextToken;
  rt.prevToken = page.position.prevToken;
}

interface CountRuntime {
  count: { value: number; exact: boolean; stale: boolean } | null;
  countOpId: string | null;
  // F13 (P108 Part 10): setFilter nulls `count` before a fresh count runs — a count that then
  // fails used to leave the toolbar blank with no signal it failed or why. Surfaced in the count
  // chip; cleared by the next count that succeeds.
  countError: string | null;
}

let countStamp = 0;

// The countOpId-stamp/guard/assign skeleton — byte-identical across documents/grid/
// keyvalue/stream's state.ts runCount(). What genuinely differs per view (the request's own
// filter, which each caller derives its own way) stays with the caller: fetchCount closes over it
// and gets `refresh` (an already-fresh count stays an L3 hit) handed in, ready to forward.
export async function runPagedCount(
  rt: CountRuntime,
  fetchCount: (refresh: boolean) => Promise<{ value: number; exact: boolean; stale: boolean }>,
): Promise<void> {
  const stamp = String(++countStamp);
  rt.countOpId = stamp;
  try {
    const response = await fetchCount(rt.count?.stale === true);
    // A filter/refresh change since this count started already stamped a newer countOpId (or
    // cleared it) — an answer to the previous request landing now would resurrect a stale total.
    if (rt.countOpId !== stamp) return;
    rt.count = { value: response.value, exact: response.exact, stale: response.stale };
    rt.countError = null;
  } catch (err) {
    // A filter/refresh change since this count started already stamped a newer countOpId (or
    // cleared it) — same supersede guard as the success path above, so a stale failure can't
    // overwrite an error (or a clear) that belongs to a newer request.
    if (rt.countOpId !== stamp) return;
    // Leave the previous count (if any) rather than blanking it on a failed refresh.
    rt.countError = classifyLoadError(err).message;
  }
}
