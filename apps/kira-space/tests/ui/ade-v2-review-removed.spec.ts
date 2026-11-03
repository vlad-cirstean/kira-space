import type { ReviewRemovedEntry } from '@kira/git-ipc';
import { expect, reviewTest } from './fixtures';
import { contract } from './support/contract';
import { gitStreamParams } from './support/gitStreamMock';
import { GIT, reviewControl, t, target } from './support/reviewHunkDiff';

// Reviewed lines removed since review. Backend half: reviewflow TestReviewRemovedSinceReview and
// adeflow TestReviewWorkingRemoved, same contracts.

const committed = contract<{ entries: ReviewRemovedEntry[] }>(
  'git-review-removed',
  'git:review.removed.list#removed',
).entries;
// Separate review databases number their rows alike; the spec makes the ids unique.
const uncommitted = contract<{ entries: ReviewRemovedEntry[] }>(
  'git-review-removed-working',
  'git:review.removed.list#working',
).entries.map((e) => ({ ...e, id: e.id + 100 }));
const all = [...committed, ...uncommitted];
const first = committed[0];

const git = (list: object) => ({
  ...GIT,
  extraResults: {
    ...GIT.extraResults,
    'review.removed.list': list,
    'review.removed.dismiss': { dismissed: 1 },
  },
});

reviewTest('removed lines list with their text, range and origin', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: reviewControl(),
    gitStream: git({ entries: all }),
  });
  const section = page.locator(t('ade-review-removed'));
  await expect(section).toBeVisible();
  await expect(section).toContainText(`Removed since review · ${all.length}`);
  const entries = page.locator(t('ade-review-removed-entry'));
  await expect(entries).toHaveCount(all.length);

  const entry = entries.filter({ hasText: first.path });
  await expect(entry.locator(t('ade-review-removed-reviewed'))).toContainText('Reviewed');
  await expect(entry.locator(t('ade-review-removed-range'))).toHaveText(
    `Lines ${first.range?.start}–${first.range?.end}`,
  );
  await expect(entry.locator(t('ade-review-removed-text'))).toContainText('a6');
  await expect(entry.locator(t('ade-review-removed-origin'))).toHaveCount(0);
  await expect(page.locator(t('ade-review-removed-origin'))).toHaveCount(uncommitted.length);
  await expect(page.locator(t('ade-review-removed-origin')).first()).toHaveText('Uncommitted');
});

reviewTest('dismissing an entry hides it and keeps the rest', async ({ relaunch }) => {
  const rest = all.filter((e) => e.id !== first.id);
  const { window: page } = await relaunch({
    control: reviewControl(),
    gitStream: git({ __sequence: [{ entries: all }, { entries: rest }] }),
  });
  const entries = page.locator(t('ade-review-removed-entry'));
  await expect(entries).toHaveCount(all.length);
  await entries.first().locator(t('ade-review-removed-dismiss')).click();
  await expect(entries).toHaveCount(rest.length);
  await expect(entries.filter({ hasText: first.path })).toHaveCount(0);
  const calls = (await gitStreamParams(page, 'review.removed.dismiss')) as {
    repoId: string;
    ids: number[];
  }[];
  expect(calls).toHaveLength(1);
  expect(calls[0]).toMatchObject({ repoId: target.gitRepoId, ids: [first.id] });
});

reviewTest('nothing removed shows no section', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: reviewControl(),
    gitStream: git({ entries: [] }),
  });
  await page.locator(t('ade-review-window')).waitFor();
  await expect
    .poll(async () => (await gitStreamParams(page, 'review.removed.list')).length)
    .toBeGreaterThan(0);
  await expect(page.locator(t('ade-review-removed'))).toHaveCount(0);
});
