// views/browse/state.ts's load() supersession guard and views/shared/keyvalue/state.ts's
// cursor-strategy reload fallback are both pinned today only by Docker-gated
// Playwright steps that cannot deterministically force the race they exist to guard against. The
// browse guard's own coverage (tests/e2e/s3.spec.ts's "descend then press Up immediately" step)
// only exercises the guard if the slow load happens to still be in flight when
// Up lands — on a fast container or a small level, the step passes whether or not the guard
// exists. Both modules are plain TypeScript over bridge/data/bridge/control, both window.kira
// wrappers a stub can satisfy — this file resolves the *older* of two in-flight loads *after* the
// newer one, the exact interleaving no Playwright test can force, by holding both on manually
// resolved promises.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { PageCursor } from '@shared/protocol/dataOps';
import type { KeyValuePage, TextColumnChunk } from '@shared/protocol/page';
import { deferred } from '@workbench/testing/unit/async';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { isReactive } from 'vue';
import { pinia } from '../../../frontend/src/studio/state/pinia';
import { seedConnectionRecord } from './support/connectionRecord.ts';

setActivePinia(pinia);

const { control } = await import('../../../frontend/src/studio/bridge/control');
// createTabsStore's saveIfChanged now serialises every save through one
// persistent chain — a real (never-settling in this harness) control.tabsSave triggered
// incidentally by opening a tab below would otherwise wedge every later spec's own tabsSave
// assertions for the rest of the process. This spec doesn't test persistence, so give it a
// benign default -- set *before* restoreAfterEach snapshots control, not in beforeEach: the
// enqueued save this triggers can settle on a tick past this file's own afterEach (confirmed
// via stream-count-honors-filter.spec.ts's identical hazard: only reproduces with ~90+ other
// spec files loaded, never in isolation), so a beforeEach/afterEach-scoped override races the
// chain back onto the hanging default and wedges it permanently for every later spec in the
// process.
(control as unknown as { tabsSave: typeof control.tabsSave }).tabsSave = () => Promise.resolve();
restoreAfterEach(control);
const { data } = await import('../../../frontend/src/studio/bridge/data');
restoreAfterEach(data);
const { useConnectionsStore } = await import('../../../frontend/src/studio/state/connections');
const connectionsStore = useConnectionsStore();
const { useTabsStore } = await import('../../../frontend/src/studio/state/tabs');
const tabsStore = useTabsStore();
const { useBrowseViewStore } = await import('../../../frontend/src/studio/views/browse/state');
const browseViewStore = useBrowseViewStore();
const { useKeyValueViewStore } = await import(
  '../../../frontend/src/studio/views/shared/keyvalue/state'
);
const keyValueViewStore = useKeyValueViewStore();
const { setPage } = await import('../../../frontend/src/studio/views/shared/keyvalue/page');
const { useGridViewStore } = await import('../../../frontend/src/studio/views/grid/state');
const gridViewStore = useGridViewStore();

function emptyChunk(): TextColumnChunk {
  return {
    data: new Uint8Array(0),
    offsets: new Uint32Array([0]),
    nulls: new Uint8Array(0),
    truncated: new Uint32Array(0),
  };
}

describe('views/browse/state.ts — load() supersession guard (P44 F47, P43 D39)', () => {
  test('1. resolving the older treeChildren call after the newer one still leaves the newer nodes in place', async () => {
    const { id } = tabsStore.openBrowseTab('conn1', 'bucket:one', { newTab: true });
    const calls: Array<ReturnType<typeof deferred<{ nodes: unknown[]; truncated: boolean }>>> = [];
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real TreeChildrenResult
    (control as any).treeChildren = () => {
      const d = deferred<{ nodes: unknown[]; truncated: boolean }>();
      calls.push(d);
      return d.promise;
    };

    const older = browseViewStore.load(id); // loadSeq 1
    const newer = browseViewStore.load(id); // loadSeq 2
    expect(calls).toHaveLength(2);

    // The newer call lands first; the older one resolves after it.
    calls[1]?.resolve({ nodes: [{ name: 'newer' }], truncated: false });
    await newer;
    calls[0]?.resolve({ nodes: [{ name: 'older' }], truncated: false });
    await older;

    expect(browseViewStore.runtime[id]?.nodes.map((n) => n.name)).toEqual(['newer']);
  });

  test('2. a superseded failure does not redden a level that loaded fine', async () => {
    const { id } = tabsStore.openBrowseTab('conn2', 'bucket:two', { newTab: true });
    const calls: Array<ReturnType<typeof deferred<{ nodes: unknown[]; truncated: boolean }>>> = [];
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real TreeChildrenResult
    (control as any).treeChildren = () => {
      const d = deferred<{ nodes: unknown[]; truncated: boolean }>();
      calls.push(d);
      return d.promise;
    };

    const older = browseViewStore.load(id); // loadSeq 1 — will fail
    const newer = browseViewStore.load(id); // loadSeq 2 — will succeed
    calls[1]?.resolve({ nodes: [{ name: 'good' }], truncated: false });
    await newer;
    expect(browseViewStore.runtime[id]?.status).toBe('idle');

    calls[0]?.reject(new Error('stale failure'));
    await older;

    expect(browseViewStore.runtime[id]?.status).toBe('idle');
    expect(browseViewStore.runtime[id]?.nodes.map((n) => n.name)).toEqual(['good']);
    expect(browseViewStore.runtime[id]?.error).toBeNull();
  });

  test('3. rt.truncated is reset to false the moment a new load starts, before the await settles', async () => {
    const { id } = tabsStore.openBrowseTab('conn3', 'bucket:three', { newTab: true });
    const first = deferred<{ nodes: unknown[]; truncated: boolean }>();
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real TreeChildrenResult
    (control as any).treeChildren = () => first.promise;

    const pending = browseViewStore.load(id);
    if (browseViewStore.runtime[id]) browseViewStore.runtime[id].truncated = true; // simulate a prior truncated level
    const second = browseViewStore.load(id); // a new load starts — must reset truncated synchronously

    expect(browseViewStore.runtime[id]?.truncated).toBe(false);

    first.resolve({ nodes: [], truncated: false });
    await Promise.all([pending, second]);
  });

  // P21 round 3 functional finding 14: setLevel used to patch levelPath and call load() without
  // ever touching `filter`/`selected` — both carried over from whatever level the tab was
  // previously showing.
  test("4. descending into a level clears the previous level's filter and selection", async () => {
    const { id } = tabsStore.openBrowseTab('conn4', 'bucket:four', { newTab: true });
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real TreeChildrenResult
    (control as any).treeChildren = async () => ({ nodes: [{ name: 'child' }], truncated: false });
    await browseViewStore.load(id); // establishes the runtime record setFilter/selectRow write through

    browseViewStore.setFilter(id, 'invoices');
    browseViewStore.selectRow(id, 'bucket:four/some-other-row');
    expect(browseViewStore.runtime[id]?.filter).toBe('invoices');
    expect(browseViewStore.runtime[id]?.selected).toBe('bucket:four/some-other-row');

    await browseViewStore.descend(id, 'bucket:four/invoices');

    expect(browseViewStore.runtime[id]?.filter).toBe('');
    expect(browseViewStore.runtime[id]?.selected).toBeNull();
  });

  // P21 round 3 functional finding 14: a failed descend used to leave `nodes` holding the
  // *previous* level's listing while `levelPath` had already advanced — a stale listing rendered
  // under a breadcrumb that no longer matches it, which a row action (Delete) could act on by
  // mistake.
  test("5. a failed descend does not leave the previous level's nodes rendered under the new breadcrumb", async () => {
    const { id } = tabsStore.openBrowseTab('conn5', 'bucket:five', { newTab: true });
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real TreeChildrenResult
    (control as any).treeChildren = async () => ({
      nodes: [{ name: 'bucket-five-child' }],
      truncated: false,
    });
    await browseViewStore.load(id);
    expect(browseViewStore.runtime[id]?.nodes.map((n) => n.name)).toEqual(['bucket-five-child']);

    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real TreeChildrenResult
    (control as any).treeChildren = async () => {
      throw new Error('E_QUERY: token expired');
    };
    await browseViewStore.descend(id, 'bucket:five/sub');

    expect(browseViewStore.runtime[id]?.status).toBe('error');
    expect(browseViewStore.runtime[id]?.nodes).toEqual([]);
  });

  // `runtime` (viewOp.ts's createRuntimeStore) is a deep
  // reactive() — assigning a plain array to `rt.nodes` used to wrap it, and every TreeNode inside
  // it, in its own reactivity Proxy the moment BrowseView.vue's filteredNodes read them. A Redis/S3
  // level can hold up to 200 000 nodes (redis/catalog.go's scanCount x maxScanRounds); nothing here
  // ever mutates a node in place (`nodes` is always replaced wholesale), so the deep wrap bought
  // nothing — the same shape project/state/tree.ts's own `children` already moved off deep
  // reactivity for.
  test('6. a loaded node list is markRaw — not wrapped in a reactivity Proxy (finding 8)', async () => {
    const { id } = tabsStore.openBrowseTab('conn6', 'bucket:six', { newTab: true });
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real TreeChildrenResult
    (control as any).treeChildren = async () => ({
      nodes: [{ name: 'a' }, { name: 'b' }],
      truncated: false,
    });
    await browseViewStore.load(id);

    const nodes = browseViewStore.runtime[id]?.nodes;
    expect(nodes).toBeDefined();
    expect(isReactive(nodes)).toBe(false);
    expect(isReactive(nodes?.[0])).toBe(false);
    expect(nodes?.map((n) => n.name)).toEqual(['a', 'b']);
  });
});

describe('views/shared/keyvalue/state.ts — cursor-strategy reload fallback (P44 F47, P43 D40)', () => {
  function makeKeyValuePage(strategy: 'offset' | 'cursor'): KeyValuePage {
    return {
      kind: 'keyvalue',
      position: {
        offset: strategy === 'offset' ? 200 : null,
        pageSize: 100,
        hasMore: true,
        nextToken: strategy === 'cursor' ? 'tok' : null,
        prevToken: null,
        strategy,
      },
      redisType: strategy === 'offset' ? 'list' : 'hash',
      ttlMs: null,
      memoryBytes: null,
      fields: emptyChunk(),
      values: emptyChunk(),
      rowCount: 0,
      byteSize: 0,
      fetchedAt: Date.now(),
    };
  }

  test('4. a cursor-strategy page reloads with offset: 0 and returns pageIndex to 0', async () => {
    seedConnectionRecord(connectionsStore, 'conn4');
    const { id } = tabsStore.openKeyValueTab('conn4', 'db0/key:big-hash', { newTab: true })!;
    setPage(id, makeKeyValuePage('cursor'));
    let capturedCursor: PageCursor | undefined;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ReadResponse
    (data as any).read = (req: { cursor: PageCursor }) => {
      capturedCursor = req.cursor;
      return Promise.resolve({ page: makeKeyValuePage('cursor'), source: 'server' });
    };

    await keyValueViewStore.load(id);

    expect(capturedCursor).toEqual({ mode: 'offset', offset: 0 });
    expect(tabsStore.findKeyValueTab(id)?.state.pageIndex).toBe(0);
  });

  test('5. an offset-strategy page on the same code path still reloads with pageIndex * pageSize, unchanged', async () => {
    seedConnectionRecord(connectionsStore, 'conn5');
    const { id } = tabsStore.openKeyValueTab('conn5', 'db0/key:big-list', { newTab: true })!;
    setPage(id, makeKeyValuePage('offset'));
    let capturedCursor: PageCursor | undefined;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ReadResponse
    (data as any).read = (req: { cursor: PageCursor }) => {
      capturedCursor = req.cursor;
      return Promise.resolve({ page: makeKeyValuePage('offset'), source: 'server' });
    };
    const tab = tabsStore.findKeyValueTab(id);
    if (!tab) throw new Error('expected the tab to exist');
    tab.state.pageIndex = 2;

    await keyValueViewStore.load(id);

    expect(capturedCursor).toEqual({ mode: 'offset', offset: 2 * tab.state.pageSize });
    expect(tabsStore.findKeyValueTab(id)?.state.pageIndex).toBe(2);
  });
});

// goNext/goPrev/etc. patch pageIndex to the new page *before* the load that
// fetches it settles — a failed or cancelled load never calls setPage, so the grid keeps
// rendering the old page's rows while the pager, left un-reverted, claimed a page that was never
// fetched.
describe('views/grid/state.ts — pageIndex reverts on a failed or cancelled load (P2 R2, task #93)', () => {
  test('6. goNext reverts pageIndex to the previous page when the load fails', async () => {
    seedConnectionRecord(connectionsStore, 'conn6');
    const { id } = tabsStore.openDataTab('conn6', 'public.orders', { newTab: true })!;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ReadResponse
    (data as any).read = () =>
      Promise.reject(Object.assign(new Error('boom'), { code: 'E_QUERY' }));

    await gridViewStore.goNext(id);

    expect(tabsStore.findDataTab(id)?.state.pageIndex).toBe(0);
    expect(gridViewStore.runtime[id]?.status).toBe('error');
  });

  test('7. goNext reverts pageIndex to the previous page when the load is cancelled', async () => {
    seedConnectionRecord(connectionsStore, 'conn7');
    const { id } = tabsStore.openDataTab('conn7', 'public.orders', { newTab: true })!;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ReadResponse
    (data as any).read = () =>
      Promise.reject(Object.assign(new Error('cancelled'), { code: 'E_CANCELLED' }));

    await gridViewStore.goNext(id);

    expect(tabsStore.findDataTab(id)?.state.pageIndex).toBe(0);
    expect(gridViewStore.runtime[id]?.status).toBe('cancelled');
  });

  test('8. goPrev reverts pageIndex to the previous page when the load fails', async () => {
    seedConnectionRecord(connectionsStore, 'conn8');
    const { id } = tabsStore.openDataTab('conn8', 'public.orders', { newTab: true })!;
    const tab = tabsStore.findDataTab(id);
    if (!tab) throw new Error('expected the tab to exist');
    tab.state.pageIndex = 3;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ReadResponse
    (data as any).read = () =>
      Promise.reject(Object.assign(new Error('boom'), { code: 'E_QUERY' }));

    await gridViewStore.goPrev(id);

    expect(tabsStore.findDataTab(id)?.state.pageIndex).toBe(3);
  });

  // This test used to lock in the exact bug fixed — a second goNext firing before the first's load
  // resolved stacked a second optimistic pageIndex advance on top of the first, so a failure could
  // only ever revert to one or the other guess, never to the page that had last loaded.
  // goNext/goPrev now no-op while a load is already in flight, so that stacking can no longer
  // happen at all — rewritten to assert the guard, not the old two-in-flight scenario it made
  // impossible.
  test('9. a second goNext call while a load is in flight is a no-op, not a second optimistic advance', async () => {
    seedConnectionRecord(connectionsStore, 'conn9');
    const { id } = tabsStore.openDataTab('conn9', 'public.orders', { newTab: true })!;
    const first = deferred<{ page: unknown; source: string }>();
    let readCalls = 0;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ReadResponse
    (data as any).read = () => {
      readCalls++;
      return first.promise;
    };

    const older = gridViewStore.goNext(id); // page 0 -> 1, opId A, still in flight
    expect(tabsStore.findDataTab(id)?.state.pageIndex).toBe(1);

    await gridViewStore.goNext(id); // stray second click while A is in flight
    expect(tabsStore.findDataTab(id)?.state.pageIndex).toBe(1); // never stacked to 2
    expect(readCalls).toBe(1); // data.read was never issued a second time

    // A itself now fails — reverts to its own previous index (0), same as any single failed load.
    first.reject(Object.assign(new Error('boom'), { code: 'E_QUERY' }));
    await older;
    expect(tabsStore.findDataTab(id)?.state.pageIndex).toBe(0);

    // The guard clears once the in-flight load resolves — goNext works normally again.
    const second = deferred<{ page: unknown; source: string }>();
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ReadResponse
    (data as any).read = () => second.promise;
    const again = gridViewStore.goNext(id);
    expect(tabsStore.findDataTab(id)?.state.pageIndex).toBe(1);
    second.reject(Object.assign(new Error('boom'), { code: 'E_QUERY' }));
    await again;
    expect(tabsStore.findDataTab(id)?.state.pageIndex).toBe(0);
  });
});

describe('views/shared/keyvalue/state.ts — pageIndex reverts on a failed load (P2 R2, task #93)', () => {
  test('10. goNext reverts pageIndex to the previous page when the load fails', async () => {
    seedConnectionRecord(connectionsStore, 'conn10');
    const { id } = tabsStore.openKeyValueTab('conn10', 'db0/key:big-list', { newTab: true })!;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ReadResponse
    (data as any).read = () =>
      Promise.reject(Object.assign(new Error('boom'), { code: 'E_QUERY' }));

    await keyValueViewStore.goNext(id);

    expect(tabsStore.findKeyValueTab(id)?.state.pageIndex).toBe(0);
    expect(keyValueViewStore.runtime[id]?.status).toBe('error');
  });
});
