import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from '../../ui/studio/fixtures';
import { createConnection } from '../../ui/studio/support/connect';
import { mariadbCreateArgs } from '../../ui/studio/support/connectionArgs';
import { IPC } from '../../ui/studio/support/ipcChannels';
import { DB_PATH, mariadbConnectionSummary, orderItemsFixture } from '../../ui/studio/support/mariadbFixture';
import { connectionRow, expandRow, openRowMenu } from '../../ui/studio/support/tree';
import { settleFonts } from './support/settle';

// v1.4 P6 (docs/HISTORY.md (v1.4 P6)): the SQL console at rest with
// syntax-highlighted content — CodeMirror's own decoration/theme layer has no existing pixel guard;
// console.spec.ts/console-format.spec.ts/console-explain.spec.ts all assert *behaviour* only.

const CONNECTION_ID = 'conn-visual-console';
const CONNECTION_SUMMARY = mariadbConnectionSummary(CONNECTION_ID, 'VisualConsole', 'blue');
const FIXTURE = orderItemsFixture(CONNECTION_ID);

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: mariadbCreateArgs('VisualConsole'),
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
];

async function connectAndExpand(page: Page): Promise<void> {
  await createConnection(page, 'mariadb', 'VisualConsole');

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
}

const SAMPLE_QUERY = [
  '-- order totals by customer',
  'select customer_id, sum(quantity * unit_price) as total',
  'from order_items',
  "where status = 'shipped'",
  'group by customer_id',
  'order by total desc;',
].join('\n');

test('SQL console at rest with syntax-highlighted content (P6)', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await connectAndExpand(page);

  await openRowMenu(page, DB_PATH);
  await page.click('[data-testid="menu-item-open-console"]');
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  await view.locator('.view-lines').click();
  await page.keyboard.type(SAMPLE_QUERY);
  await expect(view.locator('.view-lines')).toContainText('order by total desc');

  // Monaco's caret blink and its vertical scrollbar fade are JS-driven, so two captures of the same
  // state differ and never form a stable pair. Blur the editor (no caret) and mask the scrollbar.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await expect(view.locator('.monaco-editor.focused')).toHaveCount(0);
  await settleFonts(page);
  await expect(page).toHaveScreenshot('console.png', {
    mask: [view.locator('.monaco-scrollable-element > .scrollbar.vertical')],
  });
});
