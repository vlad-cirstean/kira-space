import type { ColumnDescriptor, TabularPage, TypeClass } from '@shared/protocol/page';
import { cellText, isNull } from '@shared/protocol/page';
import { t } from '@theme/i18n';
import { typeClassColor } from '../../../theme/icons';
import { typeDescription } from '../typeGlossary';

// Exported so SlickGridHost.vue/ConsoleSlickGrid.vue can clamp to the exact same floor a freshly
// measured column already gets — a stored/dragged width used to be able to undercut it (their own
// `minWidth: 40` was a different, lower number), which is how a column as short as "id" could
// persist at a one-character-wide render across a reload (minWidth only ever bounded an
// interactive drag, never a width read back from storage).
const MIN_WIDTH = 64;
// The floor a freshly *measured* column may not fall below, separate from MIN_WIDTH above.
// The two were one constant doing two different jobs: MIN_WIDTH is the interactive-resize floor (a
// deliberate narrow drag must stay possible, and a width read back from storage is a deliberate
// drag), while this is "how narrow may a column the user never touched be drawn". A column whose
// values are all short ("id", "qty", a boolean) measured to exactly 64px, of which ~20px is header
// chrome and 20px is CELL_PADDING — reported as columns rendering very narrow. 96px is the same
// figure DEFAULT_COLUMN_WIDTH below already uses for "nothing measurable at all".
const MIN_INITIAL_WIDTH = 96;
const MAX_WIDTH = 480;
const CELL_PADDING = 20; // px, both sides combined plus a little breathing room
const SAMPLE_ROWS = 50;
// The ceiling on headerAwareMinWidth below — without one, a column named
// `customer_subscription_renewal_reason` would become undraggable below ~280px. Under MAX_WIDTH's
// 480 and above 42-58px of header chrome plus ~20 characters of name, which covers the
// overwhelming majority of real column names; past it, ellipsising is the right answer and
// columnHeaderTooltip already carries the full name.
const MIN_WIDTH_CAP = 200;

// P48 F9: one constant behind what were three separate spellings of the same gutter width — the
// deleted DataGrid.vue's own, ConsoleResultGrid.vue's now-deleted tabular-branch copy (a bare `56`
// in its own CSS), and this one.
export const GUTTER_WIDTH = 56;
// P48 F9: the 96px fallback both grids' own widths computeds used when neither a stored width
// nor a measured one is available yet.
export const DEFAULT_COLUMN_WIDTH = 96;
// The pixel overscan budget the deleted DataGrid.vue's own column virtualizer used — kept for
// KiraSlickGrid's own column-overscan clamp (kiraSlickGrid.ts's clampColumnOverscan); the per-side
// column *count* cap it was paired with (MAX_OVERSCAN_COLUMNS) died with the tanstack-virtual
// column axis it belonged to — SlickGrid clamps by canvas width instead.
export const OVERSCAN_PX = 560;

// P16 D4: a small Map keyed by size token, generalized from one memoized context — the header
// (--kira-t-sm, 11px) and the body sample (--kira-font-size/--kira-t-md, 12px) are measured at
// their own actual render size rather than over-reserving every column by the larger of the two.
let measureCtxBySize = new Map<string, CanvasRenderingContext2D>();

function getMeasureCtx(sizeToken = '--kira-font-size'): CanvasRenderingContext2D {
  const cached = measureCtxBySize.get(sizeToken);
  if (cached) return cached;
  const canvas = document.createElement('canvas');
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('2D canvas context is unavailable — cannot measure column widths');
  const root = getComputedStyle(document.documentElement);
  const family = root.getPropertyValue('--kira-font-data').trim() || 'monospace';
  const size = root.getPropertyValue(sizeToken).trim() || '12px';
  ctx.font = `${size} ${family}`;
  measureCtxBySize.set(sizeToken, ctx);
  return ctx;
}

/** Drops the memoized measuring contexts so the next initialWidths()/headerAwareMinWidth() call
 *  re-reads the current appearance tokens — without this, a font change leaves every
 *  unstored column sized for whatever font was active when this module first measured, for the
 *  rest of the session. Also drops initialWidths'/initialWidthsByIndex's own result caches below,
 *  for the same reason. */
export function resetMeasureCtx(): void {
  measureCtxBySize = new Map();
  widthsCache = new WeakMap();
  namedWidthsCache = new WeakMap();
}

/** What a header cell spends before its name gets a pixel — read off slickTheme.css's own rules
 *  for `.slick-header-column` (`display: flex; gap: var(--kira-s-2)`, so every adjacent pair of
 *  flex children gets its own 4px gap in addition to any margin a child declares on itself) and
 *  `.slick-sort-indicator`/`.header-key` (measured directly in a real header:
 *  `.slick-column-name`'s own available width against its `scrollWidth`). */
export interface HeaderChrome {
  /** `.slick-header-column`'s own `padding: 0 var(--kira-s-4)`, both sides. */
  padding: number; // 16
  /** The flex gap before `.slick-sort-indicator` (4) + its own `width: 14px`, plus the flex gap
   *  before the always-present, 0px-at-rest `.slick-sort-indicator-numbered` — 0 when the
   *  column is not sortable (SlickGrid builds neither div under `m.sortable`).
   *  The control narrowed to 12px and now cancels the flex gap that used to
   *  sit between it and the numbered badge, so what a sortable header spends here is one
   *  4px gap plus the 12px control. */
  sortControl: number; // 16 | 0
  /** The flex gap before the PK/FK `.header-key` badge (4) + the badge's own
   *  `margin-left: var(--kira-s-2)` (4) + its rendered content width ("PK"/"FK" at
   *  `--kira-t-xs` in the monospace stack, ~12px) — the gap and the badge's own margin are two
   *  separate spacings, not one. 0 when the column has no PK/FK badge. */
  keyBadge: number; // 20 | 0
}

/** The narrowest this column may ever be drawn without its own header name ellipsising — the flat
 *  MIN_WIDTH floor when that is already enough, and never more than MIN_WIDTH_CAP. Feeds
 *  both a column's initial `width` and its `minWidth` (SlickGridHost.vue) — fixing only the
 *  initial width would leave a drag able to crop the header again. */
export function headerAwareMinWidth(name: string, chrome: HeaderChrome): number {
  const headerText = getMeasureCtx('--kira-t-sm').measureText(name).width;
  return Math.min(
    MIN_WIDTH_CAP,
    Math.max(
      MIN_WIDTH,
      Math.ceil(headerText + chrome.padding + chrome.sortControl + chrome.keyBadge),
    ),
  );
}

// P2 R1: the deleted DataGrid.vue's `widths` computed depended on the tab's stored columnWidths (so a resize
// drag's own patchDataTabState call invalidates it) *and* calls initialWidths(page) unconditionally
// inside — every pointermove during a drag re-ran this canvas-measurement pass over every column
// and up to 50 sample rows each, even though a resize never changes the page's own data. Pages are
// frozen and stable by reference (same premise as nameIndexCache below), so a WeakMap keyed by the
// page turns every measurement after the first, for a given page, into a reference check.
//
// Cached by column POSITION, not name (finding 6, round 2) — a console result comes from ad-hoc SQL
// (`SELECT 1 AS x, 2 AS x`), so `page.columns[i].name` is routinely not unique; a name-keyed cache
// silently let the last duplicate's own measured width win for every column sharing that name. The
// main grid's own pages (real DB tables) never have duplicate column names, so `initialWidths`
// below — the name-keyed view every existing caller (SlickGridHost.vue) still wants — stays exactly
// as correct as it always was for that caller; `initialWidthsByIndex` is the new, duplicate-safe
// view `ConsoleSlickGrid.vue` addresses columns by everywhere else in that file (`colField(i)`,
// P30 §3 follow-up fix). `namedWidthsCache` below is `initialWidths`' own second-level cache, kept
// reference-stable across repeat calls exactly like the single cache this replaced (P2 R1's own
// "no further measuring" guarantee, plus reference identity a caller could reasonably rely on).
let widthsCache = new WeakMap<TabularPage, number[]>();
let namedWidthsCache = new WeakMap<TabularPage, Record<string, number>>();

/** Measures the wider of the header and a sample of the first rows, clamped to
 *  [MIN_INITIAL_WIDTH, MAX_WIDTH] px (P28 D5 — not MIN_WIDTH, which is the resize floor) —
 *  positional, parallel to `page.columns`/`page.chunks`. */
function measuredWidths(page: TabularPage): number[] {
  const cached = widthsCache.get(page);
  if (cached) return cached;

  const ctx = getMeasureCtx();
  const decoder = new TextDecoder();
  const widths: number[] = [];
  const sampleCount = Math.min(SAMPLE_ROWS, page.rowCount);

  for (let c = 0; c < page.columns.length; c++) {
    const column = page.columns[c];
    const chunk = page.chunks[c];
    let max = ctx.measureText(column.name).width;
    for (let r = 0; r < sampleCount; r++) {
      if (isNull(chunk, r)) continue;
      const width = ctx.measureText(cellText(chunk, r, decoder)).width;
      if (width > max) max = width;
    }
    widths.push(Math.min(MAX_WIDTH, Math.max(MIN_INITIAL_WIDTH, Math.round(max + CELL_PADDING))));
  }
  widthsCache.set(page, widths);
  return widths;
}

/** Name-keyed view of `measuredWidths` — correct as long as `page.columns` has no duplicate
 *  names (every real DB table): on a duplicate, the last one measured wins, same as it always has.
 *  `initialWidthsByIndex` below is the duplicate-safe view. Its own cache, so repeat calls for the
 *  same page return the exact same object (not just an equal one) without rebuilding it. */
export function initialWidths(page: TabularPage): Record<string, number> {
  const cached = namedWidthsCache.get(page);
  if (cached) return cached;
  const arr = measuredWidths(page);
  const out: Record<string, number> = {};
  page.columns.forEach((column, i) => {
    out[column.name] = arr[i] as number;
  });
  namedWidthsCache.set(page, out);
  return out;
}

/** Position-keyed view of `measuredWidths` — the one duplicate column names can't collide in
 *  (finding 6, round 2). */
export function initialWidthsByIndex(page: TabularPage): number[] {
  return measuredWidths(page);
}

// columnOffsets/columnRangeExtractor/observeScrollElementRect/
// observeScrollElementOffset/MAX_OVERSCAN_COLUMNS — the tanstack-vue-virtual column axis these
// four served retired once ConsoleResultGrid.vue's tabular branch (their only remaining caller)
// moved onto SlickGrid's own native column virtualization. Only this column axis retired —
// @tanstack/vue-virtual itself is still a real dependency elsewhere
// (views/shared/keyvalue/KeyValuePane.vue, views/grpcrequest/ResponsePane.vue). See git
// history for the deleted implementations.

// P22 iter2 D3: velocity-adaptive, direction-biased row overscan ("runway") — see the plan's §5 D3.
// The row axis's overscan was symmetric and direction-blind (`overscan: Math.ceil(OVERSCAN_PX /
// rowHeight)`, virtual-core's own defaultRangeExtractor): it expands range.startIndex/endIndex by
// the same row count on both sides regardless of which way the user is scrolling, so a fast fling
// only ever has half the buffer's worth of runway ahead of it (F6). This extends the window further
// in the direction of travel, and only there, so the same total DOM buys more runway where it's
// actually needed.
//
// BASE_LEAD_PX/BASE_TRAIL_PX both equal OVERSCAN_PX so that at zero velocity this produces *exactly*
// the row window `overscan: Math.ceil(OVERSCAN_PX / rowHeight)` did — see rowRangeBounds' own
// comment, below, for the arithmetic that guarantees this. Every existing at-rest budget (the
// overscan-coverage invariants, the DOM-cell bounds) must see zero change from this file.
//
// LEAD_FRAMES/MAX_LEAD_PX are provisional: nobody in this repo has measured a real macOS momentum
// scroll's velocity (the plan's F5) — these are a defensible first guess, to be re-set once
// window.__kiraScrollTrace (D2) reports one from real hardware. Extra runway costs compositing
// memory while flinging only — accepted (F7) as a trade, not free.
// Row-axis runway, tuned independently of OVERSCAN_PX (column-axis clamp); defaulted equal.
export const BASE_LEAD_PX = 560;
export const BASE_TRAIL_PX = 560;
/** Provisional (see above): extra lead px granted per px/frame of measured velocity. */
export const LEAD_FRAMES = 6;
/** Provisional (see above): hard ceiling on the lead side regardless of velocity. */
export const MAX_LEAD_PX = 2400;
// The cap is expressed in *cells*, not rows — a wide table (e.g. 61 columns) is
// already close to budgets.spec.ts's < 2 500 DOM-cell bound at rest, so a flat row cap would
// let it blow that budget while a two-column table barely used its own share. Set below the
// existing bound so a wide table's extra lead self-limits well before the DOM-size gate would.
export const CELL_BUDGET = 2200;

// Provisional (same epistemic status as LEAD_FRAMES/MAX_LEAD_PX, above —
// nobody has a real renderMs-per-cell figure from the SlickGrid engine yet).
// This is a PER-CALL cap on how many new (not-yet-cached) cells a single synchronous
// SlickGrid render() pass may build, independent of CELL_BUDGET (which caps the TOTAL mounted
// window across calls, not any one call's own new-row batch). Sized to keep one render() call's
// synchronous work well under a single frame's budget even on a 120 Hz display; re-set once the
// real-Mac protocol (docs/PERF.md) reports actual renderMs figures for a cold batch of this
// size. Consumed by views/shared/slick/kiraSlickGrid.ts's own getRenderedRange override — the
// deleted tanstack-virtual/DataGrid.vue grid had no equivalent single-call batch-size hazard
// (SlickGrid's render() builds every newly-entering row synchronously in one unconditional
// DOM-construction pass, unlike Vue's own patch, which that grid went through instead), so this
// constant is SlickGrid-only.
export const MAX_NEW_CELLS_PER_RENDER = 600;

// A separate, SEPARATELY-CAPPED per-render budget for *runway* (beyond strictly-visible) growth
// only — MAX_NEW_CELLS_PER_RENDER above stays the absolute per-pass ceiling and the step-6 floor
// short-circuit unchanged. Lowering this makes the runway grow in small even steps instead of one
// large step followed by a cliff (a variance reduction, not a total-work reduction), and it trades
// directly against how fast the runway converges (uncoveredPx). Literal matches
// MAX_NEW_CELLS_PER_RENDER on purpose — i.e. behaviourally neutral, byte-identical to today's
// emitted range — because nobody has a real-hardware number for it yet; docs/PERF.md step 4 is the
// A/B that sets it. Same precedent as forceSyncScrollingOverride (main.ts): a dial with a
// documented default, not a silent behaviour change. Consumed by
// views/shared/slick/kiraSlickGrid.ts's own getRenderedRange override, step 7.
export const MAX_NEW_LEAD_CELLS_PER_RENDER = 600;

// Provisional, same epistemic status as LEAD_FRAMES/MAX_LEAD_PX/
// MAX_NEW_CELLS_PER_RENDER: how long the viewport must go without a native scroll event before a
// self-scheduled catch-up render is allowed to run. A catch-up that fires while a fling is still
// delivering scroll events lands in the same animation frame as that frame's scroll-driven
// render, doubling the frame's work (docs/HISTORY.md: 131 of 140 frames in a WebKit repro). ~1.5
// frames at 60 Hz, ~3 at 120 Hz. 0 restores the pre-fix
// "fire on the very next rAF, unconditionally" behaviour exactly — which is what makes the
// real-Mac A/B a console line (docs/PERF.md). Re-set from that A/B, not from here.
export const CHASE_QUIET_MS = 24;

export interface RowRangeExtractorConfig {
  baseLeadPx: number;
  baseTrailPx: number;
  leadFrames: number;
  maxLeadPx: number;
  cellBudget: number;
}

/**
 * The row axis's own budget arithmetic — a pixel budget (velocity-adaptive, direction-biased,
 * cell-capped) reduced to a pair of inclusive row bounds. Originally split out from a
 * `rowRangeExtractor` wrapper shaped for `@tanstack/vue-virtual`'s own `rangeExtractor` API
 * (DataGrid.vue's own consumer, deleted with it at P22 Pass B's cutover — this function's own
 * general arithmetic had no dependency on that API and so needed no counterpart), so
 * `views/shared/slick/kiraSlickGrid.ts`'s `getRenderedRange` override could reuse the exact same
 * arithmetic instead of restating it — see that file's own comment. `range` keeps that API's own
 * three-field shape structurally (P30 §3.6 C7 dropped the `@tanstack/vue-virtual` import itself,
 * the library being long gone from this arithmetic's only remaining caller). `direction`/
 * `velocityPxPerFrame` come from the caller's own scroll-velocity sampler (KiraSlickGrid's own,
 * today the only caller); `mountedColumnCount` is the row axis's own budget divisor, read from the
 * column virtualizer so a wide table's cap tightens with however many columns are actually mounted
 * right now, not the table's total column count.
 */
export function rowRangeBounds(
  range: { startIndex: number; endIndex: number; count: number },
  rowHeight: number,
  /** |Δoffset| since the previous scroll event, in px. 0 at rest. */
  velocityPxPerFrame: number,
  /** Sign of the scroll delta: 1 forward (toward endIndex/down), -1 backward (up), 0 at rest. */
  direction: 1 | -1 | 0,
  mountedColumnCount: number,
  cfg: RowRangeExtractorConfig,
): { start: number; end: number } {
  if (range.count <= 0) return { start: 0, end: -1 };

  const trailRows = Math.ceil(cfg.baseTrailPx / rowHeight);
  const leadBaselineRows = Math.ceil(cfg.baseLeadPx / rowHeight);
  // Clamped below at cfg.baseLeadPx, so at velocityPxPerFrame === 0 this equals cfg.baseLeadPx
  // exactly, leadRowsWanted === leadBaselineRows, extraLeadRows === 0 — D3(a)'s own requirement,
  // true unconditionally regardless of the budget math below.
  const leadPxWanted = Math.min(
    cfg.maxLeadPx,
    Math.max(cfg.baseLeadPx, cfg.baseLeadPx + velocityPxPerFrame * cfg.leadFrames),
  );
  const leadRowsWanted = Math.ceil(leadPxWanted / rowHeight);
  const extraLeadRows = Math.max(0, leadRowsWanted - leadBaselineRows);

  // The baseline (both sides, leadBaselineRows === trailRows) is always granted in full, exactly
  // like today — only the *extra* velocity-driven lead is budget-capped, and only on the lead side.
  const visibleRows = range.endIndex - range.startIndex + 1;
  const budgetRows = Math.floor(cfg.cellBudget / Math.max(1, mountedColumnCount));
  const grantedExtraLeadRows = Math.max(
    0,
    Math.min(extraLeadRows, budgetRows - visibleRows - trailRows - leadBaselineRows),
  );
  const leadRows = leadBaselineRows + grantedExtraLeadRows;

  const [startRows, endRows] = direction < 0 ? [leadRows, trailRows] : [trailRows, leadRows];
  const start = Math.max(range.startIndex - startRows, 0);
  const end = Math.min(range.endIndex + endRows, range.count - 1);
  return { start, end };
}

// §8.5's type-aware right-alignment for numerics.
export function alignmentFor(descriptor: ColumnDescriptor): 'left' | 'right' {
  return descriptor.typeClass === 'number' ? 'right' : 'left';
}

/** The display order: stored order filtered to live columns, then any new columns appended. */
// `kept.includes(n)` made the reordered branch O(cols²) — harmless at a normal
// column count, but resolveColumnOrder is called *per cell* by displayCell/rowSnapshot (not once
// per bulk operation, which is its own separate fix at those call sites), so a wide table with a
// stored column order turned a full-page copy into ~600,000 calls each doing an O(cols²) scan —
// an unrecoverable hang, not a slowdown. Fixed two ways: a Set instead of `.includes` makes a
// single call O(cols) regardless, and memoizing per (page, stored) — pages are frozen and
// reference-stable (the same premise nameIndexCache/widthsCache below already rely on) — makes
// repeat calls with an unchanged column order O(1).
const columnOrderCache = new WeakMap<TabularPage, WeakMap<object, string[]>>();
// A stable, non-null key so `resolveColumnOrder(page, null)` — the common case — shares one cache
// slot per page rather than never hitting the cache at all (null can't be a WeakMap key).
const NULL_STORED_KEY: object = {};

export function resolveColumnOrder(page: TabularPage, stored: string[] | null): string[] {
  const key: object = stored ?? NULL_STORED_KEY;
  let perPage = columnOrderCache.get(page);
  if (!perPage) {
    perPage = new WeakMap();
    columnOrderCache.set(page, perPage);
  }
  const cached = perPage.get(key);
  if (cached) return cached;

  const names = page.columns.map((c) => c.name);
  let result: string[];
  if (!stored) {
    result = names;
  } else {
    const known = new Set(names);
    const kept = stored.filter((n) => known.has(n));
    const keptSet = new Set(kept);
    const missing = names.filter((n) => !keptSet.has(n));
    result = [...kept, ...missing];
  }
  perPage.set(key, result);
  return result;
}

// Pages are frozen and stable by reference (page.ts's setPage), so a WeakMap keyed by the page
// itself memoises the name -> index lookup exactly once per page — the single mapping §0 note 4
// exists to guarantee, kept O(1) per call so it stays cheap on the render path.
const nameIndexCache = new WeakMap<TabularPage, Map<string, number>>();

function nameIndexFor(page: TabularPage): Map<string, number> {
  let map = nameIndexCache.get(page);
  if (!map) {
    map = new Map(page.columns.map((c, i) => [c.name, i]));
    nameIndexCache.set(page, map);
  }
  return map;
}

/** Display position -> index into page.columns/page.chunks. -1 when the name is gone. */
export function pageColumnIndexFor(page: TabularPage, order: string[], displayCol: number): number {
  const name = order[displayCol];
  if (name === undefined) return -1;
  return nameIndexFor(page).get(name) ?? -1;
}

// The column-header tooltip content the data tab and console grids share. The
// generated type explanation (`body`) and the DB comment (`note`) stay separate so the tooltip
// stacks them; `tags` carry the key/nullability facts DESCRIBE knows. `dataType` is passed in
// rather than read off `col`: the data tab overlays a DESCRIBE-derived type the console lacks.
export interface ColumnTipFacts {
  comment?: string | null;
  key?: 'PK' | 'FK' | null;
  /** From DESCRIBE only; null/undefined when unknown (console results). */
  nullable?: boolean | null;
  generated?: boolean;
}
export interface ColumnTooltipParts {
  title: string;
  meta?: string;
  metaColor?: string;
  tags?: string[];
  body?: string;
  note?: { label: string; text: string };
}
export function columnHeaderTooltip(
  col: { name: string; typeClass: TypeClass },
  dataType: string,
  facts: ColumnTipFacts = {},
): ColumnTooltipParts {
  const tags: string[] = [];
  if (facts.key === 'PK') tags.push(t('studio.definition.primaryKey.label'));
  else if (facts.key === 'FK') tags.push(t('studio.definition.foreignKey.label'));
  if (facts.nullable === false) tags.push(t('studio.definition.notNull.label'));
  else if (facts.nullable === true) tags.push(t('studio.definition.nullable.label'));
  if (facts.generated) tags.push(t('studio.cellEditor.readOnly.generated.label'));
  const comment = facts.comment?.trim();
  return {
    title: col.name,
    meta: dataType || undefined,
    metaColor: typeClassColor(col.typeClass) || undefined,
    tags: tags.length > 0 ? tags : undefined,
    body: (dataType ? typeDescription(dataType) : null) || undefined,
    note: comment ? { label: t('common.comment.label'), text: comment } : undefined,
  };
}

// P107 T2-17: byte-identical across SlickGridHost.vue/ConsoleSlickGrid.vue — the DOM attributes a
// header cell's tooltip formatter spreads onto its element, next to columnHeaderTooltip's own
// content it reads.
export function tooltipAttrs(content: ColumnTooltipParts): Record<string, string> {
  const plain = [
    content.title,
    content.meta,
    ...(content.tags ?? []),
    content.body,
    content.note?.label,
    content.note?.text,
  ]
    .filter((v): v is string => !!v)
    .join('\n');
  return {
    'data-kira-tip': plain,
    'data-kira-tip-parts': JSON.stringify(content),
    'aria-label': plain,
  };
}
