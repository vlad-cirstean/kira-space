import { expect, type Page } from '@playwright/test';
import { SHOT_CAPTURE, type ShotId } from '../src/data/shotCapture';

/** A window shot serves the phone crop on a phone and the framed window on desktop; a cropped shot
 *  serves one image everywhere, never upscaled past its natural CSS size. */
export async function expectShotImage(page: Page, id: ShotId, phone: boolean): Promise<void> {
  const img = page.locator(`img[data-shot="${id}"]`).last();
  await img.scrollIntoViewIfNeeded();
  await expect.poll(() => img.evaluate((el: HTMLImageElement) => el.currentSrc)).toContain(id);
  const src = await img.evaluate((el: HTMLImageElement) => el.currentSrc);
  expect(src).toMatch(/\.webp$/);
  if (SHOT_CAPTURE[id].kind === 'window') {
    if (phone) expect(src).toContain(`${id}-detail`);
    else expect(src).not.toContain('-detail');
    return;
  }
  const { clientWidth, naturalWidth } = await img.evaluate((el: HTMLImageElement) => ({
    clientWidth: el.clientWidth,
    naturalWidth: el.naturalWidth,
  }));
  expect(clientWidth).toBeLessThanOrEqual(naturalWidth / 2 + 1);
}
