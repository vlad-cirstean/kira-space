import { createHash } from 'node:crypto';
import type { ConnectionSummary } from '@shared/domain/connection';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../../ipc/support/types';
import { MD5_ROWS } from '../../../support/seeds/md5Rows';
import { IPC } from '../../support/ipcChannels';
import { NULLS_AND_UNICODE_COLUMNS, NULLS_AND_UNICODE_ROWS } from './cellEditorCaptures';
import {
  connectAndExpandControl as sharedConnectAndExpandControl,
  connectionSummary as sharedConnectionSummary,
  orderItemsFixture as sharedOrderItemsFixture,
} from './engineFixture';

// Real captures against a real Postgres container, seeded with
// packages/db-fixtures/fixtures/0001_seed.sql — via scripts/capture-postgres-tree.ts, not
// hand-written (a hand-written tree node once used the wrong `path` shape and was silently
// rendered rather than rejected). Shared across the remaining tests/e2e/*.spec.ts ports that all
// open the same connect -> expand -> table-open path against `app.order_items` (a small, 3-row,
// 4-column table with two FKs and two indexes — enough surface for tree/definition/data-view
// assertions without a huge fixture).

export const ROOT_CHILDREN = [
  {
    kind: 'database',
    name: 'kira_test',
    path: 'database:kira_test',
    hasChildren: true,
    detail: 'connected',
  },
  { kind: 'database', name: 'postgres', path: 'database:postgres', hasChildren: true },
];

export const DB_CHILDREN = [
  {
    kind: 'schema',
    name: 'analytics',
    path: 'database:kira_test/schema:analytics',
    hasChildren: true,
  },
  { kind: 'schema', name: 'app', path: 'database:kira_test/schema:app', hasChildren: true },
];

const ORDER_ITEMS_COLUMNS: ColumnDescriptor[] = [
  {
    name: 'id',
    dataType: 'integer',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: true,
    generated: false,
  },
  {
    name: 'order_id',
    dataType: 'integer',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: false,
    generated: false,
  },
  {
    name: 'product_id',
    dataType: 'integer',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: false,
    generated: false,
  },
  {
    name: 'quantity',
    dataType: 'integer',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: false,
    generated: false,
  },
];
const ORDER_ITEMS_ROWS = [
  ['1', '1', '1', '2'],
  ['2', '1', '2', '1'],
  ['3', '2', '1', '5'],
];

export const DB_PATH = 'database:kira_test';
export const APP_PATH = `${DB_PATH}/schema:app`;
export const ANALYTICS_PATH = `${DB_PATH}/schema:analytics`;
export const ORDER_ITEMS_PATH = `${APP_PATH}/table:order_items`;
export const WIDE_TABLE_PATH = `${APP_PATH}/table:wide_table`;
export const ORDERS_PATH = `${APP_PATH}/table:orders`;

// Real capture (scripts/capture-postgres-tree.ts) of database:kira_test/schema:analytics's own
// children — surfaced a node no hand-written guess would have included: `events_id_seq`, the
// serial PK's own backing sequence, alongside the `events` table itself (0001_seed.sql's only
// analytics-schema object) — exactly the class of miss the "capture, don't hand-write"
// discipline exists to catch.
export const ANALYTICS_CHILDREN = [
  { kind: 'table', name: 'events', path: `${ANALYTICS_PATH}/table:events`, hasChildren: false },
  {
    kind: 'sequence',
    name: 'events_id_seq',
    path: `${ANALYTICS_PATH}/sequence:events_id_seq`,
    hasChildren: false,
  },
];

export const ORDER_ITEMS_META = {
  path: ORDER_ITEMS_PATH,
  kind: 'table' as const,
  name: 'order_items',
  qualifiedName: 'app.order_items',
  columns: [
    {
      name: 'id',
      position: 1,
      dataType: 'integer',
      nullable: false,
      defaultExpr: "nextval('app.order_items_id_seq'::regclass)",
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'order_id',
      position: 2,
      dataType: 'integer',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'product_id',
      position: 3,
      dataType: 'integer',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'quantity',
      position: 4,
      dataType: 'integer',
      nullable: false,
      defaultExpr: '1',
      isPrimaryKey: false,
      comment: null,
    },
  ],
  primaryKey: ['id'],
  foreignKeys: [
    {
      name: 'order_items_order_id_fkey',
      columns: ['order_id'],
      referencedPath: `${APP_PATH}/table:orders`,
      referencedColumns: ['id'],
      onDelete: 'NO ACTION',
      onUpdate: 'NO ACTION',
    },
    {
      name: 'order_items_product_id_fkey',
      columns: ['product_id'],
      referencedPath: `${APP_PATH}/table:products`,
      referencedColumns: ['id'],
      onDelete: 'NO ACTION',
      onUpdate: 'NO ACTION',
    },
  ],
  referencedBy: [],
  indexes: [
    { name: 'order_items_pkey', columns: ['id'], unique: true, primary: true, method: 'btree' },
    {
      name: 'order_items_order_product_idx',
      columns: ['order_id', 'product_id'],
      unique: true,
      primary: false,
      method: 'btree',
    },
  ],
  rowEstimate: null,
  comment: null,
};

export const POSTGRES_CAPS = {
  tabular: true,
  documents: false,
  keyValue: false,
  stream: false,
  keyBrowser: false,
  defaultPageKind: 'tabular' as const,
  sql: true,
  definition: true,
  describe: true,
  projection: true,
  serverFilter: true,
  exactCount: true,
  pagination: 'keyset' as const,
  foreignKeys: true,
  canInsert: true,
  canUpdate: true,
  canDelete: true,
  writable: true,
  transactions: true,
  cancel: true,
  fileTransfer: false,
};

export const SERVER_VERSION =
  'PostgreSQL 17.11 on x86_64-pc-linux-musl, compiled by gcc (Alpine 15.2.0) 15.2.0, 64-bit';

/** The connect + expand-to-`app`-schema boilerplate every fixture below shares. Exported (not
 *  just used internally) so a spec needing a custom connect/disconnect/reconnect sequence of its
 *  own, or whose own tables live outside this module (a table specific to one spec stays
 *  file-local, per definition.spec.ts/cell-editor.spec.ts's own precedent) — tree.spec.ts and
 *  cell-editor.spec.ts, for two — can still start from the same real connect/root/db/app snapshots
 *  instead of re-deriving them. */
export function connectAndExpandControl(connectionId: string): ControlSnapshot[] {
  return sharedConnectAndExpandControl(connectionId, SERVER_VERSION, POSTGRES_CAPS, [
    { path: '', children: ROOT_CHILDREN },
    { path: DB_PATH, children: DB_CHILDREN },
    { path: APP_PATH, children: APP_CHILDREN },
  ]);
}

/** A `connectionsDisconnect` snapshot for a connection `connectAndExpandControl` already
 *  connected — the ConnectionState shape `state/connections.ts`'s `disconnectConnection` stores
 *  verbatim, same fields as the connected state above with `status`/`serverVersion`/`caps` swapped
 *  to their disconnected values. */
export function connectionsDisconnectSnapshot(connectionId: string): ControlSnapshot {
  return {
    channel: IPC.connectionsDisconnect,
    args: { id: connectionId },
    response: {
      connectionId,
      status: 'disconnected',
      serverVersion: null,
      error: null,
      since: 1735689600000,
      caps: null,
    },
  };
}

/** A connectable postgres ConnectionSummary, plus the control/data snapshots to connect it, expand
 *  its tree down to `app.order_items`, and open that table at both stock page sizes. Every spec
 *  using this can add its own `connectionsCreate` snapshot (name/color differ per spec) and reuse
 *  everything below unchanged. */
export function orderItemsFixture(connectionId: string): {
  control: ControlSnapshot[];
  port: PortSnapshot[];
} {
  return sharedOrderItemsFixture(
    connectionId,
    SERVER_VERSION,
    POSTGRES_CAPS,
    [
      { path: '', children: ROOT_CHILDREN },
      { path: DB_PATH, children: DB_CHILDREN },
      { path: APP_PATH, children: APP_CHILDREN },
    ],
    ORDER_ITEMS_PATH,
    ORDER_ITEMS_META,
    ORDER_ITEMS_COLUMNS,
    [
      { filter: null, pageSize: 100, rows: ORDER_ITEMS_ROWS },
      { filter: null, pageSize: 1000, rows: ORDER_ITEMS_ROWS },
    ],
  );
}

// order_items' own columns, projected into RelationColumns' shape (ObjectMeta's ColumnMeta
// reused verbatim), plus a second relation (customers) — a real SchemaColumns fetch always
// returns the WHOLE container in one round trip, so a fixture with only one relation
// would understate what the cache replaces: once any cache exists for a container, it is
// the sole source of table names for completion (no fallback to the tree's
// relation names once the cache has an answer), so a spec asserting more than one table name must
// see more than one relation here.
export const APP_SCHEMA_COLUMNS_RESPONSE = {
  relations: [
    {
      name: 'order_items',
      kind: 'table' as const,
      columns: ORDER_ITEMS_META.columns,
    },
    {
      name: 'customers',
      kind: 'table' as const,
      columns: [
        {
          name: 'id',
          position: 1,
          dataType: 'integer',
          nullable: false,
          defaultExpr: null,
          isPrimaryKey: true,
          comment: null,
        },
        {
          name: 'name',
          position: 2,
          dataType: 'text',
          nullable: false,
          defaultExpr: null,
          isPrimaryKey: false,
          comment: null,
        },
      ],
    },
  ],
  source: 'server' as const,
};

/** A `treeSchemaColumns` snapshot for `APP_PATH` — the container `orderItemsFixture`'s own
 *  console/data-tab specs open. Callers add this only when a test wants column completion driven
 *  by the cache rather than a DDL document. */
export function appSchemaColumnsSnapshot(connectionId: string): ControlSnapshot {
  return {
    channel: IPC.treeSchemaColumns,
    args: { connectionId, path: APP_PATH, refresh: false },
    response: APP_SCHEMA_COLUMNS_RESPONSE,
  };
}

export const COMPOSITE_PK_PATH = `${APP_PATH}/table:composite_pk`;

export const COMPOSITE_PK_COLUMNS: ColumnDescriptor[] = [
  {
    name: 'tenant_id',
    dataType: 'integer',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: true,
    generated: false,
  },
  {
    name: 'entity_id',
    dataType: 'integer',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: true,
    generated: false,
  },
  {
    name: 'name',
    dataType: 'text',
    typeClass: 'text',
    nullable: true,
    isPrimaryKey: false,
    generated: false,
  },
];

export const COMPOSITE_PK_META = {
  path: COMPOSITE_PK_PATH,
  kind: 'table' as const,
  name: 'composite_pk',
  qualifiedName: 'app.composite_pk',
  columns: [
    {
      name: 'tenant_id',
      position: 1,
      dataType: 'integer',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'entity_id',
      position: 2,
      dataType: 'integer',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'name',
      position: 3,
      dataType: 'text',
      nullable: true,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
  ],
  primaryKey: ['tenant_id', 'entity_id'],
  foreignKeys: [],
  referencedBy: [],
  indexes: [
    {
      name: 'composite_pk_pkey',
      columns: ['tenant_id', 'entity_id'],
      unique: true,
      primary: true,
      method: 'btree',
    },
  ],
  rowEstimate: null,
  comment: null,
};

/** Real captures of `app.composite_pk`'s starting 3-row state — (1,1)/(1,2)/(2,1), a genuine
 *  2-column PK and no inbound FK (packages/db-fixtures's own reason for choosing this table too,
 *  per packages/db-fixtures/postgres.spec.ts). Callers add their own
 *  connect/tree/read/count/mutate snapshots on
 *  top for whatever sequence their own scenario needs — unlike order_items, mutations.spec.ts and
 *  interaction.spec.ts each drive a different, stateful mutation sequence against it, so there is
 *  no one fixed "the" fixture the way orderItemsFixture() is. */
export function compositePkConnectAndOpen(connectionId: string): {
  control: ControlSnapshot[];
  port: PortSnapshot[];
} {
  return {
    control: [
      ...connectAndExpandControl(connectionId),
      {
        channel: IPC.treeDescribe,
        args: { connectionId, path: COMPOSITE_PK_PATH, refresh: false, tabId: null },
        response: { meta: COMPOSITE_PK_META, source: 'server' },
      },
    ],
    port: [
      {
        op: DATA_OP.read,
        payload: {
          connectionId,
          path: COMPOSITE_PK_PATH,
          projection: null,
          filter: null,
          sort: null,
          pageSize: 100,
          cursor: { mode: 'offset', offset: 0 },
        },
        response: {
          kind: 'read',
          page: {
            kind: 'tabular',
            columns: COMPOSITE_PK_COLUMNS,
            rows: [
              ['1', '1', 'tenant 1 / entity 1'],
              ['1', '2', 'tenant 1 / entity 2'],
              ['2', '1', 'tenant 2 / entity 1'],
            ],
            position: {
              offset: 0,
              pageSize: 100,
              hasMore: false,
              nextToken: null,
              prevToken: null,
              strategy: 'keyset',
            },
            truncatedCells: 0,
          },
          source: 'server',
        },
      },
    ],
  };
}

// database:kira_test/schema:app's own children — the full real capture (every table/view/matview/
// sequence/function `0001_seed.sql` creates), kept here rather than inline in orderItemsFixture()
// so a spec needing to find a *different* table under app (e.g. `composite_pk`) still gets a
// correct, real children list to search rather than one trimmed to what order_items alone needed.
export const APP_CHILDREN = [
  {
    kind: 'table',
    name: 'Order Items',
    path: `${APP_PATH}/table:Order%20Items`,
    hasChildren: false,
  },
  {
    kind: 'table',
    name: 'big_rows',
    path: `${APP_PATH}/table:big_rows`,
    hasChildren: false,
    detail: '~1M',
  },
  {
    kind: 'table',
    name: 'composite_pk',
    path: `${APP_PATH}/table:composite_pk`,
    hasChildren: false,
  },
  { kind: 'table', name: 'customers', path: `${APP_PATH}/table:customers`, hasChildren: false },
  { kind: 'table', name: 'employees', path: `${APP_PATH}/table:employees`, hasChildren: false },
  { kind: 'table', name: 'formats', path: `${APP_PATH}/table:formats`, hasChildren: false },
  { kind: 'table', name: 'nested_json', path: `${APP_PATH}/table:nested_json`, hasChildren: false },
  {
    kind: 'table',
    name: 'nulls_and_unicode',
    path: `${APP_PATH}/table:nulls_and_unicode`,
    hasChildren: false,
  },
  { kind: 'table', name: 'order_items', path: ORDER_ITEMS_PATH, hasChildren: false },
  { kind: 'table', name: 'orders', path: `${APP_PATH}/table:orders`, hasChildren: false },
  { kind: 'table', name: 'products', path: `${APP_PATH}/table:products`, hasChildren: false },
  { kind: 'table', name: 'regions', path: `${APP_PATH}/table:regions`, hasChildren: false },
  { kind: 'table', name: 'weird"name', path: `${APP_PATH}/table:weird%22name`, hasChildren: false },
  { kind: 'table', name: 'wide_table', path: `${APP_PATH}/table:wide_table`, hasChildren: false },
  {
    kind: 'view',
    name: 'order_summary',
    path: `${APP_PATH}/view:order_summary`,
    hasChildren: false,
  },
  {
    kind: 'matview',
    name: 'customer_totals',
    path: `${APP_PATH}/matview:customer_totals`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'Order Items_id_seq',
    path: `${APP_PATH}/sequence:Order%20Items_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'customers_id_seq',
    path: `${APP_PATH}/sequence:customers_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'employees_id_seq',
    path: `${APP_PATH}/sequence:employees_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'formats_id_seq',
    path: `${APP_PATH}/sequence:formats_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'invoice_number_seq',
    path: `${APP_PATH}/sequence:invoice_number_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'nested_json_id_seq',
    path: `${APP_PATH}/sequence:nested_json_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'nulls_and_unicode_id_seq',
    path: `${APP_PATH}/sequence:nulls_and_unicode_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'order_items_id_seq',
    path: `${APP_PATH}/sequence:order_items_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'orders_id_seq',
    path: `${APP_PATH}/sequence:orders_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'products_id_seq',
    path: `${APP_PATH}/sequence:products_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'regions_id_seq',
    path: `${APP_PATH}/sequence:regions_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'weird"name_id_seq',
    path: `${APP_PATH}/sequence:weird%22name_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'wide_table_id_seq',
    path: `${APP_PATH}/sequence:wide_table_id_seq`,
    hasChildren: false,
  },
  {
    kind: 'function',
    name: 'full_name',
    path: `${APP_PATH}/function:full_name`,
    hasChildren: false,
    detail: '(first_name text, last_name text)',
  },
  {
    kind: 'function',
    name: 'noop_procedure',
    path: `${APP_PATH}/function:noop_procedure`,
    hasChildren: false,
    detail: '()',
  },
];

/** A plausible ConnectionSummary for a postgres connection created through the dialog — the
 *  `connectionsCreate` response a spec supplies its own args/name/color for. */
export function postgresConnectionSummary(
  id: string,
  name: string,
  color: ConnectionSummary['color'],
): ConnectionSummary {
  return sharedConnectionSummary('postgres', 5432, 'kira_test', 'postgres', id, name, color);
}

export const BIG_ROWS_PATH = `${APP_PATH}/table:big_rows`;
export const NULLS_PATH = `${APP_PATH}/table:nulls_and_unicode`;

/** Real captured describe() for app.big_rows (1,000,000 rows, id/hash) — packages/db-fixtures/fixtures/0001_seed.sql. */
export const BIG_ROWS_META = {
  path: 'database:kira_test/schema:app/table:big_rows',
  kind: 'table',
  name: 'big_rows',
  qualifiedName: 'app.big_rows',
  columns: [
    {
      name: 'id',
      position: 1,
      dataType: 'integer',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'hash',
      position: 2,
      dataType: 'text',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
  ],
  primaryKey: ['id'],
  foreignKeys: [],
  referencedBy: [],
  indexes: [
    {
      name: 'big_rows_pkey',
      columns: ['id'],
      unique: true,
      primary: true,
      method: 'btree',
    },
  ],
  rowEstimate: 1000000,
  comment: null,
};

/** Real captured describe() for app.nulls_and_unicode. */
export const NULLS_META = {
  path: 'database:kira_test/schema:app/table:nulls_and_unicode',
  kind: 'table',
  name: 'nulls_and_unicode',
  qualifiedName: 'app.nulls_and_unicode',
  columns: [
    {
      name: 'id',
      position: 1,
      dataType: 'integer',
      nullable: false,
      defaultExpr: "nextval('app.nulls_and_unicode_id_seq'::regclass)",
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'label',
      position: 2,
      dataType: 'text',
      nullable: true,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'note',
      position: 3,
      dataType: 'text',
      nullable: true,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'big_text',
      position: 4,
      dataType: 'text',
      nullable: true,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'big_blob',
      position: 5,
      dataType: 'bytea',
      nullable: true,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
  ],
  primaryKey: ['id'],
  foreignKeys: [],
  referencedBy: [],
  indexes: [
    {
      name: 'nulls_and_unicode_pkey',
      columns: ['id'],
      unique: true,
      primary: true,
      method: 'btree',
    },
  ],
  rowEstimate: null,
  comment: null,
};

export const BIG_ROWS_COLUMNS: ColumnDescriptor[] = [
  {
    name: 'id',
    dataType: 'integer',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: true,
    generated: false,
  },
  {
    name: 'hash',
    dataType: 'text',
    typeClass: 'text',
    nullable: false,
    isPrimaryKey: false,
    generated: false,
  },
];

const BIG_ROWS_FIRST_PAGE: LogicalPage = {
  kind: 'tabular',
  columns: BIG_ROWS_COLUMNS,
  rows: MD5_ROWS,
  position: {
    offset: 0,
    pageSize: 100,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjEwMCJdLCJmIjoiNzY3Y2NlNDM1NDhmNjc5ZiJ9',
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};

/** Real captured pageSize-100 read of app.nulls_and_unicode's 4 rows (NULL/empty/unicode/oversized
 *  — the last row's big_text/big_blob are genuinely truncated by the server, truncatedCells: 2). */
export const NULLS_AND_UNICODE_PAGE: LogicalPage = {
  kind: 'tabular',
  columns: NULLS_AND_UNICODE_COLUMNS,
  rows: NULLS_AND_UNICODE_ROWS,
  position: {
    offset: 0,
    pageSize: 100,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 2,
};

/** A connectable postgres fixture opened straight to `app.big_rows` at the stock pageSize-100
 *  read — the boilerplate data-view.spec.ts (and any future heavy-pagination spec) needs before
 *  layering its own extra page-size/sort/filter/count captures on top. */
export function bigRowsFixture(connectionId: string): {
  control: ControlSnapshot[];
  port: PortSnapshot[];
} {
  return {
    control: [
      ...connectAndExpandControl(connectionId),
      {
        channel: IPC.treeDescribe,
        args: { connectionId, path: BIG_ROWS_PATH, refresh: false, tabId: null },
        response: { meta: BIG_ROWS_META, source: 'server' },
      },
    ],
    port: [
      {
        op: DATA_OP.read,
        payload: {
          connectionId,
          path: BIG_ROWS_PATH,
          projection: null,
          filter: null,
          sort: null,
          pageSize: 100,
          cursor: { mode: 'offset', offset: 0 },
        },
        response: { kind: 'read', page: BIG_ROWS_FIRST_PAGE, source: 'server' },
      },
    ],
  };
}

/** A connectable postgres fixture opened straight to `app.nulls_and_unicode`. */
export function nullsAndUnicodeFixture(connectionId: string): {
  control: ControlSnapshot[];
  port: PortSnapshot[];
} {
  return {
    control: [
      ...connectAndExpandControl(connectionId),
      {
        channel: IPC.treeDescribe,
        args: { connectionId, path: NULLS_PATH, refresh: false, tabId: null },
        response: { meta: NULLS_META, source: 'server' },
      },
    ],
    port: [
      {
        op: DATA_OP.read,
        payload: {
          connectionId,
          path: NULLS_PATH,
          projection: null,
          filter: null,
          sort: null,
          pageSize: 100,
          cursor: { mode: 'offset', offset: 0 },
        },
        response: { kind: 'read', page: NULLS_AND_UNICODE_PAGE, source: 'server' },
      },
    ],
  };
}

// `app.big_rows`' content is fully deterministic (`id`, `hash=md5(id::text)` —
// packages/db-fixtures/support/postgres.ts's own seed) — generated here rather than inlined as a 10
// 000-row
// literal array, the same reasoning tests/ui/data-view.spec.ts's own `bigRowsRows` helper already
// documents (verified byte-for-byte against a real capture's first/last rows there). Exported
// (unlike that file-local copy) because tests/ui/budgets.spec.ts and tests/ui/perf.spec.ts both
// need the identical pageSize=10000 page.
export function bigRowsRow(id: number): [string, string] {
  return [String(id), createHash('md5').update(String(id)).digest('hex')];
}
export function bigRowsRows(count: number, startId: number): [string, string][] {
  return Array.from({ length: count }, (_, i) => bigRowsRow(startId + i));
}

/** `app.big_rows` at pageSize=10000, offset=0 — the page `budgets.spec.ts`/`perf.spec.ts` both
 *  scroll. `nextToken` is a real capture (scripts/capture-postgres-tree.ts) — byte-identical to
 *  data-view.spec.ts's own PAGE_E, confirming the token is a pure function of the page's own
 *  content (keyset value + filter hash), not something that could drift between captures. */
export function bigRowsHugePage(connectionId: string): PortSnapshot {
  return {
    op: DATA_OP.read,
    payload: {
      connectionId,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 10000,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: BIG_ROWS_COLUMNS,
        rows: bigRowsRows(10000, 1),
        position: {
          offset: 0,
          pageSize: 10000,
          hasMore: true,
          nextToken: 'eyJ2IjoxLCJrIjpbIjEwMDAwIl0sImYiOiI1YWIwNzAyNWZmMWNkZmE1In0',
          prevToken: null,
          strategy: 'keyset',
        },
        truncatedCells: 0,
      },
      source: 'server',
    },
  };
}
