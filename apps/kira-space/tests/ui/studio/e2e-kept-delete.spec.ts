import type { ApprovalItem } from '@shared/domain/approvals';
import type { DbMcpPendingResponse } from '@shared/domain/dbmcp';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { withTimes } from './support/keptTimes';

// Contract e2e-kept-delete. Backend half: dbmcpflow TestE2EKeptDelete, the same scenario. The x on a kept row
// deletes the variable the card waits on; the x on a Will keep row drops that value from a held result.

const SC = 'e2e-kept-delete';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;

test('contract: the x on a kept row deletes that variable', async ({ relaunch }) => {
  const it = withTimes(item('waiting'));
  const clear = contract<{ ids: string[] }>(SC, 'args:KeptVarsService.Clear#row');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.keptClear, response: 1 }],
  });
  await pushApprovals(page, [it]);
  const row = page.locator(
    `[data-testid="approval-item-${it.id}"] [data-testid="api-approval-kept-token"]`,
  );
  await expect(row.locator('[data-testid="kept-used-in"]')).toBeVisible();
  await row.locator('[data-testid="api-approval-kept-token-clear"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.keptClear)?.args)
    .toEqual(clear);
});

test('contract: the x on a Will keep row drops that value and the list refetches', async ({
  relaunch,
}) => {
  const it = liveItem(item('review'));
  const both = contract<DbMcpPendingResponse>(SC, 'DbMcpService.PendingResponse#both');
  const dropped = contract<DbMcpPendingResponse>(SC, 'DbMcpService.PendingResponse#dropped');
  const args = contract<{ requestId: string; name: string }>(
    SC,
    'args:KeptVarsService.DropStaged#review',
  );
  const requestId = it.id.slice(it.kind.length + 1);
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: both },
      { channel: IPC.keptDropStaged },
    ],
  });
  await pushApprovals(page, [it]);
  const block = page.locator('[data-testid="api-review-keep"]');
  await expect(block.locator('[data-testid="api-review-keep-token"]')).toBeVisible();
  await expect(block.locator('[data-testid="api-review-keep-account_id"]')).toBeVisible();
  control.respond(IPC.dbMcpPendingResponse, dropped);
  await block.locator('[data-testid="api-review-keep-account_id-drop"]').click();
  await expect(block.locator('[data-testid="api-review-keep-account_id"]')).toHaveCount(0);
  await expect(block.locator('[data-testid="api-review-keep-token"]')).toBeVisible();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.keptDropStaged)?.args)
    .toEqual({ ...args, requestId });
});
