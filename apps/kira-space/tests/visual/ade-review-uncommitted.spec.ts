import { expect, reviewTest } from '../ui/fixtures';
import { contract } from '../ui/support/contract';
import { IPC } from '../ui/support/ipcChannels';
import { GIT, reviewControl, t, target } from '../ui/support/reviewHunkDiff';

// P599: the review window's left pane for a branch with no commits and uncommitted files.

const SCENARIO = 'ade-review-uncommitted';

reviewTest('review window with only uncommitted work', async ({ relaunch }) => {
  const working = contract<object>(SCENARIO, 'AdeTaskService.ReviewWorking#uncommittedOnly');
  const { window: page } = await relaunch({
    control: reviewControl([
      { channel: IPC.adeTaskReviewWorking, response: { ...working, worktree: target.worktree } },
      { channel: IPC.adeTaskReviewWorkingComments, response: { comments: [] } },
    ]),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'review.resolveBase': {
          ...contract<object>(SCENARIO, 'git:review.resolveBase#uncommittedOnly'),
          base: 'main',
        },
      },
    },
  });
  await page.setViewportSize({ width: 1400, height: 900 });
  await expect(page.locator(t('review-working'))).toBeVisible();
  await expect(page.locator(t('ade-review-left'))).toHaveScreenshot('review-uncommitted-only.png');
});
