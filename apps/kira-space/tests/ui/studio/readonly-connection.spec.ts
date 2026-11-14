// J-CONN-006 UI half: the console shows the refusal a read-only connection answers a write with
// (contract studio-readonly, written by dbflow TestReadOnlySQLiteRefusesWrites), verbatim.
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { createConnection, newConnectionArgs, openConsoleFromMenu } from './support/connect';
import { connectAndExpandControl, connectionSummary } from './support/engineFixture';
import { connectionRow, openRowMenu } from './support/tree';

interface Refusal {
  code: string;
  message: string;
  statement: string;
}
const scenario = 'studio-readonly';
const insert = contract<Refusal>(scenario, 'sqlite#console#insert');
const drop = contract<Refusal>(scenario, 'sqlite#console#drop');

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

test('a write on a read-only connection shows the backend refusal and nothing else', async ({
  relaunch,
}) => {
  const ID = 'conn-readonly';
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: newConnectionArgs('Read only DB', 'sqlite', 'ro.db'),
      response: {
        ...connectionSummary('sqlite', 0, 'ro.db', '', ID, 'Read only DB', 'none'),
        readOnly: true,
      },
    },
    ...connectAndExpandControl(ID, 'SQLite 3.46', CAPS, []),
  ];
  const refused = (r: Refusal): PortSnapshot => ({
    op: DATA_OP.execute,
    payload: { connectionId: ID, path: '', statements: [r.statement] },
    error: { code: r.code, message: r.message },
  });
  const { window: page } = await relaunch({
    control: CONTROL,
    stream: [refused(insert), refused(drop)],
  });

  await createConnection(page, 'sqlite', 'Read only DB', 'ro.db');
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
  const error = view.locator('[data-testid="console-error"]');
  for (const r of [insert, drop]) {
    await view.locator('.view-lines').click();
    await page.keyboard.press('ControlOrMeta+a');
    await page.keyboard.insertText(r.statement);
    await page.click('[data-testid="console-run-all"]');
    await expect(error).toContainText(r.message);
    await expect(view.locator('[data-testid="console-result-grid"]')).toHaveCount(0);
  }
});
