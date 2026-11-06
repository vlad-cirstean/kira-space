// mergeHighlightRanges is the one pure boundary-merge function this migration adds a
// dedicated unit test for — two sorted interval lists (syntax-colour "base" runs, which tile
// [0, textLength) with no gaps, and possibly-overlapping {{variable}}/find-match "highlight"
// ranges), including the degenerate inputs (zero-width, out-of-range) real callers can hand it a
// keystroke stale. parseColorizedLine's own HTML-shape parsing is exercised alongside it since both
// live in editor/paintSpans.ts and both are pure, DOM-free functions.
import { describe, expect, test } from 'bun:test';
import { mergeHighlightRanges, parseColorizedLine } from '@workbench/editor/paintSpans';

describe('mergeHighlightRanges', () => {
  test('a highlight fully inside one base run splits it into three, middle carrying both classes', () => {
    const base = [{ from: 0, to: 10, classes: ['mtk1'] }];
    const out = mergeHighlightRanges(10, base, [{ from: 3, to: 6, class: 'kira-ed-var' }]);
    expect(out).toEqual([
      { from: 0, to: 3, classes: ['mtk1'] },
      { from: 3, to: 6, classes: ['mtk1', 'kira-ed-var'] },
      { from: 6, to: 10, classes: ['mtk1'] },
    ]);
  });

  test("a highlight spanning two base runs keeps each half's own base class", () => {
    const base = [
      { from: 0, to: 5, classes: ['mtk1'] },
      { from: 5, to: 10, classes: ['mtk2'] },
    ];
    const out = mergeHighlightRanges(10, base, [{ from: 3, to: 8, class: 'kira-ed-var' }]);
    expect(out).toEqual([
      { from: 0, to: 3, classes: ['mtk1'] },
      { from: 3, to: 5, classes: ['mtk1', 'kira-ed-var'] },
      { from: 5, to: 8, classes: ['mtk2', 'kira-ed-var'] },
      { from: 8, to: 10, classes: ['mtk2'] },
    ]);
  });

  test('two overlapping highlights over the same base run both apply to the shared segment', () => {
    const base = [{ from: 0, to: 10, classes: ['mtk1'] }];
    const out = mergeHighlightRanges(10, base, [
      { from: 0, to: 6, class: 'kira-ed-find-match' },
      { from: 4, to: 10, class: 'kira-ed-var' },
    ]);
    expect(out).toEqual([
      { from: 0, to: 4, classes: ['mtk1', 'kira-ed-find-match'] },
      { from: 4, to: 6, classes: ['mtk1', 'kira-ed-find-match', 'kira-ed-var'] },
      { from: 6, to: 10, classes: ['mtk1', 'kira-ed-var'] },
    ]);
  });

  test('no highlights returns the base runs unchanged', () => {
    const base = [{ from: 0, to: 5, classes: ['mtk1'] }];
    expect(mergeHighlightRanges(5, base, [])).toEqual(base.map((b) => ({ ...b })));
  });

  test('a zero-width range (from === to) contributes nothing', () => {
    const base = [{ from: 0, to: 5, classes: ['mtk1'] }];
    expect(mergeHighlightRanges(5, base, [{ from: 2, to: 2, class: 'kira-ed-var' }])).toEqual(base);
  });

  test('an inverted range (from > to) contributes nothing', () => {
    const base = [{ from: 0, to: 5, classes: ['mtk1'] }];
    expect(mergeHighlightRanges(5, base, [{ from: 4, to: 1, class: 'kira-ed-var' }])).toEqual(base);
  });

  test('an out-of-range highlight is clamped to the text, not thrown', () => {
    const base = [{ from: 0, to: 5, classes: ['mtk1'] }];
    const out = mergeHighlightRanges(5, base, [{ from: -3, to: 999, class: 'kira-ed-var' }]);
    expect(out).toEqual([{ from: 0, to: 5, classes: ['mtk1', 'kira-ed-var'] }]);
  });

  test('a highlight entirely outside [0, textLength) contributes nothing', () => {
    const base = [{ from: 0, to: 5, classes: ['mtk1'] }];
    expect(mergeHighlightRanges(5, base, [{ from: 7, to: 9, class: 'kira-ed-var' }])).toEqual(base);
  });

  test('a gap in the base runs (no coverage) still paints a highlight there with no base class', () => {
    const base = [{ from: 0, to: 3, classes: ['mtk1'] }];
    const out = mergeHighlightRanges(10, base, [{ from: 5, to: 8, class: 'kira-ed-var' }]);
    expect(out).toEqual([
      { from: 0, to: 3, classes: ['mtk1'] },
      { from: 5, to: 8, classes: ['kira-ed-var'] },
    ]);
  });

  // A real case colorize() produces (verified against the pinned 0.56.0): it can split one token's
  // text across two adjacent `mtkN` spans for reasons unrelated to tokenization (an internal render
  // chunk boundary). A highlight range spanning that boundary must come out as one run, not two
  // adjacent ones with identical classes — the caller renders one `<span>` per output run, and two
  // side-by-side `.kira-ed-var-unknown` spans instead of one is a real, user-visible difference to
  // anything counting or measuring them (`api-ui-consistency.spec.ts`'s own test).
  test('a highlight spanning a same-class base-run boundary coalesces into one run', () => {
    const base = [
      { from: 0, to: 6, classes: ['mtk1'] },
      { from: 6, to: 10, classes: ['mtk1'] },
    ];
    const out = mergeHighlightRanges(10, base, [{ from: 2, to: 10, class: 'kira-ed-var-unknown' }]);
    expect(out).toEqual([
      { from: 0, to: 2, classes: ['mtk1'] },
      { from: 2, to: 10, classes: ['mtk1', 'kira-ed-var-unknown'] },
    ]);
  });

  test('adjacent base runs with different classes never coalesce, even with the same highlight', () => {
    const base = [
      { from: 0, to: 5, classes: ['mtk1'] },
      { from: 5, to: 10, classes: ['mtk2'] },
    ];
    const out = mergeHighlightRanges(10, base, [{ from: 0, to: 10, class: 'kira-ed-var' }]);
    expect(out).toEqual([
      { from: 0, to: 5, classes: ['mtk1', 'kira-ed-var'] },
      { from: 5, to: 10, classes: ['mtk2', 'kira-ed-var'] },
    ]);
  });
});

describe('parseColorizedLine', () => {
  test("recovers plain text and per-token offsets from colorize()'s own HTML shape", () => {
    const html =
      '<span><span class="mtk6">SELECT</span><span class="mtk1"> </span><span class="mtk5">*</span></span><br/>';
    const { text, runs } = parseColorizedLine(html);
    expect(text).toBe('SELECT *');
    expect(runs).toEqual([
      { from: 0, to: 6, classes: ['mtk6'] },
      { from: 6, to: 7, classes: ['mtk1'] },
      { from: 7, to: 8, classes: ['mtk5'] },
    ]);
  });

  test('decodes &lt;/&gt;/&amp; entities back to their real characters', () => {
    const html = '<span><span class="mtk1">a &amp; b &lt;c&gt;</span></span><br/>';
    const { text } = parseColorizedLine(html);
    expect(text).toBe('a & b <c>');
  });

  test('empty input parses to empty text and no runs', () => {
    expect(parseColorizedLine('')).toEqual({ text: '', runs: [] });
  });
});
