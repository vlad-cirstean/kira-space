import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';

// A tab's currently-visible page-row window, reported by whichever view renders it through
// reportVisibleWindow and read by that same view's runSearch as the scan's priority window
// (scan.ts's opts.priority) — the rows a search should highlight first, since they're the ones
// already on screen.
//
// The decode-cache prune is the caller's own page-module setVisibleWindow: that prunes a decode
// cache keyed by *page*; this is search priority keyed by *tab*. A plain object (not `reactive`) —
// nothing here needs to trigger a re-render on its own, it is only ever read at the moment a scan
// starts.
const visibleRows: Record<string, { from: number; to: number }> = {};

/** First and one-past-last page row of the rendered virtual items over `rowIndices`; null when
 *  nothing is rendered or an index is out of range. `rowIndices` is ascending but may be
 *  non-contiguous (filtering), so item indices are mapped through it. */
export function pageRowWindow(
  items: readonly { index: number }[],
  rowIndices: readonly number[],
): { from: number; to: number } | null {
  if (items.length === 0) return null;
  const from = rowIndices[items[0].index];
  const to = rowIndices[items[items.length - 1].index];
  if (from === undefined || to === undefined) return null;
  return { from, to: to + 1 };
}

/** Search priority (keyed by tab) plus decode-cache prune (keyed by page scope). */
export function reportVisibleWindow(
  scope: string,
  from: number,
  to: number,
  setVisibleWindow: (pageScope: string, from: number, to: number) => void,
  pageScope: string = scope,
): void {
  visibleRows[scope] = { from, to };
  setVisibleWindow(pageScope, from, to);
}

export function visibleRowsOf(scope: string): { from: number; to: number } | null {
  return visibleRows[scope] ?? null;
}

function clearVisibleRows(scope: string): void {
  delete visibleRows[scope];
  // P63: widens the same way shared/page/search.ts's own clearSearchState does — see its comment.
  delete visibleRows[`${scope}::preview`];
}

registerTabRuntimeCleanup(clearVisibleRows);
