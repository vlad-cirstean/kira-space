import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from '../../ui/studio/fixtures';
import { createConnection } from '../../ui/studio/support/connect';
import { postgresCreateArgs } from '../../ui/studio/support/connectionArgs';
import { IPC } from '../../ui/studio/support/ipcChannels';
import {
  orderItemsFixture,
  postgresConnectionSummary,
} from '../../ui/studio/support/postgresFixture';
import { connectionRow, openRowMenu } from '../../ui/studio/support/tree';
import { settleFonts } from './support/settle';

// The Schema (DDL) editor at rest — a pixel baseline is cheap insurance against a future edit
// silently regressing its layout. Mirrors sql-schema.spec.ts's own
// connectAndExpandPostgres + "menu-item-schema" opening sequence.

const CONNECTION_ID = 'conn-visual-schema-dialog';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Visual Schema', 'green');

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: postgresCreateArgs('Visual Schema', 'green'),
    response: CONNECTION_SUMMARY,
  },
  ...orderItemsFixture(CONNECTION_ID).control,
];

async function connectAndExpandPostgres(page: Page): Promise<void> {
  await createConnection(page, 'postgres', 'Visual Schema');

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
}

test('Schema (DDL) editor at rest (P6)', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await connectAndExpandPostgres(page);

  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-schema"]');
  const dialog = page.locator('[data-testid="schema-dialog"]');
  await expect(dialog).toBeVisible();
  // Monaco mounts from a lazy chunk; a capture before it renders has no editor (and nothing to mask).
  await expect(dialog.locator('.monaco-editor .line-numbers').first()).toBeAttached({
    timeout: 30_000,
  });

  // Monaco auto-focuses on mount, so the empty editor shows a blinking caret — its blink is
  // JS-driven (not a CSS animation Playwright's own animation-disabling catches), so two captures
  // of the same "at rest" state can legitimately land in different blink phases. Blur onto the
  // dialog's own static intro text first: "at rest" doesn't require focus, and an unfocused editor
  // renders no caret at all, which is what makes this deterministic.
  await dialog.locator('[data-slot="field-description"]').first().click();
  // Monaco's vertical scrollbar fades on a JS timer, so a capture lands with or without its slider
  // depending on timing. Mask it (a race, not a layout signal).
  await settleFonts(page);
  await expect(page).toHaveScreenshot('schema-dialog.png', {
    mask: [dialog.locator('.monaco-scrollable-element > .scrollbar.vertical')],
  });
});
