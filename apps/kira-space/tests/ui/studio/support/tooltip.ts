import type { Locator, Page } from '@playwright/test';
import { expect } from '../fixtures';

// `[data-slot="tooltip-content"]` is `ui/tooltip`'s own TooltipContent.vue marker (P104 §6.5),
// not tied to any one call site's DOM — AttributeTooltip.vue's grid-header bridge renders through
// this exact same component.
export const tooltipContent = (page: Page): Locator =>
  page.locator('[data-slot="tooltip-content"]');

/** A single hover's own visibility-wait window, tuned empirically against this exact app build
 *  (reka 2.10, delayDuration 400 ms) — a hover that isn't open within this window essentially
 *  never opens (see ATTEMPT_TIMEOUT_MS's own note below), so a short window plus a re-hover beats
 *  one long window every time observed. */
const ATTEMPT_TIMEOUT_MS = 600;

/** Hovers `trigger` and asserts the real tooltip becomes visible with `text`, well within
 *  TooltipProvider's 400 ms delayDuration. toContainText, not toHaveText: reka's own
 *  TooltipContent renders a visually-hidden a11y mirror span alongside the visible text, so a
 *  bare .textContent read sees the text doubled. `timeoutMs` covers only the final content match,
 *  for a tooltip whose text depends on data that is still loading (e.g. an async row count) — it
 *  does not widen the open-visibility wait below.
 *
 *  Two independent reasons a single, plain hover can land dark, confirmed by tracing reka's own
 *  source and instrumenting real event delivery against this exact app build:
 *   1. reka's TooltipTrigger opens on `pointermove`, not `pointerenter`/`mouseenter`. A hover
 *      whose target coincides with the pointer's last position dispatches no real pointer event
 *      at all (there's no move), e.g. right after clicking a context-menu item that renders near
 *      the trigger — so every attempt below moves to a neutral corner first, making each hover a
 *      genuine move-in.
 *   2. Even with that fixed, a tree row's status dot (whose own `Tooltip :disabled` only just
 *      flipped false off freshly-arrived data) still needs a second hover cycle in practice: an
 *      `expect(...).toBeVisible({ timeout })` waited *past* `ATTEMPT_TIMEOUT_MS` (1000 ms tried,
 *      repeatedly) never once opened it, while a wait held *at* `ATTEMPT_TIMEOUT_MS` and retried
 *      opened it on the 2nd attempt every time (8/8 runs) — a genuine, reproducible quirk in how
 *      reka's own delay/skip-delay timers interact with a longer poll, not settled further here.
 *      So a failed attempt retries with a fresh hover instead of failing outright, bounded by an
 *      overall deadline. */
export async function assertTooltipShows(
  page: Page,
  trigger: Locator,
  text: string | RegExp,
  timeoutMs = 1_000,
): Promise<void> {
  const content = tooltipContent(page);
  const deadline = Date.now() + Math.max(timeoutMs, 4_000);
  for (;;) {
    await page.mouse.move(4, 4);
    await trigger.hover();
    try {
      await expect(content).toBeVisible({ timeout: ATTEMPT_TIMEOUT_MS });
      break;
    } catch (err) {
      if (Date.now() >= deadline) throw err;
    }
  }
  await expect(content).toContainText(text, { timeout: timeoutMs });
}
