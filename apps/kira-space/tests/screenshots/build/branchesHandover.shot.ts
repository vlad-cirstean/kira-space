import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from '../../ui/fixtures';
import { adeFixture } from '../../ui/support/adeV2';
import { IPC } from '../../ui/support/ipcChannels';
import {
  STORY,
  STORY_LAUNCH,
  storyArtifacts,
  storyBoard,
  storyBoardWithDraft,
  storyHandovers,
} from '../support/buildStory';
import { shoot } from '../support/shoot';

const t = (id: string) => `[data-testid="${id}"]`;
const head = `${t('ade-card')}[data-task-id="${STORY.taskId}"] ${t('ade-card-head')}`;

test('build branches: panel and add branch', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({ ...STORY_LAUNCH, control: storyBoard() });
  await win.locator(t('ade-plan')).waitFor();
  await win.locator(head).click();
  await expect(win.locator(t('ade-main-tab'))).toBeVisible();
  await expect(win.locator(t('ade-task-branch'))).toHaveCount(2);
  await shoot(win, 'build-branches-panel', consoleErrors);
});

test('build branches: add branch form', async ({ relaunch, consoleErrors }) => {
  const candidates = adeFixture<{ branches: { codeRepoId: string; name: string }[] }>('candidates');
  candidates.branches[0] = {
    ...(candidates.branches[0] as object),
    codeRepoId: 'repo-web-app',
    name: STORY.branch,
  };
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    control: storyBoard([{ channel: IPC.adeTaskCandidateBranches, response: candidates }]),
  });
  await win.locator(t('ade-plan')).waitFor();
  await win.locator(head).click();
  await win.locator(t('ade-add-branch')).click();
  await expect(win.locator(t('ade-add-popover'))).toBeVisible();
  await expect(win.locator('[data-testid^="ade-branch-option-"]').first()).toBeVisible();
  await shoot(win, 'build-branches-add', consoleErrors);
});

test('build handover and artifacts', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    control: storyBoard([
      { channel: IPC.adeTaskBoard, response: storyBoardWithDraft() },
      { channel: IPC.adeTaskHandovers, response: storyHandovers() },
      { channel: IPC.adeTaskArtifacts, response: storyArtifacts() },
    ]),
  });
  await win.locator(t('ade-plan')).waitFor();
  await win.locator(head).click();
  await win.locator(t('ade-panel-tab-details')).click();
  const chain = win.locator(t('ade-task-chain'));
  await expect(chain.locator(t('ade-handover-draft'))).toContainText('Plan agreed');
  await expect(chain.locator(t('ade-handover-row'))).toHaveCount(1);
  await shoot(win, 'build-handover-notes', consoleErrors);
  await expect(win.locator(t('ade-details-links'))).toBeVisible();
  await shoot(win, 'build-artifacts-list', consoleErrors);
});

test('build handover: stage dialog', async ({ relaunch, consoleErrors }) => {
  const entry = routed('handover', STORY.taskId);
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    control: storyBoard([
      { channel: IPC.adeTaskBoard, response: storyBoardWithDraft() },
      { channel: IPC.adeTaskHandovers, response: storyHandovers() },
      { channel: IPC.adeTaskSessions, response: { sessions: [] } },
      { channel: IPC.promptsList, response: [entry] },
    ]),
  });
  await win.locator(t('ade-plan')).waitFor();
  await emitPrompts(win, [entry]);
  const dialog = win.locator(t('ade-stage-dialog'));
  await expect(dialog.locator(t('ade-stage-notes')).locator('.ProseMirror')).toContainText(
    'Plan agreed',
  );
  await shoot(win, 'build-handover-dialog', consoleErrors);
});

test('build artifacts: done dialog', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({
    ...STORY_LAUNCH,
    control: storyBoard([
      { channel: IPC.adeTaskBoard, response: storyBoardWithDraft() },
      { channel: IPC.adeTaskSessions, response: { sessions: [] } },
      { channel: IPC.adeTaskArtifacts, response: storyArtifacts() },
    ]),
  });
  await win.locator(t('ade-plan')).waitFor();
  await win.locator(head).click();
  await win.locator(t('ade-step-pick')).click();
  await win.locator(t('ade-step-option-done')).click();
  const dialog = win.locator(t('ade-stage-dialog'));
  await expect(dialog.locator(t('ade-stage-artifacts'))).toBeVisible();
  await expect(dialog.locator(t('ade-artifact-row')).first()).toBeVisible();
  await shoot(win, 'build-artifacts-done', consoleErrors);
});
