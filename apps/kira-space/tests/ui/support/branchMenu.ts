import type { Page } from '@playwright/test';

const t = (id: string) => `[data-testid="${id}"]`;

/** Opens the row menu of a task branch row and clicks one item. */
export async function pick(page: Page, branchId: string, item: string): Promise<void> {
  await page.locator(`${t('ade-task-branch-menu')}[data-branch-id="${branchId}"]`).click();
  await page.locator(t(item)).click();
}
