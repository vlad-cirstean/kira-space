// recordOffsetSample/velocity, byte-identical across SlickGridHost.vue/
// ConsoleSlickGrid.vue (the latter's own comment already called it "Mirrors SlickGridHost.vue's
// own onScroll velocity sampler verbatim"). Plain mutable state, not refs — read only from
// KiraSlickGrid's own `velocity` callback, itself called only from inside getRenderedRange
// (entirely outside Vue's reactivity graph); a Pinia/ref-backed version would pay reactivity
// tracking cost on every render pass for no consumer that needs it.
const MAX_PLAUSIBLE_ROW_VELOCITY_PX_PER_FRAME = 800;

export interface ScrollVelocityTracker {
  recordOffsetSample(offset: number, now: number): void;
  velocity(): { pxPerFrame: number; direction: 1 | -1 | 0 };
  /** `grid.lastScrollEventAt`'s own reader (P22 iter2-pacing D1's chase quiescence gate). */
  lastScrollEventAt(): number;
  /** Seeds the sampler's baseline at mount (after the restored scroll
   *  position is applied, before the first render), so the *first* gesture in a tab's life has a
   *  `prev` sample to diff against. A raw set, not recordOffsetSample: at mount `lastOffset` is
   *  still its initial 0, and a fresh tab's own scrollTop is also 0, so the dedupe there would
   *  otherwise skip seeding entirely on that one case. */
  seed(offset: number, now: number): void;
}

/** `getViewportEl` mirrors each host's own `viewportEl` module variable — read lazily since it's
 *  null until the grid mounts. */
export function createScrollVelocityTracker(
  getViewportEl: () => HTMLElement | null,
): ScrollVelocityTracker {
  let lastOffset = 0;
  let lastOffsetT = 0;
  let prevOffset = 0;
  let prevOffsetT = 0;

  function freshVelocitySample(): boolean {
    return window.__kiraGridTuning?.freshVelocitySampleOverride ?? true;
  }

  // The dedupe is what makes pulling safe: a scroll event that did not move the vertical offset is
  // not a velocity sample (a *horizontal* scroll fires this same listener), and without it a
  // listener-driven sample landing after a pull of the same position would shift `prev` up to
  // `last` and read the next frame's delta as 0.
  function recordOffsetSample(offset: number, now: number): void {
    if (freshVelocitySample() && offset === lastOffset) return;
    prevOffset = lastOffset;
    prevOffsetT = lastOffsetT;
    lastOffset = offset;
    lastOffsetT = now;
  }

  // P22 iter2-onset D1 — sample at the point of consumption, not one listener too late (see
  // recordOffsetSample above). This adds one `scrollTop` read per render pass; it is inside the
  // envelope getRenderedRange already works in, which does its own layout read
  // (`getCanvasNode(1)?.clientWidth`) and is reached from `_handleScroll`, which has just read
  // scrollHeight/clientHeight/scrollWidth/clientWidth off the same element (dist/esm/index.js:10576)
  // — layout is already flushed at this point on the scroll path.
  function velocity(): { pxPerFrame: number; direction: 1 | -1 | 0 } {
    const viewportEl = getViewportEl();
    if (freshVelocitySample() && viewportEl) {
      recordOffsetSample(viewportEl.scrollTop, performance.now());
    }
    const dt = lastOffsetT - prevOffsetT;
    if (!prevOffsetT || dt <= 0 || performance.now() - lastOffsetT > 150) {
      return { pxPerFrame: 0, direction: 0 };
    }
    const delta = lastOffset - prevOffset;
    const pxPerFrame = Math.abs(delta);
    if (pxPerFrame > MAX_PLAUSIBLE_ROW_VELOCITY_PX_PER_FRAME)
      return { pxPerFrame: 0, direction: 0 };
    return { pxPerFrame, direction: delta > 0 ? 1 : delta < 0 ? -1 : 0 };
  }

  function lastScrollEventAt(): number {
    return lastOffsetT;
  }

  function seed(offset: number, now: number): void {
    lastOffset = offset;
    lastOffsetT = now;
  }

  return { recordOffsetSample, velocity, lastScrollEventAt, seed };
}
