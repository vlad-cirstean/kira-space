import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { FIXED_NOW, openPlan } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Contract approvals-space. Backend half: flows/adeflow TestBranchRequestMerge. Concurrent branch calls
// of one task arrive as one approvals item with a group per call; the person picks per item.

const SCENARIO = 'approvals-space';
const t = (id: string) => `[data-testid="${id}"]`;

function merged(): ApprovalItem {
  const [it] = contract<ApprovalItem[]>(SCENARIO, 'ApprovalsService.List');
  if (!it) throw new Error('contract has no item');
  return liveItem(it, 1800, FIXED_NOW);
}

test('contract: three calls show as three groups and Approve sends the picked items', async ({
  relaunch,
}) => {
  const it = merged();
  const answer = contract<{ ids: string[]; options: Record<string, unknown> }>(
    SCENARIO,
    'args:ApprovalsService.Answer',
  );
  const { window: page, control } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    { channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) },
  ]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog.locator(t('approvals-count'))).toHaveText('1');
  await expect(dialog.locator('[data-testid^="ade-branch-request-call-"]')).toHaveCount(3);

  // Drop feat/c (call 1) and also rename the remote of the rename in call 2.
  await dialog.locator(t('ade-branch-request-item-c1i0')).getByRole('checkbox').click();
  await dialog.locator(t('ade-branch-request-remote-c2i0')).click();
  await dialog.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toEqual({ ...answer });
});

test('a push that joins a call keeps the person’s choices on the item', async ({ relaunch }) => {
  const it = merged();
  const { window: page } = await openPlan(relaunch, seedApprovals([it]));
  const dialog = page.locator(t('approvals-dialog'));
  const box = dialog.locator(t('ade-branch-request-item-c1i0')).getByRole('checkbox');
  await box.click();
  await expect(box).not.toBeChecked();
  await pushApprovals(page, [{ ...it, expiresAt: it.expiresAt + 60_000 }]);
  await expect(box).not.toBeChecked();
});
