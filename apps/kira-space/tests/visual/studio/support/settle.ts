import type { Page } from '@playwright/test';
import { expect } from '../../../ui/fixtures';

// Icons are the codicon web font, loaded on first use. On a starved runner the glyphs paint late,
// so two consecutive captures differ and toHaveScreenshot never gets a stable pair. Wait until no
// font is still loading, then one frame.
export async function settleFonts(page: Page): Promise<void> {
  await expect
    .poll(() => page.evaluate(() => [...document.fonts].every((f) => f.status !== 'loading')), {
      timeout: 30_000,
    })
    .toBe(true);
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        document.fonts.ready.then(() => requestAnimationFrame(() => resolve()));
      }),
  );
}
