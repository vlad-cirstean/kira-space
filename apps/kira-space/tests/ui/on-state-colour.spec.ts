import type { Locator } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem } from '@workbench/testing/ui/approvals';
import type { ControlSnapshot } from '../ipc/support/types';
import { ACME, SUMMARY } from '../screenshots/studio/support/acme';
import { expect, studioTest, test } from './fixtures';
import { openHttpMode } from './studio/support/apiMode';
import { openGrid } from './studio/support/grid';
import { openSettings } from './studio/support/settings';
import { FIXED_NOW, openPlan } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P754: on-state controls are the faint accent tint (#3b6ed1 at 20%) with a --kira-info edge
// (#69a1e8); off stays field with a dim edge. Dark theme only: the app has no light theme.
// Computed colours only. Renderer styling: no IPC, no backend half.
const ON_EDGE = 'rgb(105, 161, 232)';
const MARK = 'rgb(244, 244, 245)';
const OFF_EDGE = 'rgb(62, 62, 69)';
const KNOB = 'rgb(107, 107, 115)';
const FOCUS = 'rgb(161, 161, 170)';
const FIELD = 'rgb(39, 39, 44)';
const BORDER = 'rgb(50, 50, 55)';
const ACCENT = [59, 110, 209];

/** Blue tint: accent rgb at alpha 0.2 (computed style is `rgba(...)` or `color(srgb ...)`). */
async function expectOnTint(loc: Locator): Promise<void> {
  // Polls: toolbar buttons and toggles transition their fill.
  await expect
    .poll(async () => {
      const bg = await loc.evaluate((el) => getComputedStyle(el).backgroundColor);
      const nums = (bg.match(/[\d.]+/g) ?? []).map(Number);
      const scale = bg.startsWith('color(') ? 255 : 1;
      const rgb = nums.slice(0, 3).map((n) => n * scale);
      const near = rgb.length === 3 && rgb.every((c, i) => Math.abs(c - (ACCENT[i] ?? 0)) <= 2);
      return near ? (nums[3] ?? 1) : -1;
    })
    .toBeCloseTo(0.2, 1);
  await expect(loc).toHaveCSS('border-top-color', ON_EDGE);
}

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [SUMMARY] },
  ...ACME.control,
];

studioTest(
  'checked switches and checkboxes tint blue with a blue edge and bright mark',
  async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.connectionsList, response: [] }],
    });
    await openSettings(page);
    const on = page
      .locator(
        '[data-slot="dialog-content"] [data-slot="switch"][data-state="checked"], [data-slot="dialog-content"] [data-slot="checkbox"][data-state="checked"]',
      )
      .first();
    await expect(on).toBeVisible();
    await expectOnTint(on);
    const mark = on.locator('[data-slot="switch-thumb"], [data-slot="checkbox-indicator"]').first();
    const color = await mark.evaluate((el) => {
      const cs = getComputedStyle(el);
      return el.getAttribute('data-slot') === 'switch-thumb' ? cs.backgroundColor : cs.color;
    });
    expect(color).toBe(MARK);
  },
);

test('an unchecked switch keeps a dim edge and grey knob on the field fill', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [] }],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Advanced"]');
  const off = page
    .locator('[data-testid="settings-dialog"] [data-slot="switch"][aria-checked="false"]:visible')
    .first();
  await expect(off).toBeVisible();
  await expect(off).toHaveCSS('background-color', FIELD);
  await expect(off).toHaveCSS('border-top-color', OFF_EDGE);
  await expect(off.locator('[data-slot="switch-thumb"]')).toHaveCSS('background-color', KNOB);
});

test('a keyboard-focused checked switch rings 2px outside its edge', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [] }],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Advanced"]');
  const on = page
    .locator('[data-testid="settings-dialog"] [data-slot="switch"][aria-checked="true"]:visible')
    .first();
  await expect(on).toBeVisible();
  await on.focus();
  await page.keyboard.press('Shift+Tab');
  await page.keyboard.press('Tab');
  await expect(on).toBeFocused();
  await expect(on).toHaveCSS('outline-style', 'solid');
  await expect(on).toHaveCSS('outline-offset', '2px');
  await expect(on).toHaveCSS('outline-color', FOCUS);
});

// The environment radio sits under an `as-child` tooltip trigger that overwrites its `data-state`
// with open/closed; the fill must key on aria-checked or the checked radio loses it.
test('environment radios tint blue when checked, a dim edge when not', async ({ relaunch }) => {
  const env = (id: string, name: string, sortOrder: number, isActive: boolean) => ({
    id,
    name,
    sortOrder,
    isActive,
    color: 'none',
    description: '',
    file: `${name.toLowerCase()}.environment.yaml`,
  });
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      { channel: IPC.collectionsList, response: { collections: [], items: [] } },
      {
        channel: IPC.variablesListEnvironments,
        response: [env('e1', 'Staging', 0, true), env('e2', 'Production', 1, false)],
      },
      { channel: IPC.apiConfigView, response: { files: [], secrets: [] } },
    ],
  });
  await openHttpMode(page);
  await page.click('[data-testid="api-environments"]');
  const radios = page.locator('[data-testid^="matrix-activate-"]');
  const checked = radios.nth(0);
  await expect(checked).toHaveAttribute('aria-checked', 'true');
  await expectOnTint(checked);
  await expect(checked.locator('span span')).toHaveCSS('background-color', MARK);
  const unchecked = radios.nth(1);
  await expect(unchecked).toHaveAttribute('aria-checked', 'false');
  await expect(unchecked).toHaveCSS('background-color', FIELD);
  await expect(unchecked).toHaveCSS('border-top-color', OFF_EDGE);
});

test('an unchecked checkbox keeps a dim edge on the field fill, a checked one tints blue', async ({
  relaunch,
}) => {
  const [it] = contract<ApprovalItem[]>('ade-branch-adopt', 'ApprovalsService.List#adopt');
  if (!it) throw new Error('contract has no item');
  const { window: page } = await openPlan(relaunch, seedApprovals([liveItem(it, 1800, FIXED_NOW)]));
  const box = page.locator('[data-testid="approvals-dialog"]').getByRole('checkbox').first();
  await expect(box).toHaveAttribute('aria-checked', 'true');
  await expectOnTint(box);
  await box.click();
  await expect(box).toHaveAttribute('aria-checked', 'false');
  await expect(box).toHaveCSS('background-color', FIELD);
  await expect(box).toHaveCSS('border-top-color', OFF_EDGE);
});

studioTest(
  'grid search toggles tint blue when pressed and stay clear when off',
  async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: CONTROL, stream: ACME.port });
    await openGrid(page);
    await page.click('[data-testid="toolbar-search"]');
    const matchCase = page.locator('[data-testid="search-match-case"]');
    await matchCase.click();
    await expect(matchCase).toHaveAttribute('aria-pressed', 'true');
    await expectOnTint(matchCase);
    await page.mouse.move(5, 5);
    await matchCase.hover();
    await expectOnTint(matchCase);
    const regex = page.locator('[data-testid="search-regex"]');
    await expect(regex).not.toHaveAttribute('aria-pressed', 'true');
    await expect(regex).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');
    await expect(regex).toHaveCSS('border-top-color', 'rgba(0, 0, 0, 0)');
  },
);

test('a choice item tints blue on all sides, an off item keeps the grey border', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.connectionsList, response: [] }],
  });
  await openSettings(page);
  await page.click('[data-testid="settings-section-Advanced"]');
  const items = page.locator('[data-testid^="settings-git-log-level-"]');
  const on = items.locator('xpath=self::*[@data-state="on"]').first();
  await expect(on).toBeVisible();
  await expectOnTint(on);
  await expect(on).toHaveCSS('border-left-color', ON_EDGE);
  await expect(on).toHaveCSS('border-right-color', ON_EDGE);
  const off = items.locator('xpath=self::*[@data-state="off"]').first();
  await expect(off).toHaveCSS('border-top-color', BORDER);
});

test('the operations Toggle tints blue once on', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: [{ channel: IPC.opsRecent, response: [] }],
  });
  await window.click('[data-testid="toggle-operations-panel"]');
  const toggle = window.locator('[data-testid="ops-background"]');
  await expect(toggle).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');
  await toggle.click();
  await expectOnTint(toggle);
});
