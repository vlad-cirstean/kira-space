import { expect, test } from './fixtures';
import { FILES_TASK, openFilesWindow, openPlan, openTask } from './support/adeV2';
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

test('a task without a worktree has the icon disabled', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskOpenFilesWindow, response: true },
  ]);
  await openTask(page, 'T_alerts');
  const icon = page.locator(t('ade-panel-files'));
  await expect(icon).toBeVisible();
  await expect(icon).toBeDisabled();
  expect(calls(control, IPC.adeTaskOpenFilesWindow)).toHaveLength(0);
});

test('contract: a window booted with the target shows its task', async ({ relaunch }) => {
  const target = contract<{ taskId: string }>('ade-task-files', 'window');
  expect(Object.keys(target)).toEqual(['taskId']);
  const { window: page } = await openFilesWindow(relaunch);
  await expect(page.locator(t('ade-files-window-title'))).toHaveText(/\S/);
  await expect(page.locator(t('ade-files-body'))).toBeVisible();
  await expect(page.locator(t('ade-plan'))).toHaveCount(0);
});
