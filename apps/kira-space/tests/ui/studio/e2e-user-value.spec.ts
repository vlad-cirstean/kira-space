import type { ApprovalItem } from '@shared/domain/approvals';
import type { KeptList } from '@shared/domain/kept';
import { answeredOk, liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { expectNoLeak } from './support/e2eLeak';
import { openKeptPane } from './support/settings';

// Contract e2e-user-value. Backend half: dbmcpflow TestE2EUserValue. A request names an input the
// user must type: the card asks, Send waits for it, the value rides the approve answer only and shows
// nowhere afterwards; Don't send never carries it; the pane lists it as supplied by the user and
// Clear removes it.

const SC = 'e2e-user-value';
type Answer = {
  ids: string[];
  approve: boolean;
  options: Record<string, { inputs: Record<string, string>; inputModes: Record<string, string> }>;
};
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;
const typed = () => contract<Answer>(SC, 'args:ApprovalsService.Answer#typed');
const VALUE = () => Object.values(typed().options)[0]?.inputs.apikey ?? '';

test('contract: the card asks for the value, Send needs it, the answer carries it and nothing keeps it', async ({
  relaunch,
}) => {
  const it = item('needed');
  const answer = typed();
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(answer.ids) }],
  });
  const logs: string[] = [];
  page.on('console', (m) => logs.push(m.text()));
  await pushApprovals(page, [liveItem(it)]);
  const approve = page.locator(`[data-testid="approval-approve-${it.id}"]`);
  await expect(approve).toBeDisabled();
  const field = page.locator('[data-testid="api-input-apikey"]');
  await expect(field).toHaveAttribute('type', 'password');
  await field.fill(VALUE());
  // The recorded answer keeps the value: the card starts on "Use once".
  await page.click('[data-testid="api-input-apikey-keep"]');
  await expect(approve).toBeEnabled();
  await approve.click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(answer);
  // The typed value clears when the answer settles, which is after the IPC call leaves.
  await expect(field).toHaveValue('');
  await expectNoLeak(page, VALUE(), logs);
});

test('contract: Don’t send never carries the value', async ({ relaunch }) => {
  const it = item('needed');
  const deny = contract<Answer>(SC, 'args:ApprovalsService.Answer#deny');
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.approvalsAnswer, response: answeredOk(deny.ids) }],
  });
  const logs: string[] = [];
  page.on('console', (m) => logs.push(m.text()));
  await pushApprovals(page, [liveItem(it)]);
  const field = page.locator('[data-testid="api-input-apikey"]');
  await field.fill(VALUE());
  await page.click(`[data-testid="approval-deny-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(deny);
  await expect(field).toHaveValue('');
  await expectNoLeak(page, VALUE(), logs);
});

test('contract: a held value shows as supplied by you and Send needs no typing', async ({
  relaunch,
}) => {
  const base = item('stored');
  const payload = structuredClone(base.payload) as { kept: Record<string, unknown>[] };
  payload.kept = payload.kept.map((k) => ({
    ...k,
    createdAt: new Date(Date.now() - 5 * 60_000).toISOString(),
    expiresAt: new Date(Date.now() + 50 * 60_000).toISOString(),
  }));
  const it = liveItem({ ...base, payload });
  const { window: page } = await relaunch({});
  await pushApprovals(page, [it]);
  const row = page.locator('[data-testid="api-approval-kept-apikey"]');
  await expect(row.locator('[data-testid="kept-supplied"]')).toHaveText('Supplied by you');
  await expect(row.locator('[data-testid="kept-value-secret"]')).toBeVisible();
  await expect(page.locator(`[data-testid="approval-approve-${it.id}"]`)).toBeEnabled();
});

test('contract: a forced review shows the script result and Withhold answers deny', async ({
  relaunch,
}) => {
  const it = item('forced');
  const h = contract<{ text: string }>(SC, 'DbMcpService.PendingResponse#forced');
  const withhold = contract<Answer>(SC, 'args:ApprovalsService.Answer#withhold');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.dbMcpPendingResponse, response: h },
      { channel: IPC.approvalsAnswer, response: answeredOk(withhold.ids) },
    ],
  });
  await pushApprovals(page, [liveItem(it)]);
  await expect(page.locator('[data-testid="api-review-result"]')).toContainText('{{kept.apikey}}|');
  await page.click(`[data-testid="approval-deny-${it.id}"]`);
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.approvalsAnswer)?.args)
    .toEqual(withhold);
});

test('contract: the pane lists the value as supplied by you with its origin and Clear removes it', async ({
  relaunch,
}) => {
  const l = contract<KeptList>(SC, 'KeptVarsService.List#held');
  const live: KeptList = {
    ...l,
    entries: l.entries.map((e) => ({
      ...e,
      createdAt: new Date(Date.now() - 60_000).toISOString(),
      expiresAt: new Date(Date.now() + 59 * 60_000).toISOString(),
    })),
  };
  const clear = contract<{ ids: string[] }>(SC, 'args:KeptVarsService.Clear#cleared');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.keptList, response: live },
      { channel: IPC.keptClear, args: clear, response: 1 },
    ],
  });
  await openKeptPane(page);
  const row = page.locator('[data-testid="settings-kept-entry"]');
  await expect(row.locator('[data-testid="kept-entry-supplied"]')).toHaveText('Supplied by you');
  await expect(row.locator('[data-testid="kept-entry-origin"]')).toContainText('http://127.0.0.1:');
  await expect(row.locator('[data-testid="kept-value-secret"]')).toBeVisible();
  await row.locator('[data-testid="kept-entry-clear"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.keptClear)?.args)
    .toEqual(clear);
});
