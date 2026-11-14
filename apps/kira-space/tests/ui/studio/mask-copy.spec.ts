import type { Page } from '@playwright/test';
import type { ControlSnapshot, LogicalPage, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { primaryChord } from './support/chord';
import { clipboardRead, clipboardReset, prepareRealClipboard } from './support/clipboard';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { gridCell, gutterCell } from './support/grid';
import {
  COMPOSITE_PK_COLUMNS,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  connectionsDisconnectSnapshot,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow, openRowMenu } from './support/tree';

// J-MASK-001 / J-BC-007, UI half. The Go half (dbmcpflow TestMaskRuleHoldsAcrossGridCopyAndAgentRead)
// commits the rule, the rows, the masked clipboard text and what the agent reads. This half replays
// them: with the preview on every copy format holds masked text only, a stale (disconnected) grid
// copies the same masked text, and turning the preview off is the one way to copy real values.

const CONNECTION_ID = 'conn-mask-copy';
const RULES = contract<Record<string, unknown>[]>('studio-mask', 'rules').map((r) => ({
  ...r,
  id: 'rule-name-1',
  connectionId: CONNECTION_ID,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
}));
const ROWS = contract<(string | null)[][]>('studio-mask', 'rows');
const MASKED_TSV = contract<string>('studio-mask', 'copy:tsv');
const RAW_TSV = ROWS.map((r) => r.map((c) => c ?? '').join('\t')).join('\n');

const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const PAGE: LogicalPage = {
  kind: 'tabular',
  columns: COMPOSITE_PK_COLUMNS,
  rows: ROWS,
  position: {
    offset: 0,
    pageSize: 100,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PORT: PortSnapshot[] = [
  { ...FIXTURE.port[0], response: { kind: 'read', page: PAGE, source: 'server' } },
];
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Mask Copy DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Mask Copy DB', 'green'),
  },
  ...FIXTURE.control,
  { channel: IPC.maskRulesList, args: { connectionId: CONNECTION_ID }, response: RULES },
  { channel: IPC.maskRulesCorrelationKey, args: { connectionId: CONNECTION_ID }, response: '' },
  connectionsDisconnectSnapshot(CONNECTION_ID),
];

async function copyRows(page: Page, browserName: string, id: string): Promise<string> {
  await clipboardReset(page, browserName);
  await gridCell(page, 3, 'name').click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.locator('[data-testid="menu-item-copy-rows"]').hover();
  await expect(page.locator('[data-testid="context-submenu"]')).toBeVisible();
  await page.click(`[data-testid="menu-item-${id}"]`);
  return clipboardRead(page, browserName);
}

async function keyboardCopy(page: Page, browserName: string): Promise<string> {
  await clipboardReset(page, browserName);
  await page
    .locator('[data-testid="data-grid"] .grid-canvas')
    .first()
    .evaluate((el) => (el as HTMLElement).focus());
  await page.keyboard.press(await primaryChord(page, 'c'));
  return clipboardRead(page, browserName);
}

function expectNoRealName(text: string) {
  expect(text).not.toContain('Lovelace');
  expect(text).not.toContain('Brien');
}

test('mask preview copies masked text in every format, stale included; only preview-off copies real values', async ({
  relaunch,
  browserName,
}) => {
  test.setTimeout(180_000);
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: PORT,
    prepare: (p) => prepareRealClipboard(p, browserName),
  });
  await connectAndExpand(page, 'Mask Copy DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(gridCell(page, 0, 'name')).toBeVisible();

  const toggle = page.locator('[data-testid="toolbar-mask-preview"]');
  await expect(toggle).toBeVisible();
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await expect(gridCell(page, 0, 'name')).toHaveText('Ada Lovelace');

  // Preview off: the grid shows real values, so a copy holds them.
  await gutterCell(page, 0).click();
  await gutterCell(page, ROWS.length - 1).click({ modifiers: ['Shift'] });
  expect(await keyboardCopy(page, browserName)).toBe(RAW_TSV);

  // Preview on: display, keyboard copy and every menu format hold masked text only.
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/cell-masked/);
  await gutterCell(page, 0).click();
  await gutterCell(page, ROWS.length - 1).click({ modifiers: ['Shift'] });
  const keyboard = await keyboardCopy(page, browserName);
  expect(keyboard).toBe(MASKED_TSV);
  const formats: string[] = [keyboard];
  for (const id of ['copy-rows-tsv', 'copy-rows-csv', 'copy-rows-json', 'copy-rows-insert']) {
    formats.push(await copyRows(page, browserName, id));
  }
  expect(formats[1]).toBe(MASKED_TSV);
  for (const text of formats) expectNoRealName(text);

  // Single cell and column values follow the display too.
  await gridCell(page, 0, 'name').click();
  expectNoRealName(await keyboardCopy(page, browserName));

  // A dropped connection keeps the page; the stale grid copies the same masked text.
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-disconnect"]');
  await expect(page.locator('[data-testid="grid-stale"]')).toBeVisible();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await gutterCell(page, 0).click();
  await gutterCell(page, ROWS.length - 1).click({ modifiers: ['Shift'] });
  expect(await keyboardCopy(page, browserName)).toBe(MASKED_TSV);
  for (const id of ['copy-rows-csv', 'copy-rows-json', 'copy-rows-insert']) {
    expectNoRealName(await copyRows(page, browserName, id));
  }

  // Explicit reveal: preview off copies the real values again, even stale.
  await toggle.click();
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await gutterCell(page, 0).click();
  await gutterCell(page, ROWS.length - 1).click({ modifiers: ['Shift'] });
  expect(await keyboardCopy(page, browserName)).toBe(RAW_TSV);
});
