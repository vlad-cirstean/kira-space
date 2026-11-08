import type { Locator, Page } from '@playwright/test';

/** `n` stdout chunks, the first ANSI-green: ~18 px each, far above the default viewport. */
export const longLog = (n: number) =>
  Array.from({ length: n }, (_, i) => ({
    seq: i + 1,
    at: 1_000 + i,
    stream: 'stdout',
    text: `${i === 0 ? '\u001b[32mfirst\u001b[0m' : `line ${i + 1}`} of the log\n`,
  }));

/** Mouse-drags across log rows at index 1 and 2 (virtual rows, so by data-index); returns the page selection text. */
export async function dragSelect(page: Page, rows: Locator): Promise<string> {
  const first = rows.and(page.locator('[data-index="1"]'));
  const second = rows.and(page.locator('[data-index="2"]'));
  await first.waitFor();
  await second.waitFor();
  const a = (await first.boundingBox())!;
  const b = (await second.boundingBox())!;
  await page.mouse.move(a.x + 2, a.y + a.height / 2);
  await page.mouse.down();
  await page.mouse.move(b.x + b.width - 2, b.y + b.height / 2, { steps: 5 });
  await page.mouse.up();
  return page.evaluate(() => window.getSelection()?.toString() ?? '');
}
