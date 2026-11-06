import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import type { ControlLogEntry } from '../support/mockRuntime';
import {
  FORMATS_COLUMNS,
  FORMATS_DEFINITION_WITHOUT_PATH,
  FORMATS_ROWS,
  NESTED_JSON_COLUMNS,
  NESTED_JSON_ROWS,
  NULLS_AND_UNICODE_COLUMNS,
  NULLS_AND_UNICODE_ROWS,
  NULLS_AND_UNICODE_TRUNCATED_CELLS,
  NULLS_AND_UNICODE_TRUNCATED_ROWS,
  SELECT_1_AS_X_COLUMNS,
  SELECT_1_AS_X_ROWS,
  WIDE_TABLE_COLUMNS,
  WIDE_TABLE_ROWS,
} from './support/cellEditorCaptures';
import { primaryChord } from './support/chord';
import { connectionCreateArgs, createConnection } from './support/connect';
import { diffEditorText, editorText as sharedEditorText } from './support/editorText';
import { gridCell, gridCellSelector, gridScroller } from './support/grid';
import type { SeenPortRequest } from './support/mockStream';
import {
  APP_PATH,
  connectAndExpandControl,
  DB_PATH,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { connectionRow, expandRow, findRow, openRowMenu, settleFrames } from './support/tree';

// Ported from tests/e2e/cell-editor.spec.ts (P57 D16), against real captures of app.formats,
// app.wide_table, app.nulls_and_unicode and app.nested_json (scripts/capture-postgres-tree.ts's
// existing 'read'/'execute'/'definition' step kinds already covered every call this file needs —
// no extension required). This file is overwhelmingly real, keyboard/DOM-driven CodeMirror and
// Vue-component behaviour (format autodetection, beautify, override, the timestamp/hex/base64
// panes, staging semantics, tab ownership) — none of it depends on Electron internals, so nearly
// all of it ports.
//
// What changed getting here:
//   - `window.kira.connectionsCreate(...)` (a raw IPC shortcut, `window.kira` no longer exists
//     post-M2/M3) becomes a real connection-dialog flow, same as every other Postgres-backed port.
//   - The zero-ops invariant (scenario 10) used `window.kira.opsRecent(...)`, a Go-side op log with
//     no equivalent here. `control.log()`/`stream.ops()` (this tier's own per-call records, already
//     built for exactly this purpose) are the direct substitute: they list every control/data-plane
//     call this tier's mocks actually answered, filtered to the connection under test, so "the cell
//     editor issues no ops of its own" becomes "neither log grows across the editor-only
//     interactions" — a more precise statement than the original's Go-side count, not a weaker one.
//   - Scenario 12's own `relaunch()` half (proving cell selection doesn't survive a restart) is
//     dropped: `tests/ui/fixtures.ts`'s `relaunch()` opens a brand-new Playwright browser context
//     every call (fresh storage, not merely fresh mocks), so "nothing selected after relaunch" is
//     true by construction here regardless of the app's real behaviour — it cannot fail even if the
//     app had a genuine persistence bug, the same unfalsifiable-in-this-tier category
//     workbench.spec.ts's dropped persistence checks fall into, just inverted (proving absence
//     instead of presence). The rest of scenario 12 (visibility follows selection, no manual
//     toggle) is pure UI and ports unchanged.
//   - The Docker/`startPostgres()`/`isDockerAvailable()` bootstrap and the final screenshot capture
//     are dropped outright — no real backend, and the screenshot asserted nothing.
//
// A real, useful finding while capturing: app.nulls_and_unicode's oversized row (a 1 MB text value,
// a 256 KB bytea value) comes back from the read pipeline itself already truncated to 65536
// characters (64 KiB) per cell, with `truncatedCells: 2` — confirming the "64 KB" badge text
// scenario 7 asserts on is the adapter's own real truncation, not a value this fixture chose.
// app.nested_json's `data` column captures with `typeClass: 'json'` cleanly — no special encoding
// needed beyond that tag; `mockStreamBrowser.js`'s row encoding is uniform across every typeClass.
//
// Two genuine mock-infra gaps this file's own scenarios exposed and fixed (both optional
// additions, backward-compatible with every existing fixture):
//   - The "a truncated value is read-only" scenario needs a per-*cell* truncation marker
//     (page.ts's `isTruncated`/`chunk.truncated` bitset), which `LogicalTabularPage` had no way to
//     express and `mockStreamBrowser.js`'s `encodeChunk` always answered empty — no earlier ported
//     spec touched a truncated cell. `LogicalTabularPage.truncatedRows` (tests/ipc/support/types.ts)
//     and `encodeChunk`'s new second argument close that gap.
//   - The timestamp-translate-pane scenario asserts Local and UTC readings differ, which is only
//     true if the host machine's own timezone isn't UTC — false in this sandbox specifically (its
//     system clock is UTC). `RelaunchOptions.timezoneId` (tests/ui/fixtures.ts) lets a spec pin a
//     real, non-UTC zone instead of assuming the host's.

// CodeMirror's defaultKeymap binds selectAll to "Mod-a", which resolves to Cmd on macOS and Ctrl
// elsewhere — a literal 'Control+A' silently no-ops on macOS (the keystroke just doesn't match any
// binding), leaving the prior selection/cursor alone so typed text inserts instead of replacing.
const SELECT_ALL = process.platform === 'darwin' ? 'Meta+A' : 'Control+A';

const FORMATS_PATH = `${APP_PATH}/table:formats`;
const WIDE_PATH = `${APP_PATH}/table:wide_table`;
const NULLS_PATH = `${APP_PATH}/table:nulls_and_unicode`;
const NESTED_PATH = `${APP_PATH}/table:nested_json`;

const PAGE_POSITION = {
  offset: 0,
  pageSize: 100,
  hasMore: false,
  nextToken: null,
  prevToken: null,
  strategy: 'keyset' as const,
};

const TABLE_FIXTURES: Record<
  string,
  {
    columns: ColumnDescriptor[];
    rows: (string | null)[][];
    truncatedCells: number;
    truncatedRows?: number[][];
  }
> = {
  [FORMATS_PATH]: { columns: FORMATS_COLUMNS, rows: FORMATS_ROWS, truncatedCells: 0 },
  [WIDE_PATH]: { columns: WIDE_TABLE_COLUMNS, rows: WIDE_TABLE_ROWS, truncatedCells: 0 },
  [NULLS_PATH]: {
    columns: NULLS_AND_UNICODE_COLUMNS,
    rows: NULLS_AND_UNICODE_ROWS,
    truncatedCells: NULLS_AND_UNICODE_TRUNCATED_CELLS,
    truncatedRows: NULLS_AND_UNICODE_TRUNCATED_ROWS,
  },
  [NESTED_PATH]: { columns: NESTED_JSON_COLUMNS, rows: NESTED_JSON_ROWS, truncatedCells: 0 },
};

function tableReadsFor(connectionId: string, paths: string[]): PortSnapshot[] {
  return paths.map((path) => {
    const fx = TABLE_FIXTURES[path];
    return {
      op: DATA_OP.read,
      payload: {
        connectionId,
        path,
        projection: null,
        filter: null,
        sort: null,
        pageSize: 100,
        cursor: { mode: 'offset', offset: 0 },
      },
      response: {
        kind: 'read' as const,
        page: {
          kind: 'tabular' as const,
          columns: fx.columns,
          rows: fx.rows,
          position: PAGE_POSITION,
          truncatedCells: fx.truncatedCells,
          truncatedRows: fx.truncatedRows,
        },
        source: 'server' as const,
      },
    };
  });
}

async function cellText(
  page: import('@playwright/test').Page,
  row: number,
  column: string,
): Promise<string> {
  return (await gridCell(page, row, column)).innerText();
}

async function selectCell(
  page: import('@playwright/test').Page,
  row: number,
  column: string,
): Promise<void> {
  await gridCell(page, row, column).click();
}

// The data grid virtualizes columns the same way the tree virtualizes rows (DataGrid.vue's
// visibleColumnIndices) — a column not currently scrolled into view has no DOM node, so a
// wide_table column past the first screenful must be scrolled into view before it can be selected
// or asserted on.
//
// This used to poll `[data-testid="grid-header-cell"]` for the guard —
// correct against the incumbent engine (DataGrid.vue), which virtualized header columns exactly
// like data cells, but stale against SlickGrid: `SlickGridHost.vue`'s own `buildColumns` sets
// `headerCellAttrs` on every `Column` up front, and SlickGrid builds every header cell once per
// `setColumns` call regardless of scroll position (`onHeaderCellRendered`'s own comment — "headers
// aren't virtualized"), so a header locator's count is never 0, the `if` below never entered its
// own scroll loop, and the function silently did nothing while the *data cell* (still genuinely
// column-virtualized) stayed off-DOM — the real mechanism behind three `cell-editor.spec.ts`
// failures bisected to before this whole migration started (they never depended on which grid
// engine was live; only this helper's own guard did). Root-caused and fixed here rather than
// worked around: poll the data cell itself, the thing that's gated on scroll position
// under both engines, with `row` (default 0, matching every call site but the two `big_text` ones)
// naming which row's cell has to exist.
async function scrollColumnIntoView(
  page: import('@playwright/test').Page,
  column: string,
  row = 0,
): Promise<void> {
  const grid = gridScroller(page);
  const target = page.locator(gridCellSelector(row, column));
  if ((await target.count()) === 0) {
    await grid.evaluate((el) => {
      el.scrollLeft = 0;
    });
    await settleFrames(page);
    for (let i = 0; i < 80; i++) {
      if ((await target.count()) > 0) break;
      const atEnd = await grid.evaluate(
        (el) => el.scrollLeft + el.clientWidth >= el.scrollWidth - 1,
      );
      if (atEnd) break;
      await grid.evaluate((el) => {
        el.scrollLeft += Math.max(200, el.clientWidth);
      });
      await settleFrames(page);
    }
    if ((await target.count()) === 0) {
      await grid.evaluate((el) => {
        el.scrollLeft = el.scrollWidth;
      });
      await settleFrames(page);
    }
  }
  await settleFrames(page);
}

// P60a §9.1/OQ-3: Monaco virtualises lines — this reads the encoded pane's own MonacoHost through
// its `data-kira-editor-text` debug-hook attribute (`sharedEditorText`, tests/ui/support/editorText.ts)
// rather than `.view-lines`' rendered-subset-only DOM text.
async function editorText(page: import('@playwright/test').Page): Promise<string> {
  return sharedEditorText(page.locator('[data-testid="cell-editor-panel"]'));
}

async function kindOf(page: import('@playwright/test').Page, row: number): Promise<string> {
  return cellText(page, row, 'kind');
}

/** P42 D27: the format picker is an app-drawn menu now, not a native <select> — 'auto' or a
 *  CellFormat key opens the trigger and clicks the matching row. */
async function selectFormat(page: import('@playwright/test').Page, format: string): Promise<void> {
  await page.click('[data-testid="cell-editor-format"]');
  await page.click(`[data-testid="menu-item-format-${format}"]`);
}

/** `json-invalid` is the only fixture kind whose expected detection differs from its own name. */
// P42 D23: uuid and url are gone as detected formats (F19 — both were inert on selection) — the
// fixture's own 'uuid'/'url' sample rows now detect as plain text, same as any other string with
// no distinguishing shape.
function expectedFormatFor(kind: string): string {
  if (kind === 'json-invalid') return 'json';
  if (kind === 'uuid' || kind === 'url') return 'text';
  return kind;
}

async function fillConnectionDialog(
  page: import('@playwright/test').Page,
  opts: { name: string; color: string; readOnly?: boolean },
): Promise<void> {
  // Colour and read-only live in the file: the canned Create response carries them.
  await createConnection(page, 'postgres', opts.name);
}

/** Fills, saves, connects (via the tree's own root row) and expands down to `app` — the shared
 *  boilerplate for a spec's *first* connection, mirroring definition.spec.ts's own
 *  `connectAndExpand`. A second connection (test 1's read-only one) needs different menu handling
 *  (two rows now share `data-path=""`) and is written inline where it's used, same as
 *  mutations.spec.ts's own read-only scenario. */
async function connectAndExpand(
  page: import('@playwright/test').Page,
  opts: { name: string; color: string; readOnly?: boolean },
): Promise<void> {
  await fillConnectionDialog(page, opts);
  const connRow = connectionRow(page, opts.name);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
}

function controlOpsFor(entries: ControlLogEntry[], connectionId: string): number {
  return entries.filter(
    (e) => (e.args as { connectionId?: string } | undefined)?.connectionId === connectionId,
  ).length;
}

function streamOpsFor(entries: SeenPortRequest[], connectionId: string): number {
  return entries.filter(
    (e) => (e.payload as { connectionId?: string } | undefined)?.connectionId === connectionId,
  ).length;
}

// ================================================================================================
// Test 1
// ================================================================================================

const CONNECTION_ID = 'conn-cell-editor';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Cell Editor DB', 'green');
const RO_CONNECTION_ID = 'conn-cell-editor-ro';
const RO_CONNECTION_SUMMARY = {
  ...postgresConnectionSummary(RO_CONNECTION_ID, 'Cell Editor DB (RO)', 'red'),
  readOnly: true,
};

const CONTROL_1: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Cell Editor DB', 'green'),
    response: CONNECTION_SUMMARY,
  },
  ...connectAndExpandControl(CONNECTION_ID),
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Cell Editor DB (RO)', 'red'),
    response: RO_CONNECTION_SUMMARY,
  },
  ...connectAndExpandControl(RO_CONNECTION_ID),
];

const PORT_1: PortSnapshot[] = [
  ...tableReadsFor(CONNECTION_ID, [FORMATS_PATH, NESTED_PATH, WIDE_PATH, NULLS_PATH]),
  ...tableReadsFor(RO_CONNECTION_ID, [FORMATS_PATH]),
];

test('cell editor — autodetect, beautify, override, NULL/empty/truncated, read-only', async ({
  relaunch,
  consoleErrors,
}) => {
  // One long scenario (~30s idle); the 60s default leaves no headroom under CPU contention.
  test.setTimeout(120_000);
  // A fixed, non-UTC zone (see fixtures.ts's own doc comment on `timezoneId`): this scenario's
  // "the local reading is not UTC" assertion needs Local and UTC to genuinely differ, which isn't
  // guaranteed of the host machine's own zone.
  const {
    window: page,
    control,
    stream,
  } = await relaunch({
    control: CONTROL_1,
    stream: PORT_1,
    timezoneId: 'America/New_York',
  });

  await connectAndExpand(page, { name: 'Cell Editor DB', color: 'green' });

  const formatsRow = await findRow(page, FORMATS_PATH);
  await formatsRow.dblclick();
  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible();
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="sample"]'),
  ).toBeVisible();
  const panel = page.locator('[data-testid="cell-editor-panel"]');

  // --- scenario 1: populate --------------------------------------------------------------
  await selectCell(page, 0, 'sample');
  await panel.waitFor();
  const tabId = (await panel.getAttribute('data-cell-key'))?.split(':')[0] ?? '';
  expect(tabId).not.toBe('');
  await expect(panel).toHaveAttribute('data-cell-key', `${tabId}:0:sample`);
  await expect(panel).toHaveAttribute('data-detected', expectedFormatFor(await kindOf(page, 0)));
  await expect(page.locator('[data-testid="cell-editor-target"]')).toContainText('formats.sample');
  await expect(page.locator('[data-testid="cell-editor-target"]')).toContainText('row 1');
  expect(await editorText(page)).toBe(await cellText(page, 0, 'sample'));

  // P22 D13 (F22): the dock's own boundary still renders after .cell-dock's border-top is
  // removed in favour of PanelSplitter's `divider` prop — one mechanism, same rendered line.
  await expect
    .poll(() => page.locator('.cell-splitter').evaluate((el) => getComputedStyle(el).boxShadow))
    .not.toBe('none');

  // --- scenario 1b: search (item U, D14) — the cell's own value, often a large JSON blob,
  // through ResponsePane.vue's own find bar over the encoded pane's own doc -----------------
  await page.click('[data-testid="cell-editor-search-toggle"]');
  const findBar = panel.locator('[data-testid="http-find-bar"]');
  await expect(findBar).toBeVisible();
  await page.fill('[data-testid="http-find-input"]', 'tags');
  await expect(panel.locator('[data-testid="http-find-count"]')).toContainText('1 of 1');
  await expect(
    page.locator('[data-testid="cell-editor-encoded"] .kira-ed-find-match-current'),
  ).toHaveCount(1);
  await page.keyboard.press('Escape');
  await expect(findBar).toHaveCount(0);
  await expect(page.locator('[data-testid="cell-editor-search-toggle"]')).toHaveAttribute(
    'aria-pressed',
    'false',
  );

  // --- scenario 2: every format, read out of the fixture's own `kind` column -------------
  for (let row = 0; row < 13; row++) {
    const kind = await kindOf(page, row);
    await selectCell(page, row, 'sample');
    await expect(panel).toHaveAttribute('data-cell-key', `${tabId}:${row}:sample`);
    await expect(panel, `row ${row} (kind=${kind})`).toHaveAttribute(
      'data-detected',
      expectedFormatFor(kind),
    );
  }

  // --- scenario 3: type-driven detection --------------------------------------------------
  const nestedRow = await findRow(page, NESTED_PATH);
  await nestedRow.dblclick();
  await expect(page.locator('[data-testid="grid-header-cell"][data-column="data"]')).toBeVisible();
  await selectCell(page, 0, 'data');
  await expect(panel).toHaveAttribute('data-detected', 'json');

  const wideRow = await findRow(page, WIDE_PATH);
  await wideRow.dblclick();
  await scrollColumnIntoView(page, 'uuid_a');
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="uuid_a"]'),
  ).toBeVisible();
  await selectCell(page, 0, 'uuid_a');
  // P42 D23: uuid is gone as a format — a real UUID column's value now detects as text, the
  // same as it would for any string type_class column with no other distinguishing shape.
  await expect(panel).toHaveAttribute('data-detected', 'text');
  await scrollColumnIntoView(page, 'ts_a');
  await selectCell(page, 0, 'ts_a');
  await expect(panel).toHaveAttribute('data-detected', 'iso8601');
  // Task #72/#77: an iso8601-detected value gets its own row (under the header, local first)
  // reading out the UTC and local-timezone translation of the timestamp, with letter month
  // abbreviations (e.g. JAN) rather than numeric ones.
  await expect(page.locator('[data-testid="cell-editor-timestamp-utc"]')).toContainText('UTC');
  await expect(page.locator('[data-testid="cell-editor-timestamp-utc"]')).toContainText(
    /[A-Z]{3} \d{2} \d{4}/,
  );
  await expect(page.locator('[data-testid="cell-editor-timestamp-local"]')).not.toContainText(
    'UTC',
  );
  await scrollColumnIntoView(page, 'bytea_a');
  await selectCell(page, 0, 'bytea_a');
  await expect(panel).toHaveAttribute('data-detected', 'hex');
  await expect(page.locator('[data-testid="cell-editor-status"]')).toContainText('bytes');
  await scrollColumnIntoView(page, 'int_a');
  await selectCell(page, 0, 'int_a');
  await expect(panel).toHaveAttribute('data-detected', 'text');

  // --- back to formats (reactivates the original tab: same connectionId+path, D-tabs) ------
  await formatsRow.dblclick();
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="sample"]'),
  ).toBeVisible();
  await expect(page.locator(`[data-testid="tab"][data-tab-id="${tabId}"]`)).toHaveAttribute(
    'data-active',
    'true',
  );

  // --- scenario 4: beautify ---------------------------------------------------------------
  const jsonRow = 0; // formats row 0 is the 'json' sample (fixture insertion order)
  expect(await kindOf(page, jsonRow)).toBe('json');
  await selectCell(page, jsonRow, 'sample');
  await expect(panel).toHaveAttribute('data-detected', 'json');
  const storedJson = await cellText(page, jsonRow, 'sample');

  await page.click('[data-testid="cell-editor-beautify-indented"]');
  await expect(panel).toHaveAttribute('data-formatted', 'indented');
  expect((await editorText(page)).split('\n').length).toBeGreaterThan(1);
  await expect(page.locator('[data-testid="cell-editor-beautify-reset"]')).toBeEnabled();

  await page.click('[data-testid="cell-editor-beautify-compact"]');
  await expect(panel).toHaveAttribute('data-formatted', 'compact');
  expect((await editorText(page)).split('\n').length).toBe(1);

  await page.click('[data-testid="cell-editor-beautify-reset"]');
  await expect(panel).toHaveAttribute('data-formatted', 'none');
  expect(await editorText(page)).toBe(storedJson);
  await expect(page.locator('[data-testid="cell-editor-beautify-reset"]')).toBeDisabled();

  // The 'text' row has no lossless formatter: both buttons disabled.
  let textRowIndex = -1;
  for (let row = 0; row < 13; row++) {
    if ((await kindOf(page, row)) === 'text') {
      textRowIndex = row;
      break;
    }
  }
  expect(textRowIndex).toBeGreaterThanOrEqual(0);
  await selectCell(page, textRowIndex, 'sample');
  await expect(page.locator('[data-testid="cell-editor-beautify-indented"]')).toBeDisabled();
  await expect(page.locator('[data-testid="cell-editor-beautify-compact"]')).toBeDisabled();

  // --- scenario 5: lossless numbers --------------------------------------------------------
  await selectCell(page, jsonRow, 'sample');
  await page.click('[data-testid="cell-editor-beautify-indented"]');
  expect(await editorText(page)).toContain('12345678901234567890');
  await page.click('[data-testid="cell-editor-beautify-compact"]');
  expect(await editorText(page)).toContain('12345678901234567890');
  await page.click('[data-testid="cell-editor-beautify-reset"]');

  // --- scenario 6: manual override sticks per column, for the session --------------------
  await selectCell(page, jsonRow, 'sample');
  await selectFormat(page, 'text');
  await expect(panel).toHaveAttribute('data-format', 'text');

  for (const row of [1, 2]) {
    await selectCell(page, row, 'sample');
    await expect(panel).toHaveAttribute('data-format', 'text');
    await expect(panel).toHaveAttribute(
      'data-detected',
      expectedFormatFor(await kindOf(page, row)),
    );
  }

  await openRowMenu(page, FORMATS_PATH);
  await page.click('[data-testid="menu-item-open-data-new-tab"]');
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="sample"]'),
  ).toBeVisible();
  const secondTabId = (await page
    .locator('[data-testid="tab"][data-active="true"]')
    .getAttribute('data-tab-id')) as string;
  expect(secondTabId).not.toBe(tabId);

  await selectCell(page, jsonRow, 'sample');
  await expect(panel).toHaveAttribute('data-format', 'text'); // sticks across tabs, same (conn, path, column)

  // A different column on the same row is unaffected by the override: its format still tracks
  // its own auto-detection rather than the 'sample' column's override. Asserting format equals
  // detected (rather than merely "not text") is load-bearing — 'kind' holds a plain word like
  // "json" that itself auto-detects as 'text', so a leaked override would be indistinguishable
  // from correct behavior under a bare `not.toHaveAttribute('data-format', 'text')` check.
  await selectCell(page, jsonRow, 'kind');
  const [kindFormat, kindDetected] = await Promise.all([
    panel.getAttribute('data-format'),
    panel.getAttribute('data-detected'),
  ]);
  expect(kindFormat).toBe(kindDetected);

  await selectCell(page, jsonRow, 'sample');
  await selectFormat(page, 'auto');
  await expect(panel).toHaveAttribute('data-format', 'json');

  // --- scenario 6b: the format picker validates for real, and explains itself (D26/D28) ---
  const invalidChip = page.locator('[data-testid="cell-editor-invalid"]');
  await expect(invalidChip).toHaveCount(0);

  // An explicit override to 'json' (not auto-detection, which would just re-classify broken
  // JSON as plain text) makes a hand-typed syntax error surface as "broken JSON, invalid at
  // offset N" rather than silently falling back to a format with no opinion on validity.
  await selectFormat(page, 'json');
  await page.locator('[data-testid="cell-editor-panel"] .view-lines').click();
  await page.keyboard.press(SELECT_ALL);
  await page.keyboard.type('{"a":}');
  await expect(invalidChip).toBeVisible();
  await invalidChip.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    /Broken JSON, invalid at offset \d+/,
  );

  await page.click('[data-testid="cell-editor-beautify-reset"]');
  await expect(invalidChip).toHaveCount(0);

  // A plain-text value overridden to iso8601 is exactly the "my timestamp is wrong" case —
  // no edit needed, the stored value alone is not a parseable timestamp.
  await selectCell(page, textRowIndex, 'sample');
  await selectFormat(page, 'iso8601');
  await expect(panel).toHaveAttribute('data-format', 'iso8601');
  await expect(invalidChip).toBeVisible();
  await invalidChip.hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    'Not a valid timestamp for this format.',
  );

  // Every row in the picker explains itself on hover, reading from the same FORMAT_HELP
  // the trigger's own tooltip uses. ContextMenu.vue's own hint moved off the old
  // data-kira-tip DOM-scan onto a real Tooltip/TooltipContent per item, so the row must now
  // be hovered to render its content — same pattern as fake-data.spec.ts/http-raw.spec.ts's
  // own disabled-control tooltip assertions.
  await page.click('[data-testid="cell-editor-format"]');
  await page.locator('[data-testid="menu-item-format-iso8601"]').hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    'ISO 8601 date and time.',
  );
  await page.locator('[data-testid="menu-item-format-json"]').hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    'A JSON document. Objects and arrays get syntax highlighting and can be beautified.',
  );
  // The open tooltip's own popper content can still overlap the next menu item and intercept its
  // click — move off both trigger and content and let it close before clicking Auto.
  await page.mouse.move(0, 0);
  await expect(page.locator('[data-slot="tooltip-content"]')).toHaveCount(0);
  await page.click('[data-testid="menu-item-format-auto"]');
  await expect(panel).toHaveAttribute('data-format', 'text');
  await expect(invalidChip).toHaveCount(0);

  // --- scenario 7: NULL, empty, truncated -------------------------------------------------
  const nullsRow = await findRow(page, NULLS_PATH);
  await nullsRow.dblclick();
  await expect(page.locator('[data-testid="grid-header-cell"][data-column="label"]')).toBeVisible();

  await selectCell(page, 0, 'label');
  await expect(page.locator('[data-testid="cell-editor-badge-null"]')).toBeVisible();
  await expect(page.locator('[data-testid="cell-editor-format"]')).toBeDisabled();

  await selectCell(page, 1, 'label');
  await expect(page.locator('[data-testid="cell-editor-badge-empty"]')).toBeVisible();

  await scrollColumnIntoView(page, 'big_text', 3);
  await selectCell(page, 3, 'big_text');
  await expect(page.locator('[data-testid="cell-editor-badge-truncated"]')).toBeVisible();
  await expect(page.locator('[data-testid="cell-editor-status"]')).toContainText('64.0 KB');
  // P24 D27: a truncated value is read-only — see the sibling test's dedicated scenario for the
  // full behaviour (both editors refuse it, the value stays readable).
  await expect(panel).toHaveAttribute('data-read-only-reason', 'value-truncated');

  // Captured here, not at the very top: scenarios 1-7 legitimately open several new tables
  // (nested_json, wide_table, nulls_and_unicode), each a genuine read — the zero-ops invariant
  // below is about the cell editor's OWN interactions (selection, beautify, override), exercised
  // from here on with no further table opens on this connection.
  const opsBaseline = {
    control: controlOpsFor(control.log(), CONNECTION_ID),
    stream: streamOpsFor(await stream.ops(), CONNECTION_ID),
  };

  // --- scenario 8: selection semantics -----------------------------------------------------
  // Switch back to the original formats tab explicitly (by id) rather than dblclick, since two
  // tabs on (connection, FORMATS_PATH) exist after scenario 6 and dblclick would activate
  // whichever the tree menu resolves to first — the tab strip is unambiguous.
  await page.locator(`[data-testid="tab"][data-tab-id="${tabId}"]`).click();
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="sample"]'),
  ).toBeVisible();

  await selectCell(page, 0, 'sample');
  await panel.waitFor();
  // selectionTarget() now treats ANY genuine multi-cell range as having no single value
  // to show — same-column shift-click included — the same as a row/column selection, rather than
  // resolving to its focus end. Only a degenerate one-cell range (the click landing back on the
  // already-selected cell) still counts as "one cell selected".
  await gridCell(page, 2, 'sample').click({
    modifiers: ['Shift'],
  });
  await expect(page.locator('[data-testid="cell-editor"]')).toBeHidden();

  // A row selection carries no single cell to show either — the panel auto-hides entirely rather
  // than staying mounted with a "no cell selected" placeholder (no manual toggle to have pinned it
  // open).
  await page.locator('[data-testid="grid-gutter-cell"]').first().click();
  await expect(page.locator('[data-testid="cell-editor"]')).toBeHidden();

  await selectCell(page, 0, 'sample');
  await panel.waitFor();
  // The header's column-select trigger is a narrow strip at the header cell's left edge
  // (DataGrid.vue's `.header-select-zone`) — distinct from the sort target (the label/chevron,
  // covering the rest of the cell).
  await page
    .locator('[data-testid="grid-header-cell"][data-column="kind"] .header-select-zone')
    .click();
  await expect(page.locator('[data-testid="cell-editor"]')).toBeHidden();

  // --- scenario 9: an editable cell is genuinely editable, and blurring the editor auto-stages
  // into the SAME pending-change set the grid's own inline (double-click) edit and the toolbar's
  // Commit/Discard already operate on (the stage/preview/commit model — nothing here writes to
  // the server directly). `formats` has a primary key and this connection is writable, so this
  // cell carries no read-only-reason at all. There is no separate Save button — leaving the
  // editor (or Ctrl+Enter, without needing to leave it) is the stage signal, mirroring
  // DataGrid.vue's own inline double-click edit, which stages on blur too.
  await selectCell(page, 0, 'sample');
  await panel.waitFor();
  await expect(panel).not.toHaveAttribute('data-read-only-reason');
  await expect(page.locator('[data-testid="cell-editor-save"]')).toHaveCount(0);

  const beforeType = await editorText(page);
  await page.locator('[data-testid="cell-editor-panel"] .view-lines').click();
  await page.keyboard.press(SELECT_ALL);
  await page.keyboard.type('"edited from the cell editor"');
  expect(await editorText(page)).not.toBe(beforeType);

  // Blur by moving focus to the format select, still inside the panel but outside the editor.
  await page.locator('[data-testid="cell-editor-format"]').focus();
  await expect(gridCell(page, 0, 'sample')).toHaveClass(/pending-edit/);
  expect(await cellText(page, 0, 'sample')).toBe('"edited from the cell editor"');

  // Discard before moving on: scenario 10 below asserts zero DB operations for everything the
  // cell editor itself did, and a lingering pending edit has no business surviving into the
  // read-only-connection scenario that follows.
  await page.click('[data-testid="toolbar-discard-changes"]');
  await expect(gridCell(page, 0, 'sample')).not.toHaveClass(/pending-edit/);

  // Bug fix: clicking Revert straight from the editor (no deliberate blur first) used to *commit*
  // the edit instead of discarding it — the click moves focus off the editor, which fires the
  // same blur that auto-stages, and that firing races ahead of the click handler that's supposed
  // to undo it. resetBuffer() now un-stages via SelectedCell.onRevert regardless of that race, so
  // the pending edit must be gone, not just the on-screen text.
  const originalSample = await cellText(page, 0, 'sample');
  await page.locator('[data-testid="cell-editor-panel"] .view-lines').click();
  await page.keyboard.press(SELECT_ALL);
  await page.keyboard.type('"edited then reverted"');
  await page.click('[data-testid="cell-editor-beautify-reset"]');
  expect(await editorText(page)).toBe(originalSample);
  await expect(gridCell(page, 0, 'sample')).not.toHaveClass(/pending-edit/);
  expect(await cellText(page, 0, 'sample')).toBe(originalSample);
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0);

  // Ctrl+Enter stages immediately, without needing to blur.
  await page.locator('[data-testid="cell-editor-panel"] .view-lines').click();
  await page.keyboard.press(SELECT_ALL);
  await page.keyboard.type('"edited via ctrl-enter"');
  await page.keyboard.press(await primaryChord(page, 'Enter'));
  await expect(gridCell(page, 0, 'sample')).toHaveClass(/pending-edit/);
  expect(await cellText(page, 0, 'sample')).toBe('"edited via ctrl-enter"');

  // Show changes diffs the buffer against the stored value, also after reselecting the staged cell
  // (the base then comes from SelectedCell.original).
  const diffToggle = page.locator('[data-testid="cell-editor-diff-toggle"]');
  const diffPane = page.locator('[data-testid="cell-editor-diff"]');
  const expectStagedDiff = async (): Promise<void> => {
    await expect(diffToggle).toBeEnabled();
    await diffToggle.click();
    await expect(diffPane).toBeVisible();
    await expect(page.locator('[data-testid="cell-editor-encoded"]')).toBeHidden();
    const sides = await diffEditorText(diffPane);
    expect(sides.modified.trimEnd()).toBe('"edited via ctrl-enter"');
    expect(JSON.parse(sides.original)).toEqual(JSON.parse(originalSample));
    await diffToggle.click();
    await expect(diffPane).toBeHidden();
    await expect(page.locator('[data-testid="cell-editor-encoded"]')).toBeVisible();
  };
  await expectStagedDiff();
  await selectCell(page, 1, 'sample');
  await selectCell(page, 0, 'sample');
  expect((await editorText(page)).trimEnd()).toBe('"edited via ctrl-enter"');
  await expectStagedDiff();

  await page.click('[data-testid="toolbar-discard-changes"]');
  await expect(diffToggle).toBeDisabled();
  expect(await editorText(page)).toBe(originalSample);

  // Collapse connection 1's root before creating connection 2 — every descendant row (database,
  // schema, table) shares the exact same `data-path` values connection 2 will render, and
  // findRow()/expandRow() match on `data-path` alone with no per-connection scoping (mirrors
  // mutations.spec.ts's own read-only scenario, which hits the identical ambiguity).
  const firstConnRow = connectionRow(page);
  await expect(firstConnRow).toHaveCount(1);
  await firstConnRow.locator('[data-testid="tree-twisty"]').click();

  await fillConnectionDialog(page, { name: 'Cell Editor DB (RO)', color: 'red', readOnly: true });
  const roConnRow = connectionRow(page, 'Cell Editor DB (RO)');
  await expect(roConnRow).toBeVisible();
  await roConnRow.click({ button: 'right' });
  await page.click('[data-testid="menu-item-connect"]');
  await expect(roConnRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await roConnRow.locator('[data-testid="tree-twisty"]').click();
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  const roFormatsRow = await findRow(page, FORMATS_PATH);
  await roFormatsRow.dblclick();
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="sample"]'),
  ).toBeVisible();
  await selectCell(page, 0, 'sample');
  await panel.waitFor();
  await expect(panel).toHaveAttribute('data-read-only-reason', 'connection-read-only');

  // Back to the original (writable) tab for the remaining scenarios.
  await page.locator(`[data-testid="tab"][data-tab-id="${tabId}"]`).click();
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="sample"]'),
  ).toBeVisible();

  // --- scenario 10: zero operations ---------------------------------------------------------
  const opsAfter = {
    control: controlOpsFor(control.log(), CONNECTION_ID),
    stream: streamOpsFor(await stream.ops(), CONNECTION_ID),
  };
  expect(opsAfter).toEqual(opsBaseline);

  // --- scenario 11: the editor instance is reused across cells, never re-created --------------
  // This was a `Date.now()` budget standing in for "someone re-creates the EditorView per
  // cell". MonacoHost creates its editor once in onMounted and disposes it in
  // onUnmounted, so a re-create means a new `.monaco-editor` subtree — the marker below cannot
  // survive one.
  const monacoRoot = panel.locator('[data-testid="cell-editor-encoded"] .monaco-editor');
  await monacoRoot.evaluate((el) => el.setAttribute('data-kira-p81-instance', 'pinned'));
  await selectCell(page, 3, 'sample');
  await expect
    .poll(async () => (await panel.getAttribute('data-cell-key')) ?? '')
    .toBe(`${tabId}:3:sample`);
  await expect(
    panel.locator(
      '[data-testid="cell-editor-encoded"] .monaco-editor[data-kira-p81-instance="pinned"]',
    ),
  ).toHaveCount(1);
  // Reused *and* repopulated — the opposite failure mode a bare identity check would miss.
  // Scoped to the encoded pane, not the whole panel: row 3 is the base64 fixture row, which opens
  // a second MonacoHost (the decoded pane) alongside it, so the panel-wide `editorText(page)`
  // helper is ambiguous here (two `.monaco-host` mounts) even though the app is behaving correctly.
  expect(await sharedEditorText(panel.locator('[data-testid="cell-editor-encoded"]'))).toBe(
    await cellText(page, 3, 'sample'),
  );

  // --- scenario 12: visibility follows selection — no manual toggle exists anymore ------------
  await page.locator('[data-testid="grid-gutter-cell"]').first().click();
  await expect(page.locator('[data-testid="cell-editor"]')).toBeHidden();
  await selectCell(page, 3, 'sample');
  await expect(page.locator('[data-testid="cell-editor"]')).toBeVisible();
  await expect(panel).toHaveAttribute('data-cell-key', `${tabId}:3:sample`);
  // The original scenario also relaunched here to prove cell selection doesn't survive a
  // restart (D3/D12) — dropped, see this file's header comment: this tier's `relaunch()` opens a
  // brand-new browser context every time, so that half is true by construction, not a real check.

  expect(consoleErrors).toEqual([]);
});

// ================================================================================================
// Test 2
// ================================================================================================

const CONNECTION_ID_2 = 'conn-cell-editor-actions';
const CONNECTION_SUMMARY_2 = postgresConnectionSummary(
  CONNECTION_ID_2,
  'Format Actions DB',
  'blue',
);

const CONTROL_2: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Format Actions DB', 'blue'),
    response: CONNECTION_SUMMARY_2,
  },
  ...connectAndExpandControl(CONNECTION_ID_2),
];

const PORT_2: PortSnapshot[] = tableReadsFor(CONNECTION_ID_2, [
  FORMATS_PATH,
  WIDE_PATH,
  NULLS_PATH,
]);

test('cell editor — UUID generate, timestamp translate pane, hex/base64 decoded pane', async ({
  relaunch,
  consoleErrors,
}) => {
  // One long scenario (~30s idle); the 60s default leaves no headroom under CPU contention.
  test.setTimeout(120_000);
  const { window: page } = await relaunch({ control: CONTROL_2, stream: PORT_2 });
  await connectAndExpand(page, { name: 'Format Actions DB', color: 'blue' });

  const formatsRow = await findRow(page, FORMATS_PATH);
  await formatsRow.dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="sample"]'),
  ).toBeVisible();
  const panel = page.locator('[data-testid="cell-editor-panel"]');
  const encoded = page.locator('[data-testid="cell-editor-encoded"]');
  const encodedText = () => sharedEditorText(encoded);

  // Fixture row order (0001_seed.sql's own INSERT order, already relied on by the sibling test's
  // scenario 2 loop): 3=base64, 4=hex, 5=epochSeconds.
  // The old UUID-only detected format (and the generate button it gated) is gone —
  // row 8's former 'uuid' sample now detects as plain text, exercised by the sibling test's own
  // scenario 3 regression check.

  // --- generators panel (D29/D30/D31): never format-gated, unlike the button it replaces -------
  let plainTextRow = -1;
  for (let row = 0; row < 13; row++) {
    if ((await kindOf(page, row)) === 'text') {
      plainTextRow = row;
      break;
    }
  }
  expect(plainTextRow).toBeGreaterThanOrEqual(0);
  await selectCell(page, plainTextRow, 'sample');
  await panel.waitFor();
  await expect(panel).toHaveAttribute('data-detected', 'text');

  // Proves the gate is gone (F26): this cell's format is plain text, not uuid, and the trigger
  // is enabled anyway.
  const generateTrigger = page.locator('[data-testid="cell-editor-generate"]');
  await expect(generateTrigger).toBeEnabled();
  const plainTextCell = gridCell(page, plainTextRow, 'sample');

  await generateTrigger.click();
  await page.click('[data-testid="cell-editor-generate-uuid"]');
  expect(await editorText(page)).toMatch(
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
  );
  // One-shot, like Ctrl+Enter — stages immediately, with no blur needed first.
  await expect(plainTextCell).toHaveClass(/pending-edit/);
  await page.click('[data-testid="toolbar-discard-changes"]');

  await generateTrigger.click();
  await page.click('[data-testid="cell-editor-generate-ulid"]');
  expect(await editorText(page)).toMatch(/^[0-9A-Z]{26}$/);
  await page.click('[data-testid="toolbar-discard-changes"]');

  await generateTrigger.click();
  await page.click('[data-testid="cell-editor-generate-token"]');
  expect(await editorText(page)).toMatch(/^[0-9a-f]{32}$/);
  await page.click('[data-testid="toolbar-discard-changes"]');

  // "Now" is format-aware — this cell's effective format is 'text' (no override), so it spells an
  // ISO-8601 timestamp rather than an epoch count.
  await generateTrigger.click();
  await page.click('[data-testid="cell-editor-generate-now"]');
  expect(await editorText(page)).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/);
  await page.click('[data-testid="toolbar-discard-changes"]');

  // Exactly one byte figure (D31): EditBufferActions' own badge is gone from this mount, the
  // status badge above still carries it.
  await expect(page.locator('[data-testid="cell-editor-byte-badge"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="cell-editor-status"]')).toContainText(
    /\d+( B|\.\d+ (KB|MB))/,
  );

  // --- timestamp translate pane (P24 D14/D15/D18/D19) --------------------------------------
  await selectCell(page, 5, 'sample'); // epochSeconds row
  await panel.waitFor();
  await expect(panel).toHaveAttribute('data-detected', 'epochSeconds');
  const tsField = page.locator('[data-testid="cell-editor-timestamp-field"]');
  await expect(tsField).toBeVisible();
  // D18: no native system chrome anywhere in the panel — the app-owned calendar replaces it.
  await expect(page.locator('input[type="datetime-local"]')).toHaveCount(0);

  // D15: live and bidirectional — typing in the field updates the encoded box on every
  // keystroke, with no blur.
  await tsField.fill('2030-06-15 12:30:15');
  await expect.poll(encodedText).toMatch(/^\d+$/);
  const pickedEpoch = Number(await encodedText());
  // Round-trips through the reading row (local/UTC), not just a raw number — proves the field
  // and describeTimestamp agree on the same moment.
  await expect(page.locator('[data-testid="cell-editor-timestamp-utc"]')).toContainText('2030');
  expect(Math.abs(pickedEpoch - Date.UTC(2030, 5, 15, 12, 30, 15) / 1000)).toBeLessThan(24 * 3600);
  // Nothing staged yet — the field alone doesn't blur the editor.
  await expect(gridCell(page, 5, 'sample')).not.toHaveClass(/pending-edit/);
  await page.locator('[data-testid="cell-editor-format"]').focus();
  await expect(gridCell(page, 5, 'sample')).toHaveClass(/pending-edit/);
  await page.click('[data-testid="toolbar-discard-changes"]');

  // No translate pane for a non-timestamp format.
  await selectCell(page, 0, 'sample');
  await panel.waitFor();
  await expect(page.locator('[data-testid="cell-editor-timestamp-field"]')).toHaveCount(0);

  // --- shape preservation: wide_table.ts_a is a real Postgres timestamptz defaulted to
  // now(), so its text comes back in Postgres's own space+microseconds+offset shape, not a
  // literal this fixture chose. Editing only the hour through the field must change only the
  // hour digits, leaving the separator/offset/fractional precision exactly as they were. -----
  const wideRow2 = await findRow(page, WIDE_PATH);
  await wideRow2.dblclick();
  await scrollColumnIntoView(page, 'ts_a');
  await selectCell(page, 0, 'ts_a');
  await panel.waitFor();
  await expect(panel).toHaveAttribute('data-detected', 'iso8601');
  const originalTs = await encodedText();
  expect(originalTs).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(\.\d+)?([+-]\d{2}(:\d{2})?|Z)$/);
  const tsFieldValue = await page
    .locator('[data-testid="cell-editor-timestamp-field"]')
    .inputValue();
  const [datePart, timePart] = tsFieldValue.split(' ');
  const [hh, mm, ss] = (timePart ?? '').split(':');
  const bumpedHour = String((Number(hh) + 1) % 24).padStart(2, '0');
  await page
    .locator('[data-testid="cell-editor-timestamp-field"]')
    .fill(`${datePart} ${bumpedHour}:${mm}:${ss}`);
  const afterHourEdit = await encodedText();
  expect(afterHourEdit).not.toBe(originalTs);
  const suffixFrom = (s: string) => s.replace(/^\d{4}-\d{2}-\d{2}[ T]\d{2}/, '');
  expect(suffixFrom(afterHourEdit)).toBe(suffixFrom(originalTs)); // minute/sec/fraction/offset unchanged
  expect(afterHourEdit[10]).toBe(originalTs[10]); // the separator itself (space, not 'T')

  // Editing the encoded box back updates the field, again with no blur (D15, the reverse
  // direction).
  await encoded.locator('.view-lines').click();
  await page.keyboard.press(SELECT_ALL);
  await page.keyboard.type(originalTs);
  await expect(page.locator('[data-testid="cell-editor-timestamp-field"]')).toHaveValue(
    tsFieldValue,
  );
  await expect(page.locator('[data-testid="cell-editor-timestamp-relative"]')).not.toHaveText('');
  await expect(gridCell(page, 0, 'ts_a')).not.toHaveClass(/pending-edit/); // exploring/typing-then-reverting stages nothing

  // --- zone switch preserves the value (D19): toggling Local -> UTC -> Local must leave the
  // encoded buffer byte-identical. -----------------------------------------------------------
  const docBeforeZoneToggle = await encodedText();
  await page.click('[data-testid="cell-editor-timestamp-zone-utc"]');
  await page.click('[data-testid="cell-editor-timestamp-zone-local"]');
  expect(await encodedText()).toBe(docBeforeZoneToggle);

  // --- the calendar is app-owned (D18), and exploring it stages nothing (D15) ---------------
  await page.click('[data-testid="cell-editor-timestamp-calendar"]');
  const calendarPopover = page.locator('[data-testid="cell-editor-timestamp-calendar-popover"]');
  // P104: reka Popover's own content class replaces the old primitive's p-float marker —
  // toBeVisible() already confirms it renders positioned.
  await expect(calendarPopover).toBeVisible();
  await page.click('[data-testid="datetime-picker-next-month"]');
  await page.keyboard.press('Escape');
  await expect(calendarPopover).toHaveCount(0);
  await expect(gridCell(page, 0, 'ts_a')).not.toHaveClass(/pending-edit/);

  // --- month/year jump navigation (D33a/D33b): the label cycles days -> months -> years;
  // picking a month or year moves only the *view* — never stages anything, same as the
  // prev/next-month paging just exercised above. --------------------------------------------
  await page.click('[data-testid="cell-editor-timestamp-calendar"]');
  await expect(calendarPopover).toBeVisible();
  const dtpMode = page.locator('[data-testid="datetime-picker-mode"]');
  const monthLabel = page.locator('[data-testid="datetime-picker-month"]');
  await expect(dtpMode).toHaveAttribute('data-mode', 'days');

  await monthLabel.click(); // days -> months
  await expect(dtpMode).toHaveAttribute('data-mode', 'months');
  await monthLabel.click(); // months -> years
  await expect(dtpMode).toHaveAttribute('data-mode', 'years');

  const yearCell = page.locator('[data-testid="datetime-picker-year-cell"]').first();
  const pickedYear = await yearCell.innerText();
  await yearCell.click(); // picking a year returns to months
  await expect(dtpMode).toHaveAttribute('data-mode', 'months');

  const monthCell = page.locator('[data-testid="datetime-picker-month-cell"]').first();
  const pickedMonth = await monthCell.innerText();
  await monthCell.click(); // picking a month returns to days
  await expect(dtpMode).toHaveAttribute('data-mode', 'days');
  await expect(monthLabel).toContainText(pickedMonth);
  await expect(monthLabel).toContainText(pickedYear);

  // Nothing staged — only the view moved, exactly like the plain prev/next paging above.
  await expect(gridCell(page, 0, 'ts_a')).not.toHaveClass(/pending-edit/);
  await page.keyboard.press('Escape');
  await expect(calendarPopover).toHaveCount(0);

  // Picking a day and blurring stages exactly one pending edit.
  await page.click('[data-testid="cell-editor-timestamp-calendar"]');
  await page
    .locator(
      '[data-testid="datetime-picker-day"][data-in-month="true"]:not([data-selected="true"])',
    )
    .first()
    .click();
  // Picking a day doesn't itself close the popover (only Escape/click-outside does, D18) — close
  // it before interacting with anything else, same as the explore-only case just above, otherwise
  // its full-viewport backdrop keeps intercepting every later click in this scenario.
  await page.keyboard.press('Escape');
  await page.locator('[data-testid="cell-editor-format"]').focus();
  await expect(gridCell(page, 0, 'ts_a')).toHaveClass(/pending-edit/);
  await page.click('[data-testid="toolbar-discard-changes"]');

  // Back to the formats table for the remaining scenarios.
  await formatsRow.dblclick();
  await expect(
    page.locator('[data-testid="grid-header-cell"][data-column="sample"]'),
  ).toBeVisible();

  // --- hex/base64 decoded pane — editing the plaintext re-encodes the raw box, and vice versa ---
  await selectCell(page, 3, 'sample'); // base64 row: "Hello, World!"
  await panel.waitFor();
  await expect(panel).toHaveAttribute('data-detected', 'base64');
  const decoded = page.locator('[data-testid="cell-editor-decoded"]');
  const decodedText = () => sharedEditorText(decoded);
  await expect.poll(decodedText).toContain('Hello, World!');

  await decoded.locator('.view-lines').click();
  await page.keyboard.press(SELECT_ALL);
  await page.keyboard.type('Goodbye!');
  await expect.poll(encodedText).toBe(btoa('Goodbye!'));
  // Blurring stages the re-encoded value, not the plaintext — the grid must show base64.
  await page.locator('[data-testid="cell-editor-format"]').focus();
  await expect(gridCell(page, 3, 'sample')).toHaveClass(/pending-edit/);
  expect(await cellText(page, 3, 'sample')).toBe(btoa('Goodbye!'));
  await page.click('[data-testid="toolbar-discard-changes"]');

  // The decoded pane's loop guard (D20/F7b): retyping *identical* plaintext is a no-op write —
  // it must not leave the guard permanently armed and silently swallow the next genuine edit.
  await decoded.locator('.view-lines').click();
  await page.keyboard.press(SELECT_ALL);
  await page.keyboard.type('Hello, World!'); // re-encodes to the exact same base64 already there
  await encoded.locator('.view-lines').click();
  await page.keyboard.press(SELECT_ALL);
  await page.keyboard.type(btoa('Second edit'));
  await expect.poll(decodedText).toContain('Second edit');
  await page.locator('[data-testid="cell-editor-format"]').focus();
  await page.click('[data-testid="toolbar-discard-changes"]');

  // hex row: raw bytes 0xcafebabedeadbeef are not valid UTF-8 — the decoded pane shows a note,
  // not garbled text, and offers no second editor to type into.
  await selectCell(page, 4, 'sample');
  await panel.waitFor();
  await expect(panel).toHaveAttribute('data-detected', 'hex');
  await expect(page.locator('[data-testid="cell-editor-decoded"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="cell-editor-decoded-empty"]')).toContainText(
    'aren’t valid text',
  );

  // No decoded pane for a non-binary format.
  await selectCell(page, 0, 'sample');
  await panel.waitFor();
  await expect(page.locator('[data-testid="cell-editor-decoded"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="cell-editor-decoded-empty"]')).toHaveCount(0);

  // --- beautify/reset buttons carry a tooltip once enabled, not just when disabled -------------
  await page.locator('[data-testid="cell-editor-beautify-indented"]').hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(/./);

  // --- modified chip + data-dirty (D25), Escape reverts (D26/F10) ------------------------------
  await selectCell(page, 0, 'sample'); // json row
  await panel.waitFor();
  await expect(page.locator('[data-testid="cell-editor-modified"]')).toHaveCount(0);
  await expect(panel).toHaveAttribute('data-dirty', 'false');
  const originalJson = await editorText(page);
  await page.locator('[data-testid="cell-editor-panel"] .view-lines').click();
  await page.keyboard.press(SELECT_ALL);
  await page.keyboard.type('"escape me"');
  await expect(page.locator('[data-testid="cell-editor-modified"]')).toBeVisible();
  await expect(panel).toHaveAttribute('data-dirty', 'true');
  // Reset's tooltip while enabled (D24/F7a) — the same bug already fixed for the beautify pair.
  await page.locator('[data-testid="cell-editor-beautify-reset"]').hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(/./);
  await page.locator('[data-testid="cell-editor-panel"] .view-lines').press('Escape');
  await expect(page.locator('[data-testid="cell-editor-modified"]')).toHaveCount(0);
  await expect(panel).toHaveAttribute('data-dirty', 'false');
  expect(await editorText(page)).toBe(originalJson);
  await expect(gridCell(page, 0, 'sample')).not.toHaveClass(/pending-edit/);

  // --- a truncated value refuses both editors (D27/F7f) -----------------------------------------
  const nullsRow2 = await findRow(page, NULLS_PATH);
  await nullsRow2.dblclick();
  await scrollColumnIntoView(page, 'big_text', 3);
  await selectCell(page, 3, 'big_text');
  await panel.waitFor();
  await expect(panel).toHaveAttribute('data-read-only-reason', 'value-truncated');
  // Monaco has no contenteditable surface at all (readOnly + domReadOnly instead sets `readonly`
  // on its own hidden `textarea.inputarea`, P60a §3.2) — the read-only signal moved there.
  await expect(
    page.locator('[data-testid="cell-editor-encoded"] textarea.inputarea'),
  ).toHaveAttribute('readonly', 'true');
  // The value stays fully readable — only writing it back is refused.
  expect(await sharedEditorText(page.locator('[data-testid="cell-editor-encoded"]'))).not.toBe('');
  await gridCell(page, 3, 'big_text').dblclick();
  await expect(page.locator('[data-testid="grid-cell-input"]')).toHaveCount(0);

  expect(consoleErrors).toEqual([]);
});
// ================================================================================================
// Test 3
// ================================================================================================

const CONNECTION_ID_3 = 'conn-cell-editor-ownership';
const CONNECTION_SUMMARY_3 = postgresConnectionSummary(CONNECTION_ID_3, 'Ownership DB', 'green');

const CONTROL_3: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Ownership DB', 'green'),
    response: CONNECTION_SUMMARY_3,
  },
  ...connectAndExpandControl(CONNECTION_ID_3),
  {
    channel: IPC.treeDefinition,
    args: { connectionId: CONNECTION_ID_3, path: FORMATS_PATH, refresh: false, tabId: null },
    response: {
      definition: { ...FORMATS_DEFINITION_WITHOUT_PATH, path: FORMATS_PATH },
      source: 'server',
    },
  },
];

const PORT_3: PortSnapshot[] = [
  ...tableReadsFor(CONNECTION_ID_3, [FORMATS_PATH]),
  {
    op: DATA_OP.execute,
    payload: { connectionId: CONNECTION_ID_3, path: FORMATS_PATH, statements: ['select 1 as x'] },
    response: {
      kind: 'execute',
      pages: [
        {
          kind: 'tabular',
          columns: SELECT_1_AS_X_COLUMNS,
          rows: SELECT_1_AS_X_ROWS,
          position: {
            offset: 0,
            pageSize: 1,
            hasMore: false,
            nextToken: null,
            prevToken: null,
            strategy: 'offset',
          },
          truncatedCells: 0,
        },
      ],
    },
  },
];

test('cell editor — timestamp controls stay in view at small dock height', async ({ relaunch }) => {
  test.setTimeout(120_000);
  const { window: page } = await relaunch({ control: CONTROL_2, stream: PORT_2 });
  await connectAndExpand(page, { name: 'Format Actions DB', color: 'blue' });
  await (await findRow(page, FORMATS_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();

  const dock = page.locator('[data-testid="cell-editor"]');
  const controls = ['field', 'calendar', 'zone'].map((id) =>
    page.locator(`[data-testid="cell-editor-timestamp-${id}"]`),
  );
  const height = async (testId: string) =>
    (await page.locator(`[data-testid="${testId}"]`).boundingBox())?.height ?? Number.NaN;
  const expectControlsInside = async () => {
    const dockBox = await dock.boundingBox();
    expect(dockBox).not.toBeNull();
    for (const control of controls) {
      await expect(control).toBeInViewport();
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect((box?.y ?? 0) + (box?.height ?? 0)).toBeLessThanOrEqual(
        (dockBox?.y ?? 0) + (dockBox?.height ?? 0) + 0.5,
      );
    }
  };

  await selectCell(page, 5, 'sample'); // epochSeconds row
  await expect(controls[0]).toBeVisible();
  await expectControlsInside();
  const paneHeight = await height('cell-editor-timestamp-pane');
  const encodedHeight = await height('cell-editor-encoded');

  // Find bar sits above the pane; 34 + 34 + 20 + 72 fits the default dock, not the 120 minimum.
  await page.click('[data-testid="cell-editor-search-toggle"]');
  await expect(page.locator('[data-testid="http-find-input"]')).toBeVisible();
  await expectControlsInside();
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="http-find-bar"]')).toHaveCount(0);

  await page.locator('.cell-splitter').focus();
  await page.keyboard.press('Home');
  await expect
    .poll(async () => (await dock.boundingBox())?.height ?? Number.NaN)
    .toBeCloseTo(120, 0);
  await expectControlsInside();
  expect(Math.abs((await height('cell-editor-timestamp-pane')) - paneHeight)).toBeLessThanOrEqual(
    1,
  );
  expect(await height('cell-editor-encoded')).toBeLessThan(encodedHeight);

  const insideDock = async (testId: string) => {
    const dockBox = await dock.boundingBox();
    const box = await page.locator(`[data-testid="${testId}"]`).boundingBox();
    expect(box?.height ?? -1).toBeGreaterThanOrEqual(0);
    expect((box?.y ?? 0) + (box?.height ?? 0)).toBeLessThanOrEqual(
      (dockBox?.y ?? 0) + (dockBox?.height ?? 0) + 0.5,
    );
  };
  await selectCell(page, 3, 'sample'); // base64 row
  await expect(page.locator('[data-testid="cell-editor-decoded"]')).toBeVisible();
  await insideDock('cell-editor-decoded');
  await selectCell(page, 4, 'sample'); // undecodable row
  await expect(page.locator('[data-testid="cell-editor-decoded-empty"]')).toBeVisible();
  await insideDock('cell-editor-decoded-empty');
});

// Contract layout-cell-editor. Backend half: appflow TestLayoutCellEditorHeight. The stored dock
// height is what the dock opens at, and a drag writes the new height back inside 120..480.
test('contract: the dock opens at the stored height and a drag persists the new one', async ({
  relaunch,
}) => {
  const layout = contract<{ panel: { cellEditor: { height: number } } }>(
    'layout-cell-editor',
    'LayoutService.GetAll#cellEditor',
  );
  const stored = layout.panel.cellEditor.height;
  const { window: page, control } = await relaunch({
    control: [...CONTROL_2, { channel: IPC.layoutGetAll, response: layout }],
    stream: PORT_2,
  });
  await connectAndExpand(page, { name: 'Format Actions DB', color: 'blue' });
  await (await findRow(page, FORMATS_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await selectCell(page, 5, 'sample');

  const dock = page.locator('[data-testid="cell-editor"]');
  const dockHeight = async () => (await dock.boundingBox())?.height ?? Number.NaN;
  await expect.poll(dockHeight).toBeCloseTo(stored, 0);

  const handle = await page.locator('.cell-splitter').boundingBox();
  if (!handle) throw new Error('no resize handle');
  const x = handle.x + handle.width / 2;
  const y = handle.y + handle.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x, y - 40, { steps: 5 });
  await page.mouse.up();
  await expect.poll(dockHeight).toBeCloseTo(stored + 40, 0);

  const written = () =>
    control
      .log()
      .filter((e) => e.channel === IPC.layoutSet)
      .map((e) => (e.args as { patch: { panel?: { cellEditor?: { height?: number } } } }).patch)
      .flatMap((p) =>
        p.panel?.cellEditor?.height === undefined ? [] : [p.panel.cellEditor.height],
      );
  await expect.poll(() => written().at(-1)).toBeGreaterThan(stored);
  for (const height of written()) {
    expect(height).toBeGreaterThanOrEqual(120);
    expect(height).toBeLessThanOrEqual(480);
  }
});

test("cell editor — owned by the view, never shows another tab's cell", async ({
  relaunch,
  consoleErrors,
}) => {
  const { window: page } = await relaunch({ control: CONTROL_3, stream: PORT_3 });
  await connectAndExpand(page, { name: 'Ownership DB', color: 'green' });

  const panel = page.locator('[data-testid="cell-editor-panel"]');

  // (1) Open a data tab and select a cell.
  const formatsRow = await findRow(page, FORMATS_PATH);
  await formatsRow.dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  await selectCell(page, 0, 'sample');
  await panel.waitFor();
  const dataTabId = (await panel.getAttribute('data-cell-key'))?.split(':')[0] ?? '';
  await expect(panel).toHaveAttribute('data-cell-key', `${dataTabId}:0:sample`);

  // A manual format override, set now so step (7) below can assert it survives the moves ahead
  // untouched (D13 — the override map is keyed by (connection, path, column), not by tab).
  await selectFormat(page, 'text');
  await expect(panel).toHaveAttribute('data-format', 'text');

  // (2) Ownership: the panel lives inside the tab's own view subtree, not beside it — this is
  // the assertion that fails against the pre-P26 shell-mounted singleton and is the structural
  // statement of the whole phase.
  const dock = page.locator('[data-testid="main-view"] [data-testid="cell-editor"]');
  await expect(dock).toHaveCount(1);
  await expect(dock).toHaveAttribute('data-tab-id', dataTabId);

  // (3) Open a console tab on the same connection, run a query, click a result cell —
  // ConsoleResultGrid publishes but neither republishes on mount nor clears on unmount (the exact
  // class of publisher that produced the reported bug).
  await openRowMenu(page, FORMATS_PATH);
  await page.click('[data-testid="menu-item-open-console"]');
  const consoleTabId =
    (await page
      .locator('[data-testid="tab"][data-tab-kind="console"]')
      .last()
      .getAttribute('data-tab-id')) ?? '';
  await page.locator('[data-testid="console-view"] .view-lines').click();
  await page.keyboard.type('select 1 as x;');
  await page.click('[data-testid="console-run-statement"]');
  await expect(page.locator('[data-testid="console-result-grid"]')).toBeVisible();
  await page.locator('[data-testid="console-result-cell"]').first().click();
  await expect(panel).toHaveAttribute('data-cell-key', new RegExp(`^${consoleTabId}:`));

  // (3b) P40 D11/D12/D13: the console's dock is a viewer, not an editor refusing this cell — no
  // reason chip (there was never a write on offer to refuse), and none of the edit-buffer
  // affordances that exist only to serve staging one.
  await expect(panel).toHaveAttribute('data-read-only', 'true');
  await expect(panel).not.toHaveAttribute('data-read-only-reason');
  await expect(panel.locator('[data-testid="cell-editor-generate"]')).toHaveCount(0);
  await expect(panel.locator('[data-testid="cell-editor-modified"]')).toHaveCount(0);
  await expect(panel.locator('[data-testid="cell-editor-byte-badge"]')).toHaveCount(0);
  await expect(panel.locator('[data-testid="cell-editor-beautify-indented"]')).toHaveCount(0);
  await expect(panel.locator('[data-testid="cell-editor-beautify-compact"]')).toHaveCount(0);
  await expect(panel.locator('[data-testid="cell-editor-beautify-reset"]')).toHaveCount(0);
  // Facts about the value, not ways to write it, still show.
  await expect(panel.locator('[data-testid="cell-editor-format"]')).toBeVisible();
  await expect(panel.locator('[data-testid="cell-editor-status"]')).toBeVisible();
  await expect(panel.locator('[data-testid="cell-editor-close"]')).toBeVisible();

  // (4) The bug: switch back to the data tab. The panel must show the data tab's cell again —
  // never the console tab's, whether by the grid's own republish-on-mount or by nothing
  // overwriting a per-tab record that no longer exists in a shared slot.
  await page.locator(`[data-testid="tab"][data-tab-id="${dataTabId}"]`).click();
  await expect(panel).toHaveAttribute('data-cell-key', `${dataTabId}:0:sample`);
  const dockHeight = await dock.evaluate((el) => el.getBoundingClientRect().height);

  // (5) Switch to the console tab again: its own cell is back too, unchanged (D6 — a
  // backgrounded tab keeps its selection instead of losing it).
  await page.locator(`[data-testid="tab"][data-tab-id="${consoleTabId}"]`).click();
  await expect(panel).toHaveAttribute('data-cell-key', new RegExp(`^${consoleTabId}:`));
  // The panel's height is a persisted global, not per-tab — it must not visually reset.
  // reka's SplitterPanel (size-unit="px") converts the shared layoutStore px value to a
  // percentage of each view's own SplitterGroup pixel height (reka-ui/Splitter/utils/units.ts),
  // so a sub-pixel rounding gap between two views with slightly different total heights is
  // expected, not a reset — a real reset would jump back to a whole different height, not <1px.
  const consoleDockHeight = await dock.evaluate((el) => el.getBoundingClientRect().height);
  expect(Math.abs(consoleDockHeight - dockHeight)).toBeLessThan(1);

  // (6) A definition tab mounts no dock at all — the minimal reproduction of the user's report
  // (switch from a cell-bearing tab straight into one that never had the panel in the first
  // place, and the panel used to keep showing the old tab's cell).
  await openRowMenu(page, FORMATS_PATH);
  await page.click('[data-testid="menu-item-open-definition"]');
  await expect(page.locator('[data-testid="definition-view"]')).toBeVisible();
  await expect(page.locator('[data-testid="cell-editor"]')).toHaveCount(0);

  // (7) Switch back to the data tab: the panel returns, and the format override set in step (1)
  // still applies (D13 — untouched by the move to per-view ownership).
  await page.locator(`[data-testid="tab"][data-tab-id="${dataTabId}"]`).click();
  await expect(panel).toHaveAttribute('data-cell-key', `${dataTabId}:0:sample`);
  await expect(panel).toHaveAttribute('data-format', 'text');

  // (8) Closing the tab frees its record — no stale panel, no console error. Close the console
  // and definition tabs first: closeTab() reactivates whichever tab now sits at the closed tab's
  // old index, and with the console tab still open that would be the console tab — which has its
  // own legitimate, still-selected cell (step 5) and would correctly show its own dock. That's a
  // different (already-covered) property than the one this step means to check, so leave only the
  // data tab open, where closing it drops the tab strip to zero and the dock unambiguously to 0.
  await page
    .locator('[data-testid="tab"][data-tab-kind="definition"] [data-testid="tab-close"]')
    .click();
  await page
    .locator(`[data-testid="tab"][data-tab-id="${consoleTabId}"] [data-testid="tab-close"]`)
    .click();
  await page
    .locator(`[data-testid="tab"][data-tab-id="${dataTabId}"] [data-testid="tab-close"]`)
    .click();
  await expect(page.locator('[data-testid="cell-editor"]')).toHaveCount(0);

  expect(consoleErrors).toEqual([]);
});
