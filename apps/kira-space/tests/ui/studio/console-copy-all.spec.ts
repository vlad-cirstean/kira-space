// J-CONS-024 UI half: Copy all of a Mongo result page writes the four formats the flow test
// recorded (contract studio-console-mongo), byte for byte.
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { installClipboardSpy, lastClipboardWrite } from './support/clipboard';
import { createConnection, openConsoleFromMenu } from './support/connect';
import { mongoCreateArgs } from './support/connectionArgs';
import { typeInto } from './support/editor';
import {
  DB_PATH,
  connectAndExpandControl as mongoConnectAndExpandControl,
  mongoConnectionSummary,
  WIDGETS_PATH,
} from './support/mongoFixture';
import { connectionRow, expandRow, openRowMenu } from './support/tree';

const scenario = 'studio-console-mongo';
const result = contract<{ statement: string; ids: string[]; bodies: string[] }>(scenario, 'page');
const copy = contract<Record<'canonical' | 'relaxed' | 'plain' | 'shell', string>>(
  scenario,
  'copy',
);
// The flow contract keeps dates with a space so its timestamp mask leaves them alone.
const unmask = (s: string) => s.replace('2001-02-03 04:05:06.000Z', '2001-02-03T04:05:06.000Z');

test('Copy all writes canonical, relaxed, plain and shell text equal to the contract', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-copy-all';
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: mongoCreateArgs('Copy All'),
      response: mongoConnectionSummary(CONNECTION_ID, 'Copy All', 'green'),
    },
    ...mongoConnectAndExpandControl(CONNECTION_ID),
  ];
  const PORT: PortSnapshot[] = [
    {
      op: DATA_OP.execute,
      payload: { connectionId: CONNECTION_ID, path: WIDGETS_PATH, statements: [result.statement] },
      response: {
        kind: 'execute',
        pages: [
          {
            kind: 'document',
            ids: result.ids,
            bodies: result.bodies,
            position: {
              offset: 0,
              pageSize: result.bodies.length,
              hasMore: false,
              nextToken: null,
              prevToken: null,
              strategy: 'offset',
            },
          },
        ],
      },
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await installClipboardSpy(page, { lazy: true });
  await createConnection(page, 'mongodb', 'Copy All');
  await expect(connectionRow(page)).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await openConsoleFromMenu(page, WIDGETS_PATH);

  const view = page.locator('[data-testid="console-view"]');
  await typeInto(view, page, result.statement);
  await page.click('[data-testid="console-run-statement"]');
  const rows = view.locator('[data-testid="console-result-doc-row"]');
  await expect(rows).toHaveCount(result.bodies.length);

  const formats = [
    ['copy-all-as-json', 'canonical'],
    ['copy-all-relaxed-json', 'relaxed'],
    ['copy-all-plain-json', 'plain'],
    ['copy-all-shell', 'shell'],
  ] as const;
  for (const [item, key] of formats) {
    await rows.nth(0).click({ button: 'right' });
    await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
    await page.locator('[data-testid="menu-item-copy-all-submenu"]').hover();
    await expect(page.locator('[data-testid="context-submenu"]')).toBeVisible();
    await page.click(`[data-testid="menu-item-${item}"]`);
    await expect.poll(() => lastClipboardWrite(page)).toBe(unmask(copy[key]));
  }
});
