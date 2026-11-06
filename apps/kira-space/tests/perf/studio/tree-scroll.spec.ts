import {
  attachResults,
  FLICK_LADDER,
  type FlickResult,
  measureFlick,
  RssSampler,
} from '@workbench/testing/ui/perfProbe';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from '../../ui/studio/fixtures';
import { connectAndExpand, connectionCreateArgs } from '../../ui/studio/support/connect';
import { IPC } from '../../ui/studio/support/ipcChannels';
import {
  APP_CHILDREN,
  APP_PATH,
  connectAndExpandControl,
  postgresConnectionSummary,
} from '../../ui/studio/support/postgresFixture';
import { expandRow } from '../../ui/studio/support/tree';

// Project tree: one schema with NTABLES (default 5000) tables, expand time plus momentum flicks.

const CONNECTION_ID = 'conn-perf-tree';
const NTABLES = Number(process.env.NTABLES ?? 5000);
const nodes = [
  ...APP_CHILDREN,
  ...Array.from({ length: NTABLES }, (_, i) => ({
    kind: 'table' as const,
    name: `bulk_table_${String(i).padStart(5, '0')}`,
    path: `${APP_PATH}/table:bulk_table_${i}`,
    hasChildren: false,
    detail: `~${i} rows`,
  })),
];
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Perf DB', 'cyan'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Perf DB', 'cyan'),
  },
  ...connectAndExpandControl(CONNECTION_ID).map((snap) =>
    snap.channel === IPC.treeChildren && (snap.args as { path?: string })?.path === APP_PATH
      ? { ...snap, response: { nodes, source: 'server', truncated: false } }
      : snap,
  ),
];

test('perf: project tree, large schema', async ({ relaunch }, info) => {
  test.setTimeout(180_000);
  const { window: page } = await relaunch({ control: CONTROL });
  // connectAndExpand also expands APP_PATH; time that step separately by re-collapsing it first.
  await connectAndExpand(page, 'Perf DB', 'cyan');
  await expandRow(page, APP_PATH);
  const expandStart = Date.now();
  await expandRow(page, APP_PATH);
  await expect(
    page.locator('[data-testid="tree-row"]', { hasText: 'bulk_table_00000' }),
  ).toBeVisible();
  console.log(`PERF tree expand (${NTABLES} children): ${Date.now() - expandStart} ms`);

  const list = page.locator('[data-testid="virtual-list"]').first();
  const box = await list.boundingBox();
  if (!box) throw new Error('tree list has no box');
  await page.mouse.move(box.x + 60, box.y + 200);

  const rss = new RssSampler();
  rss.start();
  const results: FlickResult[] = [];
  for (const { label, opts } of FLICK_LADDER) {
    results.push(await measureFlick(page, rss, label, opts));
  }
  rss.stop();
  await attachResults(info, results);
});
