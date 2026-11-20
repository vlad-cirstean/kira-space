import { expect, test } from './fixtures';
import { adeFixture, openPlan, openTask } from './support/adeV2';
import { pick } from './support/branchMenu';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Add, change and remove the repos and branches of a task, row by row. Backend half: adeflow
// TestTaskReposAndBranches.

const t = (id: string) => `[data-testid="${id}"]`;
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;

const candidates = { channel: IPC.adeTaskCandidateBranches, response: adeFixture('candidates') };

test('a repo without a branch gets an existing branch from its menu', async ({ relaunch }) => {
  const all = adeFixture<{ branches: { codeRepoId: string }[] }>('candidates');
  const web = {
    ...all,
    branches: all.branches.filter((b) => b.codeRepoId === 'repo-web-app'),
  };
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCandidateBranches, response: web },
  ]);
  await openTask(page, 'T_alerts');
  await pick(page, 'd_alerts_web', 'ade-task-branch-add');
  await expect(page.locator(t('ade-branch-form'))).toBeVisible();
  const rows = page.locator('[data-testid^="ade-branch-form-option-"]');
  await expect(rows).toHaveCount(2);
  await rows.filter({ hasText: 'fix/invoice-rounding' }).click();
  await expect.poll(() => calls(control, IPC.adeTaskSetTaskBranch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetTaskBranch)[0]?.args).toEqual({
    branchId: 'd_alerts_web',
    name: 'fix/invoice-rounding',
    existing: true,
    discardToken: '',
  });
  await expect(page.locator(t('ade-branch-form'))).toHaveCount(0);
});

test('the form asks for the candidates of the row repo, and Enter picks the first option', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [candidates]);
  await openTask(page, 'T_alerts');
  await pick(page, 'd_alerts_web', 'ade-task-branch-add');
  await expect(page.locator('[data-testid^="ade-branch-form-option-"]').first()).toBeVisible();
  expect(calls(control, IPC.adeTaskCandidateBranches).at(-1)?.args).toEqual({
    codeRepoId: 'repo-web-app',
  });
  await page.locator(t('ade-branch-form-search')).press('ArrowDown');
  await page.locator(t('ade-branch-form-search')).press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskSetTaskBranch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetTaskBranch)[0]?.args).toMatchObject({ existing: true });
});

test('an invalid new branch name shows its error and sends nothing', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_alerts');
  await pick(page, 'd_alerts_api', 'ade-task-branch-add');
  await page.locator(t('ade-branch-form-search')).fill('-bad');
  await expect(page.locator(t('ade-branch-form-error'))).toBeVisible();
  await page.locator(t('ade-branch-form-search')).press('Enter');
  expect(calls(control, IPC.adeTaskSetTaskBranch)).toHaveLength(0);
});

test('a repo without a branch gets a new branch name', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_alerts');
  await pick(page, 'd_alerts_api', 'ade-task-branch-add');
  await page.locator(t('ade-branch-form-search')).fill('feat/alerts-api');
  await page.locator(t('ade-branch-form-custom')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSetTaskBranch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetTaskBranch)[0]?.args).toEqual({
    branchId: 'd_alerts_api',
    name: 'feat/alerts-api',
    existing: false,
    discardToken: '',
  });
});

test('a refused name closes the form and lands in an error toast', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskSetTaskBranch,
      error: { code: 'E_BAD_REQUEST', message: 'branch feat/x already exists' },
    },
  ]);
  await openTask(page, 'T_alerts');
  await pick(page, 'd_alerts_api', 'ade-task-branch-add');
  await page.locator(t('ade-branch-form-search')).fill('feat/x');
  await page.locator(t('ade-branch-form-custom')).click();
  await expect(page.locator(t('ade-branch-form'))).toHaveCount(0);
  await expect(page.locator('[data-sonner-toast]')).toContainText('already exists');
});

test('Change repo swaps the repo of a row without a branch', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_alerts');
  await pick(page, 'd_alerts_api', 'ade-task-repo-change');
  await page.locator(t('ade-branch-form-repo-option-repo-mobile')).click();
  await expect.poll(() => calls(control, IPC.adeTaskUpdateTaskRepo)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskUpdateTaskRepo)[0]?.args).toEqual({
    branchId: 'd_alerts_api',
    codeRepoId: 'repo-mobile',
  });
});

test('Remove repo on a clean row asks nothing', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_alerts');
  await pick(page, 'd_alerts_web', 'ade-task-repo-remove');
  await expect.poll(() => calls(control, IPC.adeTaskRemoveTaskRepo)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskRemoveTaskRepo)[0]?.args).toEqual({
    branchId: 'd_alerts_web',
    discardToken: '',
  });
  await expect(page.locator(t('confirm-dialog'))).toHaveCount(0);
});

test('a review task offers no row menu', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(t('ade-load-all')).click();
  const row = page.locator(`${card('R_sara')} ${t('ade-branch-row')}[data-branch-id="b_sara"]`);
  await row.scrollIntoViewIfNeeded();
  await row.click();
  await expect(page.locator(t('ade-panel-review-note'))).toBeVisible();
  await page.locator(`${t('ade-task-branch')}[data-branch-id="b_sara"]`).click();
  await expect(page.locator(t('ade-panel-back'))).toBeVisible();
  await expect(
    page.locator(`${t('ade-task-branch-item')} ${t('ade-task-branch-menu')}`),
  ).toHaveCount(0);
  await expect(row.locator(t('ade-task-branch-menu'))).toHaveCount(0);
});

test('Add branch on the task still attaches an existing branch to a new row', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [candidates]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-add-branch')).click();
  await expect(page.locator(t('ade-add-popover'))).toBeVisible();
  await page.locator(t('ade-branch-search')).fill('invoice');
  await page.locator('[data-testid^="ade-branch-option-"]').first().click();
  await expect.poll(() => calls(control, IPC.adeTaskAddExistingBranch)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddExistingBranch)[0]?.args).toEqual({
    codeRepoId: 'repo-web-app',
    name: 'fix/invoice-rounding',
    taskId: 'T_alerts',
  });
});
