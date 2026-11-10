import { expect, studioTest as test } from '../../ui/fixtures';
import { openPrivacyTab } from '../../ui/studio/support/privacyTab';

// The connection Privacy tab at rest: two rules (one naming a table, so it carries the Table ignored
// badge), column review on with one waiting column, and the empty tab.

test('privacy tab with rules and a waiting column', async ({ relaunch }) => {
  const page = await openPrivacyTab(relaunch, {
    masks: [
      { table: 'public.elsewhere', column: 'secret', kind: 'email' },
      { column: 'created_at', kind: 'date' },
    ],
    reviewed: ['notes.id', 'other.id'],
  });
  await expect(page.locator('[data-testid="connection-privacy-review-row"]')).toHaveCount(1);
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toHaveScreenshot(
    'connection-privacy-tab.png',
  );
  await expect(page.locator('[data-testid="connection-privacy-review"]')).toHaveScreenshot(
    'connection-privacy-review.png',
  );
});

test('privacy tab with no rules', async ({ relaunch }) => {
  const page = await openPrivacyTab(relaunch);
  await expect(page.locator('[data-testid="mask-rule-empty"]')).toBeVisible();
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toHaveScreenshot(
    'connection-privacy-empty.png',
  );
});
