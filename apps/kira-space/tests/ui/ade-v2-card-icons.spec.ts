import { expect, test } from './fixtures';
import { type AdeBoardFx, adeBoard, adeFixture, openPlan } from './support/adeV2';
import { IPC } from './support/ipcChannels';

// The Plan card's icon row: the tag and the step action in the head line, the tag and at most
// two actions at the right edge of each branch row.

const t = (id: string) => `[data-testid="${id}"]`;
const task = (id: string) => `[data-testid="ade-task"][data-task-id="${id}"]`;
const row = (id: string) => `[data-testid="ade-branch-row"][data-branch-id="${id}"]`;
const item = (id: string) => t(`menu-item-${id}`);

type Edit = (b: AdeBoardFx) => void;
const branch = (b: AdeBoardFx, id: string) =>
  b.branches.find((x) => x.id === id) as Record<string, unknown>;

function rebaseRun(over: Record<string, unknown>) {
  return {
    id: 'r_rb1',
    taskId: 'T_auth',
    stepId: '',
    branchId: 'b_auth',
    attempt: 1,
    state: 'failed',
    loops: 0,
    note: '',
    summary: '',
    sessionId: 's_rb',
    exitCode: 1,
    startedAt: 1790060000000,
    finishedAt: 1790060100000,
    purpose: 'rebase',
    outcome: null,
    ...over,
  };
}

const failedRebase: Edit = (b) => {
  Object.assign(branch(b, 'b_auth'), { base: 'develop', basePendingFrom: 'main' });
  b.tasks.find((x) => x.id === 'T_auth')?.runs.push(rebaseRun({}));
};

async function open(relaunch: Parameters<typeof openPlan>[0], edit: Edit) {
  const board = adeBoard(edit);
  return openPlan(relaunch, [{ channel: IPC.adeTaskBoard, response: board }]);
}

test('the left action column is gone', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator(t('ade-task'))).not.toHaveCount(0);
  await expect(page.locator(t('ade-task-cells'))).toHaveCount(0);
  await expect(page.locator(t('ade-action-cell'))).toHaveCount(0);
});

test('the task tag and step action are icons in the head line', async ({ relaunch }) => {
  const sessions = adeFixture<{ sessions: { id: string }[] }>('sessions');
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskSessions,
      response: { ...sessions, sessions: sessions.sessions.filter((s) => s.id !== '9ab0') },
    },
  ]);
  const head = page.locator(`${task('T_auth')} ${t('ade-card-head')}`);
  const status = head.locator(t('ade-row-status'));
  await expect(status).toHaveAttribute('aria-label', /.+/);
  await expect(head.locator(`${t('ade-tag')}[data-kind="status"][data-color]`)).toHaveCount(0);
  const order = await head.evaluate((el) => {
    const idx = (id: string) =>
      [...el.querySelectorAll('*')].findIndex((e) => e.getAttribute('data-testid') === id);
    return { tag: idx('ade-row-status'), meta: idx('ade-card-meta') };
  });
  expect(order.tag).toBeGreaterThanOrEqual(0);
  expect(order.tag).toBeLessThan(order.meta);
  const action = head.locator(t('ade-task-action-session'));
  await expect(action).toHaveText('');
  const last = await head
    .locator('> div')
    .first()
    .locator('button')
    .last()
    .getAttribute('data-testid');
  expect(last).toBe('ade-task-action-session');
  await action.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    (await action.getAttribute('aria-label')) ?? '',
  );
  await action.click();
  await expect(page.locator(t('ade-dialog'))).toBeVisible();
});

test('each branch row has one tag; kinds map to glyphs and urgent ones are solid', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, failedRebase);
  await page.locator(t('ade-load-all')).click();
  const rows = page.locator(t('ade-branch-row'));
  const n = await rows.count();
  for (let i = 0; i < n; i++) await expect(rows.nth(i).locator(t('ade-tag'))).toHaveCount(1);
  const tags = await page.locator(`${t('ade-branch-row')} ${t('ade-tag')}`).evaluateAll((els) =>
    els.map((e) => ({
      kind: e.getAttribute('data-kind') ?? '',
      icon: e.getAttribute('data-icon') ?? '',
      urgent: e.hasAttribute('data-urgent'),
      solid: /-solid/.test(e.className),
    })),
  );
  const conflictFamily = new Set(['conflict', 'rebaseConflicts', 'reviewConflict']);
  const byIcon = new Map<string, Set<string>>();
  for (const x of tags) {
    const k = conflictFamily.has(x.kind) ? 'conflict' : x.kind;
    byIcon.set(x.icon, (byIcon.get(x.icon) ?? new Set()).add(k));
    expect(x.solid).toBe(x.urgent);
  }
  for (const kinds of byIcon.values()) expect(kinds.size).toBe(1);
  expect(tags.some((x) => x.urgent)).toBe(true);
  expect(tags.some((x) => !x.urgent)).toBe(true);
  await expect(page.locator(`${row('b_search')} ${t('ade-tag')}`).first()).toBeVisible();
});

test('a failed rebase row shows the tag and one action; right-click lists the rest', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, failedRebase);
  const r = page.locator(row('b_auth'));
  await expect(r.locator(t('ade-tag'))).toHaveAttribute('data-kind', 'rebasePending');
  const cluster = r.locator(
    `${t('ade-tag')}, [data-testid^="ade-rebase"], ${t('ade-abort-rebase')}, ${t('ade-see-log')}, ${t('ade-take-over')}`,
  );
  await expect(cluster).toHaveCount(2);
  await r.click({ button: 'right' });
  await expect(page.locator(item('ade-row-action-seeLog'))).toBeVisible();
  await expect(page.locator(item('ade-row-action-takeOver'))).toBeVisible();
  await expect(page.locator('[data-testid^="menu-item-ade-fix-"]').first()).toBeVisible();
  await page.locator(item('ade-row-action-seeLog')).click();
  await expect(page.locator(t('ade-branch-details'))).toBeVisible();
});

test('state glyphs and danger actions stay visible; the action cluster shows on hover or keyboard focus', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, failedRebase);
  await page.mouse.move(0, 0);
  const r = page.locator(row('b_auth'));
  const head = page.locator(`${task('T_auth')} ${t('ade-card-head')}`);
  const danger = page.locator(`${task('T_search')} ${t('ade-see-error')}`);
  const idleVisible = [r.locator(t('ade-tag')), danger];
  for (const el of idleVisible) {
    await expect(el).toBeVisible();
    expect(await el.evaluate((e) => getComputedStyle(e).opacity)).toBe('1');
  }
  const rowCluster = r.locator(t('ade-task-branch-menu'));
  const headCluster = head.locator(t('ade-card-add'));
  await expect(rowCluster).toBeHidden();
  await expect(headCluster).toBeHidden();

  await r.hover();
  await expect(rowCluster).toBeVisible();
  await head.hover();
  await expect(headCluster).toBeVisible();
  await expect(rowCluster).toBeHidden();

  await page.mouse.move(0, 0);
  await expect(headCluster).toBeHidden();
  await head.click();
  await page.mouse.move(0, 0);
  await expect(headCluster).toBeHidden();
  for (const el of idleVisible) await expect(el).toBeVisible();

  await page.keyboard.press('Tab');
  await head.focus();
  await expect(head).toBeFocused();
  await expect(headCluster).toBeVisible();
  await expect(page.locator(`${t('ade-plan')} ${t('ade-card-more')}`)).toHaveCount(0);
  await expect(page.locator(`${t('ade-plan')} ${t('ade-branch-more')}`)).toHaveCount(0);
});

test('hover actions sit right', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, failedRebase);
  const head = page.locator(`${task('T_auth')} ${t('ade-card-head')}`);
  await head.hover();
  const actions = head.locator(t('ade-card-actions'));
  await expect(actions).toBeVisible();
  const title = await head.locator(t('ade-card-title')).boundingBox();
  const status = await head.locator(t('ade-row-status')).boundingBox();
  const box = await actions.boundingBox();
  if (!title || !status || !box) throw new Error('no box');
  expect(box.x).toBeGreaterThanOrEqual(title.x + title.width);
  expect(box.x + box.width).toBeLessThanOrEqual(status.x + 1);

  const r = page.locator(row('b_auth'));
  await r.hover();
  const rowActions = r.locator(t('ade-branch-actions'));
  await expect(rowActions).toBeVisible();
  const name = await r.locator(t('ade-branch-name')).boundingBox();
  const rb = await rowActions.boundingBox();
  const rr = await r.boundingBox();
  if (!name || !rb || !rr) throw new Error('no box');
  expect(rb.x).toBeGreaterThanOrEqual(name.x + name.width);
  expect(rr.x + rr.width - (rb.x + rb.width)).toBeLessThanOrEqual(40);
});

test('Shift+F10 on a focused row opens the row menu', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, failedRebase);
  await page.locator(row('b_auth')).focus();
  await page.keyboard.press('Shift+F10');
  await expect(page.locator(item('ade-row-action-seeLog'))).toBeVisible();
});

test('right-click on the status text or a branch tag opens the matching menu', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, failedRebase);
  await page
    .locator(`${task('T_auth')} ${t('ade-card-head')} ${t('ade-row-status')}`)
    .click({ button: 'right' });
  await expect(page.locator(item('ade-task-step'))).toBeVisible();
  await page.keyboard.press('Escape');
  await page.locator(`${row('b_auth')} ${t('ade-tag')}`).click({ button: 'right' });
  await expect(page.locator('[data-testid^="menu-item-ade-fix-"]').first()).toBeVisible();
  await expect(page.locator(item('ade-task-step'))).toHaveCount(0);
});
