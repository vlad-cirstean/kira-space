import type { CacheStats, CountRequestWire, CountResponse } from '@shared/protocol/dataOps';
import { pageChunks } from '@shared/protocol/page';
import { VueQueryPlugin } from '@tanstack/vue-query';
import { i18n, installCopyTestHook } from '@theme/i18n';
import { bootstrapShell, provideShellServices } from '@workbench/bootstrapShell';
import { windowKey } from '@workbench/bridge/rpc';
import { queryClient } from '@workbench/state/queryClient';
import { createApp } from 'vue';
import App from './App.vue';
import { initApiDataSync } from './api/state/apiQueries';
import { control } from './bridge/control';
import { data } from './bridge/data';
import { useTreeStore } from './project/state/tree';
import { mountProofWindow } from './proof/boot';
import { useAppMetricsStore } from './state/appMetrics';
import { useAppUpdateStore } from './state/appUpdate';
import { useCacheStatsStore } from './state/cacheStats';
import { initConnectionConfigSync } from './state/connectionConfig';
import { useConnectionsStore } from './state/connections';
import { useCustomScriptsStore } from './state/customScripts';
import { useDbMcpStore } from './state/dbmcp';
import { useKeepAwakeStore } from './state/keepAwake';
import { initMaskRulesSync, loadMaskRuleCounts } from './state/maskRules';
import { useOpsStore } from './state/ops';
import { pinia } from './state/pinia';
import { initSavedQueriesSync } from './state/savedQueriesSync';
import { useSchemaColumnsStore } from './state/schemaColumns';
import { initSchemaSync } from './state/schemas';
import { useTabsStore } from './state/tabs';
import { useTerminalsStore } from './state/terminals';
// workbench.css imports @theme/base.css itself now (P104) — importing both here would compile
// base.css as two separate Tailwind roots and double its output.
import '@workbench/workbench.css';
import { useLayoutStore } from './state/layout';
import { useModeStore } from './state/mode';
import { useSettingsStore } from './state/settings';
import { planCount as consolePlanCount } from './views/console/explainResults';
import {
  pageStoreEntries as consolePageStoreEntries,
  totalRetainedBytes as consoleRetainedBytes,
} from './views/console/resultPages';
import { searchState as consoleSearchState } from './views/console/search';
import {
  pageStoreEntries as documentPageStoreEntries,
  totalRetainedBytes as documentRetainedBytes,
} from './views/documents/page';
import { searchState as documentSearchState } from './views/documents/search';
import { pageStoreEntries as gridPageStoreEntries, totalRetainedBytes } from './views/grid/page';
import { searchState as gridSearchState } from './views/grid/search';
import { useDocumentRowsStore } from './views/shared/document/rows';
import {
  pageStoreEntries as keyValuePageStoreEntries,
  totalRetainedBytes as keyValueRetainedBytes,
} from './views/shared/keyvalue/page';
import { searchState as keyValueSearchState } from './views/shared/keyvalue/search';
import {
  type ScrollTraceResult,
  start as startScrollTrace,
  stop as stopScrollTrace,
} from './views/shared/slick/scrollTrace';
import {
  pageStoreEntries as streamPageStoreEntries,
  totalRetainedBytes as streamRetainedBytes,
} from './views/stream/page';
import { useStreamSearchStore } from './views/stream/search';

/** P5 C1: what `window.__kiraRetention` reports for one of the five page stores — the decode/view
 *  caches `__kiraRetainedBytes` cannot see, since that sums `page.byteSize` only (F2). */
interface KiraRetentionStoreStats {
  entries: number;
  pageBytes: number;
  decodeCacheRows: number;
  decodeCacheChars: number;
  viewCacheRows: number;
}

interface KiraRetentionSnapshot {
  stores: {
    grid: KiraRetentionStoreStats;
    documents: KiraRetentionStoreStats;
    keyvalue: KiraRetentionStoreStats;
    stream: KiraRetentionStoreStats;
    console: KiraRetentionStoreStats;
  };
  documentRows: { tabScopes: number; parseCacheRows: number; docNodeCount: number };
  searchMatches: {
    grid: number;
    documents: number;
    keyvalue: number;
    console: number;
    stream: number;
  };
  /** F8/C7: distinct `chunk.data.buffer` identities across every page every store holds, and
   *  their summed byte length — the one figure `totalRetainedBytes()` structurally cannot see,
   *  since a multi-page ExecuteResponse frame shares one ArrayBuffer across every page it carried. */
  frameBuffers: { count: number; bytes: number };
  /** explainResults.ts's own module-level plan store (P12 round 1 finding #10) — entirely
   *  separate from `stores.console` above, which only covers resultPages.ts's page store. */
  explainPlans: number;
}

function storeStats<P extends { byteSize: number }>(
  entries: readonly {
    page: P;
    decodeCacheRows: number;
    decodeCacheChars: number;
    viewCacheRows: number;
  }[],
): KiraRetentionStoreStats {
  let pageBytes = 0;
  let decodeCacheRows = 0;
  let decodeCacheChars = 0;
  let viewCacheRows = 0;
  for (const e of entries) {
    pageBytes += e.page.byteSize;
    decodeCacheRows += e.decodeCacheRows;
    decodeCacheChars += e.decodeCacheChars;
    viewCacheRows += e.viewCacheRows;
  }
  return { entries: entries.length, pageBytes, decodeCacheRows, decodeCacheChars, viewCacheRows };
}

function sumMatches(state: Record<string, { matches: readonly unknown[] }>): number {
  let n = 0;
  for (const entry of Object.values(state)) n += entry.matches.length;
  return n;
}

function frameBufferStats(allEntries: readonly { page: Parameters<typeof pageChunks>[0] }[]): {
  count: number;
  bytes: number;
} {
  const seen = new Set<ArrayBufferLike>();
  let bytes = 0;
  for (const { page } of allEntries) {
    for (const chunk of pageChunks(page)) {
      const buf = chunk.data.buffer;
      if (!seen.has(buf)) {
        seen.add(buf);
        bytes += buf.byteLength;
      }
    }
  }
  return { count: seen.size, bytes };
}

declare global {
  interface Window {
    /**
     * Playwright-only hook (tests/e2e/perf.spec.ts) — the exact retained-bytes figure, so
     * "closing a tab frees its page immediately" can be asserted deterministically instead of
     * read off a flaky RSS sample. Grid-only, kept as-is so that assertion's meaning is unchanged.
     */
    __kiraGridRetainedBytes?: () => number;
    /** D5: the sum across all five page stores — what §2.2's symmetry assertion should see. */
    __kiraRetainedBytes?: () => number;
    /**
     * Playwright-only hooks (tests/e2e/leaks.spec.ts) — the same `data` bridge and tree-state
     * accessor the app itself uses, exposed so a leak regression test can drive many distinct
     * count() requests and read L3's entry count / the tree's live connection ids directly,
     * instead of round-tripping every one of them through real UI clicks.
     */
    __kiraCount?: (req: CountRequestWire) => Promise<CountResponse>;
    __kiraCacheStats?: () => Promise<CacheStats>;
    __kiraTreeConnectionIds?: () => string[];
    /**
     * Playwright-only hook (tests/ui/budgets.spec.ts) — KiraSlickGrid.render() calls this, if a test
     * has set it, at entry, so a scroll-response budget measures the app's render work independent
     * of display refresh rate and SlickGrid's scrollRenderThrottling wait.
     */
    __kiraGridScrollWorkStart?: (t: number) => void;
    /**
     * Playwright-only hook (tests/ui/leaks.spec.ts) — the renderer-retention probe: the
     * decode/view caches, the document parse cache, per-tab search matches and distinct retained
     * frame buffers that `__kiraRetainedBytes` cannot see (that hook sums `page.byteSize` only).
     * Deterministic, engine-independent (no heap API) accounting.
     */
    __kiraRetention?: () => KiraRetentionSnapshot;
    /**
     * A real-fling scroll trace a human drives from DevTools on real hardware (a dev
     * build — View → Open DevTools, internal/shell/menutemplate.go) — NOT a Playwright hook, and
     * not gated in CI. See views/shared/slick/scrollTrace.ts's own header comment and
     * docs/HISTORY.md for the protocol.
     */
    __kiraScrollTrace?: { start: () => void; stop: () => ScrollTraceResult | null };
    /**
     * Runtime tuning for DataGrid.vue's row overscan and per-row memoisation — read from the
     * console so the real-Mac A/B needs one build, not a rebuild per variant. `undefined` on any
     * field means "use the compiled default".
     */
    __kiraGridTuning?: {
      /** Overrides columns.ts's LEAD_FRAMES. */
      leadFramesOverride?: number;
      /** Overrides columns.ts's MAX_LEAD_PX. */
      maxLeadPxOverride?: number;
      /** P22 iter2-scroll-gaps D2: overrides columns.ts's MAX_NEW_CELLS_PER_RENDER — the SlickGrid
       *  engine's per-call new-cell batch cap, read fresh on every `getRenderedRange` call. */
      maxNewCellsPerRenderOverride?: number;
      /** Real-hardware finding: `forceSyncScrolling: true` unconditionally (SlickGridHost.vue)
       *  coupled main-thread render work to every native scroll-event tick during a fling, which
       *  read on real macOS hardware as visible stutter — worse than the incumbent tanstack grid's
       *  own "content lags, motion stays smooth" gap symptom. Default flipped to `false`
       *  (the batch cap alone) pending a real A/B on whether it is the cause; `true` restores the
       *  old unconditional-sync behaviour. Unlike the runway overrides above
       *  (read fresh on every call), this is a SlickGrid construction-time option — read once, at
       *  `new KiraSlickGrid(...)`, not live thereafter. */
      forceSyncScrollingOverride?: boolean;
      /** How long (ms) the viewport must go without a native scroll event
       *  before a self-scheduled catch-up render is allowed to run — overrides columns.ts's
       *  CHASE_QUIET_MS. Read fresh on every chase callback, never cached. `0` restores the pre-fix
       *  "fire on the very next rAF, unconditionally" behaviour exactly, so the real-Mac A/B
       *  (docs/PERF.md) is a console line, not a rebuild. See kiraSlickGrid.ts's own
       *  `scheduleChase` for why the gate is scroll quiescence, not a same-frame-render token. */
      chaseQuietMsOverride?: number;
      /** A per-render cap on *runway* (beyond strictly-visible) growth,
       *  separate from maxNewCellsPerRenderOverride above (which stays the absolute per-pass
       *  ceiling and the floor short-circuit) — overrides columns.ts's
       *  MAX_NEW_LEAD_CELLS_PER_RENDER. Defaulted equal to MAX_NEW_CELLS_PER_RENDER (neutral, no
       *  behaviour change) pending a real-hardware A/B (docs/PERF.md step 4) on the
       *  variance-vs-convergence trade lowering it makes. */
      maxNewLeadCellsPerRenderOverride?: number;
      /** Whether the catch-up render's *per-frame* gate is on — a chase may
       *  only run when no native `scroll` event arrived between the previous animation frame and
       *  this one. It joins `chaseQuietMsOverride`'s wall-clock gate because that one alone cannot
       *  survive a frame longer than its own threshold (24ms against a measured real-Mac p50 of
       *  32.1ms), which is precisely a frame the main thread is already behind on. Defaults to
       *  `true`; `false` A/Bs the frame gate alone, while `chaseQuietMsOverride = 0` still disables
       *  both at once and so keeps its documented "the pre-fix policy exactly" meaning. See
       *  kiraSlickGrid.ts's own `scheduleChase` for the measurement behind it. */
      chaseFrameGateOverride?: boolean;
      /** Whether a render pass samples the viewport's scroll offset itself, at
       *  the moment it needs the velocity, instead of relying on the host's own `scroll` listener —
       *  which SlickGrid's constructor-registered listener always beats to the same event, leaving
       *  every render one sample behind and the *first* render of every fresh gesture reading "at
       *  rest" outright. Defaults to `true` (the fix on); `false` restores the pre-fix behaviour
       *  exactly, so the real-Mac A/B (docs/PERF.md) is a console line, not a rebuild. Read
       *  fresh on every sample, never cached. See SlickGridHost.vue's own `recordOffsetSample` for
       *  the source citations behind the ordering claim. */
      freshVelocitySampleOverride?: boolean;
    };
  }
}
// Every window.__kira* assignment below is Playwright- or DevTools-only (see each
// field's own doc comment above) and must never reach a packaged build — __KIRA_DEBUG_HOOKS__ is
// a vite.config.ts `define`, so a shipped build cannot be talked into enabling it at runtime the
// way the deleted window.__kiraGridEngine once could. scripts/verify-packaging.sh's S6/S7 guard
// this from regressing.
if (__KIRA_DEBUG_HOOKS__) {
  installCopyTestHook();
  window.__kiraScrollTrace = { start: startScrollTrace, stop: stopScrollTrace };
  window.__kiraGridTuning = {};
  window.__kiraGridRetainedBytes = totalRetainedBytes;
  window.__kiraRetainedBytes = () =>
    totalRetainedBytes() +
    consoleRetainedBytes() +
    documentRetainedBytes() +
    keyValueRetainedBytes() +
    streamRetainedBytes();
  window.__kiraRetention = () => {
    const gridEntries = gridPageStoreEntries();
    const documentEntries = documentPageStoreEntries();
    const keyValueEntries = keyValuePageStoreEntries();
    const streamEntries = streamPageStoreEntries();
    const consoleEntries = consolePageStoreEntries();
    return {
      stores: {
        grid: storeStats(gridEntries),
        documents: storeStats(documentEntries),
        keyvalue: storeStats(keyValueEntries),
        stream: storeStats(streamEntries),
        console: storeStats(consoleEntries),
      },
      documentRows: useDocumentRowsStore().retentionSnapshot(),
      searchMatches: {
        grid: sumMatches(gridSearchState),
        documents: sumMatches(documentSearchState),
        keyvalue: sumMatches(keyValueSearchState),
        console: sumMatches(consoleSearchState),
        stream: sumMatches(useStreamSearchStore().searchState),
      },
      frameBuffers: frameBufferStats([
        ...gridEntries,
        ...documentEntries,
        ...keyValueEntries,
        ...streamEntries,
        ...consoleEntries,
      ]),
      explainPlans: consolePlanCount(),
    };
  };
  window.__kiraCount = data.count;
  window.__kiraCacheStats = data.cacheStats;
  window.__kiraTreeConnectionIds = () => Array.from(useTreeStore().knownConnectionIds());
}

async function mountShell(): Promise<void> {
  // P112: live before any query exists, whether or not the Api panel ever mounts — needs no data,
  // so it runs synchronously before the Promise.all below rather than joining it.
  initApiDataSync();
  initConnectionConfigSync();
  // Every store used here runs before app.use(pinia) below, so each needs the module-level
  // `pinia` instance passed explicitly (Pinia has no active instance yet at this point).
  const cacheStatsStore = useCacheStatsStore(pinia);
  const appMetricsStore = useAppMetricsStore(pinia);
  const modeStore = useModeStore(pinia);
  const customScriptsStore = useCustomScriptsStore(pinia);
  const dbMcpStore = useDbMcpStore(pinia);
  const keepAwakeStore = useKeepAwakeStore(pinia);
  const opsStore = useOpsStore(pinia);
  const layoutStore = useLayoutStore(pinia);
  const connectionsStore = useConnectionsStore(pinia);
  const tabsStore = useTabsStore(pinia);
  const settingsStore = useSettingsStore(pinia);
  const terminalsStore = useTerminalsStore(pinia);
  const treeStore = useTreeStore(pinia);
  const schemaColumnsStore = useSchemaColumnsStore(pinia);

  // These three used to subscribe only from ProjectTree.vue's own onMounted —
  // a window booted with the project panel hidden (API/Terminal mode, or per-mode panel gating)
  // or with zero connections yet never got them for the whole session. onSchemaChanged and
  // onConnectionMetadataInvalidated then never arrived, so a restored console tab's completion,
  // lint and hover served stale schema/columns, and a deleted connection's DDL cache never got
  // cleaned up. Same precedent as initApiDataSync above: live before the project panel ever
  // mounts, whether or not it mounts this session. Each is its own idempotent
  // unsubscribe-then-resubscribe, so calling this instead of (rather than in addition to)
  // ProjectTree.vue's removed onMounted call changes nothing about their own behavior.
  treeStore.initTreeSync();
  initSchemaSync();
  schemaColumnsStore.initSchemaColumnsSync();
  // P108 Part 12 F18: initSchemaSync's own precedent above — live before any Privacy tab, header
  // menu or grid preview ever mounts, whether or not one does this session.
  initMaskRulesSync();
  initSavedQueriesSync();

  cacheStatsStore.initCacheStats();
  appMetricsStore.initAppMetrics();
  // Must complete before anything window-scoped below (hydrateTabs, in particular) —
  // always a no-op on the native shell, the only registration a `-tags server` browser tab ever
  // gets otherwise. Also this window's own persisted mode — set once before the first
  // render, the same way hydrateLayout/hydrateSettings below hydrate their own state.
  modeStore.hydrateMode(await control.windowsEnsure());
  await Promise.all([
    layoutStore.hydrateLayout(),
    settingsStore.hydrateSettings(),
    connectionsStore.hydrateConnections(),
    // M5 §7.5/§6.2: every connection's own masked-column count — the Settings glance's data, and
    // the toolbar's own "does this connection have any masked columns at all" visibility check
    // (deleteRowTooltip's own standing rule: a permanently inert control is worse than no control).
    loadMaskRuleCounts(),
    customScriptsStore.hydrateCustomScripts(),
    terminalsStore.hydrateTerminalDefaults(),
    dbMcpStore.hydrateDbMcp(),
    dbMcpStore.hydrateDbMcpApprovals(),
    keepAwakeStore.initKeepAwake(),
    opsStore.hydrateOps(),
    tabsStore.hydrateTabs(),
  ]);
  const app = createApp(App);
  app.use(i18n);
  app.use(pinia);
  provideShellServices(app);
  app.use(VueQueryPlugin, { queryClient });
  app.mount('#app');
  // Off the boot critical path (Promise.all above) — an update check gains nothing from blocking
  // first paint, and Go's own 6h cache floor (§3.3) decides what actually fetches.
  useAppUpdateStore().initAppUpdate();
}

// mountShell's own hydrates (modeStore.hydrateMode's windowsEnsure call and every
// entry in the Promise.all above) can reject — a DB error, or a busy DB. The catch-and-
// retry wrapper itself moved to workbench/bootstrapShell.ts, shared with apps/kira-space's own
// identical copy.
void bootstrapShell(windowKey.startsWith('proof-') ? mountProofWindow : mountShell, 'Kira Studio');
