import { expect, test } from '../../ui/fixtures';
import { STORY, STORY_LAUNCH, storyBoard } from '../support/buildStory';
import { shoot } from '../support/shoot';

const t = (id: string) => `[data-testid="${id}"]`;

test('build start task: board and new task', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({ ...STORY_LAUNCH, control: storyBoard() });
  await win.locator(t('ade-plan')).waitFor();
  await expect(win.locator(`${t('ade-task')}[data-task-id="${STORY.taskId}"]`)).toContainText(
    STORY.taskTitle,
  );
  await shoot(win, 'build-start-board', consoleErrors);

  await win.locator(t('ade-add')).click();
  await win.locator(t('ade-nw-title')).fill(STORY.taskTitle);
  await win.locator(t('ade-nw-jira')).fill('PAY-102');
  await expect(win.locator(t('ade-add-popover'))).toBeVisible();
  await shoot(win, 'build-start-capture', consoleErrors);
});

test('build start task: workflow and step', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({ ...STORY_LAUNCH, control: storyBoard() });
  await win.locator(t('ade-plan')).waitFor();
  await win.locator(t('ade-tab-workflows')).click();
  await win.locator(t('ade-workflows')).waitFor();
  await expect(win.locator(t('ade-wf-graph'))).toBeVisible();
  await expect(win.locator(t('ade-wf-form-name'))).toHaveValue(STORY.workflowName);
  await shoot(win, 'build-start-workflow', consoleErrors);

  await win.locator(`${t('ade-wf-node')}[data-step-id="impl"]`).click();
  await expect(win.locator(t('ade-wf-inspector'))).toContainText('Implement');
  await shoot(win, 'build-start-step', consoleErrors);
});

test('build start task: launch menu', async ({ relaunch, consoleErrors }) => {
  const { window: win } = await relaunch({ ...STORY_LAUNCH, control: storyBoard() });
  await win.locator(t('ade-plan')).waitFor();
  await win
    .locator(`${t('ade-card')}[data-task-id="${STORY.taskId}"] ${t('ade-card-head')}`)
    .click();
  await win.locator(t('ade-panel-tab-sessions')).click();
  await win.locator(`${t('ade-session-bar-row')} ${t('ade-task-launch')}`).click();
  await expect(win.locator(t('context-menu'))).toBeInViewport();
  await shoot(win, 'build-start-launch', consoleErrors);
});
