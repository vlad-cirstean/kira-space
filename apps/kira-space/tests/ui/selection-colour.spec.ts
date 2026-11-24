import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem } from '@workbench/testing/ui/approvals';
import type { ControlSnapshot } from '../ipc/support/types';
import {
  ACME,
  DB_PATH,
  ORDERS_PATH,
  SCHEMA_PATH,
  SUMMARY,
} from '../screenshots/studio/support/acme';
import { expect, studioTest, test } from './fixtures';
import { openHttpMode } from './studio/support/apiMode';
import { gutterCell } from './studio/support/grid';
import { openSettings } from './studio/support/settings';
import { connectionRow, expandRow, findRow, openRowMenu } from './studio/support/tree';
import { FIXED_NOW, openPlan } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// P710: selection is one neutral grey (--kira-select #36363c, edge #71717a, text #45454d), hover
// stays --kira-bg-input. Computed colours only. Renderer styling: no IPC, no backend half.
const SELECT = 'rgb(54, 54, 60)';
const HOVER = 'rgb(39, 39, 44)';
const TEXT = 'rgb(69, 69, 77)';
// P730 (M2): toggles on = dark grey fill, light edge, bright mark; off = field, dim edge, grey knob.
const ON = 'rgb(51, 51, 58)';
const ON_EDGE = 'rgb(196, 196, 204)';
const MARK = 'rgb(244, 244, 245)';
const OFF_EDGE = 'rgb(62, 62, 69)';
const KNOB = 'rgb(107, 107, 115)';
const FOCUS = 'rgb(161, 161, 170)';
const FIELD = 'rgb(39, 39, 44)';

const head = (id: string) =>
  `[data-testid="ade-card"][data-task-id="${id}"] [data-testid="ade-card-head"]`;

test('a selected plan card paints the grey fill and a task colour frame, hover is lighter than idle', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(head('T_search')).click();
  await expect(page.locator('[data-testid="ade-card"][data-task-id="T_search"]')).toHaveAttribute(
    'aria-current',
    'true',
  );
  await page.mouse.move(5, 5);
  const selected = page.locator(head('T_search'));
  await expect(selected).toHaveCSS('background-color', SELECT);
  const box = page.locator('[data-testid="ade-card"][data-task-id="T_search"]');
  await expect(box).toHaveCSS('outline-width', '2px');
  const edge = await box.evaluate((el) => getComputedStyle(el).borderLeftColor);
  await expect(box).toHaveCSS('outline-color', edge);
  const other = page.locator(head('T_auth'));
  await other.hover();
  await expect(other).toHaveCSS('background-color', HOVER);
});

test('text selection is the grey --kira-select-text', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  const color = await page.evaluate(
    () => getComputedStyle(document.body, '::selection').backgroundColor,
  );
  expect(color).toBe(TEXT);
});

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [SUMMARY] },
  ...ACME.control,
];

async function openGrid(page: Page): Promise<void> {
  const row = connectionRow(page);
  await expect(row).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(row.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, SCHEMA_PATH);
  await (await findRow(page, ORDERS_PATH)).dblclick();
  await expect(page.locator('[data-testid="grid-row"]').first()).toBeVisible({ timeout: 15_000 });
}

studioTest(
  'grid cell and row selection paint grey, a hovered row stays lighter',
  async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: CONTROL, stream: ACME.port });
    await openGrid(page);
    const rows = page.locator('[data-testid="grid-row"]');
    await rows.nth(2).locator('[data-testid="grid-cell"]').nth(2).click();
    await page.mouse.move(5, 5);
    await expect(page.locator('.kira-cell-selected').first()).toHaveCSS('background-color', SELECT);

    await gutterCell(page, 4).click();
    await page.mouse.move(5, 5);
    const cells = page.locator('[data-testid="grid-row"][data-row="4"] [data-testid="grid-cell"]');
    const n = await cells.count();
    for (let i = 0; i < n; i++) await expect(cells.nth(i)).toHaveCSS('background-color', SELECT);
    await rows.nth(6).hover();
    await expect(rows.nth(6).locator('[data-testid="grid-cell"]').first()).toHaveCSS(
      'background-color',
      HOVER,
    );
  },
);

studioTest(
  'checked switches and checkboxes fill dark grey with a light edge and bright mark',
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
    await expect(on).toHaveCSS('background-color', ON);
    await expect(on).toHaveCSS('border-top-color', ON_EDGE);
    const mark = on.locator('[data-slot="switch-thumb"], [data-slot="checkbox-indicator"]').first();
    const color = await mark.evaluate((el) => {
      const cs = getComputedStyle(el);
      return el.getAttribute('data-slot') === 'switch-thumb' ? cs.backgroundColor : cs.color;
    });
    expect(color).toBe(MARK);
  },
);

// The environment radio sits under an `as-child` tooltip trigger that overwrites its `data-state`
// with open/closed; the fill must key on aria-checked or the checked radio loses it.
test('environment radios: checked is dark filled with a light edge and bright dot, unchecked has a dim edge', async ({
  relaunch,
}) => {
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
  await expect(checked).toHaveCSS('background-color', ON);
  await expect(checked).toHaveCSS('border-top-color', ON_EDGE);
  await expect(checked.locator('span span')).toHaveCSS('background-color', MARK);
  const unchecked = radios.nth(1);
  await expect(unchecked).toHaveAttribute('aria-checked', 'false');
  await expect(unchecked).toHaveCSS('background-color', FIELD);
  await expect(unchecked).toHaveCSS('border-top-color', OFF_EDGE);
});

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

test('an unchecked checkbox keeps a dim edge on the field fill', async ({ relaunch }) => {
  const [it] = contract<ApprovalItem[]>('ade-branch-adopt', 'ApprovalsService.List#adopt');
  if (!it) throw new Error('contract has no item');
  const { window: page } = await openPlan(relaunch, seedApprovals([liveItem(it, 1800, FIXED_NOW)]));
  const box = page.locator('[data-testid="approvals-dialog"]').getByRole('checkbox').first();
  await expect(box).toHaveAttribute('aria-checked', 'true');
  await box.click();
  await expect(box).toHaveAttribute('aria-checked', 'false');
  await expect(box).toHaveCSS('background-color', FIELD);
  await expect(box).toHaveCSS('border-top-color', OFF_EDGE);
});
