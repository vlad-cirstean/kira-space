import { defaultSettings } from '../../../frontend/src/studio/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openSettings } from './support/settings';

// Contract dbmcp-focus. Backend half: dbmcpflow TestDbMcpApprovalFocus, which raises the window
// for each approval stage until this switch turns it off.
test('contract: the focus switch is on by default and sends the settings patch', async ({
  relaunch,
}) => {
  const args = contract<unknown>('dbmcp-focus', 'args:SettingsService.Set');
  const { window: page, control } = await relaunch({
    control: [
      {
        channel: IPC.studioSettingsSet,
        response: {
          ...defaultSettings,
          dbMcp: { ...defaultSettings.dbMcp, focusOnApproval: false },
        },
      },
    ],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await expect(page.locator('[data-testid="settings-db-mcp-focus"]')).toBeChecked();
  await page.click('[data-testid="settings-db-mcp-focus"]');
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.studioSettingsSet)?.args)
    .toEqual(args);
  await expect(page.locator('[data-testid="settings-db-mcp-focus"]')).not.toBeChecked();
});
