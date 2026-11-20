import { buildPackedChunk } from '@kira/git-core/testing/packedChunk';
import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { installGitStreamMock } from './support/gitStreamMock';
import {
  CHUNK_ROWS,
  CROSS_PAGE_FROM,
  CROSS_PAGE_TO,
  PAGE_SIZE,
  PAGING_ROWS,
  pagingRows,
  pagingSha,
} from './support/graphPagingFixture';
import { buildGraphStreamChunk } from './support/graphStreamFixture';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// P225: outer-lane nodes must not clip, and a second-parent line must keep its own lane, across
// scroll, row click and Load more. Needs the real layout worker, SlickGrid and paint.

const REPO = {
  id: 'repo-paging-1',
  name: 'paging-repo',
  root: '/tmp/paging-repo',
  repoId: '/tmp/paging-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const CONTROL: ControlSnapshot[] = [{ channel: IPC.codeWorkspaceListRepos, response: [REPO] }];

const DICTIONARY_BASE = 2;

function chunk(from: number, to: number, seq: number, last: boolean) {
  return buildGraphStreamChunk(
    REPO.repoId,
    seq,
    buildPackedChunk(pagingRows(from, to), {
      from,
      ...(from > 0 ? { dictionary: [], dictionaryBase: DICTIONARY_BASE } : {}),
    }),
    last ? {} : { exhausted: false, remaining: PAGING_ROWS - to },
  );
}

function pageChunks(from: number, to: number, last: boolean) {
  const out = [];
  for (let start = from; start < to; start += CHUNK_ROWS) {
    const end = Math.min(start + CHUNK_ROWS, to);
    out.push(chunk(start, end, out.length, last && end === to));
  }
  return out;
}

const OPENS = [pageChunks(0, PAGE_SIZE, false), pageChunks(PAGE_SIZE, PAGING_ROWS, true)];

const MOCK_RESULTS = {
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
        objectId: pagingSha(0),
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
  'graph.loadMore': { started: true },
  'graph.status': { loaded: PAGING_ROWS, remaining: 0, exhausted: true },
};

const GRID = '[data-testid="commit-grid"]';
const VIEWPORT = `${GRID} .slick-viewport-top.slick-viewport-left`;
const rowAt = (p: Page, row: number) => p.locator(`${GRID} .slick-row[data-row="${row}"]`);

interface RowReport {
  readonly row: number;
  readonly clipped: readonly number[];
  readonly runs: readonly number[];
  readonly nodes: readonly number[];
}

async function openRepo(p: Page, errors: string[]): Promise<void> {
  p.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(msg.text());
  });
  p.on('pageerror', (err) => errors.push(err.message));
  await installGitStreamMock(p, REPO.repoId, MOCK_RESULTS, undefined, OPENS);
  await p.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`).click();
  await expect(p.locator(`${GRID} svg[data-testid="graph-svg"]`).first()).toBeVisible();
}

/** Scrolls inside the page (no actionability waits: SlickGrid recycles row nodes under a scroll),
 *  steering by the rendered rows' own data-row until `row` is rendered and centred. */
async function scrollToRow(p: Page, row: number, loadedRows: number): Promise<void> {
  await p.locator(VIEWPORT).evaluate(
    async (vp, args) => {
      const frame = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      const avg = vp.scrollHeight / args.loadedRows;
      for (let i = 0; i < 30; i++) {
        const rows = [...vp.querySelectorAll('.slick-row[data-row]')].map((el) =>
          Number(el.getAttribute('data-row')),
        );
        const target = vp.querySelector(`.slick-row[data-row="${args.row}"]`);
        if (target) {
          target.scrollIntoView({ block: 'center' });
          await frame();
          await frame();
          if (
            vp.querySelector(`.slick-row[data-row="${args.row}"] svg[data-testid="graph-svg"] > *`)
          )
            return;
          continue;
        }
        const mid = rows.length ? rows[Math.floor(rows.length / 2)]! : 0;
        vp.scrollTop += (args.row - mid) * avg || args.row * avg;
        await frame();
        await frame();
      }
      throw new Error(`row ${args.row} never rendered`);
    },
    { row, loadedRows },
  );
}

/** Per visible row: node x's the row SVG clips, full-height runs' x's, node x's. */
async function visibleReports(p: Page): Promise<RowReport[]> {
  return p.evaluate(
    ({ viewportSel, gridSel }) => {
      const vp = document.querySelector(viewportSel)!.getBoundingClientRect();
      const out: RowReport[] = [];
      for (const el of document.querySelectorAll(`${gridSel} .slick-row[data-row]`)) {
        const rect = el.getBoundingClientRect();
        if (rect.bottom <= vp.top || rect.top >= vp.bottom) continue;
        const svg = el.querySelector('svg[data-testid="graph-svg"]');
        if (!svg) continue;
        const width = svg.getBoundingClientRect().width;
        const height = svg.getBoundingClientRect().height;
        const clipped: number[] = [];
        const nodes: number[] = [];
        for (const c of svg.querySelectorAll('circle')) {
          const cx = Number(c.getAttribute('cx'));
          const r = Number(c.getAttribute('r'));
          nodes.push(cx);
          if (cx + r > width) clipped.push(cx);
        }
        const runs: number[] = [];
        for (const path of svg.querySelectorAll('path')) {
          for (const m of (path.getAttribute('d') ?? '').matchAll(/M([\d.]+),-[\d.]+ V([\d.]+)/g)) {
            if (Number(m[2]) > height) runs.push(Number(m[1]));
          }
        }
        out.push({ row: Number(el.getAttribute('data-row')), clipped, runs, nodes });
      }
      return out;
    },
    { viewportSel: VIEWPORT, gridSel: GRID },
  );
}

let widestNode = 0;

async function clippedNodes(p: Page): Promise<string[]> {
  const reports = await visibleReports(p);
  for (const report of reports) widestNode = Math.max(widestNode, ...report.nodes);
  return reports
    .filter((r) => r.clipped.length > 0)
    .map((r) => `row ${r.row}: cx ${r.clipped.join(',')}`);
}

/** A lane holds one open edge or one node per row; two runs on one x, or a run on the node's x,
 *  means a line is drawn on top of another. */
async function stackedRuns(p: Page): Promise<string[]> {
  return (await visibleReports(p))
    .filter((r) => new Set(r.runs).size < r.runs.length || r.runs.some((x) => r.nodes.includes(x)))
    .map((r) => `row ${r.row}: runs ${r.runs.join(',')} nodes ${r.nodes.join(',')}`);
}

async function sweep(
  p: Page,
  rows: readonly number[],
  loadedRows: number,
  check: (p: Page) => Promise<string[]>,
): Promise<string[]> {
  const problems: string[] = [];
  for (const row of rows) {
    await scrollToRow(p, row, loadedRows);
    problems.push(...(await check(p)));
  }
  return [...new Set(problems)];
}

const range = (from: number, to: number, step: number) =>
  Array.from({ length: Math.ceil((to - from) / step) }, (_, i) => from + i * step);

async function loadMore(p: Page): Promise<void> {
  const button = p
    .locator('[data-testid="repo-graph-host"]')
    .getByRole('button', { name: /^Load / });
  await button.click();
  await expect(button).toHaveCount(0);
  await scrollToRow(p, PAGING_ROWS - 1, PAGING_ROWS);
  await expect(rowAt(p, PAGING_ROWS - 1).locator('.kira-cell-message')).toHaveText(
    `commit ${PAGING_ROWS - 1}`,
  );
}

test('outer-lane commits keep their node across scroll, click and Load more', async ({
  relaunch,
}) => {
  const { window: win } = await relaunch({ control: CONTROL });
  const errors: string[] = [];
  await openRepo(win, errors);

  await scrollToRow(win, PAGE_SIZE - 1, PAGE_SIZE);
  expect(await clippedNodes(win)).toEqual([]);
  await rowAt(win, PAGE_SIZE - 5)
    .locator('.kira-cell-message')
    .click();
  expect(await clippedNodes(win)).toEqual([]);

  await loadMore(win);
  const sweepRows = range(PAGE_SIZE, PAGING_ROWS, 30);
  expect(await sweep(win, sweepRows, PAGING_ROWS, clippedNodes)).toEqual([]);
  expect(widestNode).toBeGreaterThan(60); // the sweep did reach outer lanes
  expect(errors).toEqual([]);
});

test("a merge's second-parent line keeps its own lane, before and after Load more", async ({
  relaunch,
}) => {
  const { window: win } = await relaunch({ control: CONTROL });
  const errors: string[] = [];
  await openRepo(win, errors);

  const page1Rows = range(0, 120, 20);
  const boundary = [CROSS_PAGE_FROM - 5, PAGE_SIZE - 1];
  expect(await sweep(win, page1Rows, PAGE_SIZE, stackedRuns)).toEqual([]);
  expect(await sweep(win, boundary, PAGE_SIZE, stackedRuns)).toEqual([]);

  await loadMore(win);
  expect(
    await sweep(win, [CROSS_PAGE_FROM - 5, CROSS_PAGE_TO + 5], PAGING_ROWS, stackedRuns),
  ).toEqual([]);
  expect(await sweep(win, page1Rows, PAGING_ROWS, stackedRuns)).toEqual([]);
  expect(errors).toEqual([]);
});

const handle = (p: Page, name: string) =>
  p.getByRole('separator', { name: `Resize ${name} column`, exact: true });

/** Closes the detail pane. The author and date handles are not rendered while it is open. */
async function closeDetail(p: Page): Promise<void> {
  await expect(p.locator('[data-testid="detail-region"]')).toBeVisible();
  await p.keyboard.press('Escape');
  await expect(handle(p, 'author')).toBeVisible();
}

async function drag(p: Page, name: string, dx: number): Promise<void> {
  const box = (await handle(p, name).boundingBox())!;
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await p.mouse.move(x, y);
  await p.mouse.down();
  for (let i = 1; i <= 10; i++) await p.mouse.move(x + (dx * i) / 10, y);
  await p.mouse.up();
}

/** Horizontal overflow signs: canvas wider than the viewport, a scrolled viewport, a graph cell
 *  off the viewport's left edge. Empty when none. */
async function overflow(p: Page): Promise<string[]> {
  return p.evaluate(
    (sel) => {
      const vp = document.querySelector(sel.viewport)!;
      const out: string[] = [];
      if (vp.scrollWidth > vp.clientWidth) {
        out.push(`scrollWidth ${vp.scrollWidth} > clientWidth ${vp.clientWidth}`);
      }
      if (vp.scrollLeft !== 0) out.push(`scrollLeft ${vp.scrollLeft}`);
      const cell = vp.querySelector('.slick-row .kira-cell-graph, .slick-row .l0');
      if (cell) {
        const x = Math.round(cell.getBoundingClientRect().left - vp.getBoundingClientRect().left);
        if (x < 0) out.push(`graph cell at x ${x}`);
      }
      return out;
    },
    { viewport: VIEWPORT },
  );
}

async function diagonalWheel(p: Page): Promise<void> {
  const box = (await p.locator(VIEWPORT).boundingBox())!;
  await p.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  for (let i = 0; i < 10; i++) await p.mouse.wheel(40, 60);
  await p.evaluate(
    () =>
      new Promise<void>((done) => requestAnimationFrame(() => requestAnimationFrame(() => done()))),
  );
}

test('columns resized wide never push the graph out of view', async ({ relaunch }) => {
  const { window: win } = await relaunch({ control: CONTROL });
  const errors: string[] = [];
  await openRepo(win, errors);
  await closeDetail(win);

  await drag(win, 'graph', 300);
  await drag(win, 'author', 300);
  await drag(win, 'date', 300);
  expect(await overflow(win)).toEqual([]);

  await handle(win, 'date').focus();
  for (let i = 0; i < 80; i++) await win.keyboard.press('ArrowRight');
  expect(await overflow(win)).toEqual([]);

  await diagonalWheel(win);
  expect(await overflow(win)).toEqual([]);
  expect(await clippedNodes(win)).toEqual([]);

  await scrollToRow(win, 3, PAGE_SIZE);
  await rowAt(win, 3).locator('.kira-cell-message').click();
  await win.keyboard.press('Escape');
  expect(await overflow(win)).toEqual([]);

  await win.setViewportSize({ width: 1000, height: 960 });
  await expect.poll(() => overflow(win)).toEqual([]);

  await loadMore(win);
  await diagonalWheel(win);
  expect(await overflow(win)).toEqual([]);
  expect(errors).toEqual([]);
});

test('a resized graph column keeps every node after Load more', async ({ relaunch }) => {
  const { window: win } = await relaunch({ control: CONTROL });
  const errors: string[] = [];
  await openRepo(win, errors);

  await drag(win, 'graph', -100);
  await scrollToRow(win, PAGE_SIZE - 1, PAGE_SIZE);
  expect(await clippedNodes(win)).toEqual([]);

  await loadMore(win);
  const rows = range(PAGE_SIZE, PAGING_ROWS, 30);
  expect(await sweep(win, rows, PAGING_ROWS, clippedNodes)).toEqual([]);
  expect(widestNode).toBeGreaterThan(60);

  await drag(win, 'graph', 20);
  expect(await sweep(win, rows, PAGING_ROWS, clippedNodes)).toEqual([]);
  expect(errors).toEqual([]);
});
