// visibleColumnOrder is read per cell (displayCell/rowSnapshot), so it must return the same array
// reference per (page, stored order, query). Rules that interact: stored order, PK pin, no-match
// fallback, whitespace/comma term split.
import { describe, expect, test } from 'bun:test';
import {
  createTabularPageBuilder,
  unpagedPosition,
} from '../../../../../packages/shared/protocol/page';
import { visibleColumnOrder } from '../../../frontend/src/studio/views/grid/columnFilter';
import { resolveColumnOrder } from '../../../frontend/src/studio/views/shared/page/columns';

function pageWithColumns(names: string[], pk: string[] = []) {
  const columns = names.map((name) => ({
    name,
    dataType: 'text',
    typeClass: 'text' as const,
    nullable: true,
    isPrimaryKey: pk.includes(name),
    generated: false,
  }));
  const builder = createTabularPageBuilder(columns);
  builder.appendRow(names.map(() => 'v'));
  return builder.finish(unpagedPosition(1));
}

describe('visibleColumnOrder', () => {
  const page = pageWithColumns(['id', 'email', 'phone', 'name', 'Email_alt'], ['id']);

  test('repeat call returns the same array reference', () => {
    expect(visibleColumnOrder(page, null, 'email')).toBe(visibleColumnOrder(page, null, 'email'));
  });

  test('blank query returns the resolveColumnOrder result by reference', () => {
    expect(visibleColumnOrder(page, null, '  , ')).toBe(resolveColumnOrder(page, null));
  });

  test('matching is case-insensitive substring and keeps the PK', () => {
    expect(visibleColumnOrder(page, null, 'EMAIL')).toEqual(['id', 'email', 'Email_alt']);
  });

  test('stored order is respected', () => {
    const stored = ['Email_alt', 'name', 'id', 'phone', 'email'];
    expect(visibleColumnOrder(page, stored, 'email')).toEqual(['Email_alt', 'id', 'email']);
  });

  test('a PK-only match shows the PK only', () => {
    expect(visibleColumnOrder(page, null, 'id')).toEqual(['id']);
  });

  test('zero matches returns the full order', () => {
    expect(visibleColumnOrder(page, null, 'zzz')).toBe(resolveColumnOrder(page, null));
  });

  test('terms split on whitespace and commas, OR-ed', () => {
    expect(visibleColumnOrder(page, null, 'phone, name')).toEqual(['id', 'phone', 'name']);
    expect(visibleColumnOrder(page, null, 'phone name')).toEqual(['id', 'phone', 'name']);
  });
});
