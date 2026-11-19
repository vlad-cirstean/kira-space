import { defaultSettings } from '../../frontend/src/state/settingsDomain';
import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// Settings stored by v0.3.18 still carry the removed memory module's leaves. Backend half:
// appflow TestSettingsLegacyMemoryKeys.

const legacy = {
  ...defaultSettings,
  memory: { models: { gate: 'sonnet' } },
  experimental: { memory: true },
};

test('legacy memory settings keys load, show no pane and never reach a save', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.settingsGetAll, response: legacy }],
  });
  await expect(page.locator('[data-testid="mode-tab"][data-mode="memory"]')).toHaveCount(0);
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await expect(page.locator('[data-testid="settings-dialog"]')).toBeVisible();
  await expect(page.locator('[data-testid="settings-section-Memory"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="settings-section-Experimental"]')).toHaveCount(0);

  await page.locator('[data-testid="settings-section-Agent"]').click();
  await page.locator('[data-testid="settings-agent-hermetic"]').click();
  await page.locator('[data-testid="settings-save"]').click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual({ patch: { agent: { hermetic: true } } });
});
