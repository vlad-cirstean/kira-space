import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contract approvals-api-kept-review. Backend half: dbmcpflow TestKeptScriptReviewHeld. A held response
// whose transform keeps a value lists it under "Will keep" with a preview; nothing is stored yet.

interface Keep {
  name: string;
  type: string;
  items: number;
  bytes: number;
  secret: boolean;
  preview: string;
  previewTruncated: boolean;
}
interface Held {
  requestId: string;
  keep: Keep[];
  [k: string]: unknown;
}

const SC = 'approvals-api-kept-review';
const item = () => contract<ApprovalItem[]>(SC, 'ApprovalsService.List')[0] as ApprovalItem;
const held = () => contract<Held>(SC, 'DbMcpService.PendingResponse');
const mocks = (h: Held) => [{ channel: IPC.dbMcpPendingResponse, response: h }];

test('contract: the review lists the value the script will keep', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: mocks(held()) });
  await pushApprovals(page, [liveItem(item())]);
  const block = page.locator('[data-testid="api-review-keep"]');
  await expect(block).toContainText('Will keep');
  const row = block.locator('[data-testid="api-review-keep-ids"]');
  await expect(row).toContainText('List of 2');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toContainText('9007199254740993');
});

test('a cut preview offers no whole value: nothing is stored yet', async ({ relaunch }) => {
  const h = structuredClone(held());
  h.keep = h.keep.map((k) => ({ ...k, preview: `[${'1,'.repeat(40)}`, previewTruncated: true }));
  const { window: page } = await relaunch({ control: mocks(h) });
  await pushApprovals(page, [liveItem(item())]);
  const row = page.locator('[data-testid="api-review-keep-ids"]');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toBeVisible();
  await expect(row.locator('[data-testid="kept-value-copy"]')).toHaveCount(0);
  await expect(row.getByText('Show all')).toHaveCount(0);
});

test('a value copied from a secret shows dots', async ({ relaunch }) => {
  const h = structuredClone(held());
  h.keep = h.keep.map((k) => ({ ...k, secret: true, preview: '' }));
  const { window: page } = await relaunch({ control: mocks(h) });
  await pushApprovals(page, [liveItem(item())]);
  const row = page.locator('[data-testid="api-review-keep-ids"]');
  await expect(row.locator('[data-testid="kept-value-secret"]')).toHaveText('••••');
  await expect(row.locator('[data-testid="kept-value-copy"]')).toHaveCount(0);
});

test('a review without kept values shows no Will keep block', async ({ relaunch }) => {
  const h = structuredClone(held());
  h.keep = [];
  const { window: page } = await relaunch({ control: mocks(h) });
  await pushApprovals(page, [liveItem(item())]);
  await expect(page.locator('[data-testid="api-review-body"]')).toBeVisible();
  await expect(page.locator('[data-testid="api-review-keep"]')).toHaveCount(0);
});
