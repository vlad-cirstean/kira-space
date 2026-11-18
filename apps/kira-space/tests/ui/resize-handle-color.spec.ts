import type { Locator, Page } from '@playwright/test';
import { center, expectIdle, openDock } from '@workbench/testing/ui/panelResize';
import type { ControlSnapshot } from '../ipc/support/types';
import { apiTest, expect, studioTest, test } from './fixtures';
import { openHttpModeAndNewRequest } from './studio/support/apiMode';
import { createConnection } from './studio/support/connect';
import {
  APP_PATH,
  DB_PATH,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './studio/support/postgresFixture';
import { expandRow, findRow, openRowMenu } from './studio/support/tree';
import { IPC } from './support/ipcChannels';

// P663: every pane/column resize bar is neutral grey (--kira-resize*), never the blue
// --kira-focus. Idle line = --kira-border, hover and drag one grey step up each.

const IDLE_LINE = 'rgb(50, 50, 55)';
const HOVER = 'rgb(82, 82, 91)';
const DRAG = 'rgb(142, 142, 151)';
const FOCUS_BLUE = 'rgb(90, 138, 230)';
const TRANSPARENT = 'rgba(0, 0, 0, 0)';

function expectNeutral(color: string): void {
  expect(color).not.toBe(FOCUS_BLUE);
  const rgb = color.match(/\d+/g)?.slice(0, 3).map(Number);
  if (!rgb || rgb.length < 3) throw new Error(`unparsable colour ${color}`);
  expect(Math.max(...rgb) - Math.min(...rgb)).toBeLessThanOrEqual(12);
}

const bg = (el: Locator) => el.evaluate((n) => getComputedStyle(n).backgroundColor);
const shadow = (el: Locator) => el.evaluate((n) => getComputedStyle(n).boxShadow);

/** Hover, then drag with the pointer still over the bar, asserting each colour via `read`. */
async function expectHoverAndDrag(
  page: Page,
  handle: Locator,
  read: (el: Locator) => Promise<string>,
  axis: 'x' | 'y',
): Promise<void> {
  await handle.hover();
  await expect.poll(() => read(handle)).toBe(HOVER);
  expectNeutral(await read(handle));
  const at = await center(handle);
  await page.mouse.down();
  await page.mouse.move(at.x + (axis === 'x' ? 10 : 0), at.y + (axis === 'y' ? -10 : 0), {
    steps: 3,
  });
  await page.mouse.move(at.x, at.y);
  await expect(handle).toHaveAttribute('data-state', 'drag');
  await expect.poll(() => read(handle)).toBe(DRAG);
  expectNeutral(await read(handle));
  await page.mouse.up();
}

test('project panel handle: idle transparent, grey hover, grey drag', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  const handle = page.locator('.workbench-shell [data-slot="resizable-handle"]').first();
  expect(await bg(handle)).toBe(TRANSPARENT);
  await expectHoverAndDrag(page, handle, bg, 'x');
  await page.mouse.move(5, 5);
  await expectIdle(page, handle);
  await expect.poll(() => bg(handle)).toBe(TRANSPARENT);
});

test('ops dock handle: grey hover, grey drag', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await openDock(page);
  const handle = page.locator(
    '.workbench-shell [role="separator"][aria-label="Resize operations panel"]',
  );
  await expectHoverAndDrag(page, handle, bg, 'y');
});

apiTest('line handle: grey idle divider, grey hover, grey drag', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await openHttpModeAndNewRequest(page);
  const handle = page.locator('[data-testid="http-request-view"]').locator('.request-splitter');
  await expect.poll(() => shadow(handle)).toContain(IDLE_LINE);
  await expectHoverAndDrag(page, handle, bg, 'y');
});

apiTest('monaco sash hover border is grey', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-request-pane-body"]');
  await page.click('[data-testid="http-body-mode-code"]');
  const root = page.locator('[data-testid="http-request-pane"] .monaco-editor').first();
  await expect(root).toBeVisible();
  const value = await root.evaluate((n) =>
    getComputedStyle(n).getPropertyValue('--vscode-sash-hoverBorder').trim().toLowerCase(),
  );
  expect(['#52525b', '#52525bff']).toContain(value);
});

studioTest('slickgrid column handle: grey hover, grey drag', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-resize-colour';
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Resize Colour DB',
        kind: 'postgres',
        color: 'cyan',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: postgresConnectionSummary(CONNECTION_ID, 'Resize Colour DB', 'cyan'),
    },
    ...orderItemsFixture(CONNECTION_ID).control,
  ];
  const { window: page } = await relaunch({
    control,
    stream: orderItemsFixture(CONNECTION_ID).port,
  });
  await createConnection(page, 'postgres', 'Resize Colour DB');
  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  await (await findRow(page, ORDER_ITEMS_PATH)).dblclick();

  const header = page.locator('[data-testid="grid-header-cell"][data-column="quantity"]');
  await expect(header).toBeVisible();
  const handle = header.locator('.slick-resizable-handle');
  await expect(handle).toHaveCount(1);
  const after = (el: Locator) => el.evaluate((n) => getComputedStyle(n, '::after').backgroundColor);

  await handle.hover();
  await expect.poll(() => after(handle)).toBe(HOVER);
  const at = await center(handle);
  await page.mouse.down();
  await page.mouse.move(at.x + 30, at.y, { steps: 4 });
  await expect.poll(() => after(handle)).toBe(DRAG);
  expectNeutral(await after(handle));
  await page.mouse.up();
});
