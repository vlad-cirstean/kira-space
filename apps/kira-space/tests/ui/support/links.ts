import type { Locator, Page } from '@playwright/test';

// Page object for the link strip (AdeLinkStrip). The card and the add popover render in a portal, so
// they are located from the page, never from inside the strip.

export const pill = (scope: Page | Locator, text: string): Locator =>
  scope.locator('[data-testid="ade-links-pill"]', { hasText: text });

export const line = (scope: Page | Locator, category: string): Locator =>
  scope.locator(`[data-testid="ade-links-line"][data-category="${category}"]`);

const cardOf = (scope: Page | Locator): Locator =>
  ('page' in scope ? scope.page() : scope).locator('[data-testid="ade-links-card"]');

/** Clicks the pill and returns the open card. */
export async function openCard(scope: Page | Locator, text: string): Promise<Locator> {
  await pill(scope, text).click();
  const card = cardOf(scope);
  await card.waitFor();
  return card;
}

/** Opens the add popover with `+`, types the text and presses Enter. */
export async function addLine(page: Page, text: string): Promise<void> {
  await page.locator('[data-testid="ade-links-add"]').click();
  const input = page.locator('[data-testid="ade-links-paste"]');
  await input.fill(text);
  await input.press('Enter');
}
