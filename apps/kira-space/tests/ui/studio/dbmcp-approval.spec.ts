import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { editorText } from './support/editorText';

// Contract dbmcp. Backend half: dbmcpflow TestDbMcpApprovalFlow. A write from an AI client parks in
// the approvals dialog; Approve answers the item id.

test('contract: a pending write shows connection, operation and statement; Approve answers by item id', async ({
  relaunch,
}) => {
  const [item] = contract<ApprovalItem[]>('dbmcp', 'ApprovalsService.List');
  const answer = contract<{ ids: string[] }>('dbmcp', 'args:ApprovalsService.Answer');
  if (!item) throw new Error('contract has no item');
  const payload = item.payload as { statement: string };
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });

  await pushApprovals(page, [liveItem(item)]);
  const card = page.locator(`[data-testid="approval-item-${item.id}"]`);
  const statement = page.locator('[data-testid="db-mcp-approval-statement"]');
  const name = card.locator('[data-testid="approval-target-name"]');
  await expect(name).toHaveText(item.target.name);
  await expect(card.locator('[data-testid="approval-target-kind"]')).toHaveText('SQLite');
  await expect(card.locator('[data-testid="approval-operation"]')).toHaveText(
    new RegExp(`^${item.operation.class}$`, 'i'),
  );
  await expect(card.locator('[data-testid="approval-origin"]')).toContainText(item.origin.client);
  await expect(card).toContainText(item.target.location);
  await expect(card.locator('[data-testid="approval-countdown"]')).toContainText('Expires in');
  const nameBox = await name.boundingBox();
  const statementBox = await statement.boundingBox();
  expect(nameBox?.y).toBeLessThan(statementBox?.y ?? 0);
  const formatted = await editorText(statement);
  expect(formatted).toContain('\n');
  expect(formatted.replace(/\s+/g, ' ').trim()).toBe(payload.statement);
  await page.click('[data-testid="db-mcp-approval-view-as-sent"]');
  await expect.poll(() => editorText(statement)).toBe(payload.statement);
  const box = await page.locator('[data-testid="approvals-dialog"]').boundingBox();
  expect(box?.width).toBeGreaterThan(600);
  await page.click(`[data-testid="approval-approve-${item.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer);
});

test('contract: Deny is focused first and denies by item id', async ({ relaunch }) => {
  const [item] = contract<ApprovalItem[]>('dbmcp', 'ApprovalsService.List');
  if (!item) throw new Error('contract has no item');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([item.id]) }],
  });

  await pushApprovals(page, [liveItem(item)]);
  await expect(page.locator(`[data-testid="approval-deny-${item.id}"]`)).toBeFocused();
  await page.keyboard.press('Enter');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [item.id], approve: false, options: null });
});
