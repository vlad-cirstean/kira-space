import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Task row: agent indicator leads, manual status is small text with a dropdown. Backend half:
// adeflow TestTaskStatuses (same UpdateTask patch, contract ade-task-statuses).

const t = (id: string) => `[data-testid="${id}"]`;
const card = (id: string) => `[data-testid="ade-card"][data-task-id="${id}"]`;
const head = (id: string) => `${card(id)} ${t('ade-card-head')}`;

const sent = (control: Parameters<typeof calls>[0]) =>
  calls(control, IPC.adeTaskUpdateTask).map((c) => c.args as { taskId: string; patch: unknown });

async function firstChildTestId(page: Page, selector: string): Promise<string | null> {
  return page
    .locator(selector)
    .first()
    .evaluate((el) => {
      const first = el.firstElementChild as HTMLElement | null;
      return first?.getAttribute('data-testid') ?? null;
    });
}

test('the lead slot holds the agent indicator first and no colour square remains', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  for (const id of ['T_bill', 'T_hooks']) {
    const line = `${head(id)} > div`;
    expect(await firstChildTestId(page, line)).toBe('ade-row-lead');
    await expect(
      page.locator(`${line} ${t('ade-row-lead')} ${t('ade-card-sessions')}`),
    ).toHaveCount(1);
  }
  await expect(page.locator(t('ade-card-dot'))).toHaveCount(0);
  // A task without sessions keeps an empty slot so status text lines up.
  await page.locator(t('ade-load-all')).click();
  const none = page.locator(`${head('T_csv')} ${t('ade-row-lead')}`);
  await expect(none).toHaveCount(1);
  await expect(none.locator(t('ade-card-sessions'))).toHaveCount(0);
});

test('contract: the status text opens a dropdown and sends the backend patch', async ({
  relaunch,
}) => {
  const args = contract<{ patch: unknown }>('ade-task-statuses', 'args:AdeTaskService.UpdateTask');
  const { window: page, control } = await openPlan(relaunch);
  const status = page.locator(`${head('T_alerts')} ${t('ade-row-status')}`);
  await expect(status).toHaveText('In progress');
  await status.click();
  await expect(page.locator(t('ade-row-status-menu'))).toBeVisible();
  await expect(page.locator(card('T_alerts'))).not.toHaveAttribute('aria-current', 'true');
  await expect(page.locator(t('ade-row-status-option-in-progress'))).toHaveAttribute(
    'aria-checked',
    'true',
  );
  await page.locator(t('ade-row-status-option-in-review')).click();
  await expect.poll(() => sent(control)).toHaveLength(1);
  expect(sent(control)[0]?.taskId).toBe('T_alerts');
  expect(sent(control)[0]?.patch).toEqual(args.patch);
});

test('the keyboard opens the dropdown, picks, and Escape writes nothing', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  const status = page.locator(`${head('T_alerts')} ${t('ade-row-status')}`);
  await status.focus();
  await page.keyboard.press('Enter');
  await expect(page.locator(t('ade-row-status-menu'))).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(t('ade-row-status-menu'))).toHaveCount(0);
  expect(sent(control)).toHaveLength(0);
  await expect(page.locator(card('T_alerts'))).not.toHaveAttribute('aria-current', 'true');

  await status.focus();
  await page.keyboard.press('Enter');
  await expect(page.locator(t('ade-row-status-menu'))).toBeVisible();
  await page.keyboard.press('End');
  await expect(page.locator(t('ade-row-status-option-done'))).toBeFocused();
  await page.keyboard.press('Enter');
  await expect.poll(() => sent(control)).toHaveLength(1);
  expect(sent(control)[0]?.taskId).toBe('T_alerts');
});

test('picking the current status writes nothing', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await page.locator(`${head('T_alerts')} ${t('ade-row-status')}`).click();
  await page.locator(t('ade-row-status-option-in-progress')).click();
  await expect(page.locator(t('ade-row-status-menu'))).toHaveCount(0);
  expect(sent(control)).toHaveLength(0);
});

test('a write error selects the task and shows in the panel', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskUpdateTask, error: { code: 'failed', message: 'status refused' } },
  ]);
  await page.locator(`${head('T_alerts')} ${t('ade-row-status')}`).click();
  await page.locator(t('ade-row-status-option-in-review')).click();
  await expect(page.locator(t('ade-panel-action-error'))).toContainText('status refused');
  await expect(page.locator(card('T_alerts'))).toHaveAttribute('aria-current', 'true');
});

test('the status glyph is gone but a blocked task keeps its tag beside the status', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await expect(
    page.locator(`${t('ade-card-head')} ${t('ade-tag')}[data-kind="status"][data-color]`),
  ).toHaveCount(0);
  const cart = page.locator(head('T_cart'));
  await expect(cart.locator(t('ade-row-status'))).toBeVisible();
  await expect(cart.locator(`${t('ade-tag')}[data-kind="blocked"]`)).toHaveCount(1);
});

test('a Later shelf row leads with the slot, shows status text and sends the patch', async ({
  relaunch,
}) => {
  const board = contract<{ tasks: { id: string; title: string }[] }>(
    'ade-shelf',
    'AdeTaskService.Board#shelves',
  );
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(t('ade-tab-later')).click();
  await page.locator(t('ade-later')).waitFor();
  await expect(page.locator(t('ade-shelf-dot'))).toHaveCount(0);
  const row = page.locator(t('ade-shelf-row')).first();
  expect(await firstChildTestId(page, t('ade-shelf-row'))).toBe('ade-row-lead');
  const taskId = await row.getAttribute('data-task-id');
  const status = row.locator(t('ade-row-status'));
  await expect(status).toBeVisible();
  await status.click();
  const current = await status.getAttribute('data-status-id');
  await page
    .locator(
      `[data-testid^="ade-row-status-option-"]:not([data-testid="ade-row-status-option-${current}"])`,
    )
    .first()
    .click();
  await expect.poll(() => sent(control)).toHaveLength(1);
  expect(sent(control)[0]?.taskId).toBe(taskId);
});
