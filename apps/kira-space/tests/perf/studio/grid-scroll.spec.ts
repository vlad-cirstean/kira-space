import {
  attachResults,
  FLICK_LADDER,
  type FlickResult,
  measureFlick,
  RssSampler,
} from '@workbench/testing/ui/perfProbe';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../../ui/fixtures';
import { connectAndExpand, connectionCreateArgs } from '../../ui/studio/support/connect';
import { gridScroller } from '../../ui/studio/support/grid';
import {
  BIG_ROWS_PATH,
  bigRowsFixture,
  bigRowsHugePage,
  postgresConnectionSummary,
} from '../../ui/studio/support/postgresFixture';
import { findRow } from '../../ui/studio/support/tree';
import { IPC } from '../../ui/support/ipcChannels';
import { wideColumns, wideRows } from './support/wideTable';

// Data grid: a 10 000-row page, momentum flicks down and up. Run `bun run perf:probe`.
// NCOLS (default 2, the stock `big_rows` shape) above 2 swaps in a wide page of mixed types.

const NCOLS = Number(process.env.NCOLS ?? 2);
// TRACE=1 wraps each flick in __kiraScrollTrace and logs uncoveredPx over the frames that moved.
const TRACE = process.env.TRACE === '1';

function describeUncovered(values: number[]): string {
  if (values.length === 0) return 'no moving frames';
  const sorted = [...values].sort((a, b) => a - b);
  const at = (q: number): number =>
    sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * q))];
  const gap = values.filter((v) => v > 0).length;
  return `p50 ${at(0.5).toFixed(0)}, p95 ${at(0.95).toFixed(0)}, max ${sorted[sorted.length - 1].toFixed(0)}, gap frames ${gap}/${values.length} (${((gap * 100) / values.length).toFixed(0)}%)`;
}
function widePage(connectionId: string): PortSnapshot {
  const columns = wideColumns(NCOLS);
  const rows = wideRows(NCOLS);
  const base = bigRowsHugePage(connectionId);
  const response = base.response as unknown as { page: Record<string, unknown> };
  return {
    ...base,
    response: { ...response, page: { ...response.page, columns, rows } },
  } as PortSnapshot;
}

const CONNECTION_ID = 'conn-perf-grid';
const FIXTURE = bigRowsFixture(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Perf DB', 'cyan'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Perf DB', 'cyan'),
  },
  ...FIXTURE.control,
];
const PORT: PortSnapshot[] = [
  ...FIXTURE.port,
  NCOLS > 2 ? widePage(CONNECTION_ID) : bigRowsHugePage(CONNECTION_ID),
];

test('perf: data grid, 10k rows, momentum flicks', async ({ relaunch }, info) => {
  test.setTimeout(180_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Perf DB', 'cyan');
  await (await findRow(page, BIG_ROWS_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await page.click('[data-testid="page-size-10000"]');
  const viewport = gridScroller(page);
  await expect
    .poll(() => viewport.evaluate((el) => el.scrollHeight), { timeout: 15_000 })
    .toBeGreaterThan(200_000);

  const box = await viewport.boundingBox();
  if (!box) throw new Error('grid viewport has no box');
  await page.mouse.move(box.x + 200, box.y + 200);

  console.log(`PERF viewport ${Math.round(box.width)}x${Math.round(box.height)}`);

  const rss = new RssSampler();
  rss.start();
  const results: FlickResult[] = [];
  for (const { label, opts } of FLICK_LADDER) {
    if (TRACE) await page.evaluate(() => window.__kiraScrollTrace?.start());
    results.push(await measureFlick(page, rss, label, opts));
    if (TRACE) {
      const trace = await page.evaluate(() => window.__kiraScrollTrace?.stop());
      const moving = (trace?.frames ?? [])
        .filter((f) => f.pxPerFrame > 0)
        .map((f) => f.uncoveredPx);
      console.log(`PERF ${label} uncoveredPx ${describeUncovered(moving)}`);
    }
  }
  rss.stop();
  await attachResults(info, results);
});
