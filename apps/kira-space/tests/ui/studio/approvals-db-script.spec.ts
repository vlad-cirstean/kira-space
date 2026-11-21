import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { editorText } from './support/editorText';

// Contracts dbmcp-query-script and dbmcp-query-script-kept. Backend half: dbmcpflow TestDbMcpQueryScript and
// TestDbMcpQueryScriptKeep, the same scenarios. A run_query with a result script parks as a db.query item
// carrying the script: the card shows the statement that runs, then the script read-only, what it may keep
// and the kept values it reads, each call marked in the source.

interface ScriptPayload {
  script: { source: string; mayKeep: { name: string }[] };
  kept: { createdAt: string; expiresAt: string; [k: string]: unknown }[];
}

const plain = () =>
  contract<ApprovalItem[]>('dbmcp-query-script', 'ApprovalsService.List')[0] as ApprovalItem;
const withKept = () =>
  contract<ApprovalItem[]>('dbmcp-query-script-kept', 'ApprovalsService.List')[0] as ApprovalItem;
const cardOf = (page: Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-item-${it.id}"]`);

/** The recorded kept item with real kept times: created 12 minutes ago, 48 minutes left. */
function keptItem() {
  const it = withKept();
  const payload = structuredClone(it.payload) as unknown as ScriptPayload;
  payload.kept = payload.kept.map((k) => ({
    ...k,
    createdAt: new Date(Date.now() - 12 * 60_000).toISOString(),
    expiresAt: new Date(Date.now() + 2880 * 1000).toISOString(),
  }));
  return liveItem({ ...it, payload: payload as unknown as ApprovalItem['payload'] });
}

test('contract: the card shows the script next to the statement that runs', async ({
  relaunch,
}) => {
  const it = plain();
  const payload = it.payload as unknown as ScriptPayload;
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const card = cardOf(page, it);
  await expect(
    card.locator('[data-testid="approval-fact"]').filter({ hasText: 'Runs a script' }),
  ).toBeVisible();
  await expect(card.locator('[data-testid="db-approval-kept-writes"]')).toHaveCount(0);
  await expect(card.locator('[data-testid="db-approval-may-keep"]')).toHaveCount(0);
  await expect(card.locator('[data-testid="db-mcp-approval-statement"]')).toBeVisible();
  const script = card.locator('[data-testid="db-approval-script"]');
  await expect.poll(() => editorText(script)).toBe(payload.script.source);
  await expect(script.locator('.kira-ed-kept')).toHaveCount(0);
});

test('contract: Send answers the item id', async ({ relaunch }) => {
  const it = plain();
  const answer = contract<{ ids: string[] }>('dbmcp-query-script', 'args:ApprovalsService.Answer');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});

test('contract: a script that reads and keeps values marks both and lists what it reads', async ({
  relaunch,
}) => {
  const it = keptItem();
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const card = cardOf(page, it);
  await expect(card.locator('[data-testid="db-approval-kept"]')).toHaveText('Uses kept: ids');
  await expect(card.locator('[data-testid="db-approval-kept-writes"]')).toHaveText('May keep: n');
  const row = card.locator('[data-testid="db-approval-kept-ids"]');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toContainText(/1,\s*2/);
  await expect(row.locator('[data-testid="kept-used-in"]')).toHaveText('Transform script');
  await expect(card.locator('[data-testid="db-approval-may-keep"]')).toContainText('n');
  await expect(card.locator('[data-testid="db-approval-script-reads"]')).toBeVisible();
  // kept.get("ids") and kept.set("n", ...) are each marked in the source.
  await expect(card.locator('[data-testid="db-approval-script"] .kira-ed-kept')).toHaveCount(2);
});
