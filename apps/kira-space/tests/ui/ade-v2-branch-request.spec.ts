import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { adeBoard, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// An agent's branch request as an approvals item, and the Task tab row that reopens it.
// Contract ade-branch-request; backend half: flows/adeflow TestBranchRequest (branchrequest_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface Pl {
  items: { id: string; branch: string }[];
}

function item(): ApprovalItem {
  const [it] = contract<ApprovalItem[]>('ade-branch-request', 'ApprovalsService.List#branch');
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

test('contract: the item lists every branch and Approve sends the checked items', async ({
  relaunch,
}) => {
  const { page, control, it } = await openRequest(relaunch);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog.locator(t('ade-branch-request-source')).first()).toContainText(
    (it.origin.step || 'run').toString(),
  );
  const { items } = payload();
  expect(items).toHaveLength(2);
  for (const row of items) {
    await expect(dialog.locator(t(`ade-branch-request-item-${row.id}`))).toContainText(row.branch);
  }
  const [kept, dropped] = items;
  await dialog
    .locator(t(`ade-branch-request-item-${dropped?.id}`))
    .getByRole('checkbox')
    .click();
  await dialog.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toEqual({
    ids: [it.id],
    approve: true,
    options: {
      [it.id]: {
        itemIds: [kept?.id],
        remoteItemIds: [],
        shownItemIds: items.map((row) => row.id),
      },
    },
  });
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
  const it = item();
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
  await page.keyboard.press('Escape');
  await expect(page.locator(t('approvals-dialog'))).toHaveCount(0);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-task-branch-request-open')).click();
  await expect(page.locator(t('approvals-dialog'))).toBeVisible();
});

test('a repo added by an approved request shows in the task Branches', async ({ relaunch }) => {
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
