import { describe, expect, test } from 'bun:test';
import { parseTextSortTerms } from '../../../frontend/src/studio/views/grid/sortTerms';

describe('parseTextSortTerms quote handling', () => {
  const cols = ['id', 'Created At', 'a', 'b'];

  test('bare, double-quoted and backtick-quoted names all resolve', () => {
    expect(parseTextSortTerms('id desc, "Created At" ASC, `a`, b', cols)).toEqual([
      { column: 'id', direction: 'desc' },
      { column: 'Created At', direction: 'asc' },
      { column: 'a', direction: 'asc' },
      { column: 'b', direction: 'asc' },
    ]);
  });

  test('a bare-name prefix of a longer word is not a column', () => {
    expect(parseTextSortTerms('idx desc', cols)).toEqual([]);
  });
});
