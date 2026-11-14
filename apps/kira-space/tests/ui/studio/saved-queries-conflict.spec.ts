// Decision 3 (OQ-CONS-4): a saved query another window changed is not overwritten unseen. UI half
// of dbflow TestSavedQueryTwoWindows; the conflict error and the confirmed answer are its records.
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { connectAndExpand, connectionCreateArgs, openConsoleFromMenu } from './support/connect';
import { acceptConfirm, cancelConfirm } from './support/dialogs';
import {
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';

const CONNECTION_ID = 'conn-saved-conflict';
const scenario = 'saved-query-two-windows';
const conflict = contract<{ code: string; message: string; details: { changed: boolean } }>(
  scenario,
  'Update#changed',
);
const guarded = contract<{ expect: { name: string; pinned: boolean }; name: string }>(
  scenario,
  'args:Update#guarded',
);
const confirmed = contract<{ name: string; body: { text: string } }>(scenario, 'Update#confirmed');

const STALE = {
  kind: 'console' as const,
  id: 'saved-1',
  connectionId: CONNECTION_ID,
  path: ORDER_ITEMS_PATH,
  name: guarded.expect.name,
  pinned: guarded.expect.pinned,
  createdAt: '2026-01-01T00:00:00.000Z',
  usedAt: null,
  body: confirmed.body,
};
const CURRENT = { ...STALE, name: 'report a' };
const LIST_ARGS = { connectionId: CONNECTION_ID, path: ORDER_ITEMS_PATH };
const error = { code: conflict.code, message: conflict.message, details: conflict.details };

test('a rename of a query another window changed asks first and writes only after confirm', async ({
  relaunch,
}) => {
  const FIXTURE = orderItemsFixture(CONNECTION_ID);
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      {
        channel: IPC.connectionsCreate,
        args: connectionCreateArgs('Conflict DB', 'green'),
        response: postgresConnectionSummary(CONNECTION_ID, 'Conflict DB', 'green'),
      },
      ...FIXTURE.control,
      { channel: IPC.queriesListConsole, args: LIST_ARGS, response: [STALE] },
      // Guarded write refused: another window renamed the query.
      {
        channel: IPC.queriesUpdate,
        args: { id: STALE.id, name: guarded.name, pinned: null, expect: guarded.expect },
        error,
      },
      { channel: IPC.queriesListConsole, args: LIST_ARGS, response: [CURRENT] },
      // The reloaded entry carries the other window's name; this attempt is confirmed, and the
      // same write goes again without its guard.
      {
        channel: IPC.queriesUpdate,
        args: {
          id: STALE.id,
          name: guarded.name,
          pinned: null,
          expect: { name: CURRENT.name, pinned: CURRENT.pinned },
        },
        error,
      },
      {
        channel: IPC.queriesUpdate,
        args: { id: STALE.id, name: guarded.name, pinned: null, expect: null },
        response: { ...STALE, name: confirmed.name },
      },
      {
        channel: IPC.queriesListConsole,
        args: LIST_ARGS,
        response: [{ ...STALE, name: confirmed.name }],
      },
    ],
    stream: FIXTURE.port,
  });
  await connectAndExpand(page, 'Conflict DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);

  const updates = () => control.log().filter((e) => e.channel === IPC.queriesUpdate);
  const entry = page.locator('[data-testid="console-saved-entry"]');
  const rename = async () => {
    await entry.hover();
    await entry.getByRole('button', { name: 'Rename' }).click();
    await page.fill('[data-testid="text-prompt-input"]', guarded.name);
    await page.click('[data-testid="text-prompt-ok"]');
  };

  await page.click('[data-testid="console-saved-toggle"]');
  await expect(entry).toContainText(STALE.name);

  // First attempt: decline. The dialog names the conflict; the list reloads to the other window's name.
  await rename();
  await expect(page.locator('[data-testid="confirm-dialog"]')).toContainText(conflict.message);
  await cancelConfirm(page);
  await expect(entry).toContainText(CURRENT.name);
  expect(updates()).toHaveLength(1);

  // Second attempt guards on the reloaded name; confirming resends without the guard.
  await rename();
  await acceptConfirm(page);
  await expect(entry).toContainText(confirmed.name);
  expect(updates()).toHaveLength(3);
});
