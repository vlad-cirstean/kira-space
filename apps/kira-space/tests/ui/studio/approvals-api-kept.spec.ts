import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';

// Contract approvals-api-kept. Backend half: dbmcpflow TestKeptVarsChainedRequest. An api_request that
// names a kept variable parks as an api.request item whose payload lists the variable and where each
// reference sits: the card names it, shows its value, source and expiry, and marks the reference.

const base = () =>
  contract<ApprovalItem[]>('approvals-api-kept', 'ApprovalsService.List')[0] as ApprovalItem;
const card = (page: Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-item-${it.id}"]`);

interface KeptPayload {
  kept: { createdAt: string; expiresAt: string; [k: string]: unknown }[];
  [k: string]: unknown;
}

/** The recorded item with real kept times: created 12 minutes ago, `leftSeconds` to expiry. */
function withTimes(it: ApprovalItem, leftSeconds: number, edit?: (p: KeptPayload) => void) {
  const payload = structuredClone(it.payload) as KeptPayload;
  payload.kept = payload.kept.map((k) => ({
    ...k,
    createdAt: new Date(Date.now() - 12 * 60_000).toISOString(),
    expiresAt: new Date(Date.now() + leftSeconds * 1000).toISOString(),
  }));
  edit?.(payload);
  return liveItem({ ...it, payload });
}

test('contract: the card lists the variable with its value, source and expiry', async ({
  relaunch,
}) => {
  const it = withTimes(base(), 2880);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = card(page, it);
  await expect(c.locator('[data-testid="api-approval-kept"]')).toHaveText('Uses: ids');
  await expect(c.locator('[data-testid="api-approval-kept-expiring"]')).toHaveCount(0);
  const row = c.locator('[data-testid="api-approval-kept-ids"]');
  await expect(row).toContainText('ids');
  await expect(row).toContainText('List of 3');
  await expect(row.locator('[data-testid="kept-value-preview"]')).toContainText('12');
  await expect(row).toContainText('GET {{base_url}}/users');
  await expect(row.locator('[data-testid="kept-age"]')).toContainText('12');
  await expect(row.locator('[data-testid="kept-expiry"]')).toContainText('Expires in 47m');
  await expect(row.locator('[data-testid="kept-used-in"]')).toHaveText('Body');
});

test('contract: the reference is marked in the JSON body', async ({ relaunch }) => {
  const it = withTimes(base(), 2880);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const body = card(page, it).locator('[data-testid="api-approval-body-text"]');
  await expect(body.locator('.kira-ed-kept')).toHaveCount(1);
});

test('contract: a variable that expires before the card does is flagged', async ({ relaunch }) => {
  const it = withTimes(base(), 60);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  await expect(card(page, it).locator('[data-testid="api-approval-kept-expiring"]')).toHaveText(
    'Expires before you answer',
  );
});

test('contract: URL and header references are marked', async ({ relaunch }) => {
  const it = withTimes(base(), 2880, (p) => {
    const url = 'http://127.0.0.1:1/users/{{kept.ids}}?x=1';
    const header = 'Bearer {{kept.ids}}';
    Object.assign(p, {
      url,
      body: null,
      headers: [{ name: 'Authorization', value: header, masked: false }],
    });
    p.kept[0] = {
      ...p.kept[0],
      sites: [
        { field: 'url', index: 0, part: '', from: url.indexOf('{{'), to: url.indexOf('}}') + 2 },
        { field: 'header', index: 0, part: 'value', from: 7, to: header.length },
      ],
    };
  });
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const c = card(page, it);
  await expect(c.locator('[data-testid="api-approval-url"] mark')).toHaveText('{{kept.ids}}');
  await expect(c.locator('[data-testid="api-approval-headers"] mark')).toHaveText('{{kept.ids}}');
  await expect(c.locator('[data-testid="kept-used-in"]')).toHaveText(
    'Request URL, Header Authorization',
  );
});

test('contract: Manage kept variables opens the settings view', async ({ relaunch }) => {
  const it = withTimes(base(), 2880);
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  await card(page, it).locator('[data-testid="api-approval-kept-manage"]').click();
  await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();
  await expect(page.locator('[data-testid="settings-kept-retention"]')).toBeVisible();
});
