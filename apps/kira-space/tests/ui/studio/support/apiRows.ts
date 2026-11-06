import type { Locator, Page } from '@playwright/test';

export function collectionRow(page: Page, id: string): Locator {
  return page.locator(`[data-testid="collection-row"][data-id="${id}"]`);
}

export function variableRow(page: Page, id: string): Locator {
  return page.locator(`[data-testid="variable-row"][data-id="${id}"]`);
}
