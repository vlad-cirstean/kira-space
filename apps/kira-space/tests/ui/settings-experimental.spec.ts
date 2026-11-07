import { defaultSettings } from '../../frontend/src/state/settingsDomain';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// P556: Memory sits behind Settings > Experimental. Backend half: appflow TestExperimentalMemory
// (contract settings-experimental).

const MEMORY_TAB = '[data-testid="mode-tab"][data-mode="memory"]';
const MEMORY_SECTION = '[data-testid="settings-section-Memory"]';
const FLAG = '[data-testid="settings-experimental-memory"]';

const off = contract<{ memory: boolean }>(
  'settings-experimental',
  'SettingsService.GetAll#experimental-default',
);
const on = contract<{ memory: boolean }>(
  'settings-experimental',
  'SettingsService.GetAll#experimental-on',
);
const patch = contract<{ experimental: { memory: boolean } }>(
  'settings-experimental',
  'SettingsService.Set#experimental-memory',
);

test('contract: Memory is hidden by default and the Experimental switch shows it', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.settingsGetAll, response: { ...defaultSettings, experimental: off } },
      { channel: IPC.settingsSet, response: { ...defaultSettings, experimental: on } },
    ],
  });
  await expect(page.locator('[data-testid="mode-tab"][data-mode="git"]')).toBeVisible();
  await expect(page.locator(MEMORY_TAB)).toHaveCount(0);

  await emitWailsEvent(page, IPC.openSettings, undefined);
  await expect(page.locator(MEMORY_SECTION)).toHaveCount(0);
  await page.locator('[data-testid="settings-section-Experimental"]').click();
  await expect(page.locator(FLAG)).toHaveAttribute('data-state', 'unchecked');
  // Memory's pane stays unmounted while off: its queries would open memory.db.
  expect(control.log().some((e) => e.channel.startsWith('kira:memory:'))).toBe(false);

  await page.locator(FLAG).click();
  await page.locator('[data-testid="settings-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual({ patch });
  await expect(page.locator(MEMORY_TAB)).toBeVisible();

  await emitWailsEvent(page, IPC.openSettings, undefined);
  await expect(page.locator(MEMORY_SECTION)).toBeVisible();
});

test('a window persisted on Memory falls back to Git while the flag is off', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.settingsGetAll, response: { ...defaultSettings, experimental: off } },
      { channel: IPC.windowsEnsure, response: { mode: 'memory' } },
    ],
  });
  await expect(page.locator('[data-testid="mode-tab"][data-mode="git"]')).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(page.locator('[data-testid="memory-panel"]')).toHaveCount(0);
});
