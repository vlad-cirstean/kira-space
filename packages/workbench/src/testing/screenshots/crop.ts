import { expect, type Page } from '@playwright/test';
import { prepareCapture, SCALE, saveShot } from './capture';

// P550: cropped presentation shots of one element. Not a test helper: nothing here asserts app behaviour.

export interface CropOptions {
  readonly id: string;
  /** `floating`: element composited onto a transparent canvas with a shadow. `region`: viewport
   *  clip around the element, real neighbouring UI at the edges. */
  readonly kind: 'floating' | 'region';
  readonly selector: string;
  /** CSS px around the element. Default 24 (`floating`) or 12 (`region`). */
  readonly padding?: number;
  /** The `consoleErrors` fixture of the run. */
  readonly consoleErrors: readonly string[];
}

const SHADOW = '0 1px 2px rgb(0 0 0 / 0.12), 0 8px 24px rgb(0 0 0 / 0.16)';
const CAPTURE = { animations: 'disabled', caret: 'hide', scale: 'device' } as const;

async function floating(page: Page, selector: string, padding: number): Promise<Buffer> {
  const target = page.locator(selector).first();
  const radius = await target.evaluate((el) => getComputedStyle(el).borderRadius);
  const raw = await target.screenshot({ ...CAPTURE, omitBackground: true });
  const browser = page.context().browser();
  if (!browser) throw new Error('captureCrop needs a page from browser.newPage');
  const box = await target.boundingBox();
  if (!box) throw new Error(`No box for ${selector}`);
  const viewport = {
    width: Math.ceil(box.width) + padding * 2,
    height: Math.ceil(box.height) + padding * 2,
  };
  // A page made by browser.newPage owns its context, so compositing needs a fresh one at the same scale.
  const context = await browser.newContext({ viewport, deviceScaleFactor: SCALE });
  const shot = await context.newPage();
  try {
    await shot.setContent(`<!doctype html>
<html><body style="margin:0;background:transparent">
<div id="wrap" style="box-sizing:border-box;width:${viewport.width}px;height:${viewport.height}px;padding:${padding}px">
<img id="raw" style="display:block;width:${Math.ceil(box.width)}px;height:${Math.ceil(box.height)}px;border-radius:${radius};box-shadow:${SHADOW}" src="data:image/png;base64,${raw.toString('base64')}">
</div></body></html>`);
    await shot.evaluate(() => Promise.all([...document.images].map((img) => img.decode())));
    return await shot.locator('#wrap').screenshot({ omitBackground: true, animations: 'disabled' });
  } finally {
    await context.close();
  }
}

async function region(page: Page, selector: string, padding: number): Promise<Buffer> {
  const box = await page.locator(selector).first().boundingBox();
  if (!box) throw new Error(`No box for ${selector}`);
  const viewport = page.viewportSize();
  if (!viewport) throw new Error('captureCrop needs a viewport');
  const x = Math.max(0, box.x - padding);
  const y = Math.max(0, box.y - padding);
  const right = Math.min(viewport.width, box.x + box.width + padding);
  const bottom = Math.min(viewport.height, box.y + box.height + padding);
  return page.screenshot({ ...CAPTURE, clip: { x, y, width: right - x, height: bottom - y } });
}

/** Writes `screenshots/crop/<id>.webp`. Call once the element shows its key content: the caller asserts that first. */
export async function captureCrop(page: Page, options: CropOptions): Promise<void> {
  const { id, kind, selector, consoleErrors } = options;
  const target = page.locator(selector).first();
  await expect(target).toBeVisible();
  await prepareCapture(page, consoleErrors);
  const png =
    kind === 'floating'
      ? await floating(page, selector, options.padding ?? 24)
      : await region(page, selector, options.padding ?? 12);
  await saveShot(`crop/${id}`, png);
}
