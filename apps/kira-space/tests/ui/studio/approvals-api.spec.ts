import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
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
  await expect(c.locator('[data-testid="approval-operation"]')).toHaveText('GET');
  await expect(page.locator('[data-testid="approval-origin"]')).toContainText(it.origin.client);
  await expect(c.locator('[data-testid="api-approval-url"]')).toContainText('/echo');
  await expect(c.locator('[data-testid="approval-target-kind"]')).toHaveText('Dev');
  // A read sends its credential without a warning.
  await expect(c.locator('[data-testid="api-approval-credential"]')).toHaveCount(0);
  await expect(c.locator('[data-testid="api-approval-headers"]')).toContainText('Bearer {{token}}');
  await expect(c.locator('[data-testid="api-approval-variables"]')).toContainText('••••');
  await expect(c.locator('[data-testid="api-approval-body-text"]')).toHaveCount(0);
});

test('contract: a JSON body is pretty-printed and a credential on a write is flagged', async ({
  relaunch,
}) => {
  const it = item('item#json');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  await expect(card(page, it).locator('[data-testid="api-approval-credential"]')).toHaveText(
    'Credential sent',
  );
  const body = card(page, it).locator('[data-testid="api-approval-body-text"]');
  await expect.poll(() => editorText(body)).toContain('"name": "kira"');
  await expect.poll(() => editorText(body)).toContain('"tags": [');
});

test('contract: a form body renders as a table', async ({ relaunch }) => {
  const it = item('item#form');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const form = card(page, it).locator('[data-testid="api-approval-form"]');
  await expect(form).toContainText('grant_type');
  await expect(form).toContainText('client_credentials');
});

test('contract: a DELETE reads as danger', async ({ relaunch }) => {
  const it = item('item#delete');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const op = card(page, it).locator('[data-testid="approval-operation"]');
  await expect(op).toHaveText('DELETE');
  await expect(op).toHaveClass(/text-error/);
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
