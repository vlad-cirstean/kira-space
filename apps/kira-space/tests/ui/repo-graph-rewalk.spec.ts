import { buildPackedChunk } from '@kira/git-core/testing/packedChunk';
import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { installGitStreamMock } from './support/gitStreamMock';
import { buildGraphStreamChunk } from './support/graphStreamFixture';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

const REPO = {
  id: 'repo-rewalk-1',
  name: 'rewalk-repo',
  root: '/tmp/rewalk-repo',
  repoId: '/tmp/rewalk-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const CONTROL: ControlSnapshot[] = [{ channel: IPC.codeWorkspaceListRepos, response: [REPO] }];

const sha = (n: number) => n.toString(16).padStart(2, '0').repeat(20);
const DICTIONARY_BASE = 2;

// Linear history, row i's parent is row i + 1. A re-walk keeps the same tip sha (`refs.list` is
// static in the mock) but changes every subject, so the label tells which walk a row came from.
function rows(from: number, to: number, label: string) {
  return Array.from({ length: to - from }, (_, i) => {
    const n = from + i;
    return { sha: sha(n), subject: `${label} ${n}`, parents: [sha(n + 1)] };
  });
}

function page(from: number, to: number, label: string, seq: number) {
  return buildGraphStreamChunk(
    REPO.repoId,
    seq,
    buildPackedChunk(rows(from, to, label), {
      from,
      ...(from > 0 ? { dictionary: [], dictionaryBase: DICTIONARY_BASE } : {}),
    }),
  );
}

const REFS_LIST = {
  branches: [
    {
      refname: 'refs/heads/main',
      kind: 'branch',
      shortName: 'main',
      objectId: sha(0),
      peeledObjectId: undefined,
      upstream: undefined,
      track: undefined,
      committerDate: 0,
      isHead: true,
      checkedOutIn: undefined,
      annotation: undefined,
    },
  ],
  remoteBranches: [],
  tags: [],
  head: { kind: 'branch', name: 'main' },
};

function message(p: Page, row: number) {
  return p.locator(`[data-testid="commit-grid"] .slick-row[data-row="${row}"] .kira-cell-message`);
}

test('a re-walk from row 0 with a shorter first page keeps the graph rendering, and a click still selects', async ({
  relaunch,
}) => {
  const { window: win } = await relaunch({ control: CONTROL });
  const errors: string[] = [];
  win.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text());
  });
  win.on('pageerror', (err) => errors.push(err.message));

  const clickedSubject = 'new 1';
  const commitDetail = {
    sha: sha(1),
    parents: [sha(2)],
    author: { name: 'Ada Lovelace', email: 'ada@example.com', timestamp: 1_700_000_000 },
    committer: { name: 'Ada Lovelace', email: 'ada@example.com', timestamp: 1_700_000_000 },
    subject: clickedSubject,
    body: '',
    trailers: [],
    signature: { status: 'N', signer: '' },
    decoration: [],
    parentIndex: 0,
    files: [],
  };

  await installGitStreamMock(
    win,
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
      'refs.list': REFS_LIST,
      'graph.refresh': {},
      'graph.status': { loaded: 10, remaining: 0, exhausted: true },
      'commit.detail': commitDetail,
    },
    undefined,
    [
      [page(0, 5, 'old', 0), page(5, 10, 'old', 1)],
      [page(0, 3, 'new', 0), page(3, 8, 'new', 1)],
    ],
  );
  await win.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`).click();
  await expect(message(win, 9)).toHaveText('old 9');

  await win
    .locator('[data-testid="repo-graph-host"]')
    .getByRole('button', { name: 'Refresh' })
    .click();
  await expect(message(win, 0)).toHaveText('new 0');

  const rendered = win.locator('[data-testid="commit-grid"] .slick-row .kira-cell-message');
  const texts = await rendered.allTextContents();
  expect(texts.length).toBeGreaterThan(0);
  expect(texts.every((text) => text.trim() !== '')).toBe(true);

  await message(win, 1).click();
  await expect(win.locator('[data-testid="commit-meta"]')).toContainText(clickedSubject);
  await expect(message(win, 0)).toHaveText('new 0');
  await expect(message(win, 7)).toHaveText('new 7');
  expect(errors.filter((text) => text.includes('ShaTable'))).toEqual([]);
});
