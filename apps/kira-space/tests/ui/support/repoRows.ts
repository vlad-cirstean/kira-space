import type { Page } from '@playwright/test';

export function repoTab(page: Page, kind?: string) {
  return kind
    ? page.locator(`[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-tab-kind="${kind}"]`)
    : page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"]');
}

export function repoRowById(page: Page, id: string) {
  return page.locator(`[data-testid="repo-row"][data-repo-id="${id}"]`);
}
