import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import {
  gitBoundWindowKeys,
  gitStreamEmit,
  gitStreamParams,
  gitStreamRequests,
} from './support/gitStreamMock';
import {
  openPortGraph,
  PICKER_RESULTS,
  PORT_REPO,
  refRow,
  rootRow,
  SHA_A,
  singleRowChunks,
} from './support/gitUiPortFixtures';

// The frontend half of flows/gitflow/bound_writes_test.go: remote ops, cancel and stash reads go
// through the bound GitService with this page's window key, and kira:git:remote.progress reaches
// only the repo this page opened. Answers come from the flow test's contract file.

const STASHES = contract<{ entries: { message: string }[] }>(
  'git-bound-writes',
  'git:stash.list#bound',
);

const REFS = {
  ...PICKER_RESULTS['refs.list'],
  branches: [refRow('main', { upstream: 'refs/remotes/origin/main', isHead: true })],
  remoteBranches: [refRow('origin/main', { refname: 'refs/remotes/origin/main', kind: 'remote' })],
};

const grid = '[data-testid="commit-grid"]';

async function openBound(relaunch: Parameters<typeof openPortGraph>[0]) {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'Add the bound fixture')]),
    // remote.run and remote.cancel stay unanswered: the fetch hangs, as a running one does.
    results: { 'refs.list': REFS, 'stash.list': STASHES },
  });
  await expect(page.locator(`${grid} .slick-row[data-row="0"]`)).toBeVisible({ timeout: 15_000 });
  return page;
}

const progress = (p: Page) => p.locator('[data-testid="remote-progress"]');

test('a fetch, its progress and its cancel go through GitService for the opened repo', async ({
  relaunch,
}) => {
  const page = await openBound(relaunch);
  const pageKey = await page.evaluate(
    () => new URLSearchParams(location.search).get('window') ?? 'main',
  );
  const boundBefore = (await gitBoundWindowKeys(page)).length;

  await page.locator('[data-testid="fetch-button"]').click();
  await expect(progress(page)).toBeVisible();
  expect(await gitStreamRequests(page)).toContain('remote.run');
  const keys = await gitBoundWindowKeys(page);
  expect(keys.length).toBeGreaterThan(boundBefore);
  expect(new Set(keys)).toEqual(new Set([pageKey]));

  const progressEvent = (repoId: string, phase: string) => ({
    repoId,
    phase,
    percent: 40,
    done: 2,
    total: 5,
    remote: true,
  });
  await gitStreamEmit(page, 'remote.progress', progressEvent('/tmp/elsewhere', 'stranger'));
  await gitStreamEmit(page, 'remote.progress', progressEvent(PORT_REPO.repoId, 'receiving'));
  await expect(progress(page)).toContainText('receiving');
  await expect(progress(page)).not.toContainText('stranger');

  await page.locator('[data-testid="remote-cancel"]').click();
  await expect
    .poll(() => gitStreamParams(page, 'remote.cancel'))
    .toEqual([{ repoId: PORT_REPO.repoId }]);
});

test('the Stashes tab lists the bound stash.list answer', async ({ relaunch }) => {
  const page = await openBound(relaunch);
  await page.locator('[data-testid="branch-trigger"]').click();
  await page.getByRole('button', { name: /^Stashes/ }).click();
  await expect(
    page.locator(
      '[data-testid="branch-section"][aria-label="Stashes"] [data-testid="stash-message"]',
    ),
  ).toHaveText(STASHES.entries.map((e) => e.message.replace(/^On [^:]+: /, '')));
  expect(await gitStreamRequests(page)).toContain('stash.list');
});
