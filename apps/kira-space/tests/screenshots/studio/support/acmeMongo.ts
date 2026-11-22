import { DATA_OP } from '@shared/protocol/dataOps';
import { CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import type { ControlSnapshot, PortSnapshot } from '../../../ipc/support/types';
import { connectAndExpandControl as expandControl } from '../../../ui/studio/support/engineFixture';

// MongoDB fixture of the story: connection `acme-catalog`, collection `orders` of `acme_shop`.
// The control and port shapes are those of ui/studio/support/mongoFixture.ts, with story documents.

export const MONGO_ID = 'conn-acme-catalog';
export const MONGO_NAME = 'acme-catalog';
export const MONGO_DB = 'database:acme_shop';
export const ORDERS_PATH = `${MONGO_DB}/collection:orders`;

const CAPS = {
  tabular: false,
  documents: true,
  keyValue: false,
  stream: false,
  keyBrowser: false,
  defaultPageKind: 'document' as const,
  sql: true,
  definition: true,
  describe: true,
  projection: true,
  serverFilter: true,
  exactCount: false,
  pagination: 'cursor' as const,
  foreignKeys: false,
  canInsert: true,
  canUpdate: true,
  canDelete: true,
  writable: true,
  transactions: false,
  cancel: true,
  fileTransfer: false,
};

const collection = (name: string) => ({
  kind: 'collection',
  name,
  path: `${MONGO_DB}/collection:${name}`,
  hasChildren: false,
});

const oid = (n: number) => n.toString(16).padStart(24, '0');
const DAY = 86_400_000;

const CUSTOMERS = [
  'Alice Morgan',
  'Ben Carter',
  'Chloe Hughes',
  'Daniel Brooks',
  'Emma Wright',
  'Oliver Grant',
];
const STATUSES = ['paid', 'shipped', 'delivered', 'paid', 'shipped', 'paid'];
const SKUS = ['KB-104', 'MS-220', 'HD-310', 'CB-007', 'MN-270'];

/** Twelve orders as extended JSON, the way the adapter sends document bodies. */
const ORDERS = Array.from({ length: 12 }, (_, i) => {
  const number = 4217 - i;
  const total = 24.5 + ((i * 37) % 160) + (i % 2 === 0 ? 0.4 : 0.9);
  return {
    id: `{"$oid":"${oid(0x6a0000 + number)}"}`,
    body: JSON.stringify({
      _id: { $oid: oid(0x6a0000 + number) },
      number: { $numberInt: String(number) },
      customer: { name: CUSTOMERS[i % CUSTOMERS.length] },
      status: STATUSES[i % STATUSES.length],
      items: [
        { sku: SKUS[i % SKUS.length], qty: { $numberInt: String(1 + (i % 3)) } },
        { sku: SKUS[(i + 2) % SKUS.length], qty: { $numberInt: '1' } },
      ],
      total: { $numberDouble: total.toFixed(1) },
      createdAt: { $date: { $numberLong: String(CAPTURE_NOW - (i + 1) * DAY) } },
    }),
  };
});

export function acmeMongo(): { control: ControlSnapshot[]; port: PortSnapshot[] } {
  return {
    control: expandControl(MONGO_ID, 'MongoDB 7.0.14', CAPS, [
      {
        path: '',
        children: [{ kind: 'database', name: 'acme_shop', path: MONGO_DB, hasChildren: true }],
      },
      {
        path: MONGO_DB,
        children: ['customers', 'inventory', 'orders', 'products', 'reviews'].map(collection),
      },
    ]),
    port: [
      {
        op: DATA_OP.read,
        payload: {
          connectionId: MONGO_ID,
          path: ORDERS_PATH,
          projection: null,
          filter: null,
          sort: null,
          pageSize: 100,
          cursor: { mode: 'offset', offset: 0 },
        },
        response: {
          kind: 'read',
          page: {
            kind: 'document',
            ids: ORDERS.map((o) => o.id),
            bodies: ORDERS.map((o) => o.body),
            position: {
              offset: 0,
              pageSize: 100,
              hasMore: false,
              nextToken: null,
              prevToken: null,
              strategy: 'keyset',
            },
          },
          source: 'server',
        },
      },
    ],
  };
}
