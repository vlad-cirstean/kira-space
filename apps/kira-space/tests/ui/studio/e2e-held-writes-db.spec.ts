import type { ApprovalItem } from '@shared/domain/approvals';
import type { KeptList } from '@shared/domain/kept';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openKeptPane } from './support/settings';

// Contract e2e-held-writes-db. Backend half: dbmcpflow TestE2EHeldWritesDB, the same scenario. A held scripted
// result lists the value it will keep; Send and Withhold answer the item id, and the kept list shows the
// value only after Send. The other exits (timeout, cancel, scripts off, failures, caps) are the backend's.

interface Held {
  keep: { name: string; preview: string }[];
  [k: string]: unknown;
}

const SC = 'e2e-held-writes-db';
const item = () => contract<ApprovalItem[]>(SC, 'ApprovalsService.List#db')[0] as ApprovalItem;
const held = () => contract<Held>(SC, 'DbMcpService.PendingResponse#db');
const answerOf = (key: string) =>
  contract<{ ids: string[]; approve: boolean }>(SC, `args:ApprovalsService.Answer#${key}`);

const mocks = (key: string) => [
  { channel: IPC.dbMcpPendingResponse, response: held() },
  { channel: IPC.approvalsAnswer, response: answeredOk(answerOf(key).ids) },
];

test('contract: the held result lists w with its preview before anything is stored', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: mocks('send') });
  await pushApprovals(page, [liveItem(item())]);
  const row = page.locator('[data-testid="db-review-keep-w"]');
  await row.locator('[data-testid="kept-name"]').hover();
  await expect(page.locator('[data-testid="kept-details"]')).toContainText('List of 2');
  await expect(row).toContainText(/Plain\s*\[\s*1,\s*2\s*\]/);
});

for (const key of ['send', 'withhold'] as const) {
  test(`contract: ${key} answers the item id`, async ({ relaunch }) => {
    const it = item();
    const want = answerOf(key);
    const { window: page, control } = await relaunch({ control: mocks(key) });
    await pushApprovals(page, [liveItem(it)]);
    await page.click(`[data-testid="approval-${want.approve ? 'approve' : 'deny'}-${it.id}"]`);
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
      .toEqual({ ids: [it.id], approve: want.approve, options: null });
  });
}

test('contract: after Send the kept pane lists w from the db script', async ({ relaunch }) => {
  const list = contract<KeptList>(SC, 'KeptVarsService.List#afterSend');
  const now = Date.now();
  const live: KeptList = {
    ...list,
    entries: list.entries.map((e) => ({
      ...e,
      createdAt: new Date(now - 60_000).toISOString(),
      expiresAt: new Date(now + 3_540_000).toISOString(),
    })),
  };
  const { window: page } = await relaunch({ control: [{ channel: IPC.keptList, response: live }] });
  await openKeptPane(page);
  const entries = page.locator('[data-testid="settings-kept-entry"]');
  await expect(entries).toHaveCount(1);
  await entries.first().locator('[data-testid="kept-name"]').hover();
  await expect(page.locator('[data-testid="kept-details"]')).toContainText('List of 2');
  await expect(entries.first().locator('[data-testid="kept-value-preview"]')).toContainText('1');
});
