import { buildPackedChunk } from '@kira/git-core/testing/packedChunk';
import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { installGitStreamMock } from './support/gitStreamMock';
import { buildGraphStreamChunk } from './support/graphStreamFixture';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// P220: lane lines must stay painted. The graph column used to seed from `laneCount` before the
// layout worker answered (0 lanes -> 17px), and the row SVG clips lanes past the column edge, so
// every lane-0 edge vanished. Needs a real layout worker, SlickGrid and paint: pixel-level check.

const REPO = {
  id: 'repo-lines-1',
  name: 'lines-repo',
  root: '/tmp/lines-repo',
  repoId: '/tmp/lines-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const CONTROL: ControlSnapshot[] = [{ channel: IPC.codeWorkspaceListRepos, response: [REPO] }];

const ROWS = 300;
const GRAPH_MIN = 40;
// geometry.ts: TWO_LANE_WIDTH = padLeft 11 + 2 * laneWidth 13 + gutterPad 6; laneX(0) = 17.5.
const TWO_LANE_WIDTH = 43;
const LANE_0_X = 17;
const sha = (n: number) => n.toString(16).padStart(4, '0').repeat(10);

// Row 0 merges rows 1 and 2, which both fork from row 3: two lanes near the top, one below.
function commitRows() {
  return Array.from({ length: ROWS }, (_, n) => {
    const parents =
      n === 0 ? [sha(1), sha(2)] : n === 1 || n === 2 ? [sha(3)] : n < ROWS - 1 ? [sha(n + 1)] : [];
    return { sha: sha(n), subject: `commit ${n}`, parents };
  });
}

const STREAM_CHUNKS = [buildGraphStreamChunk(REPO.repoId, 0, buildPackedChunk(commitRows()))];

const REPO_OPEN = {
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

const MOCK_RESULTS = {
  'repo.open': REPO_OPEN,
  'refs.list': {
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
  },
  'graph.refresh': {},
  'graph.status': { loaded: ROWS, remaining: 0, exhausted: true },
};

const svgs = (p: Page) => p.locator('[data-testid="commit-grid"] svg[data-testid="graph-svg"]');

async function graphWidth(p: Page): Promise<number> {
  return svgs(p)
    .first()
    .evaluate((el) => el.getBoundingClientRect().width);
}

/** Fraction of pixel rows in a ~200px strip that carry a lane-0 coloured pixel on lane 0's x. */
async function laneLineCoverage(p: Page): Promise<number> {
  const box = await svgs(p).first().boundingBox();
  const viewport = await p
    .locator('[data-testid="commit-grid"] .slick-viewport-top.slick-viewport-left')
    .boundingBox();
  if (!box || !viewport) throw new Error('no graph svg rendered');
  const shot = await p.screenshot({
    clip: { x: box.x, y: viewport.y, width: GRAPH_MIN + 20, height: 200 },
    scale: 'css',
  });
  return p.evaluate(
    async ({ b64, x }) => {
      const stroke = getComputedStyle(
        document.querySelector('svg[data-testid="graph-svg"] path.stroke-graph-lane-0')!,
      ).stroke;
      const probe = document.createElement('canvas').getContext('2d')!;
      probe.fillStyle = stroke;
      probe.fillRect(0, 0, 1, 1);
      const [lr, lg, lb] = probe.getImageData(0, 0, 1, 1).data;
      const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
      const bitmap = await createImageBitmap(new Blob([bytes], { type: 'image/png' }));
      const canvas = document.createElement('canvas');
      canvas.width = bitmap.width;
      canvas.height = bitmap.height;
      const ctx = canvas.getContext('2d')!;
      ctx.drawImage(bitmap, 0, 0);
      const { data } = ctx.getImageData(x, 0, 1, bitmap.height);
      let hits = 0;
      for (let y = 0; y < bitmap.height; y++) {
        const d =
          Math.abs(data[y * 4] - lr) +
          Math.abs(data[y * 4 + 1] - lg) +
          Math.abs(data[y * 4 + 2] - lb);
        if (d < 90) hits++;
      }
      return hits / bitmap.height;
    },
    { b64: shot.toString('base64'), x: LANE_0_X },
  );
}

async function expectLines(p: Page, minWidth: number): Promise<void> {
  await expect.poll(() => graphWidth(p)).toBeGreaterThanOrEqual(minWidth);
  await expect.poll(() => laneLineCoverage(p)).toBeGreaterThanOrEqual(0.9);
}

test('first-ever mount: lane lines paint, and survive a row click and a scroll', async ({
  relaunch,
}) => {
  const { window: win } = await relaunch({ control: CONTROL });
  await installGitStreamMock(win, REPO.repoId, MOCK_RESULTS, STREAM_CHUNKS);
  await win.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`).click();
  await expect(svgs(win).first()).toBeVisible();

  await expectLines(win, TWO_LANE_WIDTH);

  await win.locator('[data-testid="commit-grid"] .slick-row[data-row="3"]').click();
  await expectLines(win, TWO_LANE_WIDTH);

  await win.locator('[data-testid="commit-grid"] .slick-viewport-top.slick-viewport-left').evaluate(
    (el, rowHeight) => {
      el.scrollTop = 150 * rowHeight;
    },
    (await svgs(win).first().boundingBox())?.height ?? 28,
  );
  await expect(win.locator('[data-testid="commit-grid"] .slick-row[data-row="150"]')).toBeVisible();
  await expectLines(win, TWO_LANE_WIDTH);
});

test('restored tab with a persisted graph width below the column minimum still paints lanes', async ({
  relaunch,
}) => {
  const viewState = {
    version: 8,
    repoId: REPO.repoId,
    loadedRows: ROWS,
    detailOpen: false,
    scrollRow: 0,
    selectedSha: null,
    columnWidths: { author: 140, date: 152, graph: 17 },
    dateFormat: 'relative',
    detailWidth: 380,
    fileListMode: 'tree',
    searchCaseSensitive: false,
    searchWholeWord: false,
    searchRegex: false,
    searchScope: 'both',
    searchOpen: false,
    collapseBranches: false,
  };
  const { window: win } = await relaunch({
    control: [
      ...CONTROL,
      {
        channel: IPC.tabsList,
        response: [
          {
            id: 'restored-repo-graph',
            kind: 'repo-graph',
            connectionId: null,
            path: '',
            order: 0,
            active: true,
            workspaceId: REPO.id,
            state: { viewState, reviewSession: null },
          },
        ],
      },
    ],
    gitStream: {
      repoId: REPO.repoId,
      extraResults: MOCK_RESULTS,
      graphStreamChunks: STREAM_CHUNKS,
    },
  });
  await expect(svgs(win).first()).toBeVisible();
  await expectLines(win, GRAPH_MIN);
});
