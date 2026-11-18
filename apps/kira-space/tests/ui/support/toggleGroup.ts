import { expect, type Locator } from '@playwright/test';

export async function expectOn(item: Locator): Promise<void> {
  await expect(item).toHaveAttribute('data-state', 'on');
}

// A click on the selected item changes nothing: a single-select group never empties.
export async function expectStaysOnAfterClick(item: Locator): Promise<void> {
  await expectOn(item);
  await item.click();
  await expectOn(item);
}
