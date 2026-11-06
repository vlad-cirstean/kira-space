import type { SortSpec } from '@shared/domain/queries';
import type { ObjectMeta } from '@shared/domain/tree';
import type { PageCursor } from '@shared/protocol/dataOps';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { control } from '../../bridge/control';
import { data } from '../../bridge/data';
import { pinia } from '../../state/pinia';
import { useStaleDataStore } from '../../state/staleData';
import type { DataTabState } from '../../state/tabDomain';
import { useTabsStore } from '../../state/tabs';
import {
  registerDataQueryCommands,
  registerTabCount,
  registerTabReload,
  reloadTabsForTarget,
} from '../../state/viewCommands';
import { runPagedLoad } from '../shared/page/load';
import { createPageNavigation } from '../shared/page/navigation';
import type { Selection } from '../shared/slick/selection';
import {
  beginOp,
  createRuntimeStore,
  defaultPagedRuntime,
  type PagedViewRuntime,
  runPagedCount,
} from '../shared/viewOp';
import { clearCellFocus } from './focusRequest';
import { getPage, setPage } from './page';
import { usePendingChangesStore } from './pendingChanges';

// P19 D7: the type itself moved to views/shared/slick/selection.ts (the file that already owns
// its geometry, and which views/console/ now needs too) — re-exported here so every existing
// `import type { Selection } from './state'` site is unchanged.
export type { Selection };

interface DataViewRuntime extends PagedViewRuntime {
  meta: ObjectMeta | null; // from kira:tree:describe (L1) — the projection menu
  lastStrategy: 'keyset' | 'offset';
  prevToken: string | null;
  selection: Selection | null;
  /** This tab's own masking preview toggle — per-tab, session-only, never persisted (a
   *  masked view must not silently outlive the session and leave a user reading buckets as real
   *  numbers tomorrow). A preview convenience, not a security boundary — the MCP path is
   *  the real one. */
  maskPreview: boolean;
  /** Widths measured for the column set `key` names; reused across reloads while that set is
   *  unchanged. Session-only, dropped with the tab's runtime. */
  settledWidths: { key: string; widths: Record<string, number> } | null;
  /** Unapplied filter-row text; `null` shows the committed value. Survives a tab switch (P375). */
  whereDraft: string | null;
  orderByDraft: string | null;
}

function defaultRuntime(): DataViewRuntime {
  return {
    ...defaultPagedRuntime(),
    meta: null,
    lastStrategy: 'offset',
    prevToken: null,
    selection: null,
    maskPreview: false,
    settledWidths: null,
    whereDraft: null,
    orderByDraft: null,
  };
}

export const useGridViewStore = defineStore('gridView', () => {
  const { runtime, ensureRuntime, setActionError, toggleSearchOpen, setSearchOpen } =
    createRuntimeStore<DataViewRuntime>(defaultRuntime);

  /** The pending-changes guard lives here, not at any one caller (it used to sit only at
   *  DataToolbar.vue's own toggle button, so menu.ts's `markColumnMaskKind` — marking a column
   *  PII via the header menu — could turn preview on directly, bypassing it, while a cell edit or
   *  insert row was staged. That left `maskPreview` and pending changes coexisting, a state
   *  nothing downstream (the toolbar's disabled toggle, DataView.vue's edit-action gating,
   *  SlickGridHost's own "no insert row while masked" assumption) was written to handle.
   *  Centralizing the guard here means no caller, menu or toolbar or otherwise, can ever turn
   *  preview on while a change is pending — the one place this invariant needs to hold.
   *
   *  Returns whether preview turned on (or off) — `on && hasPending` is the one case it
   *  didn't: `markColumnMaskKind` silently swallowed that outcome, so a column marked
   *  PII while an edit was staged looked like nothing happened at all, with no feedback anywhere.
   */
  function setMaskPreview(tabId: string, on: boolean): boolean {
    if (on && usePendingChangesStore().hasPending(tabId)) return false;
    ensureRuntime(tabId).maskPreview = on;
    return true;
  }
  function toggleMaskPreview(tabId: string): void {
    const rt = ensureRuntime(tabId);
    if (!rt.maskPreview && usePendingChangesStore().hasPending(tabId)) return;
    rt.maskPreview = !rt.maskPreview;
  }

  // D4: `runtime` is this view's per-tab record — closeTab has no way to import this leaf module
  // directly (reality 18), so it registers here instead. P67 §5.2: also drops that tab's own
  // pending focus request (focusRequest.ts), if any — a request never outlives the tab it
  // targeted.
  registerTabRuntimeCleanup((tabId) => {
    delete runtime[tabId];
    clearCellFocus(tabId);
  });

  async function loadMeta(tabId: string): Promise<void> {
    const tab = useTabsStore().findDataTab(tabId);
    if (!tab?.connectionId) return;
    const rt = ensureRuntime(tabId);
    try {
      const result = await control.treeDescribe(tab.connectionId, tab.path);
      rt.meta = result.meta;
    } catch {
      // The projection menu is a nicety fed by this — a failure here must not block reading rows.
    }
  }

  // `revertPageIndexOnFailure`, when given, is the pageIndex the tab was showing
  // before its caller optimistically advanced it for this load — goNext/goPrev/goFirst/goLast/
  // goToPage and the filter/sort/projection/pageSize setters below all patch pageIndex to the new
  // page *before* the (possibly failing) request completes, on the assumption it will land. A
  // failed or cancelled load never calls setPage (the comment below explains why: the previously
  // rendered page must stay on screen), so without this the pager would show the new page number
  // while the grid still renders the old page's rows.
  async function load(
    tabId: string,
    cursor?: PageCursor,
    revertPageIndexOnFailure?: number,
  ): Promise<void> {
    const tab = useTabsStore().findDataTab(tabId);
    if (!tab?.connectionId) return;
    const rt = ensureRuntime(tabId);

    const effectiveCursor: PageCursor = cursor ?? {
      mode: 'offset',
      offset: tab.state.pageIndex * tab.state.pageSize,
    };
    const opId = beginOp(rt);

    // A 'data' tab only ever exists against a tabular-shaped adapter (Postgres/MariaDB) — Mongo
    // opens a 'document' tab instead — so expectKind narrows rather than widening
    // setPage/getPage. A stop button that blanks the grid is worse than the query the user
    // stopped — a failure here (below) leaves the previously rendered page exactly as it was.
    await runPagedLoad({
      rt,
      opId,
      stillMounted: () => Boolean(runtime[tabId]),
      read: () =>
        data.read({
          opId,
          tabId,
          connectionId: tab.connectionId as string,
          path: tab.path,
          projection: tab.state.projection,
          filter: tab.state.filter,
          sort: tab.state.sort,
          pageSize: tab.state.pageSize,
          cursor: effectiveCursor,
        }),
      expectKind: 'tabular',
      id: tabId,
      tabNoun: 'data tab',
      apply: (page) => {
        setPage(tabId, page);
        useStaleDataStore().stamp(tabId, tab.connectionId as string);
        rt.status = 'idle';
        rt.opId = null;
        rt.hasMore = page.position.hasMore;
        rt.nextToken = page.position.nextToken;
        rt.prevToken = page.position.prevToken;
        // 'cursor'/'offsetWindow'/'batch' are keyvalue/stream-page concepts (P9/P10) — a tabular
        // page's own keyset/offset readers never produce them.
        const strategy = page.position.strategy;
        if (strategy !== 'keyset' && strategy !== 'offset') {
          throw new Error(`unexpected ${strategy} pagination for a tabular page`);
        }
        rt.lastStrategy = strategy;
        if (!rt.meta) void loadMeta(tabId);
      },
      onFailure: (superseded) => {
        // A newer load has already taken over pageIndex (and this one's own optimistic patch is
        // ancient history by comparison) — reverting here would stomp on state this failure has
        // nothing to do with.
        if (!superseded && revertPageIndexOnFailure !== undefined) {
          useTabsStore().patchDataTabState(tabId, { pageIndex: revertPageIndexOnFailure });
        }
        // P67 §5.2: a load that produced no page can never satisfy a pending focus request —
        // leaving it pending would let a *later*, unrelated load consume it and jump somewhere
        // nobody asked for.
        clearCellFocus(tabId);
      },
    });
  }

  // The explicit ↻ Refresh affordance — hard-drops both pages and the count (default `scope: 'all'`).
  async function reload(tabId: string): Promise<void> {
    const tab = useTabsStore().findDataTab(tabId);
    if (!tab?.connectionId) return;
    await data.invalidate(tab.connectionId, tab.path);
    await load(tabId, { mode: 'offset', offset: tab.state.pageIndex * tab.state.pageSize });
  }

  // D18: the post-commit reload. `handleMutate` has already dropped pages and marked the count
  // stale server-side (cache.invalidateAfterMutation) — invalidating with `scope: 'pages'` here
  // reloads the grid without erasing that stale mark a moment after it was set.
  async function reloadAfterMutation(tabId: string): Promise<void> {
    const tab = useTabsStore().findDataTab(tabId);
    if (!tab?.connectionId) return;
    await data.invalidate(tab.connectionId, tab.path, 'pages');
    await load(tabId, { mode: 'offset', offset: tab.state.pageIndex * tab.state.pageSize });
    // A commit that deletes every row on the current page left this reload
    // showing an empty grid still labelled page N — nothing else corrects it (goLast/the count
    // chip both still read the pre-delete total until a fresh count runs). Step back one page,
    // same as a manual Previous click, when the reload above came back empty.
    const reloadedTab = useTabsStore().findDataTab(tabId);
    const reloadedPage = getPage(tabId);
    if (
      reloadedPage &&
      reloadedPage.rowCount === 0 &&
      reloadedTab &&
      reloadedTab.state.pageIndex > 0
    ) {
      const prevIndex = reloadedTab.state.pageIndex - 1;
      useTabsStore().patchDataTabState(tabId, { pageIndex: prevIndex });
      await load(tabId, { mode: 'offset', offset: prevIndex * tab.state.pageSize });
    }
    // The "immediately marked stale" needs the toolbar's own `rt.count` mirror to pick up the
    // server-side mark, which nothing else here does. `rt.count?.stale` is still false at this
    // point, so runCount's own `refresh` flag stays false too — this reads the now-stale L3 entry
    // (a cache hit, not a rescan; handleRead/handleCount's contract keeps a hit out of the op log)
    // rather than forcing a real recount. Skipped when this tab never ran a count in the first
    // place — nothing to grey, and nothing cached yet to read this cheaply.
    if (runtime[tabId]?.count) await runCount(tabId);
    // P43 F10/D14: a second tab open on this same table kept rendering rows this commit just
    // changed — this tab is already correcting itself above, so it is the one excepted here.
    reloadTabsForTarget(tab.connectionId, tab.path, tabId);
  }

  async function runCount(tabId: string): Promise<void> {
    const tab = useTabsStore().findDataTab(tabId);
    if (!tab?.connectionId) return;
    const connectionId = tab.connectionId;
    const rt = ensureRuntime(tabId);
    await runPagedCount(rt, (refresh) =>
      data.count({ tabId, connectionId, path: tab.path, filter: tab.state.filter, refresh }),
    );
  }

  // stop/goFirst/goNext/goPrev/goLast/goToPage/resetTokens — mirrors views/documents/
  // state.ts's own set exactly, modulo the tab accessor pair below. Every state-changing control
  // resets paging to page 0 — page 40 at 100 rows is not page 40 at 10 000, and a changed
  // filter/sort/projection invalidates whatever tokens were held; goLast requires a count — the
  // toolbar disables ⏭ until Σ has run.
  const { stop, goNext, goPrev, goFirst, goLast, goToPage, resetTokens } = createPageNavigation({
    tab: (tabId) => useTabsStore().findDataTab(tabId)?.state,
    patch: (tabId, p) => useTabsStore().patchDataTabState(tabId, p),
    runtime: (tabId) => runtime[tabId],
    ensureRuntime,
    load,
  });

  async function setPageSize(tabId: string, pageSize: DataTabState['pageSize']): Promise<void> {
    const prevIndex = useTabsStore().findDataTab(tabId)?.state.pageIndex;
    resetTokens(tabId);
    useTabsStore().patchDataTabState(tabId, { pageSize, pageIndex: 0 });
    await load(tabId, { mode: 'offset', offset: 0 }, prevIndex);
  }

  async function setProjection(tabId: string, projection: string[] | null): Promise<void> {
    const prevIndex = useTabsStore().findDataTab(tabId)?.state.pageIndex;
    resetTokens(tabId);
    useTabsStore().patchDataTabState(tabId, { projection, pageIndex: 0 });
    await load(tabId, { mode: 'offset', offset: 0 }, prevIndex);
  }

  async function setFilter(tabId: string, filter: string | null): Promise<void> {
    resetTokens(tabId);
    // P43 F7/D10: a count taken under the previous WHERE is an answer to a different question, not
    // a drifted answer to this one — `stale` (§7) would still leave a wrong `of M` in the pager and
    // still let ⏭ page past the end. Clearing it (not staling it) returns the pager to "page N" with
    // no total, exactly what an un-counted state already looks like. Projection/sort setters below
    // don't do this: neither changes which rows match.
    const prevIndex = useTabsStore().findDataTab(tabId)?.state.pageIndex;
    const rt = ensureRuntime(tabId);
    rt.count = null;
    rt.countOpId = null;
    rt.countError = null;
    useTabsStore().patchDataTabState(tabId, { filter, pageIndex: 0 });
    await load(tabId, { mode: 'offset', offset: 0 }, prevIndex);
  }

  async function setSort(tabId: string, sort: SortSpec | null): Promise<void> {
    const prevIndex = useTabsStore().findDataTab(tabId)?.state.pageIndex;
    resetTokens(tabId);
    useTabsStore().patchDataTabState(tabId, { sort, pageIndex: 0 });
    await load(tabId, { mode: 'offset', offset: 0 }, prevIndex);
  }

  // Purely a display concern (which page.columns index each display column reads from) — unlike
  // projection/filter/sort above, reordering never changes what the query returns, so this
  // neither resets tokens nor reloads.
  function setColumnOrder(tabId: string, columnOrder: string[] | null): void {
    useTabsStore().patchDataTabState(tabId, { columnOrder });
  }

  function settledWidthsFor(tabId: string): DataViewRuntime['settledWidths'] {
    return runtime[tabId]?.settledWidths ?? null;
  }

  function setSettledWidths(tabId: string, key: string, widths: Record<string, number>): void {
    ensureRuntime(tabId).settledWidths = { key, widths };
  }

  function setFilterDraft(
    tabId: string,
    draft: { where?: string | null; orderBy?: string | null },
  ): void {
    const rt = ensureRuntime(tabId);
    if (draft.where !== undefined) rt.whereDraft = draft.where;
    if (draft.orderBy !== undefined) rt.orderByDraft = draft.orderBy;
  }

  /** Shown page is stale: connection gone, or read under another connection session. */
  function isStale(tabId: string): boolean {
    const connectionId = useTabsStore().findDataTab(tabId)?.connectionId;
    return connectionId ? useStaleDataStore().isStale(tabId, tabId, connectionId) : false;
  }

  return {
    runtime,
    isStale,
    settledWidthsFor,
    setSettledWidths,
    setFilterDraft,
    load,
    reload,
    reloadAfterMutation,
    runCount,
    stop,
    goFirst,
    goNext,
    goPrev,
    goLast,
    goToPage,
    setPageSize,
    setProjection,
    setFilter,
    setSort,
    setColumnOrder,
    setMaskPreview,
    toggleMaskPreview,
    setActionError,
    toggleSearchOpen,
    setSearchOpen,
  };
});

// pendingChanges.ts already imports from this
// module, so importing `runtime` back from there would be a cycle — it registers an accessor
// instead (the same registry-inversion shape as state/viewCommands.ts's registerTabReload below),
// so buildPlan can tell a partial composite-key projection from a complete one.
usePendingChangesStore(pinia).registerFullPrimaryKeyAccessor(
  (tabId) => useGridViewStore(pinia).runtime[tabId]?.meta?.primaryKey ?? null,
);

// D5/D6: project/ no longer imports this module directly — it reaches reload/runCount/
// setFilter/setProjection/setSort through state/viewCommands.ts's registry instead.
registerTabReload('data', (tabId) => useGridViewStore(pinia).reload(tabId));
registerTabCount('data', (tabId) => useGridViewStore(pinia).runCount(tabId));
registerDataQueryCommands({
  setFilter: (tabId, filter) => useGridViewStore(pinia).setFilter(tabId, filter),
  setSort: (tabId, sort) => useGridViewStore(pinia).setSort(tabId, sort),
  setProjection: (tabId, projection) => useGridViewStore(pinia).setProjection(tabId, projection),
});
