import type { ConnectionSummary } from '@shared/domain/connection';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../../ipc/support/types';
import {
  connectAndExpandControl as sharedConnectAndExpandControl,
  connectionSummary as sharedConnectionSummary,
} from './engineFixture';

// Real captures against a real Mongo 7 container, seeded with packages/db-fixtures/fixtures/0003_mongo_seed.ts
// (the same seed packages/db-fixtures/mongo.spec.ts and tests/e2e/mongo.spec.ts use) — via
// `bun scripts/capture-tree.ts mongo --recipe-file ...` (scripts/capture-tree.ts), not hand-written
// (P50 D5's discipline). Confirmed here, a real environment finding: unlike Postgres (CLAUDE.md's
// Docker section — its own forListeningPorts() wait strategy hangs under `bun run` in this
// sandbox), Mongo's container (Wait.forLogMessage, no forListeningPorts()) starts and this whole
// capture completes fine under plain `bun run` — no esbuild/vendored-Node workaround needed for
// this adapter.
//
// `widgets`' 25-document real page and the `{ name: 'widget-1' }` filtered single-document read
// are exactly what autocomplete.spec.ts's Mongo filter-row scenario needs; the collection listing
// is exactly what its two Mongo-console scenarios need (mongo/completion.ts's F5 reads it straight
// out of the tree cache — no separate data-plane round trip for collection-name completion at
// all, and none for methods/operators/collections used as console vocabulary either, since those
// are pure client-side grammar — see apps/kira-space/frontend/src/studio/views/console/completion.ts).

export const DB_PATH = 'database:kira_test';
export const WIDGETS_PATH = `${DB_PATH}/collection:widgets`;

const SERVER_VERSION = 'MongoDB 7.0.40';

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

export const ROOT_CHILDREN = [
  { kind: 'database', name: 'kira_analytics', path: 'database:kira_analytics', hasChildren: true },
  { kind: 'database', name: 'kira_test', path: 'database:kira_test', hasChildren: true },
];

// The real capture's full kira_test collection list — every collection 0003_mongo_seed.ts creates,
// kept whole rather than trimmed to `widgets` alone. Sorts alphabetically, unfiltered.
export const DB_CHILDREN = [
  {
    kind: 'collection',
    name: 'big_widgets',
    path: `${DB_PATH}/collection:big_widgets`,
    hasChildren: false,
  },
  {
    kind: 'collection',
    name: 'empty_collection',
    path: `${DB_PATH}/collection:empty_collection`,
    hasChildren: false,
  },
  {
    kind: 'collection',
    name: 'oversized_widgets',
    path: `${DB_PATH}/collection:oversized_widgets`,
    hasChildren: false,
  },
  {
    kind: 'collection',
    name: 'validated_widgets',
    path: `${DB_PATH}/collection:validated_widgets`,
    hasChildren: false,
  },
  { kind: 'collection', name: 'widgets', path: WIDGETS_PATH, hasChildren: false },
];

// The real capture's full unfiltered widgets page (all 25 seeded documents — WIDGET_COUNT in
// packages/db-fixtures/fixtures/0003_mongo_seed.ts): the exact EJSON text `capture-tree.ts mongo` printed for
// this read (out/mongo-capture.log step 2), copied verbatim — not re-derived from the seed
// function's own field-generation logic, which this must not silently drift from (P50 D5).
export const WIDGETS_IDS: (string | null)[] = [
  '{"$oid":"000000000000000000000000"}',
  '{"$oid":"000000000000000000000001"}',
  '{"$oid":"000000000000000000000002"}',
  '{"$oid":"000000000000000000000003"}',
  '{"$oid":"000000000000000000000004"}',
  '{"$oid":"000000000000000000000005"}',
  '{"$oid":"000000000000000000000006"}',
  '{"$oid":"000000000000000000000007"}',
  '{"$oid":"000000000000000000000008"}',
  '{"$oid":"000000000000000000000009"}',
  '{"$oid":"00000000000000000000000a"}',
  '{"$oid":"00000000000000000000000b"}',
  '{"$oid":"00000000000000000000000c"}',
  '{"$oid":"00000000000000000000000d"}',
  '{"$oid":"00000000000000000000000e"}',
  '{"$oid":"00000000000000000000000f"}',
  '{"$oid":"000000000000000000000010"}',
  '{"$oid":"000000000000000000000011"}',
  '{"$oid":"000000000000000000000012"}',
  '{"$oid":"000000000000000000000013"}',
  '{"$oid":"000000000000000000000014"}',
  '{"$oid":"000000000000000000000015"}',
  '{"$oid":"000000000000000000000016"}',
  '{"$oid":"000000000000000000000017"}',
  '{"$oid":"000000000000000000000018"}',
];
export const WIDGETS_BODIES: (string | null)[] = [
  '{"_id":{"$oid":"000000000000000000000000"},"name":"widget-0","price":{"$numberDouble":"1.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1704067200000"}},"tags":["red","small"],"meta":{"weight":{"$numberInt":"0"},"note":null}}',
  '{"_id":{"$oid":"000000000000000000000001"},"name":"widget-1","price":{"$numberInt":"3"},"active":false,"createdAt":{"$date":{"$numberLong":"1704153600000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"1"},"note":"note-1"}}',
  '{"_id":{"$oid":"000000000000000000000002"},"name":"widget-2","price":{"$numberDouble":"4.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1704240000000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"2"},"note":"note-2"}}',
  '{"_id":{"$oid":"000000000000000000000003"},"name":"widget-3","price":{"$numberInt":"6"},"active":false,"createdAt":{"$date":{"$numberLong":"1704326400000"}},"tags":["red","small"],"meta":{"weight":{"$numberInt":"3"},"note":"note-3"}}',
  '{"_id":{"$oid":"000000000000000000000004"},"name":"widget-4","price":{"$numberDouble":"7.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1704412800000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"4"},"note":"note-4"}}',
  '{"_id":{"$oid":"000000000000000000000005"},"name":"widget-5","price":{"$numberInt":"9"},"active":false,"createdAt":{"$date":{"$numberLong":"1704499200000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"5"},"note":null}}',
  '{"_id":{"$oid":"000000000000000000000006"},"name":"widget-6","price":{"$numberDouble":"10.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1704585600000"}},"tags":["red","small"],"meta":{"weight":{"$numberInt":"6"},"note":"note-6"}}',
  '{"_id":{"$oid":"000000000000000000000007"},"name":"widget-7","price":{"$numberInt":"12"},"active":false,"createdAt":{"$date":{"$numberLong":"1704672000000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"7"},"note":"note-7"}}',
  '{"_id":{"$oid":"000000000000000000000008"},"name":"widget-8","price":{"$numberDouble":"13.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1704758400000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"8"},"note":"note-8"}}',
  '{"_id":{"$oid":"000000000000000000000009"},"name":"widget-9","price":{"$numberInt":"15"},"active":false,"createdAt":{"$date":{"$numberLong":"1704844800000"}},"tags":["red","small"],"meta":{"weight":{"$numberInt":"9"},"note":"note-9"}}',
  '{"_id":{"$oid":"00000000000000000000000a"},"name":"widget-10","price":{"$numberDouble":"16.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1704931200000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"10"},"note":null}}',
  '{"_id":{"$oid":"00000000000000000000000b"},"name":"widget-11","price":{"$numberInt":"18"},"active":false,"createdAt":{"$date":{"$numberLong":"1705017600000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"11"},"note":"note-11"}}',
  '{"_id":{"$oid":"00000000000000000000000c"},"name":"widget-12","price":{"$numberDouble":"19.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1705104000000"}},"tags":["red","small"],"meta":{"weight":{"$numberInt":"12"},"note":"note-12"}}',
  '{"_id":{"$oid":"00000000000000000000000d"},"name":"widget-13","price":{"$numberInt":"21"},"active":false,"createdAt":{"$date":{"$numberLong":"1705190400000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"13"},"note":"note-13"}}',
  '{"_id":{"$oid":"00000000000000000000000e"},"name":"widget-14","price":{"$numberDouble":"22.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1705276800000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"14"},"note":"note-14"}}',
  '{"_id":{"$oid":"00000000000000000000000f"},"name":"widget-15","price":{"$numberInt":"24"},"active":false,"createdAt":{"$date":{"$numberLong":"1705363200000"}},"tags":["red","small"],"meta":{"weight":{"$numberInt":"15"},"note":null}}',
  '{"_id":{"$oid":"000000000000000000000010"},"name":"widget-16","price":{"$numberDouble":"25.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1705449600000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"16"},"note":"note-16"}}',
  '{"_id":{"$oid":"000000000000000000000011"},"name":"widget-17","price":{"$numberInt":"27"},"active":false,"createdAt":{"$date":{"$numberLong":"1705536000000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"17"},"note":"note-17"}}',
  '{"_id":{"$oid":"000000000000000000000012"},"name":"widget-18","price":{"$numberDouble":"28.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1705622400000"}},"tags":["red","small"],"meta":{"weight":{"$numberInt":"18"},"note":"note-18"}}',
  '{"_id":{"$oid":"000000000000000000000013"},"name":"widget-19","price":{"$numberInt":"30"},"active":false,"createdAt":{"$date":{"$numberLong":"1705708800000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"19"},"note":"note-19"}}',
  '{"_id":{"$oid":"000000000000000000000014"},"name":"widget-20","price":{"$numberDouble":"31.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1705795200000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"20"},"note":null}}',
  '{"_id":{"$oid":"000000000000000000000015"},"name":"widget-21","price":{"$numberInt":"33"},"active":false,"createdAt":{"$date":{"$numberLong":"1705881600000"}},"tags":["red","small"],"meta":{"weight":{"$numberInt":"21"},"note":"note-21"}}',
  '{"_id":{"$oid":"000000000000000000000016"},"name":"widget-22","price":{"$numberDouble":"34.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1705968000000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"22"},"note":"note-22"}}',
  '{"_id":{"$oid":"000000000000000000000017"},"name":"widget-23","price":{"$numberInt":"36"},"active":false,"createdAt":{"$date":{"$numberLong":"1706054400000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"23"},"note":"note-23"}}',
  '{"_id":{"$oid":"000000000000000000000018"},"name":"widget-24","price":{"$numberDouble":"37.5"},"active":true,"createdAt":{"$date":{"$numberLong":"1706140800000"}},"tags":["red","small"],"meta":{"weight":{"$numberInt":"24"},"note":"note-24"}}',
];

const WIDGET_1_ID = '{"$oid":"000000000000000000000001"}';
const WIDGET_1_BODY =
  '{"_id":{"$oid":"000000000000000000000001"},"name":"widget-1","price":{"$numberInt":"3"},"active":false,"createdAt":{"$date":{"$numberLong":"1704153600000"}},"tags":["blue"],"meta":{"weight":{"$numberInt":"1"},"note":"note-1"}}';

/** The connect + expand-to-`kira_test` boilerplate every fixture below shares. */
export function connectAndExpandControl(connectionId: string): ControlSnapshot[] {
  return sharedConnectAndExpandControl(connectionId, SERVER_VERSION, CAPS, [
    { path: '', children: ROOT_CHILDREN },
    { path: DB_PATH, children: DB_CHILDREN },
  ]);
}

/** Connect, expand to `kira_test`, and open `widgets` — a real captured 25-document unfiltered
 *  read (matching the seed's own WIDGET_COUNT) plus a real captured `{ name: 'widget-1' }`
 *  filtered read (the unique-indexed `name` field, exactly one match). */
export function widgetsFixture(connectionId: string): {
  control: ControlSnapshot[];
  port: PortSnapshot[];
} {
  return {
    control: connectAndExpandControl(connectionId),
    port: [
      {
        op: DATA_OP.read,
        payload: {
          connectionId,
          path: WIDGETS_PATH,
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
            ids: WIDGETS_IDS,
            bodies: WIDGETS_BODIES,
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
      {
        op: DATA_OP.read,
        payload: {
          connectionId,
          path: WIDGETS_PATH,
          projection: null,
          filter: "{ name: 'widget-1' }",
          sort: null,
          pageSize: 100,
          cursor: { mode: 'offset', offset: 0 },
        },
        response: {
          kind: 'read',
          page: {
            kind: 'document',
            ids: [WIDGET_1_ID],
            bodies: [WIDGET_1_BODY],
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

export function mongoConnectionSummary(
  id: string,
  name: string,
  color: ConnectionSummary['color'],
): ConnectionSummary {
  return sharedConnectionSummary('mongodb', 27017, 'kira_test', 'kira', id, name, color);
}
