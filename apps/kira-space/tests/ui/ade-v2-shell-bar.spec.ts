import { expect, test } from './fixtures';
import { openPlan } from './support/adeV2';

// The shell top bar keeps every control reachable: it wraps on narrow windows, one line when wide.

const bar = '[data-testid="ade-shell-bar"]';
const controls = `${bar} [data-testid^="ade-tab-"], ${bar} [data-testid="ade-capture"], ${bar} [data-testid="ade-add"]`;

test('at 900px every top bar control is visible and inside the window', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.setViewportSize({ width: 900, height: 800 });
  const all = page.locator(controls);
  expect(await all.count()).toBeGreaterThanOrEqual(8);
  for (let i = 0; i < (await all.count()); i++) await expect(all.nth(i)).toBeVisible();
  const boxes = await all.evaluateAll((els) =>
    els.map((e) => {
      const r = e.getBoundingClientRect();
      return { left: r.left, right: r.right, top: r.top, bottom: r.bottom };
    }),
  );
  const view = await page.evaluate(() => ({ w: innerWidth, h: innerHeight }));
  for (const b of boxes) {
    expect(b.left).toBeGreaterThanOrEqual(0);
    expect(b.right).toBeLessThanOrEqual(view.w);
    expect(b.top).toBeGreaterThanOrEqual(0);
    expect(b.bottom).toBeLessThanOrEqual(view.h);
  }
  const fits = await page.locator(bar).evaluate((el) => el.scrollWidth <= el.clientWidth);
  expect(fits).toBe(true);
});

test('at 1440px the top bar is one grid line', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.setViewportSize({ width: 1440, height: 900 });
  const nav = page.locator(bar);
  await expect(nav).toHaveCSS('display', 'grid');
  const { height, tabbar } = await nav.evaluate((el) => {
    const probe = document.createElement('div');
    probe.style.height = 'var(--kira-tabbar-h)';
    document.body.appendChild(probe);
    const h = probe.getBoundingClientRect().height;
    probe.remove();
    return { height: el.getBoundingClientRect().height, tabbar: h };
  });
  expect(height).toBe(tabbar);
});
