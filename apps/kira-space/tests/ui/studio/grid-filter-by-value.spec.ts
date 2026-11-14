import type { ConnectionKind } from '@shared/domain/connection';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionSummary } from './support/engineFixture';
import { gridCell } from './support/grid';
import { POSTGRES_CAPS, SERVER_VERSION } from './support/postgresFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// J-BC-032, UI half. The Go half (dbflow TestGridFilterByValue) runs each engine against hostile
// values and commits the filter that finds exactly the cell's row, in contract studio-filter-by-value.
// This half clicks "Filter by this value" on every cell and compares the filter the grid sends with
// that contract, so a quoting rule that drifts from the proven one fails here.

const CASES = contract<{ label: string; value: string | null }[]>(
  'studio-filter-by-value',
  'cases',
);
const FILTERS = contract<Record<string, Record<string, string>>>(
  'studio-filter-by-value',
  'filters',
);
const MATCHES = contract<Record<string, Record<string, (string | null)[][]>>>(
  'studio-filter-by-value',
  'matches',
);

const KINDS: Record<string, ConnectionKind> = {
  sqlite: 'sqlite',
  postgres: 'postgres',
  mariadb: 'mariadb',
  clickhouse: 'clickhouse',
};

const DB_PATH = 'database:kira_test';
const TABLE_PATH = `${DB_PATH}/table:fbv`;
const COLUMNS: ColumnDescriptor[] = [
  {
    name: 'id',
    dataType: 'integer',
    typeClass: 'number',
    nullable: false,
    isPrimaryKey: true,
    generated: false,
  },
  {
    name: 'v',
    dataType: 'text',
    typeClass: 'text',
    nullable: true,
    isPrimaryKey: false,
    generated: false,
  },
];
const META = {
  path: TABLE_PATH,
  kind: 'table',
  name: 'fbv',
  qualifiedName: 'fbv',
  columns: COLUMNS.map((c, i) => ({
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
  indexes: [],
  rowEstimate: CASES.length,
  comment: null,
};

function page(rows: (string | null)[][]): LogicalPage {
  return {
    kind: 'tabular',
    columns: COLUMNS,
    rows,
    position: {
      offset: 0,
      pageSize: 100,
      hasMore: false,
      nextToken: null,
      prevToken: null,
      strategy: 'keyset',
    },
    truncatedCells: 0,
  };
}

const ALL_ROWS = CASES.map((c, i) => [String(i + 1), c.value]);

for (const [engine, kind] of Object.entries(KINDS)) {
  test(`filter by this value sends the contract filter on ${engine}`, async ({ relaunch }) => {
    test.setTimeout(240_000);
    const id = `conn-fbv-${engine}`;
    const summary = connectionSummary(
      kind,
      0,
      'kira_test',
      'kira',
      id,
      `Filter ${engine}`,
      'green',
    );
    const read = (filter: string | null) => ({
      op: DATA_OP.read,
      payload: {
        connectionId: id,
        path: TABLE_PATH,
        projection: null,
        filter,
        sort: null,
        pageSize: 100,
        cursor: { mode: 'offset', offset: 0 },
      },
    });
    const control: ControlSnapshot[] = [
      { channel: IPC.connectionsList, response: [summary] },
      {
        channel: IPC.connectionsConnect,
        args: { id },
        response: {
          connectionId: id,
          status: 'connected',
          serverVersion: SERVER_VERSION,
          error: null,
          since: 1735689600000,
          caps: POSTGRES_CAPS,
        },
      },
      {
        channel: IPC.treeChildren,
        args: { connectionId: id, path: '', refresh: false },
        response: {
          nodes: [{ kind: 'database', name: 'kira_test', path: DB_PATH, hasChildren: true }],
          source: 'server',
          truncated: false,
        },
      },
      {
        channel: IPC.treeChildren,
        args: { connectionId: id, path: DB_PATH, refresh: false },
        response: {
          nodes: [
            { kind: 'table', name: 'fbv', path: TABLE_PATH, hasChildren: false, detail: '12 rows' },
          ],
          source: 'server',
          truncated: false,
        },
      },
      {
        channel: IPC.treeDescribe,
        args: { connectionId: id, path: TABLE_PATH, refresh: false, tabId: null },
        response: { meta: META, source: 'server' },
      },
    ];
    const stream: PortSnapshot[] = [
      { ...read(null), response: { kind: 'read', page: page(ALL_ROWS), source: 'server' } },
      ...CASES.flatMap((c) => [
        {
          ...read(FILTERS[engine][c.label]),
          response: { kind: 'read', page: page(MATCHES[engine][c.label]), source: 'server' },
        },
        { ...read(null), response: { kind: 'read', page: page(ALL_ROWS), source: 'server' } },
      ]),
    ] as PortSnapshot[];

    const { window: p, stream: handle } = await relaunch({ control, stream });
    const row = connectionRow(p, `Filter ${engine}`);
    await expect(row).toBeVisible();
    await openRowMenu(p, '');
    await p.click('[data-testid="menu-item-connect"]');
    await expect(row.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
      timeout: 10_000,
    });
    await expandRow(p, '');
    await expandRow(p, DB_PATH);
    await (await findRow(p, TABLE_PATH)).dblclick();
    await expect(gridCell(p, 0, 'v')).toBeVisible();

    for (const [i, c] of CASES.entries()) {
      await gridCell(p, i, 'v').click({ button: 'right' });
      await p.click('[data-testid="menu-item-filter-by-value"]');
      await expect(p.locator('[data-testid="filter-where-input"]')).toHaveValue(
        FILTERS[engine][c.label],
      );
      await expect(p.locator('[data-testid="grid-row"]')).toHaveCount(1, { timeout: 10_000 });
      await p.fill('[data-testid="filter-where-input"]', '');
      await p.press('[data-testid="filter-where-input"]', 'Enter');
      await expect(p.locator('[data-testid="grid-row"]')).toHaveCount(CASES.length, {
        timeout: 10_000,
      });
    }
    const sent = (await handle.ops())
      .filter((o) => o.op === DATA_OP.read)
      .map((o) => (o.payload as { filter: string | null }).filter)
      .filter((f) => f !== null);
    expect(sent).toEqual(CASES.map((c) => FILTERS[engine][c.label]));
  });
}
