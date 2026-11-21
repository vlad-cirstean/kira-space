import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../../ui/fixtures';
import { richApi } from '../../ui/studio/support/approvalItems';
import { withTimes } from '../../ui/studio/support/keptTimes';
import { contract } from '../../ui/support/contract';
import { IPC } from '../../ui/support/ipcChannels';
import { settleFonts } from './support/settle';

// P715: an API write with headers, a data review row resized taller, and a script card with its chain.

const mask = (page: import('@playwright/test').Page) => [
  page.locator('[data-testid="approval-countdown"]'),
];

test('api write with headers and variables', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, [liveItem(richApi('item#json'), 600, Date.now())]);
  const dialog = page.locator('[data-testid="approvals-dialog"]');
  await expect(page.locator('[data-testid="api-approval-headers"]')).toBeVisible();
  await settleFonts(page);
  await expect(dialog).toHaveScreenshot('approvals-api-write.png', { mask: mask(page) });
});

test('data review with a resized row', async ({ relaunch }) => {
  const [first] = contract<ApprovalItem[]>('dbmcp-preview', 'ApprovalsService.List');
  const held = contract<{ requestId: string; text: string }>(
    'dbmcp-preview',
    'DbMcpService.PendingResponse',
  );
  if (!first) throw new Error('contract has no item');
  const note =
    'A long note that wraps once its row is dragged taller, so the whole sentence reads without a popover.';
  const text = JSON.stringify({
    kind: 'tabular',
    columns: [
      { name: 'id', dataType: 'INTEGER', typeClass: 'number' },
      { name: 'note', dataType: 'TEXT', typeClass: 'text' },
    ],
    rows: [
      ['1', note],
      ['2', 'second'],
      ['3', 'third'],
    ],
    rowCount: 3,
    returned: 3,
    truncatedCells: 0,
  });
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: { ...held, text } },
      { channel: IPC.approvalsAnswer, response: answeredOk([first.id]) },
    ],
  });
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, [liveItem(first, 600, Date.now())]);
  const row = page.locator('[data-testid="db-mcp-preview-row"]').first();
  await expect(row).toBeVisible();
  const handle = row.locator('[data-testid="db-mcp-preview-row-resize"]').first();
  const box = await handle.boundingBox();
  if (!box) throw new Error('no handle');
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + 50, { steps: 6 });
  await page.mouse.up();
  await page.mouse.move(0, 0);
  await settleFonts(page);
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveScreenshot(
    'approvals-review-resized-row.png',
    { mask: mask(page) },
  );
});

test('script card with its chain', async ({ relaunch }) => {
  const [found] = contract<ApprovalItem[]>('e2e-kept-chain', 'ApprovalsService.List#step3');
  if (!found) throw new Error('contract has no item');
  const { window: page } = await relaunch({ control: [] });
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, [withTimes(found)]);
  await expect(page.locator('[data-testid="approval-chain"]')).toBeVisible();
  await settleFonts(page);
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveScreenshot(
    'approvals-script-chain.png',
    { mask: mask(page) },
  );
});
