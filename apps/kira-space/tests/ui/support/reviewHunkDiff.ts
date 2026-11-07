import type { Locator, Page } from '@playwright/test';
import { AGENT_PANEL_ON, type reviewTest } from '../fixtures';
import { adeFixture, adeV2Control } from './adeV2';
import { contract } from './contract';
import { IPC } from './ipcChannels';
import type { ControlSnapshot } from './types';

// Review-window fixtures shared by the review UI specs and the visual spec.

export const t = (id: string) => `[data-testid="${id}"]`;
export const target = adeFixture<{
  gitRepoId: string;
  codeRepoId: string;
  branchId: string;
  worktree: string;
}>('review-target');

export const change = (path: string) => ({
  kind: 'modified',
  path,
  similarity: 0,
});
export const status = (kind: 'none' | 'full', changedSinceReview: boolean) => ({
  kind,
  changedSinceReview,
});

export const REVIEW_FILES = {
  branchTip: 'a'.repeat(40),
  mergeBase: 'b'.repeat(40),
  files: [
    { change: change('src/pending.ts'), review: status('none', false) },
    { change: change('src/done.ts'), review: status('full', false) },
    { change: change('src/moved.ts'), review: status('full', true) },
  ],
};

export const GIT = {
  repoId: target.gitRepoId,
  extraResults: {
    'review.resolveBase': {
      branch: 'feat/billing-dashboard',
      base: 'main',
      reason: 'override',
      range: { kind: 'ready', commitCount: 3 },
      candidates: [],
    },
    'review.files': REVIEW_FILES,
    'review.session.load': null,
    'repo.open': null,
  },
};

export function reviewControl(extra: readonly ControlSnapshot[] = []): ControlSnapshot[] {
  const defaults: ControlSnapshot[] = [
    { channel: IPC.adeTaskReviewWindowTarget, response: target },
    { channel: IPC.adeTaskReviewAgent, response: { session: null, hostWindowKey: '' } },
  ];
  const overridden = new Set(extra.map((e) => e.channel));
  return adeV2Control([...defaults.filter((d) => !overridden.has(d.channel)), ...extra]);
}

export const boxOf = async (l: Locator) => {
  const b = await l.boundingBox();
  if (!b) throw new Error('no box');
  return b;
};

export const renderedLine = (modified: Locator, text: string) =>
  modified
    .locator('.view-lines .view-line')
    .filter({ hasText: new RegExp(`^\\s*${text.replace(' ', '\\s')}`) });

export type Relaunch = Parameters<Parameters<typeof reviewTest>[2]>[0]['relaunch'];

export const HUNK_COMMENT = {
  id: '1',
  path: 'src/pending.ts',
  range: { start: 12, end: 12 },
  body: 'Why this?',
  anchor: 'exact',
  anchorSha: 'a'.repeat(40),
  createdAt: 1_790_036_500_000,
  kind: 'change',
  state: 'open',
  reply: '',
  repliedAt: null,
  sentAt: null,
};

export interface HunkDiffOptions {
  comments?: readonly object[];
  /** Replaces `line 101` (unchanged context) on both sides. */
  line101?: string;
  extraResults?: Record<string, unknown>;
}

export async function openHunkDiff(
  relaunch: Relaunch,
  opts: HunkDiffOptions = {},
): Promise<{ page: Page; modified: Locator }> {
  const read = (side: 'left' | 'right') => {
    const blob = contract<{ content: string }>('git-review-hunks', `git:file.read#${side}`);
    if (opts.line101 === undefined) return blob;
    const lines = blob.content.split('\n');
    lines[100] = opts.line101;
    return { ...blob, content: lines.join('\n') };
  };
  const { window: page } = await relaunch({
    control: reviewControl([
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      {
        channel: IPC.codeWorkspaceListRepos,
        response: [
          {
            id: target.codeRepoId,
            name: 'web-app',
            root: '/tmp/web-app',
            repoId: target.gitRepoId,
            sortOrder: 1,
            color: 'none',
            createdAt: '2026-01-01T00:00:00.000Z',
          },
        ],
      },
    ]),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'file.read': { __sequence: [read('left'), read('right')] },
        'review.snapshot': { kind: 'none', text: null, reviewedAtSha: null },
        'review.fileDiff': {
          ...contract<object>('git-review-hunks', 'git:review.fileDiff#range'),
          path: 'src/pending.ts',
        },
        'review.comment.list': {
          at: 'a'.repeat(40),
          comments: opts.comments ?? [HUNK_COMMENT],
        },
        ...opts.extraResults,
      },
    },
  });
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-files-tree')} ${t('file-tree-row')}`, { hasText: 'pending.ts' })
    .click();
  const modified = page.locator(t('repo-diff-editor')).locator('.editor.modified');
  return { page, modified };
}

/** A review window over `files`, with the diff reads left to `extraResults`. */
export async function openDiffWindow(
  relaunch: Parameters<Parameters<typeof reviewTest>[2]>[0]['relaunch'],
  files: readonly string[],
  extraResults: Record<string, unknown>,
): Promise<Page> {
  const { window: page } = await relaunch({
    storage: AGENT_PANEL_ON,
    control: reviewControl([
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      {
        channel: IPC.codeWorkspaceListRepos,
        response: [
          {
            id: target.codeRepoId,
            name: 'web-app',
            root: '/tmp/web-app',
            repoId: target.gitRepoId,
            sortOrder: 1,
            color: 'none',
            createdAt: '2026-01-01T00:00:00.000Z',
          },
        ],
      },
    ]),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'review.files': {
          ...REVIEW_FILES,
          files: files.map((path) => ({ change: change(path), review: status('none', false) })),
        },
        'review.snapshot': { kind: 'none', text: null, reviewedAtSha: null },
        'review.fileDiff': {
          path: files[0],
          deltaSource: 'noSnapshot',
          body: { kind: 'text', hunks: [] },
          reviewedRanges: [],
          lineCount: 200,
          reviewedAtSha: null,
        },
        ...extraResults,
      },
    },
  });
  await page.locator(t('ade-review-window')).waitFor();
  return page;
}
