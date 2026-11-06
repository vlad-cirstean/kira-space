import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { connectAndExpand, connectionCreateArgs, openConsoleFromMenu } from './support/connect';
import { contract } from './support/contract';
import { typeInto } from './support/editor';
import { IPC } from './support/ipcChannels';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { connectionRow } from './support/tree';

// The ops throttle explanation in the connection dialog (Advanced and MCP tabs) and the busy
// error a queued op gets. Backend half: dbmcpflow TestThrottleSerialAcrossUserAndMcp,
// TestThrottlePacesReads and TestThrottleCancelFreesSlot, plus the adapterhost unit tests that
// pin the error texts.

const FILE = 'local.yaml';
const dlg = '[data-testid="connection-file-dialog"]';
const DESCRIPTION_START = 'Ops per second this app admits to the database for this connection';

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');

async function open(
  relaunch: Parameters<Parameters<typeof test>[2]>[0]['relaunch'],
  throttle: number | null,
  tab: 'mcp' | 'advanced',
) {
  const lines = ['name: Pg', 'kind: sqlite', 'mcp:', '  enabled: true'];
  const value: Record<string, unknown> = { name: 'Pg', kind: 'sqlite', mcp: { enabled: true } };
  if (throttle !== null) {
    lines.splice(2, 0, `throttle_per_sec: ${throttle}`);
    value.throttle_per_sec = throttle;
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

test('the Advanced tab explains the throttle', async ({ relaunch }) => {
  const page = await open(relaunch, 2, 'advanced');
  const description = page.locator('[data-testid="connection-throttle-description"]');
  await expect(description).toContainText(DESCRIPTION_START);
  await expect(description).toContainText('Ops run one at a time');
  await expect(description).toContainText('0 turns the throttle off');
});

test('the MCP tab names the shared throttle', async ({ relaunch }) => {
  const on = await open(relaunch, 2, 'mcp');
  const line = on.locator('[data-testid="connection-mcp-throttle"]');
  await expect(line).toContainText('share this connection’s throttle of 2 ops per second');
  await expect(line).toContainText('Advanced tab');
  await expect(line).toContainText(DESCRIPTION_START);
});

test('the MCP tab says MCP calls are not throttled at 0', async ({ relaunch }) => {
  const page = await open(relaunch, 0, 'mcp');
  const line = page.locator('[data-testid="connection-mcp-throttle"]');
  await expect(line).toContainText('MCP calls are not throttled on this connection');
  await expect(line).toContainText('0 turns the throttle off');
});

test('a console run that waits too long for the connection shows the busy error', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-throttle';
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const BUSY =
    'Waited 30 s for another operation on this connection to finish. A throttled connection runs one operation at a time. Retry, or raise or turn off the throttle in the connection settings.';
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Throttle DB', 'green'),
      response: postgresConnectionSummary(CONNECTION_ID, 'Throttle DB', 'green'),
    },
    ...FIXTURE.control,
  ];
  const PORT: PortSnapshot[] = [
    ...FIXTURE.port,
    {
      op: DATA_OP.execute,
      payload: { connectionId: CONNECTION_ID, path: ORDER_ITEMS_PATH, statements: ['SELECT 1'] },
      error: { code: 'E_TIMEOUT', message: BUSY },
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Throttle DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const consoleView = page.locator('[data-testid="console-view"]');
  await typeInto(consoleView, page, 'SELECT 1;');
  await page.click('[data-testid="console-run-all"]');
  const error = consoleView.locator('[data-testid="console-error"]');
  await expect(error).toBeVisible();
  await expect(error).toContainText('runs one operation at a time');
});
