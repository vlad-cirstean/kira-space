import { expect, reviewTest as test } from '../../ui/fixtures';
import { emitAgentSessions } from '../../ui/support/adeV2';
import { REVIEW_AGENT, STORY_LAUNCH, storyReview } from '../support/buildStory';
import { shoot } from '../support/shoot';

const t = (id: string) => `[data-testid="${id}"]`;

test('build review window', async ({ relaunch, consoleErrors }) => {
  const { control, gitStream, storage } = storyReview();
  const { window: win } = await relaunch({ ...STORY_LAUNCH, control, gitStream, storage });
  await win.locator(t('ade-review-window')).waitFor();
  await emitAgentSessions(win, [REVIEW_AGENT.session.terminalId]);
  await win
    .locator(`${t('review-files-tree')} ${t('file-tree-row')}`, { hasText: 'exportCsv.ts' })
    .click();
  const modified = win.locator(t('repo-diff-editor')).locator('.editor.modified');
  await expect(modified.locator('.kira-review-ln-comment').first()).toBeVisible({
    timeout: 15_000,
  });
  await expect(win.locator(t('ade-review-comment'))).toHaveCount(3);
  await shoot(win, 'build-review-window', consoleErrors);
  await shoot(win, 'build-review-agent', consoleErrors);
  await win.locator(t('ade-review-tab-ask')).click();
  await win.locator(t('ade-review-forward-bar')).waitFor();
  await shoot(win, 'build-review-forward', consoleErrors);

  await modified.locator('.kira-review-ln-comment').first().click();
  await expect(win.locator(t('review-thread'))).toBeVisible();
  await shoot(win, 'build-review-thread', consoleErrors);

  await win.locator(t('ade-review-actions-trigger')).click();
  await shoot(win, 'build-review-actions', consoleErrors);
});
