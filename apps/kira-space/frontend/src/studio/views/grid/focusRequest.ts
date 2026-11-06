// "act on a tab whose view may not be mounted yet" (editReferencedRow's own new tab
// landing its caret before that tab's SlickGridHost has necessarily mounted, or before its first
// filtered page has loaded) needs a pending-request pattern. This module imports nothing (a Map
// and five plain functions), so it closes no cycle with menu.ts, state.ts or the host.

export interface CellFocusRequest {
  /** Page row index. Always 0 today (the filtered page holds the one record). */
  row: number;
  /** Which column to land on: the first column that is not part of the primary key, falling back
   *  to display column 0 when every column is. */
  prefer: 'first-non-key';
  /** Enter the grid's inline editor too, not just select. Self-gating (onBeforeEditCell) — this
   *  module introduces no editability rule of its own. */
  edit: boolean;
}

const hosts = new Map<string, (req: CellFocusRequest) => boolean>();
const pending = new Map<string, CellFocusRequest>();

/** SlickGridHost's own `onMounted`: registers this tab's `apply` (page/column resolve + focus). */
export function registerGridHost(tabId: string, apply: (req: CellFocusRequest) => boolean): void {
  hosts.set(tabId, apply);
}

/** SlickGridHost's own `onUnmounted` counterpart. */
export function unregisterGridHost(tabId: string): void {
  hosts.delete(tabId);
}

/** Applies `req` immediately when a host is registered for `tabId` and its `apply` succeeds (it
 *  has a page and resolved a column); otherwise stores it for that tab's next `pageVersion` render
 *  to consume via `consumeCellFocus`. */
export function requestCellFocus(tabId: string, req: CellFocusRequest): void {
  const apply = hosts.get(tabId);
  if (apply?.(req)) return;
  pending.set(tabId, req);
}

/** SlickGridHost's own `pageVersion` watch, after `grid.render()`: takes and clears `tabId`'s own
 *  pending request, if any. */
export function consumeCellFocus(tabId: string): CellFocusRequest | null {
  const req = pending.get(tabId) ?? null;
  if (req) pending.delete(tabId);
  return req;
}

/** `views/grid/state.ts`'s own `load()` catch arm: a load that produced no page can never satisfy
 *  a pending request, and leaving it pending would let a *later*, unrelated load consume it and
 *  jump somewhere nobody asked for. Also called by the tab-close cleanup registry (state.ts) so a
 *  request never outlives the tab it targeted. */
export function clearCellFocus(tabId: string): void {
  pending.delete(tabId);
}
