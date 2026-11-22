import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { editorText } from './support/editorText';

// Contract e2e-review-request. Backend half: dbmcpflow TestE2EReviewRequestAPI, TestE2EReviewRequestDB and
// TestE2EReviewRequestKube, the same scenarios. A held scripted result opens on After with Before beside it,
// and a collapsed Request section above shows the request exactly as the prompt showed it.

const SC = 'e2e-review-request';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;
const held = (key: string) => contract<unknown>(SC, `DbMcpService.PendingResponse#${key}`);

const cases = [
  { key: 'api', prefix: 'api-review', card: 'api-approval-box-request', raw: 'Before' },
  { key: 'db', prefix: 'db-review', card: 'db-mcp-approval-body', raw: 'Before' },
  { key: 'kube', prefix: 'kube-review', card: 'kube-approval-box-request', raw: 'JSON' },
] as const;

async function cardText(page: Page, key: string, testid: string): Promise<string> {
  const it = item(`${key}-card`);
  await pushApprovals(page, [liveItem(it)]);
  const box = page.locator(`[data-testid="approval-item-${it.id}"] [data-testid="${testid}"]`);
  await expect(box).toBeVisible();
  const text = key === 'db' ? await editorText(box) : (await box.innerText()).trim();
  expect(text).not.toBe('');
  await pushApprovals(page, []);
  await expect(box).toHaveCount(0);
  return text;
}

for (const c of cases) {
  test(`contract: the ${c.key} review lists After first and keeps the request collapsed`, async ({
    relaunch,
  }) => {
    const it = item(c.key);
    const { window: page } = await relaunch({
      control: [{ channel: IPC.dbMcpPendingResponse, response: held(c.key) }],
    });
    await pushApprovals(page, [liveItem(it)]);
    const tabs = page.locator(`[data-testid="${c.prefix}-view-result"]`);
    await expect(tabs).toHaveText('After');
    await expect(
      page.locator(`[data-testid="${c.prefix}-view-${c.key === 'kube' ? 'json' : 'raw'}"]`),
    ).toHaveText(c.raw);
    await expect(page.locator(`[data-testid="${c.prefix}-box-request-toggle"]`)).toHaveText(
      'Request',
    );
    await expect(page.locator(`[data-testid="${c.prefix}-box-request-content"]`)).toBeHidden();
    const order = await page
      .locator(`[data-testid^="${c.prefix}-box-"]:not([data-testid*="-request-"])`)
      .evaluateAll((els) => els.map((e) => e.getAttribute('data-testid')));
    expect(order[0]).toBe(`${c.prefix}-box-result`);
    expect(order.at(-1)).toBe(`${c.prefix}-box-request`);
  });

  test(`contract: the ${c.key} Request section equals the prompt's request`, async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.dbMcpPendingResponse, response: held(c.key) }],
    });
    const want = await cardText(page, c.key, c.card);
    const it = item(c.key);
    await pushApprovals(page, [liveItem(it)]);
    await expect(page.locator(`[data-testid="approval-item-${it.id}"]`)).toBeVisible();
    await page.click(`[data-testid="${c.prefix}-box-request-toggle"]`);
    const content = page.locator(`[data-testid="${c.prefix}-box-request-content"]`);
    await expect(content).toBeVisible();
    if (c.key === 'db') {
      expect(await editorText(content.locator(`[data-testid="${c.card}"]`))).toBe(want);
    } else {
      expect((await content.innerText()).trim()).toBe(want);
    }
  });
}
