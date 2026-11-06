import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, PortSnapshot } from '../../../ipc/support/types';
import { connectionSummary, orderItemsFixture } from '../../../ui/studio/support/engineFixture';
import { POSTGRES_CAPS, SERVER_VERSION } from '../../../ui/studio/support/postgresFixture';

// Fictional shop data for the presentation screenshots (P280): no real people, no secrets.

export const CONNECTION_ID = 'conn-acme-shop';
export const CONNECTION_NAME = 'Acme shop';
export const DB_PATH = 'database:acme_shop';
export const SCHEMA_PATH = `${DB_PATH}/schema:public`;
export const ORDERS_PATH = `${SCHEMA_PATH}/table:orders`;

export const SUMMARY = connectionSummary(
  'postgres',
  5432,
  'acme_shop',
  'shop_app',
  CONNECTION_ID,
  CONNECTION_NAME,
  'blue',
);

const table = (name: string, detail: string) => ({
  kind: 'table',
  name,
  path: `${SCHEMA_PATH}/table:${name}`,
  hasChildren: false,
  detail,
});

const SCHEMA_CHILDREN = [
  table('customers', '~1.2k rows'),
  table('order_items', '~9.8k rows'),
  table('orders', '~3.4k rows'),
  table('products', '~240'),
];

export const col = (
  name: string,
  dataType: string,
  typeClass: ColumnDescriptor['typeClass'],
  isPrimaryKey = false,
): ColumnDescriptor => ({
  name,
  dataType,
  typeClass,
  nullable: !isPrimaryKey,
  isPrimaryKey,
  generated: false,
});

export const ORDER_COLUMNS: ColumnDescriptor[] = [
  col('id', 'integer', 'number', true),
  col('customer', 'text', 'text'),
  col('status', 'text', 'text'),
  col('items', 'integer', 'number'),
  col('total', 'numeric(10,2)', 'number'),
  col('shipped', 'boolean', 'boolean'),
  col('created_at', 'timestamp', 'temporal'),
];

const CUSTOMERS = [
  'Alice Morgan',
  'Ben Carter',
  'Chloe Hughes',
  'Daniel Brooks',
  'Emma Wright',
  'Oliver Grant',
  'Grace Turner',
  'Henry Walsh',
];
const STATUSES = ['paid', 'shipped', 'delivered', 'pending', 'delivered', 'shipped'];

const ORDER_ROWS: (string | null)[][] = Array.from({ length: 30 }, (_, i) => {
  const n = 4200 + i;
  const status = STATUSES[i % STATUSES.length] as string;
  const items = (i % 5) + 1;
  const total = (18.5 + ((i * 37) % 220) + items * 4.25).toFixed(2);
  const day = String(1 + (i % 28)).padStart(2, '0');
  const hour = String(8 + (i % 11)).padStart(2, '0');
  return [
    String(n),
    CUSTOMERS[i % CUSTOMERS.length] as string,
    status,
    String(items),
    total,
    status === 'shipped' || status === 'delivered' ? 'true' : 'false',
    `2026-09-${day} ${hour}:${String((i * 7) % 60).padStart(2, '0')}:00`,
  ];
});

const META = {
  path: ORDERS_PATH,
  kind: 'table' as const,
  name: 'orders',
  qualifiedName: 'public.orders',
  columns: ORDER_COLUMNS.map((c, i) => ({
    name: c.name,
    position: i + 1,
    dataType: c.dataType,
    nullable: c.nullable,
    defaultExpr: null,
    isPrimaryKey: c.isPrimaryKey,
    comment: null,
  })),
  primaryKey: ['id'],
  foreignKeys: [],
  referencedBy: [],
  indexes: [{ name: 'orders_pkey', columns: ['id'], unique: true, primary: true, method: 'btree' }],
  rowEstimate: null,
  comment: null,
};

/** Connect, expand to the `public` schema and open `orders` at both stock page sizes. */
export const ACME: { control: ControlSnapshot[]; port: PortSnapshot[] } = orderItemsFixture(
  CONNECTION_ID,
  SERVER_VERSION,
  POSTGRES_CAPS,
  [
    {
      path: '',
      children: [{ kind: 'database', name: 'acme_shop', path: DB_PATH, hasChildren: true }],
    },
    {
      path: DB_PATH,
      children: [{ kind: 'schema', name: 'public', path: SCHEMA_PATH, hasChildren: true }],
    },
    { path: SCHEMA_PATH, children: SCHEMA_CHILDREN },
  ],
  ORDERS_PATH,
  META,
  ORDER_COLUMNS,
  [
    { filter: null, pageSize: 100, rows: ORDER_ROWS },
    { filter: null, pageSize: 1000, rows: ORDER_ROWS },
  ],
);
