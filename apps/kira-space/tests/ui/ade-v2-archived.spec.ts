import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, archivedTaskFx, bandOf, FIXED_NOW, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Archived work stays viewable with everything it kept, read-only, and Restore brings it back.
// Contract ade-archived. Backend half: adeflow TestArchivedTaskKeepsData.

const t = (id: string) => `[data-testid="${id}"]`;

function sessionsWith(branchId: string) {
  const fx = adeFixture<{ sessions: Record<string, unknown>[] }>('sessions');
  const stopped = {
    ...fx.sessions.find((s) => s.id === 'sp11'),
    id: 'h1s1',
    taskId: 'H_1',
    branchId,
  };
  return { channel: IPC.adeTaskSessions, response: { ...fx, sessions: [...fx.sessions, stopped] } };
}

async function pickArchived(page: Page, taskId: string): Promise<void> {
  await page.locator(t('ade-archived-find')).click();
  await page.locator(`${t('ade-archived-find-item')}[data-task-id="${taskId}"]`).click();
}

test('the archived finder lists archived tasks and filters them', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator(t('ade-tab-archived'))).toHaveCount(0);
  await page.locator(t('ade-archived-find')).click();
  await expect(page.locator(t('ade-archived-find-item'))).toHaveCount(5);
  const input = page.getByPlaceholder('Find archived task');
  await input.fill('login');
  await expect(page.locator(t('ade-archived-find-item'))).toHaveCount(1);
  await expect(page.locator(t('ade-archived-find-item'))).toContainText('Fix login redirect loop');
  await input.fill('zzz');
  await expect(page.locator(t('ade-archived-find-empty'))).toBeVisible();
});

test('picking an old archived task reveals its day and opens its panel', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx('H_5') },
  ]);
  await pickArchived(page, 'H_5');
  await expect(page.locator(`${t('ade-history-row')}[data-task-id="H_5"]`)).toBeVisible();
  await expect(page.locator(t('ade-panel-title'))).toBeVisible();
});

test('finding a task archived today opens History on Today', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    const h = (b.history as { taskId: string; archivedAt: number }[]).find(
      (e) => e.taskId === 'H_1',
    );
    if (h) h.archivedAt = FIXED_NOW - 36e5;
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx('H_1') },
  ]);
  await pickArchived(page, 'H_1');
  await expect(page.locator(t('ade-load-history'))).toContainText('Hide history');
  await expect(
    bandOf(page, 0).locator(`${t('ade-history-row')}[data-task-id="H_1"]`),
  ).toBeVisible();
  await expect(page.locator(t('ade-panel-title'))).toBeVisible();
});

test('a selected archived task shows all it kept, read-only', async ({ relaunch }) => {
  const fx = archivedTaskFx();
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArchivedTask, response: fx },
    sessionsWith(fx.branches[0].id),
    {
      channel: IPC.adeTaskHandovers,
      response: {
        handovers: [
          {
            id: 'n1',
            taskId: 'H_1',
            fromStepId: '',
            fromStepName: '',
            toStepId: '',
            toStepName: '',
            body: fx.task.notes,
            source: 'user',
            state: 'note',
            finished: false,
            dismissed: false,
            edited: false,
            sessionId: '',
            createdAt: 1,
            updatedAt: 1,
          },
        ],
      },
    },
  ]);
  await pickArchived(page, 'H_1');
  await expect.poll(() => calls(control, IPC.adeTaskArchivedTask)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskArchivedTask)[0]?.args as Record<string, unknown>;
  expect(sent).toEqual({ taskId: 'H_1' });
  expect(Object.keys(sent)).toEqual(
    Object.keys(
      contract<Record<string, unknown>>('ade-archived', 'args:AdeTaskService.ArchivedTask'),
    ),
  );

  await expect(page.locator(t('ade-panel-title'))).toHaveText('Fix login');
  await expect(page.locator(t('ade-archived-status'))).toHaveText('Done');
  const labels = (
    await page.locator(t('ade-archived-task-tab')).locator('dt').allTextContents()
  ).map((s) => s.trim());
  expect(labels).toContain('Issue');
  expect(labels).toContain('PR');
  await expect(page.locator(t('ade-archived-jira-link'))).toHaveText('KIRA-12');
  await expect(page.locator(t('ade-archived-host-link'))).toHaveText('api!9');
  const branch = page.locator(t('ade-archived-branch'));
  await expect(branch).toContainText('feat/api-keep');
  await expect(page.locator(t('ade-archived-pr'))).toContainText('#7');
  await expect(page.locator(t('ade-archived-pr'))).toContainText('Fix login');
  await expect(page.locator(t('ade-archived-snapshot'))).toHaveCount(0);
  await expect(page.locator(t('ade-archived-comments'))).toContainText('Review comments (1)');
  await page.locator(t('ade-archived-comments')).getByRole('button').click();
  await expect(page.locator(t('ade-archived-comment'))).toContainText('rename this');

  await page.locator(t('ade-panel-tab-details')).click();
  const editor = page.locator(`${t('ade-details-tab')} .ProseMirror`);
  await expect(editor).toContainText('remember the cache');
  await expect(editor).toHaveAttribute('contenteditable', 'false');
  await expect(page.locator(t('ade-notes-toolbar-btn'))).toHaveCount(0);

  await page.locator(t('ade-panel-tab-sessions')).click();
  await expect(page.locator(t('ade-sessions-row')).first()).toContainText('feat/api-keep');
});

test('a stored change request is marked as of archive', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx('H_1', 'snapshot') },
  ]);
  await pickArchived(page, 'H_1');
  await expect(page.locator(t('ade-archived-pr'))).toContainText('#7');
  await expect(page.locator(t('ade-archived-snapshot'))).toBeVisible();
});

test('Restore sends the task id and opens it in the Plan', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await pickArchived(page, 'H_1');
  await page.locator(t('ade-archived-restore')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRestoreTask)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskRestoreTask)[0]?.args as Record<string, unknown>;
  expect(sent).toEqual({ taskId: 'H_1' });
  expect(Object.keys(sent)).toEqual(
    Object.keys(
      contract<Record<string, unknown>>('ade-archived', 'args:AdeTaskService.RestoreTask'),
    ),
  );
  await expect(page.locator(t('ade-plan'))).toBeVisible();
  await expect(page.locator(t('ade-panel'))).toBeVisible();
});

test('a refused Restore shows the error and keeps the archived panel', async ({ relaunch }) => {
  const refusal = JSON.parse(
    contract<string>('ade-archived', 'error:AdeTaskService.RestoreTask#branch-live'),
  ) as { code: string; message: string };
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskRestoreTask, error: refusal },
  ]);
  await pickArchived(page, 'H_1');
  await page.locator(t('ade-archived-restore')).click();
  await expect(page.locator(t('ade-archived-restore-error'))).toContainText(
    'branch feat/api-keep is on task',
  );
  await expect(page.locator(t('ade-archived-restore'))).toBeVisible();
});

test('a Plan history item opens the same full panel beside the Plan', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(t('ade-load-history')).click();
  await page.locator(t('ade-history-row')).first().click();
  await expect(page.locator(t('ade-plan'))).toBeVisible();
  await expect(page.locator(t('ade-panel-title'))).toHaveText('Fix login');
  await expect(page.locator(t('ade-archived-restore'))).toBeVisible();
  await expect(page.locator(t('ade-archived-task-tab'))).toBeVisible();
});

async function openArchivedSessions(page: Page): Promise<void> {
  await pickArchived(page, 'H_1');
  await page.locator(t('ade-panel-tab-sessions')).click();
}

test('Resume restores the task, then takes the session over', async ({ relaunch }) => {
  const fx = archivedTaskFx();
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArchivedTask, response: fx },
    sessionsWith(fx.branches[0].id),
  ]);
  await openArchivedSessions(page);
  await expect(
    page.locator(t('ade-sessions-row-hint')).filter({ hasText: 'Take over' }),
  ).toHaveCount(0);
  await page.locator(t('ade-sessions-row')).first().click();
  await expect.poll(() => calls(control, IPC.adeTaskTakeOver)).toHaveLength(1);
  const restore = calls(control, IPC.adeTaskRestoreTask);
  expect(restore).toHaveLength(1);
  expect(restore[0]?.args).toEqual({ taskId: 'H_1' });
  const sent = calls(control, IPC.adeTaskTakeOver)[0]?.args as Record<string, unknown>;
  expect(sent).toEqual({ sessionId: 'h1s1', stopIfRunning: true });
  expect(Object.keys(sent)).toEqual(
    Object.keys(contract<Record<string, unknown>>('ade-archived', 'args:AdeTaskService.TakeOver')),
  );
});

test('a refused Resume shows the error and launches nothing', async ({ relaunch }) => {
  const refusal = JSON.parse(
    contract<string>('ade-archived', 'error:AdeTaskService.RestoreTask#branch-live'),
  ) as { code: string; message: string };
  const fx = archivedTaskFx();
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArchivedTask, response: fx },
    sessionsWith(fx.branches[0].id),
    { channel: IPC.adeTaskRestoreTask, error: refusal },
  ]);
  await openArchivedSessions(page);
  await page.locator(t('ade-sessions-row')).first().click();
  await expect(page.locator(t('ade-archived-resume-error'))).toContainText(
    'branch feat/api-keep is on task',
  );
  expect(calls(control, IPC.adeTaskTakeOver)).toHaveLength(0);
});
