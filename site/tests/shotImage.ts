import { expect, type Page } from '@playwright/test';
import { SHOT_CAPTURE, type ShotId } from '../src/data/shotCapture';

/** A window shot shows the whole framed image at any width; a cropped shot is never upscaled past
 *  its natural CSS size. */
export async function expectShotImage(page: Page, id: ShotId): Promise<void> {
  const img = page.locator(`img[data-shot="${id}"]`).last();
  await img.scrollIntoViewIfNeeded();
  await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.currentSrc)).toContain(id);
  const src = await img.evaluate((el: HTMLImageElement) => el.currentSrc);
  expect(src).toMatch(/\.webp$/);
  await expect
    .poll(() => img.evaluate((el: HTMLImageElement) => el.naturalWidth))
    .toBeGreaterThan(0);
  const m = await img.evaluate((el: HTMLImageElement) => ({
    clientWidth: el.clientWidth,
    clientHeight: el.clientHeight,
    naturalWidth: el.naturalWidth,
    naturalHeight: el.naturalHeight,
    fit: getComputedStyle(el).objectFit,
    right: el.getBoundingClientRect().right,
    left: el.getBoundingClientRect().left,
    vw: document.documentElement.clientWidth,
  }));
  if (SHOT_CAPTURE[id].kind === 'window') {
    expect(src).not.toContain('-detail');
    expect(m.fit).not.toBe('cover');
    expect(m.clientWidth / m.clientHeight).toBeCloseTo(m.naturalWidth / m.naturalHeight, 1);
    expect(m.left).toBeGreaterThanOrEqual(0);
    expect(m.right).toBeLessThanOrEqual(m.vw + 1);
    return;
  }
  expect(m.clientWidth).toBeLessThanOrEqual(m.naturalWidth / 2 + 1);
}
