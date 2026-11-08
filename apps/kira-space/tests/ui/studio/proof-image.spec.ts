import type { Page } from '@playwright/test';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { imageControl, openProofWindow } from './support/proof';

// UI half of the proof-image scenario; the Go half is proofflow TestProofImage. Every answer is
// that test's recorded result, so bytes the backend stores are what the window draws here.

const TITLES = [
  '#1 Login form, from a file',
  '#2 Login form, inline',
  '#3 Login form, through a symlink',
  '#4 Long login page',
];
const tab = (page: Page, n: number) =>
  page.locator('[data-testid="tab"]', { hasText: TITLES[n - 1] });
const region = (page: Page) => page.locator('[data-testid="proof-image"]');
const stepReady = (page: Page) =>
  expect(page.locator('[data-testid="proof-step"][data-ready="true"]').first()).toBeVisible({
    timeout: 20_000,
  });

// biome-ignore lint/suspicious/noExplicitAny: logged bridge args
type Json = Record<string, any>;

test('contract: an image step draws the stored JPEG', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({ control: imageControl() });
  await openProofWindow(page, uiServer.url);
  await stepReady(page);
  for (const n of [1, 2, 3]) {
    await tab(page, n).click();
    await expect(region(page)).toBeVisible();
    const img = region(page).locator('img');
    await expect(img).toHaveAttribute('alt', TITLES[n - 1]?.replace(/^#\d+ /, '') ?? '');
    await expect
      .poll(() => img.evaluate((el: HTMLImageElement) => [el.naturalWidth, el.naturalHeight]))
      .toEqual([64, 48]);
    await stepReady(page);
  }
});

test('contract: an image step keeps its scroll', async ({ relaunch, uiServer }) => {
  const { window: page, control: log } = await relaunch({ control: imageControl() });
  await openProofWindow(page, uiServer.url);
  await stepReady(page);
  await tab(page, 4).click();
  await expect
    .poll(() =>
      region(page)
        .locator('img')
        .evaluate((el: HTMLImageElement) => el.naturalHeight),
    )
    .toBe(2000);
  await stepReady(page);
  await region(page).evaluate((el) => {
    el.scrollTop = 700;
  });
  await expect
    .poll(
      () =>
        log
          .log()
          .filter((c) => c.channel === IPC.proofSetViewState)
          .map((c) => c.args as Json)
          .at(-1)?.state.resultScrollTop,
    )
    .toBeGreaterThan(600);

  // Another tab and back: the cache carries the position.
  await tab(page, 1).click();
  await expect(region(page)).toBeVisible();
  await tab(page, 4).click();
  await expect.poll(() => region(page).evaluate((el) => el.scrollTop)).toBeGreaterThan(600);
});
