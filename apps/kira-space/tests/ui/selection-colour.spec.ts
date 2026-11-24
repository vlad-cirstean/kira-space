import type { ControlSnapshot } from '../ipc/support/types';
import { ACME, SUMMARY } from '../screenshots/studio/support/acme';
import { expect, studioTest, test } from './fixtures';
import { gutterCell, openGrid } from './studio/support/grid';
import { openPlan } from './support/adeV2';
import { IPC } from './support/ipcChannels';

// P710: selection is one neutral grey (--kira-select #36363c, edge #71717a, text #45454d), hover
// stays --kira-bg-input. On-state colours live in on-state-colour.spec.ts. Computed colours only.
// Renderer styling: no IPC, no backend half.
const SELECT = 'rgb(54, 54, 60)';
const HOVER = 'rgb(39, 39, 44)';
const TEXT = 'rgb(69, 69, 77)';

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
