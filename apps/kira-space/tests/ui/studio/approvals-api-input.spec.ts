import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';

// Contracts approvals-api-input and approvals-api-input-stored. Backend half: dbmcpflow
// TestUserInputSupplied. An api_request that declares an input parks with a field for it: Send waits
// until it is filled, the typed value rides the approve answer only, and the page keeps no copy.
// A value Kira already holds shows as supplied by the user and may be replaced.

const TYPED = 'typed-secret-value-42';
const item = () =>
  contract<ApprovalItem[]>('approvals-api-input', 'ApprovalsService.List')[0] as ApprovalItem;
const card = (page: Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-item-${it.id}"]`);

/** The stored-input recording with real kept times and a masked-free state. */
function storedItem() {
  const it = contract<ApprovalItem[]>('approvals-api-input-stored', 'ApprovalsService.List')[0];
  if (!it) throw new Error('contract has no stored item');
  const payload = structuredClone(it.payload) as { kept: Record<string, unknown>[] };
  payload.kept = payload.kept.map((k) => ({
    ...k,
    createdAt: new Date(Date.now() - 5 * 60_000).toISOString(),
    expiresAt: new Date(Date.now() + 50 * 60_000).toISOString(),
  }));
  return liveItem({ ...it, payload });
}

test('contract: the card asks for the value, marks it and holds Send until it is typed', async ({
  relaunch,
}) => {
  const it = item();
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  // One row: label, masked field. No heading, hint or warning repeats it.
  const row = c.locator('[data-testid="api-approval-input-apikey"]');
  await expect(row).toContainText('apikey');
  await expect(c.locator('[data-testid="api-approval-inputs"]')).not.toContainText(
    'Values you supply',
  );
  await expect(c.getByText('Needs a value from you')).toHaveCount(0);
  await expect(c.locator('[data-testid="api-approval-inputs-kept-for"]')).toHaveText(
    'Available for 60 min',
  );
  const field = page.locator('[data-testid="api-input-apikey"]');
  await expect(field).toHaveAttribute('type', 'password');
  await expect(field).toHaveAttribute('placeholder', 'Required');
  await expect(page.locator(`[data-testid="approval-approve-${it.id}"]`)).toBeDisabled();
  await field.fill('   ');
  await expect(field).toHaveAttribute('aria-invalid', 'true');
  await expect(page.locator(`[data-testid="approval-approve-${it.id}"]`)).toBeDisabled();
  await field.fill(TYPED);
  await expect(page.locator(`[data-testid="approval-approve-${it.id}"]`)).toBeEnabled();
  // The reference is marked in the header, and no pending row lists a value that does not exist yet.
  await expect(c.locator('[data-testid="api-approval-headers"] mark')).toHaveText(
    '{{secret.apikey}}',
  );
  await expect(c.locator('[data-testid="api-approval-kept-panel"]')).toHaveCount(0);
});

test('contract: Send carries the typed value with the answer; the page keeps no copy', async ({
  relaunch,
}) => {
  const it = item();
  const answer = contract<{ ids: string[]; options: Record<string, unknown> }>(
    'approvals-api-input',
    'args:ApprovalsService.Answer',
  );
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });
  const logs: string[] = [];
  page.on('console', (m) => logs.push(m.text()));
  await pushApprovals(page, [liveItem(it)]);
  await page.locator('[data-testid="api-input-apikey"]').fill(TYPED);
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: answer.ids, approve: true, options: answer.options });
  await expect(page.locator('[data-testid="api-input-apikey"]')).toHaveValue('');
  await expect(page.locator('body')).not.toContainText(TYPED);
  expect(logs.filter((l) => l.includes(TYPED))).toEqual([]);
});

test('contract: Don’t send never carries the value', async ({ relaunch }) => {
  const it = item();
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([it.id]) }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await page.locator('[data-testid="api-input-apikey"]').fill(TYPED);
  await page.click(`[data-testid="approval-deny-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({ ids: [it.id], approve: false, options: null });
});

test('contract: a held value shows as supplied by you, Send needs no typing, a new value replaces', async ({
  relaunch,
}) => {
  const it = storedItem();
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk([it.id]) }],
  });
  await pushApprovals(page, [it]);
  const c = card(page, it);
  const row = c.locator('[data-testid="api-approval-kept-apikey"]');
  await row.locator('[data-testid="kept-name"]').hover();
  await expect(page.locator('[data-testid="kept-details"]')).toContainText('Typed by you');
  await expect(row.locator('[data-testid="kept-value-secret"]')).toBeVisible();
  await expect(page.locator(`[data-testid="approval-approve-${it.id}"]`)).toBeEnabled();
  await c
    .locator('[data-testid="api-approval-input-replace"] [data-slot="collapsible-trigger"]')
    .click();
  await page.locator('[data-testid="api-input-apikey"]').fill(TYPED);
  await page.click(`[data-testid="approval-approve-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({
      ids: [it.id],
      approve: true,
      options: { [it.id]: { inputs: { apikey: TYPED } } },
    });
});
