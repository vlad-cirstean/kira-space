import type { SortSpec } from '@shared/domain/queries';
import type { PageCursor } from '@shared/protocol/dataOps';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { pinia } from '../../../state/pinia';
import type { DocumentTabState } from '../../../state/tabDomain';
import { useTabsStore } from '../../../state/tabs';
import { data } from '../../bridge/data';
import { useStaleDataStore } from '../../state/staleData';
import { registerTabCount, registerTabReload } from '../../state/viewCommands';
import { runPagedLoad } from '../shared/page/load';
import { createPageNavigation } from '../shared/page/navigation';
import {
  applyPagePosition,
  beginOp,
  createRuntimeStore,
  defaultPagedRuntime,
  type PagedViewRuntime,
  runPagedCount,
} from '../shared/viewOp';
import { setPage } from './page';

// Mirrors views/grid/state.ts's DataViewRuntime shape (status/pager/count) — projection, sort and
// pageSize now live on DocumentTabState (mirroring DataTabState) rather than being grid-only, so
// `searchOpen` (DataView.vue's precedent) and `selectedRow` (highlight-only local UI
// state, narrowed to a single row index since a document has no columns to select within — this
// view mounts no cell editor dock to publish into at all) are the only view-local runtime this
// adds.
interface DocumentViewRuntime extends PagedViewRuntime {
  rowCount: number;
  prevToken: string | null;
  /** P43 F3/D4: highlight-only local UI state, narrowed to a single row index since a document has
   *  no columns to select within — this view mounts no cell editor dock to publish into at all. */
  selectedRow: number | null;
  /** Unapplied filter-row text; `null` shows the committed value. Survives a tab switch (P375). */
  searchDraft: string | null;
  sortDraft: string | null;
}

function defaultRuntime(): DocumentViewRuntime {
  return {
    ...defaultPagedRuntime(),
    rowCount: 0,
    prevToken: null,
    selectedRow: null,
    searchDraft: null,
    sortDraft: null,
  };
}

export const useDocumentViewStore = defineStore('documentView', () => {
  const { runtime, ensureRuntime, setActionError, toggleSearchOpen, setSearchOpen } =
    createRuntimeStore<DocumentViewRuntime>(defaultRuntime);

  // D4: closeTab has no way to import this leaf module directly (reality 18) — registers here.
  registerTabRuntimeCleanup((tabId) => {
    delete runtime[tabId];
  });

  // Mirrors views/grid/state.ts's own `load()` — `revertPageIndexOnFailure`, when
  // given, is the pageIndex this tab was showing before its caller optimistically advanced it. A
  // failed or cancelled load never calls setPage, so without this the pager would show the new page
  // number while the grid still renders the old page's rows.
  async function load(
    tabId: string,
    cursor?: PageCursor,
    revertPageIndexOnFailure?: number,
  ): Promise<void> {
    const tab = useTabsStore().findDocumentTab(tabId);
    if (!tab?.connectionId) return;
    const rt = ensureRuntime(tabId);
    // Mirrors views/grid/state.ts's `load()`: the fallback cursor tracks `pageIndex * pageSize`,
    // not a hardcoded 0 — this is what keeps a bare `load(tabId)` (reload, or any setter below)
    // re-fetching the page the user is on instead of silently snapping back to page one.
    // Applies on the default (unsorted/`_id`-sorted) view too: mongo/read.ts's `skip` now runs on
    // any non-zero offset cursor, not just a real (non-`_id`) sort.
    const effectiveCursor: PageCursor = cursor ?? {
      mode: 'offset',
      offset: tab.state.pageIndex * tab.state.pageSize,
    };
    const opId = beginOp(rt);

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
          filter: tab.state.search.trim() === '' ? null : tab.state.search,
          sort: tab.state.sort,
          pageSize: tab.state.pageSize,
          cursor: effectiveCursor,
        }),
      expectKind: 'document',
      id: tabId,
      tabNoun: 'document tab',
      apply: (page) => {
        setPage(tabId, page);
        useStaleDataStore().stamp(tabId, tab.connectionId as string);
        applyPagePosition(rt, page);
      },
      onFailure: (superseded) => {
        if (!superseded && revertPageIndexOnFailure !== undefined) {
          useTabsStore().patchDocumentTabState(tabId, { pageIndex: revertPageIndexOnFailure });
        }
      },
    });
  }

  async function reload(tabId: string): Promise<void> {
    const tab = useTabsStore().findDocumentTab(tabId);
    if (!tab?.connectionId) return;
    await data.invalidate(tab.connectionId, tab.path);
    await load(tabId);
  }

  async function runCount(tabId: string): Promise<void> {
    const tab = useTabsStore().findDocumentTab(tabId);
    if (!tab?.connectionId) return;
    const connectionId = tab.connectionId;
    const rt = ensureRuntime(tabId);
    await runPagedCount(rt, (refresh) =>
      data.count({
        tabId,
        connectionId,
        path: tab.path,
        filter: tab.state.search.trim() === '' ? null : tab.state.search,
        refresh,
      }),
    );
  }

  // stop/goNext/goPrev/goFirst/goLast/goToPage/resetTokens — mirrors views/grid/state.ts's
  // own set exactly, modulo the tab accessor pair below. Mongo supports an arbitrary skip()/
  // limit() offset (unlike Redis's SCAN cursor or Kafka/SQS's per-partition offsets), so a
  // page-N jump is just as meaningful here as it is for SQL; goLast requires a count, same as the
  // grid's own, since the toolbar disables the Last-page button until one has run.
  const { stop, goNext, goPrev, goFirst, goLast, goToPage, resetTokens } = createPageNavigation({
    tab: (tabId) => useTabsStore().findDocumentTab(tabId)?.state,
    patch: (tabId, p) => useTabsStore().patchDocumentTabState(tabId, p),
    runtime: (tabId) => runtime[tabId],
    ensureRuntime,
    load,
  });

  function setSearch(tabId: string, text: string): void {
    const prevIndex = useTabsStore().findDocumentTab(tabId)?.state.pageIndex;
    resetTokens(tabId);
    // P43 F7/D10: same reasoning as views/grid/state.ts's setFilter — a count taken under the
    // previous search text answers a different question, not a drifted answer to this one.
    const rt = ensureRuntime(tabId);
    rt.count = null;
    rt.countOpId = null;
    rt.countError = null;
    useTabsStore().patchDocumentTabState(tabId, { search: text, pageIndex: 0 });
    void load(tabId, undefined, prevIndex);
  }

  // Mirrors views/grid/state.ts's setProjection/setSort/setPageSize — each resets `pageIndex` to 0
  // alongside the field it changes, same as the grid's own setters, since a new
  // filter/sort/projection/pageSize invalidates whatever "page N" meant under the old one. A
  // document tab pages by cursor token only when the sort is unset or purely by `_id` (mongo/
  // read.ts's keyset strategy); any other sort falls back to skip/limit and `pageIndex` is what
  // goNext/goPrev (above) use to compute that offset.
  function setProjection(tabId: string, projection: string[] | null): void {
    const prevIndex = useTabsStore().findDocumentTab(tabId)?.state.pageIndex;
    resetTokens(tabId);
    useTabsStore().patchDocumentTabState(tabId, { projection, pageIndex: 0 });
    void load(tabId, undefined, prevIndex);
  }

  function setSort(tabId: string, sort: SortSpec | null): void {
    const prevIndex = useTabsStore().findDocumentTab(tabId)?.state.pageIndex;
    resetTokens(tabId);
    useTabsStore().patchDocumentTabState(tabId, { sort, pageIndex: 0 });
    void load(tabId, undefined, prevIndex);
  }

  function setPageSize(tabId: string, pageSize: DocumentTabState['pageSize']): void {
    const prevIndex = useTabsStore().findDocumentTab(tabId)?.state.pageIndex;
    resetTokens(tabId);
    useTabsStore().patchDocumentTabState(tabId, { pageSize, pageIndex: 0 });
    void load(tabId, undefined, prevIndex);
  }

  // The clicked row's own highlight — this view has no cell editor dock to publish a
  // selection into (a document's own row is already the read/write surface). A plain runtime
  // field, not tab-persisted state — the same reasoning as the grid's own `selection`, which also
  // never round-trips through tabs.save.
  function selectRow(tabId: string, row: number | null): void {
    ensureRuntime(tabId).selectedRow = row;
  }

  // A document is expanded by default now — `expanded[id] === undefined` means expanded,
  // and only an explicit `false` collapses it. A tab saved under the old semantics (every row
  // missing meant collapsed) reads as `{}`, which under this new reading means "all expanded" —
  // exactly the new default, so no schema change and no migration.
  function isDocumentExpanded(tabId: string, id: string): boolean {
    const tab = useTabsStore().findDocumentTab(tabId);
    return tab ? tab.state.expanded[id] !== false : true;
  }

  function toggleExpanded(tabId: string, id: string): void {
    const tab = useTabsStore().findDocumentTab(tabId);
    if (!tab) return;
    const expanded = { ...tab.state.expanded };
    if (isDocumentExpanded(tabId, id)) {
      expanded[id] = false;
    } else {
      // Back to the default (expanded) — deleting the key keeps the map from growing an entry for
      // every row a user has ever touched, one direction of which never happens with `true`.
      delete expanded[id];
    }
    useTabsStore().patchDocumentTabState(tabId, { expanded });
  }

  // *Expand all* clears the map outright rather than writing one `true` per row — the
  // default is already expanded, so a 10 000-row page writes an empty object to `state_json`
  // instead of 10 000 keys. *Collapse all* is unchanged: it still needs one `false` per row.
  //
  // The collapse branch used to *replace* `state.expanded`
  // wholesale with a fresh map containing only `ids` (the currently rendered page) — discarding
  // every `false` entry for a document on any other page. Since an absent key means expanded,
  // paging away and back re-expanded everything collapsed there, and the map is persisted tab
  // state, so this survived a restart as the wrong value. Merging keeps every other page's own
  // collapsed entries intact; the `value === true` branch above is unaffected — clearing the whole
  // map genuinely does mean "everything, everywhere, expanded".
  function setAllExpanded(tabId: string, ids: string[], value: boolean): void {
    if (value) {
      useTabsStore().patchDocumentTabState(tabId, { expanded: {} });
      return;
    }
    const tab = useTabsStore().findDocumentTab(tabId);
    const expanded: Record<string, boolean> = { ...tab?.state.expanded };
    for (const id of ids) expanded[id] = false;
    useTabsStore().patchDocumentTabState(tabId, { expanded });
  }

  /** Shown page is stale: connection gone, or read under another connection session. */
  function isStale(tabId: string): boolean {
    const connectionId = useTabsStore().findDocumentTab(tabId)?.connectionId;
    return connectionId ? useStaleDataStore().isStale(tabId, tabId, connectionId) : false;
  }

  function setFilterDraft(
    tabId: string,
    draft: { search?: string | null; sort?: string | null },
  ): void {
    const rt = ensureRuntime(tabId);
    if (draft.search !== undefined) rt.searchDraft = draft.search;
    if (draft.sort !== undefined) rt.sortDraft = draft.sort;
  }

  return {
    runtime,
    isStale,
    setFilterDraft,
    load,
    reload,
    runCount,
    stop,
    goNext,
    goPrev,
    goFirst,
    goLast,
    goToPage,
    setSearch,
    setProjection,
    setSort,
    setPageSize,
    selectRow,
    isDocumentExpanded,
    toggleExpanded,
    setAllExpanded,
    setActionError,
    toggleSearchOpen,
    setSearchOpen,
  };
});

// D5/D6: project/ no longer imports this module directly — it reaches reload/runCount through
// state/viewCommands.ts's registry instead.
registerTabReload('document', (tabId) => useDocumentViewStore(pinia).reload(tabId));
registerTabCount('document', (tabId) => useDocumentViewStore(pinia).runCount(tabId));
