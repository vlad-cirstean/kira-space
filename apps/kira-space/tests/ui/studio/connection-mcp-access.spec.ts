import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// The Agent access line, known-limit notes and per-setting enforcement lines in the connection
// MCP tab. Backend half: dbmcpflow
// TestDbMcpListConnectionsAccess, whose dbmcp-list-access contract pins the summaries below.

const FILE = 'local.yaml';
const dlg = '[data-testid="connection-file-dialog"]';

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const rows = () =>
  contract<{ Name: string; Enforcement: string; Summary: string }[]>(
    'dbmcp-list-access',
    'list_connections',
  );

interface Spec {
  kind: string;
  mcp: Record<string, string | boolean>;
  readOnly?: boolean;
}

function fileOf({ kind, mcp, readOnly }: Spec) {
  const value: Record<string, unknown> = { name: 'Pg', kind, mcp: { enabled: true, ...mcp } };
  if (readOnly) value.read_only = true;
  const lines = [`name: Pg`, `kind: ${kind}`];
  if (readOnly) lines.push('read_only: true');
  lines.push('mcp:', '  enabled: true', ...Object.entries(mcp).map(([k, v]) => `  ${k}: ${v}`));
  return { text: `${lines.join('\n')}\n`, value };
}

async function open(relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'], spec: Spec) {
  const { text, value } = fileOf(spec);
  const base = contract<ConnectionFileDetail>('connection-form', 'file#read');
  const detail = { ...base, file: FILE, id: list()[0]?.id ?? '', text };
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: list() },
      { channel: IPC.connectionConfigView, response: view() },
      { channel: IPC.connectionConfigFile, response: detail },
      { channel: IPC.connectionConfigForm, response: { problem: null, text, value } },
    ],
  });
  await (await connectionRow(page, 'Local')).click({ button: 'right' });
  await page.click('[data-testid="menu-item-edit"]');
  await expect(page.locator(dlg)).toBeVisible();
  await page.locator('[data-testid="connection-tab-mcp"]').click();
  return page;
}

test('contract: a SQL engine reads on a database-enforced lane and names its limits', async ({
  relaunch,
}) => {
  const page = await open(relaunch, {
    kind: 'sqlite',
    mcp: { read: 'allow', write: 'prompt', ddl: 'deny' },
  });
  const expected = rows().find((r) => r.Name === 'notes db');
  expect(expected?.Enforcement).toBe('session');
  await expect(page.locator('[data-testid="connection-mcp-access-summary"]')).toContainText(
    expected?.Summary ?? '',
  );
  await expect(page.locator('[data-testid="connection-mcp-access-enforcement"]')).toContainText(
    'enforced by the database',
  );
  await expect(page.locator('[data-testid="connection-mcp-note-readLane"]')).toBeVisible();
  await expect(page.locator('[data-testid="connection-mcp-note-limitDdl"]')).toBeVisible();
  await expect(page.locator('[data-testid="connection-mcp-note-limitApp"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="connection-mcp-note-noWriteConsole"]')).toHaveCount(0);
});

test('the DDL note hides while write and DDL are both denied', async ({ relaunch }) => {
  const page = await open(relaunch, {
    kind: 'postgres',
    mcp: { read: 'allow', write: 'deny', ddl: 'deny' },
  });
  await expect(page.locator('[data-testid="connection-mcp-access-summary"]')).toContainText(
    'read.',
  );
  await expect(page.locator('[data-testid="connection-mcp-note-readLane"]')).toBeVisible();
  await expect(page.locator('[data-testid="connection-mcp-note-limitDdl"]')).toHaveCount(0);
});

test('a document engine says reads are checked by Kira, not the database', async ({ relaunch }) => {
  const page = await open(relaunch, {
    kind: 'mongodb',
    mcp: { read: 'allow', write: 'prompt', ddl: 'deny' },
  });
  await expect(page.locator('[data-testid="connection-mcp-access-enforcement"]')).toContainText(
    'checked by Kira',
  );
  await expect(page.locator('[data-testid="connection-mcp-note-limitApp"]')).toBeVisible();
  await expect(page.locator('[data-testid="connection-mcp-note-readLane"]')).toHaveCount(0);
});

test('a kind without a write console disables write and DDL, never hides them', async ({
  relaunch,
}) => {
  const page = await open(relaunch, {
    kind: 'kibana',
    mcp: { read: 'allow', write: 'prompt', ddl: 'prompt' },
  });
  await expect(page.locator('[data-testid="connection-mcp-access-summary"]')).toContainText(
    'read.',
  );
  await expect(page.locator('[data-testid="connection-mcp-note-noWriteConsole"]')).toBeVisible();
  await expect(page.locator('[data-testid="connection-mcp-note-limitDdl"]')).toHaveCount(0);
  for (const cls of ['write', 'ddl']) {
    await expect(page.locator(`[data-testid="connection-mcp-${cls}-allow"]`)).toBeDisabled();
  }
  await expect(page.locator('[data-testid="connection-mcp-read-allow"]')).toBeEnabled();
});

const line = (page: Awaited<ReturnType<typeof open>>, id: string) =>
  page.locator(`[data-testid="connection-mcp-enforce-${id}"]`);

test('modes, DDL, exemption, throttle, rows and masking lines render for a SQL engine', async ({
  relaunch,
}) => {
  const page = await open(relaunch, {
    kind: 'sqlite',
    mcp: { read: 'allow', write: 'prompt', ddl: 'deny' },
  });
  await expect(line(page, 'modes')).toContainText('Kira checks the mode before any query runs');
  await expect(line(page, 'modes')).toContainText('no answer in 10 minutes refuses it');
  await expect(line(page, 'ddl')).toContainText('Text it can’t classify needs DDL');
  await expect(line(page, 'exemption')).toContainText(
    'A dialog always appears, even when the mode is allow',
  );
  await expect(line(page, 'exemption')).toContainText('Approve all does not cover it');
  await expect(line(page, 'throttle')).toContainText('one gate per connection');
  await expect(line(page, 'rows')).toContainText('200 rows');
  await expect(line(page, 'rows')).toContainText('up to 2000');
  await expect(line(page, 'masking')).toContainText('lowercased name');
  await expect(line(page, 'masking')).toContainText('The rule’s table is ignored');
  await expect(line(page, 'masking')).toContainText('can reveal values by which rows come back');
});

test('the DDL line hides for a kind without a write console, the rest stay', async ({
  relaunch,
}) => {
  const page = await open(relaunch, { kind: 'kibana', mcp: { read: 'allow' } });
  await expect(line(page, 'ddl')).toHaveCount(0);
  await expect(line(page, 'modes')).toBeVisible();
});

test('the exemption, throttle, rows and masking lines hide while MCP is off', async ({
  relaunch,
}) => {
  const page = await open(relaunch, { kind: 'sqlite', mcp: { enabled: false } });
  for (const id of ['exemption', 'throttle', 'rows', 'masking']) {
    await expect(line(page, id)).toHaveCount(0);
  }
  await expect(line(page, 'modes')).toBeVisible();
});

test('the Privacy tab repeats the masking limits', async ({ relaunch }) => {
  const page = await open(relaunch, { kind: 'sqlite', mcp: {} });
  await page.locator('[data-testid="connection-tab-privacy"]').click();
  await expect(page.locator('[data-testid="connection-privacy-enforce-masking"]')).toContainText(
    'best effort',
  );
});
