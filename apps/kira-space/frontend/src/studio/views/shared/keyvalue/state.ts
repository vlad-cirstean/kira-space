import type { PageSize } from '@shared/domain/tabs';
import type { PageCursor } from '@shared/protocol/dataOps';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { data } from '../../../bridge/data';
import { pinia } from '../../../state/pinia';
import { useStaleDataStore } from '../../../state/staleData';
import { registerTabReload } from '../../../state/viewCommands';
import { runPagedLoad } from '../page/load';
import { createPageNavigation } from '../page/navigation';
import {
  applyPagePosition,
  beginOp,
  createRuntimeStore,
  defaultPagedRuntime,
  type PagedViewRuntime,
  runPagedCount,
  stopOp,
} from '../viewOp';
import { keyValueHost } from './host';
import { getPage, setPage } from './page';

// Mirrors views/documents/state.ts's DataViewRuntime shape, narrowed further: no expand/collapse
// memory (still no nesting to remember — a redis key's rows are always flat). `searchOpen`
// mirrors grid/state.ts's own field — search toggles a per-tab UI flag, not session state.
//
// Keyed by `viewKey`, not `tabId` — a real KeyValue tab's own id, or BrowseView.vue's
// `${tab.id}::preview` for the split's preview pane (host.ts's own seam).
interface KeyValueViewRuntime extends PagedViewRuntime {
  rowCount: number;
  prevToken: string | null;
  /** Path the held page was read for; a remount skips the read while it still matches. */
  loadedPath: string | null;
}

function defaultRuntime(): KeyValueViewRuntime {
  return {
    ...defaultPagedRuntime(),
    rowCount: 0,
    prevToken: null,
    loadedPath: null,
  };
}

export const useKeyValueViewStore = defineStore('keyValueView', () => {
  const { runtime, ensureRuntime, setActionError, toggleSearchOpen, setSearchOpen } =
    createRuntimeStore<KeyValueViewRuntime>(defaultRuntime);

  // D4: closeTab has no way to import this leaf module directly (reality 18) — registers here.
  // P63: also drops the browse split's own `${tabId}::preview` entry — that viewKey is not a real
  // tab id, so it never closes on its own; it dies only when the browse tab that owns it does.
  registerTabRuntimeCleanup((tabId) => {
    delete runtime[tabId];
    delete runtime[`${tabId}::preview`];
  });

  // Mirrors views/grid/state.ts's own `load()` — `revertPageIndexOnFailure`, when
  // given, is the pageIndex this tab was showing before its caller optimistically advanced it. A
  // failed or cancelled load never calls setPage, so without this the pager would show the new page
  // number while the grid still renders the old page's rows.
  async function load(
    viewKey: string,
    cursor?: PageCursor,
    revertPageIndexOnFailure?: number,
  ): Promise<void> {
    const host = keyValueHost(viewKey);
    if (!host?.connectionId) return;
    const rt = ensureRuntime(viewKey);
    let effectiveCursor: PageCursor;
    if (cursor) {
      effectiveCursor = cursor;
    } else {
      // A hash/set/zset/stream key's page is cursor-paged (redis/read.ts's
      // readScanFamily/readStream), and an `offset` cursor is neither honoured nor rejected by
      // either — it falls through and silently restarts the scan from the beginning. So a
      // no-cursor load (Refresh, a sibling tab's mutation, a post-Save reload) on any page but the
      // first of a cursor-paged key must ask for page one honestly, by resetting the tab's own
      // pager to match, rather than sending an offset the server ignores while the pager keeps
      // claiming a page further in. A list key's own LRANGE offset strategy is unaffected — this
      // only fires for a page that was already cursor-paged. Mirrors KeyValuePane's own
      // prevDisabled, which reads the same `position.strategy` to answer the same question.
      const currentPage = getPage(viewKey);
      if (currentPage && currentPage.position.strategy !== 'offset') {
        effectiveCursor = { mode: 'offset', offset: 0 };
        host.patch({ pageIndex: 0 });
      } else {
        effectiveCursor = { mode: 'offset', offset: host.pageIndex * host.pageSize };
      }
    }
    // A load still in flight for this viewKey (the row this preview/tab was
    // previously showing) must be CANCELLED, not just superseded client-side — beginOp below only
    // stamps a fresh opId, so the stale response gets dropped here (the `rt.opId !== opId` checks),
    // but the backend read it belongs to (a real Redis/S3 call) kept running to completion
    // regardless.
    // Rapidly clicking through N keys used to leave N-1 real backend reads running and discarded.
    // Mirrors FkPreviewPopover.vue's own opsCancel-on-supersede pattern.
    if (rt.opId) stopOp(rt);
    const opId = beginOp(rt);

    await runPagedLoad({
      rt,
      opId,
      stillMounted: () => Boolean(runtime[viewKey]),
      read: () =>
        data.read({
          opId,
          tabId: viewKey,
          connectionId: host.connectionId as string,
          path: host.path,
          projection: null,
          filter: null,
          sort: null,
          pageSize: host.pageSize,
          cursor: effectiveCursor,
        }),
      expectKind: 'keyvalue',
      id: viewKey,
      tabNoun: 'key/value tab',
      apply: (page) => {
        setPage(viewKey, page);
        rt.loadedPath = host.path;
        useStaleDataStore().stamp(viewKey, host.connectionId as string);
        applyPagePosition(rt, page);
      },
      onFailure: (superseded) => {
        if (!superseded && revertPageIndexOnFailure !== undefined) {
          host.patch({ pageIndex: revertPageIndexOnFailure });
        }
      },
    });
  }

  async function reload(viewKey: string): Promise<void> {
    const host = keyValueHost(viewKey);
    if (!host?.connectionId) return;
    await data.invalidate(host.connectionId, host.path);
    await load(viewKey);
  }

  async function runCount(viewKey: string): Promise<void> {
    const host = keyValueHost(viewKey);
    if (!host?.connectionId) return;
    const connectionId = host.connectionId;
    const rt = ensureRuntime(viewKey);
    await runPagedCount(rt, (refresh) =>
      data.count({ tabId: viewKey, connectionId, path: host.path, filter: null, refresh }),
    );
  }

  // stop/goNext/goPrev mirror views/grid/state.ts's own set exactly, modulo the tab
  // accessor pair below — hash/set/zset/stream's cursor strategy is preferred when available,
  // falling back to `pageIndex`-tracked offset paging (a list key's LRANGE offset strategy has no
  // token to advance by). Only stop/goNext/goPrev are exposed — this view has no goFirst/goLast/
  // goToPage (a key's rows have no addressable position to jump to the way a table's do).
  const { stop, goNext, goPrev, resetTokens } = createPageNavigation({
    tab: (viewKey) => keyValueHost(viewKey) ?? undefined,
    patch: (viewKey, p) => keyValueHost(viewKey)?.patch(p),
    runtime: (viewKey) => runtime[viewKey],
    ensureRuntime,
    load,
  });

  // Mirrors grid/state.ts's setPageSize: resets to the first page and clears whatever cursor
  // tokens were held for the old page size (a SCAN cursor from a 100-sized page is not valid
  // against a 1000-sized one).
  async function setPageSize(viewKey: string, pageSize: PageSize): Promise<void> {
    const host = keyValueHost(viewKey);
    const prevIndex = host?.pageIndex;
    resetTokens(viewKey);
    host?.patch({ pageSize, pageIndex: 0 });
    await load(viewKey, { mode: 'offset', offset: 0 }, prevIndex);
  }

  /** Shown page is stale: connection gone, or read under another connection session. A preview
   *  pane is also stale when its owning browse tab is. */
  function isStale(viewKey: string): boolean {
    const host = keyValueHost(viewKey);
    if (!host?.connectionId) return false;
    const staleStore = useStaleDataStore();
    if (staleStore.isStale(viewKey, host.gateTabId, host.connectionId)) return true;
    return (
      host.gateTabId !== viewKey &&
      staleStore.isStale(host.gateTabId, host.gateTabId, host.connectionId)
    );
  }

  /** The held page already shows `path` and is not stale, so a remount needs no read. */
  function hasPageFor(viewKey: string, path: string): boolean {
    return (
      runtime[viewKey]?.loadedPath === path && getPage(viewKey) !== undefined && !isStale(viewKey)
    );
  }

  return {
    runtime,
    isStale,
    hasPageFor,
    load,
    reload,
    runCount,
    stop,
    goNext,
    goPrev,
    setPageSize,
    // Written by KeyValueView.vue's own catch around onDeleteKey — the popover-local
    // editError/objectSaveError/addError refs already cover the edit/add surfaces,
    // this is what was missing: delete has no popover to hold a local ref, so it gets the shared
    // per-tab field every other immediate-mutation view uses.
    setActionError,
    toggleSearchOpen,
    setSearchOpen,
  };
});

// project/ no longer imports this module directly — it reaches reload through
// state/viewCommands.ts's registry instead. Registered eagerly here (module scope, explicit pinia
// singleton) since this module is reached via the static import chain before app.use(pinia) runs
// (state/viewCommands.ts's own comment: an unregistered kind throws here, it does not no-op).
registerTabReload('keyvalue', (tabId) => useKeyValueViewStore(pinia).reload(tabId));
