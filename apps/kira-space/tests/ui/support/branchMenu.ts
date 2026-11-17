import type { Page } from '@playwright/test';
import { revealRow } from './adeV2';

const t = (id: string) => `[data-testid="${id}"]`;

/**
 * Opens the row menu of a task branch row and clicks one item. `panel` is the task panel's row;
 * `board` is the plan row, whose actions show on hover.
 */
export async function pick(
  page: Page,
  branchId: string,
  item: string,
  scope: 'panel' | 'board' = 'panel',
): Promise<void> {
  const menu = `${t('ade-task-branch-menu')}[data-branch-id="${branchId}"]`;
  const trigger =
    scope === 'panel'
      ? page.locator(`${t('ade-task-branch-item')}[data-branch-id="${branchId}"] ${menu}`)
      : page.locator(`${t('ade-branch-row')}[data-branch-id="${branchId}"] ${menu}`);
  if (scope === 'board') await revealRow(page, branchId, 'board', 'ade-task-branch-menu');
  await trigger.click();
  await page.locator(t(item)).click();
}
