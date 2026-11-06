import type { PageSize } from '@shared/domain/tabs';
import { pathParent, type TreeNode } from '@shared/domain/tree';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { markRaw } from 'vue';
import { control } from '../../bridge/control';
import { pinia } from '../../state/pinia';
import { useStaleDataStore } from '../../state/staleData';
import { useTabsStore } from '../../state/tabs';
import { registerBrowseInvalidate, registerTabReload } from '../../state/viewCommands';
import { classifyLoadError, createRuntimeStore } from '../shared/viewOp';

// P41 D12: one level at a time, loaded by a single `treeChildren` call — never a second tree.
// `nodes` holds exactly what `children(level)` returned; there is nothing to page (D16 — this is
// not a cancellable engine op, `kira:tree:children` never has been).
interface BrowseViewRuntime {
  status: 'idle' | 'loading' | 'error';
  error: { code: string; message: string } | null;
  /** P43 F6/D7: the last *action* (a key/object delete from this level) that failed, verbatim
   *  from the server — sibling to `error` (a failed *load*), never a reuse of it. Cleared by the
   *  next successful action or load. */
  actionError: string | null;
  nodes: TreeNode[];
  /** P43 iter2 F16/D23: this level's own listing stopped at the adapter's own round budget —
   *  BrowseView.vue's own strip is the only surface that renders these levels (P41 D5 cut the
   *  tree at the container). Refresh is the only affordance that can try again. */
  truncated: boolean;
  /** Substring filter over `nodes` (D18), runtime-only — mirrors grid/state.ts's `searchOpen`. */
  filter: string;
  /** Filter bar visibility; kept here so a tab switch does not hide a still-active filter. */
  filterOpen: boolean;
  /** The row that holds the list's roving tab stop, by path. */
  selected: string | null;
  /** Monotonic per tab. `load()` captures it before its own await and drops its
   *  result on all three exit paths if a newer load has started since — the same supersession
   *  guard grid/documents/keyvalue/stream all keep as `opId`, expressed as a counter because
   *  `kira:tree:children` is not a cancellable engine op and has no op id to compare. */
  loadSeq: number;
  /** The split's value pane — the selected leaf's own pager position. Backs the
   *  `${tabId}::preview` KeyValueHost (BrowseView.vue registers it), so KeyValuePane's pager has
   *  somewhere to read/write for a target that is not a real tab. Runtime-only, like `selected`
   *  itself — a selection this belongs to never survives a restore either. Reset to page 0
   *  on every new selection (`selectRow` below); `previewPageSize` persists across selections,
   *  mirroring how a real KeyValue tab's own page size survives paging through different pages. */
  previewPageIndex: number;
  previewPageSize: PageSize;
  /** Redis key path -> its TYPE, filled in windowed (BrowseView.vue's own
   *  `@visible-range` handler calls ensureKeyTypes below with the paths currently on screen) —
   *  never the whole level, which can hold up to 200 000 keys (redis/catalog.go's own scanCount x
   *  maxScanRounds). `markRaw`'d for the same reason `nodes` is: a Map bounded by what has been
   *  scrolled past is far smaller than a whole level, but nothing here mutates in a way a template
   *  needs cell-by-cell reactivity for either, so `keyTypesVersion` below (bumped whenever new
   *  entries land) is what a template observes. Cleared on every level change (setLevel),
   *  same reason `selected`/`filter` are: a path from the previous level must never colour
   *  a row in this one. */
  keyTypes: Map<string, string>;
  keyTypesVersion: number;
}

function defaultRuntime(): BrowseViewRuntime {
  return {
    status: 'idle',
    error: null,
    actionError: null,
    nodes: [],
    truncated: false,
    filter: '',
    filterOpen: false,
    selected: null,
    loadSeq: 0,
    previewPageIndex: 0,
    previewPageSize: 100,
    keyTypes: markRaw(new Map()),
    keyTypesVersion: 0,
  };
}

export const useBrowseViewStore = defineStore('browseView', () => {
  const { runtime, ensureRuntime, setActionError } =
    createRuntimeStore<BrowseViewRuntime>(defaultRuntime);

  registerTabRuntimeCleanup((tabId) => {
    delete runtime[tabId];
    delete pendingKeyTypePaths[tabId];
  });

  /** The level a tab is currently showing — `''` in session state means "the tab's own container
   *  path" (D14), so a freshly opened tab and one restored at its root agree. */
  function currentLevel(tabId: string): string | null {
    const tab = useTabsStore().findBrowseTab(tabId);
    if (!tab?.connectionId) return null;
    return tab.state.levelPath === '' ? tab.path : tab.state.levelPath;
  }

  async function load(tabId: string, opts?: { refresh?: boolean }): Promise<void> {
    const tab = useTabsStore().findBrowseTab(tabId);
    if (!tab?.connectionId) return;
    const level = currentLevel(tabId);
    if (level === null) return;
    const rt = ensureRuntime(tabId);
    const seq = ++rt.loadSeq;
    rt.status = 'loading';
    rt.error = null;
    rt.actionError = null;
    rt.truncated = false;
    try {
      const result = await control.treeChildren(tab.connectionId, level, opts?.refresh ?? false);
      if (rt.loadSeq !== seq) return; // superseded by a newer load
      // `runtime` (viewOp.ts's createRuntimeStore) is a deep
      // reactive() — necessary for the plain scalar fields every other view's runtime keeps, but a
      // Redis/S3 level can hold up to 200 000 TreeNodes (redis/catalog.go's own scanCount x
      // maxScanRounds), and assigning a plain array here would wrap every one of them (and their
      // own `badges` array) in its own reactivity Proxy the moment filteredNodes reads them — for
      // state nothing here ever mutates in place (`nodes` is always replaced wholesale, never
      // pushed into). markRaw is project/state/tree.ts's own precedent for exactly this shape
      // (`children`'s own comment: "deep-wrapping every one of them in a reactivity Proxy bought
      // nothing no writer ever used").
      rt.nodes = markRaw(result.nodes);
      rt.truncated = result.truncated;
      useStaleDataStore().stamp(tabId, tab.connectionId);
      rt.status = 'idle';
    } catch (err) {
      if (rt.loadSeq !== seq) return; // superseded by a newer load
      const failure = classifyLoadError(err);
      if (failure.kind === 'disconnected') {
        rt.status = 'idle';
        useTabsStore().unmarkHydrated(tabId);
        return;
      }
      rt.status = 'error';
      rt.error = { code: failure.code, message: failure.message };
      // levelPath has already advanced (setLevel patches it
      // before calling load) — leaving the *previous* level's nodes in place here attributes a
      // listing to the wrong level under the new breadcrumb, and a row action (Delete, say) run
      // against it would target a node under the old level while the header says otherwise.
      rt.nodes = [];
    }
  }

  async function reload(tabId: string): Promise<void> {
    await load(tabId, { refresh: true });
  }

  // Normalizes `level` back to `''` when it equals the tab's own container path, so a tab that
  // descends and returns to its root looks identical (in session state) to one that never left it.
  async function setLevel(tabId: string, level: string): Promise<void> {
    const tab = useTabsStore().findBrowseTab(tabId);
    if (!tab) return;
    // A level change used to keep the previous level's own
    // `filter`/`selected` — descending into a container while a substring filter was active carried
    // that now-meaningless filter into the new level's listing (silently hiding most or all of its
    // children, with the filter box's leftover text the only clue), and `selected` could go on
    // pointing at a path that belongs to a different level entirely.
    const rt = ensureRuntime(tabId);
    rt.filter = '';
    rt.selected = null;
    rt.previewPageIndex = 0;
    rt.keyTypes.clear();
    rt.keyTypesVersion++;
    pendingKeyTypePaths[tabId]?.clear();
    useTabsStore().patchBrowseTabState(tabId, { levelPath: level === tab.path ? '' : level });
    await load(tabId);
  }

  /** A container row's own path — one level deeper. */
  async function descend(tabId: string, path: string): Promise<void> {
    await setLevel(tabId, path);
  }

  /** Up — one level shallower. A no-op at the tab's own container (nothing shallower to show). */
  async function ascend(tabId: string): Promise<void> {
    const tab = useTabsStore().findBrowseTab(tabId);
    const level = currentLevel(tabId);
    if (!tab || level === null || level === tab.path) return;
    const parent = pathParent(level);
    if (parent === null) return;
    await setLevel(tabId, parent);
  }

  /** The breadcrumb's own jump — to any ancestor level, not just the immediate parent. */
  async function goToLevel(tabId: string, path: string): Promise<void> {
    await setLevel(tabId, path);
  }

  function setFilter(tabId: string, filter: string): void {
    const rt = runtime[tabId];
    if (rt) rt.filter = filter;
  }

  function setFilterOpen(tabId: string, open: boolean): void {
    ensureRuntime(tabId).filterOpen = open;
  }

  function selectRow(tabId: string, path: string | null): void {
    const rt = runtime[tabId];
    if (!rt) return;
    // P63: a new selection previews a different value — its pager starts at page 0, same as
    // opening a fresh KeyValue tab would (defaultKeyValueTabState's own `pageIndex: 0`).
    if (rt.selected !== path) rt.previewPageIndex = 0;
    rt.selected = path;
  }

  // P63 §4.3: paths a treeKeyTypes call is already in flight for, per tab — module-level (not
  // runtime state) since it is purely a dedupe guard, never rendered. Cleared alongside `runtime`
  // itself (registerTabRuntimeCleanup above) and on every level change (setLevel above).
  const pendingKeyTypePaths: Record<string, Set<string>> = {};

  // Well under any viewport (§4.3's own guard) — a defensive cap on ensureKeyTypes itself, not
  // trusted to the caller, so a future caller that hands in more than one screen's worth of paths
  // still can't turn this into a whole-level fetch.
  const KEY_TYPES_BATCH_LIMIT = 200;

  // Fetches redis TYPE for `paths` not already known or already in flight, batched and
  // windowed by the caller (BrowseView.vue's own `@visible-range` handler) rather than eagerly for
  // a whole level. Dedupes against `rt.keyTypes` (already answered) and this tab's own in-flight
  // set (a request already running) — interacting rules over an in-flight set plus a loadSeq guard,
  // the "cache invalidation with interacting rules" case CLAUDE.md's own testing bar names,
  // covered by state.spec.ts's own dedupe/supersede test.
  function ensureKeyTypes(tabId: string, paths: readonly string[]): void {
    const rt = runtime[tabId];
    if (!rt) return;
    if (!pendingKeyTypePaths[tabId]) pendingKeyTypePaths[tabId] = new Set();
    const pending = pendingKeyTypePaths[tabId];
    const need: string[] = [];
    for (const p of paths) {
      if (rt.keyTypes.has(p) || pending.has(p)) continue;
      need.push(p);
    }
    if (need.length === 0) return;
    const batch = need.slice(0, KEY_TYPES_BATCH_LIMIT);
    for (const p of batch) pending.add(p);
    // Tagged with the level's own loadSeq (D39's existing supersession counter, reused rather than
    // duplicated) — a level change bumps it via load(), so a batch that resolves after the user has
    // already navigated away writes nothing rather than colouring the wrong level's rows.
    void loadKeyTypes(tabId, batch, rt.loadSeq, pending);
  }

  async function loadKeyTypes(
    tabId: string,
    batch: string[],
    seq: number,
    pending: Set<string>,
  ): Promise<void> {
    const tab = useTabsStore().findBrowseTab(tabId);
    if (!tab?.connectionId) {
      for (const p of batch) pending.delete(p);
      return;
    }
    try {
      const types = await control.treeKeyTypes(tab.connectionId, batch);
      for (const p of batch) pending.delete(p);
      const rt = runtime[tabId];
      if (!rt || rt.loadSeq !== seq) return; // superseded by a newer level load
      let wrote = false;
      for (let i = 0; i < batch.length; i++) {
        const t = types[i];
        const p = batch[i];
        if (t === undefined || p === undefined) continue;
        rt.keyTypes.set(p, t);
        wrote = true;
      }
      if (wrote) rt.keyTypesVersion++;
    } catch {
      // §4.3's own rule: a failed batch is silent — rows keep the generic glyph, since a decorative
      // badge must never raise the error strip a failed *listing* owns.
      for (const p of batch) pending.delete(p);
    }
  }

  // P41 D14: an S3 upload/delete lands in a level the project tree no longer renders (F22) — this
  // is how UploadObjectDialog.vue (and any future Browse-panel mutation) tells a live Browse tab
  // its own currently-shown level may be stale, without project/ importing views/ directly.
  async function invalidateLevel(connectionId: string, path: string): Promise<void> {
    for (const tabId of Object.keys(runtime)) {
      const tab = useTabsStore().findBrowseTab(tabId);
      if (!tab || tab.connectionId !== connectionId) continue;
      if (currentLevel(tabId) !== path) continue;
      await load(tabId, { refresh: true });
    }
  }

  /** Shown level is stale: connection gone, or listed under another connection session. */
  function isStale(tabId: string): boolean {
    const connectionId = useTabsStore().findBrowseTab(tabId)?.connectionId;
    return connectionId ? useStaleDataStore().isStale(tabId, tabId, connectionId) : false;
  }

  return {
    runtime,
    isStale,
    load,
    reload,
    descend,
    ascend,
    goToLevel,
    setFilter,
    setFilterOpen,
    selectRow,
    ensureKeyTypes,
    invalidateLevel,
    /** P43 F6/D7: written by browse/menu.ts's own catch around a row's Delete item. */
    setActionError,
  };
});

registerTabReload('browse', (tabId) => useBrowseViewStore(pinia).reload(tabId));
registerBrowseInvalidate((connectionId, path) =>
  useBrowseViewStore(pinia).invalidateLevel(connectionId, path),
);
