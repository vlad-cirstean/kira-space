import { kibanaConsoleStateSchema } from '@shared/domain/kibana';
import type { AppMode } from '@shared/domain/mode';
import { decodePath } from '@shared/domain/tree';
import { createTabsStore } from '@workbench/state/createTabsStore';
import { defineStore } from 'pinia';
import { reactive, toRefs } from 'vue';
import { control } from '../bridge/control';
import { usePendingChangesStore } from '../views/grid/pendingChanges';
import { useCellSelectionStore } from './cellSelection';
import { useConnectionsStore } from './connections';
import { useConsoleDefaultsStore } from './consoleDefaults';
import { useModeStore, workspaceKeyOf } from './mode';
import { pinia } from './pinia';
import { useSettingsStore } from './settings';
import {
  asBrowseTab,
  asConsoleTab,
  asDataTab,
  asDocumentTab,
  asKeyValueTab,
  asStreamTab,
  type BrowseTabRecord,
  type BrowseTabState,
  type ConsoleTabRecord,
  type ConsoleTabState,
  type DataTabRecord,
  type DataTabState,
  type DefinitionTabState,
  type DocumentTabRecord,
  type DocumentTabState,
  defaultBrowseTabState,
  defaultConsoleTabState,
  defaultDataTabState,
  defaultDefinitionTabState,
  defaultDocumentTabState,
  defaultKeyValueTabState,
  defaultStreamTabState,
  type KeyValueTabRecord,
  type KeyValueTabState,
  STUDIO_TAB_KIND_MODE,
  type StreamTabRecord,
  type StreamTabState,
  type StudioTabKind,
} from './tabDomain';
import { useTabIncognitoStore } from './tabIncognito';
import { TAB_KINDS } from './tabKinds';

// The shared skeleton (persistableTabs/saveIfChanged/openTab/closeTab/…, see
// that file's own header) now lives in packages/workbench/src/state/createTabsStore.ts. This file
// is what's left: per-kind typed sugar (openDataTab, openDocumentTab-shaped patchers, …) and
// every Studio-only feature — incognito filtering (persistable), the `hydrated` reconnect gate
// (module-local now, no longer part of the store's own reactive state — a genuinely separate
// concern, CLAUDE.md's one-store-one-concern rule), and the
// `control.onConnectionsChanged`/`onConnectionState` listeners — composed onto the factory-made
// store through `onOpened`/`onClosed`/`onDuplicated`/`extend` rather than the factory needing to
// know any of it exists.

export interface RecentTableEntry {
  connectionId: string;
  path: string;
  kind: 'data' | 'document' | 'keyvalue' | 'stream';
  openedAt: number;
}

const RECENT_TABLES_LIMIT = 20;

// Kinds whose fetched page survives a connection drop (read-only behind StaleDataStrip).
const KEEPS_DATA_ON_DROP = new Set<StudioTabKind>([
  'data',
  'document',
  'keyvalue',
  'stream',
  'console',
]);

// P99 §5.2: split out of tabs — a genuinely separate concern (the P16 design system's Empty.html
// "Recent tables" list) from the tab collection/activation store below, with exactly one external
// reader (StudioStart.vue) and one writer (openTab, in useTabsStore).
export const useRecentTablesStore = defineStore('recentTables', () => {
  // In-memory only, like the hydrated-reconnect-gate set below — resets on relaunch rather than
  // adding a new storage table for tab-open history.
  const state = reactive({ entries: [] as RecentTableEntry[] });

  function recordRecent(connectionId: string, path: string, kind: RecentTableEntry['kind']): void {
    const withoutThis = state.entries.filter(
      (e) => !(e.connectionId === connectionId && e.path === path && e.kind === kind),
    );
    withoutThis.unshift({ connectionId, path, kind, openedAt: Date.now() });
    state.entries = withoutThis.slice(0, RECENT_TABLES_LIMIT);
  }

  // A deleted connection's entry would reopen a tab whose row can never be saved (FK).
  function pruneRecent(liveConnectionIds: ReadonlySet<string>): void {
    state.entries = state.entries.filter((e) => liveConnectionIds.has(e.connectionId));
  }

  return { ...toRefs(state), recordRecent, pruneRecent };
});

// Result of an open*Tab call (`null` when the connection is gone): `reused` tells the caller
// whether an existing tab was activated rather than a fresh one created — a fresh tab is about to
// fetch on mount anyway, so only a caller that cares about the double-click "also reload the data"
// behavior needs to check this; everyone else can destructure just `id` and ignore it.
export interface OpenTabResult {
  id: string;
  reused: boolean;
}

// "not yet loaded, shows Reconnect & load" — in memory only, like useRecentTablesStore's own
// entries above; a restored session comes back with every tab in this state (hydrateTabs never
// adds to it), and reconnecting or reopening a tab against a live connection is what clears it
// (onOpened below). Module-local rather than part of tabsState (createTabsStore's own reactive
// state) — this concept has no equivalent in Kira Space at all (no live connection to lose), so it
// stays entirely on this side of the factory boundary.
const hydratedIds = reactive(new Set<string>());

function markHydrated(id: string): void {
  hydratedIds.add(id);
}

// A read that comes back E_ENGINE_DOWN/E_CONNECT means the adapter is gone — flip the tab back to
// the Reconnect & load affordance rather than showing a red error (views/grid/state.ts's load()).
function unmarkHydrated(id: string): void {
  hydratedIds.delete(id);
}

function isHydrated(id: string): boolean {
  return hydratedIds.has(id);
}

// Cross-view state (§11): tabs are read by the tab strip, the toolbar, the main view and the
// operations panel, none of which may reach into each other — hence renderer/state/, not
// workbench/state/ or views/grid/.
export const useTabsStore = createTabsStore({
  kinds: TAB_KINDS,
  workspaceKeyOf,
  control,
  fallbackWorkspaceKey: (kind) => STUDIO_TAB_KIND_MODE[kind] as AppMode,
  // 'studio'/'api' are always seeded (even at zero tabs, matching the old per-mode behavior
  // exactly) — this app's own two fixed modes.
  seedWorkspaceKeys: ['studio', 'api'],
  // P71 §3.1: an incognito tab is never written — left out of the snapshot entirely, and
  // TabsService.Save replaces the window's whole tab set, so a tab switched to incognito
  // mid-session also drops whatever row it already had, with no separate delete call needed.
  persistable: (t) =>
    !useTabIncognitoStore().isIncognito(t.id) && t.kind !== 'terminal' && t.kind !== 'script-run',
  onOpened(record, reused) {
    if (!reused) {
      markHydrated(record.id);
      return;
    }
    // Reopening (double-click, "recent tables", …) against a connection that's live right now
    // reads as "load this" just as much as a brand-new tab does — without this, a tab left
    // unhydrated by an earlier disconnect (or never hydrated after a session restore) stays stuck
    // behind the reconnect gate until its own button is clicked, even though the very re-open
    // that just happened proves the connection needs no reconnecting at all.
    if (
      record.connectionId &&
      useConnectionsStore().states[record.connectionId]?.status === 'connected'
    ) {
      markHydrated(record.id);
    }
  },
  onClosed(tabId) {
    unmarkHydrated(tabId);
    useCellSelectionStore().clearSelectedCellFor(tabId);
    useCellSelectionStore().clearSelectedCellFor(`${tabId}::preview`);
    usePendingChangesStore().clearPending(tabId);
  },
  // P71 §3.1: duplicating an incognito tab to try a variant must not silently start persisting it
  // — the copy carries the flag too.
  onDuplicated(source, duplicate) {
    if (useTabIncognitoStore().isIncognito(source.id)) {
      useTabIncognitoStore().setIncognito(duplicate.id, true);
    }
  },
  extend(actions) {
    const recentTablesStore = useRecentTablesStore(pinia);
    const tabIncognitoStore = useTabIncognitoStore(pinia);

    // main's `tabs.connection_id` is ON DELETE CASCADE, so a deleted connection's `tabs` rows
    // are already gone server-side — a tab this store still holds for it is a row that can never
    // be re-inserted (every later debounced save would throw FOREIGN KEY constraint failed and get
    // silently discarded). Closing skips the discard prompt (the connection is gone; a declined or
    // superseded prompt would keep the orphan) but shares closeTabInternal with a manual close, so
    // pages and runtime are freed by one code path.
    control.onConnectionsChanged((records) => {
      const liveIds = new Set(records.map((r) => r.id));
      const stale = actions.tabs.value
        .filter((t) => t.connectionId && !liveIds.has(t.connectionId))
        .map((t) => t.id);
      actions.dropTabs(stale);
      recentTablesStore.pruneRecent(liveIds);
    });

    // An explicit Disconnect (or a lost connection surfacing as 'error') never used to
    // touch the hydrated set at all — only a failed *load* did, so a tab kept rendering its
    // pre-disconnect rows until the moment something happened to try reading it again. Regating
    // here puts every open tab of the connection behind the Reconnect gate the instant the
    // connection itself says it is gone, matching what the engine already did with its own cache.
    // Page bytes are freed except for KEEPS_DATA_ON_DROP kinds, which keep showing them read-only
    // behind StaleDataStrip. The runtime record (count, selection, find toolbar, actionError)
    // stays, so the tab that comes back on reconnect is still the same tab, not a blank one.
    control.onConnectionState((state) => {
      if (state.status !== 'disconnected' && state.status !== 'error') return;
      for (const t of actions.tabs.value) {
        if (t.connectionId !== state.connectionId) continue;
        unmarkHydrated(t.id);
        if (!KEEPS_DATA_ON_DROP.has(t.kind)) actions.dropPageStoresForTab(t.id);
      }
    });

    // Turning incognito on flushes the tab's existing row immediately, rather than at
    // whatever unrelated state change saves next — tabIncognito.ts cannot call saveNow directly
    // (it would recreate the cycle its own module comment avoids), so it publishes the toggle here
    // instead. Saves unconditionally, not only when turning
    // incognito ON — setIncognito (tabIncognito.ts) already updates incognitoState.ids before
    // firing this listener, so persistableTabs()'s own isIncognito filter already sees the new
    // state either direction; an on-only guard would leave a tab switched back to normal
    // unpersisted until some unrelated save.
    tabIncognitoStore.registerIncognitoSetListener((_tabId, _on) => {
      actions.saveNow();
    });

    // The six Studio openers below shared this exact sequence — find-existing-and-
    // activate (opt-in per caller via `reuse`), else create-and-push-and-activate, then an opt-in
    // recordRecent — differing only in which of those two opt-ins applied and which kind/state
    // constructor built the record. `recentKind` used to be handled inline inside the shared
    // `openTab`; the factory no longer knows the concept exists, so each wrapper below calls
    // `recordRecent` itself, gated on `!result.reused` — exactly the old "only a freshly created
    // tab records" behavior (reuseExistingTab's own branch never touched recentKind either).

    // P107 iter2 I2-43: openDataTab/openDocumentTab/openKeyValueTab/openStreamTab shared this exact
    // body — open-or-reuse via actions.openTab, then recordRecent on a freshly created tab (never
    // a reused one) — differing only in tab kind and default-state factory.
    function openTrackedTab<S>(
      kind: RecentTableEntry['kind'],
      connectionId: string,
      path: string,
      defaultState: () => S,
      opts: { newTab?: boolean } | undefined,
    ): OpenTabResult | null {
      // A tab for a deleted connection violates the tabs FK and fails every later save.
      if (!useConnectionsStore(pinia).connectionRecord(connectionId)) return null;
      const result = actions.openTab(kind, connectionId, path, defaultState, {
        reuse: !opts?.newTab,
      });
      if (!result.reused) recentTablesStore.recordRecent(connectionId, path, kind);
      return result;
    }

    // Without `newTab`, activates an existing tab for the same (connectionId, path) if one exists
    // (§8.10's "Open data"). "Open data in new tab" always creates (`newTab: true`), so the same
    // table can be open N times with independent state — identity is `id`, never `path` (§8.4).
    function openDataTab(
      connectionId: string,
      path: string,
      opts?: { newTab?: boolean },
    ): OpenTabResult | null {
      return openTrackedTab(
        'data',
        connectionId,
        path,
        () => defaultDataTabState(useSettingsStore().data.defaultPageSize),
        opts,
      );
    }

    // Opens a 'definition' tab, reusing an existing one for the same (connectionId, path) — mirrors
    // openDataTab's identity rule (§8.4), minus the `newTab` escape hatch: D14 gives the definition
    // view no "open in new tab" affordance.
    function openDefinitionTab(connectionId: string, path: string): string {
      return actions.openTab('definition', connectionId, path, () => defaultDefinitionTabState(), {
        reuse: true,
      }).id;
    }

    // Opens a new 'console' tab — always a fresh one, never reused by (connectionId, path): unlike
    // data/definition, a console is a scratch work surface (like a SQL client's "New Query"), so
    // the same target routinely wants several independent consoles open at once.
    //
    // Opened at the bare connection root (`path === ''`), a Postgres console has no session-
    // level way to redirect itself to a non-primary database — substituting a remembered "Set as
    // default" path here, before the path ever reaches the engine, needs no adapter change at all.
    function openConsoleTab(connectionId: string, path: string): string {
      const effectivePath =
        path === '' ? (useConsoleDefaultsStore().consoleDefaultFor(connectionId) ?? path) : path;
      return actions.openTab(
        'console',
        connectionId,
        effectivePath,
        () => defaultConsoleTabState(),
        {
          reuse: false,
        },
      ).id;
    }

    // Opens a kibana Discover tab: a console tab at the data view's path carrying `kibana` state.
    // Reuses the tab for the same data view unless `newTab`; the root path ('') always opens fresh.
    function openDiscoverTab(
      connectionId: string,
      path: string,
      opts?: { newTab?: boolean; title?: string },
    ): OpenTabResult | null {
      if (!useConnectionsStore(pinia).connectionRecord(connectionId)) return null;
      const dataViewId = decodePath(connectionId, path).segments[0]?.name ?? null;
      return actions.openTab(
        'console',
        connectionId,
        path,
        () => ({
          ...defaultConsoleTabState(),
          kibana: kibanaConsoleStateSchema.parse({ dataViewId, dataViewName: opts?.title ?? null }),
        }),
        { reuse: !opts?.newTab && path !== '' },
      );
    }

    // Opens a 'document' tab, reusing an existing one for the same (connectionId, path) — mirrors
    // openDataTab's identity rule (§8.4); `newTab` opens a fresh one regardless.
    function openDocumentTab(
      connectionId: string,
      path: string,
      opts?: { newTab?: boolean },
    ): OpenTabResult | null {
      return openTrackedTab(
        'document',
        connectionId,
        path,
        () => defaultDocumentTabState(useSettingsStore().data.defaultPageSize),
        opts,
      );
    }

    // Opens a 'keyvalue' tab, reusing an existing one for the same (connectionId, path) — mirrors
    // openDataTab's identity rule (§8.4); `newTab` opens a fresh one regardless.
    function openKeyValueTab(
      connectionId: string,
      path: string,
      opts?: { newTab?: boolean },
    ): OpenTabResult | null {
      return openTrackedTab(
        'keyvalue',
        connectionId,
        path,
        () => defaultKeyValueTabState(useSettingsStore().data.defaultPageSize),
        opts,
      );
    }

    // Opens a 'stream' tab, reusing an existing one for the same (connectionId, path) — mirrors
    // openDataTab's identity rule (§8.4); `newTab` opens a fresh one regardless.
    function openStreamTab(
      connectionId: string,
      path: string,
      opts?: { newTab?: boolean },
    ): OpenTabResult | null {
      return openTrackedTab(
        'stream',
        connectionId,
        path,
        () => defaultStreamTabState(useSettingsStore().data.defaultPageSize),
        opts,
      );
    }

    // Opens a 'browse' tab over a redis database / s3 bucket's key or object space —
    // identity is the container (`path`), reused across "Browse keys"/"Browse objects" invocations
    // on the same container the same way openDataTab reuses a table's tab; `newTab` opens a fresh
    // one regardless, with its own independent levelPath.
    function openBrowseTab(
      connectionId: string,
      path: string,
      opts?: { newTab?: boolean },
    ): OpenTabResult {
      return actions.openTab('browse', connectionId, path, () => defaultBrowseTabState(), {
        reuse: !opts?.newTab,
      });
    }

    function patchDataTabState(id: string, patch: Partial<DataTabState>): void {
      actions.patchTabState(id, 'data', patch, { skipUnchanged: true });
    }

    function patchConsoleTabState(id: string, patch: Partial<ConsoleTabState>): void {
      actions.patchTabState(id, 'console', patch, { skipUnchanged: true });
    }

    function patchDefinitionTabState(id: string, patch: Partial<DefinitionTabState>): void {
      actions.patchTabState(id, 'definition', patch, { skipUnchanged: true });
    }

    function patchDocumentTabState(id: string, patch: Partial<DocumentTabState>): void {
      actions.patchTabState(id, 'document', patch, { skipUnchanged: false });
    }

    function patchKeyValueTabState(id: string, patch: Partial<KeyValueTabState>): void {
      actions.patchTabState(id, 'keyvalue', patch, { skipUnchanged: false });
    }

    function patchStreamTabState(id: string, patch: Partial<StreamTabState>): void {
      actions.patchTabState(id, 'stream', patch, { skipUnchanged: false });
    }

    // P41: mirrors data/console/definition's skipUnchanged: true — descending/ascending to the
    // level a tab is already showing (e.g. a duplicate reload) must not schedule a save.
    function patchBrowseTabState(id: string, patch: Partial<BrowseTabState>): void {
      actions.patchTabState(id, 'browse', patch, { skipUnchanged: true });
    }

    function findDataTab(id: string): DataTabRecord | null {
      return asDataTab(actions.tabs.value.find((t) => t.id === id));
    }

    function findConsoleTab(id: string): ConsoleTabRecord | null {
      return asConsoleTab(actions.tabs.value.find((t) => t.id === id));
    }

    function findDocumentTab(id: string): DocumentTabRecord | null {
      return asDocumentTab(actions.tabs.value.find((t) => t.id === id));
    }

    function findKeyValueTab(id: string): KeyValueTabRecord | null {
      return asKeyValueTab(actions.tabs.value.find((t) => t.id === id));
    }

    function findStreamTab(id: string): StreamTabRecord | null {
      return asStreamTab(actions.tabs.value.find((t) => t.id === id));
    }

    function findBrowseTab(id: string): BrowseTabRecord | null {
      return asBrowseTab(actions.tabs.value.find((t) => t.id === id));
    }

    // D11: Control+Tab / Control+Shift+Tab — wraps around at either end, matching the tab strip's
    // own left-to-right visual order, scoped to the current workspace's own tabs (D5, generalised).
    function activateNextTab(): void {
      actions.stepTab(useModeStore().active, 1);
    }

    function activatePrevTab(): void {
      actions.stepTab(useModeStore().active, -1);
    }

    return {
      markHydrated,
      unmarkHydrated,
      isHydrated,
      openDataTab,
      openDefinitionTab,
      openConsoleTab,
      openDiscoverTab,
      openDocumentTab,
      openKeyValueTab,
      openStreamTab,
      openBrowseTab,
      patchDataTabState,
      patchConsoleTabState,
      patchDefinitionTabState,
      patchDocumentTabState,
      patchKeyValueTabState,
      patchStreamTabState,
      patchBrowseTabState,
      findDataTab,
      findConsoleTab,
      findDocumentTab,
      findKeyValueTab,
      findStreamTab,
      findBrowseTab,
      activateNextTab,
      activatePrevTab,
    };
  },
});
