import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contract e2e-api-review-chain. Backend half: dbmcpflow TestE2EAPIReviewChain. A scripted write parks
// for approval, is sent, then its response parks for review: the review lists what the script will
// keep, says the request was already sent, Send answers by id and Withhold answers deny. A script that
// fails shows its failure text beside the raw response.

const SC = 'e2e-api-review-chain';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;
const held = (key: string) =>
  contract<{
    requestId: string;
    keep: { name: string; items: number; preview: string }[];
    text: string;
  }>(SC, `DbMcpService.PendingResponse#${key}`);
const answer = (key: string) =>
  contract<{ ids: string[]; approve: boolean; options: unknown }>(
    SC,
    `args:ApprovalsService.Answer#${key}`,
  );

test('contract: the request card is scripted and Send answers by id', async ({ relaunch }) => {
  const it = item('card');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer('card').ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator(`[data-testid="approval-item-${it.id}"]`)).toContainText('POST');
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer('card'));
});

test('contract: the review says the write was sent and lists what the script keeps; Send answers', async ({
  relaunch,
}) => {
  const it = item('review');
  const h = held('review');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: h },
      { channel: IPC.approvalsAnswer, response: answeredOk(answer('send').ids) },
    ],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator('[data-testid="approval-operation"]')).toContainText('already ran');
  const keep = page.locator('[data-testid="api-review-keep"]');
  await expect(keep).toContainText('Will keep');
  await expect(keep.locator('[data-testid="api-review-keep-domains"]')).toContainText('List of 2');
  await expect(keep.locator('[data-testid="kept-value-preview"]')).toContainText('x.test');
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});

test('contract: Withhold answers deny and nothing is kept', async ({ relaunch }) => {
  const it = item('review');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: held('review') },
      { channel: IPC.approvalsAnswer, response: answeredOk([it.id]) },
    ],
  });
  await pushApprovals(page, [liveItem(it)]);
  await page.click(`[data-testid="approval-deny-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: false, options: null });
});

test('contract: a failed script shows its failure beside the raw response', async ({
  relaunch,
}) => {
  const it = item('failed');
  const h = held('failed');
  const { window: page } = await relaunch({
    control: [{ channel: IPC.dbMcpPendingResponse, response: h }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator('[data-testid="api-review-script-failed"]')).toBeVisible();
  await expect(page.locator('[data-testid="api-review-result"]')).toContainText(h.text);
  await page.click('[data-testid="api-review-view-raw"]');
  await expect(page.locator('[data-testid="api-review-raw-body"]')).toContainText('a@x.test');
  await expect(page.locator('[data-testid="api-review-keep"]')).toHaveCount(0);
});
