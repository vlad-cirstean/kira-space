import type { ApprovalItem } from '@shared/domain/approvals';
import type { DbMcpPendingResponse } from '@shared/domain/dbmcp';
import type { RevealResult } from '@shared/domain/variables';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../../ui/fixtures';
import { contract } from '../../ui/support/contract';
import { IPC } from '../../ui/support/ipcChannels';
import { settleFonts } from './support/settle';

// P716: kept variable kinds. Contract e2e-kept-secret; behaviour in ui/studio/e2e-kept-secret.spec.ts.

const SC = 'e2e-kept-secret';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;
const mask = (page: import('@playwright/test').Page) => [
  page.locator('[data-testid="approval-countdown"]'),
];

test('login card marks secret and plain keeps', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, [liveItem(item('login'), 600, Date.now())]);
  await expect(page.locator('[data-testid="api-approval-script-transform"]')).toBeVisible();
  await settleFonts(page);
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveScreenshot(
    'approvals-kept-login.png',
    { mask: mask(page) },
  );
});

test('review previews plain and reveals secret', async ({ relaunch }) => {
  const it = liveItem(item('review'), 600, Date.now());
  const held = contract<DbMcpPendingResponse>(SC, 'DbMcpService.PendingResponse#review');
  const requestId = it.id.slice(it.kind.length + 1);
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: held },
      {
        channel: IPC.keptRevealStaged,
        args: { requestId, name: 'session_token', confirmed: false },
        response: contract<RevealResult>(SC, 'KeptVarsService.RevealStaged#confirmation'),
      },
      {
        channel: IPC.keptRevealStaged,
        args: { requestId, name: 'session_token', confirmed: true },
        response: contract<RevealResult>(SC, 'KeptVarsService.RevealStaged#review'),
      },
    ],
  });
  await page.setViewportSize({ width: 1400, height: 900 });
  await pushApprovals(page, [it]);
  const block = page.locator('[data-testid="api-review-keep"]');
  await expect(block).toBeVisible();
  await settleFonts(page);
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveScreenshot(
    'approvals-kept-review.png',
    { mask: mask(page) },
  );
  await block.locator('[data-testid="kept-value-reveal"]').click();
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect(block.locator('[data-testid="kept-value-revealed"]')).toBeVisible();
  await page.mouse.move(0, 0);
  await settleFonts(page);
  await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveScreenshot(
    'approvals-kept-review-revealed.png',
    { mask: mask(page) },
  );
});
