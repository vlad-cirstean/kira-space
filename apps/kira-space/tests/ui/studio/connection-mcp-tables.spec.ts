import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import type { DbMcpTablesListing } from '@shared/domain/dbmcp';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// The table allow list in the connection MCP tab. Backend half: dbmcpflow TestDbMcpTableAllowList, whose
// dbmcp-tables contract pins the listing below.

const FILE = 'local.yaml';
const dlg = '[data-testid="connection-file-dialog"]';

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const listing = () => contract<DbMcpTablesListing>('dbmcp-tables', 'DbMcpService.Tables');

async function open(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  kind: string,
  tables?: string[],
) {
  const mcp: Record<string, unknown> = { enabled: true };
  if (tables) mcp.tables = tables;
  const lines = ['name: Local', `kind: ${kind}`, 'mcp:', '  enabled: true'];
  if (tables) lines.push('  tables:', ...tables.map((x) => `    - ${x}`));
  const text = `${lines.join('\n')}\n`;
  const base = contract<ConnectionFileDetail>('connection-form', 'file#read');
  const detail = { ...base, file: FILE, id: list()[0]?.id ?? '', text };
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: list() },
      { channel: IPC.connectionConfigView, response: view() },
      { channel: IPC.connectionConfigFile, response: detail },
      {
        channel: IPC.connectionConfigForm,
        response: { problem: null, text, value: { name: 'Local', kind, mcp } },
      },
      { channel: IPC.dbMcpTables, response: listing() },
    ],
  });
  await (await connectionRow(page, 'Local')).click({ button: 'right' });
  await page.click('[data-testid="menu-item-edit"]');
  await expect(page.locator(dlg)).toBeVisible();
  await page.locator('[data-testid="connection-tab-mcp"]').click();
  return page;
}

const rows = '[data-testid="connection-mcp-table-row"]';

test('contract: with no list every table is allowed and the enforcement is stated', async ({
  relaunch,
}) => {
  const page = await open(relaunch, 'sqlite');
  await expect(page.locator('[data-testid="connection-mcp-tables-all"]')).toHaveAttribute(
    'aria-checked',
    'true',
  );
  await expect(page.locator(rows)).toHaveCount(0);
  const enforce = page.locator('[data-testid="connection-mcp-tables-enforce"]');
  await expect(enforce).toContainText('parses every MCP statement');
  await expect(enforce).toContainText('refused');
  await expect(enforce).toContainText('view is checked by its own name');
  await expect(enforce).toContainText('No exemption applies');
});

test('contract: a list shows the live tables with the listed ones ticked', async ({ relaunch }) => {
  const page = await open(relaunch, 'sqlite', ['notes']);
  await expect(page.locator('[data-testid="connection-mcp-tables-all"]')).toHaveAttribute(
    'aria-checked',
    'false',
  );
  const tables = listing().tables;
  await expect(page.locator(rows)).toHaveCount(tables.length);
  for (const { ref } of tables) {
    const row = page.locator(`${rows}[data-ref="${ref}"] [role="checkbox"]`);
    await expect(row).toHaveAttribute('aria-checked', ref === 'notes' ? 'true' : 'false');
  }
  await expect(page.locator('[data-testid="connection-mcp-tables-count"]')).toContainText(
    `1 of ${tables.length} allowed`,
  );
});

test('ticking a row and the select-all box change the count', async ({ relaunch }) => {
  const page = await open(relaunch, 'sqlite', ['notes']);
  const total = listing().tables.length;
  await page.locator(`${rows}[data-ref="secret"] [role="checkbox"]`).click();
  await expect(page.locator('[data-testid="connection-mcp-tables-count"]')).toContainText(
    `2 of ${total} allowed`,
  );
  await page.locator('[data-testid="connection-mcp-tables-select-all"]').click();
  await expect(page.locator('[data-testid="connection-mcp-tables-count"]')).toContainText(
    `${total} of ${total} allowed`,
  );
  await page.locator('[data-testid="connection-mcp-tables-search"]').fill('view');
  await expect(page.locator(rows)).toHaveCount(1);
});

test('a kind without tables says there is nothing to limit', async ({ relaunch }) => {
  const page = await open(relaunch, 'redis');
  await expect(page.locator('[data-testid="connection-mcp-tables-na"]')).toBeVisible();
  await expect(page.locator('[data-testid="connection-mcp-tables-all"]')).toHaveCount(0);
});
