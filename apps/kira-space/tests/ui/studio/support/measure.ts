import type { Page } from '@playwright/test';

// Ported from tests/e2e/support/measure.ts — `percentile`/`measureClickToDom`/
// `measureScrollResponses` are pure `page.evaluate()` DOM/MutationObserver timers with zero
// Electron dependency (confirmed by reading the original: its only Electron-coupled export is
// `uptimeMs`, which reads `ElectronApplication.evaluate(() => process.uptime())` for
// `startup.spec.ts` — a spec with no analogue in this tier). `uptimeMs` is dropped here rather than
// ported; everything else is byte-identical.

export function percentile(values: number[], p: number): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const idx = Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length));
  return sorted[idx];
}

/** Nearest-rank percentile: the smallest sample with at least p% of the samples at or below it.
 *  `percentile` above indexes `floor(p/100 * n)`, which is the maximum for p95 at n = 20, so a
 *  single GC pause or late frame (about 1 in 500 samples at idle) failed the gate. */
export function percentileNearestRank(values: number[], p: number): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.max(0, Math.ceil((p / 100) * sorted.length) - 1)];
}

/**
 * Clicks `click`, then resolves at the first MutationObserver callback under `observe` for which
 * `until` holds, returning ms since the synchronous click dispatch. MutationObserver callbacks are
 * microtasks, so resolution is sub-millisecond; a requestAnimationFrame poll would quantise every
 * result to a ~16.7 ms multiple and could never demonstrate a 50 ms budget, let alone 8 ms.
 * Paint is deliberately outside the measurement.
 */
export function measureClickToDom(
  page: Page,
  opts: {
    click: string;
    observe: string;
    until: { selector: string; text?: string; minCount?: number };
    timeoutMs?: number;
  },
): Promise<number> {
  const { click, observe, until, timeoutMs = 5000 } = opts;
  return page.evaluate(
    ({ click, observe, until, timeoutMs }) => {
      return new Promise<number>((resolve, reject) => {
        const clickEl = document.querySelector<HTMLElement>(click);
        const observeEl = document.querySelector<HTMLElement>(observe);
        if (!clickEl || !observeEl) {
          reject(
            new Error(`measureClickToDom: target not found (click=${click}, observe=${observe})`),
          );
          return;
        }

        function isDone(): boolean {
          if (until.minCount !== undefined) {
            return document.querySelectorAll(until.selector).length >= until.minCount;
          }
          const target = document.querySelector(until.selector);
          if (!target) return false;
          if (until.text !== undefined) return (target.textContent ?? '').includes(until.text);
          return true;
        }

        const timer = setTimeout(() => {
          observer.disconnect();
          reject(new Error(`measureClickToDom: timed out waiting for ${until.selector}`));
        }, timeoutMs);

        const observer = new MutationObserver(() => {
          if (!isDone()) return;
          clearTimeout(timer);
          observer.disconnect();
          resolve(performance.now() - start);
        });
        observer.observe(observeEl, { childList: true, subtree: true, characterData: true });

        const start = performance.now();
        clickEl.click();
      });
    },
    { click, observe, until, timeoutMs },
  );
}

export interface ScrollResponseDeltas {
  /** `KiraSlickGrid.render()` entry's __kiraGridScrollWorkStart mark → MutationObserver callback (see below). */
  workDeltas: number[];
  /** The original end-to-end number: synchronous trigger → MutationObserver callback (D5/D6). */
  e2eDeltas: number[];
}

/**
 * Per scroll step: returns both an end-to-end delta (synchronous `scrollTop` assignment → first
 * MutationObserver callback, same "trigger → DOM commit" shape D5 uses for every other budget in
 * this file) and a work-only delta that isolates the app's own main-thread work from browser
 * frame-scheduling delay.
 *
 * An earlier version gated the 8ms budget on the end-to-end number alone. That number conflates
 * two frame-scheduling waits that are not app work: a script-driven `scrollTop` change's own
 * `scroll` event is deferred to Chromium's next "update the rendering" step (the same per-frame
 * cadence requestAnimationFrame uses), and the pre-SlickGrid grid's onScroll deliberately coalesces bursty
 * scroll events by doing its actual state sync inside its own requestAnimationFrame callback — a
 * second hop stacked on the first. Confirmed by forcing a step to start right after a frame
 * boundary (via a double-rAF wait) and observing every sample jump to a full frame period.
 * SPEC.md's 8ms figure is explicitly "(120 Hz displays)" — a per-frame work budget, not a
 * display-independent latency — so gating on the end-to-end number was reintroducing exactly the
 * vsync floor D6 says a rAF-based measurement "can never" escape, on an environment with no real
 * 120Hz cadence to synthesize.
 *
 * The work-only delta starts its clock at the __kiraGridScrollWorkStart mark
 * (apps/kira-space/frontend/src/studio/main.ts's Window augmentation; called from the top of
 * `KiraSlickGrid.render()`, after the browser's scheduling hops and SlickGrid's own
 * scrollRenderThrottling wait have already resolved) instead of at
 * the synchronous property write, while keeping the same MutationObserver-based end signal. If no
 * mark ever arrives for a step (mark is only set once a test defines the hook, so this shouldn't
 * happen when called from budgets.spec.ts, but would if this were reused without wiring it up),
 * workDeltas falls back to the e2e start so the promise still resolves with a sane, if inflated,
 * number rather than a negative or NaN one.
 */
// P29 D13: one axis parameter, not a second function — this file's own header rule is that
// budgets.spec.ts/startup.spec.ts must both produce numbers with identical instrumentation so
// they stay comparable. Default 'vertical' keeps the existing call site (scrollTop) byte-for-byte
// unchanged.
export function measureScrollResponses(
  page: Page,
  gridSelector: string,
  steps: number,
  axis: 'vertical' | 'horizontal' = 'vertical',
): Promise<ScrollResponseDeltas> {
  return page.evaluate(
    ({ gridSelector, steps, axis }) => {
      const el = document.querySelector<HTMLElement>(gridSelector);
      if (!el) throw new Error(`measureScrollResponses: ${gridSelector} not found`);
      const prop = axis === 'horizontal' ? 'scrollLeft' : 'scrollTop';
      const total =
        axis === 'horizontal'
          ? Math.max(0, el.scrollWidth - el.clientWidth)
          : Math.max(0, el.scrollHeight - el.clientHeight);

      const w = window as unknown as { __kiraGridScrollWorkStart?: (t: number) => void };

      function step(target: number): Promise<{ work: number; e2e: number }> {
        return new Promise((resolve, reject) => {
          const timer = setTimeout(() => {
            w.__kiraGridScrollWorkStart = undefined;
            observer.disconnect();
            reject(new Error('measureScrollResponses: step timed out'));
          }, 5000);

          let workStart = 0;
          w.__kiraGridScrollWorkStart = (t: number) => {
            workStart = t;
          };

          const observer = new MutationObserver(() => {
            clearTimeout(timer);
            w.__kiraGridScrollWorkStart = undefined;
            observer.disconnect();
            const end = performance.now();
            resolve({ work: end - (workStart || start), e2e: end - start });
          });
          observer.observe(el as HTMLElement, { childList: true, subtree: true, attributes: true });

          const start = performance.now();
          (el as HTMLElement)[prop] = target;
        });
      }

      return (async () => {
        const workDeltas: number[] = [];
        const e2eDeltas: number[] = [];
        for (let i = 1; i <= steps; i++) {
          const { work, e2e } = await step(Math.round((total * i) / steps));
          workDeltas.push(work);
          e2eDeltas.push(e2e);
        }
        return { workDeltas, e2eDeltas };
      })();
    },
    { gridSelector, steps, axis },
  );
}

export interface SustainedScrollResult {
  /** The requested velocity this ladder rung was run at, echoed back for the caller's own logging. */
  velocity: number;
  /**
   * Per frame: how much of the viewport band the mounted `[data-testid="grid-row"]` band failed to
   * cover, in px, floored at 0 — read *at the moment of the write*, before any settle wait. The
   * symptom's own metric (P22 iter2 §5 D3).
   */
  uncoveredPx: number[];
}

/**
 * Measures rendered-band coverage *during* sustained scrolling, velocity-parameterised in px/frame
 * — replaces the `measureScrollCoverage`. That instrument
 * manufactured its own "many scroll events per frame" burst via `SUB_STEPS_PER_FRAME = 8` synthetic
 * `dispatchEvent(new Event('scroll'))` calls — a burst real browsers never produce (`scroll`
 * dispatches at most once per rendering update; docs/PERF.md methodology note, f28b25a's own
 * commit message)
 * — so its "before: up to 8/frame" numbers measured the harness's own constant, not the app.
 *
 * This drives one *real* `scroll` event per rAF (`el.scrollTop = next`, no synthetic dispatch) and
 * reads `uncoveredPx` synchronously in the same callback, before Vue's scheduler has had a chance
 * to react to it — no `await nextFrame()` settle wait, which is what let the `uncoveredPx === 0`
 * read as reassuring when it was structurally guaranteed to.
 *
 * This is still not proof the real symptom is gone: this harness drives `scrollTop` from the main
 * thread, so the DOM's scroll offset and the main thread's own knowledge of it are the same value
 * by construction — the condition that produces the symptom on real hardware (the compositor
 * showing a position the main thread hasn't rendered yet, since WebKit's scrolling thread moves the
 * composited layer independently of the main thread during a momentum scroll) cannot occur here,
 * at any velocity. Logged, not gated, for exactly that reason — a real measurement needs
 * `window.__kiraScrollTrace` on real hardware (views/shared/slick/scrollTrace.ts).
 */
export function measureSustainedScroll(
  page: Page,
  gridSelector: string,
  opts: { pxPerFrame: number; frames: number },
): Promise<SustainedScrollResult> {
  const { pxPerFrame, frames } = opts;
  return page.evaluate(
    ({ gridSelector, pxPerFrame, frames }) => {
      const found = document.querySelector<HTMLElement>(gridSelector);
      if (!found) throw new Error(`measureSustainedScroll: ${gridSelector} not found`);
      const el = found;
      const maxScrollTop = Math.max(0, el.scrollHeight - el.clientHeight);

      function uncoveredPx(): number {
        const rows = document.querySelectorAll<HTMLElement>('[data-testid="grid-row"]');
        const viewStart = el.scrollTop;
        // P182: the sticky header sits above the canvas inside the scroller; row offsets are canvas-relative.
        const canvasTop = el.querySelector<HTMLElement>('.grid-canvas')?.offsetTop ?? 0;
        const viewEnd = viewStart + el.clientHeight - canvasTop;
        if (rows.length === 0) return Math.max(0, viewEnd - viewStart);
        let mountedStart = Number.POSITIVE_INFINITY;
        let mountedEnd = Number.NEGATIVE_INFINITY;
        for (const row of rows) {
          if (row.offsetTop < mountedStart) mountedStart = row.offsetTop;
          const end = row.offsetTop + row.offsetHeight;
          if (end > mountedEnd) mountedEnd = end;
        }
        const above = Math.max(0, mountedStart - viewStart);
        const below = Math.max(0, viewEnd - mountedEnd);
        return above + below;
      }

      return new Promise<{ velocity: number; uncoveredPx: number[] }>((resolve) => {
        const uncovered: number[] = [];
        let f = 0;
        function tick(): void {
          el.scrollTop = Math.min(maxScrollTop, el.scrollTop + pxPerFrame);
          uncovered.push(uncoveredPx());
          f++;
          if (f < frames) requestAnimationFrame(tick);
          else resolve({ velocity: pxPerFrame, uncoveredPx: uncovered });
        }
        requestAnimationFrame(tick);
      });
    },
    { gridSelector, pxPerFrame, frames },
  );
}

// measureRowUpdatesDuringScroll and its own `window.__kiraGridRowUpdates` hook (GridRow.vue's own
// onUpdated count) were the sandbox-provable proof that a reference-stable RowVM made Vue skip
// re-rendering a row whose props didn't change — a property of the pre-SlickGrid grid's own Vue-
// reconciliation render path specifically. SlickGrid builds and updates its own DOM directly,
// never through Vue's prop-diffing, so this hook was never called under the SlickGrid engine at
// all — its own gate in budgets.spec.ts was vacuously true (`updates` always 0) the whole time this
// suite has run against SlickGrid exclusively, and had no SlickGrid-side equivalent to measure.
