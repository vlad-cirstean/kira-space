import { expect, apiTest as test } from '../fixtures';
import { modeTab } from './support/apiMode';

test('API tab strip "+" opens an HTTP or gRPC request tab; Databases has none', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: [] });
  await modeTab(page, 'api').click();

  const plus = page.locator('[data-testid="tab-strip-new"]');
  await expect(plus).toBeVisible();

  await plus.click();
  await expect(page.locator('[data-testid="menu-item-new-grpc-request"]')).toBeVisible();
  await page.click('[data-testid="menu-item-new-request"]');
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);

  await plus.click();
  await page.click('[data-testid="menu-item-new-grpc-request"]');
  await expect(page.locator('[data-testid="grpc-request-view"]')).toBeVisible();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(2);

  await modeTab(page, 'studio').click();
  await expect(page.locator('[data-testid="tab-strip-new"]')).toHaveCount(0);
});
