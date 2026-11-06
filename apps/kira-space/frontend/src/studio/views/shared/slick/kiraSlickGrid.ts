import type { Column } from 'slickgrid';
import { SlickGrid } from 'slickgrid';
import {
  BASE_LEAD_PX,
  BASE_TRAIL_PX,
  CELL_BUDGET,
  CHASE_QUIET_MS,
  LEAD_FRAMES,
  MAX_LEAD_PX,
  MAX_NEW_CELLS_PER_RENDER,
  MAX_NEW_LEAD_CELLS_PER_RENDER,
  OVERSCAN_PX,
  type RowRangeExtractorConfig,
  rowRangeBounds,
} from '../page/columns';
import type { RowHandle } from './dataSource';
import * as scrollTrace from './scrollTrace';

// SPEC forbids a views/<kind>/* file importing another views/<kind>/*, and
// this class (plus the scroll mechanism it carries) is exactly what
// views/console/ConsoleSlickGrid.vue needs to inherit unmodified.

// main.ts's own `declare global` (the real source of truth for this shape, D9) lives in a
// different TS program from tests/unit/tsconfig.json's — mirrors tests/ui/global.d.ts's own
// re-declaration of the same handful of `__kira*` hooks, for the identical cross-program reason.
declare global {
  interface Window {
    __kiraGridScrollWorkStart?: (t: number) => void;
    __kiraGridTuning?: {
      leadFramesOverride?: number;
      maxLeadPxOverride?: number;
      /** P22 iter2-scroll-gaps D2: overrides columns.ts's MAX_NEW_CELLS_PER_RENDER. */
      maxNewCellsPerRenderOverride?: number;
      /** P22 iter2-scroll-gaps D3: read by SlickGridHost.vue at grid construction, not by this file
       *  — declared here too only because this program's declaration merging requires an identical
       *  shape to main.ts's own. See main.ts's own doc comment for what it does. */
      forceSyncScrollingOverride?: boolean;
      /** Overrides columns.ts's CHASE_QUIET_MS. Read fresh on every chase
       *  callback, never cached — 0 restores the pre-fix "fire on the very next rAF,
       *  unconditionally" behaviour exactly, so the real-Mac A/B (docs/PERF.md) is a console
       *  line, not a rebuild. */
      chaseQuietMsOverride?: number;
      /** P22 iter2-pacing D2: overrides columns.ts's MAX_NEW_LEAD_CELLS_PER_RENDER. */
      maxNewLeadCellsPerRenderOverride?: number;
      /** `false` drops the chase's per-frame gate, leaving only the
       *  CHASE_QUIET_MS wall-clock one. See `scheduleChase` below for why the ms gate alone is not
       *  sufficient. `chaseQuietMsOverride = 0` still disables both at once, so that override keeps
       *  its documented "the pre-fix policy exactly" meaning. */
      chaseFrameGateOverride?: boolean;
      /** P22 iter2-onset D1: read by SlickGridHost.vue's own velocity sampler, not by this file —
       *  declared here too only because this program's declaration merging requires an identical
       *  shape to main.ts's own. See main.ts's own doc comment for what it does. */
      freshVelocitySampleOverride?: boolean;
    };
  }
}

export interface KiraSlickVelocity {
  pxPerFrame: number;
  direction: 1 | -1 | 0;
}

/** `__kiraGridTuning`'s overrides, read fresh on every `getRenderedRange` call (never cached) —
 *  the same contract the deleted DataGrid.vue's own row-axis `rangeExtractor` closure had, so the
 *  real-Mac A/B protocol (`docs/PERF.md` §2.1a) needs one build, not a rebuild per variant. */
function runwayConfig(): RowRangeExtractorConfig {
  const tuning = window.__kiraGridTuning;
  return {
    baseLeadPx: BASE_LEAD_PX,
    baseTrailPx: BASE_TRAIL_PX,
    leadFrames: tuning?.leadFramesOverride ?? LEAD_FRAMES,
    maxLeadPx: tuning?.maxLeadPxOverride ?? MAX_LEAD_PX,
    cellBudget: CELL_BUDGET,
  };
}

/** How many rows in `[start, end]` fall outside
 *  `[prevStart, prevEnd]`, i.e. how many of them are *not* already in SlickGrid's own `rowsCache`
 *  (`prev` is `lastRenderedRowBounds`, a reliable proxy for `rowsCache` membership —
 *  `cleanupRows` keeps exactly the previously-returned range). A pure function, split out
 *  so the batch-cap arithmetic is testable without constructing a real `SlickGrid`. */
export function countNewRows(
  start: number,
  end: number,
  prevStart: number,
  prevEnd: number,
): number {
  if (end < start) return 0;
  const total = end - start + 1;
  const overlapStart = Math.max(start, prevStart);
  const overlapEnd = Math.min(end, prevEnd);
  const overlap = overlapEnd >= overlapStart ? overlapEnd - overlapStart + 1 : 0;
  return total - overlap;
}

/** The column axis's own overscan clamp (D4's third bullet: `OVERSCAN_PX` per side instead of a
 *  full viewport width, F4's third reading) — split out as a pure function so it's testable
 *  without constructing a real `SlickGrid` (which needs a live DOM container). */
export function clampColumnOverscan(
  leftPx: number,
  rightPx: number,
  overscanPx: number,
  canvasWidth: number,
): { leftPx: number; rightPx: number } {
  return {
    leftPx: Math.max(0, leftPx - overscanPx),
    rightPx: Math.min(canvasWidth, rightPx + overscanPx),
  };
}

// WebKit rebuilds every element's style when a <style> is removed or a CSSOM rule is mutated
// (~55 ms on the test page); a sheet appended with its text already set, or a custom property on
// the grid root, restyles only what it touches. So column rules live in one append-only sheet and
// per-grid values live on the grid root.
const COLUMN_RULE_CHUNK = 256;
let columnRuleCapacity = 0;

function ensureColumnRules(columnCount: number): void {
  if (columnCount <= columnRuleCapacity) return;
  const next = Math.ceil(columnCount / COLUMN_RULE_CHUNK) * COLUMN_RULE_CHUNK;
  const rules: string[] = [];
  if (columnRuleCapacity === 0) {
    rules.push(
      '.kira-sg .slick-group-header-column,.kira-sg .slick-header-column{left:1000px}',
      '.kira-sg .slick-top-panel{height:var(--sg-top-panel-h)}',
      '.kira-sg .slick-preheader-panel{height:var(--sg-preheader-h)}',
      '.kira-sg .slick-topheader-panel{height:var(--sg-topheader-h)}',
      '.kira-sg .slick-headerrow-columns{height:var(--sg-headerrow-h)}',
      '.kira-sg .slick-footerrow-columns{height:var(--sg-footerrow-h)}',
      '.kira-sg .slick-cell{height:var(--sg-cell-h)}',
      '.kira-sg .slick-row{height:var(--sg-row-h)}',
    );
  }
  for (let i = columnRuleCapacity; i < next; i++) {
    rules.push(`.kira-sg .l${i}{left:var(--sg-l${i})}`, `.kira-sg .r${i}{right:var(--sg-r${i})}`);
  }
  const style = document.createElement('style');
  style.textContent = rules.join('\n'); // text before insertion: WebKit's additive path
  document.head.append(style);
  columnRuleCapacity = next;
}

/**
 * §6 D4 — a thin `SlickGrid` subclass overriding `getRenderedRange`, the plan's single point of
 * coupling to SlickGrid internals. SlickGrid's own runway (F4) is *smaller* than this app's at
 * rest — 3 rows/side (`minRowBuffer`) against this app's 560px (≈20 rows/side) — and not
 * velocity-scaled in motion; adopting it as-is would make the reported fast-scroll symptom worse,
 * not better. This reuses `rowRangeBounds` (the exact arithmetic the deleted `DataGrid.vue`'s own
 * row axis ran, C1's own refactor) rather than restating it — originally so both grids' at-rest
 * window was provably the same number during the migration, and still the reason this arithmetic
 * lives in its own shared function rather than inline here — `tests/unit/row-range-bounds.spec.ts`
 * covers that arithmetic; this file's own test (kira-slick-grid.spec.ts) covers the column-overscan
 * clamp and `countNewRows`, the batch-cap arithmetic below, neither of which live in columns.ts.
 *
 * Every SlickGrid method called below is public and documented in the published `.d.ts`:
 * `getVisibleRange`, `getDataLength`, `getOptions`, `getCanvasNode`. `vScrollDir` (a protected
 * field) is deliberately never read — direction comes from the host's own velocity sampler
 * (`velocity`, below), which already discards a discrete jump as "at rest"
 * (`MAX_PLAUSIBLE_ROW_VELOCITY_PX_PER_FRAME`, mirrored from the deleted DataGrid.vue's own onScroll), a case a
 * raw sign test on `vScrollDir` would not. **On a `slickgrid` version bump, re-check that
 * `render()` still calls `this.getRenderedRange()` and that its return shape is still
 * `{ top, bottom, leftPx, rightPx }`** — F4's own citation, `dist/esm/index.mjs`'s `render()`.
 *
 * P182 stage 1: `setScroller` reparents the scrolling `.slick-header` into the scroll viewport as a
 * sticky strip, so header and rows share one native scroller. `getBoundingClientRect` on that
 * viewport reports only the rows band, the box SlickGrid's `Utils.height`/`Utils.offset` reads
 * assume. Re-check on a `slickgrid` bump: `setScroller` still picks `_headerScrollContainer` and
 * `_viewportScrollContainerY`; viewport measurements still go through `Utils.height`/`Utils.offset`
 * (`grep -n "_viewportScrollContainer[XY]" slick.grid.ts`).
 */
// Column<T>'s own `field` type is a recursive PathsToStringProps<T> derived from T's shape —
// RowHandle's own fields (row/pos/insertId) are not what `field` needs to hold (the app's
// arbitrary db column names), so the column generic is deliberately widened to `any` below,
// matching slickgrid's own escape hatch for exactly this case.
// biome-ignore lint/suspicious/noExplicitAny: see comment above.
export class KiraSlickGrid extends SlickGrid<RowHandle, Column<any>> {
  /** Supplied by the host on every scroll sample (the deleted DataGrid.vue's own `rowVelocity()`
   *  analogue) —
   *  read fresh on every call below, never memoised (this runs *during* SlickGrid's own render).
   *  Defaults to "at rest" so a grid that hasn't wired a sampler yet still renders the baseline
   *  runway — the "byte-identical to today at rest" guarantee. */
  velocity: () => KiraSlickVelocity = () => ({ pxPerFrame: 0, direction: 0 });

  /** The row axis's own budget divisor — a plain variable the host updates
   *  from `onRendered`, mirroring the deleted DataGrid.vue's own `mountedColumnCount` (columns.ts's
   *  own comment: calling into the column virtualizer from *inside* the row-range computation
   *  itself measurably regressed the scroll budget; the same hazard applies here). */
  mountedColumnCount = 1;

  /** The last `{start, end}` *rendered* row bounds this override computed — read by the host's
   *  `onRendered` handler to compute the page-row window it hands `setVisibleWindow`.
   *  SlickGrid's own `onRendered` event reports the strictly *visible* range (`visible.top/bottom`
   *  in `render()`), not the wider *rendered* range this override mounts — using the
   *  narrower one would prune the decode cache back to the visible slice on every render, defeating
   *  memoisation for the whole overscan band. */
  lastRenderedRowBounds: { start: number; end: number } = { start: 0, end: -1 };

  /** Supplied by the host: performance.now() at the last native `scroll` event on the viewport.
   *  The gate — a catch-up render never fires while this is recent. Defaults
   *  to "never scrolled" so a grid that hasn't wired a sampler still chases immediately — and, like
   *  `velocity`/`mountedColumnCount`/`chaseWanted` above/below, every read below tolerates this
   *  field being `undefined` on the base constructor's own pre-field-init call. */
  lastScrollEventAt: () => number = () => Number.NEGATIVE_INFINITY;

  /** Supplied by the host: a counter incremented by every native `scroll` event on the viewport.
   *  The chase's *per-frame* gate reads this rather than a wall clock; see
   *  `scheduleChase` below. Defaults to a constant so a grid that hasn't wired a sampler still
   *  chases, and like every other host-supplied field here every read tolerates it being
   *  `undefined` on the base constructor's own pre-field-init call. */
  scrollEventSeq: () => number = () => 0;

  /** P22 iter2-pacing D1 — replaces iter2-scroll-gaps' own `chasePending` boolean. `chaseHandle` is
   *  the live `requestAnimationFrame` id (0 when none is pending); `chaseWanted` is recomputed by
   *  every `getRenderedRange` call and read by the chase callback itself, so a call that lands
   *  after the deficit has already closed (by a later, non-chase render) is a correctly-cheap no-op
   *  instead of an unconditional extra render. See `scheduleChase` below for the re-arm loop this
   *  drives. Re-entrancy note (this file's own existing comment, above, on
   *  `velocity`/`mountedColumnCount`): reads of these fields must tolerate `undefined` on the very
   *  first, pre-field-init call exactly like those two already do.
   *
   *  Finding 5 (round 2) — `declare`d for the same reason `ancestorScrollHandler` above is: a plain
   *  `= 0`/`= false`/`= -1` initializer runs *after* `super()` returns, which would unconditionally
   *  clobber a write `getRenderedRange`/`scheduleChase` made *during* `super()` (via `init()` ->
   *  `resizeCanvas()` -> `render()`, which this override's own comment already documents as a real,
   *  observed call) back to its default — losing an armed `requestAnimationFrame` id exactly like
   *  the `ancestorScrollHandler` bug lost its listener. Currently unreachable in practice (today's
   *  construction-time values leave `chaseWanted` false on that first call, so `scheduleChase` never
   *  runs early enough to matter — a numeric coincidence, not a guarantee), but defended the same way
   *  regardless. Unlike `ancestorScrollHandler`, nothing guarantees a pre-`super()`-return write
   *  actually happens here, so the constructor below (post-`super()`) nullish-assigns a real default
   *  to each field — `??=`, never `=`, so it still can't clobber a genuine early write. */
  declare private chaseHandle: number;
  declare private chaseWanted: boolean;
  /** P22 iter2-onset D2 — `scrollEventSeq()` as of the previous chase callback, or -1 for "this is
   *  the first callback of a chain, there is nothing to compare against yet". Finding 5: `declare`d
   *  alongside `chaseHandle`/`chaseWanted` above, same reasoning. */
  declare private chaseSeenSeq: number;

  /** `getRenderedRange`'s own average-column-width divisor, below, used to be an
   *  O(cols) `reduce` on *every* render call; cached here and only recomputed when the columns
   *  array itself is a new reference (`setColumns`) or `onColumnsResized` fires (a resize drag
   *  mutates each `Column.width` in place, so the array reference alone doesn't change — the
   *  event is the only signal). `declare`d for the same reason `chaseHandle`/`chaseWanted` are:
   *  `getRenderedRange` can run *during* `super()` (via `init()` -> `resizeCanvas()` -> `render()`,
   *  this file's own documented call path), so a plain initializer would clobber a write made
   *  before it runs. */
  declare private avgColumnWidthCache: number;
  declare private avgColumnWidthColumns: unknown;
  declare private avgColumnWidthDirty: boolean;

  // Finding 5 — forwards to `SlickGrid`'s own constructor unchanged (this class has no constructor
  // params of its own), then nullish-defaults the three `declare`d fields above: `??=` rather than
  // `=` so a write `getRenderedRange`/`scheduleChase` already made *during* `super()` survives —
  // only a field `super()` left genuinely untouched gets its default here.
  // biome-ignore lint/suspicious/noExplicitAny: see the class-level comment above.
  constructor(...args: ConstructorParameters<typeof SlickGrid<RowHandle, Column<any>>>) {
    super(...args);
    this.chaseHandle ??= 0;
    this.chaseWanted ??= false;
    this.chaseSeenSeq ??= -1;
    this.avgColumnWidthCache ??= 1;
    this.avgColumnWidthColumns ??= null;
    this.avgColumnWidthDirty ??= true;
    // A7: a resize drag mutates each Column.width in place (slick.grid.ts's own `c.width = ...`),
    // so the columns array reference never changes — this event is the only signal a cached
    // average needs recomputing.
    this.onColumnsResized.subscribe(() => {
      this.avgColumnWidthDirty = true;
    });
  }

  /** P22 iter2-pacing D1 — the fix itself. A catch-up render is gated on scroll *quiescence*, not
   *  on a frame token ("did a render already run this frame?"): that plan's own §3 F2 found the
   *  intra-frame ordering of the scroll-driven render and a same-frame chase differs between
   *  engines and between input paths (opposite orderings from two different sandbox harnesses), so
   *  a fix that depends on which one ran first is correct in one environment and wrong in another.
   *  "Is a scroll still live?" is true in every frame that will carry a scroll-driven render,
   *  regardless of where in the frame that render actually sits — ordering-agnostic by
   *  construction. Termination: `chaseWanted` is recomputed by every render and clears once a
   *  render reaches `target`; `target` itself shrinks as the grid goes quiet (the host's own
   *  `velocity()` reports zero 150ms after the last scroll sample), so the deficit this loop is
   *  closing shrinks while it closes it — it cannot spin forever.
   *
   *  P22 iter2-onset D2 — the wall-clock half of that gate is **not sufficient on its own**, and
   *  this is a correction to the pacing pass, not a new requirement. `CHASE_QUIET_MS` is 24ms, but
   *  the very recording that pass was written from reports a **p50 frame duration of 32.1ms** on
   *  real hardware (docs/PERF.md §2.1c), and this sandbox's own wheel-fling harness measures p50 29
   *  / p95 58 / max 65ms. Whenever a frame outlasts the threshold, "24ms since the last scroll
   *  event" stops meaning "no scroll event is driving this frame" — the last event was simply in
   *  the *previous*, long, frame — and the gate silently opens on a frame that does carry a
   *  scroll-driven render. Measured: 7-9 doubled frames out of ~80 once a chase is actually wanted
   *  on every frame. It went unnoticed because at `a9dc570` the host's velocity sampler was one
   *  scroll event stale (P22 iter2-onset D1) and therefore reported *zero* on much of a fast fling,
   *  which collapses `target` to the base runway and means no chase is wanted at all — the gate was
   *  passing its own test by never being asked.
   *
   *  So the ms gate is joined by a **per-frame** one that cannot be outrun by a slow frame: a
   *  catch-up render requires that **no native `scroll` event arrived between the previous
   *  animation frame and this one**, read as a sequence number rather than a duration. That is the
   *  refinement the pacing plan's own §10 anticipated ("deriving the threshold from the observed
   *  rAF interval ... the obvious refinement"), and it keeps §3 F2's ordering-agnosticism intact —
   *  it still never asks whether the scroll render or the chase ran first within a frame, only
   *  whether a scroll event happened at all across the last one, which is true in every frame that
   *  will carry a scroll-driven render under either ordering. Cost: after a fling stops the chase
   *  now converges from the second quiet frame rather than the first. */
  private scheduleChase(): void {
    if (this.chaseHandle) return;
    this.chaseHandle = requestAnimationFrame(() => {
      this.chaseHandle = 0;
      if (!this.chaseWanted) return;
      const tuning = window.__kiraGridTuning;
      const quietMs = tuning?.chaseQuietMsOverride ?? CHASE_QUIET_MS;
      const lastScroll = this.lastScrollEventAt
        ? this.lastScrollEventAt()
        : Number.NEGATIVE_INFINITY;
      const seq = this.scrollEventSeq ? this.scrollEventSeq() : 0;
      const seenSeq = this.chaseSeenSeq ?? -1;
      this.chaseSeenSeq = seq;
      // `chaseQuietMsOverride = 0` disables the gate *whole*, both halves — that override's
      // documented contract (main.ts, docs/PERF.md) is "restores the pre-fix fire-on-the-very-
      // next-rAF behaviour exactly", and it has to keep meaning that for the real-Mac A/B and for
      // slick-grid.spec.ts's own T3 to stay honest.
      if (quietMs > 0) {
        // Still scrolling: this frame already has (or is about to get) a scroll-driven render of
        // its own. Re-arm — a rAF scheduled from inside a rAF callback always lands in a later
        // frame, so this never re-enters the same frame it was just called from.
        if (performance.now() - lastScroll < quietMs) {
          this.scheduleChase();
          return;
        }
        // The per-frame half. `seenSeq === -1` is the first callback of a chain, which has nothing
        // to compare against and so must re-arm rather than guess.
        if ((tuning?.chaseFrameGateOverride ?? true) && seq !== seenSeq) {
          this.scheduleChase();
          return;
        }
      }
      this.render();
    });
  }

  /** P22 Pass B postscript §14.2 — root-caused the "`_viewport.includes` uncaught exception from
   *  `bindAncestorScrollEvents`" item, confirmed with a real browser (`removeEventListener` without
   *  a matching `capture` flag is a documented no-op per spec, verified live via
   *  `document.dispatchEvent(new Event('scroll'))` after the exact call shape below). Stock
   *  `bindAncestorScrollEvents` (slick.grid.ts) does:
   *    `this._bindingEventService.bind(document, "scroll", handler, true)`
   *  — the trailing `true` is `useCapture`. `BindingEventService.unbind()` (called by `destroy()`'s
   *  own `unbindAll()`, and by every other per-element unbind in `destroy()`) calls
   *  `element.removeEventListener(eventName, listener)` with **no capture argument at all** —
   *  `removeEventListener` only removes a listener registered with the *same* capture flag, so this
   *  capture-phase, document-level listener is never actually removed by any destroy path in the
   *  library. It survives the grid instance's own teardown, keeps firing on every scroll event
   *  anywhere in the document (it was never scoped to this grid's own container), and dereferences
   *  `this._viewport` — nulled by `destroyAllElements()` in the same `destroy(true)` call — on the
   *  very next one. Not a call-order gap in this app's own code (`destroy()`/`unbindAll()` do run in
   *  the order this file's own next comment already relies on) — a real bug in the vendored
   *  library's own capture-flag handling. Fixed the same way `getRenderedRange`/`render` are
   *  overridden: bind it ourselves, outside the buggy service, so it can be removed correctly.
   *
   *  P22 postscript follow-up: this field must NOT carry a `= null` initializer. `SlickGrid`'s own
   *  constructor (`super()`) calls `initialize()` synchronously, which calls this class's own
   *  `bindAncestorScrollEvents()` override *from inside* `super()` — before `super()` has returned
   *  and therefore before this subclass's own field initializers run (standard JS: field
   *  initializers run in declaration order immediately after `super()` returns, per spec, same
   *  hazard `getRenderedRange`'s comment documents for `velocity`/`mountedColumnCount` below). A
   *  `= null` initializer here doesn't just leave the field transiently unset for one read — it
   *  unconditionally OVERWRITES the real handler `bindAncestorScrollEvents()` already wrote during
   *  `super()`, back to `null`, every single time. `destroy()` then finds `null` and never calls
   *  `removeEventListener`, leaking the capture-phase document listener (and everything its closure
   *  retains) for the life of the process. `declare` tells TS to skip emitting any field
   *  initialization for this property at all — construction-time assignment inside
   *  `bindAncestorScrollEvents()` is the only writer, and it survives `super()` returning.
   *  Confirmed empirically: without `declare`, `this.ancestorScrollHandler` is `null` immediately
   *  after `super()` returns even though `bindAncestorScrollEvents()` just set it; with `declare`,
   *  it holds the real handler. */
  declare private ancestorScrollHandler: ((e: Event) => void) | null;

  override bindAncestorScrollEvents(): void {
    const handler = (e: Event): void => {
      const target = e.target;
      if (
        this._viewport?.includes(target as HTMLDivElement) ||
        (target instanceof Node && this._container && target.contains(this._container))
      ) {
        this.handleActiveCellPositionChange();
      }
    };
    this.ancestorScrollHandler = handler;
    document.addEventListener('scroll', handler, true);
  }

  // Upstream keeps one <style> per grid and mutates its rules; see the block above ensureColumnRules.
  // `rtl` keeps upstream's sheet (no call site sets it).
  protected override createCssRules(): void {
    if (this._options.rtl) {
      super.createCssRules();
      return;
    }
    ensureColumnRules(this.columns.length);
    const o = this._options;
    const box = this._container;
    box.classList.add('kira-sg');
    box.style.setProperty('--sg-top-panel-h', `${o.topPanelHeight}px`);
    box.style.setProperty('--sg-preheader-h', `${o.preHeaderPanelHeight}px`);
    box.style.setProperty('--sg-topheader-h', `${o.topHeaderPanelHeight}px`);
    box.style.setProperty('--sg-headerrow-h', `${o.headerRowHeight}px`);
    box.style.setProperty('--sg-footerrow-h', `${o.footerRowHeight}px`);
    box.style.setProperty('--sg-row-h', `${o.rowHeight}px`);
    box.style.setProperty(
      '--sg-cell-h',
      o.enableVariableRowHeight
        ? `calc(100% - ${this.cellHeightDiff}px)`
        : `${(o.rowHeight ?? 0) - this.cellHeightDiff}px`,
    );
  }

  protected override removeCssRules(): void {
    if (this._options.rtl) super.removeCssRules();
  }

  /** P182 stage 1 — see the class comment. `declare`d: runs inside `super()`. */
  declare private shimmedViewport: HTMLDivElement | null;

  protected override setScroller(): void {
    super.setScroller();
    if (this._options.rtl) return;
    const header = this._headerScrollContainer;
    const viewport = this._viewportScrollContainerY;
    if (header.parentElement !== viewport) viewport.prepend(header);
    header.classList.add('kira-sticky-header');
    viewport.classList.add('kira-scroll-viewport');
    viewport.parentElement?.classList.add('kira-scroll-pane');
    if (this.shimmedViewport === viewport) return;
    Object.defineProperty(viewport, 'getBoundingClientRect', {
      configurable: true,
      value: (): DOMRect => {
        const r = Element.prototype.getBoundingClientRect.call(viewport);
        const inset = Math.min(header.offsetHeight, r.height);
        return new DOMRect(r.x, r.y + inset, r.width, r.height - inset);
      },
    });
    this.shimmedViewport = viewport;
  }

  protected override applyColumnWidths(): void {
    if (this._options.rtl) {
      super.applyColumnWidths();
      return;
    }
    const frozen = this._options.frozenColumn;
    const style = this._container.style;
    const hasFrozen = frozen !== undefined && frozen !== -1;
    style.setProperty(
      '--sg-scroll-canvas-w',
      `${hasFrozen ? this.canvasWidthR : this.canvasWidth}px`,
    );
    let x = 0;
    for (let i = 0; i < this.columns.length; i++) {
      const column = this.columns[i];
      if (!column?.hidden) {
        const w = column?.width || 0;
        const canvasWidth =
          frozen !== undefined && frozen !== -1 && i > frozen
            ? this.canvasWidthR
            : this.canvasWidthL;
        style.setProperty(`--sg-l${i}`, `${x}px`);
        style.setProperty(`--sg-r${i}`, `${canvasWidth - x - w}px`);
        if (frozen !== i) x += column?.width ?? 0;
      }
      if (frozen === i) x = 0;
    }
    if (frozen !== undefined && frozen !== -1) {
      style.setProperty('--sg-pin-w', `${this.canvasWidthL}px`);
    }
  }

  /** P182 stage 2 — with `frozenColumn: 0` (gutter = column 0), re-homes the gutter canvas
   *  (`_canvasTopL`) into the scroll viewport beside the data canvas as one sticky column and hides
   *  the emptied left pane. SlickGrid's frozen-column bookkeeping (row clones, `scrollCellIntoView`,
   *  always-rendered gutter, range selector offsets) is untouched. Idempotent: `updateColumnsInternal`
   *  calls this on every `setColumns`. Re-check on a `slickgrid` bump: it still runs from
   *  `finishInitialization` and `updateColumnsInternal`; `internalScrollColumnIntoView`'s body is
   *  unchanged. */
  protected override setPaneFrozenClasses(): void {
    super.setPaneFrozenClasses();
    if (this._options.rtl || !this.hasFrozenColumns() || this.hasFrozenRows) return;
    if (this._container.classList.contains('kira-sticky-gutter')) return;
    const body = document.createElement('div');
    body.className = 'kira-scroll-body';
    this._canvasTopR.before(body);
    body.append(this._canvasTopL, this._canvasTopR);
    this._container.classList.add('kira-sticky-gutter');
  }

  /** P182 stage 2 — the sticky gutter hides the leftmost `canvasWidthL` px of the viewport, so the
   *  visible data band is that much narrower than stock assumes. */
  protected override internalScrollColumnIntoView(left: number, right: number): void {
    if (!this._container.classList.contains('kira-sticky-gutter')) {
      super.internalScrollColumnIntoView(left, right);
      return;
    }
    const viewport = this._viewportScrollContainerX;
    const band = viewport.clientWidth - this.canvasWidthL;
    if (left < this.scrollLeft) {
      viewport.scrollLeft = left;
      this.handleScroll();
      this.render();
    } else if (right > this.scrollLeft + band) {
      viewport.scrollLeft = Math.min(left, right - band);
      this.handleScroll();
      this.render();
    }
  }

  /** SlickGrid's own `destroy()` never clears `this.initialized` and nulls
   *  ~60 internal element references (dist/esm/index.js:7674-7700, :7723-7725), so a catch-up
   *  render armed by `getRenderedRange` and still pending when the host unmounts would re-enter
   *  `render()` against a torn-down grid, passing the `!this.initialized` guard because that flag
   *  is still `true` and dereferencing a nulled element. Cancel the pending rAF (and stop it from
   *  re-arming) before handing off to the real teardown. */
  override destroy(shouldDestroyAllElements?: boolean): void {
    // The fix above's other half — remove with the exact capture flag it was added with, before
    // `super.destroy()` ever gets a chance to null `_viewport`/`_container` out from under it.
    if (this.ancestorScrollHandler) {
      document.removeEventListener('scroll', this.ancestorScrollHandler, true);
      this.ancestorScrollHandler = null;
    }
    if (this.shimmedViewport) Reflect.deleteProperty(this.shimmedViewport, 'getBoundingClientRect');
    this.shimmedViewport = null;
    if (this.chaseHandle) cancelAnimationFrame(this.chaseHandle);
    this.chaseHandle = 0;
    this.chaseWanted = false;
    this.chaseSeenSeq = -1;
    super.destroy(shouldDestroyAllElements);
  }

  override getRenderedRange(
    viewportTop?: number,
    viewportLeft?: number,
  ): { top: number; bottom: number; leftPx: number; rightPx: number } {
    // Defensive reads below, not stylistic ones: `SlickGrid`'s own constructor calls `init()`
    // (unless `explicitInitialization` is set), which renders synchronously — i.e. calls this
    // override — *before* a JS subclass's own field initialisers run (those run only after
    // `super()` returns). `this.velocity`/`this.mountedColumnCount` are genuinely `undefined` on
    // that first, base-constructor-triggered call; every read of a subclass field here has to
    // tolerate that, not just default it once at the field declaration (confirmed empirically —
    // `TypeError: this.velocity is not a function` from inside the `super(...)` call otherwise).
    // `this.chaseWanted`/`this.lastRenderedRowBounds` are read the same defensive way below, for
    // the identical reason.
    const range = super.getVisibleRange(viewportTop, viewportLeft);
    const { pxPerFrame, direction } = this.velocity
      ? this.velocity()
      : { pxPerFrame: 0, direction: 0 as const };
    // P22 iter2-onset D3 — report the runway's own velocity input, the one term in this whole
    // computation no trace in this investigation's history could see. Reported here rather than in
    // `render()` because this is where the number exists, and because `render()` is the *only*
    // caller of this override in the shipped configuration (slickgrid's other three
    // `getRenderedRange()` call sites, dist/esm/index.js:4760/:4776/:5620, are the RowDetailView and
    // grouping plugins, neither of which this app registers) — so it stays 1:1 with a render pass.
    scrollTrace.noteRunwayVelocity(pxPerFrame);
    const dataLength = this.getDataLength();
    const rowHeight = this.getOptions().rowHeight ?? 28;

    // The 9-step algorithm, restructured from the single `rowRangeBounds` call this override used
    // to make directly.
    //
    // Step 1: the strictly-visible range, always returned in full — nothing currently on screen is
    // ever deferred to a later frame.
    const mustStart = Math.max(0, range.top);
    const mustEnd = Math.min(range.bottom, dataLength - 1);

    // Step 2: today's full computation (visible + velocity-scaled runway), unchanged — the target
    // this call would return in full if there were no per-call budget at all.
    const target = rowRangeBounds(
      { startIndex: range.top, endIndex: range.bottom, count: dataLength },
      rowHeight,
      pxPerFrame,
      direction,
      this.mountedColumnCount || 1,
      runwayConfig(),
    );

    // Step 3: the previous call's returned range — a reliable proxy for "what's currently in
    // SlickGrid's own rowsCache" (cleanupRows keeps exactly the previously-returned range,
    // enableCellRowSpan is off — P22-slickgrid-migration-plan.md F2's own citation).
    const prev = this.lastRenderedRowBounds ?? { start: 0, end: -1 };

    // Step 4: how many rows the non-negotiable floor alone requires building fresh this call.
    const mustNewRows = countNewRows(mustStart, mustEnd, prev.start, prev.end);

    // Step 5: the per-call budget, in rows — MAX_NEW_CELLS_PER_RENDER (or its runtime override),
    // divided by however many columns are currently mounted (mirrors CELL_BUDGET's own divisor,
    // columns.ts's rowRangeBounds).
    const tuning = window.__kiraGridTuning;
    const maxNewCells = tuning?.maxNewCellsPerRenderOverride ?? MAX_NEW_CELLS_PER_RENDER;
    const budgetRows = Math.floor(maxNewCells / Math.max(1, this.mountedColumnCount || 1));

    let start: number;
    let end: number;
    if (mustEnd < mustStart) {
      // No data, or nothing visible — nothing to render.
      start = 0;
      end = -1;
    } else if (mustNewRows >= budgetRows) {
      // Step 6: even the non-negotiable floor is expensive on this table/window combination this
      // call — accept a viewport-bounded (not fling-distance-bounded) cost, and defer *all* extra
      // runway to the chase rather than spending what's left of the budget trying to also add lead.
      start = mustStart;
      end = mustEnd;
    } else {
      // Step 7: expand outward from `must` toward `target`, lead side first (today's existing
      // direction bias — rowRangeBounds' own [trailRows, leadRows]/[leadRows, trailRows]
      // asymmetry), until either `target` is reached or the remaining budget is exhausted. A row
      // already inside `prev` costs nothing to add (SlickGrid skips it — it's already in
      // rowsCache); only a genuinely new row spends budget, so growth only stalls when it reaches
      // un-cached territory.
      //
      // The *runway* portion of this budget is additionally capped by
      // MAX_NEW_LEAD_CELLS_PER_RENDER (or its override), separately from budgetRows above (which
      // stays the absolute per-pass ceiling and step 6's own floor short-circuit). Defaulted equal
      // to MAX_NEW_CELLS_PER_RENDER, so `leadBudgetRows` never binds tighter than `budgetRows -
      // mustNewRows` unless a real-hardware A/B (docs/PERF.md) overrides it — this
      // `min` is then a no-op.
      const maxLeadCells =
        tuning?.maxNewLeadCellsPerRenderOverride ?? MAX_NEW_LEAD_CELLS_PER_RENDER;
      const leadBudgetRows = Math.floor(maxLeadCells / Math.max(1, this.mountedColumnCount || 1));
      let remaining = Math.min(budgetRows - mustNewRows, leadBudgetRows);
      start = mustStart;
      end = mustEnd;
      const growEnd = (limit: number) => {
        while (end < limit) {
          const next = end + 1;
          if (next < prev.start || next > prev.end) {
            if (remaining <= 0) break;
            remaining -= 1;
          }
          end = next;
        }
      };
      const growStart = (limit: number) => {
        while (start > limit) {
          const next = start - 1;
          if (next < prev.start || next > prev.end) {
            if (remaining <= 0) break;
            remaining -= 1;
          }
          start = next;
        }
      };
      // direction < 0 (scrolling up/backward) leads on the start side, matching rowRangeBounds' own
      // [leadRows, trailRows] assignment there; direction >= 0 (forward or at rest) leads on the end
      // side, matching its [trailRows, leadRows].
      if (direction < 0) {
        growStart(target.start);
        growEnd(target.end);
      } else {
        growEnd(target.end);
        growStart(target.start);
      }
    }

    // Step 8: if the returned range is narrower than `target` on either side, want a catch-up
    // render — every call (real-scroll- or catch-up-driven) recomputes `must`/`target` fresh from
    // the grid's *current* scroll position, so there is no queue of stale ranges to reconcile, only
    // "how far is `prev` from `target`, right now." `chaseWanted` is recomputed on every call (not
    // just latched true once), so a call that lands after the deficit has already closed clears it
    // again; `scheduleChase()` is the gate that decides *when* a wanted chase is allowed to render
    // (see its own comment).
    this.chaseWanted = end >= start && (start > target.start || end < target.end);
    if (this.chaseWanted) this.scheduleChase();

    // Step 9: the range actually returned this call — read by step 3 on the *next* call.
    this.lastRenderedRowBounds = { start, end };
    // getCanvasNode() with no args resolves column 0 — this app's frozen gutter,
    // whose own pane is a fixed GUTTER_WIDTH wide, not the scrollable data pane whose width this
    // clamp needs. Column index 1 (the first data column) is always > frozenColumn (0), so it
    // resolves the right/scrollable canvas instead — see `_getContainerElement`'s own
    // `isRightSide = hasFrozenColumns() && idx > frozenColumn` test, read from source. It may not
    // exist yet on that same first, pre-subclass-field render (no data column mounted yet).
    const canvasWidth = this.getCanvasNode(1)?.clientWidth ?? 0;
    const { leftPx, rightPx } = clampColumnOverscan(
      range.leftPx,
      range.rightPx,
      OVERSCAN_PX,
      canvasWidth,
    );
    // The row axis's own budget divisor, self-maintained rather than supplied by the host:
    // the deleted DataGrid.vue's own mountedColumnCount was read from a *separate* column
    // virtualizer, and its own comment warned that calling into it from inside the row-range
    // computation regressed the scroll budget. SlickGrid has no separate column virtualizer to call
    // into — this approximates the mounted column count from this same render's own column window
    // and the grid's average column width, entirely locally, so the hazard that comment warns about
    // does not apply here at all. One frame stale by construction (like the app's own precedent):
    // it's set for the *next* call, from *this* call's own leftPx/rightPx.
    const columns = this.getColumns() ?? [];
    if (this.avgColumnWidthDirty || this.avgColumnWidthColumns !== columns) {
      const totalWidth = columns.reduce((sum, c) => sum + (c.width ?? 0), 0) || 1;
      this.avgColumnWidthCache = totalWidth / Math.max(1, columns.length);
      this.avgColumnWidthColumns = columns;
      this.avgColumnWidthDirty = false;
    }
    this.mountedColumnCount = Math.max(1, Math.ceil((rightPx - leftPx) / this.avgColumnWidthCache));
    return { top: start, bottom: end, leftPx, rightPx };
  }

  /** Re-reads the viewport's scroll offsets and renders for them: after a caller set
   *  `scrollTop`/`scrollLeft` directly (restoring a saved position), before the scroll event lands. */
  syncToViewportScroll(): void {
    this.handleScroll();
    this.render();
  }

  /** `render()` is fully synchronous (no `await`, no `setTimeout`, no `requestIdleCallback`
   *  anywhere between `_handleScroll` and `onRendered` firing), so the caller already has the
   *  duration in hand; report it straight to `scrollTrace.noteRenderMs` instead of approximating
   *  it through a Vue `nextTick` (meaningless here — there is no Vue patch on this render path at
   *  all). This is also the seam: the batch-capping/chase logic lives in `getRenderedRange`,
   *  called from inside `super.render()` below, so timing wraps the whole call, chase-scheduled
   *  catch-ups included. */
  override render(): void {
    const start = performance.now();
    window.__kiraGridScrollWorkStart?.(start);
    super.render();
    scrollTrace.noteRenderMs(performance.now() - start);
  }
}
