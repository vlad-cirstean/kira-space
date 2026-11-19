import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contract e2e-db-params. Backend half: dbmcpflow TestE2EDbParams, the same scenario. A write that binds a
// kept text value parks as a db.query card naming the variable at its placeholder; Approve answers the item id.
// The refusals and the 1000-id read have no card: the backend half asserts them.

const SC = 'e2e-db-params';
const item = () => contract<ApprovalItem[]>(SC, 'ApprovalsService.List#write')[0] as ApprovalItem;

interface KeptPayload {
  kept: { createdAt: string; expiresAt: string; [k: string]: unknown }[];
  [k: string]: unknown;
}

function live(it: ApprovalItem) {
  const payload = structuredClone(it.payload) as KeptPayload;
  payload.kept = payload.kept.map((k) => ({
    ...k,
    createdAt: new Date(Date.now() - 60_000).toISOString(),
    expiresAt: new Date(Date.now() + 2880_000).toISOString(),
  }));
  return liveItem({ ...it, payload });
}

test('contract: the write card pairs ? 1 with the kept text, quote and statement included', async ({
  relaunch,
}) => {
  const it = live(item());
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const card = page.locator(`[data-testid="approval-item-${it.id}"]`);
  await expect(card.locator('[data-testid="db-approval-kept"]')).toHaveText('Uses kept: word');
  const row = card.locator('[data-testid="db-approval-kept-word"]');
  await expect(row).toContainText('Text');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toContainText('drop table');
  const params = card.locator('[data-testid="db-approval-params"]');
  await expect(params.locator('[data-testid="db-approval-param-0-placeholder"]')).toHaveText('? 1');
  await expect(params.locator('[data-testid="db-approval-param-0-value"] mark')).toHaveText(
    '{{kept.word}}',
  );
});

test('contract: Approve answers the item id', async ({ relaunch }) => {
  const it = item();
  const args = contract<{ ids: string[] }>(SC, 'args:ApprovalsService.Answer#write');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(args.ids) }],
  });
  await pushApprovals(page, [live(it)]);
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});
