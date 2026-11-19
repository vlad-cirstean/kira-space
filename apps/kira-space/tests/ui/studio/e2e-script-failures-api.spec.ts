import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { unmaskedOps } from '../support/dbOps';
import { IPC } from '../support/ipcChannels';

// Contract e2e-script-failures-api. Backend half: dbmcpflow TestE2EScriptFailuresAPI. Every limit a
// script can hit (time, memory, stack, output, a throw) logs an error row whose detail holds the text
// the agent got, and a failed script held for review shows the failure beside the raw response.

const SC = 'e2e-script-failures-api';

test('contract: each failed script row shows the text the agent got', async ({ relaunch }) => {
  const ops = unmaskedOps(contract<Record<string, unknown>[]>(SC, 'OpsService.Recent#failures'));
  const { window: page } = await relaunch({ control: [{ channel: IPC.opsRecent, response: ops }] });
  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(ops.length);
  for (let i = 0; i < ops.length; i++) await rows.nth(i).click();
  const errors = page.locator('[data-testid="op-detail-error"]');
  await expect(errors).toHaveCount(ops.length);
  const shown = new Set((await errors.allInnerTexts()).map((t) => t.trim()));
  const sent = new Set(ops.map((o) => String(o.error)));
  expect(shown).toEqual(sent);
});

test('contract: a failed script held for review shows its failure and the raw response', async ({
  relaunch,
}) => {
  const it = contract<ApprovalItem[]>(SC, 'ApprovalsService.List#failed')[0] as ApprovalItem;
  const h = contract<{ text: string }>(SC, 'DbMcpService.PendingResponse#failed');
  const { window: page } = await relaunch({
    control: [{ channel: IPC.dbMcpPendingResponse, response: h }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator('[data-testid="api-review-script-failed"]')).toBeVisible();
  await expect(page.locator('[data-testid="api-review-result"]')).toContainText(h.text);
});
