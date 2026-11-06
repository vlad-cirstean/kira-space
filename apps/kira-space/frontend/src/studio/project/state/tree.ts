import type {
  ConnectionColor,
  ConnectionStatus,
  ConnectionSummary,
} from '@shared/domain/connection';
import type { SavedFilterQuery } from '@shared/domain/queries';
import { type NodeKind, pathTail, type TreeNode } from '@shared/domain/tree';
import { EMPTY_VISIBILITY, type TreeVisibility } from '@shared/domain/treeFilter';
import { refDebounced } from '@vueuse/core';
import { matchesQuery } from '@workbench/behaviour/match';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import { defineStore } from 'pinia';
import { computed, reactive, shallowReactive, toRefs } from 'vue';
import { control } from '../../bridge/control';
import { useConnectionsStore } from '../../state/connections';
import { useSchemaColumnsStore } from '../../state/schemaColumns';
import { reloadTabsForTarget } from '../../state/viewCommands';
import { isVisible, toSets, type VisibilitySets } from '../filter';
import { isLeafNode, labelForGroup, partitionChildren } from '../grouping';

export interface TreeRowVm {
  key: string;
  depth: number;
  connectionId: string;
  path: string;
  kind: NodeKind | 'connection' | 'group';
  /** Group rows only (P19 D2): the NodeKind this folder collects, for the icon and the empty
   *  check — the row's own `kind` is always the literal `'group'`. */
  groupKind?: NodeKind;
  name: string;
  hasChildren: boolean;
  detail?: string;
  badges?: string[];
  expanded: boolean;
  loading: boolean;
  error?: string;
  matched?: boolean;
  // Connection rows only.
  color?: ConnectionColor;
  status?: ConnectionStatus;
  statusDetail?: string | null;
}

// `database:kira_test/schema:app#function` — a real encoded path can never contain '#'
// (encodeURIComponent('#') is '%23', and every path segment is `${kind}:${name}`), so this
// collides with no node's path and no other group's (P19 D2/realities #8).
function groupPath(parentPath: string, kind: NodeKind): string {
  return `${parentPath}#${kind}`;
}

// The parent path a group's own Refresh/menu actions target — the inverse of groupPath().
export function groupParentPath(path: string): string {
  return path.slice(0, path.lastIndexOf('#'));
}

// Pure (no reactive read/write) — kept at module scope so every non-store caller (schemaColumns.ts,
// filterTree.ts, completion.ts, mongoFieldSample.ts, and both spec files) can import it directly,
// with no store instance needed just to format a key.
export function rowKey(connectionId: string, path: string): string {
  return `${connectionId}|${path}`;
}

interface SearchStats {
  incomplete: boolean;
}

// `sets` (the checkbox filter) and `keyBrowser` (whether database/bucket rows are leaves here) are
// both per-connection facts computed once in searchResult below and threaded through unchanged —
// bundled together rather than as a further positional parameter, since buildRows/buildNodeRow
// already took six/seven of those each.
interface ConnectionView {
  sets: VisibilitySets;
  keyBrowser: boolean;
}

// P99: two concerns lived in this one module — the tree's own browsing/caching state (below) and a
// standalone "Saved filters" dialog's open/connectionId/focusPath (state/schemas.ts's own
// useSchemaDialogStore split is the precedent for this exact shape). Split into two stores.
export const useFiltersDialogStore = defineStore('filtersDialog', () => {
  const filtersDialogState = reactive({
    open: false,
    connectionId: null as string | null,
    /** P28 D20: the row the dialog was invoked from — its ancestors pre-expanded and the row
     *  itself scrolled into view, rather than opening with no relationship to what was clicked. */
    focusPath: null as string | null,
  });

  function openFiltersDialog(connectionId: string, focusPath?: string): void {
    filtersDialogState.open = true;
    filtersDialogState.connectionId = connectionId;
    filtersDialogState.focusPath = focusPath || null;
  }

  function closeFiltersDialog(): void {
    filtersDialogState.open = false;
    filtersDialogState.connectionId = null;
    filtersDialogState.focusPath = null;
  }

  return { ...toRefs(filtersDialogState), openFiltersDialog, closeFiltersDialog };
});

export const useTreeStore = defineStore('tree', () => {
  // key: `${connectionId}|${encodedPath}` — '' encodes the connection's own root.
  //
  // `children` is nested as its own shallowReactive rather than the plain object literal it
  // used to be. treeState as a whole stays a deep reactive() — expanded/loading are Sets mutated in
  // place (.add()/.delete()/.clear()), which only stay tracked under deep reactivity — but the only
  // two writers of `children` (loadChildren below, dropConnectionState) always replace or delete a
  // whole rowKey entry, never mutate a cached TreeNode's own fields. A big schema or a Redis
  // namespace can cache thousands of TreeNode objects (and their badges arrays) here; deep-wrapping
  // every one of them in a reactivity Proxy bought nothing no writer ever used. Vue's reactive()
  // get trap recognizes an already-reactive nested value (via its RAW flag) and returns it as-is
  // instead of re-wrapping it, so nesting a shallowReactive object inside this outer reactive()
  // works without double-proxying.
  const treeState = reactive({
    children: shallowReactive({} as Record<string, TreeNode[]>),
    expanded: new Set<string>(),
    loading: new Set<string>(),
    errors: {} as Record<string, string>,
    visibility: {} as Record<string, TreeVisibility>,
    search: '',
    selected: null as string | null,
    /** Set by revealPath(); ProjectTree.vue watches it to scroll the row into view (Step 7b). */
    pendingScrollKey: null as string | null,
    /** Populated on demand for the tree's "Saved filters ▸" submenu (Step 13). */
    savedQueries: {} as Record<string, SavedFilterQuery[]>,
  });

  function selectRow(key: string): void {
    treeState.selected = key;
  }

  // Fetched right before opening a relation's context menu (never memoised — a saved filter can
  // be added/renamed/deleted between two right-clicks, and this is a fast local IPC round-trip).
  async function loadSavedQueries(connectionId: string, path: string): Promise<void> {
    treeState.savedQueries[rowKey(connectionId, path)] = await control.queriesList(
      connectionId,
      path,
    );
  }

  // Bumped by dropConnectionState — a connection-scoped epoch, not a per-key request token.
  // A load in flight when the connection is dropped
  // (disconnect, delete) must not write its result back afterward: the drop already cleared
  // `children`/`errors` for this connection, and a late write undoes that with nothing left to
  // correct it (a later expand()'s own `if (treeState.children[k]) return` guard would then treat
  // the stale entry as already loaded).
  const connectionEpoch = new Map<string, number>();
  function connectionEpochFor(connectionId: string): number {
    return connectionEpoch.get(connectionId) ?? 0;
  }

  // A per-key request token — overlapping loadChildren calls for the same row (e.g. a Refresh
  // fired while an earlier load for the same path is still in flight) used to have
  // no sequencing at all: whichever response arrived LAST won, not whichever was issued last, and
  // the FIRST call's own `finally` cleared `loading` the moment it settled even if a later call was
  // still running. Bumped on every call for a key; only the call still holding the latest token may
  // write children/errors or clear the spinner.
  const loadToken = new Map<string, number>();
  function nextLoadToken(k: string): number {
    const token = (loadToken.get(k) ?? 0) + 1;
    loadToken.set(k, token);
    return token;
  }
  function isLatestLoadToken(k: string, token: number): boolean {
    return loadToken.get(k) === token;
  }

  async function loadChildren(connectionId: string, path: string, refresh: boolean): Promise<void> {
    const k = rowKey(connectionId, path);
    const epoch = connectionEpochFor(connectionId);
    const token = nextLoadToken(k);
    treeState.loading.add(k);
    delete treeState.errors[k];
    try {
      const result = await control.treeChildren(connectionId, path, refresh);
      if (connectionEpochFor(connectionId) !== epoch || !isLatestLoadToken(k, token)) return;
      // `result.truncated` is deliberately unread here — no level the project tree
      // ever renders can truncate (every SQL/Mongo/Kafka/SQS catalog enumerates a bounded
      // set in one round trip, and Redis/S3 stop expanding at the database/bucket). Only
      // views/browse/state.ts's own levels can, and it's the only place that shows the strip.
      treeState.children[k] = result.nodes;
    } catch (err) {
      if (connectionEpochFor(connectionId) !== epoch || !isLatestLoadToken(k, token)) return;
      treeState.errors[k] = errorText(err);
    } finally {
      if (isLatestLoadToken(k, token)) treeState.loading.delete(k);
    }
  }

  // Single-flight (a plain presence check let two concurrent expand()s for
  // different paths under the same not-yet-loaded connection both issue filtersList) and
  // generation-guarded — saveVisibility bumps `visibilityGeneration` on every explicit save, so a
  // filtersList read already in flight when the user saves new visibility settings discards its
  // now-stale result on resolve instead of overwriting the save.
  const pendingVisibilityLoads = new Map<string, Promise<void>>();
  const visibilityGeneration = new Map<string, number>();
  function visibilityGenerationFor(connectionId: string): number {
    return visibilityGeneration.get(connectionId) ?? 0;
  }

  // Concurrent callers share one flight and all await its result (the Filters dialog seeds its
  // draft from it).
  function loadVisibility(connectionId: string): Promise<void> {
    if (treeState.visibility[connectionId]) return Promise.resolve();
    const pending = pendingVisibilityLoads.get(connectionId);
    if (pending) return pending;
    const generation = visibilityGenerationFor(connectionId);
    const flight = (async () => {
      try {
        const result = await control.filtersList(connectionId);
        if (visibilityGenerationFor(connectionId) !== generation) return;
        treeState.visibility[connectionId] = result;
      } finally {
        pendingVisibilityLoads.delete(connectionId);
      }
    })();
    pendingVisibilityLoads.set(connectionId, flight);
    return flight;
  }

  async function saveVisibility(connectionId: string, visibility: TreeVisibility): Promise<void> {
    visibilityGeneration.set(connectionId, visibilityGenerationFor(connectionId) + 1);
    treeState.visibility[connectionId] = await control.filtersReplace(connectionId, visibility);
  }

  // Skips the round trip when treeState.children[k] is already populated. This is not the
  // "being clever about whether main's L1 cache would be a hit" the comment used to warn against —
  // it decides nothing of the sort. It returns data the renderer already holds and is already
  // rendering. The tree's copy has exactly two real invalidation sources and both still run:
  // onConnectionMetadataInvalidated -> refreshExpanded (`initTreeSync` below, which passes
  // refresh: true and so bypasses this early return) and an explicit context-menu refresh. A
  // collapse never discards treeState.children[k], so re-expanding it was already a pure
  // re-render — this makes that true in code.
  // Bumped by collapse() for the same row key — expand()'s own connect/
  // loadVisibility awaits capture this before either, and re-check it right before
  // `expanded.add(k)`. A collapse that lands while a connect triggered by this same expand() is
  // still pending must stick: without the check, expand() re-added the row to `expanded` (and
  // fetched its children) the instant connect resolved, silently undoing the user's own collapse.
  const collapseSignal = new Map<string, number>();
  function collapseSignalFor(k: string): number {
    return collapseSignal.get(k) ?? 0;
  }

  async function expand(connectionId: string, path: string): Promise<void> {
    const k = rowKey(connectionId, path);
    if (treeState.loading.has(k)) return;
    const intent = collapseSignalFor(k);
    // Expanding a disconnected connection's node connects it first, rather than surfacing
    // E_DISCONNECTED — the twisty is the primary way users browse, so it shouldn't require a
    // separate explicit Connect click first.
    try {
      if (useConnectionsStore().states[connectionId]?.status !== 'connected') {
        treeState.loading.add(k);
        try {
          await useConnectionsStore().connectConnection(connectionId);
        } finally {
          treeState.loading.delete(k);
        }
      }
      if (useConnectionsStore().states[connectionId]?.status !== 'connected') return;
      await loadVisibility(connectionId);
    } catch (err) {
      reportError(connectionId, path, err);
      return;
    }
    if (collapseSignalFor(k) !== intent) return;
    treeState.expanded.add(k);
    if (treeState.children[k]) return;
    await loadChildren(connectionId, path, false);
  }

  // A failed tree action (expand, menu item) shows on its row through the same ErrorPopover a
  // failed children load uses.
  function reportError(connectionId: string, path: string, err: unknown): void {
    treeState.errors[rowKey(connectionId, path)] = errorText(err);
  }

  function collapse(connectionId: string, path: string): void {
    const k = rowKey(connectionId, path);
    collapseSignal.set(k, collapseSignalFor(k) + 1);
    treeState.expanded.delete(k);
  }

  // A group row is a pure view over its parent's already-fetched children — there is no
  // adapter path for it, so toggling flips treeState.expanded directly rather than going through
  // expand()/collapse(), which would connect the connection and issue an IPC call for a synthetic
  // path no adapter has ever heard of.
  function toggleGroup(connectionId: string, path: string): void {
    const k = rowKey(connectionId, path);
    if (treeState.expanded.has(k)) treeState.expanded.delete(k);
    else treeState.expanded.add(k);
  }

  // refresh means "drop this row's whole cached entry, then read again" — a container's
  // Refresh, a connection root's Refresh, and a group's own Refresh (which targets its *parent*
  // path). treeInvalidate drops all four kinds sharing the row (a plain refresh:true
  // only ever replaced 'children', leaving describe/definition/columns for the same path stale);
  // dropSchemaColumns clears the console-facing copy of 'columns'; loadChildren's own
  // refresh:false is now a guaranteed miss, since the row was just dropped.
  // A disconnect that lands during the invalidate await drops the connection's state and bumps the
  // epoch; loadChildren would then capture the new epoch and write children for a dropped
  // connection, so both refreshers bail when it moved.
  async function refresh(connectionId: string, path: string): Promise<void> {
    const epoch = connectionEpochFor(connectionId);
    treeState.expanded.add(rowKey(connectionId, path));
    await control.treeInvalidate(connectionId, path);
    if (connectionEpochFor(connectionId) !== epoch) return;
    useSchemaColumnsStore().dropSchemaColumns(connectionId, path);
    await loadChildren(connectionId, path, false);
  }

  // A connection row's own Refresh, and *Refresh all* (via refreshAllConnections below): drops
  // every row for the whole connection (the `path` omitted), then re-reads exactly the
  // expanded set — the same lazy, per-path re-fetch a reconnect's invalidation push already
  // produces, so this and a reconnect never disagree about what "refreshed" means.
  async function refreshConnection(connectionId: string): Promise<void> {
    const epoch = connectionEpochFor(connectionId);
    await control.treeInvalidate(connectionId);
    if (connectionEpochFor(connectionId) !== epoch) return;
    useSchemaColumnsStore().dropSchemaColumns(connectionId);
    await refreshExpanded(connectionId);
  }

  // The handler: re-fetches every currently-expanded path for the reconnected connection,
  // breadth-first, sequentially — never fans out N parallel engine calls on one client. An
  // expanded group's synthetic '#'-path is skipped: it has no adapter path of its own,
  // and its members are already covered by re-fetching its real parent — issuing a `treeChildren`
  // call for it would decode as a bogus node name and error for nothing.
  //
  // Passes refresh:false, not true — every row this walks is already either freshly
  // invalidated (refreshConnection, just above) or stale under the new connection epoch
  // (tree.Service's own freshness rule, reached via the reconnect push), so refresh:false already
  // reaches the server; refresh:true on top would force a re-fetch of a path a *sibling window* may
  // have already refreshed under the same epoch, which the epoch rule says is unnecessary.
  async function refreshExpanded(connectionId: string): Promise<void> {
    const prefix = `${connectionId}|`;
    const paths = [...treeState.expanded]
      .filter((k) => k.startsWith(prefix))
      .map((k) => k.slice(prefix.length))
      .filter((path) => !path.includes('#'))
      .sort((a, b) => a.split('/').length - b.split('/').length);
    for (const path of paths) {
      await loadChildren(connectionId, path, false);
    }
  }

  // A leaf's own Refresh (a relation, a Mongo collection): drops the object's whole row
  // (describe/definition/the junk `children` a leaf never has) and reloads any open tab on it,
  // rather than calling Children on a path every adapter answers with an empty list. No
  // tree call for the leaf itself — the next data/definition tab open re-runs describe/definition,
  // and no op-log row is written for work that would only ever return [].
  async function refreshObject(connectionId: string, path: string): Promise<void> {
    await control.treeInvalidate(connectionId, path);
    reloadTabsForTarget(connectionId, path, '');
  }

  function collapseAll(): void {
    treeState.expanded.clear();
  }

  // §8.10's "Reveal in project panel" (Step 7b): expands every ancestor in order — sequentially,
  // the same discipline refreshExpanded already uses — then selects the row and asks the tree
  // view to scroll it into view.
  //
  // P41 D9: a keyBrowser connection's database/bucket is a leaf here (F20) — nothing beneath it has
  // a tree row to expand into or scroll to, so a path reaching into one truncates at that container:
  // the deepest ancestor the tree actually renders is selected/scrolled instead of a row that will
  // never exist. "Reveal in project panel" names the panel; revealing the container an item lives in
  // is the honest answer once the panel stops holding the item itself.
  async function revealPath(connectionId: string, path: string): Promise<void> {
    if (!treeState.expanded.has(rowKey(connectionId, ''))) {
      await expand(connectionId, '');
    }
    const keyBrowser = useConnectionsStore().states[connectionId]?.caps?.keyBrowser === true;
    const segments = path.split('/').filter(Boolean);
    let ancestor = '';
    for (let i = 0; i < segments.length - 1; i++) {
      ancestor = ancestor ? `${ancestor}/${segments[i]}` : segments[i];
      const kind = pathTail(ancestor)?.kind;
      if (keyBrowser && (kind === 'database' || kind === 'bucket')) {
        const key = rowKey(connectionId, ancestor);
        treeState.selected = key;
        treeState.pendingScrollKey = key;
        return;
      }
      if (!treeState.expanded.has(rowKey(connectionId, ancestor))) {
        await expand(connectionId, ancestor);
      }
    }
    // A Postgres partition's path sits beside its parent, not under it, so its row may not render.
    const lastAncestor = rowKey(connectionId, ancestor);
    const rendered = treeState.children[lastAncestor]?.some((n) => n.path === path);
    const key = rendered === false ? lastAncestor : rowKey(connectionId, path);
    treeState.selected = key;
    treeState.pendingScrollKey = key;
  }

  async function refreshAllConnections(): Promise<void> {
    for (const conn of useConnectionsStore().records) {
      await refreshConnection(conn.id);
    }
  }

  // Purges every collection this module keys by connection identity. Driven off
  // onConnectionsChanged (below) rather than off deleteConnection() directly, so every deletion
  // path is covered — the context menu, a direct IPC call, a future bulk delete — the same
  // reasoning state/connections.ts:44-50 records for that channel.
  function dropConnectionState(connectionId: string): void {
    connectionEpoch.set(connectionId, connectionEpochFor(connectionId) + 1);
    const prefix = `${connectionId}|`;
    for (const k of Object.keys(treeState.children)) {
      if (k.startsWith(prefix)) delete treeState.children[k];
    }
    for (const k of [...treeState.expanded]) {
      if (k.startsWith(prefix)) treeState.expanded.delete(k);
    }
    for (const k of [...treeState.loading]) {
      if (k.startsWith(prefix)) treeState.loading.delete(k);
    }
    for (const k of Object.keys(treeState.errors)) {
      if (k.startsWith(prefix)) delete treeState.errors[k];
    }
    delete treeState.visibility[connectionId];
    for (const k of Object.keys(treeState.savedQueries)) {
      if (k.startsWith(prefix)) delete treeState.savedQueries[k];
    }
    if (treeState.selected?.startsWith(prefix)) treeState.selected = null;
    if (treeState.pendingScrollKey?.startsWith(prefix)) treeState.pendingScrollKey = null;
  }

  // Every connection id this module currently holds state for, gathered from the six collections'
  // own keys — `visibility` is keyed directly by connectionId, the rest by rowKey (D6). Exported
  // only for the Playwright-only `window.__kiraTreeConnectionIds` hook (main.ts, D6's leak spec).
  function knownConnectionIds(): Set<string> {
    const ids = new Set<string>();
    const addFromKey = (k: string): void => {
      const i = k.indexOf('|');
      ids.add(i >= 0 ? k.slice(0, i) : k);
    };
    for (const k of Object.keys(treeState.children)) addFromKey(k);
    for (const k of treeState.expanded) addFromKey(k);
    for (const k of treeState.loading) addFromKey(k);
    for (const k of Object.keys(treeState.errors)) addFromKey(k);
    for (const k of Object.keys(treeState.visibility)) addFromKey(k);
    for (const k of Object.keys(treeState.savedQueries)) addFromKey(k);
    return ids;
  }

  let unsubscribeInvalidated: (() => void) | null = null;
  let unsubscribeConnectionsChanged: (() => void) | null = null;
  let unsubscribeConnectionState: (() => void) | null = null;
  let unsubscribeFilters: (() => void) | null = null;

  function initTreeSync(): void {
    unsubscribeInvalidated?.();
    unsubscribeInvalidated = control.onConnectionMetadataInvalidated((connectionId) => {
      void refreshExpanded(connectionId);
    });

    // A filters edit in the connection's file: take the new visibility and reload what is open.
    unsubscribeFilters?.();
    unsubscribeFilters = control.onConnectionFiltersChanged(({ connectionId, visibility }) => {
      visibilityGeneration.set(connectionId, visibilityGenerationFor(connectionId) + 1);
      treeState.visibility[connectionId] = visibility;
      void refreshExpanded(connectionId);
    });

    unsubscribeConnectionsChanged?.();
    unsubscribeConnectionsChanged = control.onConnectionsChanged((records) => {
      const liveIds = new Set(records.map((r) => r.id));
      for (const id of knownConnectionIds()) {
        if (!liveIds.has(id)) dropConnectionState(id);
      }
    });

    // A disconnected connection's tree copy is neither authoritative nor reused — L1
    // metadata is persisted server-side and refreshed on every reconnect regardless
    // (docs/ARCHITECTURE.md's Caching section) — so it is released the same way a deleted
    // connection's is (dropConnectionState, unchanged), just on a different signal. Reached through
    // control's own onConnectionState broadcast rather than by reaching into
    // state/connections.ts's disconnectConnection() directly (:249-251's own reasoning for why
    // dropConnectionState is driven off a canonical push, not a specific action function): this way
    // covers every path that can produce the transition — an explicit Disconnect click, main
    // reporting a lost connection — not just the renderer's own button. `connecting`/`error` are
    // deliberately left alone: a reconnect attempt in flight, or one that failed and might be
    // retried, is not "the user disconnected" — dropping the tree there would collapse/re-fetch a
    // still-expanded tree out from under a transient state.
    unsubscribeConnectionState?.();
    unsubscribeConnectionState = control.onConnectionState((state) => {
      if (state.status === 'disconnected') dropConnectionState(state.connectionId);
    });
  }

  // One TreeNode's own row, plus its expanded subtree — shared by ungrouped nodes and group
  // members alike (P19 D2: a group's members go through exactly this, one depth deeper, never a
  // second grouping pass). Returns whether this node or anything under it matched the search query.
  function buildNodeRow(
    connectionId: string,
    node: TreeNode,
    depth: number,
    view: ConnectionView,
    query: string,
    out: TreeRowVm[],
    stats: SearchStats,
  ): boolean {
    const k = rowKey(connectionId, node.path);
    const selfMatches = query ? matchesQuery(node.name, query) : false;
    // P19 D5: a view/matview is a leaf regardless of what a cached node's own hasChildren
    // says — its columns moved into the definition view, but an L1 payload cached before this
    // phase can still carry `hasChildren: true` until the connection's next reconnect. A table
    // expands only when hasChildren says so (P328 partitions). P41 adds
    // database/bucket for a keyBrowser connection, same reasoning (F12).
    const leaf = isLeafNode(node, view.keyBrowser);
    const childNodes = leaf ? undefined : treeState.children[k];
    const naturallyExpanded = !leaf && treeState.expanded.has(k);

    if (query && node.hasChildren && !leaf && !childNodes) stats.incomplete = true;

    const childOut: TreeRowVm[] = [];
    let descendantMatch = false;
    // Search looks inside anything already cached, regardless of expansion state (§8c:
    // "searching cached nodes only" — it never triggers a fetch).
    if (childNodes && (query ? true : naturallyExpanded)) {
      descendantMatch = buildRows(
        connectionId,
        node.path,
        childNodes,
        depth + 1,
        view,
        query,
        childOut,
        stats,
      );
    }

    if (query && !selfMatches && !descendantMatch) return false;

    const rowExpanded = query ? naturallyExpanded || descendantMatch : naturallyExpanded;
    out.push({
      key: k,
      depth,
      connectionId,
      path: node.path,
      kind: node.kind,
      name: node.name,
      hasChildren: leaf ? false : node.hasChildren,
      detail: node.detail,
      badges: node.badges,
      expanded: rowExpanded,
      loading: treeState.loading.has(k),
      error: treeState.errors[k],
      matched: query ? selfMatches : undefined,
    });
    // Array.prototype.push(...arr) spreads into the argument list — engines cap
    // that in the ~65k-125k range and throw RangeError past it. searchResult accumulates across
    // every connection and expanded level, so this is a hard failure on a large enough expansion,
    // not a slow path.
    if (rowExpanded) for (const r of childOut) out.push(r);
    return selfMatches || descendantMatch;
  }

  // Split out of buildRows (nesting one level deeper inside the store body pushed biome's
  // cognitive-complexity score over the ceiling with zero logic change) — one group folder's
  // own row plus its members, appended to `out` when it (or a member) matches. Returns whether this
  // group matched, the same contract buildRows' own inlined loop body had.
  function buildGroupRow(
    connectionId: string,
    parentPath: string,
    group: { kind: NodeKind; nodes: TreeNode[] },
    depth: number,
    view: ConnectionView,
    query: string,
    out: TreeRowVm[],
    stats: SearchStats,
  ): boolean {
    const path = groupPath(parentPath, group.kind);
    const k = rowKey(connectionId, path);
    const childOut: TreeRowVm[] = [];
    let anyChildMatch = false;
    for (const member of group.nodes) {
      if (buildNodeRow(connectionId, member, depth + 1, view, query, childOut, stats)) {
        anyChildMatch = true;
      }
    }
    if (query && !anyChildMatch) return false;

    const connectionKind = useConnectionsStore().connectionRecord(connectionId)?.kind;
    const naturallyExpanded = treeState.expanded.has(k);
    const rowExpanded = query ? naturallyExpanded || anyChildMatch : naturallyExpanded;
    out.push({
      key: k,
      depth,
      connectionId,
      path,
      kind: 'group',
      groupKind: group.kind,
      name: connectionKind ? labelForGroup(group.kind, connectionKind) : group.kind,
      hasChildren: true,
      expanded: rowExpanded,
      loading: false,
    });
    if (rowExpanded) for (const r of childOut) out.push(r);
    return anyChildMatch;
  }

  function buildRows(
    connectionId: string,
    parentPath: string,
    nodes: TreeNode[],
    depth: number,
    view: ConnectionView,
    query: string,
    out: TreeRowVm[],
    stats: SearchStats,
  ): boolean {
    let anyMatch = false;
    const visible = nodes.filter((node) => isVisible(node, view.sets));
    const { ungrouped, groups } = partitionChildren(visible);

    for (const node of ungrouped) {
      if (buildNodeRow(connectionId, node, depth, view, query, out, stats)) anyMatch = true;
    }

    for (const group of groups) {
      if (buildGroupRow(connectionId, parentPath, group, depth, view, query, out, stats)) {
        anyMatch = true;
      }
    }
    return anyMatch;
  }

  // P2 R1: buildRows/buildNodeRow recurse over every currently-cached node for every connection —
  // for a large expanded schema or key-browser namespace, that's a full traversal of thousands of
  // nodes on every single keystroke SearchBox.vue/ProjectPanel.vue write into treeState.search
  // directly (so the input itself stays instantly responsive). debouncedQuery only updates
  // TIMING.search after typing settles, and searchResult (plus TreeRow.vue's own highlight,
  // via activeSearchQuery below) reads that instead, so a fast typist causes one recompute per pause
  // rather than one per character.
  // P99 §9.3: refDebounced replaces the watch+setTimeout pair this used to hand-roll — the
  // trim/lowercase step moves to activeSearchQuery below, reactively, off the debounced value.
  const debouncedQuery = refDebounced(
    computed(() => treeState.search),
    TIMING.search,
  );

  // TreeRow.vue's highlightParts() needs the exact query that produced row.matched, not the live
  // (undebounced) treeState.search — otherwise the highlighted substring can briefly disagree with
  // why a row is even showing as matched.
  const activeSearchQuery = computed(() => debouncedQuery.value.trim().toLowerCase());

  // One connection's own row plus its (possibly search-filtered) expanded children — every reactive
  // read inside it (useConnectionsStore().states,
  // treeState.expanded/children/visibility/loading/errors) still runs inside searchResult's
  // dependency tracking, because searchResult calls this SYNCHRONOUSLY, from inside its own
  // computed callback (below) — never hoisted out or memoized separately. `null` means this
  // connection is filtered out entirely (a search query matched neither its own name nor anything
  // under it).
  function connectionRow(
    conn: ConnectionSummary,
    query: string,
    stats: SearchStats,
  ): { row: TreeRowVm; children: readonly TreeRowVm[] } | null {
    const connKey = rowKey(conn.id, '');
    const state = useConnectionsStore().states[conn.id];
    const naturallyExpanded = treeState.expanded.has(connKey);
    const childNodes = treeState.children[connKey];
    const view: ConnectionView = {
      sets: toSets(treeState.visibility[conn.id] ?? EMPTY_VISIBILITY),
      keyBrowser: state?.caps?.keyBrowser === true,
    };

    if (query && !childNodes) stats.incomplete = true;

    const childOut: TreeRowVm[] = [];
    let descendantMatch = false;
    if (childNodes && (query ? true : naturallyExpanded)) {
      descendantMatch = buildRows(conn.id, '', childNodes, 1, view, query, childOut, stats);
    }

    const selfMatches = query ? matchesQuery(conn.name, query) : false;
    if (query && !selfMatches && !descendantMatch) return null;

    const rowExpanded = query ? naturallyExpanded || descendantMatch : naturallyExpanded;
    const status = state?.status ?? 'disconnected';
    const row: TreeRowVm = {
      key: connKey,
      depth: 0,
      connectionId: conn.id,
      path: '',
      kind: 'connection',
      name: conn.name,
      hasChildren: true,
      expanded: rowExpanded,
      loading: treeState.loading.has(connKey),
      error: treeState.errors[connKey],
      matched: query ? selfMatches : undefined,
      color: conn.color,
      status,
      statusDetail: status === 'error' ? (state?.error ?? null) : (state?.serverVersion ?? null),
    };
    return { row, children: rowExpanded ? childOut : [] };
  }

  const searchResult = computed(() => {
    const rows: TreeRowVm[] = [];
    const stats: SearchStats = { incomplete: false };
    const query = activeSearchQuery.value;

    for (const conn of useConnectionsStore().records) {
      const result = connectionRow(conn, query, stats);
      if (!result) continue;
      rows.push(result.row);
      for (const r of result.children) rows.push(r);
    }

    return { rows, incomplete: query ? stats.incomplete : false };
  });

  const visibleRows = computed<TreeRowVm[]>(() => searchResult.value.rows);
  const searchIncomplete = computed<boolean>(() => searchResult.value.incomplete);

  return {
    ...toRefs(treeState),
    selectRow,
    loadSavedQueries,
    loadVisibility,
    saveVisibility,
    reportError,
    expand,
    collapse,
    toggleGroup,
    refresh,
    refreshConnection,
    refreshObject,
    collapseAll,
    revealPath,
    refreshAllConnections,
    knownConnectionIds,
    initTreeSync,
    activeSearchQuery,
    visibleRows,
    searchIncomplete,
  };
});
