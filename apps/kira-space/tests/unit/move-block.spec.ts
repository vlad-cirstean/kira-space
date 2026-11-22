// moveBlock's index arithmetic: a block lands after a later target and before an earlier one.
import { describe, expect, test } from 'bun:test';
import { moveBlock, moveId } from '@workbench/behaviour/sortableReorder';

const ids = ['a', 'b', 'c', 'd', 'e'];

describe('moveBlock', () => {
  test('block dropped onto a later tab lands after it', () => {
    expect(moveBlock(ids, ['a', 'b'], 'd')).toEqual(['c', 'd', 'a', 'b', 'e']);
  });

  test('block dropped onto an earlier tab lands before it', () => {
    expect(moveBlock(ids, ['d', 'e'], 'b')).toEqual(['a', 'd', 'e', 'b', 'c']);
  });

  test('non-contiguous set collapses in strip order', () => {
    expect(moveBlock(ids, ['d', 'b'], 'e')).toEqual(['a', 'c', 'e', 'b', 'd']);
    expect(moveBlock(ids, ['e', 'c'], 'a')).toEqual(['c', 'e', 'a', 'b', 'd']);
  });

  test('target past the first moved tab but before the last still lands after it', () => {
    expect(moveBlock(ids, ['a', 'e'], 'c')).toEqual(['b', 'c', 'a', 'e', 'd']);
  });

  test('target inside the set or unknown changes nothing', () => {
    expect(moveBlock(ids, ['b', 'd'], 'd')).toEqual(ids);
    expect(moveBlock(ids, ['b', 'd'], 'zz')).toEqual(ids);
    expect(moveBlock(ids, ['zz'], 'a')).toEqual(ids);
  });

  test('a block of one equals moveId', () => {
    for (const from of ids)
      for (const to of ids) expect(moveBlock(ids, [from], to)).toEqual(moveId(ids, from, to));
  });
});
