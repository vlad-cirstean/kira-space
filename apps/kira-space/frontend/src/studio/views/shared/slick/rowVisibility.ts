// Split out of grid/slick/rowValues.ts — that file's other exports (displayCell,
// rowSnapshot, cellNavEntry, ...) depend on grid-specific state (grid/menu, grid/page,
// grid/pendingChanges), which views/console/** must not import (biome.json's noRestrictedImports)
// — these two functions carry no such dependency, so only they move here.

/** P24 D10: while filtering, column-scoped ops (copy column values, the column-selection copy
 *  branch) walk only the *visible* rows — the column the user can see has N rows, and copying
 *  every loaded row from a grid showing 12 would be a silent mismatch pasted into a spreadsheet. */
export function rowsForColumnOps(
  displayRows: readonly number[] | null,
  rowCount: number,
): number[] {
  return displayRows ? [...displayRows] : Array.from({ length: rowCount }, (_, i) => i);
}

/** The rule ("a column-scoped op walks only the visible rows"),
 *  extended to a `range`-kind selection: its two corners (`anchorRow`/`row`) are page rows spanning
 *  a CONTIGUOUS block, which every consumer used to walk assuming nothing in between was filtered
 *  out — under an active "hide non-matching rows" filter that silently swept up hidden rows into
 *  copy/delete, never intended by the user. Ascending, matching `displayRows`' own contract; `r0`/
 *  `r1` may arrive in either order (a `SlickRange`-derived selection's anchor is always top-left,
 *  but callers here pass raw corners, not a normalised range). */
export function visibleRowsInSpan(
  displayRows: readonly number[] | null,
  r0: number,
  r1: number,
): number[] {
  const lo = Math.min(r0, r1);
  const hi = Math.max(r0, r1);
  if (!displayRows) {
    return Array.from({ length: hi - lo + 1 }, (_, i) => lo + i);
  }
  return displayRows.filter((r) => r >= lo && r <= hi);
}
