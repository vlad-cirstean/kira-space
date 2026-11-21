import type { Page } from '@playwright/test';
import { defaultLayout } from '@shared/domain/layout';
import { expect, test } from './fixtures';
import { adeBoard, openPlan } from './support/adeV2';
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

test('the lead slot holds the agent indicator first and the task colour sits beside the title', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  for (const id of ['T_bill', 'T_hooks']) {
    expect(await firstChildTestId(page, head(id))).toBe('ade-row-lead');
    await expect(
      page.locator(`${head(id)} ${t('ade-row-lead')} ${t('ade-card-sessions')}`),
    ).toHaveCount(1);
  }
  const box = await page.locator(`${head('T_bill')} ${t('ade-card-sessions')}`).boundingBox();
  expect(Math.round(box?.height ?? 0)).toBe(24);
  await expect(page.locator(t('ade-card-dot'))).toHaveCount(0);
  await expect(page.locator(`${t('ade-row-lead')} ${t('ade-task-swatch')}`)).toHaveCount(0);
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
  await expect(row.locator(t('ade-task-swatch'))).toHaveCount(1);
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

const heads = (page: Page) => page.locator(`${t('ade-card')} ${t('ade-card-head')}`);

test('a task card wears its colour as a swatch beside the title, on the plan ground', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(t('ade-load-all')).click();
  const planBg = await page
    .locator(t('ade-plan'))
    .evaluate((el) => getComputedStyle(el).backgroundColor);
  const cards = await page.locator(t('ade-card')).evaluateAll((els) =>
    els.map((e) => {
      const cs = getComputedStyle(e);
      const head = e.querySelector('[data-testid="ade-card-head"]');
      const swatch = head?.querySelector('[data-testid="ade-task-swatch"]') as HTMLElement | null;
      return {
        left: cs.borderLeftWidth,
        top: cs.borderTopWidth,
        inlineEdge: (e as HTMLElement).style.borderLeftColor,
        bg: cs.backgroundColor,
        hasHead: head !== null,
        swatches: head?.querySelectorAll('[data-testid="ade-task-swatch"]').length ?? 0,
        painted: swatch ? swatch.style.background !== '' || swatch.style.borderColor !== '' : false,
        inLead:
          head?.querySelector('[data-testid="ade-row-lead"] [data-testid="ade-task-swatch"]') !==
          null,
      };
    }),
  );
  expect(cards.length).toBeGreaterThan(0);
  for (const c of cards) {
    expect(c.left).toBe('1px');
    expect(c.top).toBe('1px');
    expect(c.inlineEdge).toBe('');
    expect(c.bg).toBe(planBg);
    if (!c.hasHead) continue;
    expect(c.swatches).toBe(1);
    expect(c.painted).toBe(true);
    expect(c.inLead).toBe(false);
  }
});

test('card head buttons stay inside the card with the panel open', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.locator(head('T_search')).click();
  await expect(page.locator(card('T_search'))).toHaveAttribute('aria-current', 'true');
  const overflow = await heads(page).evaluateAll((els) =>
    els.flatMap((h) => {
      const cardEl = h.closest('[data-testid="ade-card"]');
      const controls = [
        ...h.querySelectorAll('[data-testid^="ade-task-action-"], [data-testid="ade-card-review"]'),
      ];
      const last = controls.at(-1);
      if (!cardEl || !last) return [];
      const right = cardEl.getBoundingClientRect().right;
      return last.getBoundingClientRect().right > right + 0.5
        ? [cardEl.getAttribute('data-task-id')]
        : [];
    }),
  );
  expect(overflow).toEqual([]);
  expect(await heads(page).count()).toBeGreaterThan(0);
});

test('card head fits at a narrow plan pane', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.setViewportSize({ width: 960, height: 900 });
  await page.locator(head('T_search')).click();
  await expect(page.locator(card('T_search'))).toHaveAttribute('aria-current', 'true');
  const bad = await heads(page).evaluateAll((els) =>
    els.flatMap((h) => {
      const controls = [
        ...h.querySelectorAll('[data-testid^="ade-task-action-"], [data-testid="ade-card-review"]'),
      ];
      const last = controls.at(-1);
      // Gap from the last control to the head's right edge; the 8px padding makes 8 the flush state.
      const gap = last
        ? h.getBoundingClientRect().right - last.getBoundingClientRect().right
        : null;
      const clipped = h.scrollWidth > h.clientWidth;
      // 1px of sub-pixel slack: the control may touch the padding edge, not enter its last 7px.
      const tight = gap !== null && gap < 8 - 1;
      return clipped || tight
        ? [
            {
              id: h.closest('[data-testid="ade-card"]')?.getAttribute('data-task-id'),
              scrollWidth: h.scrollWidth,
              clientWidth: h.clientWidth,
              gap,
            },
          ]
        : [];
    }),
  );
  expect(bad).toEqual([]);
});

test('branch rows start with the depth indent, no lead cell', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator(t('ade-branch-lead'))).toHaveCount(0);
  const row = page.locator(`${t('ade-branch-row')}[data-depth="0"]`).first();
  const start = await row.evaluate((el) => {
    const body = el.children[1] as HTMLElement | undefined;
    return body ? body.getBoundingClientRect().x - el.getBoundingClientRect().x : -1;
  });
  expect(start).toBeGreaterThanOrEqual(0);
  expect(start).toBeLessThan(16);
});

// Layout only, no backend half. Line 2 of a board branch row (progress label plus target chips)
// never clips: the label gives way, the chips keep their width.
for (const [name, width] of [
  ['the default 400px list', 1280],
  ['the 360px floor', 760],
] as const) {
  test(`branch row line 2 fits at ${name}`, async ({ relaunch }) => {
    const { window: page } = await openPlan(relaunch);
    await page.setViewportSize({ width, height: 900 });
    await page.locator(head('T_cart')).click();
    await expect(page.locator(card('T_cart'))).toHaveAttribute('aria-current', 'true');
    const plan = await page.locator(t('ade-plan')).boundingBox();
    expect(plan?.width ?? 0).toBeGreaterThanOrEqual(360);
    expect(plan?.width ?? 0).toBeLessThanOrEqual(400);
    const clipped = await page
      .locator(t('ade-branch-line2'))
      .evaluateAll((els) => els.filter((e) => e.scrollWidth > e.clientWidth).length);
    expect(clipped).toBe(0);
    const cart = page.locator(`${t('ade-branch-row')}[data-branch-id="b_cart"]`);
    await expect(cart.locator(t('ade-branch-merged'))).toHaveText(['dev ✓']);
    await expect(cart.locator(t('ade-branch-deployed'))).toHaveText(['▲staging ✓', '▲prod ✓']);
    await cart.locator(t('ade-branch-line2')).hover();
    await expect(
      page.locator(t('ade-branch-tip')).getByText('Release · failed').first(),
    ).toBeVisible();
  });
}

// Layout only, no backend half. Card heads and board branch rows share fixed column tracks: the
// text flexes and truncates, every other column keeps its x, and hover never moves a box.
const LONG_TITLE =
  'Cart total test fails intermittently when the promo code and the gift card are applied in the same checkout session on Safari';
const gridBoard = adeBoard((b) => {
  Object.assign(b.tasks.find((x) => x.id === 'T_cart') ?? {}, { title: LONG_TITLE });
  Object.assign(b.tasks.find((x) => x.id === 'T_notif') ?? {}, { title: 'Fix' });
});

for (const [name, width, height, open] of [
  ['the widened list', 1440, 1000, ''],
  ['the open-task list', 1280, 900, 'T_cart'],
] as const) {
  test(`plan rows keep fixed columns and hover never reflows (${name})`, async ({ relaunch }) => {
    // Panel always open: a 340px panel leaves the list past the wide-grid breakpoint at 1440.
    const widened = { ...defaultLayout, panel: { ...defaultLayout.panel, ade: { width: 340 } } };
    const { window: page } = await openPlan(relaunch, [
      { channel: IPC.adeTaskBoard, response: gridBoard },
      ...(open ? [] : [{ channel: IPC.layoutGetAll, response: widened }]),
    ]);
    await page.setViewportSize({ width, height });
    await page.locator(t('ade-load-all')).click();
    if (open) await page.locator(head(open)).click();

    const geo = await page.evaluate(() => {
      const r = (el: Element | null | undefined) => {
        const b = el?.getBoundingClientRect();
        return b ? { x: b.x, y: b.y, w: b.width, h: b.height } : null;
      };
      const heads = [...document.querySelectorAll('[data-testid="ade-card-head"]')];
      const rows = heads.flatMap((h) => {
        const list = [
          ...(h
            .closest('[data-testid="ade-card"]')
            ?.querySelectorAll('[data-testid="ade-branch-row"]') ?? []),
        ];
        return list.length ? [list[0]] : [];
      });
      const cell = (el: Element, i: number) => el.children[1]?.children[i];
      return {
        swatch: heads.map((h) => r(h.querySelector('[data-testid="ade-task-swatch"]'))?.x),
        title: heads.map((h) => r(h.querySelector('[data-testid="ade-card-title"]'))?.x),
        swatchGap: (() => {
          const h = heads[0];
          const sw = r(h?.querySelector('[data-testid="ade-task-swatch"]'));
          const ti = r(h?.querySelector('[data-testid="ade-card-title"]'));
          return sw && ti ? ti.x - (sw.x + sw.w) : -1;
        })(),
        spanTitle: [...document.querySelectorAll('[data-testid="ade-span-row"]')].map(
          (el) => r(el.querySelector('span.truncate'))?.x,
        ),
        status: heads.map((h) => r(h.querySelector('[data-testid="ade-row-status"]'))?.x),
        meta: heads.map((h) => r(h.querySelector('[data-testid="ade-card-meta"]'))?.x),
        headH: heads.map((h) => r(h)?.h),
        action: heads.map((h) => {
          const line1 = h.children[1]?.children[0];
          const b = r(line1?.lastElementChild);
          return b ? b.x + b.w / 2 : null;
        }),
        name: rows.map((el) => r(el.querySelector('[data-testid="ade-branch-name"]'))?.x),
        tag: rows.map((el) => {
          const b = r(cell(el, 4));
          return b ? b.x + b.w / 2 : null;
        }),
        rowH: rows.map((el) => r(el)?.h),
        overflow: [
          ...document.querySelectorAll(
            '[data-testid="ade-card-title"], [data-testid="ade-branch-name"]',
          ),
        ]
          .filter((el) => el.scrollWidth > el.clientWidth + 1)
          .filter((el) => getComputedStyle(el).textOverflow !== 'ellipsis').length,
      };
    });
    const same = (xs: (number | null | undefined)[]) => {
      const v = xs.filter((x): x is number => typeof x === 'number');
      expect(v.length).toBeGreaterThan(3);
      expect(Math.max(...v) - Math.min(...v)).toBeLessThanOrEqual(1);
      return v[0];
    };
    same(geo.swatch);
    expect(geo.swatchGap).toBeLessThanOrEqual(7);
    expect(geo.swatchGap).toBeGreaterThanOrEqual(5);
    expect(geo.spanTitle.length).toBeGreaterThan(0);
    expect(
      Math.max(...geo.spanTitle.map(Number)) - Math.min(...geo.spanTitle.map(Number)),
    ).toBeLessThanOrEqual(1);
    expect(Math.abs((geo.spanTitle[0] ?? 0) - (geo.title[0] ?? 0))).toBeLessThanOrEqual(1);
    same(geo.status);
    same(geo.meta);
    same(geo.name);
    const tagX = same(geo.tag);
    expect(Math.abs((tagX ?? 0) - (same(geo.action) ?? 0))).toBeLessThanOrEqual(1);
    expect(new Set(geo.headH)).toEqual(new Set([44]));
    for (const h of geo.rowH) expect([28, 44]).toContain(Math.round(h ?? 0));
    expect(geo.overflow).toBe(0);

    const boxes = async () =>
      page.evaluate(() => {
        const q = (sel: string) => {
          const b = document.querySelector(sel)?.getBoundingClientRect();
          return b ? [b.x, b.width, b.height].map(Math.round) : null;
        };
        const c = '[data-testid="ade-card"][data-task-id="T_auth"]';
        const row = '[data-testid="ade-branch-row"][data-branch-id="b_auth"]';
        return [
          q(`${c} [data-testid="ade-card-title"]`),
          q(`${c} [data-testid="ade-row-status"]`),
          q(`${row} [data-testid="ade-branch-name"]`),
        ];
      });
    const before = await boxes();
    await page.locator(head('T_auth')).hover();
    expect(await boxes()).toEqual(before);
    await page.locator(`${t('ade-branch-row')}[data-branch-id="b_auth"]`).hover();
    expect(await boxes()).toEqual(before);
  });
}
