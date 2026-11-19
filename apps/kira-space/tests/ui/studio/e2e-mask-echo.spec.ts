import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contract e2e-mask-echo. Backend half: dbmcpflow TestE2EMaskEcho. An API echoes a typed secret back
// in six encodings; the held response shows every one of them as the reference, never the value, in
// the result and the raw view.

const SC = 'e2e-mask-echo';

test('contract: the review shows only the reference for every echoed form', async ({
  relaunch,
}) => {
  const it = contract<ApprovalItem[]>(SC, 'ApprovalsService.List#forms')[0] as ApprovalItem;
  const h = contract<{ text: string; raw: string }>(SC, 'DbMcpService.PendingResponse#forms');
  const { window: page } = await relaunch({
    control: [{ channel: IPC.dbMcpPendingResponse, response: h }],
  });
  await pushApprovals(page, [liveItem(it)]);
  const result = page.locator('[data-testid="api-review-result"]');
  for (const form of ['b64', 'b64url', 'json', 'path', 'query', 'raw']) {
    await expect(result).toContainText(`"${form}": "{{kept.apikey}}"`);
  }
  await page.click('[data-testid="api-review-view-raw"]');
  await expect(page.locator('[data-testid="api-review-raw-body"]')).toContainText(
    '"raw": "{{kept.apikey}}"',
  );
  await expect(page.locator('body')).not.toContainText('e2e mask/echo+Q9=z');
});
