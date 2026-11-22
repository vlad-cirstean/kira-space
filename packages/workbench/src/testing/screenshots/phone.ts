import type { Page } from '@playwright/test';
import { FRAME, prepareCapture, SCALE, saveShot } from './capture';

// P734: presentation shot of the phone app. Not a test helper: nothing here asserts app behaviour.

export interface PhoneOptions {
  readonly id: string;
  /** The console errors collected during the run. */
  readonly consoleErrors: readonly string[];
}

const RADIUS = 40;
const MARGIN = { top: 24, right: 32, bottom: 64, left: 32 } as const;
const CAPTURE = { animations: 'disabled', caret: 'hide', scale: 'device' } as const;

async function compose(page: Page, raw: Buffer, width: number, height: number): Promise<Buffer> {
  const browser = page.context().browser();
  if (!browser) throw new Error('capturePhone needs a page from a browser context');
  const viewport = {
    width: width + MARGIN.left + MARGIN.right,
    height: height + MARGIN.top + MARGIN.bottom,
  };
  const context = await browser.newContext({ viewport, deviceScaleFactor: SCALE });
  const shot = await context.newPage();
  try {
    await shot.setContent(`<!doctype html>
<html><body style="margin:0;background:transparent">
<div id="wrap" style="box-sizing:border-box;width:${viewport.width}px;height:${viewport.height}px;padding:${MARGIN.top}px ${MARGIN.right}px ${MARGIN.bottom}px ${MARGIN.left}px">
<img id="raw" style="display:block;width:${width}px;height:${height}px;border-radius:${RADIUS}px;box-shadow:${FRAME.outline}, ${FRAME.shadow}" src="data:image/png;base64,${raw.toString('base64')}">
</div></body></html>`);
    await shot.evaluate(() => Promise.all([...document.images].map((img) => img.decode())));
    return await shot.locator('#wrap').screenshot({ omitBackground: true, animations: 'disabled' });
  } finally {
    await context.close();
  }
}

/** Writes `screenshots/phone/<id>.webp`. Call once the screen shows its key content: the caller asserts that first. */
export async function capturePhone(page: Page, options: PhoneOptions): Promise<void> {
  const { id, consoleErrors } = options;
  await prepareCapture(page, consoleErrors);
  const size = page.viewportSize();
  if (!size) throw new Error('capturePhone needs a viewport');
  const raw = await page.screenshot(CAPTURE);
  await saveShot(`phone/${id}`, await compose(page, raw, size.width, size.height));
}
