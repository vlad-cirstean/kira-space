import { expect, test } from '@playwright/test';

const BASE = '/kira-studio/';

test.skip(process.platform !== 'linux', 'Linux-recorded baseline');

for (const route of [{ name: 'landing', path: '' }]) {
  test(`${route.name} page matches the baseline`, async ({ page }) => {
    test.slow();
    await page.goto(`${BASE}${route.path}`);
    await page.evaluate(async () => {
      for (let y = 0; y < document.documentElement.scrollHeight; y += window.innerHeight) {
        window.scrollTo({ top: y, behavior: 'instant' });
        await new Promise((resolve) => setTimeout(resolve, 60));
      }
      // Not img.decode(): WebKit leaves it pending for good under load, even on a loaded image.
      // A forced-eager image settles on load or error; the screenshot retry covers decoding.
      await Promise.all(
        [...document.images].map((img) => {
          img.loading = 'eager';
          if (img.complete) return undefined;
          return new Promise((resolve) => {
            img.addEventListener('load', resolve, { once: true });
            img.addEventListener('error', resolve, { once: true });
          });
        }),
      );
      await document.fonts.ready;
      window.scrollTo({ top: 0, behavior: 'instant' });
    });
    await expect(page).toHaveScreenshot(`${route.name}.png`, {
      fullPage: true,
      animations: 'disabled',
      caret: 'hide',
    });
  });
}
