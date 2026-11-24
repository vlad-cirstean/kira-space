import type { Locator, Page } from '@playwright/test';
import { expect, studioTest as test } from '../fixtures';
import { openProofWindow, proofControl, threeSteps } from './support/proof';

// P748: the proof window grids draw cell and row lines in --kira-grid-line. The backend half is the
// existing proof flow tests: nothing about the payload changes, only how it renders.

const result = (page: Page) => page.locator('[data-testid="proof-step-result"]');
const openTab = (page: Page, title: string) =>
  page.locator('[data-testid="tab"]', { hasText: title }).click();

// The token resolved to a computed colour, via a probe element.
const gridLine = (page: Page) =>
  page.evaluate(() => {
    const probe = document.createElement('span');
    probe.style.color = 'var(--kira-grid-line)';
    document.body.append(probe);
    const colour = getComputedStyle(probe).color;
    probe.remove();
    return colour;
  });

const rowLine = (row: Locator) =>
  row.evaluate((el) => {
    const after = getComputedStyle(el, '::after');
    return { background: after.backgroundColor, height: after.height };
  });

async function open(page: Page, baseUrl: string, tab: string): Promise<void> {
  await openProofWindow(page, baseUrl);
  await openTab(page, tab);
  await expect(result(page)).toBeVisible();
}

test('table cells draw grid lines', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({ control: proofControl(threeSteps()) });
  await open(page, uiServer.url, '#1 Orders before');
  const line = await gridLine(page);
  await expect(result(page).locator('.slick-grid-host')).toHaveClass(/kira-grid--lined/);
  const cell = result(page).locator('[data-testid="console-result-cell"]').first();
  await expect(cell).toBeVisible();
  const borders = await cell.evaluate((el) => {
    const s = getComputedStyle(el);
    return { bottom: s.borderBottomColor, right: s.borderRightColor };
  });
  expect(borders).toEqual({ bottom: line, right: line });
  const header = result(page).locator('[data-testid="console-result-header-cell"]').first();
  await expect(header).toHaveCSS('border-right-color', line);
});

test('key-value rows draw lines', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({ control: proofControl(threeSteps()) });
  await open(page, uiServer.url, '#3 Stats');
  const line = await gridLine(page);
  const rows = result(page).locator('[data-testid="console-result-kv-row"]');
  await expect(rows.first()).toBeVisible();
  for (const row of await rows.all()) {
    expect(await rowLine(row)).toEqual({ background: line, height: '1px' });
  }
  await expect(rows.first().locator('.cell').first()).toHaveCSS('border-right-color', line);
});

test('document rows draw lines', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({ control: proofControl(threeSteps()) });
  await open(page, uiServer.url, '#2 Doc hit');
  const line = await gridLine(page);
  const rows = result(page).locator('[data-testid="console-result-doc-row"]');
  await expect(rows.first()).toBeVisible();
  for (const row of await rows.all()) {
    expect(await rowLine(row)).toEqual({ background: line, height: '1px' });
  }
});
