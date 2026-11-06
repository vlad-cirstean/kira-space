import type { Page } from '@playwright/test';
import { expect, studioTest, test } from '../../ui/fixtures';
import { openProofWindow } from '../../ui/studio/support/proof';
import {
  STORY,
  STORY_LAUNCH,
  storyBoard,
  storyProof,
  storyProofChain,
} from '../support/buildStory';
import { shoot } from '../support/shoot';

const t = (id: string) => `[data-testid="${id}"]`;
const ready = (page: Page) =>
  expect(page.locator('[data-testid="proof-step"][data-ready="true"]').first()).toBeVisible({
    timeout: 20_000,
  });
const tab = (page: Page, title: string) => page.locator(t('tab'), { hasText: title });

studioTest('build proof window', async ({ relaunch, uiServer, consoleErrors }) => {
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    control: storyProof(),
    timezoneId: 'UTC',
  });
  await openProofWindow(win, uiServer.url);
  await ready(win);
  await expect(win.locator(t('proof-step-result'))).toContainText('10482');
  await shoot(win, 'build-proof-window', consoleErrors);

  await tab(win, 'Exports per day').click();
  await expect(win.locator(t('proof-chart'))).toBeVisible();
  await ready(win);
  await shoot(win, 'build-proof-chart', consoleErrors);

  await tab(win, 'Export flow').click();
  await expect(win.locator(`${t('proof-diagram')} svg`)).toBeVisible();
  await ready(win);
  await shoot(win, 'build-proof-diagram', consoleErrors);
});

studioTest('build proof compare', async ({ relaunch, uiServer, consoleErrors }) => {
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    control: storyProof(true),
    timezoneId: 'UTC',
  });
  await openProofWindow(win, uiServer.url);
  await ready(win);
  await expect(win.locator(t('proof-change-step-2'))).toHaveAttribute('data-diff', 'changed');
  await win.locator(t('proof-view-compare')).click();
  await tab(win, 'Exports per day').click();
  await expect(win.locator(`${t('proof-compare-new')} ${t('proof-chart')}`)).toBeVisible();
  await ready(win);
  await shoot(win, 'build-proof-compare', consoleErrors);
});

test('build proof chain', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    control: storyBoard(storyProofChain()),
  });
  await win.locator(t('ade-plan')).waitFor();
  await win
    .locator(`${t('ade-card')}[data-task-id="${STORY.taskId}"] ${t('ade-card-head')}`)
    .click();
  await win.locator(t('ade-panel-tab-details')).click();
  await expect(win.locator(t('ade-proof-entry'))).toBeVisible();
  await shoot(win, 'build-proof-chain', consoleErrors);
});
