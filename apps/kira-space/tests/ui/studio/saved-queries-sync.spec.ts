// P433 F4: saved-query menus read once and stay cached (staleTime: Infinity); a write announces
// itself on kira:savedQueries:changed and the menu refetches. Backend half: dbflow's SQLite
// journey asserts Go emits the event for every write.
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { connectAndExpand, connectionCreateArgs, openConsoleFromMenu } from './support/connect';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';

const CONNECTION_ID = 'conn-saved-sync';

test('a saved-queries change refetches the console menu once; reopening reads nothing', async ({
  relaunch,
}) => {
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const SAVED = {
    kind: 'console' as const,
    id: 'saved-1',
    connectionId: CONNECTION_ID,
    path: ORDER_ITEMS_PATH,
    name: 'From another window',
    pinned: false,
    createdAt: '2026-01-01T00:00:00.000Z',
    usedAt: null,
    body: { text: 'SELECT 1;' },
  };
  const LIST_ARGS = { connectionId: CONNECTION_ID, path: ORDER_ITEMS_PATH };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Saved DB', 'green'),
      response: postgresConnectionSummary(CONNECTION_ID, 'Saved DB', 'green'),
    },
    ...FIXTURE.control,
    { channel: IPC.queriesListConsole, args: LIST_ARGS, response: [] },
    { channel: IPC.queriesListConsole, args: LIST_ARGS, response: [SAVED] },
  ];
  const { window: page, control } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndExpand(page, 'Saved DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);

  const reads = () => control.log().filter((e) => e.channel === IPC.queriesListConsole).length;
  const toggle = page.locator('[data-testid="console-saved-toggle"]');
  const entries = page.locator('[data-testid="console-saved-entry"]');

  await toggle.click();
  await expect(page.locator('[data-testid="console-saved-menu"]')).toBeVisible();
  await expect(entries).toHaveCount(0);
  expect(reads()).toBe(1);
  await page.keyboard.press('Escape');

  // Closed and reopened with no write: served from cache.
  await toggle.click();
  await expect(page.locator('[data-testid="console-saved-menu"]')).toBeVisible();
  await expect(entries).toHaveCount(0);
  expect(reads()).toBe(1);
  await page.keyboard.press('Escape');

  // Another window saved a query.
  await emitWailsEvent(page, IPC.savedQueriesChanged, {
    connectionId: CONNECTION_ID,
    kind: 'console',
  });
  await toggle.click();
  await expect(entries).toHaveCount(1);
  await expect(entries.first()).toContainText('From another window');
  expect(reads()).toBe(2);
  await page.keyboard.press('Escape');

  await toggle.click();
  await expect(entries).toHaveCount(1);
  expect(reads()).toBe(2);
});
