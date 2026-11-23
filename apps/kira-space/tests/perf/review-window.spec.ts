import type { Page } from '@playwright/test';
import { AGENT_PANEL_ON, reviewTest } from '../ui/fixtures';
import { IPC } from '../ui/support/ipcChannels';
import { GIT, reviewControl, t, target } from '../ui/support/reviewHunkDiff';

// P739 review window probe: a PR-sized committed file list, then boot to first row, five file opens
// (click to diff lines shown), three checkbox marks, one folder mark, filter typing. Prints
// `REVIEWUI` lines; asserts nothing. Knobs: KIRA_REVIEW_FILES (default 3000), KIRA_REVIEW_LINES
// (lines per side, default 400). `bunx playwright test -c apps/kira-space/playwright.perf.config.ts
// --project=perf review-window`. Wall times include Playwright round trips; compare runs, not to 16 ms.

const N = Number(process.env.KIRA_REVIEW_FILES ?? 3000);
const LINES = Number(process.env.KIRA_REVIEW_LINES ?? 400);
const files = Array.from({ length: N }, (_, i) => ({
  change: {
    kind: 'modified',
    path: `pkg${String(i % 40).padStart(2, '0')}/sub${i % 7}/file${String(i).padStart(4, '0')}.ts`,
    similarity: 0,
  },
  review: { kind: i % 2 ? 'full' : 'none', changedSinceReview: i % 10 === 0 },
}));
const TEXT = Array.from({ length: LINES }, (_, i) => `export const v${i} = ${i};\n`).join('');

const settle = (page: Page) =>
  page.evaluate(
    () => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r()))),
  );

async function timed(page: Page, label: string, act: () => Promise<void>): Promise<void> {
  const t0 = Date.now();
  await act();
  await settle(page);
  console.log(`REVIEWUI ${label} ms=${Date.now() - t0}`);
}

reviewTest('review window, big PR', async ({ relaunch }) => {
  const start = Date.now();
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
        'review.files': { ...GIT.extraResults['review.files'], files },
        'file.read': { kind: 'found', content: TEXT },
        'review.snapshot': { kind: 'none', text: null, reviewedAtSha: null },
        'review.fileDiff': {
          path: 'x',
          deltaSource: 'noSnapshot',
          body: { kind: 'text', hunks: [] },
          reviewedRanges: [],
          lineCount: LINES,
          reviewedAtSha: null,
        },
        'review.mark': { review: { kind: 'full', changedSinceReview: false } },
        'review.markMany': { results: [] },
      },
    },
  });
  const rows = page.locator(`${t('review-files-tree')} ${t('file-tree-row')}`);
  await rows.first().waitFor();
  await settle(page);
  const dom = await page.evaluate(() => document.getElementsByTagName('*').length);
  console.log(
    `REVIEWUI files=${N} boot ms=${Date.now() - start} rows=${await rows.count()} dom=${dom}`,
  );

  // Files near the top: present whatever the render window.
  const fileRows = rows.filter({ hasText: '.ts' });
  for (let k = 1; k <= 5; k++) {
    await timed(page, 'open', async () => {
      await fileRows.nth(k).click();
      await page
        .locator(`${t('repo-diff-editor')} .monaco-diff-editor .editor.modified .view-line`)
        .first()
        .waitFor();
    });
  }
  for (let k = 0; k < 3; k++) {
    await timed(page, 'mark', () =>
      rows
        .nth(10 + k * 3)
        .getByRole('checkbox')
        .click(),
    );
  }
  await timed(page, 'folderMark', () => page.locator(t('review-folder-check')).nth(1).click());
  await page.locator(t('review-pane-files')).click();
  await timed(page, 'filter', () => page.keyboard.type('file12'));
});
