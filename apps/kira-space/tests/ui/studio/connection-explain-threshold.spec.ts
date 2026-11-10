import type { ConnectionSummary } from '@shared/domain/connection';
import { explainThresholdOf } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// The per-connection expensive query threshold: its Advanced tab field, the MCP tab line that says
// where it comes from, and the range error. Backend half: dbmcpflow TestDbMcpConnectionExplainThreshold,
// whose connection-explain-threshold contract pins the value list_connections reports.

const FILE = 'local.yaml';
const dlg = '[data-testid="connection-file-dialog"]';
const APP_DEFAULT = 100_000;

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const listed = () =>
  contract<{ name: string; expensiveQuery: { rowsThreshold: number } }[]>(
    'connection-explain-threshold',
    'list_connections',
  );

async function open(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  kind: string,
  threshold: number | null,
  tab: 'mcp' | 'advanced',
) {
  const lines = ['name: Pg', `kind: ${kind}`, 'mcp:', '  enabled: true'];
  const value: Record<string, unknown> = { name: 'Pg', kind, mcp: { enabled: true } };
  if (threshold !== null) {
    lines.splice(2, 0, `explain_threshold_rows: ${threshold}`);
    value.explain_threshold_rows = threshold;
  }
  const text = `${lines.join('\n')}\n`;
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
  await page.locator(`[data-testid="connection-tab-${tab}"]`).click();
  return page;
}

const field = '[data-testid="connection-explain-threshold"]';

test('postgres with its own threshold: the field shows it and the MCP tab says where it is set', async ({
  relaunch,
}) => {
  const page = await open(relaunch, 'postgres', 5000, 'advanced');
  await expect(page.locator(field)).toHaveValue('5000');
  await expect(page.locator('[data-testid="connection-explain-threshold-reset"]')).toBeEnabled();
  await expect(
    page.locator('[data-testid="connection-explain-threshold-description"]'),
  ).toContainText('not rows it returns');
  await expect(page.locator('[data-testid="connection-explain-threshold-engine"]')).toContainText(
    'largest single table scan',
  );
  await page.locator('[data-testid="connection-tab-mcp"]').click();
  await expect(page.locator('[data-testid="connection-mcp-rows-who"]')).toContainText('5,000 rows');
  await expect(page.locator('[data-testid="connection-mcp-rows-where"]')).toContainText(
    'sets its own threshold in the Advanced tab',
  );
});

test('postgres without one: empty field with the app default as placeholder, MCP tab shows the default', async ({
  relaunch,
}) => {
  const page = await open(relaunch, 'postgres', null, 'advanced');
  await expect(page.locator(field)).toHaveValue('');
  await expect(page.locator(field)).toHaveAttribute('placeholder', '100,000');
  await expect(page.locator('[data-testid="connection-explain-threshold-reset"]')).toBeDisabled();
  await page.locator('[data-testid="connection-tab-mcp"]').click();
  await expect(page.locator('[data-testid="connection-mcp-rows-who"]')).toContainText(
    '100,000 rows',
  );
  await expect(page.locator('[data-testid="connection-mcp-rows-where"]')).toContainText(
    'This is the app default',
  );
});

test('typing a value below the minimum shows the range error', async ({ relaunch }) => {
  const page = await open(relaunch, 'postgres', null, 'advanced');
  await page.locator(field).fill('10');
  await expect(page.locator('[data-testid="connection-explain-threshold-error"]')).toContainText(
    '1,000–1,000,000,000',
  );
});

test('clickhouse explains its higher counts; sqlite and kafka have no field', async ({
  relaunch,
}) => {
  const ch = await open(relaunch, 'clickhouse', null, 'advanced');
  await expect(ch.locator('[data-testid="connection-explain-threshold-engine"]')).toContainText(
    'ClickHouse counts every row in the parts it would read',
  );
  const lite = await open(relaunch, 'sqlite', null, 'advanced');
  await expect(lite.locator(field)).toHaveCount(0);
  const kafka = await open(relaunch, 'kafka', null, 'advanced');
  await expect(kafka.locator(field)).toHaveCount(0);
});

test('contract: list_connections reports the override or the app default', async () => {
  const rows = listed();
  const own = rows.find((r) => r.name === 'Own');
  const plain = rows.find((r) => r.name === 'Plain');
  expect(own?.expensiveQuery.rowsThreshold).toBe(explainThresholdOf(5000, 1000));
  expect(plain?.expensiveQuery.rowsThreshold).toBe(explainThresholdOf(0, 1000));
  expect(explainThresholdOf(undefined, APP_DEFAULT)).toBe(APP_DEFAULT);
});
