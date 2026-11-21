import type { Locator, Page } from '@playwright/test';

export function collectionRow(page: Page, id: string): Locator {
  return page.locator(`[data-testid="collection-row"][data-id="${id}"]`);
}

/** Opens the Variables tab from the collections panel header. */
export async function openVariablesTab(page: Page): Promise<void> {
  await page.click('[data-testid="api-environments"]');
}

export function matrixRow(page: Page, name: string): Locator {
  return page.locator(`[data-testid="matrix-row-${name}"]`);
}

/** The cell of one variable name in one file's column (a collection or environment id). */
export function matrixCell(page: Page, name: string, ownerId: string): Locator {
  return page.locator(`[data-testid="matrix-cell-${name}-${ownerId}"]`);
}

/** Opens a variable's history popover from its row menu. */
export async function openVariableHistory(page: Page, name: string): Promise<void> {
  await page.click(`[data-testid="matrix-row-menu-${name}"]`);
  await page.click('[data-testid="menu-item-history"]');
}
