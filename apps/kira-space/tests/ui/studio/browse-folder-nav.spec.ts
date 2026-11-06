import type { Page } from '@playwright/test';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../../ipc/redis/redis.fixture';
import { expect, studioTest as test } from '../fixtures';
import { openRedisBrowse, REDIS_DB0 } from './support/redisFixture';
import { BUCKET_PATH, OBJECT_PATH, openS3Browse, s3Control } from './support/s3Fixture';

// Double-clicking or pressing Enter on a Browse folder descends; on a leaf it previews in the
// split. Either way the Browse tab, its breadcrumb and its list stay on screen. The behaviour is
// renderer-only, so there is no backend half.

const USER_NS = `${REDIS_DB0}/namespace:user`;
const USER1_NS = `${USER_NS}/namespace:1`;
const BIGHASH = `${USER1_NS}/key:user%3A1%3Abighash`;
const PREFIX_LOGS = `${BUCKET_PATH}/prefix:logs`;

const browse = (page: Page) => page.locator('[data-testid="browse-view"]');
const row = (page: Page, path: string) =>
  browse(page).locator(`[data-testid="browse-row"][data-path="${path}"]`);

async function expectNavIntact(page: Page, leaf: string): Promise<void> {
  await expect(browse(page)).toBeVisible();
  await expect(page.locator('[data-testid="browse-list-head"]')).toBeVisible();
  await expect(row(page, leaf)).toHaveAttribute('aria-selected', 'true');
  await expect(
    page.locator('[data-testid="browse-detail-pane"] [data-testid="keyvalue-pane"]'),
  ).toBeVisible();
  await expect(page.locator('[data-testid="keyvalue-view"]')).toHaveCount(0);
}

test('redis browse keeps its navigation when a key is opened', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
  await openRedisBrowse(page);

  await row(page, USER_NS).dblclick();
  await expect(browse(page)).toHaveAttribute('data-level', USER_NS);
  await row(page, USER1_NS).dblclick();
  await expect(browse(page)).toHaveAttribute('data-level', USER1_NS);
  await expect(page.locator('[data-testid="browse-crumb"]').first()).toBeVisible();

  await row(page, BIGHASH).dblclick();
  await expectNavIntact(page, BIGHASH);
  await expect(browse(page)).toHaveAttribute('data-level', USER1_NS);
});

test('redis browse keeps its navigation when Enter opens a key', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
  await openRedisBrowse(page);
  await row(page, USER_NS).dblclick();
  await row(page, USER1_NS).dblclick();
  await expect(browse(page)).toHaveAttribute('data-level', USER1_NS);

  await row(page, BIGHASH).focus();
  await page.keyboard.press('Enter');
  await expectNavIntact(page, BIGHASH);
});

test('s3 browse keeps its navigation when an object is opened', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: s3Control });
  await openS3Browse(page);

  await row(page, PREFIX_LOGS).dblclick();
  await expect(browse(page)).toHaveAttribute('data-level', PREFIX_LOGS);
  await expect(page.locator('[data-testid="browse-up"]')).toBeEnabled();
  await page.locator('[data-testid="browse-up"]').click();
  await expect(browse(page)).toHaveAttribute('data-level', BUCKET_PATH);

  await row(page, OBJECT_PATH).dblclick();
  await expectNavIntact(page, OBJECT_PATH);
  await expect(browse(page)).toHaveAttribute('data-level', BUCKET_PATH);
});
