import type { Page } from '@playwright/test';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { imageControl, jpegHeight, openProofWindow, shotFolder } from './support/proof';

// UI half of the proof-image scenario; the Go half is proofflow TestProofImage. Every answer is
// that test's recorded result, so bytes the backend stores are what the window draws here.

const TITLES = [
  '#1 Login form, JPEG',
  '#2 Login form, PNG',
  '#3 Login logo, SVG',
  '#4 Long login page',
];
// Format, intrinsic size and data URL type of steps 1-3 (proofflow TestProofImage).
const FORMATS: { mime: string; size: [number, number] }[] = [
  { mime: 'image/jpeg', size: [64, 48] },
  { mime: 'image/png', size: [8, 8] },
  { mime: 'image/svg+xml', size: [40, 30] },
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

test('contract: an image step draws each stored format', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({ control: imageControl() });
  await openProofWindow(page, uiServer.url);
  await stepReady(page);
  for (const n of [1, 2, 3]) {
    await tab(page, n).click();
    await expect(region(page)).toBeVisible();
    const img = region(page).locator('img');
    await expect(img).toHaveAttribute('alt', TITLES[n - 1]?.replace(/^#\d+ /, '') ?? '');
    const want = FORMATS[n - 1] as (typeof FORMATS)[number];
    expect(await img.getAttribute('src')).toContain(`data:${want.mime};base64,`);
    await expect
      .poll(() => img.evaluate((el: HTMLImageElement) => [el.naturalWidth, el.naturalHeight]))
      .toEqual(want.size);
    await stepReady(page);
  }
});

test('contract: an image step fits the window', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({ control: imageControl() });
  await openProofWindow(page, uiServer.url);
  await stepReady(page);
  await tab(page, 4).click();
  const img = region(page).locator('img');
  await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.naturalHeight)).toBe(2000);
  await stepReady(page);

  // Whole image inside the region, no scroll, height filled, aspect kept.
  const expectFits = async () => {
    await expect
      .poll(() =>
        region(page).evaluate((el) => [
          el.scrollHeight - el.clientHeight,
          el.scrollWidth - el.clientWidth,
        ]),
      )
      .toEqual([0, 0]);
    await expect
      .poll(async () => {
        const r = await region(page).boundingBox();
        const i = await img.boundingBox();
        if (!r || !i) return false;
        const inside =
          i.x >= r.x - 1 &&
          i.y >= r.y - 1 &&
          i.x + i.width <= r.x + r.width + 1 &&
          i.y + i.height <= r.y + r.height + 1;
        const fillsHeight = Math.abs(i.height - r.height) <= 1;
        const aspect = Math.abs(i.width - (64 * i.height) / 2000) <= 1;
        return inside && fillsHeight && aspect;
      })
      .toBe(true);
  };
  await expectFits();

  // Follows the window.
  const before = (await img.boundingBox())?.height ?? 0;
  await page.setViewportSize({ width: 900, height: 520 });
  await expect.poll(async () => (await img.boundingBox())?.height ?? before).toBeLessThan(before);
  await expectFits();

  // Never upscaled.
  await tab(page, 2).click();
  await expect
    .poll(async () => {
      const i = await region(page).locator('img').boundingBox();
      return i ? [Math.round(i.width), Math.round(i.height)] : null;
    })
    .toEqual([8, 8]);
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
