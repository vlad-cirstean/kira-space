import type { Page } from '@playwright/test';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openSettings } from './support/settings';

type Rule = { effect: 'allow' | 'deny'; hosts: string[]; methods: string[] };
type Rules = { patch: { dbMcp: { apiRules: Rule[] } } };
type Default = { patch: { dbMcp: { apiDefault: 'ask' | 'deny' } } };

const base = { ...defaultSettings.dbMcp, apiRequests: true };
const id = (name: string) => `[data-testid="${name}"]`;

async function openPane(page: Page) {
  await openSettings(page);
  await page.click(id('settings-section-Studio MCP API'));
}

// Contract dbmcp-api-default. Backend half: dbmcpflow TestApiAccessDefault, which sets the same rules and
// defaults in the same order and asserts which requests ask, run or are refused.
test('contract: default select sends the settings the backend enforces', async ({ relaunch }) => {
  const rules = contract<Rules>('dbmcp-api-default', 'args:SettingsService.Set#rules');
  const deny = contract<Default>('dbmcp-api-default', 'args:SettingsService.Set#deny');
  const ask = contract<Default>('dbmcp-api-default', 'args:SettingsService.Set#ask');
  const withRules = { ...base, apiRules: rules.patch.dbMcp.apiRules };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.settingsGetAll, response: { ...defaultSettings, dbMcp: withRules } },
      {
        channel: IPC.settingsSet,
        args: deny,
        response: { ...defaultSettings, dbMcp: { ...withRules, apiDefault: 'deny' } },
      },
      {
        channel: IPC.settingsSet,
        args: ask,
        response: { ...defaultSettings, dbMcp: { ...withRules, apiDefault: 'ask' } },
      },
    ],
  });
  const lastSet = () =>
    control
      .log()
      .filter((e) => e.channel === IPC.settingsSet)
      .at(-1)?.args;

  await openPane(page);
  await expect(page.locator(id('settings-db-mcp-api-default'))).toHaveValue('ask');
  await expect(page.locator(id('settings-db-mcp-api-default-ask'))).toBeVisible();
  await expect(page.locator(id('settings-db-mcp-api-rules-no-allow'))).toBeHidden();

  await page.selectOption(id('settings-db-mcp-api-default'), 'deny');
  await expect.poll(lastSet).toEqual(deny);
  await expect(page.locator(id('settings-db-mcp-api-default-deny'))).toBeVisible();
  await expect(page.locator(id('settings-db-mcp-api-rules-no-allow'))).toBeVisible();

  await page.selectOption(id('settings-db-mcp-api-default'), 'ask');
  await expect.poll(lastSet).toEqual(ask);
  await expect(page.locator(id('settings-db-mcp-api-default-ask'))).toBeVisible();
  await expect(page.locator(id('settings-db-mcp-api-rules-no-allow'))).toBeHidden();
});

test('ask with approval off says requests are sent without asking', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.settingsGetAll,
        response: {
          ...defaultSettings,
          dbMcp: { ...base, apiApproval: 'never', apiDefault: 'ask' },
        },
      },
    ],
  });
  await openPane(page);
  await expect(page.locator(id('settings-db-mcp-api-default-ask-never'))).toBeVisible();
  await expect(page.locator(id('settings-db-mcp-api-default-ask'))).toBeHidden();
});

test('allow rule for any host warns that it skips approval', async ({ relaunch }) => {
  const anyHost: Rule[] = [{ effect: 'allow', hosts: ['*'], methods: ['GET'] }];
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.settingsGetAll,
        response: { ...defaultSettings, dbMcp: { ...base, apiRules: anyHost } },
      },
    ],
  });
  await openPane(page);
  await expect(page.locator(id('settings-db-mcp-api-rules-allow-any'))).toBeVisible();
});

test('allow rule for one host shows no skip-approval warning, and the rule dialog explains itself', async ({
  relaunch,
}) => {
  const rules = contract<Rules>('dbmcp-api-default', 'args:SettingsService.Set#allow');
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.settingsGetAll,
        response: { ...defaultSettings, dbMcp: { ...base, apiRules: rules.patch.dbMcp.apiRules } },
      },
    ],
  });
  await openPane(page);
  await expect(page.locator(id('settings-db-mcp-api-rules-allow-any'))).toBeHidden();

  await page.click(id('settings-db-mcp-api-rules-add'));
  await expect(page.locator(id('api-rule-effect-hint'))).toBeVisible();
  const forms = page.locator(id('api-rule-host-forms'));
  await expect(forms).toBeVisible();
  await expect(forms).toContainText('localhost:3000');
  await expect(forms).toContainText('No port means any port');
});
