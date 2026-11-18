import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';

// P267 Stream B: touch sizes, shared loading and chip primitives on the phone.

const t = (id: string) => `[data-testid="${id}"]`;
const MIN_TOUCH = 44;

async function expectTouch(page: Page, ids: readonly string[]): Promise<void> {
  for (const id of ids) {
    const box = await page.locator(t(id)).boundingBox();
    expect(box?.height ?? 0, id).toBeGreaterThanOrEqual(MIN_TOUCH);
  }
}

test.describe('paired phone', () => {
  test.beforeEach(async ({ app, server }) => {
    server.state.auth = 'ok';
    server.state.permissions = { write: true, agentInput: true };
    server.state.agentInputGlobal = true;
    await app();
  });

  test('plan step buttons and the later add row are touch sized', async ({ page }) => {
    await page.locator(t('tab-plan')).click();
    const card = page
      .locator(t('plan-task'))
      .filter({ hasText: 'Cart total test fails intermittently' });
    await card.locator('button').first().click();
    await expectTouch(page, ['plan-step-back', 'plan-step-next']);
    await page.locator(t('tab-later')).click();
    await expectTouch(page, ['later-add', 'later-add-submit']);
  });

  test('the key bar and compose row are touch sized; Ctrl exposes its pressed state', async ({
    page,
    server,
  }) => {
    await page.routeWebSocket(/\/api\/agent\/sessions\/[^/]+\/terminal/, (ws) => {
      ws.send(JSON.stringify({ type: 'hello', offset: 0, cols: 80, rows: 24 }));
    });
    await page.goto(`${server.url}terminal/9ab0`);
    await expect(page.locator(t('term-keybar'))).toBeVisible();
    await expectTouch(page, ['term-key-esc', 'term-key-ctrl', 'term-compose', 'term-compose-send']);
    const ctrl = page.locator(t('term-key-ctrl'));
    await expect(ctrl).toHaveAttribute('aria-pressed', 'false');
    await ctrl.click();
    await expect(ctrl).toHaveAttribute('aria-pressed', 'true');
  });

  test('Needs badges and Plan chips render the shared pieces', async ({ page }) => {
    await expect(page.locator(`${t('needs-item')} [data-slot="badge"]`).first()).toBeVisible();
    await page.locator(t('tab-plan')).click();
    await expect(page.locator(`${t('plan-task')} ${t('ade-chip')}`).first()).toBeVisible();
  });

  test('plan progress renders the shared Progress with a percent label', async ({ page }) => {
    await page.locator(t('tab-plan')).click();
    const bar = page.locator(`${t('plan-task')} [data-slot="progress"]`).first();
    await expect(bar).toBeVisible();
    await expect(bar.locator('xpath=..')).toContainText('%');
  });
});

for (const [tab, route, screen] of [
  ['tab-needs', '**/api/ade/board', 'needs-screen'],
  ['tab-plan', '**/api/ade/board', 'plan-screen'],
  ['tab-later', '**/api/ade/board', 'later-screen'],
] as const) {
  test(`${screen} loading shows the spinner and no plain text line`, async ({
    page,
    server,
    app,
  }) => {
    server.state.auth = 'ok';
    let release: () => void = () => {};
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(route, async (r) => {
      await gate;
      await r.continue();
    });
    await app();
    if (tab !== 'tab-needs') await page.locator(t(tab)).click();
    await expect(page.locator(`${t(screen)} .animate-spin`)).toBeVisible();
    await expect(page.locator(`${t(screen)} p`)).toHaveCount(0);
    release();
  });
}
