import type { Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { connectAndExpand, connectionCreateArgs, openConsoleFromMenu } from './support/connect';
import { typeInto } from './support/editor';
import { gridCell } from './support/grid';
import {
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// Decision 8: closing a tab that holds staged grid edits or an open console transaction asks first.
// Cancel keeps the tab and its edits; confirming discards the edits, or rolls the transaction back.

const CONNECTION_ID = 'conn-staged-close';
const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Close DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Close DB', 'green'),
  },
  ...FIXTURE.control,
];

const executeAt = (statement: string): PortSnapshot => ({
  op: DATA_OP.execute,
  payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, statements: [statement] },
  response: { kind: 'execute', pages: [] },
});

const tabClose = (page: Page, kind: string) =>
  page.locator(`[data-testid="tab"][data-tab-kind="${kind}"] [data-testid="tab-close"]`);

test('closing a grid tab with staged edits asks, and cancel keeps them', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndExpand(page, 'Close DB', 'green');
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await gridCell(page, 0, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('staged');
  await page.keyboard.press('Enter');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);

  await tabClose(page, 'data').click();
  const dialog = page.locator('[data-testid="confirm-dialog"]');
  await expect(dialog).toBeVisible();
  await page.click('[data-testid="confirm-dialog-cancel"]');
  await expect(dialog).toHaveCount(0);
  await expect(tabClose(page, 'data')).toHaveCount(1);
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);

  await tabClose(page, 'data').click();
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expect(tabClose(page, 'data')).toHaveCount(0);
});

test('closing a console tab with an open transaction asks, and confirm rolls it back', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [...FIXTURE.port, executeAt('BEGIN'), executeAt('ROLLBACK')],
  });
  await connectAndExpand(page, 'Close DB', 'green');
  await openConsoleFromMenu(page, COMPOSITE_PK_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, 'BEGIN;');
  await page.click('[data-testid="console-run-statement"]');
  await expect
    .poll(async () => (await stream.ops()).filter((o) => o.op === DATA_OP.execute).length)
    .toBe(1);
  await expect(page.locator('[data-testid="console-stop"]')).toBeDisabled();

  await tabClose(page, 'console').click();
  const dialog = page.locator('[data-testid="confirm-dialog"]');
  await expect(dialog).toBeVisible();
  await page.click('[data-testid="confirm-dialog-cancel"]');
  await expect(tabClose(page, 'console')).toHaveCount(1);

  await tabClose(page, 'console').click();
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expect(tabClose(page, 'console')).toHaveCount(0);
  await expect
    .poll(
      async () =>
        (await stream.ops()).filter(
          (o) => o.op === DATA_OP.execute && JSON.stringify(o.payload).includes('"ROLLBACK"'),
        ).length,
    )
    .toBe(1);
});
