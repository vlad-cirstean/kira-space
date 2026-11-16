import { expect, test } from './fixtures';
import { adeBoardSnapshot, openPlan } from './support/adeV2';
import { contract } from './support/contract';

// A repo whose facts are past the deadline shows Updating… on the Refresh all button until the
// next push (the plan has no repos header). The backend flag and its push are covered by the
// adeflow board tests (P622).

test('a repo that is updating marks Refresh all', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    adeBoardSnapshot((b) => {
      const repos = b.repos as { updating?: boolean }[];
      repos[0].updating = true;
    }),
  ]);
  const button = page.locator('[data-testid="ade-refresh-all"][data-updating]');
  await expect(button).toHaveCount(1);
  await button.hover();
  await expect(page.getByRole('tooltip').first()).toHaveText(/^Updating web-app…/);
});

test('a repo without the flag leaves Refresh all plain', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.getByTestId('ade-refresh-all')).toHaveCount(1);
  await expect(page.locator('[data-testid="ade-refresh-all"][data-updating]')).toHaveCount(0);
});

test('contract: the backend repo state with updating set marks Refresh all', async ({
  relaunch,
}) => {
  const state = contract<{ updating: boolean; codeRepoId: string }>(
    'ade-repo-updating',
    'AdeTaskService.Board#repo-updating',
  );
  expect(state.updating).toBe(true);
  const { window: page } = await openPlan(relaunch, [
    adeBoardSnapshot((b) => {
      const repos = b.repos as Record<string, unknown>[];
      repos[0] = { ...repos[0], ...state, codeRepoId: repos[0]?.codeRepoId };
    }),
  ]);
  await expect(page.locator('[data-testid="ade-refresh-all"][data-updating]')).toHaveCount(1);
});
