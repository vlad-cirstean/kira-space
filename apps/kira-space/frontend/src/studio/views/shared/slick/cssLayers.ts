// The search-match/current-match `setCellCssStyles` hash arithmetic, so
// views/console/ConsoleSlickGrid.vue's own search wiring can share it rather than re-deriving it.
// Pure — no reference to `grid`, `getRenderedRange`, the render band or the theme; every caller
// decides its own column-name/visibility/display-position mapping and hands it in as a callback.
export type EdgeHash = Record<number, Record<string, string>>;

/**
 * Two `setCellCssStyles`-shaped hashes — every match, and (if any) the current one — built from a
 * flat match list. `matches`/`currentIndex` are the caller's own search-result shape reduced to
 * `{row, col}` pairs; `columnNameAt` resolves a match's column index to the SlickGrid column id
 * (field name) it corresponds to, `isVisibleColumn` filters out a column the caller currently
 * hides (a reordered/hidden data-grid column; always-true for a console result, which never hides
 * one), and `toDisplayPosition` maps a page-row index to the display position `setCellCssStyles`
 * addresses (identity while nothing is filtered).
 */
export function searchCellLayers(
  matches: readonly { row: number; col: number }[],
  currentIndex: number,
  columnNameAt: (col: number) => string | undefined,
  isVisibleColumn: (name: string) => boolean,
  toDisplayPosition: (row: number) => number,
  matchClass: string,
  currentClass: string,
): [EdgeHash, EdgeHash] {
  const matchHash: EdgeHash = {};
  const currentHash: EdgeHash = {};
  for (const m of matches) {
    const name = columnNameAt(m.col);
    if (!name || !isVisibleColumn(name)) continue;
    const pos = toDisplayPosition(m.row);
    matchHash[pos] ??= {};
    (matchHash[pos] as Record<string, string>)[name] = matchClass;
  }
  const current = currentIndex >= 0 ? matches[currentIndex] : undefined;
  const currentName = current && columnNameAt(current.col);
  if (current && currentName && isVisibleColumn(currentName)) {
    currentHash[toDisplayPosition(current.row)] = { [currentName]: currentClass };
  }
  return [matchHash, currentHash];
}
