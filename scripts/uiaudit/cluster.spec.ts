import { describe, expect, test } from 'bun:test';
import {
  jaccard,
  nearClusters,
  normaliseClasses,
  qualifiesSubtree,
  stableKey,
  THRESHOLDS,
} from './cluster';

const toks = (prefix: string, n: number): string[] =>
  Array.from({ length: n }, (_, i) => `${prefix}${i}`);

describe('normaliseClasses', () => {
  test('token order and duplicates do not matter', () => {
    expect(normaliseClasses('flex gap-1 flex items-center')).toEqual(
      normaliseClasses('items-center  gap-1 flex'),
    );
  });
  test('cn folds a conflicting utility to the later one', () => {
    expect(normaliseClasses('p-2 p-4 flex')).toEqual(['flex', 'p-4']);
  });
});

describe('stableKey', () => {
  test('10 hex, same input same key', () => {
    expect(stableKey('a')).toMatch(/^[0-9a-f]{10}$/);
    expect(stableKey('a')).toBe(stableKey('a'));
    expect(stableKey('a')).not.toBe(stableKey('b'));
  });
});

describe('near clusters', () => {
  const base = toks('t', 7);
  test('template threshold 0.7: 7/10 joins, 7/11 does not', () => {
    expect(THRESHOLDS.templateNearJaccard).toBe(0.7);
    const a = base;
    const atEdge = [...base, ...toks('x', 3)];
    const below = [...base, ...toks('y', 4)];
    expect(jaccard(a, atEdge)).toBeCloseTo(0.7);
    expect(nearClusters([{ tokens: a }, { tokens: atEdge }], 0.7)).toEqual([[0, 1]]);
    expect(nearClusters([{ tokens: a }, { tokens: below }], 0.7)).toEqual([]);
  });
  test('class threshold 0.8: 8/10 joins, 8/11 does not', () => {
    expect(THRESHOLDS.classNearJaccard).toBe(0.8);
    const a = toks('t', 8);
    const atEdge = [...a, ...toks('x', 2)];
    const below = [...a, ...toks('y', 3)];
    expect(nearClusters([{ tokens: a }, { tokens: atEdge }], 0.8)).toEqual([[0, 1]]);
    expect(nearClusters([{ tokens: a }, { tokens: below }], 0.8)).toEqual([]);
  });
  test('chains join through a middle set, small sets are skipped', () => {
    const a = toks('t', 9);
    const b = [...toks('t', 9), 'u'];
    const c = [...toks('t', 9), 'u', 'v'];
    expect(nearClusters([{ tokens: a }, { tokens: b }, { tokens: c }], 0.8, 4)).toEqual([
      [0, 1, 2],
    ]);
    expect(nearClusters([{ tokens: ['a', 'b'] }, { tokens: ['a', 'b'] }], 0.8, 4)).toEqual([]);
  });
});

describe('subtree floor', () => {
  test('hashed from 4 elements up', () => {
    expect(qualifiesSubtree(3)).toBe(false);
    expect(qualifiesSubtree(4)).toBe(true);
  });
});
