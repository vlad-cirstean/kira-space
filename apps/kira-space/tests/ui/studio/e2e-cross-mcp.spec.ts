import type { ApprovalItem } from '@shared/domain/approvals';
import { pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { withTimes } from './support/keptTimes';

// Contract e2e-cross-mcp. Backend half: dbmcpflow TestE2ECrossMcp. A list kept by an API script is bound
// into a SQL write and the card names the API request it came from; rows kept by a SQL script fill an API
// body and the card names the statement. The refusals of the backend (a secret or typed value never reaches
// SQL) have no card, so there is nothing to render for them.

const SC = 'e2e-cross-mcp';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;

test('contract: a SQL write that binds an API list shows the API request that kept it', async ({
  relaunch,
}) => {
  const it = withTimes(item('apiToSql'));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = page.locator(`[data-testid="approval-item-${it.id}"]`);
  await expect(c.locator('[data-testid="db-approval-kept"]')).toHaveText('Uses kept: ids');
  const row = c.locator('[data-testid="db-approval-kept-ids"]');
  await expect(row).toContainText('List of 2');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toContainText('9007199254740993');
  await expect(row).toContainText(/GET http:\/\/127\.0\.0\.1:<port>\/raw/);
  await expect(c.locator('[data-testid="db-approval-param-0-placeholder"]')).toHaveText('? 1');
});

test('contract: an API request that fills its body from SQL rows shows the statement that kept them', async ({
  relaunch,
}) => {
  const it = withTimes(item('sqlToApi'));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = page.locator(`[data-testid="approval-item-${it.id}"]`);
  await expect(c.locator('[data-testid="api-approval-kept"]')).toHaveText('Uses kept: bodies');
  const row = c.locator('[data-testid="api-approval-kept-bodies"]');
  await expect(row).toContainText('List of 3');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toContainText('hit');
  await expect(row).toContainText('notes db: select');
  await expect(c.locator('[data-testid="api-approval-body-text"] .kira-ed-kept')).toHaveCount(1);
});
