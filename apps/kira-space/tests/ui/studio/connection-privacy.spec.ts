import { applyVisible } from '@shared/domain/mask';
import { expect, studioTest as test } from '../fixtures';
import { openPrivacyTab, type Page } from './support/privacyTab';

// The connection Privacy tab: a rule masks its column name in every table. Backend half: dbmcpflow
// TestDbMcpMaskRuleMatchesEveryTable, whose privacy-masking contract pins the listing in support/privacyTab.ts.

const rows = '[data-testid="connection-privacy-review-row"]';
const rules = '[data-testid="mask-rule"]';

const row = (page: Page, ref: string) => page.locator(`${rows}[data-ref="${ref}"]`);
const status = (page: Page) => page.locator('[data-testid="connection-privacy-review-status"]');
const showAll = (page: Page) =>
  page.locator('[data-testid="connection-privacy-review-filter"]').selectOption('all');

test('contract: a rule naming another table still masks the column in every table', async ({
  relaunch,
}) => {
  const page = await openPrivacyTab(relaunch, {
    masks: [{ table: 'public.elsewhere', column: 'secret', kind: 'email' }],
    reviewed: ['notes.id', 'notes.body', 'other.id'],
  });
  await showAll(page);
  for (const ref of ['notes.secret', 'other.secret']) {
    await expect(row(page, ref)).toHaveAttribute('data-state', 'masked');
    await expect(
      row(page, ref).locator('[data-testid="connection-privacy-review-state"]'),
    ).toHaveText('Masked · email');
  }
  await expect(status(page)).toContainText('Every column is masked or allowed');
  await expect(page.locator('[data-testid="mask-rule-table-ignored"]')).toBeVisible();
});

test('no rules: the overview says the agent sees everything and the list is empty', async ({
  relaunch,
}) => {
  const page = await openPrivacyTab(relaunch);
  await expect(page.locator('[data-testid="mask-rule-empty"]')).toBeVisible();
  const overview = page.locator('[data-testid="connection-privacy-overview"]');
  await expect(overview).toContainText('No masked columns');
  await expect(overview).toContainText('reach the agent unmasked');
  await expect(overview).not.toContainText('Database MCP is off');
  await expect(page.locator('[data-testid="mask-key-section"]')).toHaveCount(0);
});

test('with MCP off the overview says no agent reads the connection', async ({ relaunch }) => {
  const page = await openPrivacyTab(relaunch, { mcp: false });
  await expect(page.locator('[data-testid="connection-privacy-overview-mcp-off"]')).toContainText(
    'Database MCP is off',
  );
});

test('adding a rule shows what the agent sees; a repeated name is refused', async ({
  relaunch,
}) => {
  const page = await openPrivacyTab(relaunch);
  await expect(page.locator('[data-testid="mask-rule-add-table"]')).toHaveCount(0);
  const column = page.locator('[data-testid="mask-rule-add-column"]');
  await page.locator('[data-testid="mask-rule-add-kind"]').selectOption('email');
  await column.fill('email');
  await page.locator('[data-testid="mask-rule-add"]').click();
  await expect(page.locator(rules)).toHaveCount(1);
  const masked = applyVisible(
    { kind: 'email', keepHint: true, correlate: true },
    'jane.doe@example.com',
  );
  await expect(page.locator('[data-testid="mask-rule-example"]')).toHaveText(`${masked}#K3F9QZ`);
  await expect(page.locator('[data-testid="mask-key-section"]')).toBeVisible();

  await column.fill('EMAIL');
  await page.locator('[data-testid="mask-rule-add"]').click();
  await expect(page.locator('[data-testid="mask-rule-duplicate"]')).toContainText(
    'already has a rule',
  );
  await expect(page.locator(rules)).toHaveCount(1);
});

test('review rows: Mask as adds a rule, Allow and Undo flip the state, Allow all clears the rest', async ({
  relaunch,
}) => {
  const page = await openPrivacyTab(relaunch, { reviewed: ['notes.id', 'other.id'] });
  await showAll(page);
  await expect(status(page)).toContainText('3 columns hidden until you decide');

  await row(page, 'notes.body')
    .locator('[data-testid="connection-privacy-review-mask-as"]')
    .click();
  await page.locator('[data-testid="mask-kind-menu-name"]').click();
  await expect(page.locator(rules)).toHaveCount(1);
  await expect(row(page, 'notes.body')).toHaveAttribute('data-state', 'masked');

  await row(page, 'notes.secret')
    .locator('[data-testid="connection-privacy-review-allow"]')
    .click();
  await expect(row(page, 'notes.secret')).toHaveAttribute('data-state', 'reviewed');
  await expect(row(page, 'notes.secret')).toContainText('Allowed');
  await row(page, 'notes.secret').locator('[data-testid="connection-privacy-review-undo"]').click();
  await expect(row(page, 'notes.secret')).toHaveAttribute('data-state', 'pending');

  await page.locator('[data-testid="connection-privacy-review-allow-all"]').click();
  await expect(status(page)).toContainText('Every column is masked or allowed');
  await expect(row(page, 'other.secret')).toHaveAttribute('data-state', 'reviewed');
});

test('the enforcement notes are collapsed until opened', async ({ relaunch }) => {
  const page = await openPrivacyTab(relaunch);
  const notes = page.locator('[data-testid="connection-privacy-enforce-masking"]');
  await expect(notes).toHaveCount(0);
  await page.locator('[data-testid="connection-privacy-limits"] button').click();
  await expect(notes).toContainText('table is ignored');
  await expect(notes).toContainText('Best effort');
});
