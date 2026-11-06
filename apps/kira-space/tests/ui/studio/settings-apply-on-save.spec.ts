import type { Page } from '@playwright/test';
import {
  defaultSettings,
  defaultSettings as spaceDefaults,
} from '../../../frontend/src/state/settingsDomain';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { openSettings } from './support/settings';

// The settings dialog stages every control into a local draft and only reaches
// SettingsService.Set on Save — see docs/HISTORY.md for the
// five scenarios below. A settings change is observable without opening a grid via the CSS
// custom property applyAppearance() writes (state/settings.ts): --kira-row-height reads 28px at
// the comfortable default and 22px for compact.

function dialog(page: Page) {
  return page.locator('[data-testid="settings-dialog"]');
}

function densityButton(page: Page, density: 'compact' | 'comfortable') {
  return dialog(page).locator(`[data-testid="settings-appearance-rowDensity-${density}"]`);
}

function rowHeightVar(page: Page): Promise<string> {
  return page.evaluate(() => document.documentElement.style.getPropertyValue('--kira-row-height'));
}

function settingsSetCalls(control: { log(): { channel: string; args: unknown }[] }) {
  return control.log().filter((e) => e.channel === IPC.settingsSet);
}

test('staged changes reach nothing until Save', async ({ relaunch }) => {
  const { window: page, control } = await relaunch();
  await openSettings(page);

  await densityButton(page, 'compact').click();
  await page.click('[data-testid="settings-word-wrap"]');
  await page.click('[data-testid="settings-row-coloring"]');

  expect(settingsSetCalls(control)).toHaveLength(0);
  expect(await rowHeightVar(page)).toBe('28px');
});

test('Save commits once, carrying only the leaves that changed', async ({ relaunch }) => {
  const flipped = {
    ...defaultSettings,
    appearance: { ...spaceDefaults.appearance, rowDensity: 'compact' as const, wordWrap: false },
  };
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.settingsSet, response: flipped }],
  });
  await openSettings(page);

  await densityButton(page, 'compact').click();
  await page.click('[data-testid="settings-word-wrap"]');
  await page.click('[data-testid="settings-save"]');

  await expect(dialog(page)).toHaveCount(0);
  const calls = settingsSetCalls(control);
  expect(calls).toHaveLength(1);
  expect(calls[0].args).toEqual({
    patch: { appearance: { rowDensity: 'compact', wordWrap: false } },
  });
  expect(await rowHeightVar(page)).toBe('22px');
});

test('Cancel and Escape both discard the draft', async ({ relaunch }) => {
  const { window: page, control } = await relaunch();

  await openSettings(page);
  await densityButton(page, 'compact').click();
  await page.click('[data-testid="settings-cancel"]');
  await expect(dialog(page)).toHaveCount(0);
  expect(settingsSetCalls(control)).toHaveLength(0);
  expect(await rowHeightVar(page)).toBe('28px');

  await openSettings(page);
  await densityButton(page, 'compact').click();
  await page.keyboard.press('Escape');
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expect(dialog(page)).toHaveCount(0);
  expect(settingsSetCalls(control)).toHaveLength(0);
  expect(await rowHeightVar(page)).toBe('28px');

  // The draft did not survive either close — reopening shows the stored (default) value again.
  await openSettings(page);
  await expect(densityButton(page, 'comfortable')).toHaveAttribute('aria-pressed', 'true');
  await densityButton(page, 'compact').click();
  await page.click('[data-testid="settings-cancel"]');
});

test('a per-setting reset stages only that leaf', async ({ relaunch }) => {
  const nonDefault = {
    ...defaultSettings,
    appearance: { ...spaceDefaults.appearance, rowDensity: 'compact' as const, wordWrap: false },
    cache: { l2BudgetMb: 512 },
    advanced: { opLogRetentionDays: 7 },
  };
  const { window: page, control } = await relaunch({
    control: [{ channel: IPC.settingsGetAll, response: nonDefault }],
  });

  await openSettings(page);
  await page.click('[data-testid="settings-reset-appearance-rowDensity"]');

  // Only the reset leaf moved — its sibling checkbox, still bound to the un-reset draft, stays
  // exactly where the fixture put it.
  await expect(densityButton(page, 'comfortable')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('[data-testid="settings-word-wrap"]')).not.toBeChecked();
  expect(settingsSetCalls(control)).toHaveLength(0);

  await page.click('[data-testid="settings-save"]');
  await expect(dialog(page)).toHaveCount(0);
  const calls = settingsSetCalls(control);
  expect(calls).toHaveLength(1);
  expect(calls[0].args).toEqual({
    patch: { appearance: { rowDensity: 'comfortable' } },
  });
});

test('a leaf reset button is disabled once the draft matches the default', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await openSettings(page);

  const reset = page.locator('[data-testid="settings-reset-appearance-rowDensity"]');
  await expect(reset).toBeDisabled(); // fixture already at the default (comfortable)

  await densityButton(page, 'compact').click();
  await expect(reset).toBeEnabled();

  await reset.click();
  await expect(reset).toBeDisabled();
  await expect(densityButton(page, 'comfortable')).toHaveAttribute('aria-pressed', 'true');
});

// patchSettings used to apply the patch to settingsState (and re-render --kira-row-height) *before*
// awaiting control.settingsSet — a rejected Save left the change live in this window with no
// broadcast to any other window or the database, correctly showing an error and staying open, but
// silently divergent from the truth until relaunch.
test('a rejected Save leaves the live value alone and shows the error', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      {
        channel: IPC.settingsSet,
        error: { code: 'E_QUERY', message: 'settings write failed' },
      },
    ],
  });
  await openSettings(page);

  await densityButton(page, 'compact').click();
  await page.click('[data-testid="settings-save"]');

  await expect(dialog(page).locator('[data-testid="settings-save-error"]')).toContainText(
    'settings write failed',
  );
  await expect(dialog(page)).toBeVisible(); // stays open on a rejection (P17 D7)
  expect(settingsSetCalls(control)).toHaveLength(1);
  // The whole point: no live change from an unconfirmed patch, in this window or any other.
  expect(await rowHeightVar(page)).toBe('28px');
});

test('an out-of-range value blocks Save until corrected', async ({ relaunch }) => {
  const { window: page, control } = await relaunch();
  await openSettings(page);
  await page.click('[data-testid="settings-section-Cache"]');

  await page.fill('[data-testid="settings-cache-budget"]', '5000');
  await expect(page.locator('[data-testid="settings-save"]')).toBeDisabled();
  await expect(page.locator('[data-testid="settings-cache-budget-error"]')).toBeVisible();
  expect(settingsSetCalls(control)).toHaveLength(0);

  await page.fill('[data-testid="settings-cache-budget"]', '128');
  await expect(page.locator('[data-testid="settings-save"]')).toBeEnabled();
  await expect(page.locator('[data-testid="settings-cache-budget-error"]')).toHaveCount(0);
});

// P16 D6: SettingsDialog is one of the two call sites this phase's own .p-select.md opt-in
// exists for — its two selects sit correctly at h-md (26px) beside size="md" TextFields, unlike
// the ten default-h-sm call sites the height flip itself fixes.
test('the font-family select is 26px, matching its own row (D6)', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await openSettings(page);

  const height = await page
    .locator('[data-testid="settings-font-family"]')
    .evaluate((el) => (el as HTMLElement).offsetHeight);
  expect(height).toBe(26);
});

// P90 §6.3: the Api section round-trips through Save exactly like every other section — only the
// leaf that actually changed reaches settingsSet.
test('the Api section round-trips a single changed leaf through Save', async ({ relaunch }) => {
  const { window: page, control } = await relaunch();
  await openSettings(page);
  await page.click('[data-testid="settings-section-Api"]');

  await page.fill('[data-testid="settings-api-maxResponseMb"]', '10');
  await page.click('[data-testid="settings-save"]');

  await expect(dialog(page)).toHaveCount(0);
  const calls = settingsSetCalls(control);
  expect(calls).toHaveLength(1);
  expect(calls[0].args).toEqual({ patch: { api: { maxResponseMb: 10 } } });
});

// A cleared numeric field is "not a number", never 0 (0 is a valid "unlimited" for this leaf).
test('a cleared numeric field blocks Save instead of writing 0', async ({ relaunch }) => {
  const { window: page, control } = await relaunch();
  await openSettings(page);
  await page.click('[data-testid="settings-section-Api"]');

  await page.fill('[data-testid="settings-api-maxResponseMb"]', '');
  await expect(page.locator('[data-testid="settings-api-maxResponseMb-error"]')).toHaveText(
    'Enter a whole number.',
  );
  await expect(page.locator('[data-testid="settings-save"]')).toBeDisabled();
  expect(settingsSetCalls(control)).toHaveLength(0);

  await page.fill('[data-testid="settings-api-maxResponseMb"]', '1.5');
  await expect(page.locator('[data-testid="settings-api-maxResponseMb-error"]')).toHaveText(
    'Enter a whole number.',
  );
});

// P117 S2: a `:value`/`@change` NativeSelect bound alongside `useVModel({ passive: true })` shows
// blank on open -- the passive vModel's own internal state overwrites the prop-driven value before
// paint. The log-level field binds through `model-value`/`update:model-value` instead.
test('the log-level select shows its real value on open (P117 S2)', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await openSettings(page);

  await page.click('[data-testid="settings-section-Advanced"]');
  await expect(page.locator('[data-testid="settings-git-log-level"]')).toHaveValue('info');
});
