import { expect, test } from './fixtures';
import { adeBoardSnapshot, openPlan } from './support/adeV2';

// A repo whose facts are past the deadline shows Updating… on its chip until the next push. The
// backend flag and its push are covered by the adeflow board tests (P622).

test('a repo that is updating shows it on its chip', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    adeBoardSnapshot((b) => {
      const repos = b.repos as { updating?: boolean }[];
      repos[0].updating = true;
    }),
  ]);
  const chip = page.locator('[data-testid="ade-repo-chip"][data-updating]');
  await expect(chip).toHaveCount(1);
  await expect(chip.getByTestId('ade-repo-note')).toHaveText('Updating…');
});

test('a repo without the flag keeps its fetch note', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator('[data-testid="ade-repo-chip"][data-updating]')).toHaveCount(0);
});
