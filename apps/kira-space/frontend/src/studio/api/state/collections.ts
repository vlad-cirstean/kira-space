import type {
  CollectionItemKind,
  CollectionItemProtocol,
  CollectionItemSummary,
  CollectionSummary,
  GrpcSavedRequest,
  HttpSavedRequest,
  ImportResult,
} from '@shared/domain/collections';
import { defaultGrpcSavedRequest, defaultHttpSavedRequest } from '@shared/domain/collections';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { t } from '@theme/i18n';
import { refDebounced } from '@vueuse/core';
import { matchesQuery } from '@workbench/behaviour/match';
import { usePersistedState } from '@workbench/behaviour/persisted';
import { TIMING } from '@workbench/behaviour/timing';
import { errorText } from '@workbench/bridge/codedError';
import { logWarn } from '@workbench/bridge/log';
import { queryClient } from '@workbench/state/queryClient';
import { defineStore } from 'pinia';
import { computed, reactive, toRef, toRefs, watch } from 'vue';
import { control } from '../../bridge/control';
import { type Placement, topmost } from '../collectionDrop';
import {
  closeVariableSetTabsForOwner,
  openCollectionGrpcRequestTab,
  openCollectionRequestTab,
  patchGrpcRequestTabState,
  patchHttpRequestTabState,
  remapRequestTabItems,
  syncRequestTabNames,
  syncVariableSetTabNames,
} from '../tabs';
import {
  type ApiCollectionsTree,
  apiCollectionsTreeKey,
  apiCollectionsTreeQueryOptions,
  apiSavedGrpcRequestKey,
  apiSavedRequestKey,
  loadSavedGrpcRequest,
  loadSavedRequest,
  reconcileTree,
  refreshApiQuery,
} from './apiQueries';
import { collectionKey, itemKey } from './collectionKeys';
import { collectionStepOrder, itemStepBefore } from './collectionOrder';
import { useCollectionSelectionStore } from './collectionSelection';
import { useSaveRequestDialogStore } from './saveRequestDialog';

// Api's own tree store. Studio's tree is lazy because its data is remote — expanding a
// node connects a connection and issues an IPC call, which is what its children cache, loading
// set, 150 ms search debounce and "searching cached nodes only" note all exist for. A
// collections tree has none of that: the whole tree is rows in a local SQLite table, listable in
// one call, so `visibleRows` below is a **pure computed** over one array. That is a genuine
// simplification rather than a shape to copy.

/** The row TreeHost renders. Four structural members (key/depth/hasChildren/expanded — TreeHost's
 *  own StickyRowLike contract) plus seven of its own, against TreeRowVm's fourteen: connectionId,
 *  color, status, statusDetail, groupKind, badges, loading and error have no meaning here. The
 *  mechanics generalize; the rows do not. */
export interface CollectionRowVm {
  key: string;
  depth: number;
  hasChildren: boolean;
  expanded: boolean;
  kind: 'collection' | 'folder' | 'request';
  id: string;
  collectionId: string;
  parentId: string | null;
  name: string;
  /** Requests only — the row's leading chip. */
  method: string;
  /** Requests only — searched, never shown. */
  url: string;
  /** P11 D12: 'http' for every existing row (the column's own SQL default); meaningless for a
   *  folder or a collection row. */
  protocol: CollectionItemProtocol;
  matched: boolean;
}

function rowMatches(name: string, url: string, query: string): boolean {
  if (!query) return false;
  return matchesQuery(name, query) || matchesQuery(url, query);
}

// A saved request's dirty-comparison cache and its orphan status both live in the
// apiSavedRequest(Grpc)Key query now (null data === confirmed orphan, undefined === not loaded yet,
// apiQueries.ts's own convention), not a second local Record.
interface CollectionsState {
  search: string;
  /** The row key whose label is currently an inline rename input, if any. */
  renamingKey: string | null;
  busy: boolean;
  importResult: ImportResult | null;
  /** D16: "N secret values were not written to the file" — set after an export that stripped at
   *  least one, shown alongside the import report strip. */
  exportWarning: string | null;
  /** A tree mutation's (create/rename/delete/duplicate/save/import/export) own failure
   *  message — every one of them used to let this throw uncaught from a fire-and-forget `void`
   *  call in CollectionsPanel.vue/CollectionsTree.vue, so a failed write left the row exactly as it
   *  was with nothing telling the user why. Cleared on the next attempt that succeeds. */
  error: string | null;
}

// ---- saving a request into a collection ----
//
// The dialog's own open/tabId/suggestedName/payload UI state lives in useSaveRequestDialogStore
// (one-store-one-concern). submitSaveDialog stays here: it reaches deep into the
// tree's own load/reveal/cache machinery below, so a full split would only trade one store for
// two stores calling each other for everything that matters.

export const useCollectionsStore = defineStore('collections', () => {
  const selection = useCollectionSelectionStore();
  const state = reactive<CollectionsState>({
    search: '',
    renamingKey: null,
    busy: false,
    importResult: null,
    exportWarning: null,
    error: null,
  });

  // Expanded keys survive a reload (the whole tree is in memory, so restoring is just a key set).
  const expandedKeys = usePersistedState<string[]>('kira.api.expanded', []);
  const expandedSet = {
    has: (key: string): boolean => expandedKeys.value.includes(key),
    add(key: string): void {
      if (!expandedKeys.value.includes(key)) expandedKeys.value = [...expandedKeys.value, key];
    },
    delete(key: string): void {
      expandedKeys.value = expandedKeys.value.filter((k) => k !== key);
    },
  };

  function dismissError(): void {
    state.error = null;
  }

  // The tree is now TanStack Query's cache, not a store field — this observer is
  // app-lifetime (created once, with the setup store's own effect scope owning it), so
  // invalidateQueries always has an active observer to refetch. queryClient is passed explicitly
  // (not injected): main.ts creates stores before app.use(VueQueryPlugin), and unit tests mount no
  // app at all — useBaseQuery.js falls back to inject() only when no explicit client is given.
  const treeQuery = useQuery(apiCollectionsTreeQueryOptions(), queryClient);

  const collections = computed<CollectionSummary[]>(() => treeQuery.data.value?.collections ?? []);
  const items = computed<CollectionItemSummary[]>(() => treeQuery.data.value?.items ?? []);
  const loaded = computed(() => treeQuery.data.value !== undefined);
  // A rename made in another window reaches this one only as a tree refresh: bound tabs follow it
  // here, or Save in this window would write the old name back.
  watch(items, (list) => syncRequestTabNames(list));
  watch(collections, (list) => syncVariableSetTabNames('collection', list));
  /** P108 F10's successor for the read side: a failed initial/refetch List used to reject into a
   *  void'd promise with nothing shown — this surfaces it the same way state.error does for a
   *  mutation's own failure (rendered in ImportReportStrip.vue). */
  const treeLoadError = computed(() =>
    treeQuery.error.value ? errorText(treeQuery.error.value) : null,
  );

  /** Every tree mutation below refreshes this one key then reconciles — reconcileTree drops any
   *  cached saved-request query whose item no longer exists and evicts any
   *  cached collection-scope variables query whose collection no longer exists, local mutation or
   *  remote push alike. */
  async function afterTreeListChange(): Promise<void> {
    await refreshApiQuery(apiCollectionsTreeKey);
    reconcileTree();
  }

  // The store's own compatibility surface for saved-request reads
  // (fetchSavedRequest/savedRequestFor/isOrphanRequest/ensureSavedRequestLoaded and their gRPC
  // siblings) is gone — CollectionsTree.vue's own onOpen and both request views call
  // loadSavedRequest/useSavedRequest (apiQueries.ts) directly now, and nothing else in the app
  // still called the wrappers, so there was no reader left to keep them thin for.

  // ---- the row model ----

  /** childrenOf used to filter+sort the whole flat `items` array on every single call,
   *  and both visibleRows and subtreeMatches call it once per row per level — an O(N²)-ish walk on
   *  every keystroke for a collection of any real size. Built once per `items` change (a computed,
   *  not per childrenOf call), keyed by collectionId+parentId, each bucket pre-sorted once. */
  const childrenIndex = computed<Map<string, CollectionItemSummary[]>>(() => {
    const index = new Map<string, CollectionItemSummary[]>();
    for (const item of items.value) {
      const key = `${item.collectionId}\x00${item.parentId ?? ''}`;
      const bucket = index.get(key);
      if (bucket) bucket.push(item);
      else index.set(key, [item]);
    }
    for (const bucket of index.values()) bucket.sort((a, b) => a.sortOrder - b.sortOrder);
    return index;
  });

  function childrenOf(collectionId: string, parentId: string | null): CollectionItemSummary[] {
    return childrenIndex.value.get(`${collectionId}\x00${parentId ?? ''}`) ?? [];
  }

  // The same 150ms debounce this app already uses for a fast typist
  // (TIMING.search) — state.search itself stays immediate (so the
  // input never stutters), and activeSearchQuery/visibleRows read the debounced value instead, so a
  // fast typist causes one recompute per pause rather than one per character.
  // refDebounced replaces the watch+setTimeout pair this used to hand-roll.
  const debouncedSearch = refDebounced(toRef(state, 'search'), TIMING.search);

  /** Lower-cased once per render rather than per row. '' means "no search active". */
  const activeSearchQuery = computed(() => debouncedSearch.value.trim().toLowerCase());

  /** True when this subtree contains a match, so an ancestor of a hit still renders. Finding 14: a
   *  request row can never have children (only a folder or collection can), so it skips the
   *  childrenOf lookup entirely rather than querying an index bucket that can only ever be empty. */
  function subtreeMatches(item: CollectionItemSummary, query: string): boolean {
    if (rowMatches(item.name, item.url, query)) return true;
    if (item.kind === 'request') return false;
    return childrenOf(item.collectionId, item.id).some((child) => subtreeMatches(child, query));
  }

  const visibleRows = computed<CollectionRowVm[]>(() => {
    const query = activeSearchQuery.value;
    const rows: CollectionRowVm[] = [];

    const pushItems = (collectionId: string, parentId: string | null, depth: number): void => {
      for (const item of childrenOf(collectionId, parentId)) {
        if (query && !subtreeMatches(item, query)) continue;
        const children = childrenOf(collectionId, item.id);
        // While a query is active every ancestor of a match renders expanded **without mutating
        // `expanded`**, so clearing the search restores exactly the shape the user had.
        const expanded = query ? true : expandedSet.has(itemKey(item.id));
        rows.push({
          key: itemKey(item.id),
          depth,
          hasChildren: item.kind === 'folder' && children.length > 0,
          expanded,
          kind: item.kind,
          id: item.id,
          collectionId,
          parentId,
          name: item.name,
          method: item.method,
          url: item.url,
          protocol: item.protocol,
          matched: rowMatches(item.name, item.url, query),
        });
        if (item.kind === 'folder' && expanded) pushItems(collectionId, item.id, depth + 1);
      }
    };

    for (const collection of collections.value) {
      const children = childrenOf(collection.id, null);
      const hasMatch = !query || children.some((child) => subtreeMatches(child, query));
      if (query && !hasMatch && !rowMatches(collection.name, '', query)) continue;
      const expanded = query ? true : expandedSet.has(collectionKey(collection.id));
      rows.push({
        key: collectionKey(collection.id),
        depth: 0,
        hasChildren: children.length > 0,
        expanded,
        kind: 'collection',
        id: collection.id,
        collectionId: collection.id,
        parentId: null,
        name: collection.name,
        method: '',
        url: '',
        protocol: 'http',
        matched: rowMatches(collection.name, '', query),
      });
      if (expanded) pushItems(collection.id, null, 1);
    }
    return rows;
  });

  // ---- selection and expansion ----

  function toggleRow(row: CollectionRowVm): void {
    if (!row.hasChildren) return;
    if (expandedSet.has(row.key)) expandedSet.delete(row.key);
    else expandedSet.add(row.key);
  }

  function expandRow(row: CollectionRowVm): void {
    if (row.hasChildren) expandedSet.add(row.key);
  }

  function collapseRow(row: CollectionRowVm): void {
    expandedSet.delete(row.key);
  }

  /** Every ancestor of an item, so a freshly created row is visible without the user expanding to
   *  it. Walks parent ids rather than the row model, which may not contain the row yet. Called
   *  right after a mutation's own mutateAsync resolves — the
   *  cache is already fresh at that point, but the reactive `items` computed may still lag one
   *  macrotask behind it (notifyManager's own batching), so this reads the cache directly rather
   *  than `items.value`. */
  function revealItem(collectionId: string, itemId: string | null): void {
    expandedSet.add(collectionKey(collectionId));
    const currentItems =
      queryClient.getQueryData<ApiCollectionsTree>(apiCollectionsTreeKey)?.items ?? [];
    let cursor = itemId;
    while (cursor) {
      const item = currentItems.find((row) => row.id === cursor);
      if (!item) return;
      expandedSet.add(itemKey(item.id));
      cursor = item.parentId;
    }
  }

  // ---- mutations ----
  //
  // Every one of them re-lists rather than patching the local arrays: the tree is one call and a
  // pure computed over it, so a re-list is both simpler and impossible to get out of step
  // with what Go stored. The inline rename doubles as the naming step for all three
  // creation paths — one naming interaction instead of a prompt dialog this app does not have, and
  // VS Code's own explorer behaviour, which is the tree this panel is modelled on.

  const createCollectionMutation = useMutation(
    {
      mutationKey: ['apiCollectionsTree', 'createCollection'],
      mutationFn: () => control.collectionsCreateCollection(t('common.newCollection.label')),
      onSuccess: afterTreeListChange,
    },
    queryClient,
  );

  async function createCollection(): Promise<void> {
    try {
      const collection = await createCollectionMutation.mutateAsync();
      selection.selectOnly(collectionKey(collection.id));
      state.error = null;
    } catch (err) {
      state.error = errorText(err);
    }
  }

  const createItemMutation = useMutation(
    {
      mutationKey: ['apiCollectionsTree', 'createItem'],
      mutationFn: (args: {
        collectionId: string;
        parentId: string | null;
        kind: CollectionItemKind;
      }) =>
        control.collectionsCreateItem({
          collectionId: args.collectionId,
          parentId: args.parentId,
          kind: args.kind,
          name:
            args.kind === 'folder'
              ? t('studio.api.collections.newFolder.label')
              : t('studio.api.common.newRequest.label'),
        }),
      onSuccess: afterTreeListChange,
    },
    queryClient,
  );

  async function createItem(
    collectionId: string,
    parentId: string | null,
    kind: CollectionItemKind,
  ): Promise<void> {
    try {
      const item = await createItemMutation.mutateAsync({ collectionId, parentId, kind });
      revealItem(collectionId, parentId);
      selection.selectOnly(itemKey(item.id));
      state.error = null;
    } catch (err) {
      state.error = errorText(err);
    }
  }

  const createGrpcItemMutation = useMutation(
    {
      mutationKey: ['apiCollectionsTree', 'createGrpcItem'],
      mutationFn: (args: { collectionId: string; parentId: string | null }) =>
        control.collectionsCreateGrpcItem({
          collectionId: args.collectionId,
          parentId: args.parentId,
          name: t('shared.grpc.newRequest.title'),
        }),
      onSuccess: afterTreeListChange,
    },
    queryClient,
  );

  /** createItem's own gRPC sibling (P11 D12) — always a request, never a folder. */
  async function createGrpcItem(collectionId: string, parentId: string | null): Promise<void> {
    try {
      const item = await createGrpcItemMutation.mutateAsync({ collectionId, parentId });
      revealItem(collectionId, parentId);
      selection.selectOnly(itemKey(item.id));
      state.error = null;
    } catch (err) {
      state.error = errorText(err);
    }
  }

  function beginRename(row: CollectionRowVm): void {
    selection.selectOnly(row.key);
    state.renamingKey = row.key;
  }

  function cancelRename(): void {
    state.renamingKey = null;
  }

  const renameRowMutation = useMutation(
    {
      mutationKey: ['apiCollectionsTree', 'rename'],
      mutationFn: (args: { id: string; target: 'collection' | 'item'; name: string }) =>
        control.collectionsRename(args.id, args.target, args.name),
      onSuccess: afterTreeListChange,
    },
    queryClient,
  );

  /** Renames the row in its file. Bound tabs follow through the tree refresh. */
  async function renameRow(row: CollectionRowVm, name: string): Promise<void> {
    state.renamingKey = null;
    try {
      const target = row.kind === 'collection' ? 'collection' : 'item';
      await renameRowMutation.mutateAsync({ id: row.id, target, name });
      state.error = null;
    } catch (err) {
      state.error = errorText(err);
    }
  }

  const moveRowsMutation = useMutation(
    {
      mutationKey: ['apiCollectionsTree', 'moveItems'],
      mutationFn: (args: { itemIds: string[]; placement: Placement }) =>
        control.collectionsMoveItems({ itemIds: args.itemIds, ...args.placement }),
      onSuccess: afterTreeListChange,
    },
    queryClient,
  );

  /** Moves requests and folders, each with its subtree, to `placement`, keeping their order. A move
   *  into another collection file makes new rows: open tabs and the selection follow them. Returns
   *  whether it moved. */
  async function moveRows(itemIds: readonly string[], placement: Placement): Promise<boolean> {
    try {
      const moved = await moveRowsMutation.mutateAsync({ itemIds: [...itemIds], placement });
      remapRequestTabItems(moved);
      revealItem(placement.collectionId, placement.parentId);
      const keys = itemIds.map((id) => itemKey(moved[id] ?? id));
      selection.selectMany(keys, keys[0]);
      state.error = null;
      return true;
    } catch (err) {
      state.error = errorText(err);
      return false;
    }
  }

  const reorderCollectionsMutation = useMutation(
    {
      mutationKey: ['apiCollectionsTree', 'reorderCollections'],
      mutationFn: (ids: string[]) => control.collectionsReorderCollections(ids),
      onSuccess: afterTreeListChange,
    },
    queryClient,
  );

  /** Applies a whole-list collection order. Returns whether it saved. */
  async function reorderCollections(ids: readonly string[]): Promise<boolean> {
    try {
      await reorderCollectionsMutation.mutateAsync([...ids]);
      state.error = null;
      return true;
    } catch (err) {
      state.error = errorText(err);
      return false;
    }
  }

  function siblingsOfRow(row: CollectionRowVm): CollectionItemSummary[] {
    return childrenOf(row.collectionId, row.parentId);
  }

  const collectionIds = (): string[] => collections.value.map((c) => c.id);

  /** Whether `row` can step toward `step` within its own sibling set. */
  function canMoveStep(row: CollectionRowVm, step: -1 | 1): boolean {
    if (activeSearchQuery.value !== '') return false;
    if (row.kind === 'collection')
      return collectionStepOrder(collectionIds(), row.id, step) !== null;
    return itemStepBefore(siblingsOfRow(row), row.id, step) !== undefined;
  }

  /** Moves a collection, request or folder one place within its siblings. */
  async function moveStep(row: CollectionRowVm, step: -1 | 1): Promise<boolean> {
    if (!canMoveStep(row, step)) return false;
    if (row.kind === 'collection') {
      const order = collectionStepOrder(collectionIds(), row.id, step);
      return order ? reorderCollections(order) : false;
    }
    const beforeId = itemStepBefore(siblingsOfRow(row), row.id, step);
    if (beforeId === undefined) return false;
    return moveRows([row.id], { collectionId: row.collectionId, parentId: row.parentId, beforeId });
  }

  /** Moves rows to the root of another collection. Returns whether it moved. */
  function moveRowsToCollection(
    rows: readonly CollectionRowVm[],
    collectionId: string,
  ): Promise<boolean> {
    return moveRows(
      rows.map((r) => r.id),
      { collectionId, parentId: null, beforeId: null },
    );
  }

  /** Creates a collection, moves the rows into it, and opens the new collection for naming. */
  async function moveRowsToNewCollection(rows: readonly CollectionRowVm[]): Promise<void> {
    try {
      const collection = await createCollectionMutation.mutateAsync();
      if (!(await moveRowsToCollection(rows, collection.id))) return;
      state.renamingKey = collectionKey(collection.id);
    } catch (err) {
      state.error = errorText(err);
    }
  }

  /** Every item id a delete of `row` also removes: the row's subtree, read from the cache because
   *  the reactive `items` computed can lag it by one macrotask. */
  function subtreeItemIds(row: CollectionRowVm): string[] {
    if (row.kind === 'request') return [row.id];
    const currentItems =
      queryClient.getQueryData<ApiCollectionsTree>(apiCollectionsTreeKey)?.items ?? [];
    if (row.kind === 'collection') {
      return currentItems.filter((item) => item.collectionId === row.id).map((item) => item.id);
    }
    const ids: string[] = [row.id];
    const stack: string[] = [row.id];
    while (stack.length > 0) {
      const parentId = stack.pop() as string;
      for (const child of currentItems.filter(
        (item) => item.collectionId === row.collectionId && item.parentId === parentId,
      )) {
        ids.push(child.id);
        if (child.kind === 'folder') stack.push(child.id);
      }
    }
    return ids;
  }

  const deleteRowsMutation = useMutation(
    {
      mutationKey: ['apiCollectionsTree', 'delete'],
      mutationFn: (args: { collectionIds: string[]; itemIds: string[] }) =>
        control.collectionsDeleteMany(args),
      onSuccess: afterTreeListChange,
    },
    queryClient,
  );

  /** What deleting `rows` removes: the topmost rows, and every item id their subtrees hold. */
  function planDelete(rows: readonly CollectionRowVm[]): {
    top: CollectionRowVm[];
    itemIds: Set<string>;
  } {
    const top = topmost(rows, items.value);
    const itemIds = new Set<string>();
    for (const row of top) for (const id of subtreeItemIds(row)) itemIds.add(id);
    return { top, itemIds };
  }

  /** Deletes `rows` with their subtrees in one call. */
  async function deleteRows(rows: readonly CollectionRowVm[]): Promise<void> {
    const { top, itemIds } = planDelete(rows);
    if (top.length === 0) return;
    try {
      await deleteRowsMutation.mutateAsync({
        collectionIds: top.filter((r) => r.kind === 'collection').map((r) => r.id),
        itemIds: top.filter((r) => r.kind !== 'collection').map((r) => r.id),
      });
      // A request tab stays open as an editing surface; its cached saved request goes, so the tab
      // reads as unsaved. null is the orphan marker, written for every descendant explicitly.
      for (const id of itemIds) {
        queryClient.setQueryData(apiSavedRequestKey(id), null);
        queryClient.setQueryData(apiSavedGrpcRequestKey(id), null);
      }
      for (const row of top) {
        if (row.kind === 'collection') closeVariableSetTabsForOwner('collection', row.id);
      }
      state.error = null;
    } catch (err) {
      state.error = errorText(err);
    }
  }

  /** Duplicating a folder copies the folder alone, a request copies its saved request. */
  async function duplicateRow(row: CollectionRowVm): Promise<void> {
    const name = t('studio.api.collections.copyName.label', { name: row.name });
    try {
      if (row.kind === 'folder') {
        await control.collectionsCreateItem({
          collectionId: row.collectionId,
          parentId: row.parentId,
          kind: 'folder',
          name,
        });
      } else if (row.protocol === 'grpc') {
        const saved = await loadSavedGrpcRequest(row.id);
        if (!saved) throw new Error(t('studio.api.collections.requestGone.error'));
        await control.collectionsCreateGrpcItem({
          collectionId: row.collectionId,
          parentId: row.parentId,
          name,
          request: saved,
        });
      } else {
        const saved = await loadSavedRequest(row.id);
        if (!saved) throw new Error(t('studio.api.collections.requestGone.error'));
        await control.collectionsCreateItem({
          collectionId: row.collectionId,
          parentId: row.parentId,
          kind: 'request',
          name,
          request: saved,
        });
      }
      await afterTreeListChange();
      state.error = null;
    } catch (err) {
      state.error = errorText(err);
    }
  }

  /** Opens a request row into its tab (reusing one already bound to the row). A row deleted between
   *  the tree listing and the fetch loads as `null` and opens as a brand-new
   *  saved row; any other load failure lands in `state.error` and opens nothing. A gRPC row
   *  opens the 'grpc-request' kind, which the protocol column alone tells apart. */
  async function openRequestRow(row: CollectionRowVm): Promise<void> {
    try {
      if (row.protocol === 'grpc') {
        const saved = (await loadSavedGrpcRequest(row.id)) ?? defaultGrpcSavedRequest();
        openCollectionGrpcRequestTab(row.id, row.name, saved);
      } else {
        const saved = (await loadSavedRequest(row.id)) ?? defaultHttpSavedRequest();
        openCollectionRequestTab(row.id, row.name, saved);
      }
    } catch (err) {
      state.error = errorText(err);
    }
  }

  /** Creates the row, caches it as the tab's saved side, and binds the tab to it. Resolves to the
   *  failure message (also set on `state.error`) so the dialog can show it itself, null on success. */
  async function submitSaveDialog(
    collectionId: string,
    parentId: string | null,
    name: string,
  ): Promise<string | null> {
    const saveDialogStore = useSaveRequestDialogStore();
    const { tabId, payload } = saveDialogStore;
    if (!tabId || !payload) return null;

    try {
      if (payload.protocol === 'grpc') {
        const item = await control.collectionsCreateGrpcItem({
          collectionId,
          parentId,
          name,
          request: payload.request,
        });
        queryClient.setQueryData(apiSavedGrpcRequestKey(item.id), payload.request);
        try {
          await control.grpcHistoryAdopt(tabId, item.id);
        } catch (err) {
          logWarn('collections', 'adopting grpc call history into the saved request failed', err);
        }
        patchGrpcRequestTabState(tabId, { itemId: item.id, name });
        await afterTreeListChange();
        revealItem(collectionId, parentId);
        saveDialogStore.closeSaveDialog();
        state.error = null;
        return null;
      }

      const item = await control.collectionsCreateItem({
        collectionId,
        parentId,
        kind: 'request',
        name,
        request: payload.request,
      });
      queryClient.setQueryData(apiSavedRequestKey(item.id), payload.request);
      // P8 D14: a scratch tab's response history follows it into the collection, before the tab's
      // itemId is patched below. Best-effort, the same posture D2's own Go-side Record call takes
      // — Save as… itself must succeed regardless of whether adopting its history did.
      // ResponsePane.vue's own watch on tab.state.itemId is what refetches the list under the new
      // scope once the patch below actually lands (http/** may not import views/**, so the
      // refetch can't be triggered from here).
      try {
        await control.historyAdopt(tabId, item.id);
      } catch (err) {
        logWarn('collections', 'adopting response history into the saved request failed', err);
      }
      // The tab is now bound to a real row, so its title becomes the saved name and Save stops
      // falling back to Save as…
      patchHttpRequestTabState(tabId, { itemId: item.id, name });
      await afterTreeListChange();
      revealItem(collectionId, parentId);
      saveDialogStore.closeSaveDialog();
      state.error = null;
      return null;
    } catch (err) {
      state.error = errorText(err);
      return state.error;
    }
  }

  /** Save — writes an already-bound request back to its own row. */
  async function saveRequest(
    itemId: string,
    name: string,
    request: HttpSavedRequest,
  ): Promise<void> {
    try {
      await control.collectionsSaveRequest(itemId, name, request);
      // The cache is the dirty comparison's saved side, so it must move in step with the write or
      // the mark would stay lit after a successful save.
      queryClient.setQueryData(apiSavedRequestKey(itemId), request);
      await afterTreeListChange();
      state.error = null;
    } catch (err) {
      state.error = errorText(err);
    }
  }

  /** saveRequest's own gRPC sibling. */
  async function saveGrpcRequest(
    itemId: string,
    name: string,
    request: GrpcSavedRequest,
  ): Promise<void> {
    try {
      await control.collectionsSaveGrpcRequest(itemId, name, request);
      queryClient.setQueryData(apiSavedGrpcRequestKey(itemId), request);
      await afterTreeListChange();
      state.error = null;
    } catch (err) {
      state.error = errorText(err);
    }
  }

  // ---- import (D11/D12) ----

  /** Opens the native file dialog and imports the chosen files (collections and environments, each
   *  independent). **Only the paths cross the bridge**
   *  — Go opens the file (F16: a 10-50 MB collection through the control plane is 20-100 serial
   *  round trips, and above 64 MiB an unattributable refusal). Returns false when cancelled. */
  async function importPostman(): Promise<boolean> {
    // A real re-entry guard. Until this, the only thing stopping a second concurrent
    // import was the panel button disabling itself while `busy` — and that button has moved to the
    // menu bar, which has no such state. Two overlapping imports would interleave their
    // afterTreeListChange()/report writes.
    if (state.busy) return false;
    const chosen = await control.filesChooseOpenMany({
      title: t('studio.api.collections.import.title'),
      filters: [{ name: t('studio.api.collections.postmanFilter.label'), extensions: ['json'] }],
    });
    if (chosen.canceled || !chosen.files?.length) return false;

    // D11: the panel's header action is disabled with a spinner for the duration rather than joining
    // the op log — that machinery is per tab (ViewChrome + useRunState) and an import started from
    // the left panel has no tab, so a row there would buy nothing the user is looking at.
    state.busy = true;
    state.error = null;
    try {
      const result = await control.collectionsImport(chosen.files.map((f) => f.path));
      state.importResult = {
        files: result.files.map((f) => ({
          ...f,
          collection: f.collection && { ...f.collection, warnings: f.collection.warnings ?? [] },
          environment: f.environment && {
            ...f.environment,
            warnings: f.environment.warnings ?? [],
          },
        })),
      };
      const first = result.files.find((f) => f.collection)?.collection;
      if (first) {
        expandedSet.add(collectionKey(first.collectionId));
        selection.selectOnly(collectionKey(first.collectionId));
      }
      return true;
    } catch (err) {
      // This used to let the error propagate uncaught, and (since it ran only in the
      // try's success path) never reloaded the tree on failure either — a partially-applied import
      // (Go's own import is not one transaction; some rows can land before a later one fails) left
      // the panel showing the pre-import tree until something else happened to reload it.
      state.error = errorText(err);
      return false;
    } finally {
      // Always reloads, success or failure (P108 F10), before clearing `busy` — a partial import's
      // rows must be visible immediately, not just after the next unrelated refresh.
      await afterTreeListChange();
      state.busy = false;
    }
  }

  function dismissReport(): void {
    state.importResult = null;
  }

  function dismissExportWarning(): void {
    state.exportWarning = null;
  }

  // ---- export (D10/D11/D16) ----

  /** Opens the native save dialog and writes the collection there as Collection v2.1 JSON. As with
   *  import, only the path crosses the bridge — Go writes the file. Returns false when cancelled.
   *  A secret exports valueless, and ExportReport.secretCount is what surfaces that once,
   *  rather than it being a fact only discoverable by opening the file. Postman Collection v2.1
   *  has no representation for a gRPC request, so Go's own Export skips every protocol='grpc' item
   *  and reports the count in ExportReport.skippedGrpc — this used to read secretCount only,
   *  silently dropping the skipped-request half of the same report the Go side already produces. */
  async function exportCollection(collectionId: string, name: string): Promise<boolean> {
    // Same re-entry guard as import: this `finally` would otherwise clear a running import's busy.
    if (state.busy) return false;
    // The extension Postman's own exporter writes, so the file is recognisable on disk and
    // re-importable without renaming.
    const chosen = await control.filesChooseSave(`${name}.postman_collection.json`);
    if (chosen.canceled || !chosen.filePath) return false;
    state.busy = true;
    state.error = null;
    try {
      const report = await control.collectionsExport(collectionId, chosen.filePath);
      const notes: string[] = [];
      if (report.secretCount > 0) {
        notes.push(t('studio.api.collections.exportSecrets.body', { count: report.secretCount }));
      }
      if (report.skippedGrpc > 0) {
        notes.push(
          t('studio.api.collections.exportSkippedGrpc.body', { count: report.skippedGrpc }),
        );
      }
      state.exportWarning = notes.length > 0 ? notes.join(' ') : null;
      return true;
    } catch (err) {
      state.error = errorText(err);
      return false;
    } finally {
      state.busy = false;
    }
  }

  // ---- lookups the panel, the menus and the request view all share ----

  function itemRecord(itemId: string): CollectionItemSummary | undefined {
    return items.value.find((item) => item.id === itemId);
  }

  /** The collection a tab's own saved row belongs to, or '' for a scratch tab (D6). Shared by both
   *  protocols (P12 D9/F10) — both HttpRequestTabState and GrpcRequestTabState carry an `itemId`,
   *  which is the only field this reads. */
  function collectionIdFor(tabState: { itemId: string | null }): string {
    if (!tabState.itemId) return '';
    return itemRecord(tabState.itemId)?.collectionId ?? '';
  }

  function collectionRecord(collectionId: string): CollectionSummary | undefined {
    return collections.value.find((c) => c.id === collectionId);
  }

  /** Every folder in a collection, in tree order, as `Folder / Subfolder` labels — the target
   *  picker's own list (D15). */
  function folderPaths(collectionId: string): { id: string; label: string }[] {
    const out: { id: string; label: string }[] = [];
    const walk = (parentId: string | null, prefix: string): void => {
      for (const item of childrenOf(collectionId, parentId)) {
        if (item.kind !== 'folder') continue;
        const label = prefix ? `${prefix} / ${item.name}` : item.name;
        out.push({ id: item.id, label });
        walk(item.id, label);
      }
    };
    walk(null, '');
    return out;
  }

  return {
    ...toRefs(state),
    expanded: expandedKeys,
    collections,
    items,
    loaded,
    treeLoadError,
    activeSearchQuery,
    visibleRows,
    toggleRow,
    expandRow,
    collapseRow,
    createCollection,
    createItem,
    createGrpcItem,
    openRequestRow,
    beginRename,
    cancelRename,
    renameRow,
    moveRows,
    reorderCollections,
    canMoveStep,
    moveStep,
    moveRowsToCollection,
    moveRowsToNewCollection,
    planDelete,
    deleteRows,
    duplicateRow,
    submitSaveDialog,
    saveRequest,
    saveGrpcRequest,
    importPostman,
    dismissReport,
    dismissExportWarning,
    dismissError,
    exportCollection,
    itemRecord,
    collectionIdFor,
    collectionRecord,
    folderPaths,
  };
});

export type { CollectionItemKind };
