import type { Locator, Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { createConnection } from './support/connect';
import { IPC } from './support/ipcChannels';
import {
  DB_PATH as MONGO_DB_PATH,
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from './support/mongoFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// DocumentView.vue used to gate Add/Edit/Delete on Caps alone
// (a static per-adapter literal, internal/adapters/mongo/caps.go, that never narrows for a
// connection's own read-only flag) — grid/keyvalue/browse's own write gates already combine the
// two (KeyValueView.vue's own canUpdate/canDelete/canInsert), but this view didn't, so the
// buttons stayed fully enabled on a read-only MongoDB connection and only failed server-side with
// a raw E_UNSUPPORTED. Mirrors tests/ui/mutations.spec.ts's and tooltips.spec.ts's own
// read-only-connection scenario for the data grid, applied to the document view instead.

const CONNECTION_ID = 'conn-doc-ro';
const CONNECTION_SUMMARY = {
  ...mongoConnectionSummary(CONNECTION_ID, 'Mongo RO', 'red'),
  readOnly: true,
};
const FIXTURE = widgetsFixture(CONNECTION_ID);

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: {
      name: 'Mongo RO',
      kind: 'mongodb',
      color: 'red',
      mode: 'fields',
      readOnly: true,
      host: '127.0.0.1',
      port: 27017,
      database: 'kira_test',
      username: 'kira',
      password: null,
      uri: null,
      options: {},
      preconnect: null,
      preconnectSidecar: false,
      preconnectDelaySec: 0,
      autoExplain: false,
      throttlePerSec: 0,
    },
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
];

// reka generates the content id itself (no fixed "app-tooltip" id any more, tooltips.spec.ts's
// own §6.5 migration note) — `[data-slot="tooltip-content"]` is ui/tooltip's own TooltipContent.vue
// marker, not tied to any one call site's DOM.
const tooltip = (page: Page): Locator => page.locator('[data-slot="tooltip-content"]');

async function assertTooltipShows(
  page: Page,
  trigger: Locator,
  text: string | RegExp,
): Promise<void> {
  await trigger.hover();
  await expect(tooltip(page)).toBeVisible({ timeout: 3_000 });
  // toContainText, not toHaveText: reka's TooltipContent renders a visually-hidden a11y mirror
  // span alongside the visible text, so a bare .textContent read sees the text doubled.
  await expect(tooltip(page)).toContainText(text);
}

test('a read-only MongoDB connection disables Add/Edit/Delete on the document view', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });

  await createConnection(page, 'mongodb', 'Mongo RO');

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, MONGO_DB_PATH);

  await (await findRow(page, WIDGETS_PATH)).dblclick();
  const view = page.locator('[data-testid="document-view"]');
  await expect(view).toBeVisible();
  await expect(page.locator('[data-testid="document-row"]').first()).toBeVisible({
    timeout: 15_000,
  });

  // Real-interaction fix (reported bug — the pager sits on the right instead of where it made
  // sense before): mirrors DataToolbar.vue — the pager sits
  // at the toolbar's reading edge, immediately before the page-size picker it pages through,
  // rather than alone at the far #toolbar-end. Relative positions, not pixel ones
  // (data-view.spec.ts's own pattern), so a token or spacing change still cannot break this.
  const pagerBox = await page.locator('[data-testid="document-pager"]').boundingBox();
  const pageSizeBox = await page.locator('[data-testid="document-page-size-picker"]').boundingBox();
  if (!pagerBox || !pageSizeBox) {
    throw new Error('document-pager or document-page-size-picker has no bounding box');
  }
  expect(pagerBox.x).toBeLessThan(pageSizeBox.x);

  const addButton = page.locator('[data-testid="document-add"]');
  await expect(addButton).toBeDisabled();
  // The disabled <button> itself receives no pointer events in Blink, so the never-disabled
  // wrapper <span> is the real trigger and hit target (tooltips.spec.ts's own pattern).
  await assertTooltipShows(page, addButton.locator('xpath=..'), 'Connection is read-only');

  const deleteButton = page.locator('[data-testid="document-delete"]').first();
  await expect(deleteButton).toBeDisabled();

  const editButton = page.locator('[data-testid="document-edit"]').first();
  await expect(editButton).toBeDisabled();
  await assertTooltipShows(page, editButton.locator('xpath=..'), 'Connection is read-only');
});
