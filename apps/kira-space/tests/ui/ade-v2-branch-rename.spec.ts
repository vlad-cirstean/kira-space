import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { FIXED_NOW, openPlan } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// An agent's branch rename request with the optional remote rename, per item.
// Contract ade-branch-rename; backend half: flows/adeflow TestBranchRename (branchrename_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface Row {
  id: string;
  kind: string;
  branch: string;
  from: string;
  upstream: string;
}

function item(upstream?: string): { it: ApprovalItem; row: Row } {
  const [it] = contract<ApprovalItem[]>('ade-branch-rename', 'ApprovalsService.List#branch');
  if (!it) throw new Error('contract has no item');
  const pl = it.payload as { items: Row[] };
  const row = pl.items[0];
  if (!row) throw new Error('contract lacks the rename item');
  const live = liveItem(it, 1800, FIXED_NOW);
  if (upstream === undefined) return { it: live, row };
  return {
    it: { ...live, payload: { ...pl, items: [{ ...row, upstream }] } },
    row: { ...row, upstream },
  };
}

async function open(relaunch: Parameters<typeof openPlan>[0], it: ApprovalItem) {
  const { window: page, control } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    { channel: IPC.approvalsAnswer, response: answeredOk([it.id]) },
  ]);
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
  return { page, control };
}

test('contract: the item names both branches and Approve carries the remote choice', async ({
  relaunch,
}) => {
  const { it, row } = item();
  expect(row.kind).toBe('rename');
  const { page, control } = await open(relaunch, it);
  const dialog = page.locator(t('approvals-dialog'));
  const line = dialog.locator(t(`ade-branch-request-item-${row.id}`));
  await expect(line).toContainText(row.from);
  await expect(line).toContainText(row.branch);
  const remote = dialog.locator(t(`ade-branch-request-remote-${row.id}`));
  await expect(remote).toBeVisible();
  await expect(remote).not.toBeChecked();
  await remote.click();
  await dialog.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toEqual({
    ids: [it.id],
    approve: true,
    options: { [it.id]: { itemIds: [row.id], remoteItemIds: [row.id], shownItemIds: [row.id] } },
  });
});

test('without a remote branch the remote choice is hidden and the rename stays local', async ({
  relaunch,
}) => {
  const { it, row } = item('');
  const { page, control } = await open(relaunch, it);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog.locator(t(`ade-branch-request-remote-${row.id}`))).toHaveCount(0);
  await dialog.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toMatchObject({
    options: { [it.id]: { remoteItemIds: [] } },
  });
});

test('Deny answers the item with a denial', async ({ relaunch }) => {
  const { it } = item();
  const { page, control } = await open(relaunch, it);
  await page.locator(t(`approval-deny-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toMatchObject({ approve: false });
});
