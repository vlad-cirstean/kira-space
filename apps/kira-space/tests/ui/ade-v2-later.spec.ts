import type { Page } from '@playwright/test';
import { sortableHover } from '@workbench/testing/ui/drag';
import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// The Later tab: Later and Backlog sections of tasks taken off the Plan; drag to reorder or to
// the other section. Contract ade-shelf. Backend half: adeflow TestShelves.

const t = (id: string) => `[data-testid="${id}"]`;

type Json = Record<string, unknown>;
const board = () =>
  contract<Json & { tasks: { id: string; title: string }[] }>(
    'ade-shelf',
    'AdeTaskService.Board#shelves',
  );
const idOf = (title: string): string => {
  const task = board().tasks.find((x) => x.title === title);
  if (!task) throw new Error(`contract has no task ${title}`);
  return task.id;
};
const BOARD: ControlSnapshot = { channel: IPC.adeTaskBoard, response: board() };

const row = (page: Page, title: string) =>
  page.locator(`${t('ade-shelf-row')}[data-task-id="${idOf(title)}"]`);
const titles = (page: Page, shelf: 'later' | 'backlog') =>
  page.locator(`${t(`ade-shelf-${shelf}`)} ${t('ade-shelf-title')}`).allInnerTexts();

async function openLater(
  relaunch: Parameters<typeof openPlan>[0],
  extra: readonly ControlSnapshot[] = [],
) {
  const app = await openPlan(relaunch, [BOARD, ...extra]);
  await app.window.locator(t('ade-tab-later')).click();
  await app.window.locator(t('ade-later')).waitFor();
  return app;
}

type Box = { x: number; y: number; width: number; height: number };
async function dragOnto(page: Page, from: string, onto: string, frac: number): Promise<void> {
  const src = (await row(page, from).boundingBox()) as Box;
  const dst = (await row(page, onto).boundingBox()) as Box;
  const home = { x: src.x + src.width / 2, y: src.y + src.height / 2 };
  const end = { x: dst.x + dst.width / 2, y: dst.y + dst.height * frac };
  await sortableHover(page, home, end);
  await page.mouse.up();
}

test('contract: tabs read Needs you, Plan, Later with no count', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [BOARD]);
  const order = await page.locator('[role="tab"]').allInnerTexts();
  expect(order.slice(0, 3).map((x) => x.replace(/\d+$/, '').trim())).toEqual([
    'Needs you',
    'Plan',
    'Later',
  ]);
  await expect(page.locator(t('ade-tab-backlog'))).toHaveCount(0);
  await expect(page.locator(t('ade-backlog-count'))).toHaveCount(0);
});

test('contract: Later and Backlog sections list their tasks, no reorder arrows', async ({
  relaunch,
}) => {
  const { window: page } = await openLater(relaunch);
  expect(await titles(page, 'later')).toEqual(['planned', 'one']);
  expect(await titles(page, 'backlog')).toEqual(['two']);
  await expect(page.locator(t('ade-backlog-up'))).toHaveCount(0);
  await expect(page.locator(t('ade-backlog-down'))).toHaveCount(0);
  await expect(page.locator(t('ade-shelf-row')).first()).toHaveAttribute('data-selected', 'true');
});

test('contract: selecting a row opens its panel', async ({ relaunch }) => {
  const { window: page } = await openLater(relaunch);
  await row(page, 'one').click();
  await expect(row(page, 'one')).toHaveAttribute('data-selected', 'true');
  await expect(page.locator(t('ade-panel'))).toBeVisible();
  await expect(page.locator(t('ade-panel-title'))).toHaveValue('one');
  await expect(page.locator(t('ade-panel-shelve'))).toHaveText('Backlog');
  await row(page, 'two').click();
  await expect(page.locator(t('ade-panel-shelve'))).toHaveText('Later');
});

test('contract: dragging within Later sends ShelveTask before the drop target', async ({
  relaunch,
}) => {
  const { window: page, control } = await openLater(relaunch);
  await dragOnto(page, 'one', 'planned', 0.25);
  await expect.poll(() => calls(control, IPC.adeTaskShelveTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskShelveTask)[0]?.args).toEqual({
    taskId: idOf('one'),
    shelf: 'later',
    beforeId: idOf('planned'),
  });
});

test('contract: dragging into Backlog sends ShelveTask for that shelf', async ({ relaunch }) => {
  const { window: page, control } = await openLater(relaunch);
  await dragOnto(page, 'planned', 'two', 0.25);
  await expect.poll(() => calls(control, IPC.adeTaskShelveTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskShelveTask)[0]?.args).toEqual({
    taskId: idOf('planned'),
    shelf: 'backlog',
    beforeId: idOf('two'),
  });
});

test('contract: the menu offers Move to backlog right above Archive', async ({ relaunch }) => {
  const { window: page, control } = await openLater(relaunch);
  await row(page, 'one').click({ button: 'right' });
  const items = await page
    .locator(`${t('context-menu')} [role="menuitem"]`)
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-testid')));
  const at = items.indexOf('menu-item-ade-task-backlog');
  expect(at).toBeGreaterThan(-1);
  expect(items[at + 1]).toBe('menu-item-ade-task-archive');
  await page.locator(t('menu-item-ade-task-backlog')).click();
  await expect.poll(() => calls(control, IPC.adeTaskShelveTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskShelveTask)[0]?.args).toEqual({
    taskId: idOf('one'),
    shelf: 'backlog',
    beforeId: idOf('two'),
  });
});

test('contract: a Backlog task moves to Later from the panel button', async ({ relaunch }) => {
  const { window: page, control } = await openLater(relaunch);
  await row(page, 'two').click();
  await page.locator(t('ade-panel-shelve')).click();
  await expect.poll(() => calls(control, IPC.adeTaskShelveTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskShelveTask)[0]?.args).toEqual({
    taskId: idOf('two'),
    shelf: 'later',
    beforeId: '',
  });
});

test('contract: quick add sends CaptureTask', async ({ relaunch }) => {
  const { window: page, control } = await openLater(relaunch);
  const input = page.locator(t('ade-capture'));
  await input.fill('Ship the changelog');
  await input.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskCaptureTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskCaptureTask)[0]?.args).toEqual({
    title: 'Ship the changelog',
    day: null,
  });
});

test('an empty shelf says where to drag', async ({ relaunch }) => {
  const empty = { ...board(), plan: { ...(board().plan as Json), backlog: [] } };
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: empty },
  ]);
  await page.locator(t('ade-tab-later')).click();
  await expect(page.getByText('Nothing on the backlog yet. Drag tasks here')).toBeVisible();
  expect(await titles(page, 'backlog')).toEqual([]);
});
