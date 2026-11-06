import type { Locator, Page } from '@playwright/test';

// P107 I2-25: typeInto/hoverWord were copied, byte-identically or nearly so, across 6/2 spec
// files. One copy here, imported everywhere.

/** Types `text` into a Monaco `view` — `page.keyboard.type` (real per-keystroke events) by
 *  default, or `.insertText` (a paste, no keystroke events) with `{ paste: true }`. */
export async function typeInto(
  view: Locator,
  page: Page,
  text: string,
  options: { paste?: boolean } = {},
): Promise<void> {
  await view.locator('.view-lines').click();
  if (options.paste) {
    await page.keyboard.insertText(text);
  } else {
    await page.keyboard.type(text);
  }
}

/** Hovers the mouse over `word`'s own on-screen position inside a Monaco `view` — a genuine
 *  leave-then-enter, not a teleport from wherever the mouse already sits: Monaco's hover
 *  controller only arms its delay timer on a fresh "entered this token" transition. */
export async function hoverWord(page: Page, view: Locator, word: string): Promise<void> {
  const point = await view.locator('.view-lines').evaluate((el, w) => {
    const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const idx = (node.textContent ?? '').indexOf(w);
      if (idx >= 0) {
        const range = document.createRange();
        range.setStart(node, idx);
        range.setEnd(node, idx + w.length);
        const rect = range.getBoundingClientRect();
        return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
      }
    }
    return null;
  }, word);
  if (!point) throw new Error(`hoverWord: "${word}" not found in .view-lines`);
  await page.mouse.move(0, 0);
  await page.mouse.move(point.x, point.y);
}
