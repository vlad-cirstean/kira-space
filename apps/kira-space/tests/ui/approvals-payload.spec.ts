import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { editorText } from './studio/support/editorText';
import { contract } from './support/contract';

// Frontend half of adeflow TestOrchestratorSpawn (taskCreateNoBase) and dbmcpflow
// TestDbMcpExplainApproval: the payloads Go sends render a card, never a blank one.

const t = (id: string) => `[data-testid="${id}"]`;

function first(scenario: string, key: string): ApprovalItem {
  const [it] = contract<ApprovalItem[]>(scenario, key);
  if (!it) throw new Error(`contract ${scenario} has no item`);
  return liveItem(it);
}

test('contract: a task.create repo without a base shows its brief and Approve', async ({
  relaunch,
}) => {
  const it = first('ade-orchestrator', 'ApprovalsService.List#taskCreateNoBase');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const card = page.locator(t(`approval-item-${it.id}`));
  await expect(card.locator(t('ade-task-create-brief'))).toContainText('never made');
  await expect(page.locator(t(`approval-approve-${it.id}`))).toBeVisible();
  await expect(card.locator(t('approval-unreadable'))).toHaveCount(0);
});

test('contract: an explain approval is labelled Explain and shows the agent SQL', async ({
  relaunch,
}) => {
  const it = first('dbmcp-explain-approval', 'ApprovalsService.List');
  const sql = (it.payload as { statement: string }).statement;
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const card = page.locator(t(`approval-item-${it.id}`));
  await expect(card.locator(t('approval-operation'))).toHaveText('Explain');
  await expect(card.locator(t('db-mcp-approval-body'))).toBeVisible();
  // The card pretty-prints the SQL: compare without whitespace.
  const squash = (x: string) => x.replace(/\s+/g, ' ').trim();
  await expect
    .poll(async () => squash(await editorText(page.locator(t('db-mcp-approval-statement')))))
    .toBe(squash(sql));
  await expect(page.locator(t(`approval-approve-${it.id}`))).toBeVisible();
});

test('a payload the kind cannot read shows an error and Deny, never a blank card', async ({
  relaunch,
}) => {
  const good = first('ade-orchestrator', 'ApprovalsService.List#taskCreateNoBase');
  const it = { ...good, payload: { nonsense: true } };
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const card = page.locator(t(`approval-item-${it.id}`));
  await expect(card.locator(t('approval-unreadable'))).toBeVisible();
  await expect(page.locator(t(`approval-deny-${it.id}`))).toBeVisible();
  await expect(page.locator(t(`approval-approve-${it.id}`))).toHaveCount(0);
});
