import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { freezeClock, runCoalesceWindow } from './support/clock';
import { contract } from './support/contract';
import { gitBoundWindowKeys, gitStreamEmit, gitStreamRequests } from './support/gitStreamMock';
import {
  openPortGraph,
  PORT_REPO,
  PORT_REPO_OPEN,
  rootRow,
  SHA_A,
  singleRowChunks,
} from './support/gitUiPortFixtures';

// The frontend half of flows/gitflow/bound_test.go: the repo view reads through the bound
// GitService with this page's window key, and the Go-pushed kira:git:repo.changed event reaches
// only the repo this page opened. Answers come from the flow test's contract file.

const REFS = contract('git-bound', 'git:refs.list#bound');

const grid = '[data-testid="commit-grid"]';
const row = (p: Page, n: number) => p.locator(`${grid} .slick-row[data-row="${n}"]`);
const refsRequests = async (p: Page) =>
  (await gitStreamRequests(p)).filter((m) => m === 'refs.list').length;

async function openBound(relaunch: Parameters<typeof openPortGraph>[0]) {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'Add the bound fixture')]),
    results: { 'repo.open': PORT_REPO_OPEN, 'refs.list': REFS },
  });
  await expect(row(page, 0)).toBeVisible({ timeout: 15_000 });
  return page;
}

test('the repo view reads refs and opens its repo through GitService with the page window key', async ({
  relaunch,
}) => {
  const page = await openBound(relaunch);

  const requests = await gitStreamRequests(page);
  expect(requests).toContain('repo.open');
  expect(requests).toContain('refs.list');
  const pageKey = await page.evaluate(
    () => new URLSearchParams(location.search).get('window') ?? 'main',
  );
  const keys = await gitBoundWindowKeys(page);
  expect(keys.length).toBeGreaterThan(0);
  expect(new Set(keys)).toEqual(new Set([pageKey]));
});

test('kira:git:repo.changed refetches refs for the opened repo only', async ({ relaunch }) => {
  const page = await openBound(relaunch);
  const before = await refsRequests(page);
  await freezeClock(page);

  await gitStreamEmit(page, 'repo.changed', { repoId: '/tmp/elsewhere', kind: 'refsChanged' });
  await runCoalesceWindow(page);
  expect(await refsRequests(page)).toBe(before);

  await gitStreamEmit(page, 'repo.changed', { repoId: PORT_REPO.repoId, kind: 'refsChanged' });
  await runCoalesceWindow(page);
  await expect.poll(() => refsRequests(page), { timeout: 15_000 }).toBeGreaterThan(before);
});
