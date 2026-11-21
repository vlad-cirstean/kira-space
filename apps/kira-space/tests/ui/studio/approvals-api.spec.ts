import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import {
  editorOverflows,
  HEX,
  longQueryItem,
  richApi,
  scrollBodyToEnd,
} from './support/approvalItems';
import { editorText } from './support/editorText';

// Contract approvals-api. Backend half: dbmcpflow TestApiRequestApproved and TestApiRequestBodies. An
// agent's api_request parks in the approvals dialog as an api.request item: the method and URL, the
// environment and host, the credential it uses, and every part of the request as sent, with the
// credential still {{name}} and a secret variable masked.

const item = (key = 'ApprovalsService.List') => {
  const found =
    key === 'ApprovalsService.List'
      ? contract<ApprovalItem[]>('approvals-api', key)[0]
      : contract<ApprovalItem>('approvals-api', key);
  if (!found) throw new Error(`contract has no ${key}`);
  return found;
};
const card = (page: import('@playwright/test').Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-item-${it.id}"]`);

test('contract: a request shows the target, the credential and the masked variables', async ({
  relaunch,
}) => {
  const it = item();
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  await expect(c.locator('[data-testid="approval-target-name"]')).toHaveText('127.0.0.1');
  await expect(c.locator('[data-testid="approval-operation"]')).toHaveText('Read');
  await expect(c.locator('[data-testid="api-approval-url"]')).toContainText('/echo');
  await expect(c.locator('[data-testid="approval-target-kind"]')).toHaveText('Dev');
  // A read names the credential it sends, in a neutral badge.
  const sends = c.locator('[data-testid="approval-fact"]');
  await expect(sends).toHaveText('Sends: token');
  await expect(sends).not.toHaveClass(/text-warn/);
  // Headers and variables are part of the request, visible without opening Details.
  const headers = c.locator('[data-testid="api-approval-headers"]');
  const variables = c.locator('[data-testid="api-approval-variables"]');
  await expect(headers).toContainText('Bearer {{token}}');
  await expect(variables).toContainText('••••');
  await expect(headers).not.toContainText('Secret');
  await expect(variables).not.toContainText('Secret');
  await expect(c.locator('[data-testid="api-approval-body-text"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="approval-origin"]')).toBeHidden();
  await page.click('[data-testid="approval-details-toggle"]');
  await expect(page.locator('[data-testid="approval-origin"]')).toContainText(it.origin.client);
});

test('contract: a JSON body is pretty-printed and a credential on a write is flagged', async ({
  relaunch,
}) => {
  const it = item('item#json');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  await expect(c.locator('[data-testid="approval-operation"]')).toHaveText('Write');
  await expect(c.locator('[data-testid="approval-fact"]')).toHaveText('Sends: token');
  await expect(c.locator('[data-testid="approval-fact"]')).toHaveClass(/text-warn/);
  const body = c.locator('[data-testid="api-approval-body-text"]');
  await expect.poll(() => editorText(body)).toContain('"name": "kira"');
  await expect.poll(() => editorText(body)).toContain('"tags": [');
  // The JSON body is syntax highlighted.
  await expect
    .poll(
      async () =>
        new Set(
          await body
            .locator('.monaco-editor .view-line span[class*="mtk"]')
            .evaluateAll((els) => els.map((e) => e.className)),
        ).size,
    )
    .toBeGreaterThan(1);
});

test('contract: two credentials read as one fact, hosts in its tooltip', async ({ relaunch }) => {
  const it = richApi('item#json');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const fact = card(page, it).locator('[data-testid="approval-fact"]');
  await expect(fact).toHaveText('Sends: token, partner_key');
  await fact.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    'api.shop.example',
  );
});

test('contract: the request box reads request line, body, then headers and variables', async ({
  relaunch,
}) => {
  const it = richApi('item#json');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const ids = [
    'api-approval-url',
    'api-approval-body-text',
    'api-approval-headers',
    'api-approval-variables',
  ];
  const tops: number[] = [];
  for (const id of ids) {
    const box = await card(page, it).locator(`[data-testid="${id}"]`).boundingBox();
    tops.push(box?.y ?? Number.NaN);
  }
  expect(tops.every((y) => !Number.isNaN(y))).toBe(true);
  expect([...tops].sort((a, b) => a - b)).toEqual(tops);
});

test('contract: the request line colours method, host, port, query and variables', async ({
  relaunch,
}) => {
  const get = richApi('ApprovalsService.List');
  const post = richApi('item#json');
  const del = richApi('item#delete');
  const { window: page } = await relaunch({});
  const method = (it: ApprovalItem) =>
    card(page, it).locator('[data-testid="api-approval-url"] [data-token="method"]');
  await pushApprovals(page, [get]);
  await expect(method(get)).toHaveClass(/text-conn-blue/);
  await expect(method(get)).toHaveAttribute('data-method', /get/i);
  await pushApprovals(page, [post]);
  await expect(method(post)).toHaveClass(/text-conn-green/);
  await pushApprovals(page, [del]);
  await expect(method(del)).toHaveClass(/text-conn-red/);

  const long = longQueryItem();
  await pushApprovals(page, [long]);
  const line = card(page, long).locator('[data-testid="api-approval-url"]');
  await expect(line.locator('[data-token="host"]').first()).toHaveClass(/font-semibold/);
  await expect(line.locator('[data-token="port"]')).toHaveText(':8080');
  await expect(line.locator('[data-token="port"]')).toHaveClass(/font-semibold/);
  await expect(line.locator('[data-token="key"]').first()).toHaveClass(/text-syntax-property/);
  await expect(line.locator('[data-token="value"]').first()).toHaveClass(/text-syntax-string/);
  await expect(line.locator('[data-token="var"]').first()).toHaveClass(/text-var-resolved/);
  await expect(line.locator('[data-token="sep"]')).toHaveText([
    '?',
    '=',
    '&',
    '=',
    '&',
    '=',
    '&',
    '=',
    '&',
    '=',
  ]);
  await expect(line.locator('[data-token="sep"]').first()).toHaveClass(/text-syntax-meta/);
  // One query pair per line: every pair starts at the same left edge.
  const lefts = await line
    .locator('[data-testid="api-approval-query-pair"]')
    .evaluateAll((els) => els.map((e) => Math.round(e.getBoundingClientRect().left)));
  expect(lefts).toHaveLength(5);
  expect(new Set(lefts).size).toBe(1);
  // The long hex value is cut at the line end; hover shows all of it.
  const hex = line.locator('[data-testid="api-approval-query-value"]').nth(1);
  expect(await hex.locator('.truncate').evaluate((el) => el.scrollWidth > el.clientWidth)).toBe(
    true,
  );
  await hex.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(HEX);
});

test('contract: no dialog button rests on a fill', async ({ relaunch }) => {
  const it = item();
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const approve = page.locator(`[data-testid="approval-approve-${it.id}"]`);
  const deny = page.locator(`[data-testid="approval-deny-${it.id}"]`);
  const bg = (el: import('@playwright/test').Locator) =>
    el.evaluate((e) => getComputedStyle(e).backgroundColor);
  expect(await bg(approve)).toBe('rgba(0, 0, 0, 0)');
  expect(await bg(deny)).toBe('rgba(0, 0, 0, 0)');
  await approve.hover();
  await expect.poll(() => bg(approve)).not.toBe('rgba(0, 0, 0, 0)');
});

test('contract: a long request scrolls the body and wraps its editors', async ({ relaunch }) => {
  const it = richApi('item#json');
  const p = it.payload as Record<string, unknown>;
  const obj = {
    tenant: 'acme-de',
    token: 'a'.repeat(600),
    items: Array.from({ length: 60 }, (_, i) => ({
      id: i,
      sku: `SKU-${i}`,
      note: 'n'.repeat(i % 5 === 0 ? 300 : 20),
    })),
  };
  const long = {
    ...it,
    payload: {
      ...p,
      url: `${String(p.url)}?filter=${'q'.repeat(300)}`,
      body: {
        contentType: 'application/json',
        text: JSON.stringify(obj),
        truncated: false,
        form: null,
      },
    },
  } as ApprovalItem;
  const { window: page } = await relaunch({});
  await page.setViewportSize({ width: 1280, height: 900 });
  await pushApprovals(page, [liveItem(long)]);
  const footer = page.locator('[data-testid="approvals-footer"]');
  await expect(footer).toHaveAttribute('data-more-below', 'true');
  await page.click('[data-testid="approval-details-toggle"]');
  const end = await scrollBodyToEnd(page);
  expect(end.scrollable).toBe(true);
  expect(end.scrollTop).toBeGreaterThan(0);
  await expect(footer).toHaveAttribute('data-more-below', 'false');
  for (const overflow of await editorOverflows(page)) expect(overflow).toBeLessThanOrEqual(1);
  const line = page.locator('[data-testid="api-approval-url"]');
  expect(await line.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  await expect(page.locator('[data-testid="approval-details-toggle"]')).toBeInViewport();
  await expect(page.locator('[data-testid="approval-origin"]')).toBeInViewport();
});

test('contract: a form body renders as a table', async ({ relaunch }) => {
  const it = item('item#form');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const form = card(page, it).locator('[data-testid="api-approval-form"]');
  await expect(form).toContainText('grant_type');
  await expect(form).toContainText('client_credentials');
});

test('contract: a DELETE reads as Delete in the red hue', async ({ relaunch }) => {
  const it = item('item#delete');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const op = card(page, it).locator('[data-testid="approval-operation"]');
  await expect(op).toHaveText('Delete');
  await expect(op).toHaveClass(/text-status-red/);
  await expect(page.locator(`[data-testid="approval-approve-${it.id}"]`)).toHaveAttribute(
    'data-variant',
    'dialog-approve',
  );
});

test('contract: Send answers the item id, Don’t send denies it', async ({ relaunch }) => {
  const it = item();
  const answer = contract<{ ids: string[] }>('approvals-api', 'args:ApprovalsService.Answer');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator(`[data-testid="approval-approve-${it.id}"]`)).toHaveText(/^Send\b/);
  await expect(page.locator(`[data-testid="approval-deny-${it.id}"]`)).toHaveText(/^Don’t send/);
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: true, options: null });
});

const reviewItem = (key: string) => {
  const found = contract<ApprovalItem[]>('approvals-api', `ApprovalsService.List#${key}`)[0];
  if (!found) throw new Error(`contract has no ${key}`);
  return found;
};

// Backend half: dbmcpflow TestApiResponseReviewOnRequest. The card offers "review the result"; a tick
// rides the answer as an option, and settings that already hold the response show it on and locked.
test('contract: the review option is offered, ticked and sent with the answer', async ({
  relaunch,
}) => {
  const it = reviewItem('reviewOffered');
  const answer = contract<{ ids: string[]; options: Record<string, unknown> }>(
    'approvals-api',
    'args:ApprovalsService.Answer#reviewOffered',
  );
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  const box = page.locator('[data-testid="api-approval-review-result"]');
  await expect(box).toHaveAttribute('data-state', 'unchecked');
  await box.click();
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: answer.ids, approve: true, options: answer.options });
});

test('contract: a review already on by settings shows checked and locked', async ({ relaunch }) => {
  const it = reviewItem('reviewOn');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const box = page.locator('[data-testid="api-approval-review-result"]');
  await expect(box).toHaveAttribute('data-state', 'checked');
  await expect(box).toBeDisabled();
});
