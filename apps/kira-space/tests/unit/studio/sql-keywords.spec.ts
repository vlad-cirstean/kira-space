// The membership assertion the correctness requirement itself calls for — a word
// missing from a dialect's keyword set silently changes how sql-tokens.ts's scanner classifies it
// (Keyword vs. plain Identifier), which several consumers branch on by name. This is the one test
// that catches that failure mode; nothing else would (it's invisible at runtime until the exact
// SQL shape that depends on it appears).
import { describe, expect, test } from 'bun:test';
import {
  keywordsFor,
  REQUIRED_MINIMUM,
  type SqlDialect,
  typesFor,
} from '@shared/domain/sqlKeywords';

const DIALECTS: readonly SqlDialect[] = ['postgres', 'mysql', 'sqlite', 'clickhouse'];

describe('keywordsFor — every dialect carries the required minimum union', () => {
  for (const dialect of DIALECTS) {
    test(`${dialect}`, () => {
      const words = keywordsFor(dialect);
      const missing = [...REQUIRED_MINIMUM].filter((w) => !words.has(w));
      expect(missing).toEqual([]);
    });
  }
});

describe('keywordsFor — memoised, same reference across calls (D2 relies on this)', () => {
  test('postgres', () => {
    expect(keywordsFor('postgres')).toBe(keywordsFor('postgres'));
  });
});

describe('typesFor — a non-empty, dialect-specific vocabulary', () => {
  for (const dialect of DIALECTS) {
    test(`${dialect} has at least one common type name`, () => {
      expect(typesFor(dialect).size).toBeGreaterThan(0);
    });
  }

  test('clickhouse keeps its own curated type names (Array, LowCardinality, …)', () => {
    const types = typesFor('clickhouse');
    expect(types.has('array')).toBe(true);
    expect(types.has('lowcardinality')).toBe(true);
    expect(types.has('varchar')).toBe(false); // not a ClickHouse type
  });

  test('postgres/mysql share the standard vocabulary (varchar, integer, …)', () => {
    expect(typesFor('postgres').has('varchar')).toBe(true);
    expect(typesFor('mysql').has('varchar')).toBe(true);
  });
});
