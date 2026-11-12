import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { adeBoard, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// An agent's change_branch request, and how a row shows what its worktree folder holds.
// Contract ade-branch-change; backend half: flows/adeflow TestBranchChange (branchchange_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface Pl {
  items: { id: string; kind: string; branch: string; from: string }[];
}

function item(): ApprovalItem {
  const [it] = contract<ApprovalItem[]>('ade-branch-change', 'ApprovalsService.List#branch');
  if (!it) throw new Error('contract has no item');
  return liveItem(it, 1800, FIXED_NOW);
}

test('contract: the item shows the target branch and Approve sends it', async ({ relaunch }) => {
  const it = item();
  const row = (it.payload as Pl).items[0];
  if (!row) throw new Error('contract lacks the switch item');
  expect(row.kind).toBe('switch');
  const { window: page, control } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    { channel: IPC.approvalsAnswer, response: answeredOk([it.id]) },
  ]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog).toBeVisible();
  const line = dialog.locator(t(`ade-branch-request-item-${row.id}`));
  await expect(line).toContainText(row.branch);
  await expect(line).toContainText(`Change from ${row.from}`);
  await expect(dialog.locator(t(`ade-branch-request-remote-${row.id}`))).toHaveCount(0);
  await dialog.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toEqual({
    ids: [it.id],
    approve: true,
    options: {
      [it.id]: { itemIds: [row.id], remoteItemIds: [], shownItemIds: [row.id] },
    },
  });
});

const TASK = 'T_cart';
const ROW = 'b_cart';
const TREE = '~/kira-space-wts/web-app/cart-wt';

interface Fx extends Record<string, unknown> {
  name: string;
  detached?: { head: string; holder: Record<string, unknown> };
}

// The contract's masked ids become the fixture board's.
function boardWith(key: 'mismatch' | 'changed') {
  const [fx] = contract<Fx[]>('ade-branch-change', `AdeTaskService.Board#${key}`);
  if (!fx) throw new Error('contract has no row');
  return adeBoard((b) => {
    const at = b.branches.findIndex((x) => x.id === ROW);
    const cur = b.branches[at];
    if (!cur) throw new Error(`${ROW} missing`);
    b.branches[at] = {
      ...cur,
      name: fx.name,
      worktree: fx.detached ? '' : TREE,
      detached: fx.detached
        ? {
            head: fx.detached.head,
            worktree: TREE,
            holder: { kind: 'none', taskId: '', branchId: '', worktree: '' },
          }
        : undefined,
    };
  });
}

const rowOf = (page: Page, testid: string) => page.locator(`${t(testid)}[data-branch-id="${ROW}"]`);

test('contract: a row whose worktree holds another branch shows it everywhere', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: boardWith('mismatch') },
  ]);
  const chip = rowOf(page, 'ade-branch-row').locator(t('ade-branch-checked-out'));
  await expect(chip).toContainText('feat/other');
  await openTask(page, TASK);
  await expect(rowOf(page, 'ade-task-branch').locator(t('ade-branch-checked-out'))).toContainText(
    'feat/other',
  );
  await rowOf(page, 'ade-task-branch').click();
  const line = page.locator(t('ade-panel-checkout'));
  await expect(line).toHaveAttribute('data-mismatch', 'true');
  await expect(line).toContainText('feat/old');
  await expect(line).toContainText('feat/other');
});

test('contract: after the change the chip is gone and the panel reads checked out', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: boardWith('changed') },
  ]);
  await expect(rowOf(page, 'ade-branch-row').locator(t('ade-branch-name'))).toHaveText(
    'feat/other',
  );
  await expect(page.locator(t('ade-branch-checked-out'))).toHaveCount(0);
  await openTask(page, TASK);
  await rowOf(page, 'ade-task-branch').click();
  const line = page.locator(t('ade-panel-checkout'));
  await expect(line).not.toHaveAttribute('data-mismatch', 'true');
  await expect(line).toHaveText('Checked out: feat/other');
});
