import { expect, test } from './fixtures';
import { adeBoardSnapshot, openPlan } from './support/adeV2';

// A pair whose conflict check failed all retries reads as unknown, never clean or conflicting; a
// pair still checking reads as checking. The backend flags are covered by the ade facts tests.

type PairFx = { a: string; b: string; checking?: boolean; checkFailed?: boolean };

const kinds = (page: import('@playwright/test').Page) =>
  page
    .locator('[data-testid="ade-tag"]')
    .evaluateAll((els) => els.map((el) => el.getAttribute('data-kind')));

test('a pair whose check failed shows unknown on both branches, not clean', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [
    adeBoardSnapshot((b) => {
      (b.pairs as PairFx[])[0].checkFailed = true;
    }),
  ]);
  const failed = page.locator('[data-testid="ade-tag"][data-kind="checkFailed"]');
  await expect(failed.first()).toBeVisible();
  await failed.first().hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    'refreshing retries it',
  );
});

test('a pair still checking shows checking, not unknown', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    adeBoardSnapshot((b) => {
      (b.pairs as PairFx[])[0].checking = true;
    }),
  ]);
  await expect(page.locator('[data-testid="ade-tag"][data-kind="checking"]').first()).toBeVisible();
  expect((await kinds(page)).filter((k) => k === 'checkFailed')).toHaveLength(0);
});

test('a pair without the flags shows no unknown state', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator('[data-testid="ade-tag"]').first()).toBeVisible();
  expect((await kinds(page)).filter((k) => k === 'checkFailed')).toHaveLength(0);
});
