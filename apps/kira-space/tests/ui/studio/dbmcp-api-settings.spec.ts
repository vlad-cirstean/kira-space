import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { expectOn } from '../support/toggleGroup';
import { openSettings } from './support/settings';

// Contract dbmcp-api-settings. Backend half: dbmcpflow TestApiToolsToggle and TestApiApprovalModes,
// which apply the same two patches: the API tools switch and the approval mode.
test('contract: the switch lists the tools, "never" shows its warning', async ({ relaunch }) => {
  const on = contract<unknown>('dbmcp-api-settings', 'args:SettingsService.Set#on');
  const never = contract<unknown>('dbmcp-api-settings', 'args:SettingsService.Set#approval');
  const enabled = { ...defaultSettings.dbMcp, apiRequests: true };
  const { window: page, control } = await relaunch({
    control: [
      {
        channel: IPC.settingsSet,
        args: on,
        response: { ...defaultSettings, dbMcp: enabled },
      },
      {
        channel: IPC.settingsSet,
        args: never,
        response: { ...defaultSettings, dbMcp: { ...enabled, apiApproval: 'never' } },
      },
    ],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Studio MCP API"]');
  await expect(page.locator('[data-testid="settings-db-mcp-api"]')).not.toBeChecked();
  await expect(page.locator('[data-testid="settings-db-mcp-api-approval"]')).toHaveCount(0);

  await page.click('[data-testid="settings-db-mcp-api"]');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual(on);
  await expectOn(page.locator('[data-testid="settings-db-mcp-api-approval-always"]'));
  await expect(page.locator('[data-testid="settings-db-mcp-api-never-warning"]')).toHaveCount(0);

  await page.click('[data-testid="settings-db-mcp-api-approval-never"]');
  await expect
    .poll(
      () =>
        control
          .log()
          .filter((e) => e.channel === IPC.settingsSet)
          .at(-1)?.args,
    )
    .toEqual(never);
  await expect(page.locator('[data-testid="settings-db-mcp-api-never-warning"]')).toBeVisible();
});

// Backend half: dbmcpflow TestApiScriptsToggle, the same patch.
test('contract: the scripts switch is on by default and patches apiScripts', async ({
  relaunch,
}) => {
  const off = contract<unknown>('dbmcp-api-settings', 'args:SettingsService.Set#scriptsOff');
  const enabled = { ...defaultSettings.dbMcp, apiRequests: true };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.settingsGetAll, response: { ...defaultSettings, dbMcp: enabled } },
      {
        channel: IPC.settingsSet,
        args: off,
        response: { ...defaultSettings, dbMcp: { ...enabled, apiScripts: false } },
      },
    ],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Studio MCP API"]');
  const scripts = page.locator('[data-testid="settings-db-mcp-api-scripts"]');
  await expect(scripts).toBeChecked();
  await scripts.click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual(off);
});
