import type { ApprovalItem } from '@shared/domain/approvals';
import type { KeptList } from '@shared/domain/kept';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { calls } from '../support/ipcCalls';
import { IPC } from '../support/ipcChannels';
import { openKeptPane } from './support/settings';

// Contract e2e-kept-expiry. Backend half: dbmcpflow TestE2EKeptExpiry. A kept variable that expires before
// the card does is flagged on an API card and on a DB card; Clear all (the call that ends three waiting
// cards in the backend) asks first and sends the recorded call; once the list drops the cards, the dialog
// leaves and the pane is empty.

const SC = 'e2e-kept-expiry';
const items = (key: string) => contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`);

/** The recorded item with the kept variable's expiry `leftSeconds` from now, the card's own at 600 s. */
function withExpiry(it: ApprovalItem, leftSeconds: number, createdAt = Date.now()): ApprovalItem {
  const payload = structuredClone(it.payload) as { kept: Record<string, unknown>[] };
  payload.kept = payload.kept.map((k) => ({
    ...k,
    createdAt: new Date(Date.now() - 60_000).toISOString(),
    expiresAt: new Date(Date.now() + leftSeconds * 1000).toISOString(),
  }));
  return liveItem({ ...it, payload }, 600, createdAt);
}

function keptList(key: string): KeptList {
  const list = contract<KeptList>(SC, `KeptVarsService.List#${key}`);
  return {
    ...list,
    entries: list.entries.map((e) => ({
      ...e,
      createdAt: new Date(Date.now() - 60_000).toISOString(),
      expiresAt: new Date(Date.now() + 59 * 60_000).toISOString(),
    })),
  };
}

for (const [key, prefix, label] of [
  ['expiring', 'api', 'an API card'],
  ['expiringDb', 'db', 'a DB write card'],
] as const) {
  test(`contract: ${label} flags a variable that expires before the card`, async ({ relaunch }) => {
    const base = items(key)[0] as ApprovalItem;
    const { window: page } = await relaunch({});
    await pushApprovals(page, [withExpiry(base, 20)]);
    const c = page.locator(`[data-testid="approval-item-${base.id}"]`);
    await expect(c.locator(`[data-testid="${prefix}-approval-kept-expiring"]`)).toHaveText(
      'Expires before you answer',
    );
    await expect(c.locator(`[data-testid="${prefix}-approval-kept"]`)).toContainText('Uses kept:');
  });

  test(`contract: ${label} shows no flag when the variable outlives the card`, async ({
    relaunch,
  }) => {
    const base = items(key)[0] as ApprovalItem;
    const { window: page } = await relaunch({});
    await pushApprovals(page, [withExpiry(base, 3600)]);
    const c = page.locator(`[data-testid="approval-item-${base.id}"]`);
    await expect(c.locator(`[data-testid="${prefix}-approval-kept"]`)).toBeVisible();
    await expect(c.locator(`[data-testid="${prefix}-approval-kept-expiring"]`)).toHaveCount(0);
  });
}

test('contract: three cards wait on one variable, each names it, and the list dropping them closes the dialog', async ({
  relaunch,
}) => {
  const three = items('three');
  const now = Date.now();
  const cards = three.map((it, i) => withExpiry(it, 3600, now + i));
  const { window: page } = await relaunch({});
  await pushApprovals(page, [...cards].reverse());
  const dialog = page.locator('[data-testid="approvals-dialog"]');
  await expect(dialog.locator('[data-testid="approvals-count"]')).toHaveText('3');
  await expect(dialog.locator('[data-testid="approvals-position"]')).toHaveText('1 of 3');
  const first = cards[0] as ApprovalItem;
  await expect(dialog.locator(`[data-testid="approval-item-${first.id}"]`)).toBeVisible();
  await expect(dialog.locator('[data-testid="api-approval-kept"]')).toHaveText('Uses kept: ids');
  // The backend withdrew all three; the pushed list is empty and no card is left.
  await pushApprovals(page, []);
  await expect(dialog).toHaveCount(0);
});

test('contract: Clear all asks first, then sends the call that ends the waiting cards', async ({
  relaunch,
}) => {
  const list = keptList('waiting');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.keptList, response: list },
      { channel: IPC.keptClearAll, response: list.entries.length },
    ],
  });
  await openKeptPane(page);
  await expect(page.locator('[data-testid="settings-kept-entry"]')).toHaveCount(1);
  await page.click('[data-testid="settings-kept-clear-all"]');
  expect(calls(control, IPC.keptClearAll)).toHaveLength(0);
  control.respond(IPC.keptList, keptList('afterClear'));
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect.poll(() => calls(control, IPC.keptClearAll)).toHaveLength(1);
  expect(calls(control, IPC.keptClearAll)[0]?.args).toBeUndefined();
  await expect(page.locator('[data-testid="settings-kept-empty"]')).toBeVisible();
});

test('contract: after the clear the pane is empty', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.keptList, response: keptList('afterClear') }],
  });
  await openKeptPane(page);
  await expect(page.locator('[data-testid="settings-kept-empty"]')).toBeVisible();
  await expect(page.locator('[data-testid="settings-kept-clear-all"]')).toHaveCount(0);
});

test('contract: the retention setting is sent as the recorded patch', async ({ relaunch }) => {
  const set = contract<unknown>(SC, 'args:SettingsService.Set#retention');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.keptList, response: keptList('afterClear') },
      {
        channel: IPC.settingsSet,
        response: {
          ...defaultSettings,
          dbMcp: { ...defaultSettings.dbMcp, keptRetentionMinutes: 5 },
        },
      },
    ],
  });
  await openKeptPane(page);
  const field = page.locator('[data-testid="settings-kept-retention"]');
  await field.fill('5');
  await field.blur();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual(set);
});
