import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { calls } from '../support/ipcCalls';
import { IPC } from '../support/ipcChannels';

// Contract e2e-deny-timeout. Backend half: dbmcpflow TestE2EDenyTimeout, the same five card kinds. Each
// card's Don't send / Withhold sends the recorded denial, and a card whose time ran out in the backend
// leaves when the list drops it, with nothing answered.

const SC = 'e2e-deny-timeout';
const KINDS = ['api-request', 'api-response', 'db-query', 'db-response', 'db-exemption'] as const;
const item = (kind: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${kind}`)[0] as ApprovalItem;
const deny = (kind: string) =>
  contract<{ ids: string[]; approve: boolean }>(SC, `args:ApprovalsService.Answer#deny-${kind}`);

/** The kept times of an item are rewritten live; items without a kept list pass unchanged. */
function live(it: ApprovalItem): ApprovalItem {
  const payload = structuredClone(it.payload) as { kept?: Record<string, unknown>[] };
  if (payload.kept) {
    payload.kept = payload.kept.map((k) => ({
      ...k,
      createdAt: new Date(Date.now() - 60_000).toISOString(),
      expiresAt: new Date(Date.now() + 59 * 60_000).toISOString(),
    }));
  }
  return liveItem({ ...it, payload });
}

for (const kind of KINDS) {
  test(`contract: ${kind} is denied with the recorded answer and carries no typed value`, async ({
    relaunch,
  }) => {
    const it = live(item(kind));
    const answer = deny(kind);
    const held = kind.endsWith('response')
      ? [
          {
            channel: IPC.dbMcpPendingResponse,
            response: contract(SC, `DbMcpService.PendingResponse#${kind}`),
          },
        ]
      : [];
    const { window: page, control } = await relaunch({
      control: [...held, { channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
    });
    await pushApprovals(page, [it]);
    const card = page.locator(`[data-testid="approval-item-${it.id}"]`);
    await expect(card).toBeVisible();
    await page.click(`[data-testid="approval-deny-${it.id}"]`);
    await expect
      .poll(() => calls(control, IPC.approvalsAnswer)[0]?.args)
      .toEqual({ ...answer, options: null });
  });

  test(`contract: ${kind} leaves when the backend lets it time out`, async ({ relaunch }) => {
    const it = live(item(kind));
    const held = kind.endsWith('response')
      ? [
          {
            channel: IPC.dbMcpPendingResponse,
            response: contract(SC, `DbMcpService.PendingResponse#${kind}`),
          },
        ]
      : [];
    const { window: page, control } = await relaunch({ control: held });
    await pushApprovals(page, [it]);
    await expect(page.locator(`[data-testid="approval-item-${it.id}"]`)).toBeVisible();
    await pushApprovals(page, []);
    await expect(page.locator('[data-testid="approvals-dialog"]')).toHaveCount(0);
    expect(calls(control, IPC.approvalsAnswer)).toHaveLength(0);
  });
}
