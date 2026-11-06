import type { Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import { CHANNEL } from '@shared/protocol/events';
import { expect, test } from './fixtures';
import { emitWailsEvent } from './support/mockRuntime';
import {
  BUCKET_PATH,
  CAPS,
  OBJECT_PATH,
  openS3Browse,
  S3_ID,
  s3Control,
} from './support/s3Fixture';

// S3 Browse write gates after a connection drop: Upload (toolbar, container menu, dialog) and
// object Delete (menu, confirm). The listing stays; nothing reaches the server. The gates live in
// the renderer, so there is no backend half.

const PREFIX_PATH = `${BUCKET_PATH}/prefix:logs`;

function rowMenu(page: Page, path: string) {
  return page
    .locator('[data-testid="browse-view"]')
    .locator(`[data-testid="browse-row"][data-path="${path}"]`);
}

async function dropFromEvent(page: Page): Promise<void> {
  await emitWailsEvent(page, CHANNEL.connectionState, {
    connectionId: S3_ID,
    status: 'disconnected',
    serverVersion: null,
    error: null,
    since: 1735689600001,
    caps: null,
  });
}

// The connection comes back under a new session while the listing is still the old one: caps
// stay, so the entries render and only the stale gate disables them.
async function swapSession(page: Page): Promise<void> {
  await emitWailsEvent(page, CHANNEL.connectionState, {
    connectionId: S3_ID,
    status: 'connected',
    serverVersion: 'LocalStack',
    error: null,
    since: 1735689600002,
    caps: CAPS,
  });
}

test('s3 browse locks Upload and Delete entries after a drop', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({ control: s3Control });
  await openS3Browse(page);
  await expect(page.locator('[data-testid="browse-upload"]')).toBeEnabled();

  await rowMenu(page, PREFIX_PATH).click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-upload-file"]')).toBeVisible();
  await page.keyboard.press('Escape');
  await rowMenu(page, OBJECT_PATH).click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-delete-object"]')).not.toHaveAttribute(
    'data-disabled',
    '',
  );
  await page.keyboard.press('Escape');

  await swapSession(page);
  await expect(page.locator('[data-testid="browse-stale"]')).toBeVisible();
  await expect(page.locator('[data-testid="browse-row"]').first()).toBeVisible();
  await expect(page.locator('[data-testid="browse-upload"]')).toBeDisabled();

  await rowMenu(page, PREFIX_PATH).click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await expect(page.locator('[data-testid="menu-item-upload-file"]')).toHaveCount(0);
  await page.keyboard.press('Escape');
  await rowMenu(page, OBJECT_PATH).click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-delete-object"]')).toHaveAttribute(
    'data-disabled',
    '',
  );

  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(0);
});

test('s3 upload dialog refuses a listing whose connection dropped while it was open', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({ control: s3Control });
  await openS3Browse(page);
  await page.click('[data-testid="browse-upload"]');
  await page.click('[data-testid="upload-choose-file"]');
  await expect(page.locator('[data-testid="upload-chosen-file"]')).toContainText('report.csv');

  await dropFromEvent(page);
  await page.click('[data-testid="upload-submit"]');

  await expect(page.locator('[data-testid="upload-error"]')).toContainText('connection was lost');
  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(0);
});

test('s3 delete re-checks the connection after the confirm dialog', async ({ relaunch }) => {
  const { window: page, stream } = await relaunch({ control: s3Control });
  await openS3Browse(page);
  await rowMenu(page, OBJECT_PATH).click({ button: 'right' });
  await page.click('[data-testid="menu-item-delete-object"]');
  await expect(page.locator('[data-testid="confirm-dialog"]')).toBeVisible();

  await dropFromEvent(page);
  await page.click('[data-testid="confirm-dialog-confirm"]');

  await expect(page.locator('[data-testid="browse-action-error"]')).toContainText(
    'connection was lost',
  );
  expect((await stream.ops()).filter((o) => o.op === DATA_OP.mutate)).toHaveLength(0);
});
