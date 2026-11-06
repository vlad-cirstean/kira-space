// resolveColumnOrder's reordered branch used
// `kept.includes(n)` to build `missing` — O(cols²) per call. It's called *per cell* by
// displayCell/rowSnapshot (SlickGridHost.vue), not once per bulk operation, so a wide table with a
// stored column order turned a full-page copy into hundreds of thousands of calls each doing an
// O(cols²) scan. Fixed with a Set (correctness-preserving) and memoized per (page, stored) since
// pages are frozen and reference-stable.
import { describe, expect, test } from 'bun:test';
import {
  createTabularPageBuilder,
  unpagedPosition,
} from '../../../../../packages/shared/protocol/page';
import { resolveColumnOrder } from '../../../frontend/src/studio/views/shared/page/columns';

function pageWithColumns(names: string[]) {
  const columns = names.map((name) => ({
    name,
    dataType: 'text',
    typeClass: 'text' as const,
    nullable: true,
    isPrimaryKey: false,
    generated: false,
  }));
  const builder = createTabularPageBuilder(columns);
  builder.appendRow(names.map(() => 'v'));
  return builder.finish(unpagedPosition(1));
}

describe('resolveColumnOrder', () => {
  test("null stored returns the page's own natural order", () => {
    const page = pageWithColumns(['a', 'b', 'c']);
    expect(resolveColumnOrder(page, null)).toEqual(['a', 'b', 'c']);
  });

  test('a stored order is honoured, dropping a column that no longer exists', () => {
    const page = pageWithColumns(['a', 'b', 'c']);
    expect(resolveColumnOrder(page, ['c', 'gone', 'a'])).toEqual(['c', 'a', 'b']);
  });

  test('a new column not in the stored order is appended at the end, in natural order', () => {
    const page = pageWithColumns(['a', 'b', 'c', 'd']);
    expect(resolveColumnOrder(page, ['c', 'a'])).toEqual(['c', 'a', 'b', 'd']);
  });

  test('repeat calls with the same (page, stored) reference return the identical array (memoized)', () => {
    const page = pageWithColumns(['a', 'b', 'c']);
    const stored = ['c', 'a'];
    const first = resolveColumnOrder(page, stored);
    const second = resolveColumnOrder(page, stored);
    expect(second).toBe(first);
  });

  test('a different stored array (even with equal content) is resolved independently, not confused with another', () => {
    const page = pageWithColumns(['a', 'b', 'c']);
    const first = resolveColumnOrder(page, ['c', 'a']);
    const second = resolveColumnOrder(page, ['c', 'a'].slice()); // same content, different reference
    expect(second).toEqual(first);
  });

  test('stays correct at a column count large enough that the old O(cols²) scan would be the dominant cost', () => {
    const names = Array.from({ length: 500 }, (_, i) => `c${i}`);
    const page = pageWithColumns(names);
    // A stored order that reverses the first 400 columns, leaving the rest to be appended.
    const stored = names.slice(0, 400).reverse();
    const result = resolveColumnOrder(page, stored);
    expect(result.slice(0, 400)).toEqual(stored);
    expect(result.slice(400)).toEqual(names.slice(400));
    expect(result).toHaveLength(500);
  });
});
