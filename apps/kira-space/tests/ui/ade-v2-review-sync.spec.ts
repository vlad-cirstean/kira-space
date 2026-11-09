import { expect, reviewTest } from './fixtures';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { GIT, reviewControl, t, target } from './support/reviewHunkDiff';

// Review window sync: Fetch, Update for a local ref behind its remote twin, status for diverged refs
// and the change request base hint. Backend half: adeflow TestReviewSync, same contract.

const SCENARIO = 'ade-review-sync';
interface Side {
  state: string;
  [key: string]: unknown;
}
interface Sync {
  remote: string;
  lastFetchAt: number | null;
  branch: Side;
  base: Side;
}
const FETCHED_AT = 1_790_000_000_000;
const afterFetch = {
  ...contract<Sync>(SCENARIO, 'AdeTaskService.ReviewSync#afterFetch'),
  lastFetchAt: FETCHED_AT,
};
const afterUpdate = {
  ...contract<Sync>(SCENARIO, 'AdeTaskService.ReviewUpdateLocal#afterUpdate'),
  lastFetchAt: FETCHED_AT,
};
const diverged = {
  ...contract<Sync>(SCENARIO, 'AdeTaskService.ReviewSync#diverged'),
  lastFetchAt: FETCHED_AT,
};
const fetchError = contract<{ error: { message: string } }>(
  SCENARIO,
  'AdeTaskService.ReviewFetch#fetchError',
);

const REPOS = [
  { channel: IPC.windowsEnsure, response: { mode: 'git' } },
  {
    channel: IPC.codeWorkspaceListRepos,
    response: [
      {
        id: target.codeRepoId,
        name: 'web-app',
        root: '/tmp/web-app',
        repoId: target.gitRepoId,
        sortOrder: 1,
        color: 'none',
        createdAt: '2026-01-01T00:00:00.000Z',
      },
    ],
  },
];

const withSync = (sync: Sync, extra: Parameters<typeof reviewControl>[0] = []) =>
  reviewControl([...REPOS, { channel: IPC.adeTaskReviewSync, response: sync }, ...extra]);
const tip = '[data-slot="tooltip-content"]';

reviewTest('update fast-forwards the branch and the button goes away', async ({ relaunch }) => {
  const { window: page, control: mock } = await relaunch({
    control: withSync(afterFetch, [
      { channel: IPC.adeTaskReviewUpdateLocal, response: afterUpdate },
    ]),
    gitStream: GIT,
  });
  await page.locator(t('ade-review-window')).waitFor();

  const update = page.locator(t('ade-review-sync-update-branch'));
  await expect(update).toHaveText('Update branch · 1 behind');
  await expect(page.locator(t('ade-review-sync-update-base'))).toHaveCount(0);
  await update.hover();
  await expect(page.locator(tip)).toContainText(
    'Fast-forward local feat/api-sync to origin/feat/api-sync',
  );

  mock.respond(IPC.adeTaskReviewSync, afterUpdate);
  await update.click();
  await expect
    .poll(() => calls(mock, IPC.adeTaskReviewUpdateLocal).map((c) => c.args))
    .toContainEqual({ branchId: target.branchId, side: 'branch' });
  await expect(page.locator(t('ade-review-sync-update-branch'))).toHaveCount(0);
  await expect(page.locator(t('ade-review-sync-error'))).toHaveCount(0);
});

reviewTest('fetch shows pending and then a failure message', async ({ relaunch }) => {
  const { window: page, control: mock } = await relaunch({
    control: withSync(afterFetch, [
      { channel: IPC.adeTaskReviewFetch, response: { error: null }, hold: true },
    ]),
    gitStream: GIT,
  });
  await page.locator(t('ade-review-window')).waitFor();

  const fetch = page.locator(t('ade-review-sync-fetch'));
  await fetch.hover();
  await expect(page.locator(tip)).toContainText('Fetch origin · fetched');
  await fetch.click();
  await expect(fetch).toHaveAttribute('aria-busy', 'true');
  await expect(page.locator(t('ade-review-sync'))).toContainText('Fetching');
  expect(calls(mock, IPC.adeTaskReviewFetch).map((c) => c.args)).toContainEqual({
    branchId: target.branchId,
  });
  mock.release(IPC.adeTaskReviewFetch);
  await expect(fetch).not.toHaveAttribute('aria-busy', 'true');
  await expect(page.locator(t('ade-review-sync-error'))).toHaveCount(0);

  mock.respond(IPC.adeTaskReviewFetch, fetchError);
  await fetch.click();
  await expect(page.locator(t('ade-review-sync-error'))).toContainText(
    'does not appear to be a git repository',
  );
  await expect(fetch).not.toHaveAttribute('aria-busy', 'true');
});

reviewTest('a diverged branch shows a status and no update button', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: withSync(diverged), gitStream: GIT });
  await page.locator(t('ade-review-window')).waitFor();

  const state = page.locator(t('ade-review-sync-branch-state'));
  await expect(state).toHaveText('Diverged');
  await expect(state).toHaveAttribute('data-state', 'diverged');
  await expect(page.locator(t('ade-review-sync-update-branch'))).toHaveCount(0);
  await state.hover();
  await expect(page.locator(tip)).toContainText('both have new commits');
});

reviewTest('a change request into another base shows a hint', async ({ relaunch }) => {
  const request = {
    number: 7,
    title: 'Billing',
    state: 'open',
    url: 'https://github.com/acme/web-app/pull/7',
    baseRef: 'release',
    checksState: '',
  };
  const { window: page } = await relaunch({
    control: withSync(afterFetch, [
      {
        channel: IPC.adeTaskChangeRequests,
        response: { repos: [], branches: { [target.branchId]: request }, branchUrls: {} },
      },
    ]),
    gitStream: GIT,
  });
  await page.locator(t('ade-review-window')).waitFor();

  await expect(page.locator(t('ade-review-sync-pr-base'))).toHaveText('main targets release');
});
