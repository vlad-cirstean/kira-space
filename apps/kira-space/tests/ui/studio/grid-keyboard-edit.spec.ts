import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { installClipboardShim } from './support/clipboard';
import { connectionCreateArgs, createConnection } from './support/connect';
import { cellText, gridCell } from './support/grid';
import {
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// J-GRID-039: a keyboard-only edit session. Delete on a nullable cell stages NULL; on a NOT NULL
// column it is refused with a visible reason and stages nothing.

const CONNECTION_ID = 'conn-grid-keyboard';
const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const OPS = [
  {
    kind: 'update',
    key: { entity_id: '1', tenant_id: '1' },
    changes: { name: null },
    expect: { name: 'tenant 1 / entity 1' },
  },
];

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Keyboard DB'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Keyboard DB', 'green'),
  },
  ...FIXTURE.control,
];

const PORT: PortSnapshot[] = [
  ...FIXTURE.port,
  {
    op: DATA_OP.mutate,
    payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, ops: OPS },
    response: { kind: 'mutate', affectedRows: 1 },
  },
  {
    op: DATA_OP.invalidate,
    payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, scope: 'pages' },
    response: { kind: 'invalidate' },
  },
];

test('Delete clears a nullable cell to NULL and refuses a NOT NULL column', async ({
  relaunch,
}) => {
  test.setTimeout(180_000);
  const { window: page, stream } = await relaunch({ control: CONTROL, stream: PORT });
  await installClipboardShim(page);
  await createConnection(page, 'postgres', 'Keyboard DB');
  await expect(connectionRow(page)).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await expandRow(page, 'database:kira_test');
  await expandRow(page, 'database:kira_test/schema:app');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible();

  // NOT NULL column: refused, nothing staged.
  await gridCell(page, 0, 'tenant_id').click();
  await page.keyboard.press('Delete');
  await expect(page.locator('[data-testid="data-action-error"]')).toContainText('tenant_id');
  await expect(gridCell(page, 0, 'tenant_id')).not.toHaveClass(/pending-edit/);

  // Nullable column: staged as NULL.
  await gridCell(page, 0, 'name').click();
  await page.keyboard.press('Delete');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  expect(await cellText(page, 0, 'name')).toBe('NULL');

  await page.click('[data-testid="toolbar-commit-changes"]');
  await expect
    .poll(async () => (await stream.ops()).filter((o) => o.op === DATA_OP.mutate).length)
    .toBe(1);
  const sent = (await stream.ops()).filter((o) => o.op === DATA_OP.mutate);
  expect((sent[0].payload as { ops: unknown[] }).ops).toEqual(OPS);
});
