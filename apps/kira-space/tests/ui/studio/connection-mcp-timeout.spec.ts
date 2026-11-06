import type { ApprovalItem } from '@shared/domain/approvals';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import { statementTimeoutEnforcementOf } from '@shared/domain/dbmcp';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectionRow } from './support/tree';

// The statement timeout field, its enforcement lines and the exemption dialog line. Backend half:
// dbmcpflow TestDbMcpListConnectionsStatementTimeout and TestDbMcpStatementTimeoutExemption, whose
// dbmcp-statement-timeout contract pins the per-engine enforcement and the exemption item.

const FILE = 'local.yaml';
const dlg = '[data-testid="connection-file-dialog"]';

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const rows = () =>
  contract<{ kind: string; statementTimeout: { enforcement: Record<string, string> } }[]>(
    'dbmcp-statement-timeout',
    'list_connections',
  );

interface Spec {
  kind: string;
  timeout?: number;
  throttle?: number;
}

function fileOf({ kind, timeout, throttle }: Spec) {
  const mcp: Record<string, unknown> = { enabled: true };
  const lines = ['name: Pg', `kind: ${kind}`];
  if (throttle !== undefined) lines.push(`throttle_per_sec: ${throttle}`);
  lines.push('mcp:', '  enabled: true');
  if (timeout !== undefined) {
    mcp.statement_timeout_sec = timeout;
    lines.push(`  statement_timeout_sec: ${timeout}`);
  }
  const value: Record<string, unknown> = { name: 'Pg', kind, mcp };
  if (throttle !== undefined) value.throttle_per_sec = throttle;
  return { text: `${lines.join('\n')}\n`, value };
}

async function openTimeoutTab(
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

const field = (page: import('@playwright/test').Page) =>
  page.locator('[data-testid="connection-mcp-timeout"]');
const enforcement = (page: import('@playwright/test').Page) =>
  page.locator('[data-testid="connection-mcp-timeout-enforcement"]');

test('the field defaults to 10 and states what a hit does', async ({ relaunch }) => {
  const page = await openTimeoutTab(relaunch, { kind: 'postgres' });
  await expect(field(page)).toHaveValue('10');
  await expect(page.locator('[data-testid="connection-mcp-timeout-description"]')).toContainText(
    '0 turns the timeout off. Default 10 s.',
  );
  await expect(enforcement(page)).toContainText('The database stops the statement itself.');
  await expect(enforcement(page)).toContainText('approve it in a dialog');
  await expect(enforcement(page)).toContainText('queued behind other ops does not count');
});

test('0 turns the timeout off and says so', async ({ relaunch }) => {
  const page = await openTimeoutTab(relaunch, { kind: 'postgres', timeout: 0 });
  await expect(field(page)).toHaveValue('0');
  await expect(enforcement(page)).toContainText('No timeout.');
  await expect(enforcement(page)).not.toContainText('approve it in a dialog');
});

test('a value over the range shows the range error', async ({ relaunch }) => {
  const page = await openTimeoutTab(relaunch, { kind: 'postgres', timeout: 4000 });
  await expect(page.locator('[data-testid="connection-mcp-timeout-error"]')).toContainText(
    'from 1 to 3,600',
  );
});

test('contract: the enforcement line follows the engine split in list_connections', async ({
  relaunch,
}) => {
  const byKind = new Map(rows().map((r) => [r.kind, r.statementTimeout.enforcement]));
  for (const kind of ['postgres', 'mysql', 'sqlite', 'kafka']) {
    expect(statementTimeoutEnforcementOf(kind)).toEqual(byKind.get(kind));
  }
  const texts: [string, string][] = [
    ['postgres', 'The database stops the statement itself.'],
    ['mysql', 'The database stops read statements. This app stops write, ddl statements'],
    ['sqlite', 'This app stops the statement with its cancel path.'],
    ['kafka', 'agents run no statements here'],
  ];
  for (const [kind, text] of texts) {
    const page = await openTimeoutTab(relaunch, { kind });
    await expect(enforcement(page)).toContainText(text);
  }
});

test('the throttle note shows with a long or no timeout, not with a short one', async ({
  relaunch,
}) => {
  const off = await openTimeoutTab(relaunch, { kind: 'postgres', timeout: 0, throttle: 2 });
  await expect(off.locator('[data-testid="connection-mcp-timeout-throttle"]')).toBeVisible();
  const short = await openTimeoutTab(relaunch, { kind: 'postgres', timeout: 10, throttle: 2 });
  await expect(short.locator('[data-testid="connection-mcp-timeout-throttle"]')).toHaveCount(0);
});

test('contract: the exemption dialog names the timeout limit and the no-limit run', async ({
  relaunch,
}) => {
  const it = contract<ApprovalItem>('dbmcp-statement-timeout', 'item');
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const card = page.locator(`[data-testid="approval-item-${it.id}"]`);
  await expect(card.locator('[data-testid="db-mcp-exemption-limit"]')).toContainText(
    'Statement timeout:',
  );
  await expect(card.locator('[data-testid="db-mcp-exemption-limit"]')).toContainText('limit 1');
  await expect(card.locator('[data-testid="db-mcp-exemption-no-timeout"]')).toContainText(
    'no time limit',
  );
  await expect(page.locator(`[data-testid="approval-approve-${it.id}"]`)).toHaveText(
    'Lift for this query',
  );
  await expect(page.locator(`[data-testid="approval-deny-${it.id}"]`)).toHaveText('Keep the limit');
});
