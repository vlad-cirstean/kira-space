import { expect, test } from './fixtures';
import { openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Update dots on task cards and branch rows, and the Main tab strip with its Mark seen buttons.
// Contract ade-tracked-updates; backend half: flows/adeflow TestTrackedUpdates (trackedupdates_test.go).
// The contract masks the ids; the spec pins them to the board fixture's task T_bill, branch b_bill.

const t = (id: string) => `[data-testid="${id}"]`;

interface Update {
  taskId: string;
  itemKey: string;
  kind: string;
  branchId: string;
  label: string;
  concepts: string[];
  counts: Record<string, number>;
  changedAt: number;
}
const fx = contract<{ updates: Update[] }>(
  'ade-tracked-updates',
  'AdeTaskService.TrackedUpdates#changed',
);
const crFx = fx.updates.find((u) => u.kind === 'changeRequest');
const issueFx = fx.updates.find((u) => u.kind === 'issue');
if (!crFx || !issueFx) throw new Error('contract lacks a change request or an issue update');

const cr: Update = { ...crFx, taskId: 'T_bill', itemKey: 'cr:b_bill', branchId: 'b_bill' };
const issue: Update = { ...issueFx, taskId: 'T_bill', itemKey: 'issue', branchId: '' };
const branch: Update = {
  taskId: 'T_auth',
  itemKey: 'branch:b_auth',
  kind: 'branch',
  branchId: 'b_auth',
  label: 'feat/oauth-login',
  concepts: ['remoteCommits'],
  counts: {},
  changedAt: 0,
};

test('contract: a dot marks the task and the branch, and the strip lists each change', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskTrackedUpdates, response: { updates: [cr, issue, branch] } },
  ]);
  await expect(
    page.locator(`${t('ade-card')}[data-task-id="T_bill"] ${t('ade-task-updated')}`),
  ).toBeVisible();
  await expect(
    page.locator(`${t('ade-card')}[data-task-id="T_cart"] ${t('ade-task-updated')}`),
  ).toHaveCount(0);

  await openTask(page, 'T_bill');
  const strip = page.locator(t('ade-task-updates'));
  await expect(strip).toBeVisible();
  await expect(strip.locator(t('ade-task-update-cr:b_bill'))).toContainText(
    '3 new request comments',
  );
  await expect(strip.locator(t('ade-task-update-cr:b_bill'))).toContainText(
    'Request checks failing',
  );
  await expect(strip.locator(t('ade-task-update-issue'))).toContainText('Issue status changed');
  await expect(page.locator(t('ade-branch-updated-b_bill'))).toBeVisible();
  await expect(page.locator(t('ade-branch-updated-b_billdash'))).toHaveCount(0);
});

// The mark call is held so the refetch it triggers does not bring the static reply back.
test('Mark seen sends the item key and clears its line', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskTrackedUpdates, response: { updates: [cr, issue] } },
    { channel: IPC.adeTaskMarkTrackedSeen, hold: true },
  ]);
  await openTask(page, 'T_bill');
  await page.locator(t('ade-task-update-seen-issue')).click();
  await expect.poll(() => calls(control, IPC.adeTaskMarkTrackedSeen)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskMarkTrackedSeen)[0]?.args).toEqual({
    taskId: 'T_bill',
    itemKeys: ['issue'],
  });
  await expect(page.locator(t('ade-task-update-issue'))).toHaveCount(0);
});

test('Mark all seen sends an empty key list', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskTrackedUpdates, response: { updates: [cr, issue] } },
    { channel: IPC.adeTaskMarkTrackedSeen, hold: true },
  ]);
  await openTask(page, 'T_bill');
  await page.locator(t('ade-task-updates-seen-all')).click();
  await expect.poll(() => calls(control, IPC.adeTaskMarkTrackedSeen)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskMarkTrackedSeen)[0]?.args).toEqual({
    taskId: 'T_bill',
    itemKeys: [],
  });
  await expect(page.locator(t('ade-task-updates'))).toHaveCount(0);
});

test('a concept the settings turned off shows no dot', async ({ relaunch }) => {
  const quiet: Update = { ...cr, concepts: ['titleChanged'], counts: {} };
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskTrackedUpdates, response: { updates: [quiet] } },
  ]);
  await expect(page.locator(t('ade-card'))).not.toHaveCount(0);
  await expect(page.locator(t('ade-task-updated'))).toHaveCount(0);
});

test('no updates: no dot and no strip', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_bill');
  await expect(page.locator(t('ade-task-updated'))).toHaveCount(0);
  await expect(page.locator(t('ade-task-updates'))).toHaveCount(0);
});
