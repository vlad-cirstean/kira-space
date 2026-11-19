import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { unmaskedOps } from '../support/dbOps';
import { IPC } from '../support/ipcChannels';

// Contract e2e-script-failures-db. Backend half: dbmcpflow TestE2EScriptFailuresDB, the same scenario. A
// failed result script under review parks as a held item whose result view shows the failure and whose raw
// view still shows the rows; Send answers the item id. Every other failure leaves an error jsTransform row
// whose text equals the agent text.

interface Held {
  requestId: string;
  text: string;
  raw: string;
  isError: boolean;
  keep: unknown[];
}
interface Op {
  kind: string;
  status: string;
  error: string | null;
  command: string;
}

const SC = 'e2e-script-failures-db';
const item = () => contract<ApprovalItem[]>(SC, 'ApprovalsService.List#failed')[0] as ApprovalItem;
const held = () => contract<Held>(SC, 'DbMcpService.PendingResponse#failed');
const answer = () => contract<{ ids: string[] }>(SC, 'args:ApprovalsService.Answer#failed');
const mocks = () => [
  { channel: IPC.dbMcpPendingResponse, response: held() },
  { channel: IPC.approvalsAnswer, response: answeredOk(answer().ids) },
];

test('contract: the held failure shows its text and keeps the raw rows readable', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: mocks() });
  await pushApprovals(page, [liveItem(item())]);
  const result = page.locator('[data-testid="db-review-result"]');
  await expect(result).toContainText('boom held');
  await expect(page.locator('[data-testid="db-review-keep"]')).toHaveCount(0);
  await page.click('[data-testid="db-review-view-raw"]');
  const rows = page.locator('[data-testid="db-mcp-preview-row"]');
  // The fixture's notes table holds a third row by then, from the write-throw case.
  await expect(rows).toHaveCount(3);
  await expect(rows.first()).toContainText('one');
  await expect(rows.nth(1)).toContainText('two');
  await page.click('[data-testid="db-review-view-result"]');
  await expect(result).toContainText('boom held');
});

test('contract: Send answers the item id', async ({ relaunch }) => {
  const it = item();
  const { window: page, control } = await relaunch({ control: mocks() });
  await pushApprovals(page, [liveItem(it)]);
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});

const ROWS = [
  'readTimeout',
  'readMemory',
  'readStack',
  'readOutput',
  'readInput',
  'writeThrow',
  'ddlThrow',
];

test('contract: every failed script is an error row whose detail is the agent text', async ({
  relaunch,
}) => {
  const all = contract<Op[]>(SC, 'OpsService.Recent#failures');
  const ops = unmaskedOps(all as unknown as Record<string, unknown>[]);
  const { window: page } = await relaunch({ control: [{ channel: IPC.opsRecent, response: ops }] });
  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(all.length);
  for (const key of ROWS) {
    const row = contract<{ kind: string; command: string }>(SC, `failure:${key}`);
    const text = contract<string>(SC, `failureText:${key}`);
    expect(text).toContain('The query ran; its result is not returned.');
    expect(all.some((o) => o.command === row.command && o.error === text)).toBe(true);
  }
  await rows.first().click();
  const detail = page.locator('[data-testid="op-detail-error"]');
  await expect(detail).toBeVisible();
  const shown = (await detail.textContent()) ?? '';
  expect(all.map((o) => o.error)).toContain(shown);
});
