import type { Page } from '@playwright/test';
import { defaultSettings } from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { openSettings } from './support/settings';

type Rule = { effect: 'allow' | 'deny'; hosts: string[]; methods: string[] };
type Patch = { patch: { dbMcp: { apiRules: Rule[] } } };

const KEYS = ['allow', 'wildcard', 'deny', 'remove', 'removeWildcard', 'clear'] as const;
const dbMcp = { ...defaultSettings.dbMcp, apiRequests: true };
// The backend half denies unmatched requests until its #clear step, which sets the default back to ask.
const denying = { ...dbMcp, apiDefault: 'deny' as const };

const rows = (page: Page) => page.locator('[data-testid="settings-db-mcp-api-rule"]');
const empty = (page: Page) => page.locator('[data-testid="settings-db-mcp-api-rules-empty"]');

async function addRule(page: Page, host: string, effect: 'allow' | 'deny', methods: string[]) {
  await page.click('[data-testid="settings-db-mcp-api-rules-add"]');
  await page.selectOption('[data-testid="api-rule-effect"]', effect);
  const input = page.locator('[data-testid="api-rule-hosts"] input');
  await input.fill(host);
  await input.press('Enter');
  if (methods.includes('*')) await page.click('[data-testid="api-rule-any-method"]');
  else for (const m of methods) await page.click(`[data-testid="api-rule-method-${m}"]`);
  await page.click('[data-testid="api-rule-save"]');
  await expect(page.locator('[data-testid="api-rule-dialog"]')).toHaveCount(0);
}

// Contract dbmcp-api-rules. Backend half: dbmcpflow TestApiAccessRules, which applies the same six
// rule lists in the same order, with unmatched requests denied until the last one, and asserts the gate
// each one produces.
test('contract: add, edit and remove rules send the same lists the backend enforces', async ({
  relaunch,
}) => {
  const patches = KEYS.map((key) => ({
    key,
    args: contract<Patch>('dbmcp-api-rules', `args:SettingsService.Set#${key}`),
  }));
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.settingsGetAll, response: { ...defaultSettings, dbMcp: denying } },
      ...patches.map(({ key, args }) => ({
        channel: IPC.settingsSet,
        args,
        response: {
          ...defaultSettings,
          dbMcp: {
            ...(key === 'clear' ? dbMcp : denying),
            apiRules: args.patch.dbMcp.apiRules,
          },
        },
      })),
    ],
  });
  const lastSet = () =>
    control
      .log()
      .filter((e) => e.channel === IPC.settingsSet)
      .at(-1)?.args;
  const [allow, wildcard, deny, remove, removeWildcard, clear] = patches.map((p) => p.args);

  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await expect(empty(page)).toBeVisible();
  await expect(page.locator('[data-testid="settings-db-mcp-api-rules-no-allow"]')).toBeVisible();

  await addRule(page, '127.0.0.1', 'allow', ['GET']);
  await expect.poll(lastSet).toEqual(allow);
  await expect(rows(page)).toHaveCount(1);
  await expect(rows(page).first()).toContainText('Allow');
  await expect(rows(page).first()).toContainText('127.0.0.1');
  await expect(rows(page).first()).toContainText('GET');

  await addRule(page, '*.example.com', 'allow', ['*']);
  await expect.poll(lastSet).toEqual(wildcard);
  await expect(rows(page).nth(1)).toContainText('Any method');

  await addRule(page, '*', 'deny', ['DELETE']);
  await expect.poll(lastSet).toEqual(deny);
  await expect(rows(page)).toHaveCount(3);

  await page.click('[data-testid="settings-db-mcp-api-rules-add"]');
  const input = page.locator('[data-testid="api-rule-hosts"] input');
  await input.fill('a/b');
  await input.press('Enter');
  await expect(page.locator('[data-testid="api-rule-host-error"]')).toBeVisible();
  await expect(page.locator('[data-testid="api-rule-save"]')).toBeDisabled();
  await page.click('[data-testid="api-rule-cancel"]');

  await rows(page).first().locator('[data-testid="api-rule-remove"]').click();
  await expect.poll(lastSet).toEqual(remove);
  await expect(rows(page)).toHaveCount(2);

  await rows(page).first().locator('[data-testid="api-rule-remove"]').click();
  await expect.poll(lastSet).toEqual(removeWildcard);
  await expect(page.locator('[data-testid="settings-db-mcp-api-rules-no-allow"]')).toBeVisible();

  await rows(page).first().locator('[data-testid="api-rule-remove"]').click();
  await expect.poll(lastSet).toEqual(clear);
  await expect(empty(page)).toBeVisible();
});

test('unreadable stored rules show an error', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.settingsGetAll,
        response: { ...defaultSettings, dbMcp: { ...dbMcp, apiRulesInvalid: true } },
      },
    ],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Database MCP"]');
  await expect(page.locator('[data-testid="settings-db-mcp-api-rules-invalid"]')).toBeVisible();
});
