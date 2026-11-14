import type { Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
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

const MIGRATION = contract<string[]>('studio-console-script', 'pg#migration');
const MIGRATION_ERROR = contract<{ code: string; message: string; details: string }>(
  'studio-console-script',
  'pg#migrationError',
);

// `transaction` is what Postgres reports after the statement (the response carries the engine's
// own state; the console takes it over reading the statement).
const executeAt = (statement: string, transaction: 'idle' | 'open'): PortSnapshot => ({
  op: DATA_OP.execute,
  payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, statements: [statement] },
  response: { kind: 'execute', pages: [], transaction },
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
    stream: [...FIXTURE.port, executeAt('BEGIN', 'open'), executeAt('ROLLBACK', 'idle')],
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

// UI half of dbflow.TestConsolePostgresTransactionAndCap: the migration stops at its third
// statement, and the error names it, so the BEGIN before it is known to have run and the
// transaction it left open (aborted on the server) is rolled back on close.
test('closing a console tab after a script that failed past its BEGIN asks, and confirm rolls back', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [
      ...FIXTURE.port,
      {
        op: DATA_OP.execute,
        payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, statements: MIGRATION },
        error: MIGRATION_ERROR,
      },
      executeAt('ROLLBACK', 'idle'),
    ],
  });
  await connectAndExpand(page, 'Close DB', 'green');
  await openConsoleFromMenu(page, COMPOSITE_PK_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, MIGRATION.join(';\n'), { paste: true });
  await page.click('[data-testid="console-run-all"]');
  await expect
    .poll(async () => (await stream.ops()).filter((o) => o.op === DATA_OP.execute).length)
    .toBe(1);
  await expect(page.locator('[data-testid="console-stop"]')).toBeDisabled();

  await tabClose(page, 'console').click();
  await expect(page.locator('[data-testid="confirm-dialog"]')).toBeVisible();
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

// Round-2 M1: the tab closes while its run is still in flight. The close cancels the run, which
// fails after its BEGIN with the engine reporting the transaction still open; the tab is gone by
// then, so the rollback goes out without a prompt to answer. The first statements of the script
// ran before the cancel reached the pg_sleep.
test('closing a console tab mid-run still rolls back the transaction the run opened', async ({
  relaunch,
}) => {
  const script = ['BEGIN', 'INSERT INTO app.t VALUES (1)', 'SELECT pg_sleep(60)'];
  const { window: page, stream } = await relaunch({
    control: CONTROL,
    stream: [
      ...FIXTURE.port,
      {
        op: DATA_OP.execute,
        payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, statements: script },
        error: {
          code: 'E_CANCELLED',
          message: 'cancelled',
          details: JSON.stringify({ statementIndex: 2, inTransaction: true }),
        },
        untilCancel: true,
      },
      executeAt('ROLLBACK', 'idle'),
    ],
  });
  await connectAndExpand(page, 'Close DB', 'green');
  await openConsoleFromMenu(page, COMPOSITE_PK_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, script.join(';\n'), { paste: true });
  await page.click('[data-testid="console-run-all"]');
  await expect
    .poll(async () => (await stream.ops()).filter((o) => o.op === DATA_OP.execute).length)
    .toBe(1);

  // Nothing is known to be open yet, so the tab closes without asking.
  await tabClose(page, 'console').click();
  await expect(tabClose(page, 'console')).toHaveCount(0);
  await expect(page.locator('[data-testid="confirm-dialog"]')).toHaveCount(0);
  await expect
    .poll(
      async () =>
        (await stream.ops()).filter(
          (o) => o.op === DATA_OP.execute && JSON.stringify(o.payload).includes('"ROLLBACK"'),
        ).length,
    )
    .toBe(1);
});
