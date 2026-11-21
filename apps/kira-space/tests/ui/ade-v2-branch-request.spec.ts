import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { adeBoard, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// An agent's branch request as an approvals item, and the Task tab row that reopens it. Creating a
// branch needs no approval (P702), so the generic dialog cases run on an adopt item and the Task tab
// row on a rename item. Contracts ade-branch-adopt and ade-branch-rename; backend halves:
// flows/adeflow TestBranchAdopt and TestBranchRename. Contract ade-branch-request keeps the added row.

const t = (id: string) => `[data-testid="${id}"]`;

interface Pl {
  items: { id: string; branch: string; base?: string }[];
}

function item(scenario = 'ade-branch-adopt', key = 'adopt'): ApprovalItem {
  const [it] = contract<ApprovalItem[]>(scenario, `ApprovalsService.List#${key}`);
  if (!it) throw new Error('contract has no item');
  return liveItem(it, 1800, FIXED_NOW);
}

const payload = () => item().payload as Pl;

async function openRequest(relaunch: Parameters<typeof openPlan>[0]) {
  const it = item();
  const { window: page, control } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    { channel: IPC.approvalsAnswer, response: answeredOk([it.id]) },
  ]);
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
  return { page, control, it };
}

test('contract: the dialog shows the request origin and its branch', async ({ relaunch }) => {
  const { page } = await openRequest(relaunch);
  const dialog = page.locator(t('approvals-dialog'));
  await dialog.locator(t('approval-details-toggle')).click();
  await expect(dialog.locator(t('approval-origin'))).toContainText('Session');
  const [row] = payload().items;
  if (!row) throw new Error('item has no row');
  await expect(dialog.locator(t(`ade-branch-request-item-${row.id}`))).toContainText(row.branch);
});

test('an answered item that stays listed (late call) can be approved again', async ({
  relaunch,
}) => {
  const { page, control, it } = await openRequest(relaunch);
  const dialog = page.locator(t('approvals-dialog'));
  const approve = dialog.locator(t(`approval-approve-${it.id}`));
  await approve.click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  await expect(approve).toBeEnabled();
  await approve.click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(2);
});

test('a request has no countdown and Approve still answers it late', async ({ relaunch }) => {
  // The call wait ending cancels nothing: the item stays open with no expiry. Backend half: flows/adeflow
  // TestBranchRequestSurvivesClockAdvance.
  const open = { ...item(), expiresAt: 0 };
  const { window: page, control } = await openPlan(relaunch, [
    ...seedApprovals([open]),
    { channel: IPC.approvalsAnswer, response: answeredOk([open.id]) },
  ]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog).toBeVisible();
  await expect(dialog.locator(t('approval-countdown'))).toHaveCount(0);
  await dialog.locator(t(`approval-approve-${open.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
});

test('Deny answers the item with a denial', async ({ relaunch }) => {
  const { page, control, it } = await openRequest(relaunch);
  await page.locator(t(`approval-deny-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toMatchObject({
    ids: [it.id],
    approve: false,
  });
});

test('approving with nothing checked shows the refusal inline', async ({ relaunch }) => {
  const it = item();
  const { window: page } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    {
      channel: IPC.approvalsAnswer,
      response: [{ id: it.id, code: 'E_CONTRACT', error: 'approve at least one branch' }],
    },
  ]);
  const dialog = page.locator(t('approvals-dialog'));
  for (const row of payload().items) {
    await dialog
      .locator(t(`ade-branch-request-item-${row.id}`))
      .getByRole('checkbox')
      .click();
  }
  await dialog.locator(t(`approval-approve-${it.id}`)).click();
  await expect(dialog.locator(t('approval-error'))).toHaveText('approve at least one branch');
});

test('the Task tab row reopens the item after it was minimised', async ({ relaunch }) => {
  const it = item('ade-branch-rename', 'branch');
  const { window: page } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        const task = b.tasks.find((x) => x.id === 'T_alerts');
        if (!task) throw new Error('T_alerts missing');
        task.branchRequestId = it.id.slice('branch.request:'.length);
      }),
    },
  ]);
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
  await page.keyboard.press('Shift+Escape');
  await expect(page.locator(t('approvals-dialog'))).toHaveCount(0);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-task-branch-request-open')).click();
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
});

test('a repo an agent added shows in the task Branches', async ({ relaunch }) => {
  const added = contract<{ id: string; setup: Record<string, unknown> | null }>(
    'ade-branch-request',
    'AdeTaskBoard.Board.addedBranch',
  );
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        const task = b.tasks.find((x) => x.id === 'T_alerts');
        if (!task) throw new Error('T_alerts missing');
        (task.branchIds as string[]).push('b_agent_mobile');
        b.branches.push({
          ...added,
          id: 'b_agent_mobile',
          taskId: 'T_alerts',
          codeRepoId: 'repo-mobile',
          name: 'feat/agent-named',
          base: 'main',
        });
      }),
    },
  ]);
  await openTask(page, 'T_alerts');
  await expect(
    page.locator(`${t('ade-task-branch')}[data-branch-id="b_agent_mobile"]`),
  ).toContainText('feat/agent-named');
});
