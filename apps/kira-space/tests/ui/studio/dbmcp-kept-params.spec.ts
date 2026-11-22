import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { editorText } from './support/editorText';

// Contracts dbmcp-kept-params and dbmcp-kept-params-exemption. Backend half: dbmcpflow
// TestDbMcpKeptParams, TestDbMcpKeptParamsExemption and TestDbMcpEngineKeptParams. A run_query that binds
// kept variables parks as a db.query item (or db.exemption after a limit refusal) whose payload names
// each variable and where the statement reads it: the card lists the variable, marks the placeholder in
// the statement as sent and shows which variable fills which placeholder.

const query = () =>
  contract<ApprovalItem[]>('dbmcp-kept-params', 'ApprovalsService.List')[0] as ApprovalItem;
const exemption = () =>
  contract<ApprovalItem[]>(
    'dbmcp-kept-params-exemption',
    'ApprovalsService.List',
  )[0] as ApprovalItem;
const cardOf = (page: Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-item-${it.id}"]`);

interface KeptPayload {
  kept: { createdAt: string; expiresAt: string; [k: string]: unknown }[];
  [k: string]: unknown;
}

/** The recorded item with real kept times: created 12 minutes ago, `leftSeconds` to expiry. */
function withTimes(it: ApprovalItem, leftSeconds: number) {
  const payload = structuredClone(it.payload) as KeptPayload;
  payload.kept = payload.kept.map((k) => ({
    ...k,
    createdAt: new Date(Date.now() - 12 * 60_000).toISOString(),
    expiresAt: new Date(Date.now() + leftSeconds * 1000).toISOString(),
  }));
  return liveItem({ ...it, payload });
}

test('contract: the card lists the variable with its value, source and where the statement reads it', async ({
  relaunch,
}) => {
  const it = withTimes(query(), 2880);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = cardOf(page, it);
  await expect(c.locator('[data-testid="db-approval-kept"]')).toHaveText('Uses: word');
  await expect(c.locator('[data-testid="db-approval-kept-expiring"]')).toHaveCount(0);
  const row = c.locator('[data-testid="db-approval-kept-word"]');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toContainText('drop table');
  await expect(row.locator('[data-testid="kept-expiry"]')).toContainText(/47m( \d+s)? left/);
  await row.locator('[data-testid="kept-name"]').hover();
  await expect(page.locator('[data-testid="kept-details"]')).toContainText('Text');
  await expect(page.locator('[data-testid="kept-details"]')).toContainText(
    'Used in Statement, Parameter 1',
  );
});

test('contract: the placeholder is marked in the statement as sent', async ({ relaunch }) => {
  const it = withTimes(query(), 2880);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const statement = cardOf(page, it).locator('[data-testid="db-mcp-approval-statement"]');
  await expect(statement.locator('.kira-ed-kept')).toHaveCount(1);
  expect((await editorText(statement)).trim()).toBe('insert into notes (body) values (?)');
  await page.click('[data-testid="db-mcp-approval-view-formatted"]');
  await expect(statement.locator('.kira-ed-kept')).toHaveCount(0);
});

test('contract: the parameters block pairs each placeholder with its variable', async ({
  relaunch,
}) => {
  const it = withTimes(query(), 2880);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const params = cardOf(page, it).locator('[data-testid="db-approval-params"]');
  await expect(params.locator('[data-testid="db-approval-param-0-placeholder"]')).toHaveText('? 1');
  const value = params.locator('[data-testid="db-approval-param-0-value"]');
  await expect(value.locator('mark')).toHaveText('{{plain.word}}');
  await expect(value).toContainText('Text');
});

test('contract: a variable that expires before the card does is flagged', async ({ relaunch }) => {
  const it = withTimes(query(), 60);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  await expect(cardOf(page, it).locator('[data-testid="db-approval-kept-expiring"]')).toHaveText(
    'Expires before you answer',
  );
});

test('contract: a lifted-limit request shows the same variables and marks', async ({
  relaunch,
}) => {
  const it = withTimes(exemption(), 2880);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = cardOf(page, it);
  await expect(c.locator('[data-testid="db-mcp-exemption-limit"]')).toHaveCount(1);
  await expect(c.locator('[data-testid="db-approval-kept"]')).toHaveText('Uses: n');
  await expect(c.locator('[data-testid="db-approval-param-0-placeholder"]')).toHaveText('? 1');
  await expect(c.locator('[data-testid="db-mcp-approval-statement"] .kira-ed-kept')).toHaveCount(1);
});

test('contract: Manage temporary variables opens the settings view', async ({ relaunch }) => {
  const it = withTimes(query(), 2880);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  await cardOf(page, it).locator('[data-testid="db-approval-kept-manage"]').click();
  await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();
});

test('contract: an item without params shows no kept panel and no parameters block', async ({
  relaunch,
}) => {
  const [plain] = contract<ApprovalItem[]>('dbmcp', 'ApprovalsService.List');
  if (!plain) throw new Error('contract has no item');
  const it = liveItem(plain);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = cardOf(page, it);
  await expect(c.locator('[data-testid="db-mcp-approval-statement"]')).toBeVisible();
  await expect(c.locator('[data-testid="db-approval-params"]')).toHaveCount(0);
  await expect(c.locator('[data-testid="db-approval-kept-panel"]')).toHaveCount(0);
});
