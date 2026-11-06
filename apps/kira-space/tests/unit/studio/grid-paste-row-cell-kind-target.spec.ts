// F4 (P108 Part 10): resolvePasteTarget's row-kind mapping used to be plain arithmetic —
// `startRow = Math.min(...sel.rows)`, `rowAt = startRow + ri` — which ignored which rows were
// actually selected. A ctrl-click selection of rows 2, 7 and 9 pasted into 2, 3 and 4 instead. The
// `cell` kind had a parallel bug: it never went through the filter-aware `pasteTargetRows` walk
// `range` already used, so a multi-row clipboard pasted from one cell could land on a row the
// active search filter was hiding.
//
// paste.ts transitively reaches bridge/data.ts -> '/wails/runtime.js' at module scope (it imports
// usePendingChangesStore), hence the window mock — same pattern grid-stage-delete-idempotent.spec.ts
// already uses.
import '@workbench/testing/unit/window';

import { describe, expect, test } from 'bun:test';
import { createTabularPageBuilder, unpagedPosition } from '@shared/protocol/page';

const { resolvePasteTarget } = await import('../../../frontend/src/studio/views/grid/paste');

function pageWithRowCount(rowCount: number) {
  const builder = createTabularPageBuilder([]); // resolvePasteTarget reads only page.rowCount
  for (let i = 0; i < rowCount; i++) builder.appendRow([]);
  return builder.finish(unpagedPosition(rowCount));
}

describe('resolvePasteTarget row kind (F4, P108 Part 10)', () => {
  test('1. a disjoint ctrl-click selection (rows 2, 7, 9) maps clipboard row ri onto the ri-th selected row, in ascending order — never plain startRow + ri', () => {
    const target = resolvePasteTarget(
      { kind: 'row', rows: [9, 2, 7] }, // deliberately out of order, as a ctrl-click selection can build it
      pageWithRowCount(20),
      null,
      3,
    );
    expect(target.rowAt(0)).toBe(2);
    expect(target.rowAt(1)).toBe(7);
    expect(target.rowAt(2)).toBe(9);
  });

  test('2. a clipboard with fewer rows than the selection only ever gets asked for the rows it has — never reaches past the selection', () => {
    const target = resolvePasteTarget(
      { kind: 'row', rows: [2, 7, 9] },
      pageWithRowCount(20),
      null,
      1, // pastedRowCount — the caller's own loop never calls rowAt(1) or rowAt(2)
    );
    expect(target.rowAt(0)).toBe(2);
  });

  test('3. a clipboard with more rows than the selection continues into the insert region (>= rowCount) past the last selected row, contiguously', () => {
    const target = resolvePasteTarget({ kind: 'row', rows: [2, 7] }, pageWithRowCount(10), null, 4);
    expect(target.rowAt(0)).toBe(2);
    expect(target.rowAt(1)).toBe(7);
    expect(target.rowAt(2)).toBe(10); // rowCount, not lastSelected + 1 (8) — a real row can't be inserted mid-table
    expect(target.rowAt(3)).toBe(11);
  });
});

describe('resolvePasteTarget cell kind (F4, P108 Part 10)', () => {
  test('4. a multi-row clipboard pasted from one cell skips rows the active filter hides, same as a range paste', () => {
    // Page has 10 real rows; the active filter hides rows 2 and 4 — displayRows lists only what
    // the filter still shows.
    const displayRows = [0, 1, 3, 5, 6, 7, 8, 9];
    const target = resolvePasteTarget(
      { kind: 'cell', row: 3, col: 0 },
      pageWithRowCount(10),
      displayRows,
      3,
    );
    expect(target.rowAt(0)).toBe(3);
    expect(target.rowAt(1)).toBe(5); // row 4 is filtered out — skipped, not silently written to
    expect(target.rowAt(2)).toBe(6);
  });
});
