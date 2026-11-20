import { expect, test } from '../ui/fixtures';
import {
  openPlan,
  openTask,
  openTaskFile,
  openTaskFiles,
  taskFilesControl,
  taskSearchEvent,
} from '../ui/support/adeV2';
import { contract } from '../ui/support/contract';
import { IPC } from '../ui/support/ipcChannels';
import { emitWailsEvent } from '../ui/support/mockRuntime';

// P691: the Files tab of a task panel.

const t = (id: string) => `[data-testid="${id}"]`;

async function open(relaunch: Parameters<typeof openPlan>[0], extra = taskFilesControl()) {
  const { window } = await openPlan(relaunch, extra);
  await window.setViewportSize({ width: 1400, height: 900 });
  await openTaskFiles(window);
  await expect(window.locator(t('ade-files-row')).first()).toBeVisible();
  return window;
}

test('files tab: tree with an open file', async ({ relaunch }) => {
  const window = await open(relaunch);
  await openTaskFile(window, 'README.md');
  await expect(window.locator(t('ade-files-tab'))).toHaveScreenshot('files-open.png');
});

test('files tab: markdown reading', async ({ relaunch }) => {
  const window = await open(relaunch);
  await openTaskFile(window, 'README.md');
  await window.locator(t('ade-files-view-reading')).click();
  await expect(window.locator(t('ade-files-markdown'))).toBeVisible();
  await expect(window.locator(t('ade-files-tab'))).toHaveScreenshot('files-reading.png');
});

test('files tab: search results', async ({ relaunch }) => {
  const window = await open(relaunch);
  await window.locator(t('ade-files-mode-search')).click();
  const input = window.locator(t('ade-files-search-input'));
  await input.fill('needle');
  await input.press('Enter');
  await emitWailsEvent(window, IPC.codeSearch, taskSearchEvent());
  await expect(window.locator(t('ade-files-search-file-row')).first()).toBeVisible();
  await expect(window.locator(t('ade-files-tab'))).toHaveScreenshot('files-search.png');
});

test('files tab: launch error', async ({ relaunch }) => {
  const missing = contract<{ code: string; message: string }>('ade-task-files', 'editorMissing');
  const window = await open(
    relaunch,
    taskFilesControl([{ channel: IPC.adeTaskFileOpenInEditor, error: missing }]),
  );
  await window.locator(t('ade-files-open-editor')).click();
  await expect(window.locator(t('ade-files-launch-error'))).toBeVisible();
  await expect(window.locator(t('ade-files-tab'))).toHaveScreenshot('files-launch-error.png');
});

test('files tab: no worktree', async ({ relaunch }) => {
  const { window } = await openPlan(relaunch, taskFilesControl());
  await window.setViewportSize({ width: 1400, height: 900 });
  await openTask(window, 'T_alerts');
  await window.locator(t('ade-panel-tab-files')).click();
  await expect(window.locator(t('ade-files-empty'))).toBeVisible();
  await expect(window.locator(t('ade-files-empty'))).toHaveScreenshot('files-empty.png');
});
