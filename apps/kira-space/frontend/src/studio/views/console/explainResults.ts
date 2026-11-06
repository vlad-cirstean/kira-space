import type { QueryPlan } from './planModel';

// The plan-result sibling of resultPages.ts's own `pages` map — a plan is not a
// `Page` (ConsoleResult.kind distinguishes the two, state.ts), so it gets its own store rather
// than widening Page's union for one client-only shape. `statement` is the exact text that was
// explained, kept alongside the plan so the result view's header can show it without the caller
// threading it through a second prop.
export interface ExplainResult {
  plan: QueryPlan;
  statement: string;
}

const results = new Map<string, ExplainResult>();

export function setPlan(key: string, result: ExplainResult): void {
  results.set(key, result);
}

export function getPlan(key: string): ExplainResult | undefined {
  return results.get(key);
}

/** Mirrors resultPages.ts's `drop` — called unconditionally by state.ts's `releaseResult` for
 *  every closed result, a harmless no-op for a `kind: 'page'` result that was never in this map. */
export function dropPlan(key: string): void {
  results.delete(key);
}

/** Playwright-only (main.ts's `window.__kiraRetention`, P12 round 1 finding #10) — this module-
 *  level map has no size limit and no per-tab bulk-drop of its own, so it was invisible to the
 *  leak-detection tooling entirely until a closed tab's plans stopped being released into it. */
export function planCount(): number {
  return results.size;
}
