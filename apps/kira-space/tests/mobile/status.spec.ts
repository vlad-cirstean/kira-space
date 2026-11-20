import { contract } from '../ui/support/contract';
import { expect, test } from './fixtures';

// P705: Activity tab and richer Plan cards. Contract mobile-status; backend half:
// flows/mobileflow TestPhoneStatus (status_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface Run {
  id: string;
  scriptName: string;
  state: string;
  outcome?: { status: string; reason: string; exitCode: number };
  [key: string]: unknown;
}

// Masked ids and zero stamps from the contract, made distinct and recent.
const failedRun = (): Run => {
  const run = contract<{ runs: Run[] }>('mobile-status', 'http:GET /api/runs#failed').runs[0];
  if (!run) throw new Error('contract has no run');
  return {
    ...run,
    id: 'run-1',
    taskId: '',
    branchId: '',
    createdAt: 1,
    startedAt: 1,
    finishedAt: 2,
  };
};

// `app` resets the server on first use, so the beforeEach requests it before pairing.
test.beforeEach(async ({ app: _app, server }) => {
  server.state.auth = 'ok';
});

test('contract: the Activity tab lists agents, the failed run with its reason and the computer', async ({
  app,
  page,
  server,
}) => {
  server.state.replies.runs = { runs: [failedRun()] };
  await app();
  await page.locator(t('tab-activity')).click();
  await expect(page).toHaveURL(/\/activity$/);
  await expect(page.locator(t('activity-agent')).first()).toBeVisible();
  await expect(page.locator(t('activity-agent-meta')).first()).toContainText('ago');
  const run = page.locator(t('phone-run'));
  await expect(run).toHaveCount(1);
  await expect(run.locator(t('phone-run-name'))).toHaveText('job');
  await expect(run.locator(t('phone-run-why'))).toHaveText('exited with status 3');
  await expect(page.locator(t('activity-computer'))).toBeVisible();
  await expect(page.locator(t('activity-connection'))).toBeVisible();
  await expect(page.locator(t('activity-docker'))).toHaveCount(0);
});

test('contract: a pushed run flips to failed without a reload', async ({ app, page, server }) => {
  const failed = failedRun();
  server.state.replies.runs = {
    runs: [{ ...failed, state: 'running', outcome: null, finishedAt: null }],
  };
  await app();
  await page.locator(t('tab-activity')).click();
  await expect(page.locator(t('phone-run'))).toHaveCount(1);
  await expect(page.locator(t('phone-run-why'))).toHaveCount(0);
  // Frames sent before the app subscribed are lost: send until the row changes.
  await expect(async () => {
    server.emit('kira:scriptRuns:changed', {
      ...contract<Run>('mobile-status', 'sse:kira:scriptRuns:changed#failed'),
      id: failed.id,
      taskId: '',
      branchId: '',
      createdAt: 1,
      startedAt: 1,
      finishedAt: 2,
    });
    await expect(page.locator(t('phone-run-why'))).toHaveText('exited with status 3', {
      timeout: 500,
    });
  }).toPass();
  await expect(page.locator(t('phone-run'))).toHaveCount(1);
});

test('Docker counts show when the backend reports an engine', async ({ app, page, server }) => {
  server.state.replies.computer = {
    docker: { state: 'ok', reason: '', running: 2, stopped: 1, total: 3 },
  };
  await app();
  await page.locator(t('tab-activity')).click();
  await expect(page.locator(t('activity-docker'))).toContainText('2 running, 1 stopped');
});

test('Plan cards show sessions and branch rows with their tag', async ({ app, page }) => {
  await app();
  await page.locator(t('tab-plan')).click();
  await expect(page.locator(t('plan-sessions')).first()).toBeVisible();
  const card = page
    .locator(t('plan-task'))
    .filter({ has: page.locator(t('plan-sessions')) })
    .first();
  await card.locator('button').first().click();
  await expect(card.locator(t('plan-branches'))).toBeVisible();
  await expect(card.locator(t('phone-branch-tag')).first()).toBeVisible();
  await expect(card.locator(t('phone-branch-pr')).first()).toHaveAttribute('data-pr-state', /.+/);
});

test('a dropped stream shows the update age instead of a Refresh button', async ({
  app,
  page,
  server,
}) => {
  await app();
  await expect(page.locator(t('connection-dot'))).toBeVisible();
  await expect(page.locator(t('shell-updated'))).toHaveCount(0);
  server.dropStreams();
  await expect(page.locator(t('shell-updated'))).toContainText('Updated');
  await expect(page.getByRole('button', { name: 'Refresh' })).toHaveCount(0);
});
