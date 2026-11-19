import { buildPackedChunk } from '@kira/git-core/testing/packedChunk';
import {
  attachResults,
  FLICK_LADDER,
  type FlickResult,
  measureFlick,
  RssSampler,
} from '@workbench/testing/ui/perfProbe';
import { expect, test } from '../ui/fixtures';
import { installGitStreamMock } from '../ui/support/gitStreamMock';
import { buildGraphStreamChunk } from '../ui/support/graphStreamFixture';
import { IPC } from '../ui/support/ipcChannels';
import type { ControlSnapshot } from '../ui/support/types';

// Git graph: open time plus momentum flicks. GRAPH_N commits (default 10000), 500 per chunk.

const N = Number(process.env.GRAPH_N ?? 10_000);
const CHUNK = 500;
const REPO = {
  id: 'repo-perf-graph',
  name: 'perf-repo',
  root: '/tmp/perf-repo',
  repoId: '/tmp/perf-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};
const sha = (i: number): string => (i + 1).toString(16).padStart(40, '0');
const CONTROL: ControlSnapshot[] = [{ channel: IPC.codeWorkspaceListRepos, response: [REPO] }];

test('graph scroll', async ({ relaunch }, info) => {
  const rows = Array.from({ length: N }, (_, i) => {
    const parents: string[] = [];
    if (i + 1 < N) parents.push(sha(i + 1));
    if (i % 7 === 0 && i + 4 < N) parents.push(sha(i + 4));
    return {
      sha: sha(i),
      subject: `commit ${i}: realistic length subject line for row ${i}`,
      parents,
    };
  });
  const chunks = Array.from({ length: Math.ceil(N / CHUNK) }, (_, c) =>
    buildGraphStreamChunk(
      REPO.repoId,
      c,
      buildPackedChunk(
        rows.slice(c * CHUNK, (c + 1) * CHUNK),
        c === 0 ? { from: 0 } : { from: c * CHUNK, dictionary: [], dictionaryBase: 2 },
      ),
    ),
  );
  const { window: page } = await relaunch({ control: CONTROL });
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
      'refs.list': {
        branches: [
          {
            refname: 'refs/heads/main',
            kind: 'branch',
            shortName: 'main',
            objectId: sha(0),
            committerDate: 1735598880,
            isHead: true,
          },
        ],
        remoteBranches: [],
        tags: [],
        head: { kind: 'branch', name: 'main' },
      },
    },
    chunks,
  );

  const rss = new RssSampler();
  rss.start();
  rss.phase('open');
  const openedAt = Date.now();
  await page.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`).click();
  await expect(page.locator('[data-testid="commit-grid"] .slick-row[data-row="5"]')).toBeVisible({
    timeout: 60_000,
  });
  console.log(`PERF graph-open: first rows ${Date.now() - openedAt} ms (N=${N})`);
  await page.waitForTimeout(2000);

  const box = await page
    .locator('[data-testid="commit-grid"] .slick-viewport')
    .first()
    .boundingBox();
  if (!box) throw new Error('graph viewport not laid out');
  await page.mouse.move(box.x + 300, box.y + 200);

  const results: FlickResult[] = [];
  for (const { label, opts } of FLICK_LADDER)
    results.push(await measureFlick(page, rss, label, opts));
  rss.stop();
  await attachResults(info, results);
});
