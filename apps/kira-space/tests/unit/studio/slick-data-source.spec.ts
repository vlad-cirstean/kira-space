import { describe, expect, test } from 'bun:test';
import {
  type DisplayRowIndex,
  dataLength,
  displayPositionOf,
  rowAtDisplayPosition,
  rowHandleAt,
} from '../../../frontend/src/studio/views/shared/slick/dataSource';

// This file's subject moved from views/grid/slick/dataSource.ts to
// views/shared/slick/dataSource.ts — the generic display-position/RowHandle core split out of
// that one file so views/console/ConsoleSlickGrid.vue could import it without reaching into
// views/grid/*. The split also drops this test's old dynamic-import workaround: the
// grid-specific half left behind (createDisplayValueExtractor/pendingRowClasses, which transitively
// reach pendingChanges.ts -> bridge/data.ts -> '/wails/runtime.js' at module scope) is what forced
// it, and this file never exercised that half — a plain static import is correct here now.

// P22 spike C3: display-position <-> page-row translation across a filter, the insert-row region
// past the page end, and the "handle must be truthy" invariant — exactly CLAUDE.md's own bar for a
// dedicated unit test ("cursor/pagination arithmetic with real boundary cases").

describe('rowHandleAt/dataLength (P22 spike D1) — display-position -> RowHandle translation', () => {
  test('1. unfiltered: identity — pos === row, no insertId', () => {
    const idx: DisplayRowIndex = { displayRows: null, pageRowCount: 100 };
    expect(rowHandleAt(idx, [], 0)).toEqual({ row: 0, pos: 0 });
    expect(rowHandleAt(idx, [], 42)).toEqual({ row: 42, pos: 42 });
    expect(dataLength(idx, 0)).toBe(100);
  });

  test('2. filtered: a display position maps through displayRows to its page row', () => {
    const idx: DisplayRowIndex = { displayRows: [3, 7, 9, 40], pageRowCount: 100 };
    expect(rowHandleAt(idx, [], 0)).toEqual({ row: 3, pos: 0 });
    expect(rowHandleAt(idx, [], 2)).toEqual({ row: 9, pos: 2 });
    expect(rowHandleAt(idx, [], 3)).toEqual({ row: 40, pos: 3 });
    expect(dataLength(idx, 0)).toBe(4);
  });

  test('3. the insert region starts exactly at displayRowCount and carries the insert id', () => {
    const idx: DisplayRowIndex = { displayRows: null, pageRowCount: 10 };
    const inserts = [{ id: 'a' }, { id: 'b' }];
    // Position 10 (the first past the 10 real rows) is the first insert.
    expect(rowHandleAt(idx, inserts, 10)).toEqual({ row: 10, pos: 10, insertId: 'a' });
    expect(rowHandleAt(idx, inserts, 11)).toEqual({ row: 11, pos: 11, insertId: 'b' });
    expect(dataLength(idx, inserts.length)).toBe(12);
  });

  test("4. the insert region's row identity is page.rowCount + offset, matching onPaste's own rule", () => {
    // A filter shrinks the display-row count but the insert region's row identity is still keyed
    // off pageRowCount (the real row count), not the filtered display count (P24 D5's own rule,
    // reused here — see DataGrid.vue's own template comment on the pending-insert row's identity).
    const idx: DisplayRowIndex = { displayRows: [3, 7], pageRowCount: 100 };
    const inserts = [{ id: 'only' }];
    expect(rowHandleAt(idx, inserts, 2)).toEqual({ row: 100, pos: 2, insertId: 'only' });
  });

  test('5. an insert position past the known inserts array still returns a truthy, frozen handle', () => {
    // F1's own guard: appendCellHtml/appendRowHtml require getItem to return something truthy for
    // every position getLength() reports — a stale count must never produce a falsy handle.
    const idx: DisplayRowIndex = { displayRows: null, pageRowCount: 5 };
    const handle = rowHandleAt(idx, [], 5);
    expect(handle).toBeTruthy();
    expect(Object.isFrozen(handle)).toBe(true);
    expect(handle.insertId).toBeUndefined();
  });

  test('6. every handle produced is frozen — nothing downstream can mutate a RowHandle', () => {
    const idx: DisplayRowIndex = { displayRows: [1, 2, 3], pageRowCount: 10 };
    expect(Object.isFrozen(rowHandleAt(idx, [], 1))).toBe(true);
    expect(Object.isFrozen(rowHandleAt(idx, [{ id: 'x' }], 3))).toBe(true);
  });

  test('7. an empty page (no rows, no inserts) has zero length and the boundary position is still handled', () => {
    const idx: DisplayRowIndex = { displayRows: null, pageRowCount: 0 };
    expect(dataLength(idx, 0)).toBe(0);
    const handle = rowHandleAt(idx, [], 0);
    expect(handle).toEqual({ row: 0, pos: 0 });
  });
});

// rowAtDisplayPosition reuses rowHandleAt's own pageRowAt arithmetic (never a second
// implementation that could drift), and displayPositionOf is its inverse, real once a live search
// filter is wired, identity until then.
describe('displayPositionOf/rowAtDisplayPosition (P22 Pass B C4) — page row <-> display position', () => {
  test('1. unfiltered: identity in both directions', () => {
    const idx: DisplayRowIndex = { displayRows: null, pageRowCount: 100 };
    expect(rowAtDisplayPosition(idx, 42)).toBe(42);
    expect(displayPositionOf(idx, 42)).toBe(42);
  });

  test('2. filtered: an exact hit round-trips through both directions', () => {
    const idx: DisplayRowIndex = { displayRows: [3, 7, 9, 40], pageRowCount: 100 };
    expect(rowAtDisplayPosition(idx, 2)).toBe(9);
    expect(displayPositionOf(idx, 9)).toBe(2);
    expect(displayPositionOf(idx, 3)).toBe(0);
    expect(displayPositionOf(idx, 40)).toBe(3);
  });

  test('3. filtered: a page row the filter just hid falls through to the nearest visible position', () => {
    const idx: DisplayRowIndex = { displayRows: [3, 7, 9, 40], pageRowCount: 100 };
    // 8 isn't in displayRows — sorts between 7 (pos 1) and 9 (pos 2), landing on pos 2.
    expect(displayPositionOf(idx, 8)).toBe(2);
    // Past the last match, clamped to the last position rather than running off the end.
    expect(displayPositionOf(idx, 99)).toBe(3);
  });

  test("4. the insert region: rowAtDisplayPosition reuses rowHandleAt's own identity rule", () => {
    const idx: DisplayRowIndex = { displayRows: [3, 7], pageRowCount: 100 };
    // Position 2 is the first past the 2 filtered rows -> the insert region, row = pageRowCount.
    expect(rowAtDisplayPosition(idx, 2)).toBe(100);
    expect(displayPositionOf(idx, 100)).toBe(2);
  });

  test('5. an empty display set: displayPositionOf clamps to 0, never a negative index', () => {
    const idx: DisplayRowIndex = { displayRows: [], pageRowCount: 100 };
    expect(displayPositionOf(idx, 50)).toBe(0);
  });
});
