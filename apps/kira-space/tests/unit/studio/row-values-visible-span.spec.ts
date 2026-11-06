import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';

// A `range`-kind selection stores only its two page-row corners; every
// consumer used to walk the CONTIGUOUS span between them, silently sweeping in rows the active
// filter was hiding. `visibleRowsInSpan` (copy/delete) and `pasteTargetRows` (paste) are the fix —
// boundary arithmetic worth its own coverage independent of the UI test (slick-grid.spec.ts's
// test), which only exercises the copy/delete path end to end.
//
// rowValues.ts transitively reaches `bridge/control`/`bridge/data` (via `./menu`/`./pendingChanges`
// — Bun's own module graph resolves those against `/wails/runtime.js` eagerly), so it has to be a
// dynamic `import()` here, after `@workbench/testing/unit/window`'s mock.module registration
// has run — the same pattern console-auto-explain-race.spec.ts and its siblings already use.
const { pasteTargetRows, visibleRowsInSpan } = await import(
  '../../../frontend/src/studio/views/grid/slick/rowValues'
);

describe('visibleRowsInSpan (finding 3)', () => {
  test('unfiltered (displayRows: null) — the full contiguous span, ascending', () => {
    expect(visibleRowsInSpan(null, 2, 5)).toEqual([2, 3, 4, 5]);
  });

  test('unfiltered, corners in either order — still ascending', () => {
    expect(visibleRowsInSpan(null, 5, 2)).toEqual([2, 3, 4, 5]);
  });

  test('filtered — only the rows within [lo, hi] that are actually visible', () => {
    // Filter shows page rows 0, 1, 3, 89 — a range spanning 0..89 must only touch these four.
    const displayRows = [0, 1, 3, 89];
    expect(visibleRowsInSpan(displayRows, 0, 89)).toEqual([0, 1, 3, 89]);
  });

  test('filtered, span narrower than the full visible set — only rows inside the span', () => {
    const displayRows = [0, 1, 3, 89];
    expect(visibleRowsInSpan(displayRows, 0, 3)).toEqual([0, 1, 3]);
  });

  test('filtered, corners reversed — still filters correctly and returns ascending', () => {
    const displayRows = [0, 1, 3, 89];
    expect(visibleRowsInSpan(displayRows, 89, 0)).toEqual([0, 1, 3, 89]);
  });

  test('filtered, no visible row in span — empty', () => {
    const displayRows = [0, 1, 89];
    expect(visibleRowsInSpan(displayRows, 10, 20)).toEqual([]);
  });

  test('a single-row span (degenerate "range" of one cell) — just that row if visible', () => {
    expect(visibleRowsInSpan([0, 5, 9], 5, 5)).toEqual([5]);
    expect(visibleRowsInSpan([0, 5, 9], 6, 6)).toEqual([]);
  });
});

describe('pasteTargetRows (finding 3)', () => {
  test('unfiltered — the old, purely arithmetic contiguous walk, unchanged', () => {
    expect(pasteTargetRows(null, 100, 10, 3)).toEqual([10, 11, 12]);
  });

  test('count 0 — nothing to target', () => {
    expect(pasteTargetRows([0, 1, 2], 100, 5, 0)).toEqual([]);
  });

  test('filtered — the ri-th clipboard row lands on the ri-th visible row at/after startRow', () => {
    // displayRows real page rows only, rowCount 100. Starting at row 1, the next 3 visible rows
    // (skipping the filtered-out gap) are 1, 3, 89.
    const displayRows = [0, 1, 3, 89];
    expect(pasteTargetRows(displayRows, 100, 1, 3)).toEqual([1, 3, 89]);
  });

  test('filtered, walk runs past the last visible row — continues into the pending-insert region, contiguous from rowCount (never filtered)', () => {
    const displayRows = [0, 1, 3, 89];
    // Only 2 visible rows at/after startRow=3 (3, 89); the remaining 2 of 4 requested targets
    // continue contiguously from rowCount=100.
    expect(pasteTargetRows(displayRows, 100, 3, 4)).toEqual([3, 89, 100, 101]);
  });

  test('startRow already past the loaded page (pasting straight into pending inserts) — filter is irrelevant, always contiguous', () => {
    const displayRows = [0, 1, 3];
    expect(pasteTargetRows(displayRows, 100, 105, 3)).toEqual([105, 106, 107]);
  });

  test('startRow at exactly rowCount — the boundary between real and pending-insert rows', () => {
    const displayRows = [0, 1, 3];
    expect(pasteTargetRows(displayRows, 100, 100, 2)).toEqual([100, 101]);
  });
});
