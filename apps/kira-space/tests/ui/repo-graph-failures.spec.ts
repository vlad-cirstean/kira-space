import { buildPackedChunk } from '@kira/git-core/testing/packedChunk';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { gitStreamRequests, installGitStreamMock } from './support/gitStreamMock';
import { buildGraphStreamChunk, buildOneCommitChunk } from './support/graphStreamFixture';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

const REPO = {
  id: 'repo-failures-1',
  name: 'failures-repo',
  root: '/tmp/failures-repo',
  repoId: '/tmp/failures-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};
const SHA = 'ab'.repeat(20);

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
  {
    channel: IPC.codeWorkspaceListFiles,
    args: { id: REPO.id },
    response: { paths: ['a.ts'], status: {}, truncated: false },
  },
];

const repoOpen = {
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
};

const refsWithRemote = {
  branches: [
    {
      refname: 'refs/heads/main',
      kind: 'branch',
      shortName: 'main',
      objectId: SHA,
      upstream: 'refs/remotes/origin/main',
      track: { ahead: 0, behind: 0 },
      committerDate: 0,
      isHead: true,
    },
  ],
  remoteBranches: [
    {
      refname: 'refs/remotes/origin/main',
      kind: 'remote',
      shortName: 'origin/main',
      objectId: SHA,
      committerDate: 0,
      isHead: false,
    },
  ],
  tags: [],
  head: { kind: 'branch', name: 'main' },
};

function status(autoFetch: unknown): unknown {
  return {
    head: { kind: 'branch', name: 'main' },
    inProgress: null,
    counts: { staged: 0, unstaged: 0, untracked: 0, conflicted: 0 },
    autoFetch,
  };
}

const base = { 'repo.open': repoOpen, 'refs.list': refsWithRemote };
const goodChunks = [buildGraphStreamChunk(REPO.repoId, 0, buildOneCommitChunk(SHA, 'one'))];

function openGraph(page: import('@playwright/test').Page) {
  return page.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`).click();
}

test('a failed remote op shows the banner, hint, and an Operations shortcut', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await installGitStreamMock(
    page,
    REPO.repoId,
    {
      ...base,
      'status.get': status(null),
      'remote.run': {
        ok: false,
        error: { kind: 'NonFastForward', message: 'rejected' },
        head: { kind: 'branch', name: 'main' },
        inProgress: null,
        updates: [],
      },
    },
    goodChunks,
  );
  await openGraph(page);
  const fetch = page.locator('[data-testid="fetch-button"]');
  await expect(fetch).toBeEnabled();
  await fetch.click();

  const banner = page.locator('[data-testid="failure-banner"]');
  await expect(banner).toBeVisible();
  await expect(banner).not.toHaveAttribute('role', 'alert');
  await expect(page.locator('[data-testid="failure-banner-hint"]')).toHaveText(
    'Pull, then push again.',
  );
  await expect(page.locator('[data-testid="live-announcements"]').first()).not.toHaveText('');

  await page.locator('[data-testid="failure-banner-operations"]').click();
  await expect(page.locator('[data-testid="operations-panel"]')).toBeVisible();
});

test('a stopped auto-fetch shows the marker', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await installGitStreamMock(
    page,
    REPO.repoId,
    {
      ...base,
      'status.get': status({
        state: 'stopped',
        kind: 'AuthFailed',
        message: 'denied',
        at: 1,
      }),
    },
    goodChunks,
  );
  await openGraph(page);
  await expect(page.locator('[data-testid="autofetch-stopped"]')).toBeVisible();
});

test('a corrupted graph stream shows the banner and Retry re-sends graph.refresh', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL });
  const corrupt = buildGraphStreamChunk(
    REPO.repoId,
    0,
    buildPackedChunk([{ sha: SHA, subject: 'one' }], { from: 5 }),
  );
  await installGitStreamMock(
    page,
    REPO.repoId,
    { ...base, 'status.get': status(null), 'graph.refresh': { restarted: true } },
    [corrupt],
  );
  await openGraph(page);

  await expect(page.locator('[data-testid="failure-banner"]')).toBeVisible();
  await expect(page.locator('[data-testid="live-announcements"]').first()).not.toHaveText('');
  await page.locator('[data-testid="failure-banner-retry"]').click();
  await expect.poll(() => gitStreamRequests(page)).toContain('graph.refresh');
});

// Contract git-path. Backend half: appflow TestGitPathSettingEverywhere. An unusable git blocks
// the graph and the panel names the path from the server's app.init answer.
test('contract: an unusable git blocks the graph naming the configured path', async ({
  relaunch,
}) => {
  const init = contract<{ git: { kind: string; path: string } }>(
    'git-path',
    'git:app.init#unusable',
  );
  const { window: page } = await relaunch({ control: CONTROL });
  await installGitStreamMock(page, REPO.repoId, { ...base, 'app.init': init }, goodChunks);
  await openGraph(page);
  await expect(page.locator('[data-testid="git-blocked-panel"]')).toContainText(init.git.path);
});
