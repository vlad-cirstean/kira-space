import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import {
  POSTGRES_CAPS,
  postgresConnectionSummary,
  ROOT_CHILDREN,
  SERVER_VERSION,
} from './support/postgresFixture';
import { connectionRow, settleFrames } from './support/tree';

// P308: connections reorder by pointer drag (project/connectionReorder.ts), Alt+Up/Down and the
// context menu. The Go half is dbflow TestConnectionOrder; its contract fixes the call's args.

const A = postgresConnectionSummary('conn-a', 'Alpha DB', 'blue');
const B = postgresConnectionSummary('conn-b', 'Bravo DB', 'cyan');
const C = postgresConnectionSummary('conn-c', 'Charlie DB', 'green');
const LIST: ControlSnapshot = { channel: IPC.connectionsList, response: [A, B, C] };

const orderOf = (page: Page) =>
  connectionRow(page).evaluateAll((els) => els.map((el) => el.getAttribute('data-connection-id')));

const reorderCalls = (control: { log(): { channel: string; args?: unknown }[] }) =>
  control.log().filter((e) => e.channel === IPC.connectionsReorder);

/** Press on `from`, cross the drag tolerance, then park over `to`'s top or bottom quarter. */
async function dragTo(
  page: Page,
  from: ReturnType<typeof connectionRow>,
  to: ReturnType<typeof connectionRow>,
  edge: 'top' | 'bottom',
): Promise<void> {
  const src = await from.boundingBox();
  const dst = await to.boundingBox();
  if (!src || !dst) throw new Error('row has no box');
  const x = src.x + src.width * 0.6;
  await page.mouse.move(x, src.y + src.height / 2);
  await page.mouse.down();
  await page.mouse.move(x, src.y + src.height / 2 + 10, { steps: 4 });
  await page.mouse.move(x, dst.y + dst.height * (edge === 'top' ? 0.25 : 0.75), { steps: 10 });
  await settleFrames(page);
}

test('drag a connection above another: drop line, one reorder call, new order', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [LIST, { channel: IPC.connectionsReorder, response: [C, A, B] }],
  });
  await expect(connectionRow(page)).toHaveCount(3);

  await dragTo(page, connectionRow(page, 'Charlie'), connectionRow(page, 'Alpha'), 'top');
  await expect(
    connectionRow(page, 'Alpha').locator('[data-testid="tree-drop-indicator"]'),
  ).toBeVisible();
  await page.mouse.up();

  await expect.poll(() => orderOf(page)).toEqual(['conn-c', 'conn-a', 'conn-b']);
  await expect(page.locator('[data-testid="tree-drop-indicator"]')).toHaveCount(0);
  const calls = reorderCalls(control);
  expect(calls).toHaveLength(1);
  expect(calls[0].args).toEqual({
    ...contract<object>('connection-order', 'args:ConnectionsService.Reorder'),
    ids: ['conn-c', 'conn-a', 'conn-b'],
  });
});

test('dropping on a child row, or pressing Escape, reorders nothing', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      LIST,
      {
        channel: IPC.connectionsConnect,
        args: { id: A.id },
        response: {
          connectionId: A.id,
          status: 'connected',
          serverVersion: SERVER_VERSION,
          error: null,
          since: 1735689600000,
          caps: POSTGRES_CAPS,
        },
      },
      {
        channel: IPC.treeChildren,
        args: { connectionId: A.id, path: '', refresh: false },
        response: { nodes: ROOT_CHILDREN, source: 'server', truncated: false },
      },
    ],
  });
  await connectionRow(page, 'Alpha').locator('[data-testid="tree-twisty"]').click();
  const child = page.locator('[data-testid="tree-row"][data-path="database:kira_test"]');
  await expect(child).toBeVisible();

  await dragTo(page, connectionRow(page, 'Bravo'), child, 'top');
  await expect(page.locator('[data-testid="tree-drop-indicator"]')).toHaveCount(0);
  await page.mouse.up();

  await dragTo(page, connectionRow(page, 'Bravo'), connectionRow(page, 'Alpha'), 'top');
  await expect(page.locator('[data-testid="tree-drop-indicator"]')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="tree-drop-indicator"]')).toHaveCount(0);
  await page.mouse.up();

  expect(reorderCalls(control)).toHaveLength(0);
  await expect.poll(() => orderOf(page)).toEqual(['conn-a', 'conn-b', 'conn-c']);
});

test('menu Move up is disabled on the first row; Alt+ArrowDown moves and keeps focus', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({
    control: [LIST, { channel: IPC.connectionsReorder, response: [B, A, C] }],
  });
  const alpha = connectionRow(page, 'Alpha');

  await alpha.click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-move-up"]')).toHaveAttribute(
    'data-disabled',
    '',
  );
  await expect(page.locator('[data-testid="menu-item-move-down"]')).not.toHaveAttribute(
    'data-disabled',
    '',
  );
  await expect(page.locator('[data-testid="menu-item-move-down-shortcut"]')).toHaveText(/Alt|⌥/);
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

  await alpha.focus();
  await page.keyboard.press('Alt+ArrowDown');
  await expect.poll(() => orderOf(page)).toEqual(['conn-b', 'conn-a', 'conn-c']);
  await expect(connectionRow(page, 'Alpha')).toBeFocused();
  expect(reorderCalls(control)).toHaveLength(1);
  expect(reorderCalls(control)[0].args).toMatchObject({ ids: ['conn-b', 'conn-a', 'conn-c'] });
});

test('an active search pauses drag and the menu moves', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: [LIST] });
  await page.click('[data-testid="toggle-search"]');
  await page.fill('[data-testid="tree-search"]', 'DB');
  await expect(connectionRow(page)).toHaveCount(3);

  await dragTo(page, connectionRow(page, 'Charlie'), connectionRow(page, 'Alpha'), 'top');
  await expect(page.locator('[data-testid="tree-drop-indicator"]')).toHaveCount(0);
  await page.mouse.up();
  expect(reorderCalls(control)).toHaveLength(0);

  await connectionRow(page, 'Bravo').click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-move-up"]')).toHaveAttribute(
    'data-disabled',
    '',
  );
});

test('a rejected reorder restores the persisted order', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({
    control: [
      LIST,
      LIST,
      { channel: IPC.connectionsReorder, error: { code: 'E_NOT_FOUND', message: 'gone' } },
    ],
  });
  await dragTo(page, connectionRow(page, 'Charlie'), connectionRow(page, 'Alpha'), 'top');
  await page.mouse.up();
  await expect.poll(() => reorderCalls(control).length).toBe(1);
  await expect.poll(() => orderOf(page)).toEqual(['conn-a', 'conn-b', 'conn-c']);
});
