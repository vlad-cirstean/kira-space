import type { Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../ipc/support/types';
import { expect } from '../ui/fixtures';
import { createConnection } from '../ui/support/connect';
import { mongoCreateArgs } from '../ui/support/connectionArgs';
import { IPC } from '../ui/support/ipcChannels';
import {
  DB_PATH,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from '../ui/support/mongoFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from '../ui/support/tree';

// Shared by the documents probes: a Mongo widgets collection served with `n` synthetic documents.

export const CONNECTION_ID = 'conn-doc-perf';
const SUMMARY = mongoConnectionSummary(CONNECTION_ID, 'Mongo Perf', 'red');
export const FIXTURE = widgetsFixture(CONNECTION_ID);

export const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: mongoCreateArgs('Mongo Perf'),
    response: SUMMARY,
  },
  ...FIXTURE.control,
];

export const hex = (n: number): string => n.toString(16).padStart(24, '0');

/** Same 7-field shape as `WIDGETS_BODIES[0]` (expanded height 160 px), unique `_id`, varied values. */
export function makeDocs(n: number, padBytes = 0): { ids: string[]; bodies: string[] } {
  const ids: string[] = [];
  const bodies: string[] = [];
  const pad = padBytes > 0 ? `,"pad":"${'x'.repeat(padBytes)}"` : '';
  for (let i = 0; i < n; i++) {
    const id = `{"$oid":"${hex(i)}"}`;
    ids.push(id);
    bodies.push(
      `{"_id":${id},"name":"widget-${i}","price":{"$numberDouble":"${(i % 997) + 0.5}"},"active":${i % 2 === 0},"createdAt":{"$date":{"$numberLong":"${1704067200000 + i * 86400000}"}},"tags":["${i % 3 === 0 ? 'red' : 'blue'}","small"],"meta":{"weight":{"$numberInt":"${i}"},"note":${i % 5 === 0 ? 'null' : `"note-${i}"`}}${pad}}`,
    );
  }
  return { ids, bodies };
}

export function readSnapshot(
  pageSize: number,
  docs: { ids: string[]; bodies: string[] },
  hasMore: boolean,
): PortSnapshot {
  return {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: WIDGETS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: {
      kind: 'read',
      page: {
        kind: 'document',
        ids: docs.ids.slice(0, pageSize),
        bodies: docs.bodies.slice(0, pageSize),
        position: {
          offset: 0,
          pageSize,
          hasMore,
          nextToken: null,
          prevToken: null,
          strategy: 'keyset',
        },
      },
      source: 'server',
    },
  };
}

/** Creates the Mongo perf connection through the new-connection dialog, connects it and expands to the database node. */
export async function connectMongo(page: Page): Promise<void> {
  await createConnection(page, 'mongodb', 'Mongo Perf');
  const connRow = connectionRow(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
}

/** Connects, expands to the widgets collection and opens it (first page rendered). */
export async function openWidgets(page: Page): Promise<void> {
  await connectMongo(page);
  await (await findRow(page, WIDGETS_PATH)).dblclick();
  await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible({
    timeout: 15_000,
  });
}
