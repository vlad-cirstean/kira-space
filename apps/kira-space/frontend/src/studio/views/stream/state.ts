import { encodeKafkaStreamFilter, encodeSqsStreamFilter } from '@shared/domain/streamFilter';
import type { PageSize } from '@shared/domain/tabs';
import type { PageCursor } from '@shared/protocol/dataOps';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { pinia } from '../../../state/pinia';
import type { StreamTabRecord } from '../../../state/tabDomain';
import { useTabsStore } from '../../../state/tabs';
import { control } from '../../bridge/control';
import { data } from '../../bridge/data';
import { useConnectionsStore } from '../../state/connections';
import { useStaleDataStore } from '../../state/staleData';
import { registerTabReload } from '../../state/viewCommands';
import { runPagedLoad } from '../shared/page/load';
import {
  beginOp,
  createRuntimeStore,
  defaultPagedRuntime,
  type PagedViewRuntime,
  runPagedCount,
  stopOp,
} from '../shared/viewOp';
import { drop, setPage } from './page';
import { useStreamFilterHistoryStore } from './streamFilterHistory';

// SQS's 'batch' pagination must never be re-read except on an explicit Poll press —
// every read is a real ReceiveMessage against the live queue. StreamView.vue's own isBatch
// computed reads this same field off caps; mirrored here (rather than imported) because state.ts
// has no dependency on the view layer.
function isBatchPagination(connectionId: string): boolean {
  return useConnectionsStore().states[connectionId]?.caps?.pagination === 'batch';
}

// Mirrors views/keyvalue/state.ts's KeyValueViewRuntime shape, minus pageIndex (StreamTabState
// is deliberately empty — offsetWindow is always token-driven, batch has no position
// at all) plus `polled`: runtime-only (never persisted) tracking of whether this tab has loaded
// at least once, so SQS's view (caps.pagination === 'batch') can show a "click Poll" placeholder
// until the user explicitly asks for a page (never auto-loaded).
interface StreamViewRuntime extends PagedViewRuntime {
  rowCount: number;
  visibilityTimeoutSeconds: number | null;
  polled: boolean;
  /** Read-only SQS: the user confirmed the receive-count effect of polling; runtime-only. */
  receiveAcknowledged: boolean;
  /** The row last clicked, for the cell-editor preview (item 6) and — for SQS — Delete message's
   *  target; `null` once the page reloads out from under it (see stream/page.ts's pageVersion). */
  selectedRow: number | null;
  /** Unapplied filter-row input; `null` shows the committed value. Survives a tab switch (P375). */
  offsetDraft: string | null;
  timestampDraft: string | null;
  partitionsDraft: number[] | null;
}

function defaultRuntime(): StreamViewRuntime {
  return {
    ...defaultPagedRuntime(),
    rowCount: 0,
    visibilityTimeoutSeconds: null,
    polled: false,
    receiveAcknowledged: false,
    selectedRow: null,
    offsetDraft: null,
    timestampDraft: null,
    partitionsDraft: null,
  };
}

// currentStreamFilter is load()'s own filter encoding, factored out so runCount can send the *same*
// filter a browse under this tab is scoped to, rather than re-deriving it. Kafka-only in effect;
// always null for SQS, since StreamView.vue never lets an SQS tab's three filter fields become
// non-null in the first place — encodeKafkaStreamFilter itself would still collapse them to null
// even if it did.
function currentStreamFilter(tab: StreamTabRecord): string | null {
  // Date.parse returns NaN for junk, and isEmptyKafkaStreamFilter's own `!== null`
  // check doesn't catch it — a NaN would silently ride through encodeKafkaStreamFilter (JSON.
  // stringify turns it into `null` on the wire, so the engine reads "no timestamp filter" while
  // the field looks applied). StreamView.vue validates before ever calling load(), but this guard
  // makes the wire payload honest regardless of caller.
  const parsedTimestampMs =
    tab.state.timestampFilter === null ? null : Date.parse(tab.state.timestampFilter);
  return encodeKafkaStreamFilter({
    offset: tab.state.offsetFilter,
    partitions: tab.state.partitions,
    timestampMs: Number.isNaN(parsedTimestampMs) ? null : parsedTimestampMs,
  });
}

// A read-only batch (SQS) poll carries its hide time in the filter; every other read keeps the
// Kafka-only encoding. Reads caps and the record's readOnly, never the connection kind.
function readFilter(tab: StreamTabRecord): string | null {
  const connectionId = tab.connectionId;
  if (
    connectionId &&
    isBatchPagination(connectionId) &&
    useConnectionsStore().connectionRecord(connectionId)?.readOnly
  ) {
    return encodeSqsStreamFilter({
      visibilityTimeoutSeconds: tab.state.sqsVisibilityTimeoutSeconds,
    });
  }
  return currentStreamFilter(tab);
}

export interface StreamFilterInput {
  offset: string | null;
  partitions: number[];
  timestamp: string | null;
}

export const useStreamViewStore = defineStore('streamView', () => {
  const { runtime, ensureRuntime, setActionError, toggleSearchOpen, setSearchOpen } =
    createRuntimeStore<StreamViewRuntime>(defaultRuntime);

  // D4: closeTab has no way to import this leaf module directly (reality 18) — registers here.
  registerTabRuntimeCleanup((tabId) => {
    delete runtime[tabId];
  });

  // Adopts shared/page/load.ts's runPagedLoad frame, which documents/grid/keyvalue's own load()
  // already use — this view's supersede/tab-closed/kind-check/applyLoadFailure tail was the exact
  // same shape load.ts's own note flagged as a candidate, never migrated.
  async function load(tabId: string, cursor?: PageCursor): Promise<void> {
    const tab = useTabsStore().findStreamTab(tabId);
    if (!tab?.connectionId) return;
    const rt = ensureRuntime(tabId);
    const effectiveCursor: PageCursor = cursor ?? { mode: 'offset', offset: 0 };
    const opId = beginOp(rt);
    rt.polled = true;

    const filter = readFilter(tab);
    const connectionId = tab.connectionId;

    await runPagedLoad({
      rt,
      opId,
      stillMounted: () => Boolean(runtime[tabId]),
      read: () =>
        data.read({
          opId,
          tabId,
          connectionId,
          path: tab.path,
          projection: null,
          filter,
          sort: null,
          pageSize: tab.state.pageSize,
          cursor: effectiveCursor,
        }),
      expectKind: 'stream',
      id: tabId,
      tabNoun: 'stream tab',
      apply: (page) => {
        setPage(tabId, page);
        useStaleDataStore().stamp(tabId, connectionId);
        rt.status = 'idle';
        rt.opId = null;
        rt.rowCount = page.rowCount;
        rt.hasMore = page.position.hasMore;
        rt.nextToken = page.position.nextToken;
        rt.visibilityTimeoutSeconds = page.visibilityTimeoutSeconds;
        rt.selectedRow = null; // a fresh page invalidates whatever row index used to be selected
      },
    });
  }

  async function reload(tabId: string): Promise<void> {
    const tab = useTabsStore().findStreamTab(tabId);
    if (!tab?.connectionId) return;
    await data.invalidate(tab.connectionId, tab.path);
    // reload() is reached from three paths that are not the
    // explicit Poll button — a Send/Delete-message mutation's own reload-self, that same
    // mutation's fan-out to sibling tabs on the same queue (reloadTabsForTarget), and a
    // project-tree double-click on an already-open tab (ProjectTree.vue's `reused` branch). None
    // of those is the user asking to poll, so for a batch (SQS) tab this must behave like a
    // no-op read rather than a real ReceiveMessage: drop the now-invalidated page and return the
    // tab to its "click Poll" placeholder instead of loading a fresh one.
    if (isBatchPagination(tab.connectionId)) {
      const rt = ensureRuntime(tabId);
      drop(tabId);
      rt.polled = false;
      rt.rowCount = 0;
      rt.hasMore = false;
      rt.nextToken = null;
      rt.selectedRow = null;
      return;
    }
    await load(tabId);
  }

  async function runCount(tabId: string): Promise<void> {
    const tab = useTabsStore().findStreamTab(tabId);
    if (!tab?.connectionId) return;
    const connectionId = tab.connectionId;
    const rt = ensureRuntime(tabId);
    // This used to hard-code `filter: null`, so the Σ readout
    // answered a different question than the rows beside it — the high-low watermark summed across
    // *every* partition, printed next to a page that load() had already scoped to the selected
    // partition/offset/timestamp filter. Sending the same encoded filter here scopes the count
    // identically (kafka/count.go's countTopic now shares load's own freshWindows, so "N total"
    // and the browse agree on what they are counting).
    await runPagedCount(rt, (refresh) =>
      data.count({
        tabId,
        connectionId,
        path: tab.path,
        filter: currentStreamFilter(tab),
        refresh,
      }),
    );
  }

  function stop(tabId: string): void {
    stopOp(runtime[tabId]);
  }

  // SQS's toolbar calls this directly from an explicit "Poll" click — same operation as
  // `load`, named separately so the view never has to explain why a batch-strategy tab "loads".
  //
  // SQS has no addressable position (read.go's own comment: "every poll is an independent,
  // non-resumable snapshot"), so every poll's data:read carries the identical connectionId/path/
  // pageSize/cursor — the exact same enginecache.PageCacheKey as the poll before it. Without
  // invalidating first, a second Poll click would silently be served the first poll's cached page
  // and never reach the adapter's real ReceiveMessage call — same fix as reload()'s own invalidate,
  // same default scope (ipcfixture/sqs_test.go already recorded this exact call for this exact
  // scenario, anticipating this fix).
  function acknowledgeReceive(tabId: string): void {
    ensureRuntime(tabId).receiveAcknowledged = true;
  }

  // A new hide time changes what the confirm names, so the next Poll asks again.
  function setPollVisibilityTimeout(tabId: string, seconds: number): void {
    useTabsStore().patchStreamTabState(tabId, { sqsVisibilityTimeoutSeconds: seconds });
    ensureRuntime(tabId).receiveAcknowledged = false;
  }

  // The first confirm runs before any page exists, so read the redrive limit off the queue's
  // attributes (the same Definition call the definition tab makes). 'none' means the definition
  // loaded with no RedrivePolicy; null means it could not be read.
  async function fetchRedriveLimit(tabId: string): Promise<number | 'none' | null> {
    const tab = useTabsStore().findStreamTab(tabId);
    if (!tab?.connectionId) return null;
    try {
      const { definition } = await control.treeDefinition(tab.connectionId, tab.path, false, tabId);
      const row = definition.sections
        .flatMap((section) => section.rows)
        .find((r) => r.name === 'RedrivePolicy');
      if (!row) return 'none';
      const limit = Number(JSON.parse(row.value).maxReceiveCount);
      return Number.isInteger(limit) ? limit : null;
    } catch {
      return null;
    }
  }

  async function poll(tabId: string): Promise<void> {
    const tab = useTabsStore().findStreamTab(tabId);
    if (!tab?.connectionId) return;
    await data.invalidate(tab.connectionId, tab.path);
    await load(tabId);
  }

  // Kafka's offsetWindow strategy is always token-driven (no plain-offset fallback — a browse
  // tab has no addressable position to go back to, per the ground rules' forward-only browsing).
  async function goNext(tabId: string): Promise<void> {
    const rt = runtime[tabId];
    if (!rt?.nextToken) return;
    await load(tabId, { mode: 'after', token: rt.nextToken });
  }

  // Item 1: mirrors grid/state.ts's/keyvalue/state.ts's own setPageSize — reset whatever
  // continuation token was held for the old size (never valid against a different one), persist the
  // new size, and start over from the top. SQS's `batch` pagination has no continuation to reset and
  // is never auto-loaded (D10/D12) — changing the size there just takes effect on the next Poll.
  async function setPageSize(tabId: string, pageSize: PageSize): Promise<void> {
    const tabsStore = useTabsStore();
    const tab = tabsStore.findStreamTab(tabId);
    if (!tab) return;
    const rt = ensureRuntime(tabId);
    rt.nextToken = null;
    tabsStore.patchStreamTabState(tabId, { pageSize });
    const caps = tab.connectionId ? useConnectionsStore().states[tab.connectionId]?.caps : null;
    if (caps?.pagination === 'batch') return;
    if (!rt.polled) return; // mirrors onMounted's own guard — never auto-load before the first view
    await load(tabId, { mode: 'offset', offset: 0 });
  }

  // Kafka-only (StreamView.vue only renders the filter row, and thus only ever calls
  // this, when connection.kind === 'kafka'). Mirrors grid/state.ts's setFilter: reset the
  // continuation token, persist, record it in the (session-only) filter history, and restart the
  // browse fresh — a filter changes which messages a *new* browse would see, so continuing an old
  // token under it would silently ignore it.
  async function applyStreamFilter(tabId: string, filter: StreamFilterInput): Promise<void> {
    const tabsStore = useTabsStore();
    const tab = tabsStore.findStreamTab(tabId);
    if (!tab?.connectionId) return;
    const rt = ensureRuntime(tabId);
    rt.nextToken = null;
    // P21 round 2 functional finding 7: now that runCount sends this same filter, a count taken
    // under the *previous* filter answers a different question than the browse this narrows to —
    // grid/state.ts's own setFilter clears rather than stales its count for the identical reason
    // ("an answer to the previous WHERE is an answer to a different question, not a drifted answer
    // to this one"). Cleared, not staled, so the toolbar returns to "no total" rather than showing a
    // wrong one under a `stale` label that would still be visible until the next Σ click.
    rt.count = null;
    rt.countOpId = null;
    // P108 Part 11 F15: grid/state.ts's own setFilter (and documents/state.ts's setSearch) clear
    // this alongside count/countOpId — left out here, a count failure from before this filter
    // change kept showing in the Σ tooltip under a total this filter never produced.
    rt.countError = null;
    tabsStore.patchStreamTabState(tabId, {
      offsetFilter: filter.offset,
      partitions: filter.partitions,
      timestampFilter: filter.timestamp,
    });
    useStreamFilterHistoryStore().recordStreamFilterUse(tab.connectionId, tab.path, filter);
    await load(tabId, { mode: 'offset', offset: 0 });
  }

  // The row last clicked — StreamView.vue pairs this with cellSelection.ts's
  // publishSelectedCell(). Kept here (rather than only local component state) so SQS's Delete
  // message toolbar action, which lives beside the row list but isn't itself a per-row control,
  // can read the same target.
  function selectRow(tabId: string, row: number | null): void {
    ensureRuntime(tabId).selectedRow = row;
  }

  /** Shown page is stale: connection gone, or read under another connection session. */
  function isStale(tabId: string): boolean {
    const connectionId = useTabsStore().findStreamTab(tabId)?.connectionId;
    return connectionId ? useStaleDataStore().isStale(tabId, tabId, connectionId) : false;
  }

  function setFilterDraft(
    tabId: string,
    draft: {
      offset?: string | null;
      timestamp?: string | null;
      partitions?: number[] | null;
    },
  ): void {
    const rt = ensureRuntime(tabId);
    if (draft.offset !== undefined) rt.offsetDraft = draft.offset;
    if (draft.timestamp !== undefined) rt.timestampDraft = draft.timestamp;
    if (draft.partitions !== undefined) rt.partitionsDraft = draft.partitions;
  }

  return {
    runtime,
    isStale,
    setFilterDraft,
    load,
    reload,
    runCount,
    stop,
    poll,
    fetchRedriveLimit,
    acknowledgeReceive,
    setPollVisibilityTimeout,
    goNext,
    setPageSize,
    applyStreamFilter,
    selectRow,
    setActionError,
    toggleSearchOpen,
    setSearchOpen,
  };
});

// D5/D6: project/ no longer imports this module directly — it reaches reload through
// state/viewCommands.ts's registry instead.
registerTabReload('stream', (tabId) => useStreamViewStore(pinia).reload(tabId));
