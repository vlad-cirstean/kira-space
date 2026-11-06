import {
  attachResults,
  FLICK_LADDER,
  type FlickResult,
  measureFlick,
  RssSampler,
} from '@workbench/testing/ui/perfProbe';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../../ui/fixtures';
import { createConnection } from '../../ui/studio/support/connect';
import { mongoCreateArgs } from '../../ui/studio/support/connectionArgs';
import {
  DB_PATH,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from '../../ui/studio/support/mongoFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from '../../ui/studio/support/tree';
import { IPC } from '../../ui/support/ipcChannels';

// Document view: NDOCS (default 5000) documents of ~1 KB each, momentum flicks.

const CONNECTION_ID = 'conn-perf-docs';
const NDOCS = Number(process.env.NDOCS ?? 5000);
const FIXTURE = widgetsFixture(CONNECTION_ID);

const oid = (i: number): string => `{"$oid":"${i.toString(16).padStart(24, '0')}"}`;
const body = (i: number): string =>
  JSON.stringify({
    _id: JSON.parse(oid(i)),
    name: `widget-${i}`,
    price: { $numberInt: String(i) },
    active: i % 2 === 0,
    tags: ['alpha', 'beta', 'gamma', `t${i}`],
    dims: { w: { $numberInt: '10' }, h: { $numberInt: '20' }, notes: 'x'.repeat(200) },
    history: Array.from({ length: 8 }, (_, k) => ({
      at: { $date: { $numberLong: String(1704067200000 + k * 1000) } },
      by: `user-${k}`,
      note: 'changed field value '.repeat(3),
    })),
  });

const ids = Array.from({ length: NDOCS }, (_, i) => oid(i));
const bodies = Array.from({ length: NDOCS }, (_, i) => body(i));
const PORT: PortSnapshot[] = FIXTURE.port.map((snap, index) => {
  if (index !== 0) return snap;
  const response = snap.response as unknown as { page: Record<string, unknown> };
  return {
    ...snap,
    response: { ...response, page: { ...response.page, ids, bodies } },
  } as PortSnapshot;
});
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: mongoCreateArgs('Perf Mongo'),
    response: mongoConnectionSummary(CONNECTION_ID, 'Perf Mongo', 'red'),
  },
  ...FIXTURE.control,
];

test('perf: document view, momentum flicks', async ({ relaunch }, info) => {
  test.setTimeout(180_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await createConnection(page, 'mongodb', 'Perf Mongo');
  const connRow = connectionRow(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await (await findRow(page, WIDGETS_PATH)).dblclick();

  const list = page.locator('[data-testid="document-view"] [data-testid="virtual-list"]');
  await expect(list).toBeVisible({ timeout: 30_000 });
  const box = await list.boundingBox();
  if (!box) throw new Error('document list has no box');
  await page.mouse.move(box.x + 200, box.y + 200);

  const rss = new RssSampler();
  rss.start();
  const results: FlickResult[] = [];
  for (const { label, opts } of FLICK_LADDER) {
    results.push(await measureFlick(page, rss, label, opts));
  }
  rss.stop();
  await attachResults(info, results);
});
