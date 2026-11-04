import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, adeV2Control, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// Repo scripts that reference a Run script. Backend half: adeflow reposcript_test.go and
// repoflow TestRepoScriptRefFile.

interface Deployment {
  env: string;
  status: string;
  error: string;
}

interface Repo {
  codeRepoId: string;
  problem: { line: number; message: string } | null;
}

const t = (id: string) => `[data-testid="${id}"]`;

const setupLog = (lines: string[], stream: string) => ({
  kind: 'setup',
  id: 'b_searchui',
  chunks: lines.map((text, i) => ({
    seq: i + 1,
    at: 1790066880000 + i * 1000,
    stream,
    text: `${text}\n`,
  })),
  nextSeq: lines.length + 1,
  done: true,
  truncated: false,
});

async function openSetup(page: Page): Promise<void> {
  await page.locator('[data-testid="ade-branch-row"][data-branch-id="b_searchui"]').first().click();
  await expect(page.locator(t('ade-worktree-setup'))).toBeVisible();
}

test('contract: a Run script prepare shows its output with the secret masked', async ({
  relaunch,
}) => {
  const lines = contract<string[]>('ade-repo-script', 'AdeTaskService.ReadLog#setup-ready');
  const board = adeBoard((b) => {
    const row = b.branches.find((x) => x.id === 'b_searchui');
    if (row) row.setup = { state: 'running', startedAt: 1790067000000 };
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
    { channel: IPC.adeTaskReadLog, response: setupLog(lines, 'stdout') },
  ]);
  await openSetup(page);
  const log = page.locator(t('ade-worktree-setup')).locator(t('ade-run-log'));
  await expect(log).toContainText('running Run script "Install deps"');
  await expect(log).toContainText('mode=ci task=Fix login');
  await expect(log).toContainText('token=••••');
});

test('contract: a Run script that is not found fails the setup with that note', async ({
  relaunch,
}) => {
  const fx = contract<{ note: string; log: string }>(
    'ade-repo-script',
    'AdeTaskService.Branch#setup-failed',
  );
  const board = adeBoard((b) => {
    const row = b.branches.find((x) => x.id === 'b_searchui');
    if (row)
      row.setup = {
        state: 'failed',
        startedAt: 1790067000000,
        finishedAt: 1790067001000,
        note: fx.note,
      };
  });
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
    { channel: IPC.adeTaskReadLog, response: setupLog([fx.log.trim()], 'event') },
  ]);
  await openSetup(page);
  const setup = page.locator(t('ade-worktree-setup'));
  await expect(setup).toContainText('failed');
  await expect(setup.locator(t('script-progress-reason'))).toHaveText(fx.note);
  await expect(setup.locator(t('ade-run-log'))).toContainText(fx.note);
});

test('contract: a deployed environment and one with a missing Run script show as the backend reports them', async ({
  relaunch,
}) => {
  const facts = contract<Deployment[]>('ade-repo-script', 'AdeTaskService.Branch#deployments');
  const board = adeFixture<{ branches: { id: string; deployments: unknown[] }[] }>('board');
  const target = board.branches.find((b) => b.id === 'b_bill');
  if (target)
    target.deployments = facts.map((d) => ({
      ...d,
      deployedSha: d.status === 'deployed' ? 'abc1234' : '',
    }));
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await page
    .locator(`${t('ade-branch-row')}[data-branch-id="b_bill"]`)
    .first()
    .click();
  const row = (env: string) => page.locator(`${t('ade-branch-deploy')}[data-env="${env}"]`);
  const ok = facts.find((d) => d.status === 'deployed');
  const broken = facts.find((d) => d.error);
  await expect(row(ok?.env ?? '')).toContainText('Runs abc1234, contains this branch');
  await expect(row(broken?.env ?? '')).toContainText(broken?.error ?? '');
});

test('contract: a repo file with an invalid Run script reference shows its error', async ({
  relaunch,
}) => {
  const problem = contract<{ line: number; message: string }>(
    'repo-script-ref',
    'RepoConfigService.File#prepare-both',
  );
  const file = contract<Record<string, unknown>>(
    'repo-config-file',
    'RepoConfigService.File#broken',
  );
  const repos = adeFixture<{ repos: Repo[] }>('repos');
  const web = repos.repos.find((r) => r.codeRepoId === 'repo-web-app');
  if (web) web.problem = problem;
  const { window: page } = await relaunch({
    control: adeV2Control([
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      { channel: IPC.adeTaskRepos, response: repos },
      { channel: IPC.repoConfigFile, response: { ...file, problem } },
    ]),
  });
  await page.locator(t('manage-repos')).click();
  await page.locator(t('repos-dialog')).waitFor();
  await expect(
    page.locator(
      `${t('repos-dialog-repo')}[data-repo-id="repo-web-app"] ${t('repos-dialog-repo-problem')}`,
    ),
  ).toBeVisible();
  await expect(page.locator(t('config-file-status'))).toContainText(problem.message);
});
