import { expect, test } from './fixtures';
import { openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { openCard, openIn, pill } from './support/links';

// The linked issue in the Jira link's card in the Details tab: title, status and the PR count from the tracker. Contract
// ade-issue-tracker; backend half: flows/adeflow TestTaskIssueTracker, TestTaskIssueFastLoading and TestTaskLinksBeforeJira (issuetracker_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface IssueFx {
  provider: string;
  key: string;
  title: string;
  status: string;
  summary: { known: boolean; count: number };
  reason: string;
  stale?: boolean;
}
const issue = (key: string) => contract<IssueFx>('ade-issue-tracker', key);

test('contract: the Details tab shows the issue title, status and PR count', async ({
  relaunch,
}) => {
  const fx = issue('IssueTrackerService.TaskIssue');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskIssue, response: fx },
  ]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  // The pill's dot carries the tone of the tracker's status category; the card holds the issue line.
  await expect(pill(page, 'PAY-130').locator(t('ade-links-pill-dot'))).toHaveAttribute(
    'data-tone',
    'info',
  );
  const card = await openCard(page, 'PAY-130');
  await expect(card.locator(t('ade-task-issue'))).toHaveCount(1);
  await expect(page.locator(t('ade-task-issue-title'))).toHaveText(fx.title);
  await expect(page.locator(t('ade-task-issue-status'))).toHaveText(fx.status);
  await expect(page.locator(t('ade-task-issue-prs'))).toHaveText(
    `${fx.summary.count} pull requests`,
  );
  await expect.poll(() => calls(control, IPC.adeTaskIssue)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskIssue)[0]?.args).toEqual({ taskId: 'T_alerts' });
});

test('contract: links show while Jira loads, details fill in after', async ({ relaunch }) => {
  const listed = contract<{ artifacts: { kind: string; source: string; status: string }[] }>(
    'ade-issue-tracker',
    'AdeTaskService.Artifacts#held',
  );
  expect(listed.artifacts.some((a) => a.kind === 'issue' && a.status === '')).toBe(true);
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArtifacts, response: { ...listed, gaps: [] } },
    { channel: IPC.adeTaskIssue, response: issue('IssueTrackerService.TaskIssue'), hold: true },
  ]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  // Artifacts answered, TaskIssue is held: every link shows, the Jira pill has a pending dot.
  const pills = page.locator(t('ade-links-pill'));
  await expect(pills.first()).toBeVisible();
  const jira = pill(page, 'PAY-130');
  await expect(jira.locator(t('ade-links-pill-dot-pending'))).toHaveCount(1);
  await expect(jira.locator(t('ade-links-pill-dot'))).toHaveCount(0);
  await expect(page.locator(t('ade-links-skeleton'))).toHaveCount(0);
  const count = await pills.count();
  expect(count).toBeGreaterThan(1);
  const before = (await jira.boundingBox())?.width ?? 0;
  expect(before).toBeGreaterThan(0);

  control.release(IPC.adeTaskIssue);
  await expect(jira.locator(t('ade-links-pill-dot'))).toHaveAttribute('data-tone', 'info');
  await expect(jira.locator(t('ade-links-pill-dot-pending'))).toHaveCount(0);
  await expect(pills).toHaveCount(count);
  const after = (await jira.boundingBox())?.width ?? 0;
  expect(Math.abs(after - before)).toBeLessThanOrEqual(1);
});

test('contract: a stale issue line shows at once and refreshes in place', async ({ relaunch }) => {
  const stale = issue('IssueTrackerService.TaskIssue#stale');
  const fresh = issue('IssueTrackerService.TaskIssue#fresh');
  const freshArgs = contract<Record<string, unknown>>(
    'ade-issue-tracker',
    'args:IssueTrackerService.TaskIssue#fresh',
  );
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskIssue, args: { taskId: 'T_alerts' }, response: stale },
    {
      channel: IPC.adeTaskIssue,
      args: { taskId: 'T_alerts', fresh: true },
      response: fresh,
      hold: true,
    },
  ]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await openCard(page, 'PAY-130');
  await expect(page.locator(t('ade-task-issue-status'))).toHaveText(stale.status);
  await expect(page.locator(t('ade-task-issue-loading'))).toHaveCount(0);
  await expect
    .poll(() => calls(control, IPC.adeTaskIssue).map((c) => c.args))
    .toEqual([{ taskId: 'T_alerts' }, { taskId: 'T_alerts', fresh: true }]);
  // The copy stays on screen while the fresh read is pending.
  await expect(page.locator(t('ade-task-issue-status'))).toHaveText(stale.status);
  await expect(page.locator(t('ade-task-issue-loading'))).toHaveCount(0);
  control.release(IPC.adeTaskIssue);
  await expect(page.locator(t('ade-task-issue-status'))).toHaveText(fresh.status);
  expect(Object.keys(calls(control, IPC.adeTaskIssue)[1]?.args as object).sort()).toEqual(
    Object.keys(freshArgs).sort(),
  );
  // A fresh answer never starts another read.
  expect(calls(control, IPC.adeTaskIssue)).toHaveLength(2);
});

test('contract: clicking the Jira pill opens the link in the browser', async ({ relaunch }) => {
  const fx = issue('IssueTrackerService.TaskIssue');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskIssue, response: fx },
    { channel: IPC.linkOpenExternal, response: null },
  ]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await openIn(page, 'PAY-130');
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({
    url: 'https://acme.atlassian.net/browse/PAY-130',
  });
  await expect(page.locator(t('ade-links-card'))).toHaveCount(0);
});

test('clicking the Jira pill marks its pending issue update seen', async ({ relaunch }) => {
  const fx = issue('IssueTrackerService.TaskIssue');
  const tracked = contract<{ updates: Record<string, unknown>[] }>(
    'ade-tracked-updates',
    'AdeTaskService.TrackedUpdates#changed',
  );
  const update = tracked.updates.find((u) => u.kind === 'issue');
  if (!update) throw new Error('contract lacks an issue update');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskIssue, response: fx },
    {
      channel: IPC.adeTaskTrackedUpdates,
      response: { updates: [{ ...update, taskId: 'T_alerts', itemKey: 'issue', branchId: '' }] },
    },
    { channel: IPC.adeTaskMarkTrackedSeen, hold: true },
    { channel: IPC.linkOpenExternal, response: null },
  ]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await openIn(page, 'PAY-130');
  await expect.poll(() => calls(control, IPC.adeTaskMarkTrackedSeen)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskMarkTrackedSeen)[0]?.args).toEqual({
    taskId: 'T_alerts',
    itemKeys: ['issue'],
  });
});

test('contract: a signed-out tracker explains how to sign in', async ({ relaunch }) => {
  const fx = issue('IssueTrackerService.TaskIssue#unauthenticated');
  const { window: page } = await openPlan(relaunch, [{ channel: IPC.adeTaskIssue, response: fx }]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await openCard(page, 'PAY-130');
  await expect(page.locator(t('ade-task-issue-gap'))).toContainText('not signed in to Jira');
  await expect(page.locator(t('ade-task-issue-title'))).toHaveCount(0);
});

test('a task no tracker recognises shows no issue row', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-details-tab'))).toBeVisible();
  const card = await openCard(page, 'PAY-130');
  await expect(card.locator(t('ade-task-issue'))).toHaveCount(0);
});
