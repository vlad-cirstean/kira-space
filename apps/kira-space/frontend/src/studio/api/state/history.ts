import { useQuery } from '@tanstack/vue-query';
import { errorText } from '@workbench/bridge/codedError';
import { KEEP_WHILE_OPEN, queryClient } from '@workbench/state/queryClient';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { computed, type MaybeRefOrGetter, reactive, toValue, watch } from 'vue';
import { refreshApiQuery } from './apiQueries';

// The per-protocol response-history server state — the list per scope and one snapshot per entry —
// as TanStack Query, plus the small per-tab UI state (the entry being viewed, the last action
// error) the two Pinia stores wrap. Deliberately does not reuse views/shared/viewOp.ts's
// createRuntimeStore: that file lives under views/**, which api/** may not import.
//
// Keys mirror Go's scope key (repos/history.go): `[domain, itemId, scratchTabId]`, the tab id only
// for a scratch tab, so two tabs on one saved request share one list.
//
// `enabled`: the list observer is enabled only while the History pane shows. A disabled
// observer still reads the cached list (the segment count) but is not `active`, so invalidating
// after a send only marks it stale; showing the pane refetches once. A prefetch on pane mount and
// on an itemId change (Save as adopts the scratch history) is the "one initial fetch".

interface HistoryTab {
  id: string;
  state: { itemId?: string | null; responsePane: string };
}

interface HistoryQueriesOptions<Entry, Snapshot, Extra extends object> {
  domain: 'httpHistory' | 'grpcHistory';
  list: (itemId: string, tabId: string) => Promise<Entry[]>;
  get: (id: string) => Promise<Snapshot>;
  remove: (id: string) => Promise<void>;
  clear: (itemId: string, tabId: string) => Promise<void>;
  findTab: (tabId: string) => HistoryTab | null;
  extra?: () => Extra;
}

/** Per-tab UI state — not server state. */
interface HistoryUi {
  viewingId: string | null;
  /** A failed delete or clear; cleared by the next view or successful action. */
  actionError: string | null;
}

function message(err: unknown): string {
  return errorText(err);
}

export function createHistoryQueries<
  Entry extends { id: string },
  Snapshot,
  Extra extends object = Record<string, never>,
>(opts: HistoryQueriesOptions<Entry, Snapshot, Extra>) {
  type Ui = HistoryUi & Extra;
  const { domain } = opts;
  const ui = reactive({} as Record<string, Ui>);

  function listKey(itemId: string, tabId: string) {
    return [domain, itemId, itemId ? '' : tabId] as const;
  }
  function listOptions(itemId: string, tabId: string) {
    return {
      queryKey: listKey(itemId, tabId),
      queryFn: () => opts.list(itemId, tabId),
      // Tab-scope lists go at tab close below, item-scope lists with their item (reconcileTree).
      gcTime: KEEP_WHILE_OPEN,
    };
  }
  function snapshotKey(id: string) {
    return [`${domain}Snapshot`, id] as const;
  }
  function snapshotOptions(id: string) {
    return {
      queryKey: snapshotKey(id),
      queryFn: () => opts.get(id),
    };
  }
  /** Null once the tab is closed. */
  function listKeyFor(tabId: string) {
    const tab = opts.findTab(tabId);
    return tab ? listKey(tab.state.itemId ?? '', tabId) : null;
  }

  function ensure(tabId: string): Ui {
    // Always hand back ui[tabId], never the fresh literal: the reactive proxy tracks writes.
    if (!ui[tabId]) {
      ui[tabId] = {
        viewingId: null,
        actionError: null,
        ...(opts.extra ? opts.extra() : ({} as Extra)),
      };
    }
    return ui[tabId];
  }

  // `tab:` scope lists die with the tab.
  registerTabRuntimeCleanup((tabId) => {
    const viewingId = ui[tabId]?.viewingId;
    delete ui[tabId];
    queryClient.removeQueries({ queryKey: listKey('', tabId), exact: true });
    if (viewingId) queryClient.removeQueries({ queryKey: snapshotKey(viewingId), exact: true });
  });

  /** The tab's history list; enabled only while the History pane shows (P8 D11). */
  function useHistoryList(tab: MaybeRefOrGetter<HistoryTab>) {
    const query = useQuery(() => {
      const t = toValue(tab);
      return {
        ...listOptions(t.state.itemId ?? '', t.id),
        enabled: t.state.responsePane === 'history',
      };
    }, queryClient);
    watch(
      () => {
        const t = toValue(tab);
        return listOptions(t.state.itemId ?? '', t.id);
      },
      (options) => void queryClient.prefetchQuery(options),
      { immediate: true },
    );
    return { query, entries: computed<Entry[]>(() => query.data.value ?? []) };
  }

  /** The stored entry being viewed, null until its snapshot has loaded. */
  function useHistoryViewing(tabId: MaybeRefOrGetter<string>) {
    const viewingId = computed(() => ui[toValue(tabId)]?.viewingId ?? null);
    const query = useQuery(
      () => ({ ...snapshotOptions(viewingId.value ?? ''), enabled: viewingId.value !== null }),
      queryClient,
    );
    const viewing = computed(() =>
      viewingId.value !== null && query.data.value
        ? { id: viewingId.value, snapshot: query.data.value }
        : null,
    );
    return { viewing, viewingId, error: computed(() => query.error.value) };
  }

  /** Selects one entry to view; its snapshot loads through useHistoryViewing. */
  function view(tabId: string, id: string): void {
    if (!opts.findTab(tabId)) return;
    const state = ensure(tabId);
    state.actionError = null;
    state.viewingId = id;
  }

  /** The viewing band's "Back to latest" / "Close" action. */
  function backToLatest(tabId: string): void {
    const state = ui[tabId];
    if (state) state.viewingId = null;
  }

  /** D3: a send/call always asks for *this* response, so it also clears any stored entry being
   *  viewed. Refetches the list only while the pane shows; otherwise the list just turns stale. */
  function noteRecorded(tabId: string): void {
    const key = listKeyFor(tabId);
    if (!key) return;
    ensure(tabId).viewingId = null;
    void refreshApiQuery(key);
  }

  async function del(tabId: string, id: string): Promise<void> {
    try {
      await opts.remove(id);
    } catch (err) {
      if (opts.findTab(tabId)) ensure(tabId).actionError = message(err);
      return;
    }
    const key = listKeyFor(tabId);
    if (!key) return;
    const state = ensure(tabId);
    state.actionError = null;
    if (state.viewingId === id) state.viewingId = null;
    queryClient.removeQueries({ queryKey: snapshotKey(id), exact: true });
    await refreshApiQuery(key);
  }

  /** The destructive, unrecoverable action — the caller gates this behind confirm(). */
  async function clearAll(tabId: string): Promise<void> {
    const tab = opts.findTab(tabId);
    if (!tab) return;
    const itemId = tab.state.itemId ?? '';
    try {
      await opts.clear(itemId, tabId);
    } catch (err) {
      if (opts.findTab(tabId)) ensure(tabId).actionError = message(err);
      return;
    }
    const key = listKeyFor(tabId);
    if (!key) return;
    const state = ensure(tabId);
    state.actionError = null;
    state.viewingId = null;
    for (const entry of queryClient.getQueryData<Entry[]>(key) ?? []) {
      queryClient.removeQueries({ queryKey: snapshotKey(entry.id), exact: true });
    }
    await refreshApiQuery(key);
  }

  return {
    ui,
    ensure,
    listKeyFor,
    useHistoryList,
    useHistoryViewing,
    view,
    backToLatest,
    noteRecorded,
    del,
    clearAll,
  };
}
