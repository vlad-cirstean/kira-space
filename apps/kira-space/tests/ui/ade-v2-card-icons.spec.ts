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
  const tag = head.locator(t('ade-tag'));
  await expect(tag).toHaveAttribute('data-kind', 'status');
  await expect(tag).toHaveAttribute('aria-label', /.+/);
  const order = await head.evaluate((el) => {
    const idx = (id: string) =>
      [...el.querySelectorAll('*')].findIndex((e) => e.getAttribute('data-testid') === id);
    return { tag: idx('ade-tag'), meta: idx('ade-card-meta') };
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

test('a failed rebase row shows the tag, one action and More; More lists the rest', async ({
  relaunch,
}) => {
  const { window: page } = await open(relaunch, failedRebase);
  const r = page.locator(row('b_auth'));
  await expect(r.locator(t('ade-tag'))).toHaveAttribute('data-kind', 'rebasePending');
  await expect(r.locator(t('ade-branch-more'))).toBeVisible();
  const cluster = r.locator(
    `${t('ade-tag')}, ${t('ade-branch-more')}, [data-testid^="ade-rebase"], ${t('ade-abort-rebase')}, ${t('ade-see-log')}, ${t('ade-take-over')}`,
  );
  await expect(cluster).toHaveCount(3);
  await r.locator(t('ade-branch-more')).click();
  await expect(page.locator(item('ade-row-action-seeLog'))).toBeVisible();
  await expect(page.locator(item('ade-row-action-takeOver'))).toBeVisible();
  await expect(page.locator('[data-testid^="menu-item-ade-fix-"]').first()).toBeVisible();
  await page.locator(item('ade-row-action-seeLog')).click();
  await expect(page.locator(t('ade-branch-details'))).toBeVisible();
});

test('row and card icons are always visible, never hover-only', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, failedRebase);
  await page.mouse.move(0, 0);
  const r = page.locator(row('b_auth'));
  for (const id of ['ade-tag', 'ade-branch-more']) {
    const el = r.locator(t(id));
    await expect(el).toBeVisible();
    expect(await el.evaluate((e) => getComputedStyle(e).opacity)).toBe('1');
  }
  const review = page.locator(t('ade-branch-review')).first();
  if (await review.count()) {
    await expect(review).toBeVisible();
    expect(await review.evaluate((e) => getComputedStyle(e).opacity)).toBe('1');
  }
});

test('right-click on a tag opens the matching menu', async ({ relaunch }) => {
  const { window: page } = await open(relaunch, failedRebase);
  await page
    .locator(`${task('T_auth')} ${t('ade-card-head')} ${t('ade-tag')}`)
    .click({ button: 'right' });
  await expect(page.locator(item('ade-task-step'))).toBeVisible();
  await page.keyboard.press('Escape');
  await page.locator(`${row('b_auth')} ${t('ade-tag')}`).click({ button: 'right' });
  await expect(page.locator('[data-testid^="menu-item-ade-fix-"]').first()).toBeVisible();
  await expect(page.locator(item('ade-task-step'))).toHaveCount(0);
});
