import type { KeptList, KeptValue } from '@shared/domain/kept';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openKeptPane } from './support/settings';

// Contract kept-vars. Backend half: dbmcpflow TestKeptVarsService and TestKeptVarsRetentionSetting. The
// Studio MCP variables pane lists live kept variables with value, source and time left, reads a whole
// value on demand, clears one or all, and sets how long new values live. A secret shows dots only.

function liveList(): KeptList {
  const list = contract<KeptList>('kept-vars', 'KeptVarsService.List');
  const now = Date.now();
  return {
    ...list,
    entries: list.entries.map((e, i) => ({
      ...e,
      createdAt: new Date(now - (i + 1) * 60_000).toISOString(),
      expiresAt: new Date(now + (60 - i) * 60_000).toISOString(),
    })),
  };
}
const idOf = (name: string) => {
  const e = liveList().entries.find((x) => x.name === name);
  if (!e) throw new Error(`contract has no ${name}`);
  return e.id;
};
const entry = (page: import('@playwright/test').Page, name: string) =>
  page.locator('[data-testid="settings-kept-entry"]').filter({
    has: page.locator('[data-testid="kept-entry-name"]', { hasText: new RegExp(`^${name}$`) }),
  });

test('contract: the pane lists each variable with its type, scope, source and time left', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.keptList, response: liveList() }],
  });
  await openKeptPane(page);
  await expect(page.locator('[data-testid="settings-kept-entry"]')).toHaveCount(3);
  const ids = entry(page, 'ids');
  await expect(ids).toContainText('List of 800');
  await expect(ids).toContainText('External clients');
  await expect(ids).toContainText('GET {{base_url}}/users');
  await expect(ids.locator('[data-testid="kept-entry-expiry"]')).toContainText('Expires in');
  await expect(entry(page, 'word').locator('[data-testid="kept-value-preview"]')).toHaveText(
    'hello',
  );
});

test('contract: a secret shows dots and no copy', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.keptList, response: liveList() }],
  });
  await openKeptPane(page);
  const token = entry(page, 'token');
  await expect(token.locator('[data-testid="kept-value-secret"]')).toHaveText('••••••••');
  await expect(token.locator('[data-testid="kept-value-copy"]')).toHaveCount(0);
  await expect(token.locator('[data-testid="kept-value-preview"]')).toHaveCount(0);
});

test('contract: Show all reads the whole value', async ({ relaunch }) => {
  const value = contract<KeptValue>('kept-vars', 'KeptVarsService.Value');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.keptList, response: liveList() },
      { channel: IPC.keptValue, response: value },
    ],
  });
  await openKeptPane(page);
  const ids = entry(page, 'ids');
  await expect(ids.locator('[data-testid="kept-value-full"]')).toHaveCount(0);
  await ids.getByText('Show all').click();
  await expect(ids.locator('[data-testid="kept-value-full"]')).toBeVisible();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.keptValue)?.args)
    .toEqual({ id: idOf('ids') });
});

test('contract: Clear removes one variable', async ({ relaunch }) => {
  const clear = contract<{ ids: string[] }>('kept-vars', 'args:KeptVarsService.Clear');
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.keptList, response: liveList() },
      { channel: IPC.keptClear, response: 1 },
    ],
  });
  await openKeptPane(page);
  await entry(page, 'word').locator('[data-testid="kept-entry-clear"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.keptClear)?.args)
    .toEqual(clear);
});

test('contract: Clear all asks first, then clears everything', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.keptList, response: liveList() },
      { channel: IPC.keptClearAll, response: 3 },
    ],
  });
  await openKeptPane(page);
  await page.click('[data-testid="settings-kept-clear-all"]');
  expect(control.log().some((e) => e.channel === IPC.keptClearAll)).toBe(false);
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect.poll(() => control.log().some((e) => e.channel === IPC.keptClearAll)).toBe(true);
});

test('contract: nothing kept shows the empty state', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await openKeptPane(page);
  await expect(page.locator('[data-testid="settings-kept-empty"]')).toContainText(
    'for up to 60 minutes',
  );
  await expect(page.locator('[data-testid="settings-kept-clear-all"]')).toHaveCount(0);
});

test('contract: retention patches the setting and refuses a value out of range', async ({
  relaunch,
}) => {
  const set = contract<unknown>('kept-vars', 'args:SettingsService.Set#retention');
  const { window: page, control } = await relaunch({
    control: [
      {
        channel: IPC.settingsSet,
        args: set,
        response: {
          ...defaultSettings,
          dbMcp: { ...defaultSettings.dbMcp, keptRetentionMinutes: 10 },
        },
      },
    ],
  });
  await openKeptPane(page);
  const field = page.locator('[data-testid="settings-kept-retention"]');
  await expect(field).toHaveValue('60');
  await field.fill('4');
  await expect(page.locator('[data-testid="settings-kept-retention-error"]')).toBeVisible();
  expect(control.log().some((e) => e.channel === IPC.settingsSet)).toBe(false);
  await field.fill('10');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual(set);
});

test('contract: a failed load shows the error with Retry, which reloads the list', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.keptList, error: { code: 'E_INTERNAL', message: 'memory store is busy' } },
    ],
  });
  await openKeptPane(page);
  await expect(page.locator('[data-testid="settings-kept-error"]')).toContainText(
    'memory store is busy',
  );
  control.respond(IPC.keptList, liveList());
  await page.locator('[data-testid="query-retry"]').click();
  await expect(page.locator('[data-testid="settings-kept-entry"]')).toHaveCount(3);
  await expect(page.locator('[data-testid="settings-kept-error"]')).toHaveCount(0);
});

test('contract: the retention reset button restores the default of 60 minutes', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      {
        channel: IPC.settingsSet,
        response: {
          ...defaultSettings,
          dbMcp: { ...defaultSettings.dbMcp, keptRetentionMinutes: 10 },
        },
      },
    ],
  });
  await openKeptPane(page);
  const reset = page.locator('[data-testid="settings-reset-dbMcp-keptRetentionMinutes"]');
  await expect(reset).toBeDisabled();
  await page.locator('[data-testid="settings-kept-retention"]').fill('10');
  await expect(reset).toBeEnabled();
  control.respond(IPC.settingsSet, defaultSettings);
  await reset.click();
  await expect
    .poll(
      () =>
        control
          .log()
          .filter((e) => e.channel === IPC.settingsSet)
          .at(-1)?.args,
    )
    .toEqual({ patch: { dbMcp: { keptRetentionMinutes: 60 } } });
  await expect(page.locator('[data-testid="settings-kept-retention"]')).toHaveValue('60');
});
