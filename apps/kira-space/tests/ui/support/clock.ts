import type { Page } from '@playwright/test';
import { AUTO_REFRESH_COALESCE_MS } from '../../../../../packages/git-ui/src/state/refreshTiming';
import {
  TOOLTIP_DELAY_MS,
  TOOLTIP_SKIP_DELAY_MS,
} from '../../../../../packages/theme/src/components/ui/tooltip/timing';

/** Installs the fake clock after boot, then freezes it: app timers fire only when advanced.
 *  The git transport double keeps the real timers, so its replies still flow. */
export async function freezeClock(page: Page): Promise<void> {
  await page.clock.install();
  await page.clock.pauseAt(Date.now() + 10_000);
}

/** Runs out the auto-refresh coalesce window. */
export async function runCoalesceWindow(page: Page): Promise<void> {
  await page.clock.runFor(AUTO_REFRESH_COALESCE_MS);
}

// P81 §7.1 — a thin wrapper over `page.clock.install()`, kept for the doc comment: the one
// non-obvious constraint below is written once instead of re-derived at each call site.

/** Install Playwright's fake clock, paused. Call this **after** `relaunch()` has booted the
 *  page, never before it — `relaunch()` navigates and waits for `[data-testid="status-bar"]`
 *  (`fixtures.ts:85`-`86`), and timers frozen across boot risk stalling that wait.
 *
 *  `page.clock.install()` alone does not freeze time — timers keep firing on real time until the
 *  clock is explicitly paused (Playwright's own doc for `pauseAt`: "install... and let the page
 *  load naturally... once loaded, pause"). Without the immediate `pauseAt` below, real time spent
 *  on subsequent CDP round trips (e.g. `page.keyboard.type`'s per-character dispatches) keeps
 *  advancing the "fake" clock exactly like the real one — silently defeating the point of
 *  installing it at all. Confirmed empirically: typing 20 characters can by itself burn
 *  60-100ms of real time that a bare `install()` still counts.
 *
 *  Safe for this suite's mock transport: control answers are `page.route` network fulfills
 *  (`support/mockRuntime.ts`), not timer-driven, so freezing time never blocks a response. Any
 *  native timer already scheduled before install keeps running on real time — that's what makes a
 *  mid-test install safe, not just after-boot.
 *
 *  Callers advance time with `page.clock.runFor(ms)` directly — no wrapper for that; the
 *  Playwright API is the clearer thing to read at the call site. */
export async function installFakeTimers(page: Page): Promise<void> {
  await page.clock.install();
  // `pauseAt` rejects a time at or before what the fake clock already considers "now" — and since
  // `install()` leaves it ticking on real time (see above), any snapshot of "now" taken before the
  // CDP round trip that carries `pauseAt` there is stale by the time it arrives, however it was
  // captured (Node-side `Date.now()` or a `page.evaluate` read-back). A several-second forward
  // buffer clears that race outright: nothing in this suite schedules a real timer between
  // `install()` and this call (the debounce this helper exists for hasn't been armed yet), so
  // jumping past a few idle seconds fires nothing early.
  await page.clock.pauseAt(Date.now() + 10_000);
}

/** Runs out the tooltip provider's skip-delay window, so the next hover waits the full delay. */
export async function settleTooltipSkipDelay(page: Page): Promise<void> {
  await page.clock.runFor(TOOLTIP_SKIP_DELAY_MS);
}

/** Runs the clock up to, but not past, the tooltip open delay. */
export async function runToJustBeforeTooltipDelay(page: Page): Promise<void> {
  await page.clock.runFor(TOOLTIP_DELAY_MS - 1);
}

export async function runPastTooltipDelay(page: Page): Promise<void> {
  await page.clock.runFor(1);
}
