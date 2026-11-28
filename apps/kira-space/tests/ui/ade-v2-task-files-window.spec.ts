import { expect, taskFilesWindowTest, test } from './fixtures';
import { adeFixture, FILES_TASK, openFilesWindow, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Opening a task's Files window from the task panel header icon, and what the window shows at
// boot. Backend half: flows/adeflow TestTaskFilesWindow; both read the ade-task-files contract.

const t = (id: string) => `[data-testid="${id}"]`;

test('contract: the header icon opens the task Files window once', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskOpenFilesWindow, response: true },
  ]);
  await openTask(page, FILES_TASK);
  const icon = page.locator(t('ade-panel-files'));
  await expect(icon.locator('.codicon-files')).toBeVisible();
  await expect(icon).toBeEnabled();
  await icon.click();
  await expect.poll(() => calls(control, IPC.adeTaskOpenFilesWindow)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskOpenFilesWindow)[0]?.args;
  expect(sent).toEqual({ taskId: FILES_TASK });
  const want = contract<Record<string, string>>(
    'ade-task-files',
    'args:AdeTaskService.OpenFilesWindow',
  );
  expect(Object.keys(sent as object)).toEqual(Object.keys(want));
});

test('a refused open shows the action error in the panel', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskOpenFilesWindow, error: { code: 'failed', message: 'no window' } },
  ]);
  await openTask(page, FILES_TASK);
  await page.locator(t('ade-panel-files')).click();
  await expect(page.locator(t('ade-panel-action-error'))).toContainText('no window');
});

test('a branch without a worktree keeps the icon enabled; a task with no repo has none', async ({
  relaunch,
}) => {
  const board = adeFixture<{
    branches: { id: string; worktree: string }[];
    tasks: { id: string; branchIds: string[] }[];
  }>('board');
  for (const br of board.branches) if (br.id === 'b_cart') br.worktree = '';
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
    { channel: IPC.adeTaskOpenFilesWindow, response: true },
  ]);
  await openTask(page, FILES_TASK);
  const icon = page.locator(t('ade-panel-files'));
  await expect(icon).toBeVisible();
  await expect(icon).toBeEnabled();
  await icon.click();
  await expect.poll(() => calls(control, IPC.adeTaskOpenFilesWindow)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskOpenFilesWindow)[0]?.args).toEqual({ taskId: FILES_TASK });
  // Repo rows without a branch still open the repo's main worktree.
  await openTask(page, 'T_alerts');
  await expect(page.locator(t('ade-panel-files'))).toBeEnabled();
});

test('a task with no repo row shows no Files icon', async ({ relaunch }) => {
  const board = adeFixture<{ tasks: { id: string; branchIds: string[] }[] }>('board');
  for (const task of board.tasks) if (task.id === 'T_alerts') task.branchIds = [];
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await openTask(page, 'T_alerts');
  await expect(page.locator(t('ade-panel-files'))).toHaveCount(0);
});

taskFilesWindowTest(
  'contract: a window booted with the target shows its task',
  async ({ relaunch }) => {
    const target = contract<{ taskId: string }>('ade-task-files', 'window');
    expect(Object.keys(target)).toEqual(['taskId']);
    const { window: page } = await openFilesWindow(relaunch);
    await expect(page.locator(t('ade-files-window-title'))).toHaveText(/\S/);
    await expect(page.locator(t('ade-files-body'))).toBeVisible();
    await expect(page.locator(t('ade-plan'))).toHaveCount(0);
    // The task colour is a 3px line directly under the title bar.
    const [bar, line] = await Promise.all([
      page.locator('.wails-drag').boundingBox(),
      page.locator(t('ade-files-window-colour')).boundingBox(),
    ]);
    expect(line?.height).toBe(3);
    expect(
      Math.abs((line?.y ?? 0) + 3 - ((bar?.y ?? 0) + (bar?.height ?? 99))),
    ).toBeLessThanOrEqual(1);
  },
);
