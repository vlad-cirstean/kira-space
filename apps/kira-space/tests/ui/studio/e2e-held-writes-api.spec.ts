import type { ApprovalItem } from '@shared/domain/approvals';
import type { KeptList } from '@shared/domain/kept';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openKeptPane } from './support/settings';

// Contract e2e-held-writes-api. Backend half: dbmcpflow TestE2EHeldWritesAPI. A held response whose
// script keeps a value stores it only on Send: before Send the pane is empty and the review lists it
// under "Will keep"; Withhold stores nothing; after Send the pane lists it.

const SC = 'e2e-held-writes-api';
const item = () => contract<ApprovalItem[]>(SC, 'ApprovalsService.List#api')[0] as ApprovalItem;
const held = () => contract<{ keep: unknown[] }>(SC, 'DbMcpService.PendingResponse#api');
const list = (key: string): KeptList => {
  const l = contract<KeptList>(SC, `KeptVarsService.List#${key}`);
  return {
    ...l,
    entries: l.entries.map((e) => ({
      ...e,
      createdAt: new Date(Date.now() - 60_000).toISOString(),
      expiresAt: new Date(Date.now() + 59 * 60_000).toISOString(),
    })),
  };
};

test('contract: the review lists the kept value; Send answers by id, Withhold answers deny', async ({
  relaunch,
}) => {
  const it = item();
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: held() },
      { channel: IPC.approvalsAnswer, response: answeredOk([it.id]) },
    ],
  });
  await pushApprovals(page, [liveItem(it)]);
  const row = page.locator('[data-testid="api-review-keep-w"]');
  await expect(row).toContainText('List of 2');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toHaveText(/^\[\s*1,\s*2\s*\]$/);
  await page.click(`[data-testid="approval-deny-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: false, options: null });
});

test('contract: Send answers approve', async ({ relaunch }) => {
  const it = item();
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: held() },
      { channel: IPC.approvalsAnswer, response: answeredOk([it.id]) },
    ],
  });
  await pushApprovals(page, [liveItem(it)]);
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});

test('contract: the pane is empty before Send and lists the value after', async ({ relaunch }) => {
  const before = await relaunch({
    control: [{ channel: IPC.keptList, response: list('beforeSend') }],
  });
  await openKeptPane(before.window);
  await expect(before.window.locator('[data-testid="settings-kept-entry"]')).toHaveCount(0);
  const after = await relaunch({
    control: [{ channel: IPC.keptList, response: list('afterSend') }],
  });
  await openKeptPane(after.window);
  const row = after.window.locator('[data-testid="settings-kept-entry"]');
  await expect(row).toHaveCount(1);
  await expect(row.locator('[data-testid="kept-value-preview"]')).toHaveText(/^\[\s*1,\s*2\s*\]$/);
});
