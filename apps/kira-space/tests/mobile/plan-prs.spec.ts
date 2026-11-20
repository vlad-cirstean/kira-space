import { expect, test } from './fixtures';

// Pull request glyphs on the Plan tab, against the committed change-requests fixture.

const t = (id: string) => `[data-testid="${id}"]`;

test.beforeEach(async ({ app, server }) => {
  server.state.auth = 'ok';
  await app();
});

test('a plan card shows each branch PR with its glyph and status lines', async ({ page }) => {
  await page.locator(t('tab-plan')).click();
  const hooks = page.locator(`${t('plan-task-pr')}[data-branch-id="b_hooks"]`);
  await expect(hooks).toContainText('#212 Outgoing webhooks');
  const glyph = hooks.locator('[data-pr-state]');
  await expect(glyph).toHaveAttribute('data-pr-state', 'open');
  await expect(glyph).toHaveAttribute('data-tone', 'purple');
  await expect(glyph).toHaveAttribute('data-checks', 'success');
  await expect(glyph).toHaveAttribute('data-review', 'approved');
  await expect(glyph).toHaveAttribute('data-auto-merge', 'true');
  await expect(hooks.locator('[data-part="checks"]')).toHaveText('Checks passed');
  await expect(hooks.locator('[data-part="review"]')).toHaveText('Approved');
  await expect(hooks.locator('[data-part="mergeable"]')).toHaveText('No conflicts');
  await expect(hooks.locator('[data-part="autoMerge"]')).toHaveText('Auto-merge on');
});

test('conflicts and changes requested tint the glyph red', async ({ page }) => {
  await page.locator(t('tab-plan')).click();
  const sara = page.locator(`${t('plan-task-pr')}[data-branch-id="b_sara"] [data-pr-state]`);
  await expect(sara).toHaveAttribute('data-tone', 'red');
  await expect(sara).toHaveAttribute('data-mergeable', 'conflicting');
  await expect(sara).toHaveAttribute('data-checks', 'failure');
});

test('a draft is grey and a merged PR shows no checks', async ({ page }) => {
  await page.locator(t('tab-plan')).click();
  const draft = page.locator(`${t('plan-task-pr')}[data-branch-id="b_authui"] [data-pr-state]`);
  await expect(draft).toHaveAttribute('data-pr-state', 'draft');
  await expect(draft).toHaveAttribute('data-tone', 'grey');
  const merged = page.locator(`${t('plan-task-pr')}[data-branch-id="b_cart"] [data-pr-state]`);
  await expect(merged).toHaveAttribute('data-pr-state', 'merged');
  await expect(merged).toHaveAttribute('data-tone', 'purple');
  await expect(merged).not.toHaveAttribute('data-checks', /.+/);
});
