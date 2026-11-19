import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openSettings } from './support/settings';

// Contract e2e-settings-api. Backend half: dbmcpflow TestE2ESettingsAPI. The four Studio MCP API
// switches (scripts, response review, tools) send the exact patches the flow applies, in the order
// the flow flips them, and each reads back checked or unchecked.

const SC = 'e2e-settings-api';
const patch = (key: string) => contract<unknown>(SC, `args:SettingsService.Set#${key}`);

test('contract: each API switch sends the patch the flow applies', async ({ relaunch }) => {
  const base = { ...defaultSettings.dbMcp, apiRequests: true };
  const reply = (over: Record<string, boolean>) => ({
    ...defaultSettings,
    dbMcp: { ...base, ...over },
  });
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.settingsGetAll, response: reply({}) },
      {
        channel: IPC.settingsSet,
        args: patch('scriptsOff'),
        response: reply({ apiScripts: false }),
      },
      { channel: IPC.settingsSet, args: patch('scriptsOn'), response: reply({ apiScripts: true }) },
      {
        channel: IPC.settingsSet,
        args: patch('reviewOn'),
        response: reply({ apiReviewResponses: true }),
      },
      {
        channel: IPC.settingsSet,
        args: patch('reviewOff'),
        response: reply({ apiReviewResponses: false }),
      },
      {
        channel: IPC.settingsSet,
        args: patch('toolsOff'),
        response: reply({ apiRequests: false }),
      },
    ],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Studio MCP API"]');
  const scripts = page.locator('[data-testid="settings-db-mcp-api-scripts"]');
  const review = page.locator('[data-testid="settings-db-mcp-api-review"]');
  const sent = () =>
    control
      .log()
      .filter((e) => e.channel === IPC.settingsSet)
      .map((e) => e.args);

  await scripts.click();
  await expect(scripts).not.toBeChecked();
  await scripts.click();
  await expect(scripts).toBeChecked();
  await review.click();
  await expect(review).toBeChecked();
  await review.click();
  await expect(review).not.toBeChecked();
  await page.click('[data-testid="settings-db-mcp-api"]');
  await expect(page.locator('[data-testid="settings-db-mcp-api"]')).not.toBeChecked();
  await expect
    .poll(sent)
    .toEqual([
      patch('scriptsOff'),
      patch('scriptsOn'),
      patch('reviewOn'),
      patch('reviewOff'),
      patch('toolsOff'),
    ]);
});
