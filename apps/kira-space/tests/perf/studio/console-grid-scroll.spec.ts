import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import {
  attachResults,
  FLICK_LADDER,
  type FlickResult,
  measureFlick,
  RssSampler,
} from '@workbench/testing/ui/perfProbe';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../../ui/fixtures';
import {
  connectAndExpand,
  connectionCreateArgs,
  openConsoleFromMenu,
} from '../../ui/studio/support/connect';
import { typeInto } from '../../ui/studio/support/editor';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from '../../ui/studio/support/postgresFixture';
import { IPC } from '../../ui/support/ipcChannels';

// Console result grid: NROWS x NCOLS text cells. Visible column count drives frame cost.

const CONNECTION_ID = 'conn-perf-console';
const NROWS = Number(process.env.NROWS ?? 10_000);
const NCOLS = Number(process.env.NCOLS ?? 12);
const FIXTURE = orderItemsFixture(CONNECTION_ID);

const columns: ColumnDescriptor[] = Array.from({ length: NCOLS }, (_, i) => ({
  name: `c${i}`,
  dataType: i === 0 ? 'int4' : 'text',
  typeClass: i === 0 ? 'number' : 'text',
  nullable: true,
  isPrimaryKey: false,
  generated: false,
}));
const rows = Array.from({ length: NROWS }, (_, r) =>
  columns.map((_, c) => (c === 0 ? String(r) : `value ${r}-${c} lorem ipsum dolor sit amet`)),
);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Perf DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Perf DB', 'green'),
  },
  ...FIXTURE.control,
];
const PORT: PortSnapshot[] = [
  ...FIXTURE.port,
  {
    op: DATA_OP.execute,
    payload: {
      connectionId: CONNECTION_ID,
      path: ORDER_ITEMS_PATH,
      statements: ['SELECT * FROM big'],
    },
    response: {
      kind: 'execute',
      pages: [
        {
          kind: 'tabular',
          columns,
          rows,
          position: {
            offset: 0,
            pageSize: NROWS,
            hasMore: false,
            nextToken: null,
            prevToken: null,
            strategy: 'offset',
          },
          truncatedCells: 0,
        },
      ],
    },
  },
];

test('perf: console result grid, momentum flicks', async ({ relaunch }, info) => {
  test.setTimeout(180_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Perf DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, 'SELECT * FROM big;');

  const started = Date.now();
  await page.click('[data-testid="console-run-statement"]');
  const grid = view.locator('[data-testid="console-result-grid"]').first();
  await expect(grid).toContainText('value 0-1', { timeout: 60_000 });
  console.log(`PERF console run -> rows visible: ${Date.now() - started} ms (${NROWS} x ${NCOLS})`);

  const viewport = view
    .locator('[data-testid="console-result-grid"] .slick-viewport-right')
    .first();
  const box = await viewport.boundingBox();
  if (!box) throw new Error('result grid viewport has no box');
  await page.mouse.move(box.x + 200, box.y + Math.min(80, box.height / 2));

  const rss = new RssSampler();
  rss.start();
  const results: FlickResult[] = [];
  for (const { label, opts } of FLICK_LADDER) {
    results.push(await measureFlick(page, rss, label, opts));
  }
  rss.stop();
  await attachResults(info, results);
});
