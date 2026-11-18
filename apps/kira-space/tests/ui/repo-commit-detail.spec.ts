import { expect, test } from './fixtures';
import { installClipboardSpy, lastClipboardWrite } from './studio/support/clipboard';
import { contract } from './support/contract';
import { gitStreamParams } from './support/gitStreamMock';
import {
  COMMIT_DETAIL,
  openPortGraph,
  rootRow,
  SHA_A,
  SHA_B,
  singleRowChunks,
} from './support/gitUiPortFixtures';

// Contract git-commit-detail. Backend half: gitflow TestCommitDetailAndDiff (rename) and
// TestCommitDetailMergeParentSelector (merge). The sha, parents and times are the graph fixture's;
// the file list, kinds and rename paths are the backend's.

interface DetailFile {
  path: string;
  kind: string;
  originalPath?: string;
  similarity?: number;
}
interface Detail {
  files: DetailFile[];
}

async function openDetail(
  relaunch: Parameters<typeof openPortGraph>[0],
  detail: Detail,
  parents: string[],
) {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'merge side')]),
    results: { 'commit.detail': { ...detail, sha: SHA_A, parents } },
  });
  await page.locator('[data-testid="commit-grid"] .slick-row[data-row="0"]').click();
  await expect(page.locator('[data-testid="file-tree"]')).toBeVisible();
  return page;
}

test('contract: a rename commit lists the new path with its old path in the tip', async ({
  relaunch,
}) => {
  const detail = contract<Detail>('git-commit-detail', 'git:commit.detail#rename');
  const renamed = detail.files.find((f) => f.kind === 'renamed');
  if (!renamed?.originalPath) throw new Error('contract lost the rename');
  const page = await openDetail(relaunch, detail, [SHA_B]);
  const tree = page.locator('[data-testid="file-tree"]');
  await expect(tree).toContainText(renamed.path.split('/').pop() ?? renamed.path);
  await expect(tree.locator(`[data-kira-tip*="${renamed.originalPath}"]`).first()).toBeVisible();
  await expect(
    tree.locator(`[data-kira-tip*="${renamed.similarity}% similar"]`).first(),
  ).toBeVisible();
});

test('contract: a binary and a mode-only change list by name-status with no line counts', async ({
  relaunch,
}) => {
  const detail = contract<Detail>('git-commit-detail', 'git:commit.detail#binary-mode');
  const letters: Record<string, string> = { added: 'A', modified: 'M', deleted: 'D' };
  expect(detail.files.length).toBeGreaterThanOrEqual(3);
  const page = await openDetail(relaunch, detail, [SHA_B]);
  const tree = page.locator('[data-testid="file-tree"]');
  for (const f of detail.files) {
    const row = tree.locator('[data-testid="file-tree-row"]', {
      hasText: f.path.split('/').pop() ?? f.path,
    });
    await expect(row).toHaveCount(1);
    await expect(row.locator('[data-testid="file-tree-status"]')).toHaveText(letters[f.kind]);
    expect(await row.innerText()).not.toMatch(/[+\u2212-]\d+/);
  }
});

for (const parent of [0, 1] as const) {
  test(`contract: a merge commit lists the files against parent ${parent + 1}`, async ({
    relaunch,
  }) => {
    const detail = contract<Detail & { parentIndex: number }>(
      'git-commit-detail',
      `git:commit.detail#merge-parent-${parent}`,
    );
    const page = await openDetail(relaunch, detail, [SHA_B, '55'.repeat(20)]);
    const tree = page.locator('[data-testid="file-tree"]');
    for (const f of detail.files) {
      await expect(tree).toContainText(f.path);
    }
    await expect(tree.locator('select option')).toHaveCount(2);
    await tree.locator('select').selectOption(String(detail.parentIndex));
    await expect
      .poll(async () => (await gitStreamParams(page, 'commit.detail')).at(-1))
      .toMatchObject({ parentIndex: detail.parentIndex });
  });
}

// Contract git-copy-values. Backend half: gitflow TestCopyValues (sha, subject, body and paths equal git's own).
test('contract: the commit sha button copies the full 40-character sha', async ({ relaunch }) => {
  const detail = contract<{ sha: string; subject: string }>(
    'git-copy-values',
    'git:commit.detail#copy',
  );
  expect(detail.sha).toMatch(/^[0-9a-f]{40}$/);
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(detail.sha, detail.subject)]),
    results: { 'commit.detail': { ...COMMIT_DETAIL, ...detail, files: [], parents: [] } },
  });
  await page.locator('[data-testid="commit-grid"] .slick-row[data-row="0"]').click();
  await expect(page.locator('[data-testid="commit-meta-sha"]')).toBeVisible();
  await installClipboardSpy(page);
  await page.locator('[data-testid="commit-meta-sha"]').click();
  await expect.poll(() => lastClipboardWrite(page)).toBe(detail.sha);
});
