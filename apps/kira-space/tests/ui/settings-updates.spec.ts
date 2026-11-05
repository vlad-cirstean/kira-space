import type { Page } from '@playwright/test';
import { TRACKED_KINDS } from '../../frontend/src/state/trackingConcepts';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// UI half of adeflow's TestTrackingSettings: the Updates pane in Settings. Contract settings-tracking.

type Tracking = Record<string, Record<string, boolean>>;
const tracking = () => contract<Tracking>('settings-tracking', 'SettingsService.GetAll#tracking');

const id = (s: string) => `[data-testid="${s}"]`;

async function openPane(page: Page): Promise<void> {
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator(id('settings-section-Updates')).click();
}

test('contract: the frontend catalog equals updates.Catalog', () => {
  const mirrored = Object.fromEntries(
    TRACKED_KINDS.map((k) => [
      k.kind,
      Object.fromEntries(k.concepts.map((c) => [c.id, c.default])),
    ]),
  );
  expect(mirrored).toEqual(tracking());
});

test('contract: one switch per concept, set to its default', async ({ relaunch }) => {
  const fx = tracking();
  const { window: page } = await relaunch();
  await openPane(page);
  for (const [kind, concepts] of Object.entries(fx)) {
    for (const [concept, on] of Object.entries(concepts)) {
      await expect(page.locator(id(`settings-updates-${kind}-${concept}`))).toHaveAttribute(
        'aria-checked',
        String(on),
      );
    }
    await expect(page.locator(`[data-testid^="settings-updates-${kind}-"]`)).toHaveCount(
      Object.keys(concepts).length,
    );
  }
});

test('Save writes the whole leaf of the changed kind only', async ({ relaunch }) => {
  const fx = tracking();
  const { window: page, control } = await relaunch();
  await openPane(page);
  await page.locator(id('settings-updates-changeRequest-checksPassed')).click();
  await page.locator(id('settings-save')).click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual({
      patch: { tracking: { changeRequest: { ...fx.changeRequest, checksPassed: true } } },
    });
});

test('reset restores the kind defaults', async ({ relaunch }) => {
  const fx = tracking();
  const { window: page } = await relaunch();
  await openPane(page);
  const merged = page.locator(id('settings-updates-branch-merged'));
  await merged.click();
  await expect(merged).toHaveAttribute('aria-checked', String(!fx.branch?.merged));
  await page.locator(id('settings-reset-tracking-branch')).click();
  await expect(merged).toHaveAttribute('aria-checked', String(fx.branch?.merged));
});
