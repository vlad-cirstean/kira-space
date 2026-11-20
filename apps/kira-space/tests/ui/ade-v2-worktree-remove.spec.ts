import { expect, test } from './fixtures';
import { adeBoard, adeFixture, openPlan, openTask } from './support/adeV2';
import { pick } from './support/branchMenu';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Removing a worktree needs no confirm dialog (P702): a clean row goes at once, a row with changes
// shows the labelled discard. Contract ade-worktree-remove; backend half: flows/adeflow
// TestUserWorktreeRemoveNoConfirm (adeflow/worktreeremove_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

const { dirty, dirtyToken } = contract<{
  dirty: { code: string; path: string }[];
  dirtyToken: string;
}>('ade-worktree-remove', 'AdeTaskService.Board#dirty');

const dirtyBoard = {
  channel: IPC.adeTaskBoard,
  response: adeBoard((b) => {
    const row = b.branches.find((x) => x.id === 'b_cart');
    if (!row) throw new Error('b_cart missing');
    Object.assign(row, { dirty, dirtyToken });
  }),
};

const candidates = { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') };

test('contract: a clean row removes its branch with no dialog and no discard', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_cart');
  await pick(page, 'b_cart', 'ade-task-branch-remove');
  await expect.poll(() => calls(control, IPC.adeTaskRemoveTaskBranch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRemoveTaskBranch)[0]?.args).toEqual({
    branchId: 'b_cart',
    discardToken: '',
  });
  await expect(page.locator(t('confirm-dialog'))).toHaveCount(0);
});

test('contract: a dirty row offers Discard N changes and remove branch, and sends the discard', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [dirtyBoard]);
  await openTask(page, 'T_cart');
  await page
    .locator(`${t('ade-task-branch-item')}[data-branch-id="b_cart"] ${t('ade-task-branch-menu')}`)
    .click();
  await expect(page.locator(t('ade-task-branch-remove'))).toHaveCount(0);
  await expect(page.locator(t('ade-task-branch-remove-discard'))).toHaveText(
    `Discard ${dirty.length} changes and remove branch`,
  );
  await page.locator(t('ade-task-branch-remove-discard')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRemoveTaskBranch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRemoveTaskBranch)[0]?.args).toEqual({
    branchId: 'b_cart',
    discardToken: dirtyToken,
  });
  await expect(page.locator(t('confirm-dialog'))).toHaveCount(0);
});

test('a dirty row offers Discard N changes and remove repository, and sends the discard', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [dirtyBoard]);
  await openTask(page, 'T_cart');
  await pick(page, 'b_cart', 'ade-task-repo-remove-discard');
  await expect.poll(() => calls(control, IPC.adeTaskRemoveTaskRepo)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRemoveTaskRepo)[0]?.args).toEqual({
    branchId: 'b_cart',
    discardToken: dirtyToken,
  });
  await expect(page.locator(t('confirm-dialog'))).toHaveCount(0);
});

test('changing the branch of a dirty row holds the choice until Discard changes and change', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [dirtyBoard, candidates]);
  await openTask(page, 'T_cart');
  await pick(page, 'b_cart', 'ade-task-branch-change');
  await page.locator('[data-testid^="ade-branch-form-option-"]').first().click();
  await expect(page.locator(t('ade-branch-form-dirty-error'))).toContainText(
    `${dirty.length} uncommitted changes`,
  );
  expect(calls(control, IPC.adeTaskSetTaskBranch)).toHaveLength(0);
  await expect(page.locator(t('ade-branch-form-discard'))).toHaveText(
    `Discard ${dirty.length} changes and change`,
  );
  await page.locator(t('ade-branch-form-discard')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSetTaskBranch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetTaskBranch)[0]?.args).toMatchObject({
    branchId: 'b_cart',
    existing: true,
    discardToken: dirtyToken,
  });
});
