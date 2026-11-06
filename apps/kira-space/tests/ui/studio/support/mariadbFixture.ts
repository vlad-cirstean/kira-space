import type { ConnectionSummary } from '@shared/domain/connection';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, PortSnapshot } from '../../../ipc/support/types';
import {
  connectAndExpandControl as sharedConnectAndExpandControl,
  connectionSummary as sharedConnectionSummary,
  orderItemsFixture as sharedOrderItemsFixture,
} from './engineFixture';

// Real captures against a real MariaDB container, seeded with
// packages/db-fixtures/fixtures/0002_mariadb_seed.sql
// — via `node out/scripts/capture-tree.cjs mariadb --recipe-file ...` (scripts/capture-tree.ts, the
// Postgres-only scripts/capture-postgres-tree.ts generalized to any
// packages/db-fixtures/support/<adapter>.ts
// fixture), not hand-written. Confirmed here: unlike Postgres (CLAUDE.md's
// Docker section, its own forListeningPorts() wait strategy hangs under `bun run` in this sandbox),
// MariaDB's container (Wait.forHealthCheck() only, no forListeningPorts()) starts and this whole
// capture completes fine under plain `bun run` — no esbuild/vendored-Node workaround needed for
// this adapter specifically, though the capture was still run once each way to be sure.
//
// `database:kira_test/table:order_items` is MariaDB's own tree shape — no schema level between
// database and table the way Postgres's `app` schema has one (mirrors tests/ipc/mariadb's own real
// capture, which this reuses the exact tree/column shapes of).

export const DB_PATH = 'database:kira_test';
export const ORDER_ITEMS_PATH = `${DB_PATH}/table:order_items`;

const SERVER_VERSION = 'MariaDB 11.4.13-MariaDB-ubu2404';

const CAPS = {
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

export const ROOT_CHILDREN = [
  { kind: 'database', name: 'kira_analytics', path: 'database:kira_analytics', hasChildren: true },
  {
    kind: 'database',
    name: 'kira_test',
    path: 'database:kira_test',
    hasChildren: true,
    detail: 'connected',
  },
];

// The real capture's full app.kira_test child list — kept whole (not trimmed to order_items
// alone) so a spec needing a different sibling node still gets a correct real listing.
export const DB_CHILDREN = [
  {
    kind: 'table',
    name: 'big_rows',
    path: `${DB_PATH}/table:big_rows`,
    hasChildren: false,
    detail: '~0',
  },
  {
    kind: 'table',
    name: 'composite_pk',
    path: `${DB_PATH}/table:composite_pk`,
    hasChildren: false,
    detail: '~3',
  },
  {
    kind: 'table',
    name: 'customers',
    path: `${DB_PATH}/table:customers`,
    hasChildren: false,
    detail: '~2',
  },
  {
    kind: 'table',
    name: 'employees',
    path: `${DB_PATH}/table:employees`,
    hasChildren: false,
    detail: '~3',
  },
  {
    kind: 'table',
    name: 'nested_json',
    path: `${DB_PATH}/table:nested_json`,
    hasChildren: false,
    detail: '~1',
  },
  {
    kind: 'table',
    name: 'nulls_and_unicode',
    path: `${DB_PATH}/table:nulls_and_unicode`,
    hasChildren: false,
    detail: '~4',
  },
  {
    kind: 'table',
    name: 'Order Items',
    path: `${DB_PATH}/table:Order%20Items`,
    hasChildren: false,
    detail: '~1',
  },
  {
    kind: 'table',
    name: 'orders',
    path: `${DB_PATH}/table:orders`,
    hasChildren: false,
    detail: '~2',
  },
  {
    kind: 'table',
    name: 'order_items',
    path: ORDER_ITEMS_PATH,
    hasChildren: false,
    detail: '~3',
  },
  {
    kind: 'table',
    name: 'products',
    path: `${DB_PATH}/table:products`,
    hasChildren: false,
    detail: '~2',
  },
  {
    kind: 'table',
    name: 'regions',
    path: `${DB_PATH}/table:regions`,
    hasChildren: false,
    detail: '~2',
  },
  {
    kind: 'table',
    name: 'weird`name',
    path: `${DB_PATH}/table:weird%60name`,
    hasChildren: false,
    detail: '~1',
  },
  {
    kind: 'table',
    name: 'wide_table',
    path: `${DB_PATH}/table:wide_table`,
    hasChildren: false,
    detail: '~2',
  },
  {
    kind: 'view',
    name: 'order_summary',
    path: `${DB_PATH}/view:order_summary`,
    hasChildren: false,
  },
  {
    kind: 'sequence',
    name: 'invoice_number_seq',
    path: `${DB_PATH}/sequence:invoice_number_seq`,
    hasChildren: false,
  },
  {
    kind: 'function',
    name: 'full_name',
    path: `${DB_PATH}/function:full_name`,
    hasChildren: false,
    detail: 'varchar(511)',
  },
  {
    kind: 'function',
    name: 'noop_procedure',
    path: `${DB_PATH}/function:noop_procedure`,
    hasChildren: false,
    detail: 'procedure',
  },
];

export const ORDER_ITEMS_COLUMNS: ColumnDescriptor[] = [
  {
    name: 'id',
    dataType: 'int(11)',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: true,
    generated: false,
  },
  {
    name: 'order_id',
    dataType: 'int(11)',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: false,
    generated: false,
  },
  {
    name: 'product_id',
    dataType: 'int(11)',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: false,
    generated: false,
  },
  {
    name: 'quantity',
    dataType: 'int(11)',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: false,
    generated: false,
  },
];

export const ORDER_ITEMS_META = {
  path: ORDER_ITEMS_PATH,
  kind: 'table' as const,
  name: 'order_items',
  qualifiedName: 'kira_test.order_items',
  columns: [
    {
      name: 'id',
      position: 1,
      dataType: 'int(11)',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'order_id',
      position: 2,
      dataType: 'int(11)',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'product_id',
      position: 3,
      dataType: 'int(11)',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'quantity',
      position: 4,
      dataType: 'int(11)',
      nullable: false,
      defaultExpr: '1',
      isPrimaryKey: false,
      comment: null,
    },
  ],
  primaryKey: ['id'],
  foreignKeys: [
    {
      name: 'fk_order_items_order',
      columns: ['order_id'],
      referencedPath: `${DB_PATH}/table:orders`,
      referencedColumns: ['id'],
      onDelete: 'RESTRICT',
      onUpdate: 'RESTRICT',
    },
    {
      name: 'fk_order_items_product',
      columns: ['product_id'],
      referencedPath: `${DB_PATH}/table:products`,
      referencedColumns: ['id'],
      onDelete: 'RESTRICT',
      onUpdate: 'RESTRICT',
    },
  ],
  referencedBy: [],
  indexes: [
    {
      name: 'fk_order_items_product',
      columns: ['product_id'],
      unique: false,
      primary: false,
      method: 'BTREE',
    },
    {
      name: 'order_items_order_product_idx',
      columns: ['order_id', 'product_id'],
      unique: true,
      primary: false,
      method: 'BTREE',
    },
    { name: 'PRIMARY', columns: ['id'], unique: true, primary: true, method: 'BTREE' },
  ],
  rowEstimate: 3,
  comment: null,
};

function orderItemsRows(filtered: boolean) {
  return filtered
    ? [
        ['1', '1', '1', '2'],
        ['3', '2', '1', '5'],
      ]
    : [
        ['1', '1', '1', '2'],
        ['2', '1', '2', '1'],
        ['3', '2', '1', '5'],
      ];
}

/** The connect + expand-to-`kira_test` boilerplate every fixture below shares. */
export function connectAndExpandControl(connectionId: string): ControlSnapshot[] {
  return sharedConnectAndExpandControl(connectionId, SERVER_VERSION, CAPS, [
    { path: '', children: ROOT_CHILDREN },
    { path: DB_PATH, children: DB_CHILDREN },
  ]);
}

/** Connect, expand to `kira_test`, open `order_items` (describe + unfiltered read) and — the one
 *  thing this port actually needs beyond definition.spec.ts's/mutations.spec.ts's own postgres
 *  shape — a real captured `quantity > 1` filtered read for the WHERE-autocomplete scenario. */
export function orderItemsFixture(connectionId: string): {
  control: ControlSnapshot[];
  port: PortSnapshot[];
} {
  return sharedOrderItemsFixture(
    connectionId,
    SERVER_VERSION,
    CAPS,
    [
      { path: '', children: ROOT_CHILDREN },
      { path: DB_PATH, children: DB_CHILDREN },
    ],
    ORDER_ITEMS_PATH,
    ORDER_ITEMS_META,
    ORDER_ITEMS_COLUMNS,
    [
      { filter: null, pageSize: 100, rows: orderItemsRows(false) },
      { filter: 'quantity > 1', pageSize: 100, rows: orderItemsRows(true) },
    ],
  );
}

export function mariadbConnectionSummary(
  id: string,
  name: string,
  color: ConnectionSummary['color'],
): ConnectionSummary {
  return sharedConnectionSummary('mariadb', 3306, 'kira_test', 'kira', id, name, color);
}
