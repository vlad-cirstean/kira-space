import type { ControlSnapshot } from '../ipc/support/types';
import { expect, studioTest as test } from './fixtures';
import {
  POSTGRES_CAPS,
  postgresConnectionSummary,
  SERVER_VERSION,
} from './studio/support/postgresFixture';
import { contract } from './support/contract';
import { dbOp } from './support/dbOps';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// The op log (workbench/panels/OperationsPanel.vue) is the fifth of the five search gaps found.
// Its own always-visible filter (a TextField with icon="filter", opsState.ts's own filterText)
// already existed — the grep for icon="search"/PanelSearchBox missed it — and already matched
// command/kind/error. The one real gap is the connection's own name, which the row asks for
// explicitly ("the op label and its connection name") and which the pre-existing filter
// did not match.

const NOW = '2026-01-01T00:00:00.000Z';

function opRecord(overrides: Record<string, unknown>): Record<string, unknown> {
  return dbOp({
    id: 'op-1',
    connectionId: null,
    tabId: null,
    startedAt: NOW,
    durationMs: 12,
    kind: 'read',
    status: 'ok',
    rows: 3,
    command: 'select 1',
    error: null,
    source: 'user',
    ...overrides,
  });
}

test('Operations panel — the filter matches the connection name, not only the command', async ({
  relaunch,
}) => {
  const CONN = postgresConnectionSummary('conn-ops-1', 'Warehouse DB', 'blue');
  const OP_A = opRecord({ id: 'op-a', connectionId: 'conn-ops-1', command: 'select * from x' });
  const OP_B = opRecord({ id: 'op-b', connectionId: null, kind: 'http', command: 'GET /health' });

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [CONN] },
    { channel: IPC.opsRecent, response: [OP_A, OP_B] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(2);

  const filter = page.locator('[data-testid="ops-filter"]');
  await filter.fill('Warehouse');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('Warehouse DB');

  await filter.fill('health');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('GET /health');

  await filter.fill('');
  await expect(rows).toHaveCount(2);
});

// commandTruncated disables Re-run but not Copy command — the one product surface the op_log byte
// cap adds. Both rows share a connected connection (caps.sql: true) and a
// non-empty command, so a plain rows-not-connected/no-command guard cannot explain the difference
// — only commandTruncated can.
test('Operations panel — a truncated command cannot be re-run, but can still be copied', async ({
  relaunch,
}) => {
  const CONN = postgresConnectionSummary('conn-ops-2', 'Warehouse DB', 'blue');
  const TRUNCATED = opRecord({
    id: 'op-truncated',
    connectionId: 'conn-ops-2',
    command: 'select 1',
    commandTruncated: true,
  });
  const WHOLE = opRecord({
    id: 'op-whole',
    connectionId: 'conn-ops-2',
    command: 'select 2',
    commandTruncated: false,
  });

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [CONN] },
    {
      channel: IPC.connectionsStates,
      response: [
        {
          connectionId: 'conn-ops-2',
          status: 'connected',
          serverVersion: SERVER_VERSION,
          error: null,
          since: 1735689600000,
          caps: POSTGRES_CAPS,
        },
      ],
    },
    { channel: IPC.opsRecent, response: [TRUNCATED, WHOLE] },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(2);

  await rows.filter({ hasText: 'select 1' }).click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-re-run"]')).toHaveAttribute(
    'data-disabled',
    '',
  );
  await expect(page.locator('[data-testid="menu-item-copy-command"]')).not.toHaveAttribute(
    'data-disabled',
  );
  await page.keyboard.press('Escape');

  await rows.filter({ hasText: 'select 2' }).click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-re-run"]')).not.toHaveAttribute(
    'data-disabled',
  );
  await expect(page.locator('[data-testid="menu-item-copy-command"]')).not.toHaveAttribute(
    'data-disabled',
  );
});

// The row's own acceptance for the full-width dock fix — the dock no longer sits in a nested
// SplitterGroup under just the main panel, so it spans project's left edge to main's right edge,
// and project's own bottom now moves with main's (no more blank strip below a shorter project
// panel, no margin hack to produce it).
test('Operations panel — the dock spans the full shell width, flush under project and main (P132 Part 1)', async ({
  kira,
}) => {
  const { window } = kira;
  await window.click('[data-testid="toggle-operations-panel"]');
  const dock = window.locator('[data-testid="operations-panel"]');
  await expect(dock).toBeVisible();

  const project = window.locator('[data-testid="project-panel"]');
  const main = window.locator('[data-testid="main-panel"]');
  const [projectBox, mainBox, dockBox, marginBottom] = await Promise.all([
    project.boundingBox(),
    main.boundingBox(),
    dock.boundingBox(),
    project.evaluate((el) => getComputedStyle(el).marginBottom),
  ]);
  if (!projectBox || !mainBox || !dockBox) throw new Error('bounding boxes not found');

  // ±0.5px throughout (the row's own tolerance) — Playwright geometry can land on a sub-pixel
  // boundary even at integer CSS values.
  expect(Math.abs(dockBox.x - projectBox.x)).toBeLessThanOrEqual(0.5);
  expect(Math.abs(dockBox.x + dockBox.width - (mainBox.x + mainBox.width))).toBeLessThanOrEqual(
    0.5,
  );
  expect(
    Math.abs(projectBox.y + projectBox.height - (mainBox.y + mainBox.height)),
  ).toBeLessThanOrEqual(0.5);
  expect(Math.abs(dockBox.y - (projectBox.y + projectBox.height) - 6)).toBeLessThanOrEqual(0.5);
  expect(marginBottom).toBe('0px');
});

// P132 Part 1 (§4.1): drag, keyboard step, and clamp — the dock ResizeHandle's three behaviours
// (§2.4), exercised through the real separator rather than unit-testing its clamp math directly.
test('Operations panel — the dock resize handle drags, steps by keyboard, and clamps (P132 Part 1)', async ({
  kira,
}) => {
  const { window } = kira;
  await window.click('[data-testid="toggle-operations-panel"]');
  const dock = window.locator('[data-testid="operations-panel"]');
  await expect(dock).toBeVisible();
  // The project panel's own reka SplitterResizeHandle is also role="separator" — this one is
  // named (its aria-label) to disambiguate.
  const handle = window.getByRole('separator', { name: 'Resize operations panel' });

  const initialBox = await dock.boundingBox();
  if (!initialBox) throw new Error('dock bounding box not found');
  const initialHeight = initialBox.height;

  const handleBox = await handle.boundingBox();
  if (!handleBox) throw new Error('handle bounding box not found');
  const cx = handleBox.x + handleBox.width / 2;
  const cy = handleBox.y + handleBox.height / 2;
  await window.mouse.move(cx, cy);
  await window.mouse.down();
  await window.mouse.move(cx, cy - 60, { steps: 5 });
  await window.mouse.up();
  const afterDragHeight = (await dock.boundingBox())?.height ?? 0;
  expect(Math.abs(afterDragHeight - initialHeight - 60)).toBeLessThanOrEqual(1);

  await handle.focus();
  await window.keyboard.press('ArrowUp');
  const afterArrowHeight = (await dock.boundingBox())?.height ?? 0;
  expect(Math.abs(afterArrowHeight - afterDragHeight - 10)).toBeLessThanOrEqual(1);

  const clampHandleBox = await handle.boundingBox();
  if (!clampHandleBox) throw new Error('handle bounding box not found');
  const clampCx = clampHandleBox.x + clampHandleBox.width / 2;
  const clampCy = clampHandleBox.y + clampHandleBox.height / 2;
  await window.mouse.move(clampCx, clampCy);
  await window.mouse.down();
  await window.mouse.move(clampCx, clampCy - 1000, { steps: 5 });
  await window.mouse.up();
  const clampedHeight = (await dock.boundingBox())?.height ?? 0;
  expect(Math.abs(clampedHeight - 500)).toBeLessThanOrEqual(1);
});

// P264: the op log is one list on the shared key set — Up/Down/Home/End move, Enter and Space
// toggle the row's detail, Shift+F10 opens the row menu and Esc returns focus to the row.
test('Operations panel — arrows, Home/End, Enter and Space toggle detail, Shift+F10 menu', async ({
  relaunch,
}) => {
  const ops = ['op-a', 'op-b', 'op-c'].map((id, i) =>
    opRecord({ id, command: `select ${i}`, startedAt: `2026-01-01T00:0${i}:00.000Z` }),
  );
  const { window: page } = await relaunch({
    control: [{ channel: IPC.opsRecent, response: ops }],
  });

  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  const items = page.locator('ul[aria-label="Operations"] > li');
  await expect(rows).toHaveCount(3);
  await expect(items).toHaveCount(3);

  await rows.first().focus();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(1)).toBeFocused();
  await expect(page.locator('[data-testid="op-row"][tabindex="0"]')).toHaveCount(1);
  await page.keyboard.press('End');
  await expect(rows.nth(2)).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(rows.nth(2)).toBeFocused();
  await page.keyboard.press('Home');
  await expect(rows.first()).toBeFocused();
  // Up from the first row returns to the filter field.
  await page.keyboard.press('ArrowUp');
  await expect(page.locator('[data-testid="ops-filter"]')).toBeFocused();
  await rows.first().focus();

  await page.keyboard.press('Enter');
  await expect(items).toHaveCount(4);
  await page.keyboard.press('Space');
  await expect(items).toHaveCount(3);

  // Right opens the detail, Left closes it; focus stays on the row.
  await page.keyboard.press('ArrowRight');
  await expect(items).toHaveCount(4);
  await page.keyboard.press('ArrowLeft');
  await expect(items).toHaveCount(3);
  await expect(rows.first()).toBeFocused();

  await page.keyboard.press('Shift+F10');
  const menu = page.locator('[data-testid="context-menu"]');
  await expect(menu).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(menu).toHaveCount(0);
  await expect(rows.first()).toBeFocused();
});

// P307: a pre-connect script is an op of its own. The Go flow test (TestSQLitePreconnectOp) stores
// its running/ok updates; this spec pushes them and checks the live timer and the Re-run gate.
test('contract: a pre-connect script row ticks a live timer and cannot be re-run', async ({
  relaunch,
}) => {
  const CONN = postgresConnectionSummary('conn-pre-1', 'Tunnel DB', 'blue');
  const running = contract<Record<string, unknown>>('preconnect-op', 'event:preconnect-running');
  const ok = contract<Record<string, unknown>>('preconnect-op', 'event:preconnect-ok');
  const base = { id: 'op-pre', connectionId: 'conn-pre-1', command: 'sleep 0.3' };

  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [CONN] },
      {
        channel: IPC.connectionsStates,
        response: [
          {
            connectionId: 'conn-pre-1',
            status: 'connected',
            serverVersion: SERVER_VERSION,
            error: null,
            since: 1735689600000,
            caps: POSTGRES_CAPS,
          },
        ],
      },
      { channel: IPC.opsRecent, response: [] },
    ],
  });
  const t0 = Date.now();
  await page.clock.install({ time: t0 });
  await page.clock.pauseAt(t0 + 1000);
  await page.click('[data-testid="toggle-operations-panel"]');

  await emitWailsEvent(page, IPC.opUpdate, {
    ...running,
    ...base,
    startedAt: new Date(t0 + 1000).toISOString(),
  });
  const row = page.locator('[data-testid="op-row"]');
  await expect(row).toHaveCount(1);
  await expect(row).toContainText('preconnect');
  await expect(row).toContainText('Tunnel DB');
  const elapsed = row.locator('[data-testid="op-elapsed"]');
  await expect(elapsed).toHaveText('0:00');
  await page.clock.runFor(2000);
  await expect(elapsed).toHaveText('0:02');

  await emitWailsEvent(page, IPC.opUpdate, { ...ok, ...base, durationMs: 2300 });
  await expect(elapsed).toHaveCount(0);
  await expect(row).toContainText('2.3');

  await row.click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-re-run"]')).toHaveAttribute(
    'data-disabled',
    '',
  );
  await expect(page.locator('[data-testid="menu-item-copy-command"]')).not.toHaveAttribute(
    'data-disabled',
  );
});

// P307 Part 2: a delayed pre-connect counts the wait down in the command cell. Backend half:
// TestSQLitePreconnectDelay.
test('contract: a delayed pre-connect row counts down before the connect', async ({ relaunch }) => {
  const CONN = postgresConnectionSummary('conn-pre-2', 'Slow Tunnel DB', 'blue');
  const running = contract<Record<string, unknown>>(
    'preconnect-delay',
    'event:preconnect-delay-running',
  );

  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [CONN] },
      { channel: IPC.opsRecent, response: [] },
    ],
  });
  const t0 = Date.now();
  await page.clock.install({ time: t0 });
  await page.clock.pauseAt(t0 + 1000);
  await page.click('[data-testid="toggle-operations-panel"]');

  await emitWailsEvent(page, IPC.opUpdate, {
    ...running,
    id: 'op-pre-delay',
    connectionId: 'conn-pre-2',
    startedAt: new Date(t0 + 1000).toISOString(),
  });
  const row = page.locator('[data-testid="op-row"]');
  await expect(row).toHaveCount(1);
  const note = row.locator('[data-testid="op-running-note"]');
  const elapsed = row.locator('[data-testid="op-elapsed"]');
  await expect(note).toHaveText('Connecting in 0:01');
  await expect(elapsed).toHaveText('0:00');

  await page.clock.runFor(1000);
  await expect(note).toHaveCount(0);
  await expect(elapsed).toHaveText('0:01');
});

// P328: the source column tells MCP tool calls from the user's own ops. Backend half:
// TestDbMcpOpsCarrySource.
test('contract: an op shows whether the user or MCP ran it', async ({ relaunch }) => {
  const ops = contract<Record<string, unknown>[]>('ops-source', 'OpsService.Recent');
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [] },
      { channel: IPC.opsRecent, response: ops },
    ],
  });

  await page.click('[data-testid="toggle-operations-panel"]');
  await expect(page.locator('[data-testid="op-row"]')).toHaveCount(2);
  const sources = page.locator('[data-testid="op-source-cell"]');
  await expect(sources).toHaveCount(2);
  const byKind = Object.fromEntries(ops.map((o, i) => [o.kind as string, i]));
  await expect(sources.nth(byKind.execute)).toHaveText('MCP');
  await expect(sources.nth(byKind.children)).toHaveText('User');
});

// P432: detail is plain code, not an editor. A two-line error shows both lines, the command is
// highlighted through Monaco's colorizer, and the fields name the tab and path.
test('Operations panel — detail shows highlighted command, full error and fields without an editor', async ({
  relaunch,
}) => {
  const CONN = postgresConnectionSummary('conn-ops-3', 'Warehouse DB', 'blue');
  const op = opRecord({
    id: 'op-err',
    connectionId: 'conn-ops-3',
    tabId: 'tab-1',
    status: 'error',
    command: 'select * from missing',
    error: 'relation "missing" does not exist\nLINE 1: select * from missing',
  });
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [CONN] },
      { channel: IPC.opsRecent, response: [op] },
    ],
  });

  await page.click('[data-testid="toggle-operations-panel"]');
  const row = page.locator('[data-testid="op-row"]');
  await expect(row).toHaveCount(1);
  await row.click();
  const detail = page.locator('[data-testid="op-detail"]');
  await expect(detail).toBeVisible();
  await expect(detail.locator('.monaco-editor')).toHaveCount(0);
  const error = detail.locator('[data-testid="op-detail-error"]');
  await expect(error).toContainText('does not exist');
  await expect(error).toContainText('LINE 1: select * from missing');
  await expect(
    detail.locator('[data-testid="op-detail-command"] span[class^="mtk"]').first(),
  ).toBeVisible();
  await expect(detail).toContainText('Warehouse DB');
});

// P432: the Connection facet narrows the list; a narrow dock keeps the command and drops Source.
test('Operations panel — Connection facet filters and a narrow dock drops Source, keeps command', async ({
  relaunch,
}) => {
  const A = postgresConnectionSummary('conn-f-a', 'Alpha DB', 'blue');
  const B = postgresConnectionSummary('conn-f-b', 'Beta DB', 'green');
  const ops = [
    opRecord({ id: 'op-fa', connectionId: 'conn-f-a', command: 'select 1' }),
    opRecord({ id: 'op-fb', connectionId: 'conn-f-b', command: 'select 2' }),
  ];
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [A, B] },
      { channel: IPC.opsRecent, response: ops },
    ],
  });

  await page.click('[data-testid="toggle-operations-panel"]');
  const rows = page.locator('[data-testid="op-row"]');
  await expect(rows).toHaveCount(2);
  await page.click('[data-testid="ops-facet"]');
  await page.click('[data-testid="ops-facet-database:conn-f-b"]');
  await page.keyboard.press('Escape');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('Beta DB');
  await page.click('[data-testid="ops-facet"]');
  await page.click('[data-testid="ops-facet-all"]');
  await page.keyboard.press('Escape');
  await expect(rows).toHaveCount(2);

  await page.setViewportSize({ width: 820, height: 700 });
  await expect(rows.first()).toContainText('select');
  await expect(page.locator('[data-testid="op-source-cell"]')).toHaveCount(0);
});
