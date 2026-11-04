import type { Page } from '@playwright/test';
import { defaultSettings } from '../../frontend/src/state/settingsDomain';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { createConnection } from './support/connect';
import { gridCell as gridCellAt } from './support/grid';
import { IPC } from './support/ipcChannels';
import {
  BIG_ROWS_PATH,
  bigRowsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { expandRow, findRow, openRowMenu } from './support/tree';

// appearance.rowColoring, and the string/uuid colour drop from icons.ts's CATEGORY_COLOR
// (verified by scenario 1 below — before that change `hash` read #ce9178, not --kira-fg).
// bigRowsFixture's page is the minimal shape that distinguishes all three states: `id` is
// typeClass 'number' (colours), `hash` is typeClass 'text' (plain).

const CONNECTION_ID = 'conn-row-coloring';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Row Coloring DB', 'blue');
const FIXTURE = bigRowsFixture(CONNECTION_ID);

const NUMBER_COLOR = 'rgb(181, 206, 168)'; // --kira-syntax-number, #b5cea8
const PLAIN_COLOR = 'rgb(212, 212, 216)'; // --kira-fg, #d4d4d8

const CONNECTION_CREATE_SNAPSHOT: ControlSnapshot = {
  channel: IPC.connectionsCreate,
  args: {
    name: 'Row Coloring DB',
    kind: 'postgres',
    color: 'blue',
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
  response: CONNECTION_SUMMARY,
};

async function connectAndOpenBigRows(page: Page): Promise<void> {
  await createConnection(page, 'postgres', 'Row Coloring DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });

  await expandRow(page, '');
  await expandRow(page, 'database:kira_test');
  await expandRow(page, 'database:kira_test/schema:app');
  const bigRowsRow = await findRow(page, BIG_ROWS_PATH);
  await bigRowsRow.dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
}

function gridCell(page: Page, column: string) {
  return gridCellAt(page, 0, column);
}

async function computedColor(page: Page, column: string): Promise<string> {
  return gridCell(page, column).evaluate((el) => getComputedStyle(el).color);
}

async function inlineColor(page: Page, column: string): Promise<string> {
  return gridCell(page, column).evaluate((el) => (el as HTMLElement).style.color);
}

test('colouring on (default boot) — a number column is coloured, a text column is not', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [CONNECTION_CREATE_SNAPSHOT, ...FIXTURE.control],
    stream: FIXTURE.port,
  });
  await connectAndOpenBigRows(page);
  await expect.poll(() => computedColor(page, 'id')).toBe(NUMBER_COLOR);
  // The C1 guard: before C1 this read rgb(206, 145, 120) (--kira-syntax-string, #ce9178).
  expect(await computedColor(page, 'hash')).toBe(PLAIN_COLOR);
});

test('colouring off at boot — both columns plain, no inline colour at all', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      CONNECTION_CREATE_SNAPSHOT,
      ...FIXTURE.control,
      {
        channel: IPC.settingsGetAll,
        response: {
          ...defaultSettings,
          appearance: { ...defaultSettings.appearance, rowColoring: false },
        },
      },
    ],
    stream: FIXTURE.port,
  });
  await connectAndOpenBigRows(page);
  await expect.poll(() => computedColor(page, 'id')).toBe(PLAIN_COLOR);
  expect(await computedColor(page, 'hash')).toBe(PLAIN_COLOR);
  expect(await inlineColor(page, 'id')).toBe('');
  expect(await inlineColor(page, 'hash')).toBe('');
});

test('saving the toggle repaints the open grid', async ({ relaunch }) => {
  // Must supply its own settingsSet snapshot (mockRuntime.ts:154's wildcard otherwise echoes the
  // untouched defaults back, and patchSettings would revert the flag right after Save).
  const flippedSettings = {
    ...defaultSettings,
    appearance: { ...defaultSettings.appearance, rowColoring: false },
  };
  const { window: page } = await relaunch({
    control: [
      CONNECTION_CREATE_SNAPSHOT,
      ...FIXTURE.control,
      { channel: IPC.settingsSet, response: flippedSettings },
    ],
    stream: FIXTURE.port,
  });
  await connectAndOpenBigRows(page);
  await expect.poll(() => computedColor(page, 'id')).toBe(NUMBER_COLOR);

  await page.click('[data-testid="open-settings"]');
  await page.click('[data-testid="settings-row-coloring"]');
  await page.click('[data-testid="settings-save"]');
  await expect(page.locator('[data-testid="settings-save"]')).toHaveCount(0);

  await expect.poll(() => computedColor(page, 'id')).toBe(PLAIN_COLOR);
  expect(await inlineColor(page, 'id')).toBe('');
});
