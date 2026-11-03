import type { ConnectionSummary } from '@shared/domain/connection';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { IPC } from './ipcChannels';

/**
 * The connect/expand/order-items/connection-summary scaffolding `mariadbFixture.ts`,
 * `mongoFixture.ts` and `postgresFixture.ts` each built on their own, real-capture data and all —
 * every value below comes from the caller's own engine-specific constants, this file only shapes
 * them into the snapshot objects `installControlMocks`/`installPortMocks` expect.
 */

/** One `treeChildren` expand step `connectAndExpandControl` walks through, in order — mariadb and
 *  mongo each expand one level (root -> database); postgres expands two (root -> database -> `app`
 *  schema), which is why this takes a list of steps rather than a fixed root/database pair. */
export interface TreeExpandStep {
  path: string;
  children: readonly unknown[];
}

/** The connect + tree-expand boilerplate every engine fixture's own `connectAndExpandControl`
 *  shared byte-for-byte bar its own `serverVersion`/`caps`/expand steps. */
export function connectAndExpandControl(
  connectionId: string,
  serverVersion: string,
  caps: unknown,
  expandSteps: readonly TreeExpandStep[],
): ControlSnapshot[] {
  return [
    {
      channel: IPC.connectionsConnect,
      args: { id: connectionId },
      response: {
        connectionId,
        status: 'connected',
        serverVersion,
        error: null,
        since: 1735689600000,
        caps,
      },
    },
    ...expandSteps.map((step) => ({
      channel: IPC.treeChildren,
      args: { connectionId, path: step.path, refresh: false },
      response: { nodes: step.children, source: 'server', truncated: false },
    })),
  ];
}

/** The tabular-page envelope `mariadbFixture.ts`'s and `postgresFixture.ts`'s own `orderItemsPage`
 *  each built — mariadb varies the rows (its own filtered/unfiltered read), postgres the page size
 *  (its own 100/1000 stock sizes); this takes both so either caller's own variation still goes
 *  through one shape. */
export function orderItemsPage(
  columns: ColumnDescriptor[],
  rows: (string | null)[][],
  pageSize: number,
) {
  return {
    kind: 'tabular' as const,
    columns,
    rows,
    position: {
      offset: 0,
      pageSize,
      hasMore: false,
      nextToken: null,
      prevToken: null,
      strategy: 'keyset' as const,
    },
    truncatedCells: 0,
  };
}

/** One `data.read` snapshot `orderItemsFixture` mocks — mariadb's own `quantity > 1` filtered
 *  read, postgres's own second 1000-row-page-size read, and either engine's own plain unfiltered
 *  read, all the same shape bar `filter`/`pageSize`/`rows`. */
export interface OrderItemsRead {
  filter: string | null;
  pageSize: number;
  rows: (string | null)[][];
}

/** Connect, expand, and open `order_items`'s own describe plus its own reads — mariadbFixture.ts's
 *  and postgresFixture.ts's own `orderItemsFixture`, generalized over each engine's own connect
 *  config, path, meta and read list. mongoFixture.ts's own `widgetsFixture` opens a document
 *  collection, not a tabular table, and is not part of this consolidation (different page kind
 *  entirely, not the same shape duplicated). */
export function orderItemsFixture(
  connectionId: string,
  serverVersion: string,
  caps: unknown,
  expandSteps: readonly TreeExpandStep[],
  orderItemsPath: string,
  orderItemsMeta: unknown,
  columns: ColumnDescriptor[],
  reads: readonly OrderItemsRead[],
): { control: ControlSnapshot[]; port: PortSnapshot[] } {
  return {
    control: [
      ...connectAndExpandControl(connectionId, serverVersion, caps, expandSteps),
      {
        channel: IPC.treeDescribe,
        args: { connectionId, path: orderItemsPath, refresh: false, tabId: null },
        response: { meta: orderItemsMeta, source: 'server' },
      },
    ],
    port: reads.map((r) => ({
      op: DATA_OP.read,
      payload: {
        connectionId,
        path: orderItemsPath,
        projection: null,
        filter: r.filter,
        sort: null,
        pageSize: r.pageSize,
        cursor: { mode: 'offset', offset: 0 },
      },
      response: {
        kind: 'read',
        page: orderItemsPage(columns, r.rows, r.pageSize),
        source: 'server',
      },
    })),
  };
}

/** A plausible `ConnectionSummary` for a freshly-created connection — mariadbFixture.ts's,
 *  mongoFixture.ts's and postgresFixture.ts's own `*ConnectionSummary`, differing only in `kind`,
 *  `port`, `database` and `username`. */
export function connectionSummary(
  kind: ConnectionSummary['kind'],
  port: number,
  database: string,
  username: string,
  id: string,
  name: string,
  color: ConnectionSummary['color'],
): ConnectionSummary {
  return {
    file: 'fixture.yaml',
    id,
    name,
    kind,
    color,
    mode: 'fields',
    readOnly: false,
    host: '127.0.0.1',
    port,
    database,
    username,
    uri: null,
    options: {},
    preconnect: null,
    preconnectSidecar: false,
    preconnectDelaySec: 0,
    autoExplain: false,
    throttlePerSec: 0,
    mcpEnabled: false,
    mcpDescription: '',
    mcpReadMode: 'allow',
    mcpWriteMode: 'prompt',
    mcpDdlMode: 'deny',
    mcpAutoExplain: true,
    mcpReviewResults: 'inherit',
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };
}
