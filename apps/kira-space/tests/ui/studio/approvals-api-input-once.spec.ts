import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { expectNoLeak } from './support/e2eLeak';

// Contract e2e-user-value-once. Backend half: dbmcpflow TestE2EUserValueOnce. A typed value is used for
// one request or kept: the card shows one row per value (label, masked field, the choice), starts on
// the agent's suggestion, and the answer carries the choice with the value.

const SC = 'e2e-user-value-once';
type Answer = {
  ids: string[];
  approve: boolean;
  options: Record<string, { inputs: Record<string, string>; inputModes: Record<string, string> }>;
};
const items = (key: string) => contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`);
const answerOf = (key: string) => contract<Answer>(SC, `args:ApprovalsService.Answer#${key}`);
const card = (page: Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-item-${it.id}"]`);
const approveOf = (page: Page, it: ApprovalItem) =>
  page.locator(`[data-testid="approval-approve-${it.id}"]`);
const valuesOf = (a: Answer) => Object.values(a.options)[0]?.inputs ?? {};
const field = (page: Page, name: string) => page.locator(`[data-testid="api-input-${name}"]`);
const choice = (page: Page, name: string, mode: 'once' | 'keep') =>
  page.locator(`[data-testid="api-input-${name}-${mode}"]`);

/** A stored-input recording with real kept times. */
function liveStored(it: ApprovalItem) {
  const payload = structuredClone(it.payload) as { kept: Record<string, unknown>[] };
  payload.kept = payload.kept.map((k) => ({
    ...k,
    createdAt: new Date(Date.now() - 5 * 60_000).toISOString(),
    expiresAt: new Date(Date.now() + 50 * 60_000).toISOString(),
  }));
  return liveItem({ ...it, payload });
}

const answerControl = (args: Answer) => [
  { channel: IPC.approvalsAnswer, response: answeredOk(args.ids) },
];

test('contract: the card starts on Use once, one row with no second explanation, and the answer carries it', async ({
  relaunch,
}) => {
  const it = items('once')[0] as ApprovalItem;
  const answer = answerOf('once');
  const { window: page, control } = await relaunch({
    control: answerControl(answer),
  });
  const logs: string[] = [];
  page.on('console', (m) => logs.push(m.text()));
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  const row = c.locator('[data-testid="api-approval-input-otp"]');
  await expect(row).toContainText('One-time code');
  await expect(choice(page, 'otp', 'once')).toHaveAttribute('data-state', 'on');
  await expect(choice(page, 'otp', 'keep')).toHaveText('Keep 60 min');
  // The badge names the value once; nothing else on the card repeats what the choice means.
  await expect(c.getByText('Sends: otp')).toBeVisible();
  await expect(c.getByText('Needs a value from you')).toHaveCount(0);
  await expect(c.getByText('The agent never sees')).toHaveCount(0);
  await expect(c.getByText('then wiped')).toHaveCount(0);
  await expect(approveOf(page, it)).toBeDisabled();
  await field(page, 'otp').fill(valuesOf(answer).otp ?? '');
  await expect(approveOf(page, it)).toBeEnabled();
  await approveOf(page, it).click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer);
  await expect(field(page, 'otp')).toHaveValue('');
  await expectNoLeak(page, valuesOf(answer).otp ?? '', logs);
});

test('contract: picking Keep switches the answer to keep', async ({ relaunch }) => {
  const it = items('once')[0] as ApprovalItem;
  const answer = answerOf('once');
  const { window: page, control } = await relaunch({
    control: answerControl(answer),
  });
  await pushApprovals(page, [liveItem(it)]);
  await field(page, 'otp').fill(valuesOf(answer).otp ?? '');
  await choice(page, 'otp', 'keep').click();
  await expect(choice(page, 'otp', 'keep')).toHaveAttribute('data-state', 'on');
  await expect(choice(page, 'otp', 'once')).toHaveAttribute('data-state', 'off');
  await approveOf(page, it).click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual({
      ...answer,
      options: { [it.id]: { inputs: valuesOf(answer), inputModes: { otp: 'keep' } } },
    });
});

test('contract: a later request asks the one-time value again', async ({ relaunch }) => {
  const it = items('again')[0] as ApprovalItem;
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  await expect(card(page, it).locator('[data-testid="api-approval-input-otp"]')).toBeVisible();
  await expect(choice(page, 'otp', 'once')).toHaveAttribute('data-state', 'on');
  await expect(approveOf(page, it)).toBeDisabled();
});

test('contract: several values, the agent suggests keeping one, the answer carries both choices', async ({
  relaunch,
}) => {
  const it = items('mixed')[0] as ApprovalItem;
  const answer = answerOf('mixed');
  const { window: page, control } = await relaunch({
    control: answerControl(answer),
  });
  await pushApprovals(page, [liveItem(it)]);
  const c = card(page, it);
  await expect(c.getByText('Sends: partner_key, otp')).toBeVisible();
  await expect(choice(page, 'partner_key', 'keep')).toHaveAttribute('data-state', 'on');
  await expect(choice(page, 'otp', 'once')).toHaveAttribute('data-state', 'on');
  const values = valuesOf(answer);
  await field(page, 'partner_key').fill(values.partner_key ?? '');
  await expect(approveOf(page, it)).toBeDisabled();
  await field(page, 'otp').fill(values.otp ?? '');
  await approveOf(page, it).click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer);
});

test('contract: a kept value is reused without a row; only the one-time value is asked', async ({
  relaunch,
}) => {
  const it = liveStored(items('later')[0] as ApprovalItem);
  const answer = answerOf('later');
  const { window: page, control } = await relaunch({
    control: answerControl(answer),
  });
  await pushApprovals(page, [it]);
  const c = card(page, it);
  await expect(c.locator('[data-testid="api-approval-input-partner_key"]')).toHaveCount(0);
  await expect(c.locator('[data-testid="api-approval-input-otp"]')).toBeVisible();
  await expect(
    c.locator('[data-testid="api-approval-kept-partner_key"] [data-testid="kept-supplied"]'),
  ).toHaveText('Supplied by you');
  await field(page, 'otp').fill(valuesOf(answer).otp ?? '');
  await approveOf(page, it).click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer);
});

test('contract: a held value is replaced with a one-time one', async ({ relaunch }) => {
  const it = liveStored(items('replace')[0] as ApprovalItem);
  const answer = answerOf('replace');
  const { window: page, control } = await relaunch({
    control: answerControl(answer),
  });
  await pushApprovals(page, [it]);
  await expect(approveOf(page, it)).toBeEnabled();
  await card(page, it).getByText('Enter a new value').click();
  await expect(choice(page, 'partner_key', 'once')).toHaveAttribute('data-state', 'on');
  await field(page, 'partner_key').fill(valuesOf(answer).partner_key ?? '');
  await approveOf(page, it).click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer);
});

test('contract: a script that read a one-time value is shown as keeping nothing', async ({
  relaunch,
}) => {
  const it = items('derived')[0] as ApprovalItem;
  const held = contract<{ text: string }>(SC, 'DbMcpService.PendingResponse#derived');
  const { window: page } = await relaunch({
    control: [{ channel: IPC.dbMcpPendingResponse, response: held }],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator('[data-testid="api-review-keep-derived-blocked"]')).toContainText(
    'Not kept',
  );
  await expect(page.locator('[data-testid="api-review-keep-derived"]')).not.toContainText(
    'preview',
  );
});
