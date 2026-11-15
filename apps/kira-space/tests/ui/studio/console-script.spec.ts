// J-CONS-001 / J-CONS-002 UI half: a SQLite script typed into the console is split into the
// statement list the flow test (contract studio-console-script) executed, and the pages it
// returned render, repeated column names included.
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { installClipboardSpy, lastClipboardWrite } from './support/clipboard';
import { createConnection, newConnectionArgs, openConsoleFromMenu } from './support/connect';
import { textPage } from './support/consolePages';
import { typeInto } from './support/editor';
import { connectAndExpandControl, connectionSummary } from './support/engineFixture';
import { connectionRow, openRowMenu } from './support/tree';

interface PageSummary {
  columns: string[];
  rows: string[][];
}
const scenario = 'studio-console-script';
const script = contract<string>(scenario, 'sqlite#script');
const statements = contract<string[]>(scenario, 'sqlite#statements');
const pages = contract<PageSummary[]>(scenario, 'sqlite#pages');
const dup = contract<{ statement: string; page: PageSummary }>(scenario, 'sqlite#dupColumns');

const CAPS = {
  tabular: true,
  documents: false,
  keyValue: false,
  stream: false,
  keyBrowser: false,
  defaultPageKind: 'tabular' as const,
  sql: true,
  definition: true,
  describe: true,
  projection: true,
  serverFilter: true,
  exactCount: true,
  pagination: 'keyset' as const,
  foreignKeys: true,
  canInsert: true,
  canUpdate: true,
  canDelete: true,
  writable: true,
  transactions: true,
  cancel: true,
  fileTransfer: false,
};

test('a typed SQLite script runs as the contract statement list and renders its pages', async ({
  relaunch,
}) => {
  const ID = 'conn-console-script';
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: newConnectionArgs('Script DB', 'sqlite', 'script.db'),
      response: connectionSummary('sqlite', 0, 'script.db', '', ID, 'Script DB', 'none'),
    },
    ...connectAndExpandControl(ID, 'SQLite 3.46', CAPS, []),
  ];
  const run = (stmts: string[], out: PageSummary[]): PortSnapshot => ({
    op: DATA_OP.execute,
    payload: { connectionId: ID, path: '', statements: stmts },
    response: { kind: 'execute', pages: out.map((p) => textPage(p.columns, p.rows)) },
  });
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: [run(statements, pages), run([dup.statement], [dup.page])],
  });

  await installClipboardSpy(page, { lazy: true });
  await createConnection(page, 'sqlite', 'Script DB', 'script.db');
  await expect(connectionRow(page)).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await openConsoleFromMenu(page, '');

  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, script, { paste: true });
  await page.click('[data-testid="console-run-all"]');
  await expect(view.locator('[data-testid="console-result-tab"]')).toHaveCount(statements.length);
  await expect(view.locator('[data-testid="console-error"]')).toHaveCount(0);
  const last = pages[pages.length - 1];
  await view.locator('[data-testid="console-result-tab"]').last().click();
  await expect(view.locator('[data-testid="console-result-grid"]').first()).toContainText(
    last.rows[last.rows.length - 1][0],
  );

  // J-CONS-002: repeated names stay distinct in the grid.
  await view.locator('.view-lines').click();
  await page.keyboard.press('ControlOrMeta+a');
  await page.keyboard.insertText(dup.statement);
  await page.click('[data-testid="console-run-all"]');
  await expect(view.locator('[data-testid="console-result-tab"]')).toHaveCount(
    statements.length + 1,
  );
  await view.locator('[data-testid="console-result-tab"]').last().click();
  const grid = view.locator('[data-testid="console-result-grid"]').first();
  await expect(grid.locator('[data-testid="console-result-header-cell"]')).toHaveCount(2);
  // Both cells keep their own value; a copy names the repeat x_2.
  const cell = (c: number) =>
    grid.locator(
      `[data-testid="console-result-row"][data-row="0"] [data-testid="console-result-cell"][data-col-index="${c}"]`,
    );
  await expect(cell(0)).toHaveText(dup.page.rows[0][0]);
  await expect(cell(1)).toHaveText(dup.page.rows[0][1]);
  await cell(0).click();
  await cell(1).click({ modifiers: ['Shift'] });
  await cell(0).click({ button: 'right' });
  await page.click('[data-testid="menu-item-copy-as-json"]');
  expect(JSON.parse(await lastClipboardWrite(page))).toEqual([
    { x: dup.page.rows[0][0], x_2: dup.page.rows[0][1] },
  ]);
});

// P636 UI half of J-CONS-028: a MySQL script with `DELIMITER` lines is split by the console into the
// statement list the flow test ran (no directive, no custom terminator); its pages render.
test('a typed MySQL DELIMITER script runs as the contract statement list', async ({ relaunch }) => {
  const ID = 'conn-console-delimiter';
  const delimiterScript = contract<string>(scenario, 'mysql#delimiterScript');
  const delimiterStatements = contract<string[]>(scenario, 'mysql#delimiterStatements');
  const delimiterPages = contract<PageSummary[]>(scenario, 'mysql#delimiterPages');
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: newConnectionArgs('Delimiter DB', 'mysql'),
      response: connectionSummary('mysql', 3306, 'kira_test', 'root', ID, 'Delimiter DB', 'none'),
    },
    ...connectAndExpandControl(ID, 'MySQL 8.4.11', CAPS, []),
  ];
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: [
      {
        op: DATA_OP.execute,
        payload: { connectionId: ID, path: '', statements: delimiterStatements },
        response: {
          kind: 'execute',
          pages: delimiterPages.map((p) => textPage(p.columns, p.rows)),
        },
      },
    ],
  });

  await createConnection(page, 'mysql', 'Delimiter DB');
  await expect(connectionRow(page)).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await openConsoleFromMenu(page, '');

  const view = page.locator('[data-testid="console-view"]');
  // A paste event, not insertText: Monaco auto-indents inserted lines, which would change the text.
  await view.locator('.view-lines').click();
  await view
    .locator('textarea')
    .first()
    .evaluate((el, text) => {
      const data = new DataTransfer();
      data.setData('text/plain', text);
      el.dispatchEvent(
        new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }),
      );
    }, delimiterScript);
  await page.click('[data-testid="console-run-all"]');
  const tabs = view.locator('[data-testid="console-result-tab"]');
  await expect(tabs).toHaveCount(delimiterStatements.length);
  await expect(view.locator('[data-testid="console-error"]')).toHaveCount(0);
  const grid = view.locator('[data-testid="console-result-grid"]').first();
  await tabs.nth(delimiterStatements.length - 2).click();
  await expect(grid).toContainText('a;b$$');
  await tabs.last().click();
  await expect(grid).toContainText('kira_delim_bi');
});
