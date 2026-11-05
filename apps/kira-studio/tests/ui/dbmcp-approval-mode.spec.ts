import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { contract } from './support/contract';

// Contract dbmcp-approval-mode. Backend half: dbmcpflow TestDbMcpModeTightenOnly. A SELECT declared
// write parks for approval as a write and says what it looks like.

test('contract: a declared write that looks like a read shows both and the read-only hint', async ({
  relaunch,
}) => {
  const [item] = contract<ApprovalItem[]>('dbmcp-approval-mode', 'ApprovalsService.List');
  if (!item) throw new Error('contract has no item');
  const { window: page } = await relaunch({ control: [] });
  await pushApprovals(page, [liveItem(item)]);

  const card = page.locator(`[data-testid="approval-item-${item.id}"]`);
  await expect(card).toBeVisible();
  await expect(card.locator('[data-testid="db-mcp-approval-body"]')).toContainText('write');
  await expect(card.locator('[data-testid="db-mcp-approval-declared-higher"]')).toContainText(
    'Declared write, looks like a read',
  );
  await expect(card.locator('[data-testid="db-mcp-approval-detected"]')).toContainText(
    'Looks like read',
  );
  await expect(card.locator('[data-testid="db-mcp-approval-enforcement"]')).toHaveCount(0);
});

test('a read item names what enforces it', async ({ relaunch }) => {
  const [base] = contract<ApprovalItem[]>('dbmcp-approval-mode', 'ApprovalsService.List');
  if (!base) throw new Error('contract has no item');
  const payload = {
    ...(base.payload as object),
    mode: 'read',
    detected: 'read',
    enforcement: 'app',
  };
  const item = {
    ...base,
    operation: { ...base.operation, class: 'read', tone: 'neutral' as const },
    payload,
  };
  const { window: page } = await relaunch({ control: [] });
  await pushApprovals(page, [liveItem(item)]);

  const card = page.locator(`[data-testid="approval-item-${item.id}"]`);
  await expect(card.locator('[data-testid="db-mcp-approval-enforcement"]')).toContainText(
    'checked by Kira',
  );
  await expect(card.locator('[data-testid="db-mcp-approval-declared-higher"]')).toHaveCount(0);
});
