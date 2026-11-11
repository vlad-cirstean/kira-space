import type { Page } from '@playwright/test';
import { expect, reviewTest as test } from '../../ui/fixtures';
import { emitAgentSessions } from '../../ui/support/adeV2';
import { REVIEW_AGENT, STORY_LAUNCH, storyReview } from '../support/buildStory';
import { shoot } from '../support/shoot';

const t = (id: string) => `[data-testid="${id}"]`;

test('build review window', async ({ relaunch, consoleErrors }) => {
  const { control, gitStream, storage } = storyReview();
  // Site images show the inline diff; the app default stays side by side.
  const prepare = async (page: Page) => {
    await STORY_LAUNCH.prepare?.(page);
    await page.addInitScript(() => localStorage.setItem('kira.repo.diffLayout', 'inline'));
  };
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    prepare,
    control,
    gitStream,
    storage,
  });
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
  await expect(win.locator(t('repo-diff-editor')).locator('.monaco-diff-editor')).not.toHaveClass(
    /side-by-side/,
  );
  await expect(win.locator(t('repo-diff-layout-inline'))).toHaveAttribute('data-state', 'on');
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
