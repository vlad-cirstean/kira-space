// views/grid/state.ts's runCount passes `refresh: rt.count?.stale === true` to data.count
// (a Σ click on an already-fresh count stays an L3 hit, only a stale one bypasses it),
// but the copy-pasted runCount in views/documents, views/keyvalue and views/stream never carried
// that line over: every call always sent refresh as undefined/false, so once their cached count
// went stale, clicking Σ could never force a real recount — it kept re-reading the same stale L3
// entry until it fell out of cache entirely. This file pins the fix across all three.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/studio/state/pinia';
import { seedConnectionRecord } from './support/connectionRecord.ts';

setActivePinia(pinia);

const { data } = await import('../../../frontend/src/studio/bridge/data');
restoreAfterEach(data);
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
const { useConnectionsStore } = await import('../../../frontend/src/studio/state/connections');
const connectionsStore = useConnectionsStore();
const { useTabsStore } = await import('../../../frontend/src/studio/state/tabs');
const tabsStore = useTabsStore();
const { useDocumentViewStore } = await import('../../../frontend/src/studio/views/documents/state');
const documentViewStore = useDocumentViewStore();
const { useKeyValueViewStore } = await import(
  '../../../frontend/src/studio/views/shared/keyvalue/state'
);
const keyValueViewStore = useKeyValueViewStore();
const { useStreamViewStore } = await import('../../../frontend/src/studio/views/stream/state');
const streamViewStore = useStreamViewStore();

interface Case {
  name: string;
  open: () => string;
  runCount: (tabId: string) => Promise<void>;
  // biome-ignore lint/suspicious/noExplicitAny: each module's own runtime record shape
  runtime: Record<string, any>;
}

const cases: Case[] = [
  {
    name: 'documents',
    open: () => {
      seedConnectionRecord(connectionsStore, 'conn-doc');
      return tabsStore.openDocumentTab('conn-doc', 'db/coll', { newTab: true })!.id;
    },
    runCount: (tabId: string) => documentViewStore.runCount(tabId),
    runtime: documentViewStore.runtime,
  },
  {
    name: 'keyvalue',
    open: () => {
      seedConnectionRecord(connectionsStore, 'conn-kv');
      return tabsStore.openKeyValueTab('conn-kv', 'db0/key:big', { newTab: true })!.id;
    },
    runCount: (tabId: string) => keyValueViewStore.runCount(tabId),
    runtime: keyValueViewStore.runtime,
  },
  {
    name: 'stream',
    open: () => {
      seedConnectionRecord(connectionsStore, 'conn-strm');
      return tabsStore.openStreamTab('conn-strm', 'topic:events', { newTab: true })!.id;
    },
    runCount: (tabId: string) => streamViewStore.runCount(tabId),
    runtime: streamViewStore.runtime,
  },
];

for (const { name, open, runCount, runtime } of cases) {
  describe(`views/${name}/state.ts — runCount's refresh flag (P2 R2, P13 D18)`, () => {
    test('a first count (no prior count cached) is requested without forcing a refresh', async () => {
      const id = open();
      let captured: { refresh?: boolean } | undefined;
      // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real CountResponse
      (data as any).count = (req: { refresh?: boolean }) => {
        captured = req;
        return Promise.resolve({
          value: 10,
          exact: true,
          at: Date.now(),
          stale: false,
          source: 'server',
        });
      };

      await runCount(id);

      expect(captured?.refresh).toBe(false);
      expect(runtime[id]?.count).toEqual({ value: 10, exact: true, stale: false });
    });

    test('once the cached count comes back stale, the next Σ click sets refresh: true', async () => {
      const id = open();
      // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real CountResponse
      (data as any).count = () =>
        Promise.resolve({ value: 5, exact: true, at: Date.now(), stale: true, source: 'server' });
      await runCount(id);
      expect(runtime[id]?.count?.stale).toBe(true);

      let captured: { refresh?: boolean } | undefined;
      // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real CountResponse
      (data as any).count = (req: { refresh?: boolean }) => {
        captured = req;
        return Promise.resolve({
          value: 6,
          exact: true,
          at: Date.now(),
          stale: false,
          source: 'server',
        });
      };
      await runCount(id);

      expect(captured?.refresh).toBe(true);
      expect(runtime[id]?.count).toEqual({ value: 6, exact: true, stale: false });
    });
  });
}
