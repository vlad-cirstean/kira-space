import { t } from '@theme/i18n';
import { useDebounceFn } from '@vueuse/core';
import { defineStore } from 'pinia';
import { type Ref, reactive, toRefs } from 'vue';
import { TIMING } from '../behaviour/timing';
import { logError } from '../bridge/log';
import { useConfirmDialogStore } from './confirmDialog';
import { cleanupTabRuntime } from './tabRuntime';

// The shared skeleton both apps' `state/tabs.ts` used to duplicate —
// `persistableTabs`, `saveIfChanged`, the `saveNow`/`flushPendingTabState` pair and its two
// `control.on*` registrations, `setActiveTabId`, `removeFromPreviewCohort`, `evictPreviewCohort`,
// `reuseExistingTab`, `insertNewTabRecord`, `openTab`, `duplicateTab`, `closeTabInternal`,
// `closeTab`, `closeOthers`, `closeToTheRight`, `closeAll`, `activateTab`, `moveTab`, `stepTab`,
// `isPreview`, `promoteTab`, `patchChanged`, `patchTabState` — over the one axis of real
// divergence, the workspace-key type `K` (`AppMode` in Kira Studio, `WorkspaceKey` in Kira Space).
//
// Everything Studio-only (the `hydrated` reconnect gate, the `tabIncognitoStore` filter and its
// listener, `control.onConnectionsChanged`/`onConnectionState`, the per-kind `findXTab` readers)
// stays entirely in Kira Studio's own `state/tabs.ts`, composed on through `extend` below — the
// factory itself never references any of it. Kira Space's own extras (`createPinnedRepoGraphTab`,
// `closeWorkspaceTabs`, the per-kind patchers) compose the same way.

interface TabRecordLike {
  id: string;
  connectionId: string | null;
  path: string;
  kind: string;
  state: unknown;
  order: number;
  active: boolean;
  workspaceId?: string | null;
}

export interface TabsControl<R> {
  tabsList(): Promise<R[]>;
  tabsSave(tabs: R[]): Promise<void>;
  onFlushBeforeClose(cb: () => void): () => void;
  onWindowFlushBeforeClose(cb: () => void): () => void;
  appFlushed(): void;
  windowFlushed(): void;
}

interface KindLookup<R> {
  readonly [kind: string]: {
    pinned?: true;
    /** True while the tab holds unsaved work; closing it asks to discard first. */
    isDirty?(tabId: string): boolean;
    dropResources(tabId: string): void;
    /** P108 Part 12 F14: hydrateTabs' own reset-on-unparseable fallback. */
    defaultState(): unknown;
    parseState(raw: unknown): unknown | null;
    duplicateState(tab: R): unknown;
  };
}

export interface OpenTabResult {
  id: string;
  reused: boolean;
}

export interface OpenTabOpts {
  reuse: boolean;
  workspaceId?: string | null;
  preview?: boolean;
  /** P74 §5.2: a bulk open's own files join the workspace's preview cohort instead of each
   *  replacing the last — never evicts. Ignored when `preview` is falsy. */
  previewCohort?: boolean;
  /** Id of an existing tab the new one takes the place of: same strip slot, the old tab closes. */
  replace?: string;
}

/** Every generic action the store exposes, before an app's own `extend` adds more — also what
 *  `extend` itself receives, so it can compose (e.g. Kira Space's `closeWorkspaceTabs` reuses
 *  `dropAllPagesForTab`/`saveNow`; Kira Studio's `onConnectionState` listener reuses
 *  `dropPageStoresForTab`). */
export interface TabsStoreActions<K extends string, R extends TabRecordLike> {
  tabs: Ref<R[]>;
  activeIdByWorkspace: Ref<Record<K, string | null>>;
  previewIdsByWorkspace: Ref<Record<K, readonly string[]>>;
  hydrateTabs(): Promise<void>;
  removeFromPreviewCohort(key: K, id: string): void;
  evictPreviewCohort(key: K, keepId?: string): void;
  openTab<S>(
    kind: R['kind'],
    connectionId: string | null,
    path: string,
    makeState: () => S,
    opts: OpenTabOpts,
  ): OpenTabResult;
  duplicateTab(id: string): string;
  dropPageStoresForTab(id: string): void;
  dropAllPagesForTab(id: string): void;
  /** With a dirty tab among those closing, the reader confirms the discard first; declining closes
   *  nothing. With none dirty they close synchronously, before the promise returns. */
  closeTab(id: string): Promise<void>;
  /** System-initiated cleanup (a deleted connection's tabs): closes without the discard prompt. */
  dropTabs(ids: readonly string[]): void;
  closeOthers(id: string): Promise<void>;
  closeToTheRight(id: string): Promise<void>;
  closeAll(key: K): Promise<void>;
  activateTab(id: string): void;
  moveTab(fromId: string, toId: string): void;
  stepTab(key: K, delta: 1 | -1): void;
  isPreview(id: string): boolean;
  promoteTab(id: string): void;
  patchTabState<S extends object>(
    id: string,
    kind: R['kind'],
    patch: Partial<S>,
    opts: { skipUnchanged: boolean },
  ): void;
  saveNow(): void;
}

export interface TabsHost<
  K extends string,
  R extends TabRecordLike,
  E extends Record<string, unknown> = Record<string, never>,
> {
  kinds: KindLookup<R>;
  workspaceKeyOf(tab: R): K;
  control: TabsControl<R>;
  /** Where a fresh `openTab` call with no explicit `workspaceId` lands — Kira Studio's own kind's
   *  mode (`STUDIO_TAB_KIND_MODE[kind]`); Kira Space's one shared unscoped slot
   *  (`GENERAL_WORKSPACE`, ignoring `kind`). */
  fallbackWorkspaceKey(kind: R['kind']): K;
  /** Keys `hydrateTabs` always seeds into `activeIdByWorkspace`, even with zero restored tabs —
   *  Kira Studio's `['studio', 'api']`. Kira Space needs none: every workspace key it has comes
   *  from a restored tab's own `workspaceId`. */
  seedWorkspaceKeys?: readonly K[];
  /** A restored tab persists by default whenever `kind` is neither 'terminal' nor 'script-run' (Kira Space's own rule,
   *  the whole rule). Kira Studio narrows it further: also never an incognito tab. */
  persistable?(tab: R): boolean;
  /** Runs once, after `hydrateTabs` has parsed every restored tab and seeded
   *  `activeIdByWorkspace` — Kira Space's own hook, deriving `useWorkspaceStore().openRepos` from
   *  the restored tabs' workspace keys. */
  onHydrated?(tabs: R[]): void;
  /** Runs at the end of a tab's creation (`openTab`'s fresh-record branch, and `duplicateTab`,
   *  both with `reused: false`) and at the end of a reuse (`reused: true`) — Kira Studio's own
   *  hook, marking the tab hydrated (unconditionally when fresh/duplicated; only when reusing a
   *  tab whose connection is live right now, mirroring the old inline check exactly). */
  onOpened?(record: R, reused: boolean): void;
  /** Runs right alongside every `dropAllPagesForTab` call inside `closeTabInternal`/`closeOthers`/
   *  `closeToTheRight`/`closeAll` — Kira Studio's own hook, unmarking hydration and clearing the
   *  grid-view cell-selection/pending-edit state a closed tab could have left behind. No ordering
   *  dependency on `dropAllPagesForTab` (or vice versa), so call order doesn't matter. */
  onClosed?(tabId: string): void;
  /** Runs right after `duplicateTab` pushes the new record, before `onOpened` — Kira Studio's own
   *  hook, copying the source tab's incognito flag onto the duplicate so "Duplicate tab" on an
   *  incognito tab doesn't silently start persisting the copy. */
  onDuplicated?(source: R, duplicate: R): void;
  /** Adds extra actions/state to the returned store, given the generic actions above plus
   *  `dropPageStoresForTab`/`dropAllPagesForTab`/`saveNow` (public precisely so `extend` — and,
   *  for Kira Studio, code outside the store entirely — can reach them). Runs once, inside the
   *  store's own `setup()`, so a listener registered here (Kira Studio's
   *  `control.onConnectionsChanged`/`onConnectionState`, its `tabIncognitoStore` listener) fires
   *  exactly when the original inline registration did — on first store access, not at module
   *  load.
   *
   *  Required, even for a caller with nothing to add (`extend: () => ({})`) — a call that leaves
   *  `E` at its default with no argument to infer it from breaks Pinia's own action/state
   *  extraction for the *whole* store (every instantiation, not just that one), a real TS+Pinia
   *  interaction — the fix as harmless as it is unobvious, so it stays required rather than a
   *  trap for the next caller. */
  extend(actions: TabsStoreActions<K, R>): E;
}

export function createTabsStore<
  K extends string,
  R extends TabRecordLike,
  E extends Record<string, unknown> = Record<string, never>,
>(host: TabsHost<K, R, E>) {
  return defineStore('tabs', () => {
    function dropPageStoresForTab(id: string): void {
      for (const kind of Object.keys(host.kinds)) {
        host.kinds[kind]?.dropResources(id);
      }
    }

    function dropAllPagesForTab(id: string): void {
      dropPageStoresForTab(id);
      cleanupTabRuntime(id);
    }

    // Vue's own `reactive<T>` return type (`UnwrapNestedRefs<T>`) recomputes a structurally-equal
    // but nominally different type for a generic `R`/`K` — this cast restores the plain types
    // below without changing `reactive`'s actual runtime behavior at all.
    const tabsState = reactive({
      tabs: [] as R[],
      activeIdByWorkspace: {} as Record<K, string | null>,
      previewIdsByWorkspace: {} as Record<K, readonly string[]>,
    }) as {
      tabs: R[];
      activeIdByWorkspace: Record<K, string | null>;
      previewIdsByWorkspace: Record<K, readonly string[]>;
    };

    // D17: the last serialisation actually written — a save whose snapshot is identical to this
    // skips the IPC and the write entirely, not just the debounce.
    let lastSavedSnapshot: string | null = null;
    // `nextSnapshot` coalesces every change that arrives while a save is already
    // chained down to just the latest one; `saveChain` is what serialises — each
    // enqueueSave call chains its own link after whatever is already queued, so at most one
    // tabsSave call is ever in flight. Before this, two overlapping saves (an incognito toggle
    // firing saveNow() while an earlier debounced save was still in flight) had no ordering
    // guarantee — Wails v3 dispatches each bound call on its own goroutine and ReplaceKeyed is one
    // transaction with nothing serialising two calls against each other, so whichever HTTP round
    // trip landed last in the DB won, not whichever was issued last client-side. A close-time flush
    // raced the exact same way against any save already in flight.
    let nextSnapshot: string | null = null;
    let saveChain: Promise<void> = Promise.resolve();

    function persistableTabs(): R[] {
      const isPersistable =
        host.persistable ?? ((t: R) => t.kind !== 'terminal' && t.kind !== 'script-run');
      return tabsState.tabs.filter(isPersistable);
    }

    // Chains onto whatever save is already in flight (or already queued) rather than firing its
    // own — the returned promise resolves once every save queued up to and including this one has
    // settled, which is what flushPendingTabState needs to await. No early-return on
    // `snapshot === lastSavedSnapshot` here: a save targeting a since-superseded snapshot must
    // still run its link so it can re-read `nextSnapshot` fresh at execution time and no-op
    // correctly — an early return here could skip clearing a stale queued target.
    function enqueueSave(snapshot: string): Promise<void> {
      nextSnapshot = snapshot;
      saveChain = saveChain.then(async () => {
        if (nextSnapshot === null || nextSnapshot === lastSavedSnapshot) return;
        const toSave = nextSnapshot;
        nextSnapshot = null;
        await host.control.tabsSave(JSON.parse(toSave) as R[]).then(
          () => {
            lastSavedSnapshot = toSave;
          },
          (err) => {
            logError('tabs', 'save failed', err);
            // Re-queue for the next saveIfChanged/flush; never loops on its own.
            nextSnapshot ??= toSave;
          },
        );
      });
      return saveChain;
    }

    function saveIfChanged(): void {
      void enqueueSave(JSON.stringify(persistableTabs()));
    }

    const saveDebounced = useDebounceFn(saveIfChanged, TIMING.tabsSave);

    function saveNow(): void {
      saveDebounced.cancel();
      saveIfChanged();
    }

    function flushPendingTabState(ack: () => void): void {
      saveDebounced.cancel();
      void enqueueSave(JSON.stringify(persistableTabs())).finally(ack);
    }

    host.control.onFlushBeforeClose(() => flushPendingTabState(host.control.appFlushed));
    host.control.onWindowFlushBeforeClose(() => flushPendingTabState(host.control.windowFlushed));

    async function hydrateTabs(): Promise<void> {
      const raw = await host.control.tabsList();
      const tabs = raw.map((t) => {
        const kind = host.kinds[t.kind];
        const parsed = kind?.parseState(t.state);
        if (parsed) return { ...t, state: parsed } as R;
        // Every per-kind schema's later fields now carry their own
        // `.default(...)` (tabDomain.ts), so a row only reaches here when it fails on a field the
        // schema still requires — genuinely corrupt state, not merely an old row predating a
        // field. Reset to this kind's own defaultState() instead of keeping the unparsed raw
        // state: a data tab's grid used to compute `pageIndex * pageSize` as NaN off a missing
        // `pageIndex`, and DataToolbar.vue's `columnOrder !== null` check treated `undefined` as a
        // custom order rather than "no custom order set".
        return kind ? ({ ...t, state: kind.defaultState() } as R) : t;
      });
      tabsState.tabs = tabs;

      const keys = new Set<K>(host.seedWorkspaceKeys ?? []);
      for (const t of tabs) keys.add(host.workspaceKeyOf(t));
      for (const key of keys) {
        const keyTabs = tabs.filter((t) => host.workspaceKeyOf(t) === key);
        const persistedActive = keyTabs.find((t) => t.active);
        const active = persistedActive ?? keyTabs[0];
        tabsState.activeIdByWorkspace[key] = active?.id ?? null;
        // No restored tab was `active` (the previously-active tab was incognito or a terminal,
        // neither persisted) -- defaulting `activeIdByWorkspace` to the first tab without also
        // flipping its own `active` field left TabStrip (which reads that field) highlighting
        // nothing while MainView (which reads the id) rendered that tab.
        if (active && !persistedActive) {
          for (const t of keyTabs) t.active = t.id === active.id;
        }
      }

      host.onHydrated?.(tabs);
    }

    function setActiveTabId(id: string, key: K): void {
      for (const t of tabsState.tabs) {
        if (host.workspaceKeyOf(t) === key) t.active = t.id === id;
      }
      tabsState.activeIdByWorkspace[key] = id;
    }

    function removeFromPreviewCohort(key: K, id: string): void {
      const cohort = tabsState.previewIdsByWorkspace[key];
      if (!cohort?.includes(id)) return;
      tabsState.previewIdsByWorkspace[key] = cohort.filter((x) => x !== id);
    }

    function evictPreviewCohort(key: K, keepId?: string): void {
      const cohort = tabsState.previewIdsByWorkspace[key] ?? [];
      for (const id of cohort) {
        if (id !== keepId) closeTabInternal(id);
      }
      tabsState.previewIdsByWorkspace[key] = keepId ? [keepId] : [];
    }

    function reuseExistingTab(
      kind: R['kind'],
      connectionId: string | null,
      path: string,
      workspaceId: string | null,
      workspaceKey: K,
      opts: OpenTabOpts,
    ): OpenTabResult | null {
      if (!opts.reuse) return null;
      const existing = tabsState.tabs.find(
        (t) =>
          t.kind === kind &&
          t.connectionId === connectionId &&
          t.path === path &&
          (t.workspaceId ?? null) === workspaceId,
      );
      if (!existing) return null;
      activateTab(existing.id);
      if (!opts.preview) removeFromPreviewCohort(workspaceKey, existing.id);
      host.onOpened?.(existing, true);
      return { id: existing.id, reused: true };
    }

    function insertNewTabRecord(record: R, workspaceKey: K, opts: OpenTabOpts): void {
      if (opts.preview && opts.previewCohort) {
        tabsState.tabs.push(record);
        tabsState.previewIdsByWorkspace[workspaceKey] = [
          ...(tabsState.previewIdsByWorkspace[workspaceKey] ?? []),
          record.id,
        ];
        return;
      }
      if (!opts.preview) {
        tabsState.tabs.push(record);
        return;
      }
      const evictedIds = tabsState.previewIdsByWorkspace[workspaceKey] ?? [];
      if (evictedIds.length > 0) {
        const evictedIdx = tabsState.tabs.findIndex((t) => t.id === evictedIds[0]);
        const insertAt = evictedIdx < 0 ? tabsState.tabs.length : evictedIdx;
        tabsState.tabs.splice(insertAt, 0, record);
        evictPreviewCohort(workspaceKey);
      } else {
        tabsState.tabs.push(record);
      }
      tabsState.previewIdsByWorkspace[workspaceKey] = [record.id];
    }

    function openTab<S>(
      kind: R['kind'],
      connectionId: string | null,
      path: string,
      makeState: () => S,
      opts: OpenTabOpts,
    ): OpenTabResult {
      const workspaceId = opts.workspaceId ?? null;
      // Same derivation as every other action, so an app whose partition key differs from the
      // tab's `workspaceId` (Kira Space's files window) keeps one key per tab.
      const workspaceKey =
        workspaceId === null
          ? host.fallbackWorkspaceKey(kind)
          : host.workspaceKeyOf({ kind, workspaceId } as unknown as R);

      const reused = reuseExistingTab(kind, connectionId, path, workspaceId, workspaceKey, opts);
      if (reused) return reused;

      const id = crypto.randomUUID();
      const record = {
        id,
        connectionId,
        path,
        kind,
        state: makeState(),
        order: tabsState.tabs.length,
        active: true,
        workspaceId,
      } as unknown as R;

      const replaceIdx =
        opts.replace === undefined ? -1 : tabsState.tabs.findIndex((t) => t.id === opts.replace);
      if (replaceIdx >= 0 && opts.replace !== undefined) {
        tabsState.tabs.splice(replaceIdx + 1, 0, record);
        closeTabInternal(opts.replace);
      } else {
        insertNewTabRecord(record, workspaceKey, opts);
      }
      setActiveTabId(id, workspaceKey);
      host.onOpened?.(record, false);
      saveNow();
      return { id, reused: false };
    }

    function duplicateTab(id: string): string {
      const source = tabsState.tabs.find((t) => t.id === id);
      if (!source) return id;
      if (host.kinds[source.kind]?.pinned) return id; // §6.1: a pinned tab is never duplicated.

      const newId = crypto.randomUUID();
      const def = host.kinds[source.kind];
      const record = {
        id: newId,
        connectionId: source.connectionId,
        path: source.path,
        kind: source.kind,
        state: def?.duplicateState(source),
        order: tabsState.tabs.length,
        active: true,
        workspaceId: source.workspaceId ?? null,
      } as unknown as R;
      tabsState.tabs.push(record);
      host.onDuplicated?.(source, record);
      setActiveTabId(newId, host.workspaceKeyOf(source));
      host.onOpened?.(record, false);
      saveNow();
      return newId;
    }

    function closeTabInternal(id: string): void {
      const idx = tabsState.tabs.findIndex((t) => t.id === id);
      if (idx < 0) return;
      const closed = tabsState.tabs[idx];
      if (host.kinds[closed.kind]?.pinned) return; // §6.1: a pinned tab never closes.
      const key = host.workspaceKeyOf(closed);
      const wasActive = closed.active;
      const keyIdxBefore = tabsState.tabs
        .filter((t) => host.workspaceKeyOf(t) === key)
        .findIndex((t) => t.id === id);

      tabsState.tabs.splice(idx, 1);
      dropAllPagesForTab(id);
      host.onClosed?.(id);
      removeFromPreviewCohort(key, id);

      if (wasActive) {
        const keyTabs = tabsState.tabs.filter((t) => host.workspaceKeyOf(t) === key);
        if (keyTabs.length === 0) {
          tabsState.activeIdByWorkspace[key] = null;
        } else {
          const next = keyTabs[Math.min(keyIdxBefore, keyTabs.length - 1)];
          next.active = true;
          tabsState.activeIdByWorkspace[key] = next.id;
        }
      }
    }

    function dirtyCount(ids: Iterable<string>): number {
      let n = 0;
      for (const tabId of ids) {
        const tab = tabsState.tabs.find((x) => x.id === tabId);
        if (tab !== undefined && host.kinds[tab.kind]?.isDirty?.(tabId) === true) n++;
      }
      return n;
    }

    function askDiscard(count: number): Promise<boolean> {
      return useConfirmDialogStore().confirm({
        title: t('workbench.tabStrip.discard.title'),
        message: t('workbench.tabStrip.discard.body', { count }),
        confirmLabel: t('common.action.discard.label'),
        tone: 'danger',
      });
    }

    function othersToClose(id: string): Set<string> {
      const keep = tabsState.tabs.find((t) => t.id === id);
      if (!keep) return new Set();
      const key = host.workspaceKeyOf(keep);
      return new Set(
        tabsState.tabs
          .filter(
            (t) => t.id !== id && host.workspaceKeyOf(t) === key && !host.kinds[t.kind]?.pinned,
          )
          .map((t) => t.id),
      );
    }

    function rightToClose(id: string): Set<string> {
      const target = tabsState.tabs.find((t) => t.id === id);
      if (!target) return new Set();
      const keyTabs = tabsForWorkspace(host.workspaceKeyOf(target));
      const keyIdx = keyTabs.findIndex((t) => t.id === id);
      return new Set(
        keyTabs
          .slice(keyIdx + 1)
          .filter((t) => !host.kinds[t.kind]?.pinned)
          .map((t) => t.id),
      );
    }

    function allToClose(key: K): Set<string> {
      return new Set(
        tabsState.tabs
          .filter((t) => host.workspaceKeyOf(t) === key && !host.kinds[t.kind]?.pinned)
          .map((t) => t.id),
      );
    }

    async function closeTab(id: string): Promise<void> {
      const dirty = dirtyCount([id]);
      if (dirty > 0 && !(await askDiscard(dirty))) return;
      closeTabInternal(id);
      saveNow();
    }

    function dropTabs(ids: readonly string[]): void {
      if (ids.length === 0) return;
      for (const id of ids) closeTabInternal(id);
      saveNow();
    }

    async function closeOthers(id: string): Promise<void> {
      const dirty = dirtyCount(othersToClose(id));
      if (dirty > 0 && !(await askDiscard(dirty))) return;
      const keep = tabsState.tabs.find((t) => t.id === id);
      if (!keep) return;
      const key = host.workspaceKeyOf(keep);
      const closeIds = othersToClose(id);
      for (const tabId of closeIds) {
        dropAllPagesForTab(tabId);
        host.onClosed?.(tabId);
        removeFromPreviewCohort(key, tabId);
      }
      tabsState.tabs = tabsState.tabs.filter((t) => !closeIds.has(t.id));
      // F8: `keep.active = true` alone never clears a surviving pinned tab that was already
      // active -- two tabs then read `active`, both get persisted, and both render `is-active`.
      // `setActiveTabId` clears every other tab in the workspace as it sets this one.
      setActiveTabId(id, key);
      saveNow();
    }

    async function closeToTheRight(id: string): Promise<void> {
      const dirty = dirtyCount(rightToClose(id));
      if (dirty > 0 && !(await askDiscard(dirty))) return;
      const target = tabsState.tabs.find((t) => t.id === id);
      if (!target) return;
      const key = host.workspaceKeyOf(target);
      const closeIds = rightToClose(id);
      for (const tabId of closeIds) {
        dropAllPagesForTab(tabId);
        host.onClosed?.(tabId);
        removeFromPreviewCohort(key, tabId);
      }
      tabsState.tabs = tabsState.tabs.filter((t) => !closeIds.has(t.id));
      const remaining = tabsState.tabs.filter((t) => host.workspaceKeyOf(t) === key);
      if (!remaining.some((t) => t.active)) {
        target.active = true;
        tabsState.activeIdByWorkspace[key] = id;
      }
      saveNow();
    }

    async function closeAll(key: K): Promise<void> {
      const dirty = dirtyCount(allToClose(key));
      if (dirty > 0 && !(await askDiscard(dirty))) return;
      const closeIds = allToClose(key);
      for (const tabId of closeIds) {
        dropAllPagesForTab(tabId);
        host.onClosed?.(tabId);
      }
      tabsState.tabs = tabsState.tabs.filter((t) => !closeIds.has(t.id));
      tabsState.previewIdsByWorkspace[key] = [];
      const remaining = tabsState.tabs.filter((t) => host.workspaceKeyOf(t) === key);
      if (remaining.length === 0) {
        tabsState.activeIdByWorkspace[key] = null;
      } else if (!remaining.some((t) => t.active)) {
        remaining[0].active = true;
        tabsState.activeIdByWorkspace[key] = remaining[0].id;
      }
      saveNow();
    }

    function activateTab(id: string): void {
      const target = tabsState.tabs.find((t) => t.id === id);
      if (!target) return;
      setActiveTabId(id, host.workspaceKeyOf(target));
      saveNow();
    }

    function moveTab(fromId: string, toId: string): void {
      if (fromId === toId) return;
      const tabs = tabsState.tabs;
      const fromIdx = tabs.findIndex((t) => t.id === fromId);
      // The target's index in `tabs`, *before* the dragged tab is spliced out -- looking it up
      // again in the post-splice array (the old code) shifts it left by one whenever the dragged
      // tab sat earlier in the list, so inserting there lands one slot short and a rightward drag
      // never advances past the tab it's hovering.
      const toIdx = tabs.findIndex((t) => t.id === toId);
      if (fromIdx < 0 || toIdx < 0) return;
      const fromTab = tabs[fromIdx];
      const toTab = tabs[toIdx];
      if (host.kinds[fromTab.kind]?.pinned || host.kinds[toTab.kind]?.pinned) return;
      const key = host.workspaceKeyOf(fromTab);
      removeFromPreviewCohort(key, fromId);

      const next = [...tabs];
      const [moved] = next.splice(fromIdx, 1);
      next.splice(toIdx, 0, moved);
      tabsState.tabs = next;
      saveNow();
    }

    function stepTab(key: K, delta: 1 | -1): void {
      const tabs = tabsForWorkspace(key);
      if (tabs.length === 0) return;
      const idx = tabs.findIndex((t) => t.id === tabsState.activeIdByWorkspace[key]);
      const next = tabs[(idx + delta + tabs.length) % tabs.length];
      activateTab(next.id);
    }

    function isPreview(id: string): boolean {
      const tab = tabsState.tabs.find((t) => t.id === id);
      if (!tab) return false;
      return (tabsState.previewIdsByWorkspace[host.workspaceKeyOf(tab)] ?? []).includes(id);
    }

    function promoteTab(id: string): void {
      const tab = tabsState.tabs.find((t) => t.id === id);
      if (!tab) return;
      const key = host.workspaceKeyOf(tab);
      if (!tabsState.previewIdsByWorkspace[key]?.includes(id)) return;
      removeFromPreviewCohort(key, id);
      saveNow();
    }

    function patchChanged<T extends object>(target: T, patch: Partial<T>): boolean {
      for (const key of Object.keys(patch) as (keyof T)[]) {
        if (!Object.is(target[key], patch[key])) return true;
      }
      return false;
    }

    function patchTabState<S extends object>(
      id: string,
      kind: R['kind'],
      patch: Partial<S>,
      opts: { skipUnchanged: boolean },
    ): void {
      const target = tabsState.tabs.find((t) => t.id === id);
      if (target?.kind !== kind) return;
      const state = target.state as S;
      if (opts.skipUnchanged && !patchChanged(state, patch)) return;
      Object.assign(state, patch);
      void saveDebounced();
    }

    // Every tab in workspace `key` — pinned-kind tabs first (in their existing relative order),
    // then the rest, regardless of where each sits in `tabsState.tabs`. Two-pass filter/concat,
    // not a comparator sort — a sort's stability is not a property to lean on here.
    function tabsForWorkspace(key: K): R[] {
      const pinned: R[] = [];
      const rest: R[] = [];
      for (const t of tabsState.tabs) {
        if (host.workspaceKeyOf(t) !== key) continue;
        (host.kinds[t.kind]?.pinned ? pinned : rest).push(t);
      }
      return [...pinned, ...rest];
    }

    const { tabs, activeIdByWorkspace, previewIdsByWorkspace } = toRefs(tabsState);

    const actions: TabsStoreActions<K, R> = {
      tabs,
      activeIdByWorkspace,
      previewIdsByWorkspace,
      hydrateTabs,
      removeFromPreviewCohort,
      evictPreviewCohort,
      openTab,
      duplicateTab,
      dropPageStoresForTab,
      dropAllPagesForTab,
      closeTab,
      dropTabs,
      closeOthers,
      closeToTheRight,
      closeAll,
      activateTab,
      moveTab,
      stepTab,
      isPreview,
      promoteTab,
      patchTabState,
      saveNow,
    };

    const extra = host.extend(actions);

    return {
      ...actions,
      tabsForWorkspace,
      ...extra,
    };
  });
}
