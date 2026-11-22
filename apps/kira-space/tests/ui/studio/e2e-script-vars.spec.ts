import type { ApprovalItem } from '@shared/domain/approvals';
import type { DbMcpPendingResponse } from '@shared/domain/dbmcp';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { withTimes } from './support/keptTimes';

// Contract e2e-script-vars. Backend half: dbmcpflow TestE2EScriptVars, the same scenario. Scripts read and
// write saved variables through plain and secret: the prepare card marks both writes in its script and the
// request, the transform card marks writes and the delete, and the review lists what Send saves and deletes.

const SC = 'e2e-script-vars';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;

test('contract: the prepare card marks its plain and secret writes in the script', async ({
  relaunch,
}) => {
  const it = withTimes(item('prepare'));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = page.locator(`[data-testid="approval-item-${it.id}"]`);
  const script = c.locator('[data-testid="api-approval-script-prepare"]');
  await expect(script).toBeVisible();
  await expect(script.locator('.kira-ed-kept').first()).toBeVisible();
  await expect(c.locator('[data-testid="api-approval-prepared"]')).toBeVisible();
});

test('contract: the transform card marks writes and the delete', async ({ relaunch }) => {
  const it = withTimes(item('transform'));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = page.locator(`[data-testid="approval-item-${it.id}"]`);
  await expect(
    c.locator('[data-testid="api-approval-script-transform"] .kira-ed-kept'),
  ).toHaveCount(3);
});

test('contract: the review lists the saved values and the delete', async ({ relaunch }) => {
  const it = liveItem(item('review'));
  const held = contract<DbMcpPendingResponse>(SC, 'DbMcpService.PendingResponse#review');
  const { window: page } = await relaunch({
    control: [{ channel: IPC.dbMcpPendingResponse, response: held }],
  });
  await pushApprovals(page, [it]);
  const block = page.locator('[data-testid="api-review-keep"]');
  await expect(block).toContainText('Will set');
  await expect(block.locator('[data-testid="api-review-keep-next"]')).toContainText(
    '9007199254740993',
  );
  await expect(
    block.locator('[data-testid="api-review-keep-token"] [data-testid="kept-value-secret"]'),
  ).toBeVisible();
  await expect(
    block.locator('[data-testid="api-review-keep-old"] [data-testid="kept-deleted"]'),
  ).toBeVisible();
});
