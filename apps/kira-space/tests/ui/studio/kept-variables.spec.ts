import type { KeptList, KeptValue } from '@shared/domain/kept';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openSettings } from './support/settings';

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

async function open(page: import('@playwright/test').Page) {
  await openSettings(page);
  await page.click('[data-testid="settings-section-Studio MCP variables"]');
}

test('contract: the pane lists each variable with its type, scope, source and time left', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.keptList, response: liveList() }],
  });
  await open(page);
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
  await open(page);
  const token = entry(page, 'token');
  await expect(token.locator('[data-testid="kept-value-secret"]')).toHaveText('••••');
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
  await open(page);
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
  await open(page);
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
  await open(page);
  await page.click('[data-testid="settings-kept-clear-all"]');
  expect(control.log().some((e) => e.channel === IPC.keptClearAll)).toBe(false);
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect.poll(() => control.log().some((e) => e.channel === IPC.keptClearAll)).toBe(true);
});

test('contract: nothing kept shows the empty state', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await open(page);
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
  await open(page);
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
