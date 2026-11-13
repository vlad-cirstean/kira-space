import { expect, test } from './fixtures';
import { adeBoard, emitAgentEvent, emitAgentSessions, openPlan } from './support/adeV2';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The Needs you page: its list, the action behind each kind, the footer and All sessions.

const t = (id: string) => `[data-testid="${id}"]`;
const item = (kind: string) => `${t('ade-needs-item')}[data-kind="${kind}"]`;

test('lists the items most urgent first, with the badge and the footer', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await expect(page.locator(t('ade-needs-count'))).toHaveText('4');
  await page.locator(t('ade-tab-needs')).click();
  // The page is a lazy chunk: wait for the list before reading it once.
  await expect(page.locator(t('ade-needs-item'))).toHaveCount(4);
  const kinds = await page
    .locator(t('ade-needs-item'))
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-kind')));
  expect(kinds).toEqual(['failed', 'setup failed', 'stale merge', 'stale merge']);
  await expect(page.locator(t('ade-needs-footer'))).toHaveText(
    '10 background runs · 3 interactive sessions',
  );
  await expect(page.locator(`${item('failed')} ${t('ade-needs-action')}`)).toHaveText('Retry');
  await expect(page.locator(`${item('setup failed')} ${t('ade-needs-action')}`)).toHaveText(
    'See error',
  );
  await expect(page.locator(`${item('stale merge')} ${t('ade-needs-action')}`).first()).toHaveText(
    'Re-merge',
  );
});

test('Retry re-runs the failed runs', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await page.locator(t('ade-tab-needs')).click();
  await page.locator(`${item('failed')} ${t('ade-needs-action')}`).click();
  await expect.poll(() => calls(control, IPC.adeTaskRetryRun).length).toBeGreaterThan(0);
  expect(calls(control, IPC.adeTaskRetryRun)[0]?.args).toHaveProperty('runId');
});

test('Re-merge opens the merge dialog, See error opens the branch setup', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(t('ade-tab-needs')).click();
  await page
    .locator(`${item('stale merge')} ${t('ade-needs-action')}`)
    .first()
    .click();
  await expect(page.locator(t('ade-dialog-title'))).toHaveText('Re-merge into develop');
  await page.locator(t('ade-dialog-cancel')).click();
  await page.locator(`${item('setup failed')} ${t('ade-needs-action')}`).click();
  await expect(page.locator(t('ade-plan'))).toBeVisible();
  await expect(page.locator(t('ade-panel-tab-details'))).toBeVisible();
});

test('a failed item shows the step log', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(t('ade-tab-needs')).click();
  await page.locator(`${item('failed')} ${t('ade-needs-log')}`).click();
  await expect(page.locator(t('ade-run-log-line')).first()).toBeVisible();
});

test('a stuck run leads the list and Take over launches it', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    for (const task of b.tasks) {
      for (const run of task.runs)
        if (String(run.id).startsWith('r_T_bill_impl_')) run.state = 'stuck';
    }
  });
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(t('ade-tab-needs')).click();
  const first = page.locator(t('ade-needs-item')).first();
  await expect(first).toHaveAttribute('data-kind', 'stuck run');
  await first.locator(t('ade-needs-action')).click();
  await expect.poll(() => calls(control, IPC.adeTaskTakeOver)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskTakeOver)[0]?.args).toMatchObject({ stopIfRunning: true });
});

test('a session waiting for an answer is a question and Open raises its terminal', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await emitAgentSessions(page, ['term-9ab0']);
  await emitAgentEvent(page, 'term-9ab0', 'Notification');
  await page.locator(t('ade-tab-needs')).click();
  const q = page.locator(item('question'));
  await expect(q).toHaveCount(1);
  await expect(q.locator(t('ade-needs-action'))).toHaveText('Open');
  await q.locator(t('ade-needs-action')).click();
  await expect.poll(() => calls(control, IPC.adeTaskFocusSession)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskFocusSession)[0]?.args).toMatchObject({
    sessionId: '9ab0',
    taskId: 'T_auth',
  });
});

test('an empty list says nothing needs you', async ({ relaunch }) => {
  const board = adeBoard((b) => {
    for (const br of b.branches) {
      br.setup = null;
      for (const g of (br.integration as { status: string }[]) ?? []) g.status = 'merged';
    }
    for (const task of b.tasks) {
      for (const run of task.runs) if (run.state === 'failed') run.state = 'done';
    }
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page.locator(t('ade-tab-needs')).click();
  await expect(page.locator(t('ade-needs-empty'))).toHaveText('Nothing needs you right now.');
  await expect(page.locator(t('ade-needs-count'))).toHaveCount(0);
});

test('All sessions groups by task and filters running or stopped', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(t('ade-tab-needs')).click();
  await page.locator(t('ade-needs-all')).click();
  await expect(page.locator(t('ade-all-session'))).toHaveCount(13);
  expect(await page.locator(t('ade-all-group')).count()).toBeGreaterThan(5);
  await page.locator(t('ade-all-stopped')).click();
  await expect(page.locator(t('ade-all-session'))).toHaveCount(7);
  await page.locator(t('ade-all-running')).click();
  await expect(page.locator(t('ade-all-session'))).toHaveCount(13);
  await page.locator(t('ade-needs-all')).click();
  await expect(page.locator(t('ade-all-sessions'))).toHaveCount(0);
});
