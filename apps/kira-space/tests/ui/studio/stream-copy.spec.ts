import { DATA_OP } from '@shared/protocol/dataOps';
import {
  controlSnapshots as kafkaControl,
  portSnapshots as kafkaPorts,
} from '../../ipc/kafka/kafka.fixture';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { installClipboardSpy, lastClipboardWrite } from './support/clipboard';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// J-STR-010 (OQ-STR-1), UI half. The Go half (TestKafkaBrowseTombstoneHeadersAndLostPosition) reads a
// tombstone, a record with repeated headers and a null-key empty-body record from a real broker and
// records the page. Here the same rows must show a null marker for the tombstone and copy as JSON:
// null stays null (never an empty string), repeated header names keep their order and repeats.

const TOPIC_PATH = 'topic:orders';
const ROWS = contract<(string | null)[][]>('studio-kafka-browse', 'read:shapes');

const ordersRead = kafkaPorts.find(
  (s) => s.op === DATA_OP.read && (s.payload as { path: string }).path === TOPIC_PATH,
) as PortSnapshot;
const SHAPES_READ = {
  ...ordersRead,
  response: {
    kind: 'read',
    page: {
      kind: 'stream',
      keys: ROWS.map((r) => r[0]),
      headers: ROWS.map((r) => r[1]),
      attrs: ROWS.map((r) => r[2]),
      timestamps: ROWS.map((r) => r[3]),
      bodies: ROWS.map((r) => r[4]),
      position: {
        offset: null,
        pageSize: 100,
        hasMore: false,
        nextToken: null,
        prevToken: null,
        strategy: 'offsetWindow',
      },
      visibilityTimeoutSeconds: null,
    },
    source: 'server',
  },
} as PortSnapshot;

// The copied row, as the oracle states it: cells as the broker returned them, parsed where they are JSON.
const expectedCopy = (r: (string | null)[]) => ({
  key: r[0],
  headers: JSON.parse(r[1] as string),
  attributes: JSON.parse(r[2] as string),
  timestamp: r[3],
  body: r[4],
});

test('a tombstone shows a null marker and copies as JSON null; repeated headers keep their repeats', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: kafkaControl as ControlSnapshot[],
    stream: [SHAPES_READ, ...kafkaPorts.filter((s) => s !== ordersRead)],
  });
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await (await findRow(page, TOPIC_PATH)).dblclick();
  const view = page.locator(`[data-testid="stream-view"][data-path="${TOPIC_PATH}"]`);
  const rows = view.locator('[data-testid="stream-row"]');
  await expect(rows).toHaveCount(3, { timeout: 15_000 });
  await installClipboardSpy(page);

  // The tombstone shows the marker; the empty-body record does not.
  await expect(rows.nth(0).locator('[data-testid="stream-body-null"]')).toBeVisible();
  await expect(rows.nth(2).locator('[data-testid="stream-body-null"]')).toHaveCount(0);

  for (const [index, row] of ROWS.entries()) {
    await rows.nth(index).click({ button: 'right' });
    await page.click('[data-testid="menu-item-copy-row"]');
    await expect.poll(async () => lastClipboardWrite(page)).not.toBe('');
    const copied = JSON.parse(await lastClipboardWrite(page));
    expect(copied).toEqual(expectedCopy(row));
    await page.evaluate(() => {
      (window as unknown as { __clipboard: string[] }).__clipboard = [];
    });
  }

  // A tombstone offers no Copy body: an empty paste would read as an empty message.
  await rows.nth(0).click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-copy-body"]')).toHaveCount(0);
  await page.keyboard.press('Escape');
  await rows.nth(1).click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-copy-body"]')).toHaveCount(1);
});
