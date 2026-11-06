import { describe, expect, test } from 'bun:test';
import {
  BASE_LEAD_PX,
  BASE_TRAIL_PX,
  type RowRangeExtractorConfig,
  rowRangeBounds,
} from '../../../frontend/src/studio/views/shared/page/columns';

// This arithmetic's real coverage (velocity-adaptive, direction-biased row overscan —
// cursor/pagination-style boundary cases, CLAUDE.md's own bar for a dedicated unit test) was
// genuinely lost, not "moved", when tests/unit/row-range.spec.ts was deleted in the cutover
// (commit 52b8a80) — kiraSlickGrid.ts and kira-slick-grid.spec.ts both carried a stale, circular
// comment claiming the coverage lived in kira-slick-grid.spec.ts, which in fact only ever covered
// clampColumnOverscan. Ported from the deleted file's own test cases
// (`git show 52b8a80^:apps/kira-space/tests/unit/row-range.spec.ts`) against the CURRENT
// function: `rowRangeBounds` (this file, which dropped the old `@tanstack/vue-virtual`-shaped
// `rowRangeExtractor` wrapper this arithmetic used to sit behind) returns `{start, end}` row
// bounds directly rather than a materialised array of row indices — every assertion below is
// rewritten against that shape, not a blind restore of the deleted array-index assertions.

const ROW_HEIGHT = 28; // BASE_LEAD_PX/BASE_TRAIL_PX (560) / 28 = 20 rows per side at rest.
const BASE_ROWS = BASE_LEAD_PX / ROW_HEIGHT;

function cfg(overrides: Partial<RowRangeExtractorConfig> = {}): RowRangeExtractorConfig {
  return {
    baseLeadPx: BASE_LEAD_PX,
    baseTrailPx: BASE_TRAIL_PX,
    leadFrames: 6,
    maxLeadPx: 2400,
    cellBudget: 2200,
    ...overrides,
  };
}

describe('rowRangeBounds (P22 iter2 D3) — velocity-adaptive, direction-biased row overscan', () => {
  test('1. D3(a): zero velocity is byte-identical to a symmetric ±BASE_ROWS window, in every direction', () => {
    const range = { startIndex: 100, endIndex: 120, count: 100_000 };
    const forward = rowRangeBounds(range, ROW_HEIGHT, 0, 1, 10, cfg());
    const backward = rowRangeBounds(range, ROW_HEIGHT, 0, -1, 10, cfg());
    const atRest = rowRangeBounds(range, ROW_HEIGHT, 0, 0, 10, cfg());
    const expected = { start: range.startIndex - BASE_ROWS, end: range.endIndex + BASE_ROWS };
    expect(forward).toEqual(expected);
    expect(backward).toEqual(expected);
    expect(atRest).toEqual(expected);
  });

  test('2. forward velocity extends the end (lead) side only, trail stays at baseline', () => {
    const range = { startIndex: 100, endIndex: 120, count: 100_000 };
    // leadPx = 560 + 200*6 = 1760 -> 63 rows lead; trail stays 20.
    const out = rowRangeBounds(range, ROW_HEIGHT, 200, 1, 10, cfg());
    expect(out.start).toBe(range.startIndex - BASE_ROWS);
    expect(out.end).toBe(range.endIndex + Math.ceil(1760 / ROW_HEIGHT));
  });

  test('3. backward velocity extends the start (lead) side only, trail stays at baseline', () => {
    const range = { startIndex: 500, endIndex: 520, count: 100_000 };
    const out = rowRangeBounds(range, ROW_HEIGHT, 200, -1, 10, cfg());
    expect(out.start).toBe(range.startIndex - Math.ceil(1760 / ROW_HEIGHT));
    expect(out.end).toBe(range.endIndex + BASE_ROWS);
  });

  test('4. leadPx clamps at maxLeadPx regardless of how large velocity gets', () => {
    const range = { startIndex: 1000, endIndex: 1020, count: 100_000 };
    const huge = rowRangeBounds(range, ROW_HEIGHT, 100_000, 1, 10, cfg());
    const atCap = rowRangeBounds(range, ROW_HEIGHT, 100, 1, 10, cfg({ maxLeadPx: 2400 }));
    // 100px/frame * 6 frames = 600, +560 base = 1160 < 2400, so not yet capped — huge must exceed it.
    expect(huge.end - range.endIndex).toBeGreaterThan(atCap.end - range.endIndex);
    expect(huge.end).toBe(range.endIndex + Math.ceil(cfg().maxLeadPx / ROW_HEIGHT));
  });

  test('5. clamps at 0 on the start side and count-1 on the end side', () => {
    const out = rowRangeBounds(
      { startIndex: 0, endIndex: 5, count: 10 },
      ROW_HEIGHT,
      0,
      0,
      10,
      cfg(),
    );
    expect(out.start).toBe(0);
    expect(out.end).toBe(9);
  });

  test('6. an empty table produces an empty (start=0, end=-1) range', () => {
    expect(
      rowRangeBounds({ startIndex: 0, endIndex: 0, count: 0 }, ROW_HEIGHT, 500, 1, 10, cfg()),
    ).toEqual({ start: 0, end: -1 });
  });

  test('7. D3(c): the cell budget caps the *extra* lead only — the baseline is always granted in full', () => {
    // 20 columns, 11 visible rows: budgetRows = floor(2200/20) = 110; remaining after baseline+
    // visible = 110 - 11 - 20 (trail) - 20 (lead baseline) = 59 extra rows grantable. Uncapped
    // wanted lead at this velocity would be maxLeadPx (2400px = 86 rows), 66 rows of *extra* —
    // more than the 59 the budget allows, so the cap must bind at exactly baseline + 59.
    const range = { startIndex: 1000, endIndex: 1010, count: 100_000 }; // 11 visible rows
    const out = rowRangeBounds(range, ROW_HEIGHT, 456, 1, 20, cfg());
    expect(out.start).toBe(range.startIndex - BASE_ROWS); // trail: untouched by the cap
    const leadRows = out.end - range.endIndex;
    expect(leadRows).toBe(BASE_ROWS + 59);
    // Confirms the cap is actually doing something here, not coincidentally equal to uncapped.
    expect(leadRows).toBeLessThan(Math.ceil(cfg().maxLeadPx / ROW_HEIGHT));
  });

  test('8. a wide-enough table still grants full baseline even when total budget is razor-thin', () => {
    // cellBudget 2200 / 400 columns = 5 rows total — less than baseline (20) on ONE side alone.
    // The extra-lead cap floors at 0 (never negative), so lead == baseline, exactly like at rest.
    const range = { startIndex: 1000, endIndex: 1010, count: 100_000 };
    const out = rowRangeBounds(range, ROW_HEIGHT, 456, 1, 400, cfg());
    expect(out.start).toBe(range.startIndex - BASE_ROWS);
    expect(out.end).toBe(range.endIndex + BASE_ROWS);
  });
});
