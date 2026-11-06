// P22 iter2 D2: a real-fling scroll trace, driven by a human on real hardware — see
// docs/HISTORY.md (v1.1 P22)/§7.3. Every scroll
// measurement anywhere else in this repo (tests/ui/support/measure.ts included) drives `scrollTop`
// from the main thread, so the DOM's scroll offset and the main thread's knowledge of it are
// necessarily the same value — the condition that produces the user's real symptom (the compositor
// showing a position the main thread hasn't rendered yet, since WebKit's scrolling thread moves the
// composited layer independently of the main thread during a momentum scroll) can never occur in
// that harness. This module has no such limitation: it reads the *live* DOM during a *real*
// trackpad fling, from inside the packaged app's own DevTools (View → Open DevTools, a dev build —
// internal/shell/menutemplate.go). It is not a tests/ui/ instrument and is not gated in CI; it exists
// so a human can capture a number nobody in this repo's history has ever measured. Reachable at
// `window.__kiraScrollTrace` (wired from SlickGridHost.vue's registerGrid/noteScrollEvent/
// noteRenderMs calls — not from columns.ts's offset observer; see the deleted DataGrid.vue's own
// onScroll comment for why).
//
// Usage (Web Inspector console, one hard two-finger flick between the two calls):
//   __kiraScrollTrace.start()
//   copy(JSON.stringify(__kiraScrollTrace.stop()))
import { copyOrReportError } from '@workbench/behaviour/clipboard';
import { logWarn } from '@workbench/bridge/log';

interface ScrollTraceEvent {
  /** el.scrollTop at the moment this native `scroll` event was handled. */
  offset: number;
  /** performance.now() at the moment this native `scroll` event was handled. */
  t: number;
  /**
   * Whether this event's own timestamp falls after the rAF tick it got bucketed into.
   *
   * Correction: this field's ordering premise, as originally written here,
   * was that `scroll` always dispatches *before* "run the animation frame callbacks" in the same
   * rendering update (per the HTML spec's own step order), so `true` should be rare and only
   * `requestAnimationFrame`-timestamp jitter. That premise does not hold universally — this
   * investigation's own sandbox reproduction found `afterRaf: true` on essentially *every* event
   * under a wheel/compositor-driven scroll in WebKit (`tick, chase,
   * scrollEvent, scrollRender` in 131 of 131 doubled frames), the opposite ordering from a
   * `scrollTop +=` write driven from inside a rAF callback. The ordering is engine- and
   * input-path-dependent, not a fixed spec guarantee this app can rely on. The
   * fix (`CHASE_QUIET_MS`) deliberately does not depend on which ordering a given frame has —
   * see kiraSlickGrid.ts's own `scheduleChase` comment. A real-Mac trace's `afterRaf` values are
   * therefore the first honest reading of this field on the real target platform; `true` there is
   * expected, not a bug signal, until proven otherwise.
   */
  afterRaf: boolean;
}

interface ScrollTraceFrame {
  /** requestAnimationFrame's own timestamp for this tick. */
  t: number;
  /** Native `scroll` events observed since the previous rAF tick. F2's direct, real-hardware test
   *  — if this is ever > 1, a browser really did fire more than one scroll per frame here, and the
   *  premise pass 1 assumed (and this phase's D1 refuted from source/spec) needs revisiting. */
  scrollEvents: number;
  scrollTopAtEvent: ScrollTraceEvent[];
  /** |Δoffset| since the previous rAF tick, in px — the number nobody in this repo has measured
   *  from a real macOS momentum scroll (the plan's F5). */
  pxPerFrame: number;
  /** Whether either virtualizer's onChange fired since the previous rAF tick. */
  notified: boolean;
  mountedTop: number;
  mountedBottom: number;
  liveScrollTop: number;
  clientHeight: number;
  /** max(0, mountedTop − liveScrollTop) + max(0, liveScrollTop + clientHeight − mountedBottom) —
   *  the symptom's own metric: how much of the viewport the mounted row band fails to cover, read
   *  from the *live* DOM during a *real* scroll (unlike every sandbox measurement of this same
   *  quantity, which cannot see the compositor-ahead condition that makes it non-zero — see this
   *  module's own header comment). */
  uncoveredPx: number;
  /** P22 iter2-pacing D3: wall time since the previous rAF tick, in px-free ms — the frame-
   *  duration series the "smooth, ideally 60fps, but it can be lower with proper pacing" goal is
   *  stated in directly. 0 on the first frame of a recording (no previous tick to diff against). */
  frameMs: number;
  /** How many render passes (Vue notify-and-flush cycles, or KiraSlickGrid
   *  render() calls) completed since the previous tick. The pacing bug's own direct signal: a
   *  self-scheduled catch-up render sharing a frame with a scroll-driven one shows up here as > 1
   *  — invisible before this field existed. */
  renderCount: number;
  /** The SUM of this frame's render passes' own durations (Vue: nextTick wall time per notify;
   *  SlickGrid: KiraSlickGrid.render()'s own performance.now() delta per call) — not the most
   *  recent pass's duration carried forward unreset into every later frame. Resets to 0 at the top
   *  of every tick, so a frame with `renderCount === 0` reports `renderMs === 0` rather than
   *  silently repeating whatever the previous render cost. */
  renderMs: number;
  /**
   * The largest velocity, in px/frame, that any of this frame's render passes
   * fed into the runway arithmetic (`columns.ts`'s `rowRangeBounds`). 0 when nothing
   * rendered this frame, and 0 when the render itself read "at rest".
   *
   * Deliberately *not* the same quantity as `pxPerFrame` above, which is what the viewport
   * moved: the gap between the two is the runway's own **input lag**, and it is the one input to
   * the whole runway computation that no trace in this investigation's history has ever been able
   * to see. A frame that renders with `runwayVelocity === 0` while `pxPerFrame` is a plausible
   * fling delta is a render that sized its runway as if the grid were standing still — the
   * gesture-onset defect (`summary.staleVelocityFrames` tallies
   * exactly those frames). Reported by `KiraSlickGrid.getRenderedRange`, the only place this app
   * computes it; always 0 for the incumbent tanstack engine, whose own `rangeExtractor` has no
   * equivalent reporting seam.
   */
  runwayVelocity: number;
  /** Mounted [data-testid="grid-row"] count. */
  rows: number;
}

interface ScrollTraceStats {
  p50: number;
  p95: number;
  max: number;
  /** P22 iter2-pacing D3: pacing is a statement about spread, not about a percentile — p50/p95/max
   *  of a metric can't answer "how uneven is this", so both moments are reported alongside them. */
  mean: number;
  stddev: number;
}

interface ScrollTraceSummary {
  pxPerFrame: ScrollTraceStats;
  uncoveredPx: ScrollTraceStats;
  renderMs: ScrollTraceStats;
  /** P22 iter2-pacing D3: the frame-duration series' own stats — see ScrollTraceFrame.frameMs. */
  frameMs: ScrollTraceStats;
  /** scrollEvents-per-frame → how many frames saw that count. F2's direct test, tallied. */
  scrollEventsHistogram: Record<number, number>;
  /** P22 iter2-pacing D3: renderCount-per-frame → how many frames saw that count, mirroring
   *  scrollEventsHistogram. A key >= 2 anywhere during a live scroll is the doubling this phase's
   *  D1 fix exists to remove — T1/T3 (tests/ui/slick-grid.spec.ts) gate exactly this field. */
  renderCountHistogram: Record<number, number>;
  /** The runway's own velocity input, as a series — see
   *  ScrollTraceFrame.runwayVelocity. Compare its `mean`/`p95` against `pxPerFrame`'s: the runway
   *  is sized from the former while the viewport moves at the latter, so a persistent shortfall
   *  here is a persistent runway shortfall. */
  runwayVelocity: ScrollTraceStats;
  /**
   * How many frames rendered *while the viewport was measurably moving* but
   * sized their runway at zero velocity (`renderCount > 0 && pxPerFrame > 0 && runwayVelocity ===
   * 0`). The gesture-onset defect's own direct signal — before the fix this is ~one frame per
   * rest-to-motion transition (the first render of every fling, which read a scroll sample taken
   * before the gesture began); after it, it should be 0 for a real fling.
   *
   * One honest caveat for a real-Mac reading: the host's own sampler deliberately reports "at rest"
   * for a delta above `MAX_PLAUSIBLE_ROW_VELOCITY_PX_PER_FRAME` (800 px/frame — a scrollbar click
   * or a programmatic jump, not a fling), so a genuine discrete jump also lands in this count. Read
   * it alongside `pxPerFrame.max`; a fling that never exceeds 800 px/frame has no such frames.
   */
  staleVelocityFrames: number;
}

export interface ScrollTraceResult {
  frames: ScrollTraceFrame[];
  summary: ScrollTraceSummary;
}

let recording = false;
let rafId = 0;
let gridEl: HTMLElement | null = null;
// The mounted-row selector `measureMountedBand` queries — the default
// `[data-testid="grid-row"]` is still the correct value: SlickGridHost.vue's own rows carry the
// same testid, see its own comment.
let mountedRowSelector = '[data-testid="grid-row"]';

let pendingEvents: { offset: number; t: number }[] = [];
let pendingNotified = false;
// Accumulators, drained and reset by every tick() — see that function's own
// comment for why this replaces the old sticky `lastRenderMs` (never reset, so a no-render frame
// reported the *previous* frame's duration and a doubled-render frame reported only its second
// render's cost).
let pendingRenderMs = 0;
let pendingRenderCount = 0;
// P22 iter2-onset D3: drained and reset by every tick(), exactly like the two accumulators above.
let pendingRunwayVelocity = 0;
let prevRafT = 0;
let prevLiveScrollTop = 0;
let frames: ScrollTraceFrame[] = [];

/** Called from SlickGridHost.vue's own onMounted/onUnmounted — at most one grid is ever mounted at
 *  a time (MainView.vue keys its DataView by tab id), so a single module-level target is enough.
 *  `rowSelector` is the mounted-row query `measureMountedBand` below uses — defaults to
 *  `[data-testid="grid-row"]`. */
export function registerGrid(el: HTMLElement, rowSelector = '[data-testid="grid-row"]'): void {
  gridEl = el;
  mountedRowSelector = rowSelector;
}

export function unregisterGrid(el: HTMLElement): void {
  if (gridEl === el) gridEl = null;
}

/** Called from the grid's own onScroll (SlickGridHost.vue today; the deleted DataGrid.vue before
 *  it) on every native `scroll` event — a single array push behind a boolean when not recording,
 *  per D2's own "inert until start()" requirement. */
export function noteScrollEvent(offset: number, t: number): void {
  if (!recording) return;
  pendingEvents.push({ offset, t });
}

/** For an engine whose render pass is fully synchronous (SlickGrid — no Vue patch/flush involved
 *  on this path at all), the caller already has the duration in hand; report it directly rather
 *  than approximating it through a Vue `nextTick` the way the deleted DataGrid.vue's own
 *  markScrollWork did. Called from KiraSlickGrid's own `render()` override — including a
 *  chase-scheduled catch-up render, so two calls landing in the same frame both accumulate rather
 *  than one clobbering the other. */
export function noteRenderMs(ms: number): void {
  if (!recording) return;
  pendingNotified = true;
  pendingRenderMs += ms;
  pendingRenderCount++;
}

/** Called from `KiraSlickGrid.getRenderedRange` with the velocity that call handed
 *  `rowRangeBounds`. Keeps the *largest* value seen this frame rather than the last:
 *  a quiescent catch-up render legitimately reads 0, and a frame that contains both it and a
 *  scroll-driven render must not report the runway as having been sized at rest. See
 *  ScrollTraceFrame.runwayVelocity for what the number is and is not. */
export function noteRunwayVelocity(pxPerFrame: number): void {
  if (!recording) return;
  if (pxPerFrame > pendingRunwayVelocity) pendingRunwayVelocity = pxPerFrame;
}

function measureMountedBand(el: HTMLElement): { top: number; bottom: number; rows: number } {
  const rows = el.querySelectorAll<HTMLElement>(mountedRowSelector);
  if (rows.length === 0) return { top: 0, bottom: 0, rows: 0 };
  let top = Number.POSITIVE_INFINITY;
  let bottom = Number.NEGATIVE_INFINITY;
  for (const row of rows) {
    if (row.offsetTop < top) top = row.offsetTop;
    const end = row.offsetTop + row.offsetHeight;
    if (end > bottom) bottom = end;
  }
  return { top, bottom, rows: rows.length };
}

function tick(rafT: number): void {
  if (!recording) return;

  const events = pendingEvents;
  pendingEvents = [];
  const notified = pendingNotified;
  pendingNotified = false;
  // P22 iter2-pacing D3: drain-and-reset, every tick, unconditionally — the fix for the sticky
  // `lastRenderMs` bug. A frame with no render this tick sees renderCount 0 / renderMs 0, not
  // whatever the previous frame happened to leave behind.
  const renderMs = pendingRenderMs;
  pendingRenderMs = 0;
  const renderCount = pendingRenderCount;
  pendingRenderCount = 0;
  const runwayVelocity = pendingRunwayVelocity;
  pendingRunwayVelocity = 0;
  const frameMs = prevRafT ? rafT - prevRafT : 0;
  prevRafT = rafT;

  const el = gridEl;
  const liveScrollTop = el?.scrollTop ?? 0;
  const clientHeight = el?.clientHeight ?? 0;
  const band = el ? measureMountedBand(el) : { top: 0, bottom: 0, rows: 0 };
  const canvasTop = el?.querySelector<HTMLElement>('.grid-canvas')?.offsetTop ?? 0;
  const uncoveredPx = el
    ? Math.max(0, band.top - liveScrollTop) +
      Math.max(0, liveScrollTop + clientHeight - canvasTop - band.bottom)
    : 0;
  const pxPerFrame = Math.abs(liveScrollTop - prevLiveScrollTop);
  prevLiveScrollTop = liveScrollTop;

  frames.push({
    t: rafT,
    scrollEvents: events.length,
    scrollTopAtEvent: events.map((e) => ({ offset: e.offset, t: e.t, afterRaf: e.t > rafT })),
    pxPerFrame,
    notified,
    mountedTop: band.top,
    mountedBottom: band.bottom,
    liveScrollTop,
    clientHeight,
    uncoveredPx,
    frameMs,
    renderCount,
    renderMs,
    runwayVelocity,
    rows: band.rows,
  });

  rafId = requestAnimationFrame(tick);
}

function percentile(values: number[], p: number): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const idx = Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length));
  return sorted[idx] ?? 0;
}

function stats(values: number[]): ScrollTraceStats {
  const mean = values.length ? values.reduce((sum, v) => sum + v, 0) / values.length : 0;
  const variance = values.length
    ? values.reduce((sum, v) => sum + (v - mean) ** 2, 0) / values.length
    : 0;
  return {
    p50: percentile(values, 50),
    p95: percentile(values, 95),
    max: values.length ? Math.max(...values) : 0,
    mean,
    stddev: Math.sqrt(variance),
  };
}

function summarize(fr: ScrollTraceFrame[]): ScrollTraceSummary {
  const scrollEventsHistogram: Record<number, number> = {};
  const renderCountHistogram: Record<number, number> = {};
  let staleVelocityFrames = 0;
  for (const f of fr) {
    scrollEventsHistogram[f.scrollEvents] = (scrollEventsHistogram[f.scrollEvents] ?? 0) + 1;
    renderCountHistogram[f.renderCount] = (renderCountHistogram[f.renderCount] ?? 0) + 1;
    if (f.renderCount > 0 && f.pxPerFrame > 0 && f.runwayVelocity === 0) staleVelocityFrames++;
  }
  return {
    pxPerFrame: stats(fr.map((f) => f.pxPerFrame)),
    uncoveredPx: stats(fr.map((f) => f.uncoveredPx)),
    renderMs: stats(fr.map((f) => f.renderMs)),
    frameMs: stats(fr.map((f) => f.frameMs)),
    scrollEventsHistogram,
    renderCountHistogram,
    runwayVelocity: stats(fr.map((f) => f.runwayVelocity)),
    staleVelocityFrames,
  };
}

export function start(): void {
  recording = true;
  frames = [];
  pendingEvents = [];
  pendingNotified = false;
  pendingRenderMs = 0;
  pendingRenderCount = 0;
  pendingRunwayVelocity = 0;
  prevRafT = 0;
  prevLiveScrollTop = gridEl?.scrollTop ?? 0;
  if (rafId) cancelAnimationFrame(rafId);
  rafId = requestAnimationFrame(tick);
}

export function stop(): ScrollTraceResult | null {
  if (!recording) {
    console.warn('__kiraScrollTrace.stop(): not recording — call start() first');
    return null;
  }
  recording = false;
  if (rafId) cancelAnimationFrame(rafId);
  rafId = 0;

  const result: ScrollTraceResult = { frames, summary: summarize(frames) };
  // Best-effort: the console return value is the primary output.
  void copyOrReportError(JSON.stringify(result), (message) =>
    logWarn('scrollTrace', 'clipboard write failed', message),
  );
  return result;
}
