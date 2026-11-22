import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { editorText, squash } from './support/editorText';

// Contract dbmcp-query-script-review. Backend half: dbmcpflow TestDbMcpQueryScriptReview, the same scenario.
// A held run_query result with a script parks as a db.response item whose payload is sizes only. The
// pending-response query supplies what the agent gets, the rows the script read, the script and the
// values it will keep; nothing is stored until Send.

interface Held {
  requestId: string;
  text: string;
  raw: string;
  isError: boolean;
  script: { source: string; hash: string } | null;
  keep: { name: string }[];
}

const SC = 'dbmcp-query-script-review';
const item = () => contract<ApprovalItem[]>(SC, 'ApprovalsService.List')[0] as ApprovalItem;
const held = () => contract<Held>(SC, 'DbMcpService.PendingResponse');

const mocks = () => [
  { channel: IPC.dbMcpPendingResponse, response: held() },
  {
    channel: IPC.approvalsAnswer,
    response: answeredOk(contract<{ ids: string[] }>(SC, 'args:ApprovalsService.Answer').ids),
  },
];

test('contract: the result view shows what the agent gets, the script and the values it keeps', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: mocks() });
  await pushApprovals(page, [liveItem(item())]);
  const result = page.locator('[data-testid="db-review-result"]');
  await expect.poll(() => editorText(result)).toContain('"count": 2');
  await expect(result).not.toContainText('one');
  const script = page.locator('[data-testid="db-review-script"]');
  await expect
    .poll(async () => squash(await editorText(script)))
    .toBe(squash(held().script?.source ?? ''));
  const keep = page.locator('[data-testid="db-review-keep"]');
  await expect(keep.locator('[data-testid="db-review-keep-ids"]')).toContainText(
    /ids\s*Plain\s*\[\s*1,\s*2\s*\]/,
  );
});

test('contract: the rows view shows the result the script read', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: mocks() });
  await pushApprovals(page, [liveItem(item())]);
  await page.click('[data-testid="db-review-view-raw"]');
  const rows = page.locator('[data-testid="db-mcp-preview-row"]');
  await expect(rows).toHaveCount(2);
  await expect(rows.first()).toContainText('one');
  await page.click('[data-testid="db-review-view-result"]');
  await expect(page.locator('[data-testid="db-review-result"]')).toBeVisible();
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
