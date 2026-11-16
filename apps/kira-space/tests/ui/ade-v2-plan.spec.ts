import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, bandOf, FIXED_NOW, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// The ade v2 Plan on the committed board fixture, anchored to Tue 2026-09-22.

interface BoardFx {
  plan: { order: string[]; day: Record<string, string>; unpushed: Record<string, boolean> };
  branches: { id: string; conflictCheck: string; conflictCheckReason: string }[];
  repos: { codeRepoId: string }[];
}

const card = (page: Page, taskId: string) =>
  page.locator(`[data-testid="ade-card"][data-task-id="${taskId}"]`);

test('shows the first 10 items, loads all and collapses back', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const tasks = page.locator('[data-testid="ade-task"]');
  await expect(tasks).toHaveCount(10);
  await page.locator('[data-testid="ade-load-all"]').click();
  await expect(tasks).toHaveCount(12);
  await expect(page.locator('[data-testid="ade-load-all"]')).toHaveCount(0);
  await page.locator('[data-testid="ade-collapse"]').click();
  await expect(tasks).toHaveCount(10);
  await expect(page.locator('[data-testid="ade-load-all"]')).toContainText(
    'Load all items · 2 more',
  );
});

test('loads the archived history into the day bands', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const load = page.locator('[data-testid="ade-load-history"]');
  await expect(load).toContainText('History');
  await load.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    '5 archived tasks in the last 2 weeks',
  );
  await load.click();
  await expect(page.locator('[data-testid="ade-history-row"]')).toHaveCount(5);
  await expect(load).toContainText('Hide history');
  await load.click();
  await expect(page.locator('[data-testid="ade-history-row"]')).toHaveCount(0);
});

test('a task archived today stays hidden until History opens, name first', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    const h = (b.history as { taskId: string; archivedAt: number }[]).find(
      (e) => e.taskId === 'H_1',
    );
    if (h) h.archivedAt = FIXED_NOW - 36e5;
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  const rows = page.locator('[data-testid="ade-history-row"]');
  const load = page.locator('[data-testid="ade-load-history"]');
  await expect(rows).toHaveCount(0);
  await expect(bandOf(page, 0).locator('[data-testid="ade-history-row"]')).toHaveCount(0);
  await expect(load).toContainText('5');
  await load.click();
  const row = bandOf(page, 0).locator('[data-testid="ade-history-row"][data-task-id="H_1"]');
  await expect(row).toBeVisible();
  await expect(row).toContainText('Audit log for admin actions');
  const box = await row.locator('span.font-medium').boundingBox();
  expect(box?.width ?? 0).toBeGreaterThan(80);
  await load.click();
  await expect(rows).toHaveCount(0);
});

test('weekend bands carry the hatched pattern; workdays do not', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const weekend = page.locator('[data-testid="ade-day-band"][data-weekend]');
  expect(await weekend.count()).toBeGreaterThan(0);
  const image = (band: ReturnType<Page['locator']>) =>
    band
      .locator('[data-testid="ade-band-drop"]')
      .evaluate((el) => getComputedStyle(el).backgroundImage);
  expect(await image(weekend.first())).toContain('repeating-linear-gradient');
  expect(await image(bandOf(page, 0))).toBe('none');
});

test('branch status tags keep their tones', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator('[data-testid="ade-load-all"]').click();
  const tags = await page
    .locator('[data-testid="ade-branch-row"] [data-testid="ade-tag"]')
    .evaluateAll((els) =>
      els.map((el) => ({
        text: (el.getAttribute('aria-label') ?? '').trim(),
        tone: el.getAttribute('data-tone'),
      })),
    );
  const toneOf = (text: string) => tags.filter((x) => x.text === text).map((x) => x.tone);
  expect(toneOf('Merged').length).toBeGreaterThan(0);
  expect(new Set(toneOf('Merged'))).toEqual(new Set(['purple']));
  expect(toneOf('Clean').length).toBeGreaterThan(0);
  expect(new Set(toneOf('Clean'))).toEqual(new Set(['green']));
  expect(tags.some((x) => x.tone === 'blue')).toBe(true);
});

test('a card shows its agent sessions as one state; click opens the Sessions tab', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  const state = (id: string) => card(page, id).locator('[data-testid="ade-card-sessions"]');
  await expect(state('T_bill')).toHaveAttribute('data-state', 'input');
  await expect(
    card(page, 'T_bill').locator('[data-testid="ade-row-lead"] [data-testid="ade-card-sessions"]'),
  ).toHaveCount(1);
  await expect(state('T_hooks')).toHaveAttribute('data-state', 'running');
  await expect(state('T_hooks')).toContainText('2');
  await expect(state('T_notif')).toHaveAttribute('data-state', 'running');
  await expect(state('T_notif')).not.toContainText('1');
  await expect(state('T_cart')).toHaveAttribute('data-state', 'finished');
  await state('T_hooks').click();
  await expect(page.locator('[data-testid="ade-sessions-tab"]')).toBeVisible();
});

test('a card shows its step, progress, span and a one-line title with the full text in its tooltip', async ({
  relaunch,
}) => {
  const longTitle =
    'Push notification settings with quiet hours, per-channel digests and a very long title';
  const { window: page } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        const t = b.tasks.find((x) => x.id === 'T_push') as { title: string } | undefined;
        if (t) t.title = longTitle;
      }),
    },
  ]);
  await page.setViewportSize({ width: 560, height: 900 });
  const search = card(page, 'T_search');
  await expect(search.locator('[data-testid="ade-step-label"]')).toHaveText('Make CI green 3/6');
  await expect(search).toContainText('50.0%');
  await expect(search.locator('[data-testid="ade-card-meta"]')).toHaveText(
    '3d → Mon 28 · SRCH-41 · web-app',
  );
  await expect(search.locator('[data-testid="ade-card-title"]')).toHaveText('Search v2');
  const title = card(page, 'T_push').locator('[data-testid="ade-card-title"]');
  await expect(title).toHaveText(longTitle);
  const line = await title
    .locator('span')
    .first()
    .evaluate((el) => {
      const s = getComputedStyle(el);
      return {
        textOverflow: s.textOverflow,
        whiteSpace: s.whiteSpace,
        cut: el.scrollWidth > el.clientWidth,
      };
    });
  expect(line).toEqual({ textOverflow: 'ellipsis', whiteSpace: 'nowrap', cut: true });
  await title.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(longTitle);
});

test('the Plan shows 7 days; later work sits in Later with its date; no date control', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator('[data-testid="ade-load-all"]').click();
  for (let k = 0; k <= 6; k++) await expect(bandOf(page, k)).toBeVisible();
  await expect(bandOf(page, 7)).toHaveCount(0);
  await expect(bandOf(page, 8)).toHaveCount(0);
  const later = bandOf(page, 9999);
  const ids = await later
    .locator('[data-testid="ade-card"]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-task-id')));
  expect(ids.indexOf('T_alerts')).toBeGreaterThanOrEqual(0);
  expect(ids.indexOf('T_alerts')).toBeLessThan(ids.indexOf('T_csv'));
  await expect(card(page, 'T_alerts').locator('[data-testid="ade-card-meta"]')).toHaveText(
    /^Tue 29 Sep · /,
  );
  await expect(card(page, 'T_csv').locator('[data-testid="ade-card-meta"]')).toHaveText(
    /^Wed 30 Sep · /,
  );
  await expect(page.locator('[data-testid="ade-more-week"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="ade-add-day"]')).toHaveCount(0);
});

test('branch rows show base markers and the derived branch tags', async ({ relaunch }) => {
  const board = adeFixture<BoardFx>('board');
  const deps = board.branches.find((b) => b.id === 'b_deps');
  if (!deps) throw new Error('fixture lost b_deps');
  deps.conflictCheck = 'failed';
  deps.conflictCheckReason = 'merge-tree exited 128';
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);

  const markers = await page.locator('[data-testid="ade-base-marker"]').allInnerTexts();
  expect(markers.map((m) => m.replace(/\s+/g, ''))).toContain('search-schema');
  const tags = await page
    .locator('[data-testid="ade-tag"]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('aria-label') ?? ''));
  expect(tags).toContain('3 behind main');
  expect(tags).toContain('Checking…');
  expect(tags).toContain('Conflict check failed');
  expect(tags).toContain('Setup failed');
});

test('Refresh all sends every repo the plan shows', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskRefresh, response: adeFixture('refresh') },
  ]);
  const calls = () => control.log().filter((e) => e.channel === IPC.adeTaskRefresh);
  await page.locator('[data-testid="ade-refresh-all"]').click();
  await expect.poll(() => calls().length).toBe(1);
  expect(calls()[0]?.args).toEqual({
    codeRepoIds: adeFixture<BoardFx>('board').repos.map((r) => r.codeRepoId),
  });
});

/** Drags a card onto a day band. Waits for the layout to settle first, else the band moves under the pointer. */
async function dragCardToBand(
  page: Page,
  taskId: string,
  day: number,
  target: 'ade-band-drop' | 'ade-band-head' = 'ade-band-drop',
): Promise<void> {
  const drop = bandOf(page, day).locator(`[data-testid="${target}"]`);
  await card(page, taskId).scrollIntoViewIfNeeded();
  const geometry = async () =>
    JSON.stringify([await card(page, taskId).boundingBox(), await drop.boundingBox()]);
  let last = '';
  await expect
    .poll(async () => {
      const now = await geometry();
      const settled = now === last;
      last = now;
      return settled;
    })
    .toBe(true);
  const [from, to] = JSON.parse(last) as [DOMRect | null, DOMRect | null];
  if (!from || !to) throw new Error('drag geometry missing');
  // A review card is one 22px row: grab inside it, not 30px down.
  const grabY = from.y + Math.min(30, from.height / 2);
  await page.mouse.move(from.x + 40, grabY);
  await page.mouse.down();
  await page.mouse.move(from.x + 60, grabY + 20, { steps: 4 });
  // Below the band's last card means "append to the day"; the top padding now means "before the
  // first card". The slot opens in each band the pointer crosses and shifts the bands below it, so
  // re-aim at the target's live rect until its own slot shows.
  const slot = bandOf(page, day).locator('[data-testid="ade-drop-slot"]');
  await expect(async () => {
    const rect = await drop.boundingBox();
    if (!rect) throw new Error('drop target missing');
    const aimY = target === 'ade-band-head' ? rect.y + rect.height / 2 : rect.y + rect.height - 2;
    await page.mouse.move(rect.x + 4, aimY, { steps: 12 });
    // The drop target is hit-tested per animation frame: let it catch up with the pointer.
    await page.evaluate(
      () => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))),
    );
    await expect(slot).toBeVisible({ timeout: 500 });
  }).toPass({ timeout: 10_000 });
  await page.mouse.up();
}

test('dragging a card to another day writes the plan once, with no dialog', async ({
  relaunch,
}) => {
  const board = adeFixture<BoardFx>('board');
  const { window: page, control } = await openPlan(relaunch);
  await page.setViewportSize({ width: 1440, height: 2400 });
  await dragCardToBand(page, 'T_deps', 6);

  const calls = () => control.log().filter((e) => e.channel === IPC.adeTaskSetPlan);
  await expect.poll(() => calls().length).toBe(1);
  expect(calls()[0]?.args).toEqual({
    order: [...board.plan.order.filter((id) => id !== 'T_deps'), 'T_deps'],
    days: { T_deps: '2026-09-28' },
  });
  await expect(page.locator('[data-testid="ade-confirm-dialog"]')).toHaveCount(0);
});

test('dragging a review card to another day pins it', async ({ relaunch }) => {
  const board = adeFixture<BoardFx>('board');
  const { window: page, control } = await openPlan(relaunch);
  await page.setViewportSize({ width: 1440, height: 3200 });
  await page.locator('[data-testid="ade-load-all"]').click();
  await dragCardToBand(page, 'R_sara', 6);

  const calls = () => control.log().filter((e) => e.channel === IPC.adeTaskSetPlan);
  await expect.poll(() => calls().length).toBe(1);
  expect(calls()[0]?.args).toEqual({
    order: [...board.plan.order.filter((id) => id !== 'R_sara'), 'R_sara'],
    days: { R_sara: '2026-09-28' },
  });
});

test('a review with a plan day sits on that day', async ({ relaunch }) => {
  const board = adeFixture<BoardFx>('board');
  board.plan.day.R_sara = '2026-09-28';
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator('[data-testid="ade-load-all"]').click();
  await expect(
    bandOf(page, 6).locator('[data-testid="ade-card"][data-task-id="R_sara"]'),
  ).toBeVisible();
});

test('the day-off confirm says what moves, offers Cancel and Escape leaves the plan alone', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await page.setViewportSize({ width: 1440, height: 2400 });
  // Today (Tue 22) has T_bill planned on it, so marking it a day off asks to move that task.
  await bandOf(page, 0).locator('[data-testid="ade-band-label"]').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-ade-day-toggle"]').click();
  const dialog = page.locator('[data-testid="ade-confirm-dialog"]');
  await expect(dialog).toContainText('Move tasks?');
  await expect(dialog.locator('[data-testid="ade-confirm-no"]')).toHaveText('Cancel');
  await expect(dialog.locator('[data-testid="ade-confirm-yes"]')).toHaveText('Move');
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  expect(control.log().filter((e) => e.channel === IPC.adeTaskSetPlan)).toHaveLength(0);
});

test('a card is a 6px rounded box with a task-colour left edge on the terminal ground, 16px from its same-day neighbour', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  const box = card(page, 'T_search');
  const style = await box.evaluate((el) => {
    const s = getComputedStyle(el);
    return {
      radius: s.borderTopLeftRadius,
      top: s.borderTopWidth,
      left: s.borderLeftWidth,
      leftColor: s.borderLeftColor,
      bg: s.backgroundColor,
    };
  });
  expect({ radius: style.radius, top: style.top, left: style.left }).toEqual({
    radius: '6px',
    top: '1px',
    left: '3px',
  });
  expect(style.leftColor).not.toBe('rgba(0, 0, 0, 0)');
  const [chrome, plan] = await Promise.all([
    page.evaluate(() => {
      const span = document.createElement('span');
      span.style.color = 'var(--kira-bg-chrome)';
      document.body.appendChild(span);
      const color = getComputedStyle(span).color;
      span.remove();
      return color;
    }),
    page.locator('[data-testid="ade-plan"]').evaluate((el) => getComputedStyle(el).backgroundColor),
  ]);
  expect(plan).toBe(chrome);
  expect(style.bg).not.toBe(plan);
  const head = await box.locator('[data-testid="ade-card-head"]').boundingBox();
  expect(head?.height).toBe(44);

  const band = bandOf(page, 3);
  await expect(band.locator('[data-testid="ade-task-separator"]')).toHaveCount(0);
  const rects = await band
    .locator('[data-testid="ade-card"]')
    .evaluateAll((els) =>
      els.map((e) => e.getBoundingClientRect()).map((r) => ({ top: r.top, bottom: r.bottom })),
    );
  expect(rects.length).toBeGreaterThanOrEqual(2);
  expect((rects[1]?.top ?? 0) - (rects[0]?.bottom ?? 0)).toBe(16);
});

test('the task open in the panel is marked on its card, spans and branch rows', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  const marked = page.locator('[data-testid="ade-card"][aria-current="true"]');
  const focusColor = await page.evaluate(() => {
    const span = document.createElement('span');
    span.style.color = 'var(--kira-focus)';
    document.body.appendChild(span);
    const color = getComputedStyle(span).color;
    span.remove();
    return color;
  });

  await card(page, 'T_search').locator('[data-testid="ade-card-head"]').click();
  await expect(card(page, 'T_search')).toHaveAttribute('aria-current', 'true');
  await expect(marked).toHaveCount(1);
  expect(await card(page, 'T_search').evaluate((el) => getComputedStyle(el).borderTopColor)).toBe(
    focusColor,
  );

  await card(page, 'T_push').locator('[data-testid="ade-card-head"]').click();
  await expect(card(page, 'T_push')).toHaveAttribute('aria-current', 'true');
  expect(await card(page, 'T_search').evaluate((el) => getComputedStyle(el).borderTopColor)).toBe(
    'rgba(0, 0, 0, 0)',
  );
  await expect(card(page, 'T_search')).not.toHaveAttribute('aria-current', 'true');

  const row = card(page, 'T_push').locator('[data-testid="ade-branch-row"]').first();
  await row.click();
  await expect(card(page, 'T_push')).toHaveAttribute('aria-current', 'true');
  await expect(row).toHaveAttribute('data-selected', 'true');

  const span = page.locator('[data-testid="ade-span-row"]').first();
  const taskId = await span.getAttribute('data-task-id');
  await span.click();
  await expect(card(page, taskId ?? '')).toHaveAttribute('aria-current', 'true');
  await expect(span).toHaveAttribute('data-selected', 'true');
});

test('pressing a card shows no ring, border or outline; keyboard focus still rings', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  const focusColor = await page.evaluate(() => {
    const span = document.createElement('span');
    span.style.color = 'var(--kira-focus)';
    document.body.appendChild(span);
    const color = getComputedStyle(span).color;
    span.remove();
    return color;
  });
  const task = page.locator('[data-testid="ade-task"]').first();
  const cardEl = task.locator('[data-testid="ade-card"]');
  const head = task.locator('[data-testid="ade-card-head"]');
  const state = () =>
    task.evaluate((el) => {
      const h = el.querySelector('[data-testid="ade-card-head"]');
      return {
        shadow: getComputedStyle(el).boxShadow,
        outline: getComputedStyle(el).outlineStyle,
        headOutline: h ? getComputedStyle(h).outlineStyle : '',
        headVisible: h?.matches(':focus-visible') ?? false,
        ring: [...el.classList].some((c) => c.startsWith('ring-')),
      };
    });
  const expectQuiet = async () =>
    expect(await state()).toEqual({
      shadow: 'none',
      outline: 'none',
      headOutline: 'none',
      headVisible: false,
      ring: false,
    });

  const targets = [
    head,
    task.locator('[data-testid="ade-card-title"]'),
    task.locator('[data-testid="ade-branch-row"]').first(),
  ];
  for (const target of targets) {
    await target.hover();
    await page.mouse.down();
    await expectQuiet();
    await page.mouse.up();
    await expectQuiet();
  }
  expect(await cardEl.evaluate((el) => getComputedStyle(el).borderTopColor)).toBe(focusColor);

  await page.keyboard.press('Tab');
  await head.focus();
  await expect(head).toBeFocused();
  const ring = await head.evaluate((el) => {
    const s = getComputedStyle(el);
    return { visible: el.matches(':focus-visible'), style: s.outlineStyle, color: s.outlineColor };
  });
  expect(ring).toEqual({ visible: true, style: 'solid', color: focusColor });
});

test('plan cards use colour only for task edge, status, problems and the repo badge', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await expect(
    page.locator(
      '[data-testid="ade-card"] [data-testid="ade-tag"]:not([data-urgent])[class*="-solid"]',
    ),
  ).toHaveCount(0);
  await expect(card(page, 'T_search').locator('[data-testid="ade-step-label"]')).not.toHaveClass(
    /tone-amber|tone-green/,
  );
  const repoBadges = page.locator('[data-testid="ade-branch-repo"]');
  await expect(repoBadges.first()).toHaveAttribute('style', /--repo: var\(--kira-conn-/);
  await expect(page.locator('[data-testid="ade-branch-repo"]:not([style*="--repo"])')).toHaveCount(
    0,
  );
});

test('the Plan scrolls inside the window instead of growing it', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.setViewportSize({ width: 1440, height: 900 });
  const m = await page
    .locator('[data-testid="ade-plan"]')
    .evaluate((el) => ({ client: el.clientHeight, scroll: el.scrollHeight, win: innerHeight }));
  expect(m.client).toBeLessThan(m.win);
  expect(m.scroll).toBeGreaterThan(m.client);
});

test('the Plan has no top bar; Refresh all sits in the sticky header beside History', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  const plan = page.locator('[data-testid="ade-plan"]');
  await expect(plan.locator('[data-slot="panel-header"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="ade-plan-header"]')).toHaveCount(0);
  const refresh = page.locator('[data-testid="ade-refresh-all"]');
  await expect(refresh).toBeVisible();
  const [history, button] = await Promise.all([
    page.locator('[data-testid="ade-load-history"]').boundingBox(),
    refresh.boundingBox(),
  ]);
  expect(Math.abs((history?.y ?? 0) - (button?.y ?? -99))).toBeLessThan(8);
  expect(button?.x ?? 0).toBeGreaterThan(history?.x ?? 0);
});

test('a branch row shows its own progress under the name', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const row = page.locator('[data-testid="ade-branch-row"][data-branch-id="b_meter"]');
  await expect(row.locator('[data-testid="ade-branch-prog"]')).toHaveText('Implement');
  await expect(
    page.locator(
      '[data-testid="ade-branch-row"][data-branch-id="b_cart"] [data-testid="ade-branch-prog"]',
    ),
  ).toHaveText('Release · failed');
});

test('line 2 shows merged and deployed chips with stale in amber and tooltips', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  const cart = page.locator('[data-testid="ade-branch-row"][data-branch-id="b_cart"]').first();
  await expect(cart.locator('[data-testid="ade-branch-merged"]')).toHaveText(['dev ✓']);
  await expect(cart.locator('[data-testid="ade-branch-deployed"]')).toHaveText([
    '▲staging ✓',
    '▲prod ✓',
  ]);
  const auth = page.locator('[data-testid="ade-branch-row"][data-branch-id="b_auth"]').first();
  const stale = auth.locator('[data-testid="ade-branch-merged"]');
  await expect(stale).toHaveText('dev ⚠');
  await expect(stale).toHaveCSS('color', 'rgb(219, 175, 113)');
  await auth.locator('[data-testid="ade-branch-line2"]').hover();
  await expect(
    page
      .locator('[data-testid="ade-branch-tip"]')
      .getByText('stale in develop: 2 commits since it was merged')
      .first(),
  ).toBeVisible();
});

test('branch tooltip shows plan colours', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page
    .locator(
      '[data-testid="ade-branch-row"][data-branch-id="b_search"] [data-testid="ade-branch-name"]',
    )
    .first()
    .hover();
  const tip = page.locator('[data-testid="ade-branch-tip"]');
  await expect(tip).toBeVisible();
  await expect(tip.locator('[data-testid="ade-chip"]').first()).toBeVisible();
  await expect(tip.locator('[data-var="base"]').first()).toBeVisible();
});

// Contract ade-board. Backend half: adeflow TestRefreshDefaultSettings (one repo with a remote that
// another clone pushed to, one without). Fixture repos repo-api and repo-mobile take the two states.
// The plan shows no fetch note; the UI half asserts what Refresh all sends and that no error shows.
function boardWithStates(phase: 'before' | 'after'): BoardFx {
  const board = adeFixture<BoardFx>('board');
  const state = (name: string, id: string) => ({
    ...contract<Record<string, unknown>>('ade-board', `AdeTaskService.Board#repo-${name}-${phase}`),
    codeRepoId: id,
    ...(phase === 'after' && name === 'tracked' ? { lastFetchAt: FIXED_NOW - 30_000 } : {}),
  });
  board.repos = board.repos.map((r) =>
    r.codeRepoId === 'repo-api'
      ? (state('tracked', 'repo-api') as typeof r)
      : r.codeRepoId === 'repo-mobile'
        ? (state('noremote', 'repo-mobile') as typeof r)
        : r,
  );
  return board;
}

test('contract: Refresh all sends both repos and a repo without a remote raises no error', async ({
  relaunch,
}) => {
  const refresh = (name: string, id: string) => ({
    ...contract<Record<string, unknown>>('ade-board', `AdeTaskService.Refresh#${name}`),
    codeRepoId: id,
  });
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: boardWithStates('after') },
    {
      channel: IPC.adeTaskRefresh,
      response: { repos: [refresh('tracked', 'repo-api'), refresh('noremote', 'repo-mobile')] },
    },
  ]);
  await page.locator('[data-testid="ade-refresh-all"]').click();
  await expect
    .poll(() => control.log().filter((e) => e.channel === IPC.adeTaskRefresh))
    .toHaveLength(1);
  expect(control.log().find((e) => e.channel === IPC.adeTaskRefresh)?.args).toEqual({
    codeRepoIds: boardWithStates('after').repos.map((r) => r.codeRepoId),
  });
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0);
});

test('today has no band tint; its heading rule is a subtle grey that differs from other days', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  const today = page.locator('[data-testid="ade-day-band"][data-today="true"]');
  await expect(today).toHaveCount(1);
  const ruleColor = (band: import('@playwright/test').Locator) =>
    band
      .locator('[data-testid="ade-band-head"] > span[aria-hidden="true"]')
      .evaluate((el) => getComputedStyle(el).backgroundColor);
  const other = page.locator('[data-testid="ade-day-band"]:not([data-today])').first();
  const todayRule = await ruleColor(today);
  const otherRule = await ruleColor(other);
  expect(todayRule).not.toBe(otherRule);
  const bg = (band: import('@playwright/test').Locator) =>
    band.evaluate((el) => getComputedStyle(el).backgroundColor);
  expect(await bg(today)).toBe('rgba(0, 0, 0, 0)');
  expect(await bg(other)).toBe('rgba(0, 0, 0, 0)');
  expect(
    await today
      .locator('[data-testid="ade-band-label"]')
      .evaluate((el) => getComputedStyle(el).backgroundColor),
  ).toBe('rgba(0, 0, 0, 0)');
  // color-mix computes to `color(srgb r g b)` in 0..1, plain colours to `rgb(r, g, b)` in 0..255.
  const rgb = (c: string) => {
    const n = (c.match(/[\d.]+/g) ?? []).slice(0, 3).map(Number);
    return c.startsWith('color(') ? n.map((v) => v * 255) : n;
  };
  const [r = 0, g = 0, b = 0] = rgb(todayRule);
  expect(Math.max(r, g, b) - Math.min(r, g, b)).toBeLessThanOrEqual(12);
  const lum = (c: string) => {
    const [x = 0, y = 0, z = 0] = rgb(c);
    return 0.2126 * x + 0.7152 * y + 0.0722 * z;
  };
  expect(lum(todayRule)).toBeGreaterThan(lum(otherRule));
});

test("each day has a left bar from its heading to its last row; today's is wider and lighter", async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await page.setViewportSize({ width: 1440, height: 2400 });
  const bands = page.locator('[data-testid="ade-day-band"]');
  const count = await bands.count();
  expect(count).toBeGreaterThan(2);
  const info = await bands.evaluateAll((els) =>
    els.map((el) => {
      const b = el.getBoundingClientRect();
      const bars = el.querySelectorAll(':scope > [data-testid="ade-day-bar"]');
      const bar = bars[0]?.getBoundingClientRect();
      const cs = bars[0] ? getComputedStyle(bars[0]) : null;
      const rows = [
        ...el.querySelectorAll('[data-testid="ade-card"], [data-testid="ade-span-row"]'),
      ];
      const rule = el.querySelector('[data-testid="ade-band-head"] > span[aria-hidden="true"]');
      return {
        key: (el as HTMLElement).dataset.adeDay,
        today: (el as HTMLElement).dataset.today === 'true',
        band: { x: b.x, y: b.y, bottom: b.bottom },
        bars: bars.length,
        bar: bar ? { x: bar.x, y: bar.y, w: bar.width, bottom: bar.bottom } : null,
        radius: cs ? Number.parseFloat(cs.borderTopLeftRadius) : 0,
        width: cs?.width,
        color: cs?.backgroundColor,
        rule: rule ? getComputedStyle(rule).backgroundColor : '',
        headMid: (() => {
          const h = el.querySelector('[data-testid="ade-band-head"]')?.getBoundingClientRect();
          return h ? h.y + h.height / 2 : 0;
        })(),
        lastRow: rows.length ? Math.max(...rows.map((r) => r.getBoundingClientRect().bottom)) : 0,
        bg: getComputedStyle(el).backgroundColor,
      };
    }),
  );
  const days = info.filter((i) => i.key !== '9999');
  for (const i of info.filter((x) => x.key === '9999')) expect(i.bars).toBe(0);
  for (const [n, d] of days.entries()) {
    expect(d.bars).toBe(1);
    const bar = d.bar;
    if (!bar) throw new Error('bar missing');
    expect(bar.x + bar.w).toBeLessThanOrEqual(d.band.x);
    expect(bar.x).toBeGreaterThanOrEqual(d.band.x - 16);
    expect(Math.abs(bar.y - (d.band.y + 4))).toBeLessThanOrEqual(1);
    expect(Math.abs(bar.bottom - (d.band.bottom - 4))).toBeLessThanOrEqual(1);
    expect(d.headMid).toBeGreaterThan(bar.y);
    if (d.lastRow) expect(d.lastRow).toBeLessThanOrEqual(bar.bottom);
    expect(d.radius).toBeGreaterThanOrEqual(bar.w / 2);
    const next = days[n + 1]?.bar;
    if (next) expect(bar.bottom).toBeLessThan(next.y);
  }
  const today = days.find((d) => d.today);
  const other = days.find((d) => !d.today);
  if (!today || !other) throw new Error('bands missing');
  expect(today.width).toBe('3px');
  expect(other.width).toBe('2px');
  expect(today.color).not.toBe(other.color);
  expect(today.color).toBe(today.rule);
  const rgb = (c: string) => {
    const n = (c.match(/[\d.]+/g) ?? []).slice(0, 3).map(Number);
    return c.startsWith('color(') ? n.map((v) => v * 255) : n;
  };
  const lum = (c: string) => {
    const [x = 0, y = 0, z = 0] = rgb(c);
    return 0.2126 * x + 0.7152 * y + 0.0722 * z;
  };
  expect(lum(today.color ?? '')).toBeGreaterThan(lum(other.color ?? ''));
  for (const d of info) expect(d.bg).toBe('rgba(0, 0, 0, 0)');
});

test('the timeline has no date gutter and each date shows once', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const gutters = await page.locator('[data-testid="ade-plan"] *').evaluateAll(
    (els) =>
      els.filter((e) => {
        const cs = getComputedStyle(e);
        return cs.width === '60px' && cs.borderRightWidth === '2px';
      }).length,
  );
  expect(gutters).toBe(0);
  const band = bandOf(page, 0);
  const inset = await band.evaluate((el) => {
    const c = el.querySelector('[data-testid="ade-card"]');
    return c ? c.getBoundingClientRect().x - el.getBoundingClientRect().x : null;
  });
  expect(inset).not.toBeNull();
  expect(inset ?? 99).toBeLessThanOrEqual(8);
  const bands = page.locator('[data-testid="ade-day-band"]');
  const count = await bands.count();
  expect(count).toBeGreaterThan(1);
  for (let i = 0; i < count; i++)
    await expect(bands.nth(i).locator('[data-testid="ade-band-label"]')).toHaveCount(1);
  const labels = await page.locator('[data-testid="ade-band-label"]').allInnerTexts();
  expect(labels).toHaveLength(count);
  expect(new Set(labels).size).toBe(labels.length);
});

test("dropping on an empty day's heading moves the task there", async ({ relaunch }) => {
  const board = adeFixture<BoardFx>('board');
  const { window: page, control } = await openPlan(relaunch);
  await page.setViewportSize({ width: 1440, height: 2400 });
  await expect(bandOf(page, 4).locator('[data-testid="ade-card"]')).toHaveCount(0);
  await dragCardToBand(page, 'T_deps', 4, 'ade-band-head');

  const calls = () => control.log().filter((e) => e.channel === IPC.adeTaskSetPlan);
  await expect.poll(() => calls().length).toBe(1);
  expect(calls()[0]?.args).toEqual({
    order: [...board.plan.order.filter((id) => id !== 'T_deps'), 'T_deps'],
    days: { T_deps: '2026-09-26' },
  });
});
