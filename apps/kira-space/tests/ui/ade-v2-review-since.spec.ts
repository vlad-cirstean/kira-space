import type { Locator } from '@playwright/test';
import { expect, reviewTest } from './fixtures';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import {
  change,
  GIT,
  type Relaunch,
  renderedLine,
  reviewControl,
  t,
  target,
} from './support/reviewHunkDiff';

// A diff colours the same whether or not the file was reviewed: partial review shows as a rail in
// the gutter, never as a content background that reads as "added" (P741).
// Backend half: reviewflow TestSinceReviewSides (committed, since review) and adeflow
// TestReviewWorkingReview (uncommitted), same contracts.

const SINCE = 'git-review-since';
const WORKING = 'ade-review-working-review';
const REVIEWED_AT = 'c'.repeat(40);
const TIP = 'a'.repeat(40);
const sinceDiff = contract<{ reviewedRanges: { start: number; end: number }[] }>(
  SINCE,
  'git:review.fileDiff#sinceReview',
);
const working = contract<{ files: { path: string }[] }>(WORKING, 'AdeTaskService.ReviewWorking');
const shown = contract<{ path: string; contentHash: string }>(
  WORKING,
  'AdeTaskService.ReviewWorkingDiff',
);
const workingFile = (variant: 'reviewed' | 'unreviewed') =>
  contract<{
    path: string;
    body: { hunks: { lines: { kind: string; text: string }[] }[] };
    reviewedRanges: { start: number; end: number }[];
  }>(WORKING, `AdeTaskService.ReviewWorkingFile#${variant}`);

const REPOS = [
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
];

interface Row {
  classes: string[];
  background: string;
  shadow: string;
}
type Overlays = Record<string, { content: Row; margin: Row }>;

// Per rendered line (keyed by its text): classes, computed background and box-shadow of every
// overlay node, content area and margin apart. Rows pair with lines by their `top`.
async function overlays(pane: Locator): Promise<Overlays> {
  return pane.evaluate((el): Overlays => {
    const row = (line: Element | undefined): Row => {
      const nodes = line ? Array.from(line.querySelectorAll('div')) : [];
      const styled = nodes
        .filter((n) => n.classList.contains('cdr') || n.classList.contains('cmdr'))
        .map((n) => getComputedStyle(n));
      return {
        classes: nodes.flatMap((n) => Array.from(n.classList)),
        background: styled.map((s) => s.backgroundColor).join('|'),
        shadow: styled.map((s) => s.boxShadow).join('|'),
      };
    };
    const at = (sel: string, top: string) =>
      Array.from(el.querySelectorAll(`${sel} > div`)).find(
        (d) => (d as HTMLElement).style.top === top,
      );
    const out: Overlays = {};
    for (const l of Array.from(el.querySelectorAll('.view-lines .view-line'))) {
      const top = (l as HTMLElement).style.top;
      const text = (l.textContent ?? '').replace(/\u00a0/g, ' ').trim();
      out[text] = {
        content: row(at('.view-overlays', top)),
        margin: row(at('.margin-view-overlays', top)),
      };
    }
    return out;
  });
}

const okColor = (page: import('@playwright/test').Page) =>
  page.evaluate(() => {
    const probe = document.createElement('div');
    probe.style.color = 'var(--kira-ok)';
    document.body.append(probe);
    const c = getComputedStyle(probe).color;
    probe.remove();
    return c;
  });

const TINT = 'kira-review-line-reviewed';
const hasRail = (r: Row, ok: string) => r.classes.includes(TINT) && r.shadow.includes(ok);

function openSince(relaunch: Relaunch, ranges = sinceDiff.reviewedRanges, lineCount = 5) {
  const left = contract<{ content: string }>(SINCE, 'git:file.read#left');
  const right = contract<{ content: string }>(SINCE, 'git:file.read#right');
  const diff = contract<{ body: object; path: string }>(SINCE, 'git:review.fileDiff#sinceReview');
  return relaunch({
    control: reviewControl(REPOS),
    gitStream: {
      ...GIT,
      extraResults: {
        ...GIT.extraResults,
        'review.files': {
          branchTip: TIP,
          mergeBase: 'b'.repeat(40),
          files: [
            {
              change: change(diff.path),
              review: {
                kind: 'full',
                changedSinceReview: true,
                reviewedAt: 1,
                reviewedAtSha: REVIEWED_AT,
              },
            },
          ],
        },
        'file.read': { __sequence: [left, right] },
        'review.snapshot': { kind: 'text', text: left.content, reviewedAtSha: REVIEWED_AT },
        'review.fileDiff': {
          ...diff,
          reviewedRanges: ranges,
          lineCount,
          reviewedAtSha: REVIEWED_AT,
        },
        'review.comment.list': { at: TIP, comments: [] },
      },
    },
  });
}

reviewTest('a file changed since its review colours like a normal diff', async ({ relaunch }) => {
  const { window: page } = await openSince(relaunch);
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-files-tree')} ${t('file-tree-row')}`)
    .first()
    .click();
  const host = page.locator(t('repo-diff-editor'));
  const modified = host.locator('.editor.modified');
  await expect(renderedLine(modified, 'LINE2')).toHaveCount(1, { timeout: 15_000 });
  await expect(modified.locator('.line-insert')).toHaveCount(2);
  const ok = await okColor(page);

  const check = async (pane: Locator) => {
    const o = await overlays(pane);
    // The content area never carries the review class; only LINE2 and line5 are added.
    for (const [name, r] of Object.entries(o)) {
      expect(r.content.classes.includes(TINT)).toBe(false);
      expect([name, r.content.classes.includes('line-insert')]).toEqual([
        name,
        name === 'LINE2' || name === 'line5',
      ]);
    }
    // Reviewed unchanged lines paint no review tint; the rail sits in the margin.
    for (const name of ['line1', 'line3', 'line4']) {
      const r = o[name];
      // No review green in the content area (Monaco's own zero-width char mark may sit here).
      expect(r?.content.background.includes('87, 188, 138')).toBe(false);
      expect(r && hasRail(r.margin, ok)).toBe(true);
    }
    for (const name of ['LINE2', 'line5']) {
      const r = o[name];
      expect(r && hasRail(r.margin, ok)).toBe(false);
    }
  };
  await check(modified);
  await expect(host.locator('.editor.original .line-delete')).toHaveCount(1);

  await page.locator(t('repo-diff-layout-inline')).click();
  await expect(host.locator('.monaco-diff-editor')).not.toHaveClass(/side-by-side/);
  await expect(renderedLine(host.locator('.editor.modified'), 'LINE2')).toHaveCount(1);
  await check(host.locator('.editor.modified'));
});

reviewTest('a fully reviewed file shows no rail and no tint', async ({ relaunch }) => {
  const { window: page } = await openSince(relaunch, [{ start: 1, end: 5 }]);
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-files-tree')} ${t('file-tree-row')}`)
    .first()
    .click();
  const modified = page.locator(t('repo-diff-editor')).locator('.editor.modified');
  await expect(renderedLine(modified, 'LINE2')).toHaveCount(1, { timeout: 15_000 });
  await expect(modified.locator('.line-insert')).toHaveCount(2);
  const o = await overlays(modified);
  expect(
    Object.values(o).some(
      (r) => r.content.classes.includes(TINT) || r.margin.classes.includes(TINT),
    ),
  ).toBe(false);
});

const workingControl = (file: object, diff: object) =>
  reviewControl([
    ...REPOS,
    { channel: IPC.adeTaskReviewWorking, response: { ...working, worktree: target.worktree } },
    { channel: IPC.adeTaskReviewWorkingDiff, response: diff },
    { channel: IPC.adeTaskReviewWorkingFile, response: file },
    { channel: IPC.adeTaskReviewWorkingComments, response: { comments: [] } },
  ]);

const textOf = (variant: 'reviewed' | 'unreviewed', kinds: string[]) =>
  workingFile(variant)
    .body.hunks.flatMap((h) => h.lines)
    .filter((l) => kinds.includes(l.kind))
    .map((l) => `${l.text}\n`)
    .join('');
const workingDiff = (head: string, worktree: string) => ({
  ...shown,
  language: 'plaintext',
  head: { kind: 'found', text: head, bytes: 1, limitBytes: 8_388_608 },
  worktree: { kind: 'found', text: worktree, bytes: 1, limitBytes: 8_388_608 },
});

// Monaco's own added-line colour: the same for an unreviewed and a reviewed file.
const addedBackground = async (relaunch: Relaunch, variant: 'reviewed' | 'unreviewed') => {
  const diff = workingDiff(
    textOf(variant, ['context', 'del']),
    textOf(variant, ['context', 'add']),
  );
  const { window: page } = await relaunch({
    control: workingControl(workingFile(variant), diff),
    gitStream: GIT,
  });
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-working-tree')} ${t('file-tree-row')}`)
    .first()
    .click();
  const modified = page.locator(t('repo-diff-editor')).locator('.editor.modified');
  await expect(renderedLine(modified, 'C!')).toHaveCount(1, { timeout: 15_000 });
  await expect(modified.locator('.line-insert')).toHaveCount(1);
  const ok = await okColor(page);
  const o = await overlays(modified);
  const row = o['C!'];
  expect(row?.content.classes.includes(TINT)).toBe(false);
  return {
    background: row?.content.background,
    railOnAdded: row ? hasRail(row.margin, ok) : false,
    rails: Object.values(o).filter((r) => hasRail(r.margin, ok)).length,
  };
};

const ADDED = 'rgba(155, 185, 85, 0.2)|rgba(156, 204, 44, 0.2)';

reviewTest(
  'an uncommitted unreviewed added line has only the diff colours',
  async ({ relaunch }) => {
    const got = await addedBackground(relaunch, 'unreviewed');
    expect(got.rails).toBe(0);
    expect(got.background).toBe(ADDED);
  },
);

reviewTest(
  'an uncommitted reviewed added line keeps the same diff colours',
  async ({ relaunch }) => {
    const got = await addedBackground(relaunch, 'reviewed');
    expect(got.railOnAdded).toBe(true);
    expect(got.background).toBe(ADDED);
  },
);

reviewTest('an added file colours every line the same, reviewed or not', async ({ relaunch }) => {
  const text = 'one\ntwo\nthree\n';
  const file = {
    path: 'new.txt',
    body: {
      kind: 'text',
      hunks: [
        {
          oldStart: 0,
          oldLines: 0,
          newStart: 1,
          newLines: 3,
          heading: '',
          lines: ['one', 'two', 'three'].map((s, i) => ({
            kind: 'add',
            text: s,
            newLine: i + 1,
            noNewlineAtEof: false,
          })),
        },
      ],
    },
    reviewedRanges: [{ start: 1, end: 2 }],
    lineCount: 3,
  };
  const diff = { ...workingDiff('', text), path: 'new.txt', head: { kind: 'missing' } };
  const { window: page } = await relaunch({
    control: workingControl(file, diff),
    gitStream: GIT,
  });
  await page.locator(t('ade-review-window')).waitFor();
  await page
    .locator(`${t('review-working-tree')} ${t('file-tree-row')}`)
    .first()
    .click();
  const modified = page.locator(t('repo-diff-editor')).locator('.editor.modified');
  await expect(renderedLine(modified, 'three')).toHaveCount(1, { timeout: 15_000 });
  // Monaco also marks the empty line after the final newline.
  await expect(modified.locator('.line-insert')).toHaveCount(4);
  const ok = await okColor(page);
  const o = await overlays(modified);
  const lines = ['one', 'two', 'three'].map((n) => o[n]);
  expect(
    lines.every(
      (r) => r?.content.classes.includes('line-insert') && !r.content.classes.includes(TINT),
    ),
  ).toBe(true);
  expect(new Set(lines.map((r) => r?.content.background)).size).toBe(1);
  expect(lines.map((r) => r && hasRail(r.margin, ok))).toEqual([true, true, false]);
});
