import { describe, expect, test } from 'bun:test';
import {
  clampColumnOverscan,
  countNewRows,
} from '../../../frontend/src/studio/views/shared/slick/kiraSlickGrid';

// KiraSlickGrid.getRenderedRange's own row-bounds arithmetic is rowRangeBounds
// (views/shared/page/columns.ts) — covered by tests/unit/row-range-bounds.spec.ts, not this file
// (This file has only ever covered the column-axis overscan clamp below, and separately
// `countNewRows`, the batch-cap arithmetic `getRenderedRange` also uses).

describe('clampColumnOverscan (P22 spike D4) — the column-axis overscan clamp', () => {
  test('1. expands both sides by overscanPx when there is room', () => {
    expect(clampColumnOverscan(1000, 1500, 560, 10_000)).toEqual({ leftPx: 440, rightPx: 2060 });
  });

  test('2. clamps the left side at 0 rather than going negative', () => {
    expect(clampColumnOverscan(200, 1500, 560, 10_000)).toEqual({ leftPx: 0, rightPx: 2060 });
  });

  test('3. clamps the right side at the canvas width rather than exceeding it', () => {
    expect(clampColumnOverscan(1000, 1500, 560, 1800)).toEqual({ leftPx: 440, rightPx: 1800 });
  });

  test('4. both sides clamp simultaneously on a canvas narrower than the overscan budget', () => {
    expect(clampColumnOverscan(100, 900, 560, 700)).toEqual({ leftPx: 0, rightPx: 700 });
  });
});

// Finding 4 (round 2) — `countNewRows` (P22 iter2-scroll-gaps D2 step 4/7) is exported "so the
// batch-cap arithmetic is testable without constructing a real SlickGrid" (its own doc comment)
// but had zero tests until now, despite that stated intent.
describe('countNewRows (P22 iter2-scroll-gaps D2) — how many rows in [start,end] are NOT already cached', () => {
  test('1. no overlap with the previous range — every row is new', () => {
    expect(countNewRows(100, 120, 200, 220)).toBe(21);
  });

  test('2. fully contained in the previous range — nothing new', () => {
    expect(countNewRows(105, 110, 100, 120)).toBe(0);
  });

  test('3. partial overlap on the lead side — only the non-overlapping rows count as new', () => {
    // [100,130] vs prev [80,110]: overlap is [100,110] (11 rows), total is 31 -> 20 new.
    expect(countNewRows(100, 130, 80, 110)).toBe(20);
  });

  test('4. partial overlap on the trail side — only the non-overlapping rows count as new', () => {
    // [70,100] vs prev [90,120]: overlap is [90,100] (11 rows), total is 31 -> 20 new.
    expect(countNewRows(70, 100, 90, 120)).toBe(20);
  });

  test('5. an inverted/empty range (end < start) is always 0 new rows', () => {
    expect(countNewRows(50, 40, 0, 100)).toBe(0);
  });

  test('6. identical to the previous range — nothing new, the whole point of the cap', () => {
    expect(countNewRows(1000, 1020, 1000, 1020)).toBe(0);
  });

  test('7. touching but not overlapping (adjacent ranges) — every row is new', () => {
    // prevEnd (99) is one less than start (100): Math.min(end,prevEnd) < Math.max(start,prevStart)
    // so overlap is 0, not a spurious negative-length overlap.
    expect(countNewRows(100, 120, 50, 99)).toBe(21);
  });
});
