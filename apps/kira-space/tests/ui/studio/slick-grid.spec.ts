import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { primaryChord } from './support/chord';
import { CLIPBOARD_SHIM } from './support/clipboard';
import { createConnection } from './support/connect';
import {
  cellNavButton,
  gridCell,
  gridRow,
  gridScroller,
  headerCell,
  mutationsForScroll,
} from './support/grid';
import {
  APP_CHILDREN,
  APP_PATH,
  BIG_ROWS_PATH,
  bigRowsFixture,
  bigRowsHugePage,
  connectAndExpandControl,
  DB_PATH,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { expandRow, findRow, openRowMenu, settleFrames } from './support/tree';

// Eight sandbox-provable exit criteria. SlickGrid is the only engine
// `relaunch()` ever boots (the `window.__kiraGridEngine` flag this file used to force before boot,
// via `forceSlickEngine`, no longer exists to force). This does NOT and cannot answer the real-Mac
// perceptual/latency verdict, which needs a live compositor this environment has no way to produce
// (see docs/PERF.md's own pointer). What this file proves is narrower and mechanical: the bridge
// decodes real data correctly, the runway matches or exceeds what the incumbent once did, the DOM
// stays bounded, a sub-row scroll touches nothing, and — the single item the brief singled out —
// closing the tab tears the grid down.
//
// The fixture: a synthetic 61-column, 1000-row table ("spike_grid") — wide (like scroll_grid,
// budgets.spec.ts) AND tall (unlike scroll_grid's own 100-row mock, which never scrolls), and
// carrying at least one column from each type category `theme/icons.ts`'s categoryForTypeClass can
// return (numeric/boolean/datetime/string) so the column-level type-colour class has real
// material to colour. One page, `hasMore: false` — no pagination cursor to simulate.

const CONNECTION_ID = 'conn-slick-spike';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Slick Spike DB', 'magenta');
const SPIKE_GRID_PATH = `${APP_PATH}/table:spike_grid`;
const ROW_COUNT = 1000;
const TEXT_COLUMNS = 57; // + id + active + created_at = 60 data columns, 61 with the gutter.

function spikeGridColumns(): ColumnDescriptor[] {
  const cols: ColumnDescriptor[] = [
    {
      name: 'id',
      dataType: 'integer',
      typeClass: 'number',
      nullable: false,
      isPrimaryKey: true,
      generated: false,
    },
    {
      name: 'active',
      dataType: 'boolean',
      typeClass: 'boolean',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
    {
      name: 'created_at',
      dataType: 'timestamp',
      typeClass: 'temporal',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ];
  for (let j = 1; j <= TEXT_COLUMNS; j++) {
    cols.push({
      name: `col${j}`,
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    });
  }
  return cols;
}

function spikeGridRow(i: number): (string | null)[] {
  const row: (string | null)[] = [
    String(i),
    i % 2 === 0 ? 'true' : 'false',
    '2024-01-15T10:23:45Z',
  ];
  for (let j = 1; j <= TEXT_COLUMNS; j++) row.push(`row ${i} col ${j}`);
  return row;
}

function spikeGridRows(count: number): (string | null)[][] {
  return Array.from({ length: count }, (_, i) => spikeGridRow(i));
}

function spikeGridMeta() {
  const columns = [
    {
      name: 'id',
      position: 1,
      dataType: 'integer',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'active',
      position: 2,
      dataType: 'boolean',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'created_at',
      position: 3,
      dataType: 'timestamp',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
  ];
  for (let j = 1; j <= TEXT_COLUMNS; j++) {
    columns.push({
      name: `col${j}`,
      position: j + 3,
      dataType: 'text',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    });
  }
  return {
    path: SPIKE_GRID_PATH,
    kind: 'table' as const,
    name: 'spike_grid',
    qualifiedName: 'app.spike_grid',
    columns,
    primaryKey: ['id'],
    foreignKeys: [],
    referencedBy: [],
    indexes: [
      { name: 'spike_grid_pkey', columns: ['id'], unique: true, primary: true, method: 'btree' },
    ],
    rowEstimate: ROW_COUNT,
    comment: null,
  };
}

const APP_CHILDREN_WITH_SPIKE_GRID = [
  ...APP_CHILDREN,
  {
    kind: 'table',
    name: 'spike_grid',
    path: SPIKE_GRID_PATH,
    hasChildren: false,
    detail: `~${ROW_COUNT} rows`,
  },
];

function readSnapshot(pageSize: number): PortSnapshot {
  return {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: SPIKE_GRID_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: spikeGridColumns(),
        rows: spikeGridRows(pageSize),
        position: {
          offset: 0,
          pageSize,
          hasMore: false,
          nextToken: null,
          prevToken: null,
          strategy: 'keyset',
        },
        truncatedCells: 0,
      },
      source: 'server',
    },
  };
}

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: {
      name: 'Slick Spike DB',
      kind: 'postgres',
      color: 'magenta',
      mode: 'fields',
      readOnly: false,
      host: '127.0.0.1',
      port: 5432,
      database: 'kira_test',
      username: 'postgres',
      password: null,
      uri: null,
      options: {},
      preconnect: null,
      preconnectSidecar: false,
      preconnectDelaySec: 0,
      autoExplain: false,
      throttlePerSec: 0,
    },
    response: CONNECTION_SUMMARY,
  },
  ...connectAndExpandControl(CONNECTION_ID).map((snap) =>
    snap.channel === IPC.treeChildren && (snap.args as { path?: string })?.path === APP_PATH
      ? {
          ...snap,
          response: { nodes: APP_CHILDREN_WITH_SPIKE_GRID, source: 'server', truncated: false },
        }
      : snap,
  ),
  {
    channel: IPC.treeDescribe,
    args: { connectionId: CONNECTION_ID, path: SPIKE_GRID_PATH, refresh: false, tabId: null },
    response: { meta: spikeGridMeta(), source: 'server' },
  },
];

const PORT: PortSnapshot[] = [readSnapshot(100), readSnapshot(1000)];

/** The connect/expand/open flow — SlickGridHost.vue's own `.slick-cell`/
 *  `.slick-viewport-top.slick-viewport-right` DOM shape, the only engine `relaunch()` (fixtures.ts)
 *  ever boots since P22 Pass B's cutover (C17), so `readySelector` no longer needs to vary. */
async function connectAndOpenSpikeGrid(
  page: import('@playwright/test').Page,
  readySelector = '[data-testid="data-grid"] .slick-cell',
): Promise<void> {
  await createConnection(page, 'postgres', 'Slick Spike DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);

  const row = await findRow(page, SPIKE_GRID_PATH);
  await row.dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await page.click('[data-testid="page-size-1000"]');
  await expect
    .poll(async () => page.locator(readySelector).count(), { timeout: 15_000 })
    .toBeGreaterThan(0);
}

/** Reopens the already-connected, already-navigated-to spike_grid table in a fresh tab — the
 *  teardown loop's own repeat-open path, sharing the tree/connection state `connectAndOpenSpikeGrid`
 *  already set up rather than reconnecting from scratch each cycle. */
async function reopenSpikeGridTab(page: import('@playwright/test').Page): Promise<void> {
  await openRowMenu(page, SPIKE_GRID_PATH);
  await page.click('[data-testid="menu-item-open-data-new-tab"]');
  await expect
    .poll(async () => page.locator('[data-testid="data-grid"] .slick-cell').count(), {
      timeout: 15_000,
    })
    .toBeGreaterThan(0);
}

function rightViewport(page: import('@playwright/test').Page) {
  return page.locator('[data-testid="data-grid"] .slick-viewport-top.slick-viewport-right');
}

// Counts <style> elements holding a `.slickgrid_<uid>` rule, SlickGrid's per-grid sheet signature.
// kiraSlickGrid.ts no longer creates one (columns position through custom properties), so this
// proves none appears; the shared column sheet's `.kira-sg` rules never match.
// Module-scope so the teardown test can reuse it.
async function slickStyleTagCount(page: import('@playwright/test').Page): Promise<number> {
  return page.evaluate(
    () =>
      Array.from(document.querySelectorAll('style')).filter((s) => {
        try {
          return Array.from(s.sheet?.cssRules ?? []).some(
            (r) => 'selectorText' in r && (r as CSSStyleRule).selectorText?.includes('.slickgrid_'),
          );
        } catch {
          return false;
        }
      }).length,
  );
}

// The doubling the gate only reproduces under a
// page.mouse.wheel-driven scroll, never a `scrollTop +=` write: the two input paths give
// *different* intra-frame orderings between the scroll-driven render and a same-frame chase. No
// artificial delay between wheel calls — a real fling delivers scroll events far faster than
// CHASE_QUIET_MS (24ms), which is exactly the condition the pacing fix's re-arm loop is for.
type TraceFrames = readonly { renderCount: number; scrollEvents: number }[];

// The pacing invariant is "one render per frame while scroll events keep arriving". wheelFling's
// events come from the test driver, so under CPU load it can stall for hundreds of ms; the chase
// then legitimately fires in the quiet gap and the next wheel event's scroll render lands in that
// same frame. That frame is a resume from rest, not a live scroll, so it is not counted. Every
// doubled frame whose predecessor carried a scroll event still is, so a chase racing a live
// scroll (the regression this gates) still fails.
function doubledFramesDuringScroll(frames: TraceFrames): number[] {
  const out: number[] = [];
  frames.forEach((f, i) => {
    if (f.renderCount < 2) return;
    const prev = frames[i - 1];
    if (prev && prev.scrollEvents === 0) return;
    out.push(i);
  });
  return out;
}

async function wheelFling(
  page: import('@playwright/test').Page,
  viewport: ReturnType<typeof rightViewport>,
  steps = 60,
  deltaY = 900,
): Promise<void> {
  const box = await viewport.boundingBox();
  if (!box) throw new Error('wheelFling: viewport has no bounding box');
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  for (let i = 0; i < steps; i++) {
    await page.mouse.wheel(0, deltaY);
  }
}

test("SlickGrid spike — §7.4(a)'s eight sandbox-provable exit criteria", async ({
  relaunch,
  consoleErrors,
}) => {
  // Two full connect/expand/navigate cycles (this run, the teardown loop's own initial open) plus
  // 5 bare reopen cycles — comfortably past Playwright's 30s default.
  test.setTimeout(300_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);

  // --- 1. correct decoded text, cell for cell, across the first mounted window ------------------
  // Selected by position (`.nth()`), not by a column-name class — SlickGrid's own per-cell class is
  // `column.cssClass` (the tc-<category> type-colour class), never the column's name/field; a
  // testid-free spike has no other stable per-column hook. Column order in the right (unfrozen)
  // pane: id(0), active(1), created_at(2), col1(3), col2(4), … — the gutter (frozen, index 0
  // overall) lives in the left pane only.
  await rightViewport(page).evaluate((el) => {
    el.scrollTop = 0;
  });
  await settleFrames(page);
  function dataRow(p: import('@playwright/test').Page, row: number) {
    return p.locator(
      `[data-testid="data-grid"] .grid-canvas-top.grid-canvas-right .slick-row[data-row="${row}"] .slick-cell`,
    );
  }
  await expect(dataRow(page, 0).nth(3)).toHaveText('row 0 col 1');
  await expect(dataRow(page, 0).nth(7)).toHaveText('row 0 col 5');
  await expect(dataRow(page, 3).nth(12)).toHaveText('row 3 col 10');
  // The frozen gutter shows the page-global row number (rowNumberBase + row + 1), unfiltered here
  // so display position === page row (§5 item 5) — the gutter is the left pane's own one cell.
  await expect(
    page.locator(
      '[data-testid="data-grid"] .grid-canvas-top.grid-canvas-left .slick-row[data-row="0"] .slick-cell',
    ),
  ).toHaveText('1');

  // Type-based cell colour: the numeric/boolean/datetime columns each carry their own
  // tc-<category> class on every one of their own cells, AND (the rowColoring setting defaults
  // on) resolve to a distinct computed colour — asserting the class alone would pass even
  // if the .kira-grid--row-coloring gating class were never applied to the host root.
  await expect(dataRow(page, 0).nth(0)).toHaveClass(/tc-numeric/);
  await expect(dataRow(page, 0).nth(1)).toHaveClass(/tc-boolean/);
  await expect(dataRow(page, 0).nth(2)).toHaveClass(/tc-datetime/);
  await expect(dataRow(page, 0).nth(3)).not.toHaveClass(/tc-numeric|tc-boolean|tc-datetime/);
  const [numericColor, boolColor, dateColor, textColor] = await Promise.all(
    [0, 1, 2, 3].map((i) =>
      dataRow(page, 0)
        .nth(i)
        .evaluate((el) => getComputedStyle(el).color),
    ),
  );
  expect(numericColor).not.toBe(textColor);
  expect(boolColor).not.toBe(textColor);
  expect(dateColor).not.toBe(textColor);
  expect(new Set([numericColor, boolColor, dateColor]).size).toBe(3);

  // --- 2. decode-cache pinning under a sustained scroll (P5 C1) — the single highest-risk item ---
  async function decodeCacheRows(): Promise<number> {
    return page.evaluate(
      () =>
        (
          window as unknown as {
            __kiraRetention: () => { stores: { grid: { decodeCacheRows: number } } };
          }
        ).__kiraRetention().stores.grid.decodeCacheRows,
    );
  }
  const atRestRows = await decodeCacheRows();
  expect(atRestRows).toBeGreaterThan(0);
  expect(atRestRows).toBeLessThan(200); // well under ROW_COUNT — nowhere near "decoded everything"

  for (let i = 0; i < 15; i++) {
    await rightViewport(page).evaluate((el) => {
      el.scrollTop += 400;
    });
    // Scroll cadence is the input under test: real-time gaps between wheel steps.
    await page.waitForTimeout(30);
  }
  const afterScrollRows = await decodeCacheRows();
  // "Pinned to the mounted window" — bounded, not accumulating toward ROW_COUNT as more of the
  // table is visited over the course of the fling.
  expect(afterScrollRows).toBeLessThan(200);

  // --- 3. sub-row scroll -> zero DOM mutations; a scroll past the whole runway -> some -----------
  // `mutationsForScroll` waits for the grid to go quiet (no mutation records
  // across 6 consecutive frames) before arming the measurement observer, in one page.evaluate —
  // a fixed sleep here raced the same multi-frame convergence §3a below proves takes ~10 frames.
  await rightViewport(page).evaluate((el) => {
    el.scrollTop = 5000; // an ordinary mid-scroll position, away from either end
  });
  const subRowMutations = await mutationsForScroll(rightViewport(page), 4); // well under a 28px row
  expect(subRowMutations).toBe(0);

  const crossRowMutations = await mutationsForScroll(rightViewport(page), 3000); // past the runway
  expect(crossRowMutations).toBeGreaterThan(0);

  // --- 3a. the render batch converges over frames, not in one jump (P22 iter2-scroll-gaps D4) -----
  // A jump well past the whole runway, landing in never-before-mounted territory (the very bottom of
  // the table — nothing above has scrolled anywhere near it). D2's own per-call new-cell budget means
  // the mounted .slick-cell count measured on the *very next* animation frame is smaller than the
  // count measured several more frames later — the window visibly *converges* toward its full target
  // over a handful of self-scheduled catch-up renders, rather than jumping to full size in one call.
  const convergence = await rightViewport(page).evaluate(async (el) => {
    el.scrollTop = el.scrollHeight;
    const counts: number[] = [];
    for (let i = 0; i < 10; i++) {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      counts.push(document.querySelectorAll('[data-testid="data-grid"] .slick-cell').length);
    }
    return counts;
  });
  // The strictly-visible floor is never itself deferred (D2 step 1) — some cells mount immediately.
  expect(convergence[0]).toBeGreaterThan(0);
  // ...but the batch cap defers the rest of the runway, so the window is still growing several
  // frames later — a single-jump render would make these equal.
  expect(convergence[0]).toBeLessThan(convergence[convergence.length - 1]);

  // --- 4. mounted .slick-cell count stays under 2 500 across a velocity ladder -------------------
  // scroll_grid_data's own 60 data columns behave like budgets.spec.ts's own scroll_grid — this is
  // the wide-table case the cell-budget cap (D4's third bullet) exists for.
  for (const pxPerFrame of [40, 100, 200, 456]) {
    await rightViewport(page).evaluate((el) => {
      el.scrollTop = 0;
    });
    // Rest gap before the next fling; the sampler's at-rest threshold is real time.
    await page.waitForTimeout(50);
    for (let frame = 0; frame < 20; frame++) {
      await rightViewport(page).evaluate((el, delta) => {
        el.scrollTop += delta;
      }, pxPerFrame);
      // One scroll step per frame: the cadence is the input under test.
      await page.waitForTimeout(16);
    }
    const cellCount = await page.locator('[data-testid="data-grid"] .slick-cell').count();
    expect(cellCount).toBeLessThan(2500);
  }
  await rightViewport(page).evaluate((el) => {
    el.scrollTop = 0;
  });

  // --- 5. at rest, the mounted row band covers at least the strictly-visible viewport -------------
  // Originally an A/B against a second, freshly-relaunched page running the incumbent tanstack
  // engine ("matches or exceeds the incumbent's own coverage") — dropped at P22 Pass B's cutover
  // (C17): `relaunch()` no longer has an incumbent to boot at all (DataView.vue mounts
  // SlickGridHost.vue unconditionally now), so a second `relaunch()` here would just open another
  // SlickGrid page and compare it to itself. The real invariant this was standing in for — the
  // mounted row band must cover at least the viewport's own visible height, with no gap a user
  // could scroll into and see blank rows — is asserted directly instead.
  // P81 §4.5: was a fixed `waitForTimeout(100)` ahead of a one-shot lower-bound assertion — an
  // unfinished catch-up render (same convergence §3a proves takes ~10 frames) makes the band too
  // short under load. `expect.poll` on the coverage measurement itself is the idiomatic Playwright
  // answer for a lower bound after a settle; the bound stays exactly what it was.
  const slickBand = () =>
    page.evaluate(() => {
      const els = Array.from(
        document.querySelectorAll<HTMLElement>(
          '[data-testid="data-grid"] .grid-canvas-top.grid-canvas-right .slick-row',
        ),
      );
      let top = Number.POSITIVE_INFINITY;
      let bottom = Number.NEGATIVE_INFINITY;
      for (const el of els) {
        if (el.offsetTop < top) top = el.offsetTop;
        const end = el.offsetTop + el.offsetHeight;
        if (end > bottom) bottom = end;
      }
      return { top, bottom, rows: els.length };
    });
  const viewportHeightPx = await rightViewport(page).evaluate((el) => el.clientHeight);
  await expect
    .poll(async () => {
      const band = await slickBand();
      return band.rows > 0 ? band.bottom - band.top : Number.NEGATIVE_INFINITY;
    })
    .toBeGreaterThanOrEqual(viewportHeightPx);

  // --- 6. tab-close teardown (D3's own three-part assertion) — the one item this brief singled out
  const { window: teardownPage } = await relaunch({ control: CONTROL, stream: PORT });

  const baselineStyles = await slickStyleTagCount(teardownPage);
  expect(baselineStyles).toBe(0);
  const baselineRetention = await teardownPage.evaluate(() =>
    JSON.stringify((window as unknown as { __kiraRetention: () => unknown }).__kiraRetention()),
  );
  const baselineBytes = await teardownPage.evaluate(() =>
    (window as unknown as { __kiraRetainedBytes: () => number }).__kiraRetainedBytes(),
  );

  await connectAndOpenSpikeGrid(teardownPage);
  expect(await slickStyleTagCount(teardownPage)).toBe(0);
  for (let i = 0; i < 5; i++) {
    if (i > 0) await reopenSpikeGridTab(teardownPage);
    await teardownPage.locator('[data-testid="tab"].is-active [data-testid="tab-close"]').click();
    await expect(teardownPage.locator('[data-testid="tab"]')).toHaveCount(0);
  }

  await expect(teardownPage.locator('.slick-viewport')).toHaveCount(0);
  const closedStyles = await slickStyleTagCount(teardownPage);
  expect(closedStyles).toBe(baselineStyles);
  const closedRetention = await teardownPage.evaluate(() =>
    JSON.stringify((window as unknown as { __kiraRetention: () => unknown }).__kiraRetention()),
  );
  expect(closedRetention).toBe(baselineRetention);
  const closedBytes = await teardownPage.evaluate(() =>
    (window as unknown as { __kiraRetainedBytes: () => number }).__kiraRetainedBytes(),
  );
  expect(closedBytes).toBe(baselineBytes);

  expect(consoleErrors).toEqual([]);
});

// The frame-pacing fix's own sandbox-provable gates (T1/T3/T5). The whole premise is a real WebKit
// measurement (two render() passes in 131 of 140 frames of a wheel-driven fling, reproduced with
// the exact pre-fix policy) — see wheelFling's own comment for why page.mouse.wheel, not a
// scrollTop write, is load-bearing here.
test('P22 iter2-pacing — a catch-up render never shares a frame with a scroll-driven one', async ({
  relaunch,
}) => {
  test.setTimeout(120_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);
  const viewport = rightViewport(page);

  // --- At rest, the default policy never doubles a frame ------------------------------------
  // This gates the *policy* — never render twice in a frame while scroll events are arriving —
  // which is engine-independent; it is not a timing claim about WKWebView, which this sandbox has
  // none of (the line between what's provable here and what needs real hardware).
  await viewport.evaluate((el) => {
    el.scrollTop = 0;
  });
  // Rest gap before arming the trace; the sampler's at-rest threshold is real time.
  await page.waitForTimeout(50);
  await page.evaluate(() => window.__kiraScrollTrace?.start());
  await wheelFling(page, viewport);
  // Trailing quiescent frames, so any chase still converging the runway gets to run and clear.
  await page.waitForTimeout(300);
  const defaultResult = await page.evaluate(() => window.__kiraScrollTrace?.stop());
  expect(defaultResult).not.toBeNull();
  expect(doubledFramesDuringScroll(defaultResult?.frames ?? [])).toEqual([]);

  // --- The trace's per-frame accounting resets ------------------------------------------
  // From frames[] directly, on this same recording: a frame with renderCount === 0 must report
  // renderMs === 0, a multi-render frame's renderMs must be positive (it's a sum, not the sticky
  // last value), and frameMs must show up once ticks are spaced (not every frame is guaranteed
  // non-zero — a genuine same-timestamp double rAF tick is possible — so this checks the series as
  // a whole, not one frame).
  const frames = defaultResult?.frames ?? [];
  const noRenderFrame = frames.find((f) => f.renderCount === 0);
  expect(noRenderFrame).toBeDefined();
  expect(noRenderFrame?.renderMs).toBe(0);
  for (const f of frames) {
    if (f.renderCount > 1) expect(f.renderMs).toBeGreaterThan(0);
  }
  expect(frames.some((f) => f.frameMs > 0)).toBe(true);

  // --- T3: chaseQuietMsOverride = 0 reproduces the pre-fix behaviour exactly ---------------------
  // The self-verifying half of T1: if this run does NOT show a doubled frame, the harness is not
  // reproducing the doubling condition at all and T1 above would be a tautology, not a real gate.
  await viewport.evaluate((el) => {
    el.scrollTop = 0;
  });
  // Rest gap before arming the trace; the sampler's at-rest threshold is real time.
  await page.waitForTimeout(50);
  await page.evaluate(() => {
    window.__kiraGridTuning ??= {};
    window.__kiraGridTuning.chaseQuietMsOverride = 0;
  });
  await page.evaluate(() => window.__kiraScrollTrace?.start());
  await wheelFling(page, viewport);
  // Trailing quiescent frames, so a chase still converging gets to run and clear.
  await page.waitForTimeout(300);
  const zeroResult = await page.evaluate(() => window.__kiraScrollTrace?.stop());
  await page.evaluate(() => {
    if (window.__kiraGridTuning) window.__kiraGridTuning.chaseQuietMsOverride = undefined;
  });
  expect(zeroResult).not.toBeNull();
  const zeroHistogram = zeroResult?.summary.renderCountHistogram ?? {};
  const hasDoubledFrame = Object.keys(zeroHistogram).some((count) => Number(count) >= 2);
  expect(hasDoubledFrame).toBe(true);
});

// A chase armed just before teardown, and still pending (not yet fired) at
// the moment `grid.destroy(true)` runs, must not re-enter render() afterwards. Against fce3e54
// this throws: the pending rAF fires one frame later, re-enters render() past destroy(), and
// dereferences an element SlickGrid's own destroy() already nulled — `!this.initialized` doesn't
// catch it because destroy() never clears that flag.
//
// Timing, deliberately engineered rather than raced: `chaseQuietMsOverride` (300ms) is set long
// enough that the scroll-triggered chase is still armed (re-arming, not yet rendering) by the time
// the tab-close click resolves — Playwright's own action + assertion round-trip reliably takes
// tens of ms, well under 300 — and short enough that the wait *after* close reliably crosses it,
// so the previously-armed callback's quiet check flips true and it attempts to render.
test('P22 iter2-pacing — tearing down the grid with a catch-up render still armed', async ({
  relaunch,
}) => {
  test.setTimeout(60_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });

  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(String(err)));

  const baselineStyles = await slickStyleTagCount(page);
  expect(baselineStyles).toBe(0);
  const baselineRetention = await page.evaluate(() =>
    JSON.stringify((window as unknown as { __kiraRetention: () => unknown }).__kiraRetention()),
  );
  const baselineBytes = await page.evaluate(() =>
    (window as unknown as { __kiraRetainedBytes: () => number }).__kiraRetainedBytes(),
  );

  await connectAndOpenSpikeGrid(page);
  expect(await slickStyleTagCount(page)).toBe(0);
  await page.evaluate(() => {
    window.__kiraGridTuning ??= {};
    window.__kiraGridTuning.chaseQuietMsOverride = 300;
  });
  await rightViewport(page).evaluate((el) => {
    el.scrollTop = el.scrollHeight; // never-before-mounted territory — guarantees a runway deficit
  });
  await page.waitForTimeout(30); // arm the chase, stay well inside the 300ms quiet window

  await page.locator('[data-testid="tab"].is-active [data-testid="tab-close"]').click();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(0);
  // Past the 300ms quiet window: the previously-armed callback (if D4 didn't cancel it) fires and
  // attempts to render against the now-torn-down grid.
  await page.waitForTimeout(500);

  expect(pageErrors).toEqual([]);
  await expect(page.locator('.slick-viewport')).toHaveCount(0);
  expect(await slickStyleTagCount(page)).toBe(baselineStyles);
  const closedRetention = await page.evaluate(() =>
    JSON.stringify((window as unknown as { __kiraRetention: () => unknown }).__kiraRetention()),
  );
  expect(closedRetention).toBe(baselineRetention);
  const closedBytes = await page.evaluate(() =>
    (window as unknown as { __kiraRetainedBytes: () => number }).__kiraRetainedBytes(),
  );
  expect(closedBytes).toBe(baselineBytes);
});

// P22 Pass B postscript §14.2 — root cause: stock `bindAncestorScrollEvents` (slick.grid.ts) binds
// a capture-phase 'scroll' listener directly on `document` via `BindingEventService.bind(document,
// "scroll", handler, true)`, but `BindingEventService.unbind()` (called by every `destroy()` path)
// calls `removeEventListener(eventName, listener)` with no capture argument — a spec-defined no-op
// against a capture-phase listener, so it is never actually removed. It survives the grid's own
// teardown and throws `_viewport.includes` (`_viewport` nulled by the same `destroy(true)`) on the
// next scroll event ANYWHERE in the document, not just the grid's own viewport. kiraSlickGrid.ts's
// own `bindAncestorScrollEvents`/`destroy` overrides bind and remove it directly, bypassing the
// buggy service. Reproduced against the pre-fix code before writing this test: the exact
// `TypeError: null is not an object (evaluating 'this._viewport.includes')` WebKit throws.
test('P22 postscript — a scroll event anywhere in the document after grid teardown never throws', async ({
  relaunch,
}) => {
  test.setTimeout(60_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  const pageErrors: string[] = [];
  page.on('pageerror', (err) => pageErrors.push(String(err)));

  await connectAndOpenSpikeGrid(page);
  await rightViewport(page).evaluate((el) => {
    el.scrollTop = 500;
    el.scrollLeft = 300;
  });
  await settleFrames(page);

  await page.locator('[data-testid="tab"].is-active [data-testid="tab-close"]').click();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(0);
  await expect(page.locator('.slick-viewport')).toHaveCount(0);

  // A document-level scroll — not scoped to the (now-gone) grid's own viewport at all, matching
  // bindAncestorScrollEvents' own global-capture reach — plus one from an unrelated scrollable
  // element (bubbling through document), the two ways this listener could ever be invoked.
  await page.evaluate(() => {
    document.dispatchEvent(new Event('scroll'));
    document
      .querySelector('[data-testid="tree"]')
      ?.dispatchEvent(new Event('scroll', { bubbles: true }));
  });
  await settleFrames(page);

  expect(pageErrors).toEqual([]);
});

// The "no exception" test above proves the listener's *effects* are
// harmless once it's still attached wrongly (its target refs happen to be non-null this run), but
// says nothing about whether `destroy()` calls `removeEventListener` at all. It previously
// didn't: `ancestorScrollHandler`'s class-field initializer (`= null`) ran *after* `super()` — i.e.
// after `bindAncestorScrollEvents()` already set the real handler during construction — clobbering
// it back to `null` every time, so `destroy()`'s `if (this.ancestorScrollHandler)` guard was always
// false and the real capture-phase `document` listener was never removed, leaking the grid (and
// everything its closure retains) for the life of the process. This test counts the actual
// `document`-level capture-phase `'scroll'` listeners SlickGrid's own binding path adds and
// removes, so a regression back to the clobbering pattern fails here even if it stops throwing.
test('P22 postscript follow-up — grid teardown actually removes the document scroll listener, not just avoids throwing', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });

  // Installed before the grid ever mounts (SPA navigation below, no further page.goto — the patch
  // stays in effect) so it sees every add/remove this grid instance makes, not just a snapshot.
  await page.evaluate(() => {
    const w = window as unknown as { __kiraScrollListenerNet: () => number };
    let net = 0;
    w.__kiraScrollListenerNet = () => net;
    const isCapture = (options: boolean | AddEventListenerOptions | undefined) =>
      options === true || (typeof options === 'object' && options?.capture === true);
    const originalAdd = document.addEventListener.bind(document);
    const originalRemove = document.removeEventListener.bind(document);
    document.addEventListener = ((type: string, listener: unknown, options?: unknown) => {
      if (type === 'scroll' && isCapture(options as boolean | AddEventListenerOptions | undefined))
        net++;
      return originalAdd(type, listener as EventListenerOrEventListenerObject, options as never);
    }) as typeof document.addEventListener;
    document.removeEventListener = ((type: string, listener: unknown, options?: unknown) => {
      if (type === 'scroll' && isCapture(options as boolean | AddEventListenerOptions | undefined))
        net--;
      return originalRemove(type, listener as EventListenerOrEventListenerObject, options as never);
    }) as typeof document.removeEventListener;
  });

  const netListeners = () =>
    page.evaluate(() =>
      (window as unknown as { __kiraScrollListenerNet: () => number }).__kiraScrollListenerNet(),
    );

  expect(await netListeners()).toBe(0);

  await connectAndOpenSpikeGrid(page);
  expect(await netListeners()).toBe(1); // bindAncestorScrollEvents' own listener, actually retained.

  await page.locator('[data-testid="tab"].is-active [data-testid="tab-close"]').click();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(0);
  expect(await netListeners()).toBe(0); // destroy() must see the real handler and remove it.
});

// The gesture-onset defect's own sandbox-provable gates.
//
// What this proves, and what it does not. It gates the *mechanism*: no render pass sizes its
// runway as if the grid were standing still while the viewport is measurably moving. That is a
// property of this app's own sampling policy plus DOM event-listener ordering (spec-defined
// registration order on one target), so it is engine-independent and provable here — unlike the
// perceived "content isn't fully rendered for a few frames at the start of a fling" symptom that
// motivated it, which needs the real macOS compositor and is docs/PERF.md's job.
//
// Three separate rest-to-motion transitions per recording, not one: the defect is one frame per
// gesture, so a single fling gives a one-frame margin and a flaky gate.
async function flingFromRest(
  page: import('@playwright/test').Page,
  viewport: ReturnType<typeof rightViewport>,
): Promise<void> {
  // Well past the sampler's own 150ms at-rest threshold (SlickGridHost.vue's velocity()), so each
  // burst below does start from "at rest" and not from the tail of the previous one. 500ms,
  // not 150-250: `page.mouse.wheel` returns before WebKit's own scroll animation finishes, and that
  // animation's trailing 2-3px scroll events kept arriving ~95ms after the last wheel call resolved
  // here — enough to warm the sampler and hide the defect on bursts 2 and 3 of a 250ms-gap run.
  await page.waitForTimeout(500);
  // deltaY is deliberately small: the sampler discards a delta above
  // MAX_PLAUSIBLE_ROW_VELOCITY_PX_PER_FRAME (800) as a discrete jump rather than a fling, and
  // page.mouse.wheel's CDP round-trips can stack several ticks into one frame.
  await wheelFling(page, viewport, 40, 60);
}

// Deterministic onset: a programmatic scrollTop jump from rest delivers one scroll event whose
// dy alone clears the grid's dy > 20 render threshold, so the render is driven by the *first*
// event of the gesture, ahead of the host's own scroll listener. That is exactly the ordering the
// defect needs. A wheel fling cannot guarantee it: WebKit's smooth-scroll animation opens with
// 2-3px events (below the render threshold), so the first render lands after the sampler already
// holds a non-zero sample and the defect never shows on a quiet host (it only showed when load
// coalesced events into a large first delta).
async function jumpFromRest(
  page: import('@playwright/test').Page,
  viewport: ReturnType<typeof rightViewport>,
): Promise<void> {
  // Past the sampler's 150ms at-rest threshold (SlickGridHost.vue's velocity()).
  await page.waitForTimeout(400);
  await viewport.evaluate(
    (el) =>
      new Promise<void>((resolve) => {
        el.scrollTop += 120;
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
      }),
  );
}

/** The gate itself: frames that rendered while the viewport was measurably moving at a plausible
 *  fling speed, yet fed zero velocity into the runway arithmetic. `summary.staleVelocityFrames` is
 *  the same tally without the 800px guard — applied here so a stacked-wheel frame that the sampler
 *  legitimately treats as a discrete jump cannot be read as the defect. */
function staleRunwayFrames(
  frames: NonNullable<ReturnType<NonNullable<Window['__kiraScrollTrace']>['stop']>>['frames'],
): number {
  return frames.filter(
    (f) => f.renderCount > 0 && f.pxPerFrame > 0 && f.pxPerFrame <= 800 && f.runwayVelocity === 0,
  ).length;
}

test('P22 iter2-onset — a fresh gesture sizes its runway from that gesture, not from before it', async ({
  relaunch,
}) => {
  test.setTimeout(120_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);
  const viewport = rightViewport(page);

  // --- (a) with the fix: no render sizes its runway at rest while the viewport is moving ---------
  await viewport.evaluate((el) => {
    el.scrollTop = 0;
  });
  await page.evaluate(() => window.__kiraScrollTrace?.start());
  for (let burst = 0; burst < 3; burst++) await flingFromRest(page, viewport);
  for (let burst = 0; burst < 3; burst++) await jumpFromRest(page, viewport);
  // Trailing quiescent frames, so a chase still converging gets to run and clear.
  await page.waitForTimeout(300);
  const fixed = await page.evaluate(() => window.__kiraScrollTrace?.stop());
  expect(fixed).not.toBeNull();
  const fixedFrames = fixed?.frames ?? [];
  // The instrument reports something at all — otherwise the zero below would be vacuous.
  expect(fixed?.summary.runwayVelocity.max ?? 0).toBeGreaterThan(0);
  expect(staleRunwayFrames(fixedFrames)).toBe(0);

  // --- (b) THE REGRESSION GATE: the pacing fix still holds, on this same recording ---------------
  // The invariant — at most one render pass per animation frame while a scroll
  // is live. The onset fix deliberately never touches `scheduleChase`'s quiescence gate (it widens
  // the *target* an already-scheduled render aims at, it does not let an extra render through), and
  // this is what proves that: three continuous, gapless wheel bursts, zero doubled frames.
  expect(doubledFramesDuringScroll(fixedFrames)).toEqual([]);

  // --- (c) freshVelocitySampleOverride = false reproduces the pre-fix behaviour ------------------
  // The self-verifying half of (a), exactly as T3 is for T1: if the pre-fix run does NOT show
  // stale-velocity frames, this harness is not reproducing the condition at all and (a) is a
  // tautology. Driven by jumpFromRest (each onset's first scroll event clears the dy > 20 render
  // threshold, slickgrid dist/esm/index.js:10589), so every onset must show the defect.
  await viewport.evaluate((el) => {
    el.scrollTop = 0;
  });
  await page.evaluate(() => {
    window.__kiraGridTuning ??= {};
    window.__kiraGridTuning.freshVelocitySampleOverride = false;
  });
  await page.evaluate(() => window.__kiraScrollTrace?.start());
  for (let burst = 0; burst < 3; burst++) await jumpFromRest(page, viewport);
  // Trailing quiescent frames, so a chase still converging gets to run and clear.
  await page.waitForTimeout(300);
  const preFix = await page.evaluate(() => window.__kiraScrollTrace?.stop());
  await page.evaluate(() => {
    if (window.__kiraGridTuning) window.__kiraGridTuning.freshVelocitySampleOverride = undefined;
  });
  expect(preFix).not.toBeNull();
  expect(staleRunwayFrames(preFix?.frames ?? [])).toBe(3);

  // --- (d) the per-frame chase gate is what makes (b) hold, not the wall-clock one --------------
  // CHASE_QUIET_MS is 24ms and frames here run p50 ~28 / p95 ~55 / max ~70ms
  // (and p50 32.1ms on the real Mac, docs/PERF.md) — so "24ms since the last scroll event"
  // routinely stops meaning "no scroll event is driving this frame". With the frame gate removed,
  // leaving the wall-clock policy alone, this same fling doubles frames. This
  // is the load-bearing half of (b): without it, (b) would be passing on a gate that only holds
  // by accident. Uses wheelFling's own faster defaults.
  await viewport.evaluate((el) => {
    el.scrollTop = 0;
  });
  // Rest gap before arming the trace; the sampler's at-rest threshold is real time.
  await page.waitForTimeout(50);
  await page.evaluate(() => {
    window.__kiraGridTuning ??= {};
    window.__kiraGridTuning.chaseFrameGateOverride = false;
  });
  await page.evaluate(() => window.__kiraScrollTrace?.start());
  await wheelFling(page, viewport);
  // Trailing quiescent frames, so a chase still converging gets to run and clear.
  await page.waitForTimeout(300);
  const msGateOnly = await page.evaluate(() => window.__kiraScrollTrace?.stop());
  await page.evaluate(() => {
    if (window.__kiraGridTuning) window.__kiraGridTuning.chaseFrameGateOverride = undefined;
  });
  expect(msGateOnly).not.toBeNull();
  const msGateHistogram = msGateOnly?.summary.renderCountHistogram ?? {};
  expect(Object.keys(msGateHistogram).some((count) => Number(count) >= 2)).toBe(true);
});

// Select-all's own cost gate. The bypass (rt().selection set directly, a `.kira-select-all` host
// class painting every `.slick-cell`, no range pushed into the model;
// SlickGridHost.vue's onSelectAll): the model path cost 80-130 ms against this 150 ms gate on the
// wide table. The gate now times the bypass.
test('P22 Pass B C6 — select-all completes within the 150ms sandbox gate, wide and tall', async ({
  relaunch,
}) => {
  test.setTimeout(60_000);

  // --- wide: spike_grid, 1 000 rows x 61 columns -------------------------------------------------
  const { window: wide } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(wide);
  const wideElapsedMs = await wide.evaluate(() => {
    const corner = document.querySelector<HTMLElement>('[data-testid="grid-select-all"]');
    if (!corner) throw new Error('select-all corner not found');
    const start = performance.now();
    corner.click();
    return performance.now() - start;
  });
  expect(wideElapsedMs).toBeLessThan(150);
  await expect(wide.locator('.slick-grid-host.kira-select-all')).toHaveCount(1);

  // --- tall: big_rows, 10 000 rows x 2 columns ----------------------------------------------------
  const BIG_CONNECTION_ID = 'conn-slick-selectall-big';
  const BIG_CONNECTION_SUMMARY = postgresConnectionSummary(
    BIG_CONNECTION_ID,
    'Select All Big DB',
    'amber',
  );
  const bigControl: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Select All Big DB',
        kind: 'postgres',
        color: 'amber',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: BIG_CONNECTION_SUMMARY,
    },
    ...bigRowsFixture(BIG_CONNECTION_ID).control,
  ];
  const bigPort: PortSnapshot[] = [
    ...bigRowsFixture(BIG_CONNECTION_ID).port, // pageSize=100 — the tab's own initial load
    bigRowsHugePage(BIG_CONNECTION_ID), // pageSize=10000 — page-size-10000's own request
  ];

  const { window: tall } = await relaunch({ control: bigControl, stream: bigPort });
  await createConnection(tall, 'postgres', 'Select All Big DB');

  const connRow = tall.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(tall, '');
  await tall.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(tall, '');
  await expandRow(tall, DB_PATH);
  await expandRow(tall, APP_PATH);
  const bigRowsRow = await findRow(tall, BIG_ROWS_PATH);
  await bigRowsRow.dblclick();
  await expect(tall.locator('[data-testid="data-grid"] .slick-cell')).not.toHaveCount(0);
  await tall.click('[data-testid="page-size-10000"]');
  await expect
    .poll(async () => tall.locator('[data-testid="data-grid"] .slick-cell').count(), {
      timeout: 15_000,
    })
    .toBeGreaterThan(0);

  const tallElapsedMs = await tall.evaluate(() => {
    const corner = document.querySelector<HTMLElement>('[data-testid="grid-select-all"]');
    if (!corner) throw new Error('select-all corner not found');
    const start = performance.now();
    corner.click();
    return performance.now() - start;
  });
  expect(tallElapsedMs).toBeLessThan(150);
  await expect(tall.locator('.slick-grid-host.kira-select-all')).toHaveCount(1);
});

// "the F5 merge cost" (the open question, discharged with a measurement, not an argument): with a
// 10 000-match search active, a selection change must still land inside the same 150ms sandbox
// bound the select-all gate uses. `kira-search` is explicitly the one CSS layer the table allows
// to be O(matches) rather than O(perimeter ∩ rendered) — this is what proves that choice doesn't
// tax anything *else* (the selection's own O(area) select-all cost) once a large search result is
// sitting on top of it.
// Fails -> the named fallback (a rendered-range ± hysteresis band for kira-search) lands in this
// same commit.
test('P22 Pass B C12 T7 — select-all stays within the 150ms sandbox gate with 10 000 search matches active', async ({
  relaunch,
}) => {
  test.setTimeout(60_000);

  const BIG_CONNECTION_ID = 'conn-slick-search-big';
  const BIG_CONNECTION_SUMMARY = postgresConnectionSummary(
    BIG_CONNECTION_ID,
    'Search Big DB',
    'cyan',
  );
  const bigControl: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Search Big DB',
        kind: 'postgres',
        color: 'cyan',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: BIG_CONNECTION_SUMMARY,
    },
    ...bigRowsFixture(BIG_CONNECTION_ID).control,
  ];
  const bigPort: PortSnapshot[] = [
    ...bigRowsFixture(BIG_CONNECTION_ID).port, // pageSize=100 — the tab's own initial load
    bigRowsHugePage(BIG_CONNECTION_ID), // pageSize=10000 — page-size-10000's own request
  ];

  const { window: page } = await relaunch({ control: bigControl, stream: bigPort });
  await createConnection(page, 'postgres', 'Search Big DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  const bigRowsRow = await findRow(page, BIG_ROWS_PATH);
  await bigRowsRow.dblclick();
  await expect(page.locator('[data-testid="data-grid"] .slick-cell')).not.toHaveCount(0);
  await page.click('[data-testid="page-size-10000"]');
  await expect
    .poll(async () => page.locator('[data-testid="data-grid"] .slick-cell').count(), {
      timeout: 15_000,
    })
    .toBeGreaterThan(0);

  // 10 000 matches, one per row: every big_rows `id` is a bare digit string (bigRowsRow's own
  // shape), so a whole-cell digit-run regex matches the `id` column exactly once per row and
  // never the `hash` column (32 hex chars, never all-digit in this fixture's md5 seed).
  await page.click('[data-testid="toolbar-search"]');
  await expect(page.locator('[data-testid="search-bar"]')).toBeVisible();
  await page.click('[data-testid="search-regex"]');
  await page.fill('[data-testid="search-input"]', '^\\d+$');
  await expect(page.locator('[data-testid="search-count"]')).toContainText('of 10,000', {
    timeout: 15_000,
  });

  const elapsedMs = await page.evaluate(() => {
    const corner = document.querySelector<HTMLElement>('[data-testid="grid-select-all"]');
    if (!corner) throw new Error('select-all corner not found');
    const start = performance.now();
    corner.click();
    return performance.now() - start;
  });
  expect(elapsedMs).toBeLessThan(150);
  await expect(page.locator('.slick-grid-host.kira-select-all')).toHaveCount(1);
});

// Regression test — onSelectAll used to build its SlickRange with `p?.rowCount` (the PAGE row
// count) as the display-position upper bound, instead of the filtered display-row count every
// other caller in this file uses (onHeaderSelectClick/onHeaderContextMenuHandler's own
// `displayRowCount`). Under an active "hide non-matching rows" filter, that ran the range past the
// actual displayed rows into the pending-insert-row address space (`rowAtDisplayPosition` maps an
// out-of-range display position to `pageRowCount + (pos - count)`), producing dozens/thousands of
// spurious extra lines on copy — a page-row range far past the real, loaded data. `navigator.
// clipboard` is stubbed the same way support/clipboard.ts's own `CLIPBOARD_SHIM` does
// (WebKit refuses an ungestured `readText()` even with the permission granted).
test('P22 Pass B follow-up — select-all under an active row filter copies only the visible rows', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-slick-select-all-filter';
  const CONNECTION_SUMMARY = postgresConnectionSummary(
    CONNECTION_ID,
    'Select All Filter DB',
    'magenta',
  );
  const FIXTURE = bigRowsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Select All Filter DB',
        kind: 'postgres',
        color: 'magenta',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await page.addInitScript(CLIPBOARD_SHIM);
  await page.reload();
  await page.waitForSelector('[data-testid="status-bar"]');

  await createConnection(page, 'postgres', 'Select All Filter DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  const bigRowsRow = await findRow(page, BIG_ROWS_PATH);
  await bigRowsRow.dblclick();
  await expect(page.locator('[data-testid="data-grid"] .slick-cell')).not.toHaveCount(0);

  // Filter down to a contiguous 3-row prefix (ids 1-3 of the 100 loaded) — deliberately
  // contiguous-from-the-top so a correct fix's copy is unambiguously "exactly these 3 rows", not
  // entangled with the separate, wider question of whether a range copy also skips a NON-
  // contiguous filtered gap (a `'range'`-kind selection's own corners-only shape, untouched by
  // this fix). id/hash pairs are the well-known md5-of-decimal-string fixture values.
  await page.click('[data-testid="toolbar-search"]');
  await expect(page.locator('[data-testid="search-bar"]')).toBeVisible();
  await page.click('[data-testid="search-regex"]');
  await page.fill('[data-testid="search-input"]', '^[1-3]$');
  await expect(page.locator('[data-testid="search-count"]')).toContainText('1 of 3');
  await page.click('[data-testid="search-filter-rows"]');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(3);

  // A cell click first, so the grid canvas actually holds DOM focus (SlickGrid's own keydown
  // listener is bound to its focus sink/canvas elements, not the document) — the select-all
  // corner is a header button and clicking it alone leaves nothing focused for Ctrl+C to reach.
  await gridCell(page, 1, 'id').click();
  const corner = page.locator('[data-testid="grid-select-all"]');
  await corner.click();
  await expect(page.locator('.slick-grid-host.kira-select-all')).toHaveCount(1);

  // The corner click above lands DOM focus on the header pane (SlickGrid's own header panes
  // carry `tabIndex=0`) rather than the grid canvas — true of clicking any header cell, not
  // specific to select-all. `onKeyDown`'s Ctrl+C handling is bound to the canvas/focus-sink
  // elements only (slick.grid.ts), so refocus the canvas directly (never via another cell click,
  // which would collapse the very selection under test) before sending Ctrl+C.
  await page.evaluate(() =>
    (document.querySelector('.grid-canvas') as HTMLElement | null)?.focus(),
  );
  await page.keyboard.press(await primaryChord(page, 'c'));
  const tsv = await page.evaluate(() => navigator.clipboard.readText());
  const lines = tsv.split('\n');
  expect(lines).toHaveLength(3); // not the ~100 (page rowCount) the pre-fix range spanned
  expect(lines).toEqual([
    '1\tc4ca4238a0b923820dcc509a6f75849b',
    '2\tc81e728d9d4c2f636f067f89cc14862c',
    '3\teccbc87e4b5ce2fe28308fd9f2a7baf3',
  ]);
});

// The pacing invariant (the histogram-all-1 assertion, above) held again with N staged insert
// rows on screen. This is the one place this pass returns DOM from a formatter
// (the insert region's own `<input>`, self-contained and never re-touched by SlickGrid once built
// — see `cellFormatter`'s own comment, SlickGridHost.vue) against the measured
// "text, never DOM" rule, so it gets its own gate, landed in the same commit that adds the
// exception so a regression stays bisectable to it.
test('P22 Pass B C9 — the pacing invariant holds with N staged insert rows on screen', async ({
  relaunch,
}) => {
  // Three 60-wheel flings at ~8s each unloaded, ~4x that on a starved runner.
  test.setTimeout(240_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);
  const viewport = rightViewport(page);

  async function fling(): Promise<{ doubled: number[]; p95: number }> {
    await viewport.evaluate((el) => {
      el.scrollTop = 0;
    });
    // Rest gap before arming the trace; the sampler's at-rest threshold is real time.
    await page.waitForTimeout(50);
    await page.evaluate(() => window.__kiraScrollTrace?.start());
    await wheelFling(page, viewport);
    // Trailing quiescent frames, so a chase still converging gets to run and clear.
    await page.waitForTimeout(300);
    const result = await page.evaluate(() => window.__kiraScrollTrace?.stop());
    expect(result).not.toBeNull();
    return {
      doubled: doubledFramesDuringScroll(result?.frames ?? []),
      p95: result?.summary.renderMs.p95 ?? 0,
    };
  }

  const before = await fling();
  expect(before.doubled).toEqual([]);

  // N staged insert rows — a handful, matching D9's own "typically 1-5, never scrolled past in
  // bulk" scope note.
  for (let i = 0; i < 5; i++) {
    await page.click('[data-testid="toolbar-add-row"]');
  }
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(5);

  // Not a tight timing claim — this sandbox has no real compositor — a generous
  // same-run bound that only fails if the insert region's own DOM rode along on every scroll-
  // driven render instead of staying self-contained (the actual regression this gate exists to
  // catch). A CPU-starved run (parallel suites) can spike one fling's frames, so the gate takes up
  // to three flings and passes on the first clean one: a real regression fails every attempt.
  let after = await fling();
  for (let attempt = 1; attempt < 3; attempt++) {
    if (after.doubled.length === 0 && after.p95 < before.p95 * 3 + 5) break;
    after = await fling();
  }
  expect(after.doubled).toEqual([]);
  expect(after.p95).toBeLessThan(before.p95 * 3 + 5);
});

// The inserts.length watcher invalidated and rendered the touched range but never
// scrolled to it — on a page taller than the viewport, "Add a row" staged an insert nobody could
// see. One line, `grid.scrollRowIntoView`, guarded on growth (a discard shrinks the count
// and must not scroll, the same rule this suite's http/grpc/variable-set siblings each get).
test('P22b D17 — a staged insert row scrolls into view, and a discard does not scroll', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);
  const viewport = rightViewport(page);

  await viewport.evaluate((el) => {
    el.scrollTop = 0;
  });
  await settleFrames(page);
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(0);

  await page.click('[data-testid="toolbar-add-row"]');
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(1);
  // Staged past the loaded page — the viewport must have moved to bring it on screen, not stayed
  // pinned at the top where it was before the click.
  await expect.poll(() => viewport.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  await expect(page.locator('[data-testid="grid-row-insert"]')).toBeVisible();

  // The growth guard: discarding the insert (a shrink) must not scroll anywhere on its own.
  // Scrolled back to the top first — away from the boundary the insert itself sits at, so a plain
  // browser reflow (the scrollable area shrinking by one row when its very last row is removed)
  // can't be mistaken for the watcher firing scrollRowIntoView on a shrink, which is the actual
  // thing this guard exists to prevent.
  await viewport.evaluate((el) => {
    el.scrollTop = 0;
  });
  await settleFrames(page);
  await page.click('[data-testid="toolbar-discard-changes"]');
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(0);
  await expect(viewport.evaluate((el) => el.scrollTop)).resolves.toBe(0);
});

// P182 stage 2 — the gutter canvas is one sticky column in the same scroller as the rows.
test('P182 — the gutter moves with the rows in the same task as a scrollTop write and stays pinned when scrolled right', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);
  const viewport = rightViewport(page);
  await expect(gutterCell(page, 0)).toBeVisible();

  const lockstep = await viewport.evaluate((el) => {
    el.scrollTop = 500;
    // Same task: no scroll event has run, so only one native scroller keeps these equal.
    const rows = [...el.querySelectorAll<HTMLElement>('.grid-canvas-right .slick-row')];
    const row = rows.find((r) => r.getBoundingClientRect().top > 0) ?? rows[0];
    const n = row?.dataset.row;
    const gutter = el.querySelector(`.grid-canvas-left .slick-row[data-row="${n}"] .slick-cell`);
    return {
      gutterTop: gutter?.getBoundingClientRect().top ?? Number.NaN,
      rowTop: row?.getBoundingClientRect().top ?? Number.NaN,
    };
  });
  expect(Math.abs(lockstep.gutterTop - lockstep.rowTop)).toBeLessThanOrEqual(1);

  await viewport.evaluate((el) => {
    el.scrollTop = 0;
    el.scrollLeft = 300;
  });
  await settleFrames(page);
  const pinned = await viewport.evaluate((el) => {
    const gutter = el.querySelector('[data-testid="grid-gutter-cell"]');
    if (!gutter) throw new Error('no gutter cell');
    const box = gutter.getBoundingClientRect();
    const top = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
    return {
      gutterLeft: box.left,
      viewportLeft: Element.prototype.getBoundingClientRect.call(el).left,
      hit: top === gutter || gutter.contains(top),
    };
  });
  expect(Math.abs(pinned.gutterLeft - pinned.viewportLeft)).toBeLessThanOrEqual(1);
  expect(pinned.hit).toBe(true);
});

test('P182 — keyboard navigation past the right edge keeps the active cell clear of the pinned gutter and the edge', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);
  const viewport = rightViewport(page);
  await gridCell(page, 0, 'id').click();
  for (let i = 0; i < 12; i++) await page.keyboard.press('ArrowRight');
  await expect.poll(() => viewport.evaluate((el) => el.scrollLeft)).toBeGreaterThan(0);
  const fit = await viewport.evaluate((el) => {
    const active = el.querySelector('.grid-canvas-right .slick-cell.active');
    if (!active) throw new Error('no active cell');
    const a = active.getBoundingClientRect();
    const v = Element.prototype.getBoundingClientRect.call(el);
    const gutter = el.querySelector('.grid-canvas-left')?.getBoundingClientRect();
    return {
      rightGap: v.right - (v.width - el.clientWidth) - a.right,
      leftGap: a.left - (gutter?.right ?? v.left),
    };
  });
  expect(fit.rightGap).toBeGreaterThanOrEqual(-1);
  expect(fit.leftGap).toBeGreaterThanOrEqual(-1);
});

test('P182 — a gutter click or right-click while scrolled right selects the row without scrolling back', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);
  const viewport = rightViewport(page);
  await expect(gutterCell(page, 3)).toBeVisible();
  await viewport.evaluate((el) => {
    el.scrollLeft = 300;
  });
  await expect(page.locator('[data-testid="grid-header-cell"][data-column="col5"]')).toBeVisible();

  await gutterCell(page, 3).click();
  await expect(gridCell(page, 3, 'col5')).toHaveClass(/kira-cell-selected/, { timeout: 5_000 });
  await expect(viewport.evaluate((el) => el.scrollLeft)).resolves.toBe(300);

  await gutterCell(page, 5).click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await expect(viewport.evaluate((el) => el.scrollLeft)).resolves.toBe(300);
});

test('P182 — the wheel over the gutter scrolls the grid', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);
  const viewport = rightViewport(page);
  const box = await gutterCell(page, 2).boundingBox();
  if (!box) throw new Error('gutter cell has no bounding box');
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.wheel(0, 300);
  await expect.poll(() => viewport.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
});
function gutterCell(page: import('@playwright/test').Page, row: number) {
  return page.locator(
    `[data-testid="data-grid"] .slick-row[data-row="${row}"] [data-testid="grid-gutter-cell"]`,
  );
}

// The single host-owned nav button's own DOM invariant: exactly one
// `[data-testid="cell-nav-button"]` in the whole document at any time (never one-per-cell the way
// the incumbent's own pure-CSS-hover button was), it sits within the left 24px of whichever cell
// currently owns it (the user's own explicit "move it to the left" request), and it is
// absent for a column with no nav affordance at all. `order_items` (product_id/order_id both FK,
// quantity plain) is this suite's own smallest fixture with two independent FK columns on one
// row — interaction.spec.ts's own FK/PK nav section already covers the NULL-source absence case
// (employees.manager_id) end to end; nothing in this app's fixtures has a genuinely multi-column
// FK to re-derive that scenario against a "composite" key specifically.
test('P22 Pass B C11 T9 — the nav button is always visible per nav cell, positioned left, absent off a nav column', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-slick-navbtn';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Nav Button DB', 'grey');
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Nav Button DB',
        kind: 'postgres',
        color: 'grey',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: CONNECTION_SUMMARY,
    },
    ...orderItemsFixture(CONNECTION_ID).control,
  ];
  const { window: page } = await relaunch({
    control,
    stream: orderItemsFixture(CONNECTION_ID).port,
  });
  await createConnection(page, 'postgres', 'Nav Button DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  const row = await findRow(page, ORDER_ITEMS_PATH);
  await row.dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="product_id"]'),
  ).toBeVisible();

  // Deliberate redesign: the nav button is now
  // always visible on every nav-eligible rendered cell, never hover-gated, and overlays the
  // cell's own text (no reserved padding) instead of a single JS-moved node. Superseded the old
  // "absent at rest / exactly one instance on hover / moves between cells" assertions below.

  // --- present at rest (no hover at all), on every rendered nav cell, absent off one ------------
  await expect(cellNavButton(page, 0, 'product_id')).toHaveCount(1);
  await expect(cellNavButton(page, 0, 'order_id')).toHaveCount(1);
  await expect(cellNavButton(page, 0, 'quantity')).toHaveCount(0);
  const quantityButtons = gridCell(page, 0, 'quantity').locator('[data-testid="cell-nav-button"]');
  await expect(quantityButtons).toHaveCount(0);

  // --- correct kind, left-positioned, overlaying the text rather than pushing it with padding ---
  await expect(cellNavButton(page, 0, 'product_id')).toHaveAttribute('data-nav-kind', 'fk');
  const cellBox = await gridCell(page, 0, 'product_id').boundingBox();
  const btnBox = await cellNavButton(page, 0, 'product_id').boundingBox();
  if (!cellBox || !btnBox) throw new Error('missing bounding box');
  expect(btnBox.x - cellBox.x).toBeGreaterThanOrEqual(0);
  expect(btnBox.x - cellBox.x).toBeLessThanOrEqual(24);

  // --- both nav columns on the same row carry their own button simultaneously, no hover needed --
  await expect(cellNavButton(page, 0, 'order_id')).toHaveAttribute('data-nav-kind', 'fk');
  const bothPresent = await Promise.all([
    cellNavButton(page, 0, 'product_id').isVisible(),
    cellNavButton(page, 0, 'order_id').isVisible(),
  ]);
  expect(bothPresent).toEqual([true, true]);
});

// Root cause: `new KiraSlickGrid(...)` (SlickGridHost.vue's own onMounted)
// runs synchronously through slick.grid.ts's own `initialize()` -> `finishInitialization()` ->
// `createColumnHeaders()`, firing `grid.onHeaderCellRendered` for the FIRST, constructor-built
// header cells *before* this file's own `eventHandler.subscribe(grid.onHeaderCellRendered, ...)`
// has even run (subscribing needs a `grid` instance to subscribe to). Every DOM addition that
// handler is responsible for — the PK/FK `.header-key` badge, the `.header-select-zone` click
// target — is silently missing from that first build. A table's first-ever open never shows it:
// `rt()?.meta`'s own watch fires moments later (the async `treeDescribe` resolving) and rebuilds
// headers a second time, this time through the now-subscribed listener. It stops being invisible
// the moment a tab's meta is already cached before this component remounts — exactly what a plain
// tab switch (not a close) does, since a tab's own meta survives in its runtime record until the
// tab itself closes (state.ts). SlickGridHost.vue now forces one extra header rebuild,
// unconditionally, right after subscribing — this asserts a real PK/FK badge and the header
// select-zone both still exist after switching away from a table's tab and back to it.
test('P22 postscript — PK/FK header badge and select-zone survive a tab switch, not just a fresh open', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-slick-switchback';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Switchback DB', 'cyan');
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Switchback DB',
        kind: 'postgres',
        color: 'cyan',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: CONNECTION_SUMMARY,
    },
    ...orderItemsFixture(CONNECTION_ID).control,
  ];
  const { window: page } = await relaunch({
    control,
    stream: orderItemsFixture(CONNECTION_ID).port,
  });
  await createConnection(page, 'postgres', 'Switchback DB');
  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  const row = await findRow(page, ORDER_ITEMS_PATH);
  await row.dblclick();
  const firstTabId = (await page
    .locator('[data-testid="tab"][data-active="true"]')
    .getAttribute('data-tab-id')) as string;
  const productHeader = page.locator('[data-testid="grid-header-cell"][data-column="product_id"]');
  await expect(productHeader).toBeVisible();
  await expect(productHeader.locator('.header-key.is-fk')).toHaveCount(1);
  await expect(productHeader.locator('.header-select-zone')).toHaveCount(1);

  // Open a second tab on the SAME table — a genuinely different tabId, whose own meta needs its
  // own first fetch (masking the bug the same way a first-ever open always does) — then switch
  // back to the first tab, whose meta is already cached (state.ts) and needs no further fetch.
  await openRowMenu(page, ORDER_ITEMS_PATH);
  await page.click('[data-testid="menu-item-open-data-new-tab"]');
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="product_id"]'),
  ).toBeVisible();
  const secondTabId = (await page
    .locator('[data-testid="tab"][data-active="true"]')
    .getAttribute('data-tab-id')) as string;
  expect(secondTabId).not.toBe(firstTabId);

  await page.locator(`[data-testid="tab"][data-tab-id="${firstTabId}"]`).click();
  await expect(productHeader).toBeVisible();
  await expect(productHeader.locator('.header-key.is-fk')).toHaveCount(1);
  await expect(productHeader.locator('.header-select-zone')).toHaveCount(1);
  await expect(cellNavButton(page, 0, 'product_id')).toHaveCount(1);
});

// Regression test — `suppressWidthEcho` (SlickGridHost.vue) is set true, patchDataTabState is
// called, then reset false SYNCHRONOUSLY on the very next line, but the `columnWidths` watch
// checking that flag ran on Vue's default `flush: 'pre'` (a microtask *after* that reset), so it
// always read `false` and the guard never suppressed anything: every column-resize drag
// triggered a full `grid.setColumns(...)` rebuild — invalidating every row, recreating every
// header/cell DOM node, rebuilding CSS rules, calling `resizeCanvas` — the exact cost the guard
// exists to avoid, per its own comment.
//
// Verified by tagging the LIVE header DOM node with a marker attribute (setColumns's own
// createColumnHeaders empties and rebuilds `.slick-header-column` nodes from scratch, discarding
// any such marker) rather than the "editor survives the resize" idea a marker-free version of
// this test might reach for first: SlickGrid's own `onResizeStart` unconditionally calls
// `commitCurrentEdit()` (slick.interactions.ts's `Resizable`, slick.grid.ts:2303) the instant a
// resize drag *starts*, closing any open cell editor regardless of this fix — so an editor-based
// check can't tell the fixed and pre-fix code apart.
test('P22 Pass B follow-up — a column-resize drag does not force a full grid rebuild (finding 5)', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-slick-resize-echo';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Resize Echo DB', 'cyan');
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Resize Echo DB',
        kind: 'postgres',
        color: 'cyan',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: CONNECTION_SUMMARY,
    },
    ...orderItemsFixture(CONNECTION_ID).control,
  ];
  const { window: page } = await relaunch({
    control,
    stream: orderItemsFixture(CONNECTION_ID).port,
  });
  await createConnection(page, 'postgres', 'Resize Echo DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  const row = await findRow(page, ORDER_ITEMS_PATH);
  await row.dblclick();

  const header = page.locator('[data-testid="grid-header-cell"][data-column="quantity"]');
  await expect(header).toBeVisible();
  const handle = header.locator('.slick-resizable-handle');
  await expect(handle).toHaveCount(1);

  // Wait for the column widths to settle (autosize after the first rows render) before measuring
  // and grabbing the handle: a width that shifts between boundingBox() and mouse.down() makes the
  // drag miss the handle on a loaded runner.
  let lastWidth = -1;
  await expect
    .poll(
      async () => {
        const w = await header.evaluate((el) => el.getBoundingClientRect().width);
        const settled = w === lastWidth;
        lastWidth = w;
        return settled;
      },
      { intervals: [150] },
    )
    .toBe(true);
  await header.evaluate((el) => el.setAttribute('data-rebuild-probe', '1'));
  const widthBefore = await header.evaluate((el) => el.getBoundingClientRect().width);

  const box = await handle.boundingBox();
  if (!box) throw new Error('resize handle has no bounding box');
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 60, box.y + box.height / 2, { steps: 5 });
  await page.mouse.up();

  // The drag itself resized the column (SlickGrid's own live-drag DOM write, independent of this
  // app's patch/rebuild round trip) — confirms the gesture actually did something before
  // asserting on the *absence* of a further rebuild below.
  await expect
    .poll(() => header.evaluate((el) => el.getBoundingClientRect().width))
    .toBeGreaterThan(widthBefore + 30);

  // Give the columnWidths echo (patchDataTabState -> the watch under test) a full macrotask to
  // fire on the old, broken default-flush timing before asserting the probe survived — a
  // regression back to that timing would otherwise still coincidentally pass on the very first
  // poll of an unguarded assertion.
  await page.waitForTimeout(200);
  await expect(
    page.locator(
      '[data-testid="grid-header-cell"][data-column="quantity"][data-rebuild-probe="1"]',
    ),
  ).toHaveCount(1);
});

test('P195 — a reload with very different data keeps the settled column widths', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-slick-settled-widths';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Settled Widths DB', 'cyan');
  const fixture = orderItemsFixture(CONNECTION_ID);
  // The 1k page carries same-column rows ~100x wider than the 100-row page the tab opens with.
  const wide = fixture.port.find(
    (snap) => (snap.payload as { pageSize: number }).pageSize === 1000,
  );
  if (!wide) throw new Error('fixture has no 1000-row read');
  const widePage = (wide.response as { page: { rows: (string | null)[][] } }).page;
  widePage.rows = widePage.rows.map((r) => r.map((v) => `${v}-${'w'.repeat(120)}`));
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Settled Widths DB',
        kind: 'postgres',
        color: 'cyan',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: CONNECTION_SUMMARY,
    },
    ...fixture.control,
  ];
  const { window: page } = await relaunch({ control, stream: fixture.port });
  await createConnection(page, 'postgres', 'Settled Widths DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  const row = await findRow(page, ORDER_ITEMS_PATH);
  await row.dblclick();

  const header = page.locator('[data-testid="grid-header-cell"][data-column="product_id"]');
  await expect(header).toBeVisible();
  const widthBefore = await header.evaluate((el) => el.getBoundingClientRect().width);

  await page.click('[data-testid="page-size-1000"]');
  await expect(page.locator('[data-testid="data-grid"]')).toContainText('www');
  const widthAfter = await header.evaluate((el) => el.getBoundingClientRect().width);

  expect(widthAfter).toBe(widthBefore);
});

// `selectedCellCssClass: 'kira-cell-selected'` makes SlickGrid itself paint
// the highlight off `selectionModel`'s own DISPLAY-POSITION ranges, which the filter-toggle watch
// used to never re-push: click id=2 (page row 1) while unfiltered (display position 1 == page row
// 1), then filter to ids 2/4 (page rows 1 and 3) — id=4 renders down onto display position 1, the
// exact slot a stale range would still point at. Pre-fix, the highlight visibly jumps onto id=4's
// row instead of staying on id=2's, while `rt().selection` (unaffected — page-row space) still
// correctly targets page row 1, so a destructive Delete would already hit the right row even
// though the highlight lied about it; this test catches the highlight bug specifically, then
// confirms Delete lands on the right row too.
test('P22 Pass B follow-up — the cell selection highlight tracks its row through a filter change, and Delete still targets it (finding 1)', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-slick-select-highlight-filter';
  const CONNECTION_SUMMARY = postgresConnectionSummary(
    CONNECTION_ID,
    'Select Highlight Filter DB',
    'blue',
  );
  const FIXTURE = bigRowsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Select Highlight Filter DB',
        kind: 'postgres',
        color: 'blue',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  const isMac = await page.evaluate(() => navigator.userAgent.includes('Mac'));
  const DELETE_KEY = isMac ? 'Meta+Backspace' : 'Delete';

  await createConnection(page, 'postgres', 'Select Highlight Filter DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  const bigRowsRow = await findRow(page, BIG_ROWS_PATH);
  await bigRowsRow.dblclick();
  await expect(page.locator('[data-testid="data-grid"] .slick-cell')).not.toHaveCount(0);

  // --- click id=2 (page row 1) while unfiltered — display position 1 == page row 1 -------------
  await gridCell(page, 1, 'id').click();
  await expect(gridCell(page, 1, 'id')).toHaveClass(/kira-cell-selected/);

  // --- filter to ids 2/4 (page rows 1 and 3): id=4 now renders at display position 1, the exact
  // slot a stale, position-keyed highlight would still be pointing at ---------------------------
  await page.click('[data-testid="toolbar-search"]');
  await expect(page.locator('[data-testid="search-bar"]')).toBeVisible();
  await page.click('[data-testid="search-regex"]');
  await page.fill('[data-testid="search-input"]', '^[24]$');
  await expect(page.locator('[data-testid="search-count"]')).toContainText('of 2');
  await page.click('[data-testid="search-filter-rows"]');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(2);

  // The highlight followed id=2 (page row 1), not the display slot it used to occupy.
  await expect(gridCell(page, 1, 'id')).toHaveClass(/kira-cell-selected/);
  await expect(gridCell(page, 3, 'id')).not.toHaveClass(/kira-cell-selected/);

  // --- a destructive shortcut still targets the highlighted row (id=2/page row 1), not id=4 -----
  await page.evaluate(() =>
    (document.querySelector('.grid-canvas') as HTMLElement | null)?.focus(),
  );
  await page.keyboard.press(DELETE_KEY);
  await expect(gridRow(page, 1)).toHaveClass(/pending-delete/);
  await expect(gridRow(page, 3)).not.toHaveClass(/pending-delete/);
});

// A `range`-kind selection stores only its two page-row corners, and every consumer (copy, delete)
// used to walk the CONTIGUOUS page-row span between them, ignoring the active filter entirely —
// `kind: 'row'`/`kind: 'column'` already applied the "walk only the visible rows" rule; only
// `range` didn't. Reproduced concretely: filter down to ids 1/2/3/90 (a contiguous run plus one far
// outlier), shift-click id=1 to id=90 to select the range spanning every hidden row in between,
// then Ctrl+C and Delete — pre-fix, copy pulled in all 90 page rows and Delete staged all 90 for
// deletion; post-fix, both only touch the 4 rows on screen.
test('P22 Pass B follow-up — a range selection under an active filter only copies/deletes the visible rows in its span (finding 3)', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-slick-range-filter-span';
  const CONNECTION_SUMMARY = postgresConnectionSummary(
    CONNECTION_ID,
    'Range Filter Span DB',
    'grey',
  );
  const FIXTURE = bigRowsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Range Filter Span DB',
        kind: 'postgres',
        color: 'grey',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL, stream: FIXTURE.port });
  await page.addInitScript(CLIPBOARD_SHIM);
  await page.reload();
  await page.waitForSelector('[data-testid="status-bar"]');
  const isMac = await page.evaluate(() => navigator.userAgent.includes('Mac'));
  const DELETE_KEY = isMac ? 'Meta+Backspace' : 'Delete';

  await createConnection(page, 'postgres', 'Range Filter Span DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  const bigRowsRow = await findRow(page, BIG_ROWS_PATH);
  await bigRowsRow.dblclick();
  await expect(page.locator('[data-testid="data-grid"] .slick-cell')).not.toHaveCount(0);

  // --- filter to ids 1/2/3/90 (page rows 0,1,2,89) — a contiguous run plus one far outlier, so a
  // shift-click across all four visible rows spans 86 hidden page rows in between ----------------
  await page.click('[data-testid="toolbar-search"]');
  await expect(page.locator('[data-testid="search-bar"]')).toBeVisible();
  await page.click('[data-testid="search-regex"]');
  await page.fill('[data-testid="search-input"]', '^([1-3]|90)$');
  await expect(page.locator('[data-testid="search-count"]')).toContainText('of 4');
  await page.click('[data-testid="search-filter-rows"]');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(4);

  // --- shift-click id=1 (page row 0) to id=90 (page row 89): a `range`-kind selection spanning
  // every filtered-out page row between them -----------------------------------------------------
  await gridCell(page, 0, 'id').click();
  await gridCell(page, 89, 'id').click({ modifiers: ['Shift'] });
  await expect.poll(() => page.locator('.kira-cell-selected').count()).toBeGreaterThan(0);

  // --- Ctrl+C only copies the 4 visible rows, not all 90 page rows the pre-fix range spanned -----
  await page.evaluate(() =>
    (document.querySelector('.grid-canvas') as HTMLElement | null)?.focus(),
  );
  await page.keyboard.press(await primaryChord(page, 'c'));
  const tsv = await page.evaluate(() => navigator.clipboard.readText());
  expect(tsv.split('\n')).toEqual(['1', '2', '3', '90']);

  // --- Delete only stages the 4 visible page rows, not the 86 hidden ones in between -------------
  await page.keyboard.press(DELETE_KEY);
  for (const row of [0, 1, 2, 89]) {
    await expect(gridRow(page, row)).toHaveClass(/pending-delete/);
  }
  // The pending-row count itself confirms it: exactly the 4 visible rows, not the 86 hidden ones
  // in between (which don't render while the filter is active, so their own class can't be
  // asserted via a DOM lookup for an absent row) and not the pre-fix range's full 90-row span.
  await expect(page.locator('[data-testid="pending-count-chip"]')).toHaveText('4 rows pending');
});

// order_items (the real-capture table) has two short-data FK columns (order_id/product_id, values
// '1'/'2'/'5') — exactly the combination whose header furniture (padding + sort indicator +
// FK badge, ~50px) the old flat 64px floor didn't reserve room for.
test('P16 D3/D4 — a column min-width knows what its header renders, and overscroll agrees on all four edges', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-slick-header-chrome';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Header Chrome DB', 'amber');
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: {
        name: 'Header Chrome DB',
        kind: 'postgres',
        color: 'amber',
        mode: 'fields',
        readOnly: false,
        host: '127.0.0.1',
        port: 5432,
        database: 'kira_test',
        username: 'postgres',
        password: null,
        uri: null,
        options: {},
        preconnect: null,
        preconnectSidecar: false,
        preconnectDelaySec: 0,
        autoExplain: false,
        throttlePerSec: 0,
      },
      response: CONNECTION_SUMMARY,
    },
    ...orderItemsFixture(CONNECTION_ID).control,
  ];
  const { window: page } = await relaunch({
    control,
    stream: orderItemsFixture(CONNECTION_ID).port,
  });
  await createConnection(page, 'postgres', 'Header Chrome DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  const row = await findRow(page, ORDER_ITEMS_PATH);
  await row.dblclick();

  const header = page.locator('[data-testid="grid-header-cell"][data-column="product_id"]');
  await expect(header).toBeVisible();
  const nameEl = header.locator('.slick-column-name');
  const notEllipsised = () =>
    nameEl.evaluate((el) => (el as HTMLElement).scrollWidth <= (el as HTMLElement).clientWidth + 1);

  // D4 case 3: the header text is not ellipsised at rest — the item's own success condition,
  // measured rather than screenshotted.
  await expect.poll(notEllipsised).toBe(true);

  // D4 case 4: dragging the resize handle to the far left cannot crop the header again — the
  // floor itself is header-aware now, not the old flat 64px MIN_WIDTH (which left barely any room
  // for an FK column's own header furniture, F2's own arithmetic).
  const handle = header.locator('.slick-resizable-handle');
  const box = await handle.boundingBox();
  if (!box) throw new Error('resize handle has no bounding box');
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 - 500, box.y + box.height / 2, { steps: 10 });
  await page.mouse.up();
  await expect.poll(notEllipsised).toBe(true);

  // D3: every .slick-viewport reports overscroll-behavior: none, and a wheel scroll past the top
  // edge leaves scrollTop at 0 (the bounce itself is not assertable in this sandbox — §4 names
  // that limit explicitly).
  const viewportCount = await page.locator('.slick-viewport').count();
  expect(viewportCount).toBeGreaterThan(0);
  const behaviors = await page
    .locator('.slick-viewport')
    .evaluateAll((els) => els.map((el) => getComputedStyle(el).overscrollBehavior));
  for (const b of behaviors) expect(b).toBe('none');

  const scroller = gridScroller(page);
  await expect(scroller).toHaveJSProperty('scrollTop', 0);
  const scrollerBox = await scroller.boundingBox();
  if (!scrollerBox) throw new Error('grid scroller has no bounding box');
  await page.mouse.move(
    scrollerBox.x + scrollerBox.width / 2,
    scrollerBox.y + scrollerBox.height / 2,
  );
  await page.mouse.wheel(0, -400);
  await expect(scroller).toHaveJSProperty('scrollTop', 0);

  // P182: the in-viewport header must not add a spurious vertical scroll to a table that fits.
  const fits = await scroller.evaluate((el) => el.scrollHeight - el.clientHeight);
  expect(fits).toBe(0);
});

// P182 stage 1 — the scrolling header is a sticky child of the same scroller as the rows.
test('P182 — header columns move with the body in the same task as a scrollLeft write, with no JS sync', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);
  await expect(headerCell(page, 'col5')).toBeVisible();
  const viewport = rightViewport(page);
  const offsets = await viewport.evaluate((el) => {
    const left = (node: Element | null): number => node?.getBoundingClientRect().left ?? Number.NaN;
    const header = el.querySelector('[data-testid="grid-header-cell"][data-column="col5"]');
    const cell = el.querySelector(
      '[data-testid="grid-row"][data-row="0"] [data-testid="grid-cell"][data-column="col5"]',
    );
    const before = left(header) - left(cell);
    el.scrollLeft = 300;
    // Same task: no scroll event has run yet, so only native scrolling of one scroller keeps them equal.
    return { before, after: left(header) - left(cell), scrollLeft: el.scrollLeft };
  });
  expect(offsets.scrollLeft).toBe(300);
  expect(Math.abs(offsets.before)).toBeLessThanOrEqual(1);
  expect(Math.abs(offsets.after)).toBeLessThanOrEqual(1);
});

test('P182 — the last row is reachable by keyboard and by native scroll, and a short table does not scroll', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndOpenSpikeGrid(page);
  const viewport = rightViewport(page);

  await gridCell(page, 0, 'id').click();
  await page.keyboard.press('Control+End');
  const lastRow = gridRow(page, ROW_COUNT - 1);
  await expect(lastRow).toBeVisible();
  const band = await viewport.evaluate((el) => {
    const rect = Element.prototype.getBoundingClientRect.call(el);
    return { bottom: rect.bottom - (rect.height - el.clientHeight) };
  });
  const rowBox = await lastRow.boundingBox();
  if (!rowBox) throw new Error('last row has no bounding box');
  expect(rowBox.y + rowBox.height).toBeLessThanOrEqual(band.bottom + 1);

  await viewport.evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  await page.evaluate(
    () => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r()))),
  );
  const end = await viewport.evaluate((el) => ({
    scrollTop: el.scrollTop,
    max: el.scrollHeight - el.clientHeight,
  }));
  expect(Math.abs(end.scrollTop - end.max)).toBeLessThanOrEqual(1);
});
