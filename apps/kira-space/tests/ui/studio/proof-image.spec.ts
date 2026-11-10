import type { Page } from '@playwright/test';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import {
  imageControl,
  jpegHeight,
  menuTarget,
  openProofWindow,
  SESSION_ID,
  shotFolder,
} from './support/proof';

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

test('contract: saving writes a JPEG of the image step', async ({ relaunch, uiServer }) => {
  test.setTimeout(180_000);
  const { window: page, control } = await relaunch({
    control: imageControl([{ channel: IPC.proofOpenShotFolder, response: shotFolder() }]),
  });
  await openProofWindow(page, uiServer.url);
  await stepReady(page);
  await expect
    .poll(() =>
      region(page)
        .locator('img')
        .evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth),
    )
    .toBe(64);

  await page.locator('[data-testid="proof-shot"]').click();
  await expect
    .poll(() => control.log().filter((c) => c.channel === IPC.proofSaveJpegTo).length, {
      timeout: 60_000,
    })
    .toBeGreaterThanOrEqual(1);
  const args = control.log().find((c) => c.channel === IPC.proofSaveJpegTo)?.args as Json;
  expect(args.filename).toMatch(/^01-Login-form/);
  expect(args.jpegBase64.startsWith('/9j/')).toBe(true);
  expect(jpegHeight(args.jpegBase64)).toBeGreaterThan(0);
});

test('contract: a window opened from the menu shows the newest session', async ({
  relaunch,
  uiServer,
}) => {
  const older = {
    id: 'session-9',
    name: 'Older proof',
    closedAt: 1,
    steps: 1,
    pending: false,
    seen: true,
  };
  const { window: page, control: log } = await relaunch({
    control: imageControl([], menuTarget(), [older]),
  });
  await openProofWindow(page, uiServer.url);
  await stepReady(page);
  expect(menuTarget()).toEqual({ sessionId: null });
  await expect(tab(page, 1)).toBeVisible();
  await expect(region(page)).toBeVisible();
  expect(
    log
      .log()
      .filter((c) => c.channel === IPC.proofSession)
      .map((c) => (c.args as Json).sessionId),
  ).toEqual([SESSION_ID]);
});
