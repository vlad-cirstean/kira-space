import { expect, test } from './fixtures';
import {
  gitStreamEmit,
  gitStreamParams,
  gitStreamRequests,
  installGitStreamMock,
} from './support/gitStreamMock';
import {
  buildGraphStreamChunk,
  buildMultiBranchChunk,
  buildMultiBranchRefsList,
  MULTI_BRANCH_SHAS,
} from './support/graphStreamFixture';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

const REPO = {
  id: 'repo-cr-index',
  name: 'cr-index-repo',
  root: '/tmp/cr-index-repo',
  repoId: '/tmp/cr-index-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const CONTROL: ControlSnapshot[] = [{ channel: IPC.codeWorkspaceListRepos, response: [REPO] }];

// Collapsed default display order: F0 [placeholder] G0 G1 M0 M1 M2.
const NEWER_TIP_ROW = 0;
const OLDER_TIP_ROW = 2;
const MAIN_TIP_ROW = 4;

function record(number: number, headRef: string, headSha: string, state: string) {
  return {
    number,
    title: headRef,
    url: `https://github.com/acme/widgets/pull/${number}`,
    state,
    headRef,
    headSha,
    baseRef: 'main',
    updatedAt: 0,
  };
}

const FIRST_INDEX = {
  complete: true,
  syncedAt: 1,
  branches: [
    {
      name: 'feature-older',
      changeRequest: record(42, 'feature-older', MULTI_BRANCH_SHAS.featureOlderTip, 'open'),
    },
    {
      name: 'feature-newer',
      changeRequest: record(43, 'feature-newer', MULTI_BRANCH_SHAS.featureNewerTip, 'draft'),
    },
    { name: 'main', changeRequest: record(44, 'main', MULTI_BRANCH_SHAS.mainTip, 'merged') },
  ],
};

const SECOND_INDEX = {
  ...FIRST_INDEX,
  branches: [
    {
      name: 'feature-older',
      changeRequest: record(45, 'feature-older', MULTI_BRANCH_SHAS.featureOlderTip, 'open'),
    },
    ...FIRST_INDEX.branches.slice(1),
  ],
};

function badge(page: import('@playwright/test').Page, row: number) {
  return page
    .locator(
      `[data-testid="commit-grid"] .slick-row[data-row="${row}"] .kira-cell-message [data-testid="badge-change-request"]`,
    )
    .first();
}

async function open(page: import('@playwright/test').Page) {
  await installGitStreamMock(
    page,
    REPO.repoId,
    {
      'repo.open': {
        kind: 'ok',
        repo: {
          repoId: REPO.repoId,
          root: REPO.root,
          gitDir: `${REPO.root}/.git`,
          commonDir: `${REPO.root}/.git`,
          isBare: false,
          isLinkedWorktree: false,
          head: { kind: 'branch', name: 'main' },
        },
      },
      'refs.list': buildMultiBranchRefsList(),
      'branch.changeRequestIndex': { __sequence: [FIRST_INDEX, SECOND_INDEX] },
      'branch.syncChangeRequests': {},
    },
    [buildGraphStreamChunk(REPO.repoId, 0, buildMultiBranchChunk())],
  );
  await page.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`).click();
}

test('the persisted index badges every branch tip with no per-branch lookup', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await open(page);

  await expect(badge(page, OLDER_TIP_ROW)).toHaveText('#42');
  await expect(badge(page, NEWER_TIP_ROW)).toHaveText('#43');
  await expect(badge(page, NEWER_TIP_ROW)).toHaveAttribute('data-kira-tip', /draft/i);
  await expect(badge(page, MAIN_TIP_ROW)).toHaveText('#44');

  expect(await gitStreamRequests(page)).not.toContain('branch.resolveChangeRequest');
  expect(await gitStreamParams(page, 'branch.changeRequestIndex')).toEqual([
    { repoId: REPO.repoId },
  ]);
});

test('changeRequestsChanged reloads the index and swaps the badge', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await open(page);
  await expect(badge(page, OLDER_TIP_ROW)).toHaveText('#42');

  await gitStreamEmit(page, 'repo.changed', {
    repoId: REPO.repoId,
    kind: 'changeRequestsChanged',
  });

  await expect(badge(page, OLDER_TIP_ROW)).toHaveText('#45');
  expect(await gitStreamRequests(page)).not.toContain('branch.resolveChangeRequest');
});

test('a quick blur and focus sends one sync request', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await open(page);
  await expect(badge(page, OLDER_TIP_ROW)).toBeVisible();

  await page.evaluate(() => window.dispatchEvent(new Event('blur')));
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));

  await expect
    .poll(async () => (await gitStreamParams(page, 'branch.syncChangeRequests')).length)
    .toBe(1);
  // Absence of a second sync: spans the throttle window of real time.
  await page.waitForTimeout(1500);
  const calls = (await gitStreamParams(page, 'branch.syncChangeRequests')) as {
    focused?: boolean;
  }[];
  expect(calls.filter((c) => c.focused !== false)).toHaveLength(1);
  expect(calls.filter((c) => c.focused === false)).toHaveLength(0);
});
