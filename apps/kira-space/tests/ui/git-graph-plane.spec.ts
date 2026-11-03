import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import {
  gitGraphClose,
  gitGraphDials,
  gitGraphFrames,
  gitStreamRequests,
  installGitStreamMock,
  installGitStreamMockOnInit,
} from './support/gitStreamMock';
import { BASE_RESULTS, PORT_CONTROL, PORT_REPO, type Relaunch } from './support/gitUiPortFixtures';
import {
  HISTORY_ROWS,
  historyRows,
  historySha,
  PAGE_SIZE,
  pageChunks,
  refsList,
} from './support/graphBranchHistoryFixture';

// The graph rows ride the `gitgraph` data plane (repo/git/graphStream.ts); every other git call is
// a bound GitService call behind a once-per-page-load ResetWindow. The mock logs the page's frames.

const HISTORY = historyRows(0, HISTORY_ROWS);
const FIRST_PAGE = pageChunks(HISTORY, 0, PAGE_SIZE, false);
const SECOND_PAGE = pageChunks(HISTORY, PAGE_SIZE, HISTORY_ROWS, true);
const RESULTS = {
  ...BASE_RESULTS,
  'refs.list': refsList(historySha(0)),
  'graph.loadMore': { started: true },
};
const ROW = `[data-testid="repo-row"][data-repo-id="${PORT_REPO.id}"]`;

const loadButton = (p: Page) =>
  p.locator('[data-testid="repo-graph-host"]').getByRole('button', { name: /^Load / });
const pageKey = (p: Page) =>
  p.evaluate(() => new URLSearchParams(location.search).get('window') ?? 'main');
const framesOf = async (p: Page, t: string) => (await gitGraphFrames(p)).filter((f) => f.t === t);

async function openGraph(relaunch: Relaunch, options: { holdAfter?: number } = {}): Promise<Page> {
  const { window: page } = await relaunch({ control: PORT_CONTROL });
  await installGitStreamMock(
    page,
    PORT_REPO.repoId,
    RESULTS,
    undefined,
    [FIRST_PAGE, SECOND_PAGE],
    options.holdAfter,
  );
  await page.locator(ROW).click();
  return page;
}

test('the first mount opens one stream on the page window key and credits each chunk', async ({
  relaunch,
}) => {
  const page = await openGraph(relaunch);
  await expect(loadButton(page)).toBeVisible();

  const opens = await framesOf(page, 'open');
  expect(opens).toHaveLength(1);
  expect(opens[0]).toMatchObject({ windowKey: await pageKey(page), credit: 2 });
  await expect.poll(async () => (await framesOf(page, 'credit')).length).toBe(FIRST_PAGE.length);
  expect(await framesOf(page, 'cancel')).toEqual([]);
});

test('closing the workspace while a chunk is held cancels the stream', async ({ relaunch }) => {
  const page = await openGraph(relaunch, { holdAfter: 1 });
  await expect(page.locator('[data-testid="repo-graph-host"] .slick-row').first()).toBeVisible();
  const [open] = await framesOf(page, 'open');

  await page.locator('[data-testid="git-panel-tab-repos"]').click();
  await page.locator(ROW).click({ button: 'right' });
  await page.locator('[data-testid="menu-item-close"]').click();

  await expect
    .poll(async () => await framesOf(page, 'cancel'))
    .toEqual([expect.objectContaining({ id: open?.id })]);
});

test('Load more is a bound call, then a second open resumes through the loaded rows', async ({
  relaunch,
}) => {
  const page = await openGraph(relaunch);
  await loadButton(page).click();
  await expect(loadButton(page)).toHaveCount(0);

  expect(await gitStreamRequests(page)).toContain('graph.loadMore');
  const opens = await framesOf(page, 'open');
  expect(opens).toHaveLength(2);
  expect(opens[1]?.params).toMatchObject({ repoId: PORT_REPO.repoId, resumeThroughRow: PAGE_SIZE });
});

test('a closed plane is redialed by the next graph action', async ({ relaunch }) => {
  const page = await openGraph(relaunch);
  await expect(loadButton(page)).toBeVisible();
  expect(await gitGraphDials(page)).toBe(1);

  await gitGraphClose(page);
  await loadButton(page).click();

  await expect(loadButton(page)).toHaveCount(0);
  expect(await gitGraphDials(page)).toBe(2);
});

test('the window reset runs once per page load, before the first git call', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: PORT_CONTROL });
  await installGitStreamMockOnInit(page, {
    repoId: PORT_REPO.repoId,
    extraResults: RESULTS,
    graphStreamOpens: [FIRST_PAGE, SECOND_PAGE],
  });
  const resets = async () =>
    (await gitStreamRequests(page)).filter((m) => m === 'ResetWindow').length;

  for (const load of [1, 2]) {
    await page.reload();
    await expect.poll(resets).toBe(1);
    await page.locator(ROW).click();
    await expect(loadButton(page)).toBeVisible();
    const requests = await gitStreamRequests(page);
    expect(requests[0], `load ${load}`).toBe('ResetWindow');
    expect(requests.filter((m) => m === 'ResetWindow')).toHaveLength(1);
  }
});
