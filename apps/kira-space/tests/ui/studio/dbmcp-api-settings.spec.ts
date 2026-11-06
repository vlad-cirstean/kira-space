import { defaultSettings } from '../../../frontend/src/studio/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
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
        channel: IPC.studioSettingsSet,
        args: on,
        response: { ...defaultSettings, dbMcp: enabled },
      },
      {
        channel: IPC.studioSettingsSet,
        args: never,
        response: { ...defaultSettings, dbMcp: { ...enabled, apiApproval: 'never' } },
      },
    ],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await expect(page.locator('[data-testid="settings-db-mcp-api"]')).not.toBeChecked();
  await expect(page.locator('[data-testid="settings-db-mcp-api-approval"]')).toHaveCount(0);

  await page.click('[data-testid="settings-db-mcp-api"]');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.studioSettingsSet)?.args)
    .toEqual(on);
  const select = page.locator('[data-testid="settings-db-mcp-api-approval"]');
  await expect(select).toHaveValue('always');
  await expect(page.locator('[data-testid="settings-db-mcp-api-never-warning"]')).toHaveCount(0);

  await select.selectOption('never');
  await expect
    .poll(
      () =>
        control
          .log()
          .filter((e) => e.channel === IPC.studioSettingsSet)
          .at(-1)?.args,
    )
    .toEqual(never);
  await expect(page.locator('[data-testid="settings-db-mcp-api-never-warning"]')).toBeVisible();
});
