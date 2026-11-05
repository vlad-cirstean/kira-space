import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from './fixtures';
import { adeBoard, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// An agent's branch request: the routed prompt, the dialog and the Task tab row.
// Contract ade-branch-request; backend half: flows/adeflow TestBranchRequest (branchrequest_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface RequestFx {
  id: string;
  taskId: string;
  taskTitle: string;
  source: string;
  stepName: string;
  items: {
    id: string;
    codeRepoId: string;
    repo: string;
    kind: string;
    branch: string;
    base: string;
  }[];
  createdAt: number;
  expiresAt: number;
}

/** The recorded request, with stable item and repo ids and an expiry far enough ahead to stay open. */
function request(): RequestFx {
  const r = contract<RequestFx>('ade-branch-request', 'AdeTaskService.BranchRequest');
  return {
    ...r,
    id: 'br_1',
    taskId: 'T_alerts',
    expiresAt: Date.now() + 30 * 60_000,
    items: r.items.map((item, n) => ({ ...item, id: `i${n}`, codeRepoId: `R_${item.repo}` })),
  };
}

function snapshots(extra: ControlSnapshot[] = []): ControlSnapshot[] {
  return [
    { channel: IPC.adeTaskBranchRequest, response: request() },
    { channel: IPC.adeTaskApproveBranchRequest, response: { resolved: true } },
    { channel: IPC.adeTaskDenyBranchRequest, response: { resolved: true } },
    ...extra,
  ];
}

async function openRouted(relaunch: Parameters<typeof openPlan>[0], extra?: ControlSnapshot[]) {
  const entry = routed('branch-request', 'br_1');
  const { window: page, control } = await openPlan(relaunch, [
    ...snapshots(extra),
    { channel: IPC.promptsList, response: [entry] },
  ]);
  await emitPrompts(page, [entry]);
  await expect(page.locator(t('ade-branch-request-dialog'))).toBeVisible();
  return { page, control };
}

test('contract: the routed prompt lists every branch and Approve sends the checked items', async ({
  relaunch,
}) => {
  const req = request();
  const { page, control } = await openRouted(relaunch);
  const dialog = page.locator(t('ade-branch-request-dialog'));
  await expect(dialog.locator(t('ade-branch-request-source'))).toContainText(req.stepName);
  for (const item of req.items) {
    await expect(dialog.locator(t(`ade-branch-request-item-${item.id}`))).toBeVisible();
    await expect(dialog.locator(t(`ade-branch-request-item-${item.id}`))).toContainText(
      item.branch,
    );
  }
  expect(req.items).toHaveLength(2);
  const [kept, dropped] = req.items;
  await dialog
    .locator(t(`ade-branch-request-item-${dropped?.id}`))
    .getByRole('checkbox')
    .click();
  await expect(dialog.locator(t('ade-branch-request-approve'))).toContainText('(1)');
  await dialog.locator(t('ade-branch-request-approve')).click();
  await expect.poll(() => calls(control, IPC.adeTaskApproveBranchRequest)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskApproveBranchRequest)[0]?.args).toEqual({
    id: 'br_1',
    itemIds: [kept?.id],
    renameRemote: false,
  });
  await expect(dialog).toBeHidden();
});

test('Deny sends DenyBranchRequest', async ({ relaunch }) => {
  const { page, control } = await openRouted(relaunch);
  await page.locator(t('ade-branch-request-deny')).click();
  await expect.poll(() => calls(control, IPC.adeTaskDenyBranchRequest)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskDenyBranchRequest)[0]?.args).toEqual({
    id: 'br_1',
  });
  expect(calls(control, IPC.adeTaskApproveBranchRequest)).toHaveLength(0);
});

test('Approve is disabled with nothing checked', async ({ relaunch }) => {
  const { page } = await openRouted(relaunch);
  const dialog = page.locator(t('ade-branch-request-dialog'));
  for (const item of request().items) {
    await dialog
      .locator(t(`ade-branch-request-item-${item.id}`))
      .getByRole('checkbox')
      .click();
  }
  await expect(dialog.locator(t('ade-branch-request-approve'))).toBeDisabled();
});

test('Escape hides the dialog without answering the agent', async ({ relaunch }) => {
  const { page, control } = await openRouted(relaunch);
  await page.keyboard.press('Escape');
  await expect(page.locator(t('ade-branch-request-dialog'))).toBeHidden();
  expect(calls(control, IPC.adeTaskApproveBranchRequest)).toHaveLength(0);
  expect(calls(control, IPC.adeTaskDenyBranchRequest)).toHaveLength(0);
});

test('the Task tab row opens the dialog', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    ...snapshots(),
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        const task = b.tasks.find((x) => x.id === 'T_alerts');
        if (!task) throw new Error('T_alerts missing');
        task.branchRequestId = 'br_1';
      }),
    },
  ]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-task-branch-request-open')).click();
  await expect(page.locator(t('ade-branch-request-dialog'))).toBeVisible();
});

test('a request that is already answered closes the dialog quietly', async ({ relaunch }) => {
  const entry = routed('branch-request', 'br_1');
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBranchRequest,
      error: {
        code: 'E_NOT_FOUND',
        message: 'This branch request was already answered.',
      },
    },
    { channel: IPC.promptsList, response: [entry] },
  ]);
  await emitPrompts(page, [entry]);
  await expect(page.locator(t('ade-branch-request-dialog'))).toBeHidden();
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
