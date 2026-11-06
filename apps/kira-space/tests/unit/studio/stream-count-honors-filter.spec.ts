// load() encodes the partition/offset/timestamp filter a Kafka
// stream tab is browsing under and sends it with every data.read, but runCount used to hard-code
// `filter: null` — the toolbar's own "<N> total" therefore answered the high/low watermark summed
// across *every* partition, a different question than the "<n> rows on this page" printed beside
// it. This pins two things: runCount now sends the exact same encoded filter load() would send for
// the tab's current filter state, and applying a new filter clears the previous (now
// differently-scoped) count rather than leaving a stale total on screen under the new filter.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import type { ConnectionState } from '@shared/domain/connection';
import { restoreAfterEach } from '@workbench/testing/unit/restoreAfterEach';
import { setActivePinia } from 'pinia';
import { pinia } from '../../../frontend/src/state/pinia';
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
// enqueued save this triggers can settle on a tick past this file's own afterEach (confirmed:
// only reproduces with ~90+ other spec files loaded, never in isolation), so a beforeEach/
// afterEach-scoped override races the chain back onto the hanging default and wedges it
// permanently for every later spec in the process.
(control as unknown as { tabsSave: typeof control.tabsSave }).tabsSave = () => Promise.resolve();
restoreAfterEach(control);
const { useConnectionsStore } = await import('../../../frontend/src/studio/state/connections');
const connectionsStore = useConnectionsStore();
const { useTabsStore } = await import('../../../frontend/src/state/tabs');
const tabsStore = useTabsStore();
const { useStreamViewStore } = await import('../../../frontend/src/studio/views/stream/state');
const streamViewStore = useStreamViewStore();

type Caps = NonNullable<ConnectionState['caps']>;

const kafkaCaps: Caps = {
  tabular: false,
  documents: false,
  keyValue: false,
  stream: true,
  keyBrowser: false,
  keyTypes: false,
  defaultPageKind: 'stream',
  sql: false,
  definition: true,
  describe: false,
  schemaColumns: false,
  projection: false,
  serverFilter: true,
  exactCount: true,
  pagination: 'cursor',
  foreignKeys: false,
  canInsert: true,
  canUpdate: false,
  canDelete: false,
  writable: true,
  transactions: false,
  cancel: true,
  fileTransfer: false,
};

function markConnected(connectionId: string, caps: Caps): void {
  connectionsStore.states[connectionId] = {
    connectionId,
    status: 'connected',
    serverVersion: 'Kafka',
    error: null,
    since: 0,
    caps,
  };
}

describe('Kafka stream count honors the active filter (finding 7)', () => {
  test('runCount sends null filter when no partition/offset/timestamp filter is set', async () => {
    const connectionId = 'kafka-conn-1';
    markConnected(connectionId, kafkaCaps);
    seedConnectionRecord(connectionsStore, connectionId);
    const { id } = tabsStore.openStreamTab(connectionId, 'topic:orders', { newTab: true })!;

    let captured: { filter?: string | null } | undefined;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real CountResponse
    (data as any).count = (req: { filter?: string | null }) => {
      captured = req;
      return Promise.resolve({
        value: 6,
        exact: true,
        at: Date.now(),
        stale: false,
        source: 'server',
      });
    };

    await streamViewStore.runCount(id);

    expect(captured?.filter).toBeNull();
  });

  test('runCount sends the same encoded filter the tab is browsing under, once one is applied', async () => {
    const connectionId = 'kafka-conn-2';
    markConnected(connectionId, kafkaCaps);
    seedConnectionRecord(connectionsStore, connectionId);
    const { id } = tabsStore.openStreamTab(connectionId, 'topic:orders', { newTab: true })!;

    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ReadResponse
    (data as any).read = () =>
      Promise.resolve({
        page: {
          kind: 'stream',
          rowCount: 0,
          position: { hasMore: false, nextToken: null },
          visibilityTimeoutSeconds: null,
        },
      });

    await streamViewStore.applyStreamFilter(id, {
      offset: null,
      partitions: [0, 1],
      timestamp: null,
    });

    let captured: { filter?: string | null } | undefined;
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real CountResponse
    (data as any).count = (req: { filter?: string | null }) => {
      captured = req;
      return Promise.resolve({
        value: 2,
        exact: true,
        at: Date.now(),
        stale: false,
        source: 'server',
      });
    };

    await streamViewStore.runCount(id);

    expect(captured?.filter).not.toBeNull();
    const parsed = JSON.parse(captured?.filter as string);
    expect(parsed.partitions).toEqual([0, 1]);
  });

  test('applying a new filter clears the previous count instead of leaving a stale total under it', async () => {
    const connectionId = 'kafka-conn-3';
    markConnected(connectionId, kafkaCaps);
    seedConnectionRecord(connectionsStore, connectionId);
    const { id } = tabsStore.openStreamTab(connectionId, 'topic:orders', { newTab: true })!;

    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real ReadResponse
    (data as any).read = () =>
      Promise.resolve({
        page: {
          kind: 'stream',
          rowCount: 0,
          position: { hasMore: false, nextToken: null },
          visibilityTimeoutSeconds: null,
        },
      });
    // biome-ignore lint/suspicious/noExplicitAny: a minimal fake, not the real CountResponse
    (data as any).count = () =>
      Promise.resolve({ value: 6, exact: true, at: Date.now(), stale: false, source: 'server' });

    await streamViewStore.runCount(id);
    expect(streamViewStore.runtime[id]?.count?.value).toBe(6);

    await streamViewStore.applyStreamFilter(id, { offset: null, partitions: [0], timestamp: null });

    expect(streamViewStore.runtime[id]?.count).toBeNull();
  });
});
