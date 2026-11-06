import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { createConnection } from './support/connect';
import { GRID_SCROLLER_SELECTOR } from './support/grid';
import {
  BIG_ROWS_META,
  BIG_ROWS_PATH,
  bigRowsFixture,
  bigRowsHugePage,
  connectAndExpandControl,
  DB_PATH,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { expandRow, findRow, openRowMenu } from './support/tree';

// window.__kiraScrollTrace is a real-hardware field probe, not a tests/ui/ instrument
// (its own header comment, views/shared/slick/scrollTrace.ts) — nothing here can drive a real macOS
// momentum scroll or observe the compositor-ahead condition that produces the user's actual
// symptom. What this file *can* prove, sandboxed: the probe exists, is inert until start(), and
// start()/stop() produce the documented shape when driven by ordinary (main-thread) scrolling —
// the mechanism working as designed, not the real-hardware fix being proven (that's
// docs/HISTORY.md, run by a human on a Mac).

const CONNECTION_ID = 'conn-scroll-trace';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Scroll Trace DB', 'grey');

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: {
      name: 'Scroll Trace DB',
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
  ...connectAndExpandControl(CONNECTION_ID),
  {
    channel: IPC.treeDescribe,
    args: { connectionId: CONNECTION_ID, path: BIG_ROWS_PATH, refresh: false, tabId: null },
    response: { meta: BIG_ROWS_META, source: 'server' },
  },
];

const PORT: PortSnapshot[] = [
  ...bigRowsFixture(CONNECTION_ID).port,
  bigRowsHugePage(CONNECTION_ID),
];

test('__kiraScrollTrace — inert until start(), documented shape on stop()', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });

  await createConnection(page, 'postgres', 'Scroll Trace DB');

  const connRow = page.locator('[data-testid="tree-row"][data-kind="connection"]');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });

  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, 'database:kira_test/schema:app');
  const bigRowsRow = await findRow(page, BIG_ROWS_PATH);
  await bigRowsRow.dblclick();

  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible();
  await page.click('[data-testid="page-size-10000"]');
  // The actual scrollable element is SlickGrid's own right/data viewport
  // (support/grid.ts's own GRID_SCROLLER_SELECTOR), not the outer `[data-testid="data-grid"]`
  // host div: that div never scrolls itself (it now also carries the
  // empty-state overlays as siblings of the actual SlickGrid-owned mount node), so setting
  // `scrollTop` on it moved nothing.
  const viewport = page.locator(GRID_SCROLLER_SELECTOR);
  await expect
    .poll(() => viewport.evaluate((el) => el.scrollHeight), { timeout: 15_000 })
    .toBeGreaterThan(200_000);

  // 1. The hook exists and has the documented start/stop shape.
  const hookShape = await page.evaluate(() => ({
    hasHook: typeof window.__kiraScrollTrace === 'object',
    hasStart: typeof window.__kiraScrollTrace?.start === 'function',
    hasStop: typeof window.__kiraScrollTrace?.stop === 'function',
  }));
  expect(hookShape).toEqual({ hasHook: true, hasStart: true, hasStop: true });

  // 2. Inert until start(): stop() without a prior start() returns null, not a crash or a
  // fabricated result — the D2 requirement that the per-event capture is "a single array push
  // behind a boolean" when not recording.
  const stopBeforeStart = await page.evaluate(() => window.__kiraScrollTrace?.stop());
  expect(stopBeforeStart).toBeNull();

  // 3. start()...scroll a few times...stop() produces the documented per-frame/summary shape.
  // This drives the grid the same way every other budgets.spec.ts/perf.spec.ts scroll does — a
  // main-thread scrollTop write — which is exactly the condition scrollTrace.ts's own header
  // comment says cannot reproduce the real symptom; this step proves the *plumbing*, not the fix.
  await page.evaluate(() => window.__kiraScrollTrace?.start());
  const result = await viewport.evaluate(async (el) => {
    const total = Math.max(0, el.scrollHeight - el.clientHeight);
    for (let i = 1; i <= 5; i++) {
      el.scrollTop = Math.round((total * i) / 20);
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    }
    return window.__kiraScrollTrace?.stop();
  });

  expect(result).not.toBeNull();
  expect(Array.isArray(result?.frames)).toBe(true);
  expect(result?.frames.length).toBeGreaterThan(0);
  for (const frame of result?.frames ?? []) {
    expect(typeof frame.t).toBe('number');
    expect(typeof frame.scrollEvents).toBe('number');
    expect(Array.isArray(frame.scrollTopAtEvent)).toBe(true);
    expect(frame.scrollTopAtEvent.length).toBe(frame.scrollEvents);
    expect(typeof frame.pxPerFrame).toBe('number');
    expect(typeof frame.notified).toBe('boolean');
    expect(typeof frame.uncoveredPx).toBe('number');
    expect(typeof frame.renderMs).toBe('number');
    expect(typeof frame.rows).toBe('number');
    expect(frame.rows).toBeGreaterThan(0); // big_rows is visible throughout this scroll
  }
  expect(typeof result?.summary.pxPerFrame.p50).toBe('number');
  expect(typeof result?.summary.uncoveredPx.max).toBe('number');
  expect(typeof result?.summary.renderMs.p95).toBe('number');
  expect(typeof result?.summary.scrollEventsHistogram).toBe('object');

  // 4. stop() disarms recording — a second stop() (no intervening start()) is inert again.
  const secondStop = await page.evaluate(() => window.__kiraScrollTrace?.stop());
  expect(secondStop).toBeNull();

  // 5. start() again resets state — no leftover frames from the previous recording leak in.
  // One evaluate: a round trip between start and stop lets real frames land under load.
  const freshResult = await page.evaluate(() => {
    window.__kiraScrollTrace?.start();
    return window.__kiraScrollTrace?.stop();
  });
  expect(freshResult?.frames.length ?? 0).toBeLessThanOrEqual(1); // at most the immediate tick
});
