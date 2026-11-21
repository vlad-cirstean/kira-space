import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { editorText } from './support/editorText';

// Contract e2e-db-script-chain. Backend half: dbmcpflow TestE2EDbScriptChain and
// TestE2EDbScriptChainExemption, the same scenarios. A scripted read is held with the rows it read, the
// values it keeps and the masked cells redacted; a scripted write asks as a write; an exemption carries the
// script and the kept params. Each answer is the item id.

interface Held {
  requestId: string;
  text: string;
  raw: string;
  isError: boolean;
  script: { source: string; hash: string } | null;
  keep: { name: string }[];
}

const SC = 'e2e-db-script-chain';
const items = (key: string) => contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`);
const item = (key: string) => items(key)[0] as ApprovalItem;
const held = (key: string) => contract<Held>(SC, `DbMcpService.PendingResponse#${key}`);
const approveArgs = (key: string) =>
  contract<{ ids: string[] }>(SC, `args:ApprovalsService.Answer#${key}`);

const approve = async (page: import('@playwright/test').Page, it: ApprovalItem) =>
  page.click(`[data-testid="approval-approve-${it.id}"]`);
const answered = (control: { log(): { channel: string; args?: unknown }[] }) =>
  control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args;

test('contract: the held read shows the envelope, the script and the ids it keeps', async ({
  relaunch,
}) => {
  const it = item('review');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: held('review') },
      { channel: IPC.approvalsAnswer, response: answeredOk(approveArgs('review').ids) },
    ],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect
    .poll(() => editorText(page.locator('[data-testid="db-review-result"]')))
    .toContain('"count": 2');
  await expect(page.locator('[data-testid="db-review-keep-ids"]')).toContainText(
    /List of 2\s*Plain\s*\[\s*1,\s*2\s*\]/,
  );
  await page.click('[data-testid="db-review-view-raw"]');
  await expect(page.locator('[data-testid="db-mcp-preview-row"]')).toHaveCount(2);
  await approve(page, it);
  await expect
    .poll(() => answered(control))
    .toEqual({ ids: [it.id], approve: true, options: null });
});

test('contract: a masked column shows redacted in the raw view and the result', async ({
  relaunch,
}) => {
  const it = item('review');
  const { window: page } = await relaunch({
    control: [{ channel: IPC.dbMcpPendingResponse, response: held('masked') }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await page.click('[data-testid="db-review-view-raw"]');
  const rows = page.locator('[data-testid="db-mcp-preview-row"]');
  await expect(rows).toHaveCount(2);
  await expect(rows.first()).toContainText('[redacted]');
  await expect(rows.first()).not.toContainText('one');
});

test('contract: a scripted write asks as a write with its script, Deny answers the id', async ({
  relaunch,
}) => {
  const it = item('write');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(approveArgs('deny').ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator('[data-testid="db-approval-script"]')).toBeVisible();
  await expect(page.locator('[data-testid="approval-chain"]')).toHaveCount(0);
  await page.click(`[data-testid="approval-deny-${it.id}"]`);
  await expect
    .poll(() => answered(control))
    .toEqual({ ids: [it.id], approve: false, options: null });
});

test('contract: the exemption shows the script and the kept param, Approve answers the id', async ({
  relaunch,
}) => {
  const it = item('exemption');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(approveArgs('exemption').ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator('[data-testid="db-mcp-exemption-limit"]')).toHaveCount(1);
  await expect(page.locator('[data-testid="db-approval-script"]')).toBeVisible();
  await expect(page.locator('[data-testid="db-approval-kept"]')).toHaveText('Uses kept: ids');
  await expect(page.locator('[data-testid="approval-chain"]')).toHaveCount(0);
  await approve(page, it);
  await expect
    .poll(() => answered(control))
    .toEqual({ ids: [it.id], approve: true, options: null });
});
