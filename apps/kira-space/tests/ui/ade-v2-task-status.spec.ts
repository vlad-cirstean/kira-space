import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// Task statuses editor on the Workflows page and manual moves. Backend half: adeflow TestTaskStatuses.

const t = (id: string) => `[data-testid="${id}"]`;
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;
const item = (id: string) => t(`menu-item-${id}`);

interface StatusDef {
  id: string;
  name: string;
  color: string;
}
interface Defaults {
  new: string;
  step: string;
  review: string;
  finished: string;
}
interface Saved {
  statuses: StatusDef[];
  defaults: Defaults;
}
interface SaveArgs extends Saved {
  reassign: Record<string, string>;
}

async function openEditor(page: Page): Promise<void> {
  await page.locator(t('ade-tab-workflows')).click();
  await page.locator(t('ade-workflows')).waitFor();
  await page.locator(t('ade-wf-tab-statuses')).click();
  await expect(page.locator(t('ade-status-list'))).toBeVisible();
}

const sentSaves = (control: Parameters<typeof calls>[0]) =>
  calls(control, IPC.adeTaskSaveStatuses).map((c) => c.args as SaveArgs);

test('contract: the menu Status submenu sends the backend patch', async ({ relaunch }) => {
  const args = contract<{ patch: unknown }>('ade-task-statuses', 'args:AdeTaskService.UpdateTask');
  const { window: page, control } = await openPlan(relaunch);
  await page.locator(card('T_alerts')).click({ button: 'right', position: { x: 60, y: 30 } });
  await page.locator(item('ade-task-status')).click();
  await page.locator(item('ade-task-status-in-review')).click();
  await expect.poll(() => calls(control, IPC.adeTaskUpdateTask)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskUpdateTask)[0]?.args as { taskId: string; patch: unknown };
  expect(sent.taskId).toBe('T_alerts');
  expect(sent.patch).toEqual(args.patch);
});

test('contract: the saved set renders its renamed and recoloured status', async ({ relaunch }) => {
  const saved = contract<Saved>('ade-task-statuses', 'AdeTaskService.SaveStatuses#saved');
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        b.statuses = saved.statuses;
        b.statusDefaults = saved.defaults;
      }),
    },
  ]);
  await openTask(page, 'T_alerts');
  await expect(page.locator(t('ade-status-pick'))).toContainText('Doing');
  await expect(page.locator(`${t('ade-chip')}[data-color="orange"]`).first()).toContainText(
    'Doing',
  );
  await page.locator(t('ade-status-pick')).click();
  await expect(page.locator(t('ade-status-option-in-review'))).toContainText('In review');
});

test('the panel moves a task back and forth, by arrows and by picker', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openTask(page, 'T_alerts');
  await expect(page.locator(t('ade-status-pick'))).toContainText('In progress');
  const sent = () =>
    calls(control, IPC.adeTaskUpdateTask).map(
      (c) => (c.args as { patch: { statusId: string } }).patch.statusId,
    );

  await page.locator(t('ade-status-next')).click();
  await expect.poll(sent).toEqual(['in-review']);
  await expect(page.locator(t('ade-task-status-why'))).toBeVisible();
  await page.locator(t('ade-status-back')).click();
  await expect.poll(sent).toHaveLength(2);
  await page.locator(t('ade-status-pick')).click();
  await page.locator(t('ade-status-option-done')).click();
  await expect.poll(sent).toHaveLength(3);
  expect(sent()[2]).toBe('done');
});

test('back is off on the first status', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(t('ade-load-all')).click();
  await page.locator(card('T_csv')).scrollIntoViewIfNeeded();
  await openTask(page, 'T_csv');
  await expect(page.locator(t('ade-status-pick'))).toContainText('To do');
  await expect(page.locator(t('ade-status-back'))).toBeDisabled();
});

test('a stuck task shows Blocked beside its status', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_cart');
  await expect(page.locator(t('ade-status-pick'))).toContainText('In review');
  await expect(page.locator(t('ade-task-blocked'))).toBeVisible();
});

test('the editor lists the set with its colours and defaults', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openEditor(page);
  await expect(page.locator(t('ade-status-row'))).toHaveCount(4);
  await expect(
    page.locator(`${t('ade-status-row')}[data-status-id="in-progress"] ${t('ade-status-color')}`),
  ).toHaveAttribute('data-color', 'amber');
  await expect(page.locator(t('ade-status-default-new'))).toHaveValue('todo');
  await expect(page.locator(t('ade-status-default-step'))).toHaveValue('in-progress');
  await expect(page.locator(t('ade-status-default-review'))).toHaveValue('in-review');
  await expect(page.locator(t('ade-status-default-finished'))).toHaveValue('done');
  // New tasks always start somewhere: no keep option there.
  await expect(page.locator(`${t('ade-status-default-new')} option`)).toHaveCount(4);
  await expect(page.locator(`${t('ade-status-default-step')} option`)).toHaveCount(5);
});

test('the editor adds, renames and recolours a status', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openEditor(page);

  await page.locator(t('ade-status-add')).click();
  await expect.poll(() => sentSaves(control)).toHaveLength(1);
  const added = sentSaves(control)[0] as SaveArgs;
  expect(added.statuses).toHaveLength(5);
  // The first palette colour no status uses yet.
  expect(added.statuses[4]).toEqual({ id: '', name: 'New status', color: 'brown' });

  const first = page.locator(`${t('ade-status-row')}[data-status-id="in-progress"]`);
  await first.locator(t('ade-status-name')).fill('Working');
  await first.locator(t('ade-status-name')).press('Enter');
  await expect.poll(() => sentSaves(control)).toHaveLength(2);
  expect(sentSaves(control)[1]?.statuses.map((s) => s.name)).toEqual([
    'To do',
    'Working',
    'In review',
    'Done',
  ]);

  await first.locator(t('ade-status-color')).click();
  await expect(page.locator('[data-testid^="ade-status-color-"][role="radio"]')).toHaveCount(16);
  await page.locator(t('ade-status-color-purple')).click();
  await expect.poll(() => sentSaves(control)).toHaveLength(3);
  expect(sentSaves(control)[2]?.statuses[1]?.color).toBe('purple');
});

test('the editor reorders with the keyboard', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openEditor(page);
  await page
    .locator(`${t('ade-status-row')}[data-status-id="in-review"] ${t('ade-status-handle')}`)
    .focus();
  await page.keyboard.press('Alt+ArrowUp');
  await expect.poll(() => sentSaves(control)).toHaveLength(1);
  expect(sentSaves(control)[0]?.statuses.map((s) => s.id)).toEqual([
    'todo',
    'in-review',
    'in-progress',
    'done',
  ]);
});

test('a default select sends the defaults', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openEditor(page);
  await page.locator(t('ade-status-default-finished')).selectOption('');
  await expect.poll(() => sentSaves(control)).toHaveLength(1);
  expect(sentSaves(control)[0]?.defaults).toEqual({
    new: 'todo',
    step: 'in-progress',
    review: 'in-review',
    finished: '',
  });
  await page.locator(t('ade-status-default-review')).selectOption('done');
  await expect.poll(() => sentSaves(control)).toHaveLength(2);
  expect(sentSaves(control)[1]?.defaults.review).toBe('done');
});

test('contract: deleting a status asks where its tasks go', async ({ relaunch }) => {
  const args = contract<SaveArgs>('ade-task-statuses', 'args:AdeTaskService.SaveStatuses#delete');
  const saved = contract<Saved>('ade-task-statuses', 'AdeTaskService.SaveStatuses#saved');
  const { window: page, control } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        b.statuses = saved.statuses;
        b.statusDefaults = saved.defaults;
      }),
    },
  ]);
  await openEditor(page);
  await page
    .locator(`${t('ade-status-row')}[data-status-id="in-progress"] ${t('ade-status-delete')}`)
    .click();
  await expect(page.locator(t('ade-status-delete-dialog'))).toBeVisible();
  // What uses it: tasks, the step default; the fixture workflows name it on steps.
  await expect(page.locator(t('ade-status-uses'))).toContainText('When a step starts');
  await expect(page.locator(t('ade-status-reassign'))).toHaveValue('todo');
  await page.locator(t('ade-status-delete-confirm')).click();
  await expect.poll(() => sentSaves(control)).toHaveLength(1);
  expect(sentSaves(control)[0]).toEqual(args);
});

test('the last status cannot be deleted', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        b.statuses = [{ id: 'todo', name: 'To do', color: 'grey' }];
        b.statusDefaults = { new: 'todo', step: '', review: '', finished: '' };
        for (const task of b.tasks) task.statusId = 'todo';
      }),
    },
  ]);
  await openEditor(page);
  await expect(page.locator(t('ade-status-delete'))).toBeDisabled();
});

test('the step inspector offers the default and opens the editor', async ({ relaunch }) => {
  const saved = contract<Saved>('ade-task-statuses', 'AdeTaskService.SaveStatuses#saved');
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        b.statuses = saved.statuses;
        b.statusDefaults = saved.defaults;
      }),
    },
  ]);
  await page.locator(t('ade-tab-workflows')).click();
  await page.locator(t('ade-workflows')).waitFor();
  await page.locator(t('ade-wf-row')).first().click();
  await page.locator(t('ade-wf-node')).first().click();
  const toggle = page.locator(t('ade-wf-step-advanced'));
  if ((await toggle.getAttribute('aria-expanded')) !== 'true') await toggle.click();
  const options = await page.locator(t('ade-wf-step-status')).locator('option').allTextContents();
  expect(options).toContain('Doing');
  expect(options.some((o) => o.startsWith('Default ('))).toBe(true);
  await page.locator(t('ade-step-edit-statuses')).click();
  await expect(page.locator(t('ade-status-list'))).toBeVisible();
});

test('Settings has no Task statuses section', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await expect(page.locator(t('settings-section-Appearance'))).toBeVisible();
  await expect(page.locator(t('settings-section-Task statuses'))).toHaveCount(0);
});
