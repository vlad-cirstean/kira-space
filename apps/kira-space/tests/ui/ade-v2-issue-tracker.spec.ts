import { expect, test } from './fixtures';
import { openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The linked issue in the Task tab: title, status and the PR count from the tracker. Contract
// ade-issue-tracker; backend half: flows/adeflow TestTaskIssueTracker (issuetracker_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface IssueFx {
  provider: string;
  key: string;
  title: string;
  status: string;
  summary: { known: boolean; count: number };
  reason: string;
}
const issue = (key: string) => contract<IssueFx>('ade-issue-tracker', key);

test('contract: the Task tab shows the issue title, status and PR count', async ({ relaunch }) => {
  const fx = issue('IssueTrackerService.TaskIssue');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskIssue, response: fx },
  ]);
  await openTask(page, 'T_alerts');
  await expect(page.locator(t('ade-task-issue-title'))).toHaveText(fx.title);
  await expect(page.locator(t('ade-task-issue-status'))).toHaveText(fx.status);
  await expect(page.locator(t('ade-task-issue-prs'))).toHaveText(
    `${fx.summary.count} pull requests`,
  );
  await expect.poll(() => calls(control, IPC.adeTaskIssue)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskIssue)[0]?.args).toEqual({ taskId: 'T_alerts' });
});

test('contract: View issue opens the issue view for the task link', async ({ relaunch }) => {
  const fx = issue('IssueTrackerService.TaskIssue');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskIssue, response: fx },
    { channel: IPC.hostViewOpen, response: { key: 'hostview-x', opened: true } },
  ]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-task-issue-view')).click();
  await expect.poll(() => calls(control, IPC.hostViewOpen)).toHaveLength(1);
  expect(calls(control, IPC.hostViewOpen)[0]?.args).toEqual({
    url: 'https://acme.atlassian.net/browse/PAY-130',
  });
});

test('contract: a signed-out tracker explains how to sign in', async ({ relaunch }) => {
  const fx = issue('IssueTrackerService.TaskIssue#unauthenticated');
  const { window: page } = await openPlan(relaunch, [{ channel: IPC.adeTaskIssue, response: fx }]);
  await openTask(page, 'T_alerts');
  await expect(page.locator(t('ade-task-issue-gap'))).toContainText('not signed in to Jira');
  await expect(page.locator(t('ade-task-issue-title'))).toHaveCount(0);
});

test('a task no tracker recognises shows no issue row', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_alerts');
  await expect(page.locator(t('ade-task-tab'))).toBeVisible();
  await expect(page.locator(t('ade-task-issue'))).toHaveCount(0);
});
