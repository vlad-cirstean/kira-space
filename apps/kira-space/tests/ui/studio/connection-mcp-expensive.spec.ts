import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import { expensiveQueryCheckOf } from '@shared/domain/dbmcp';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import { openSettings } from './support/settings';
import { connectionRow } from './support/tree';

// The row-estimate check: its explanation in the MCP tab (who enforces, what a hit does, known limits),
// per-engine state and the exemption dialog line. Backend half: dbmcpflow TestDbMcpExpensiveQueryPostgres,
// TestDbMcpExpensiveQueryNoEstimate and TestDbMcpListConnectionsExpensiveQuery, whose dbmcp-expensive-query
// contract pins the per-engine check and the exemption item.

const FILE = 'local.yaml';
const dlg = '[data-testid="connection-file-dialog"]';

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const rows = () =>
  contract<
    { kind: string; name: string; expensiveQuery: { rowsThreshold: number; check: string } }[]
  >('dbmcp-expensive-query', 'list_connections');

interface Spec {
  kind: string;
  autoExplain?: boolean;
}

function fileOf({ kind, autoExplain }: Spec) {
  const mcp: Record<string, unknown> = { enabled: true };
  const lines = ['name: Pg', `kind: ${kind}`, 'mcp:', '  enabled: true'];
  if (autoExplain !== undefined) {
    mcp.auto_explain = autoExplain;
    lines.push(`  auto_explain: ${autoExplain}`);
  }
  return { text: `${lines.join('\n')}\n`, value: { name: 'Pg', kind, mcp } };
}

async function openMcpTab(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  spec: Spec,
) {
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

const at = (page: Page, id: string) => page.locator(`[data-testid="connection-mcp-rows-${id}"]`);

test('postgres: the check says who enforces, what a hit does and its limits', async ({
  relaunch,
}) => {
  const page = await openMcpTab(relaunch, { kind: 'postgres' });
  await expect(page.getByText('Check agent queries against the row threshold')).toBeVisible();
  await expect(at(page, 'threshold')).toContainText('estimated rows a query’s plan reads');
  await expect(at(page, 'who')).toContainText('Kira enforces this, not the database');
  await expect(at(page, 'who')).toContainText('100,000 rows');
  await expect(at(page, 'hit')).toContainText('refused before it runs');
  await expect(at(page, 'hit')).toContainText('you always approve it in a dialog');
  await expect(at(page, 'limits')).toContainText('ignore LIMIT');
  await expect(at(page, 'off')).toHaveCount(0);
});

test('unticking the check shows the off note', async ({ relaunch }) => {
  const page = await openMcpTab(relaunch, { kind: 'postgres' });
  await page.locator('[data-testid="connection-mcp-auto-explain"]').click();
  await expect(at(page, 'off')).toContainText('Agent queries run without a row check');
  await expect(at(page, 'who')).toHaveCount(0);
});

test('sqlite and kafka: no row estimate, no check', async ({ relaunch }) => {
  const lite = await openMcpTab(relaunch, { kind: 'sqlite' });
  await expect(at(lite, 'none')).toContainText('no row estimate');
  await expect(lite.locator('[data-testid="connection-mcp-auto-explain"]')).toBeEnabled();
  const kafka = await openMcpTab(relaunch, { kind: 'kafka' });
  await expect(at(kafka, 'none')).toContainText('no row estimate');
  await expect(kafka.locator('[data-testid="connection-mcp-auto-explain"]')).toBeDisabled();
});

test('contract: the check state follows list_connections', async () => {
  const all = rows();
  expect(all.length).toBeGreaterThan(0);
  for (const r of all) {
    const autoExplain = r.name !== 'e-postgres-off' && r.name !== 'notes db';
    expect(expensiveQueryCheckOf(r.kind, autoExplain), r.name).toBe(r.expensiveQuery.check);
  }
});

test('contract: the exemption dialog shows the estimate against the limit and the plan', async ({
  relaunch,
}) => {
  const it = contract<ApprovalItem>('dbmcp-expensive-query', 'item');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const card = page.locator(`[data-testid="approval-item-${it.id}"]`);
  await expect(card.locator('[data-testid="db-mcp-exemption-limit"]')).toHaveText(
    'Estimated rows read: 5,000, limit 1,000',
  );
  await expect(card.locator('[data-testid="db-mcp-approval-plan"]')).toBeVisible();
});

test('settings: Advanced explains the estimate and who acts on it', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await openSettings(page);
  await page.click('[data-testid="settings-section-Advanced"]');
  const description = page.locator('[data-testid="settings-expensive-query-rows-description"]');
  await expect(description).toContainText('not the rows it returns');
  await expect(description).toContainText('refuses an agent’s query before it runs');
});
