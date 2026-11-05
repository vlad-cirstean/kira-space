import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// An agent's branch rename request in the routed dialog, with the optional remote rename.
// Contract ade-branch-rename; backend half: flows/adeflow TestBranchRename (branchrename_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface RequestFx {
  id: string;
  taskId: string;
  expiresAt: number;
  items: {
    id: string;
    repo: string;
    kind: string;
    branch: string;
    from: string;
    upstream: string;
  }[];
}

function request(): RequestFx {
  const r = contract<RequestFx>('ade-branch-rename', 'AdeTaskService.BranchRequest');
  return {
    ...r,
    id: 'br_1',
    taskId: 'T_alerts',
    expiresAt: Date.now() + 30 * 60_000,
    items: r.items.map((item, n) => ({ ...item, id: `i${n}` })),
  };
}

async function openRouted(relaunch: Parameters<typeof openPlan>[0], req: RequestFx) {
  const entry = routed('branch-request', 'br_1');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBranchRequest, response: req },
    { channel: IPC.adeTaskApproveBranchRequest, response: { resolved: true } },
    { channel: IPC.adeTaskDenyBranchRequest, response: { resolved: true } },
    { channel: IPC.promptsList, response: [entry] },
  ]);
  await emitPrompts(page, [entry]);
  await expect(page.locator(t('ade-branch-request-dialog'))).toBeVisible();
  return { page, control };
}

test('contract: the dialog names both branches and Approve carries the remote choice', async ({
  relaunch,
}) => {
  const req = request();
  const item = req.items[0];
  if (!item) throw new Error('contract lacks the rename item');
  expect(item.kind).toBe('rename');
  const { page, control } = await openRouted(relaunch, req);
  const dialog = page.locator(t('ade-branch-request-dialog'));
  const row = dialog.locator(t(`ade-branch-request-item-${item.id}`));
  await expect(row).toContainText(item.from);
  await expect(row).toContainText(item.branch);
  const remote = dialog.locator(t('ade-branch-request-rename-remote'));
  await expect(remote).toBeVisible();
  await expect(remote).not.toBeChecked();
  await remote.click();
  await dialog.locator(t('ade-branch-request-approve')).click();
  await expect.poll(() => calls(control, IPC.adeTaskApproveBranchRequest)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskApproveBranchRequest)[0]?.args).toEqual({
    id: 'br_1',
    itemIds: [item.id],
    renameRemote: true,
  });
});

test('without a remote branch the remote choice is hidden and the rename stays local', async ({
  relaunch,
}) => {
  const req = request();
  const item = req.items[0];
  if (!item) throw new Error('contract lacks the rename item');
  const { page, control } = await openRouted(relaunch, {
    ...req,
    items: [{ ...item, upstream: '' }],
  });
  const dialog = page.locator(t('ade-branch-request-dialog'));
  await expect(dialog.locator(t('ade-branch-request-rename-remote'))).toHaveCount(0);
  await dialog.locator(t('ade-branch-request-approve')).click();
  await expect.poll(() => calls(control, IPC.adeTaskApproveBranchRequest)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskApproveBranchRequest)[0]?.args).toMatchObject({
    renameRemote: false,
  });
});

test('Deny sends DenyBranchRequest', async ({ relaunch }) => {
  const { page, control } = await openRouted(relaunch, request());
  await page.locator(t('ade-branch-request-deny')).click();
  await expect.poll(() => calls(control, IPC.adeTaskDenyBranchRequest)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskApproveBranchRequest)).toHaveLength(0);
});
