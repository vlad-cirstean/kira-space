// isExplainable's raw-semicolon guard, the TS twin of the Go test in
// statements_test.go. A leading SELECT/WITH is not enough — the splitter that decided this was
// "one statement" can itself be fooled (Postgres E'...' strings, `$` inside an identifier), and
// wrapping a merged statement in EXPLAIN would still execute whatever follows the `;` (Postgres's
// simple protocol runs every command in one Query message).
import { describe, expect, test } from 'bun:test';
import { isExplainable } from '../../../frontend/src/studio/views/console/explain';

describe('isExplainable rejects an embedded semicolon (P108 Part 11 F2)', () => {
  test.each([
    ['SELECT 1', true],
    ['SELECT 1;', true],
    ['SELECT 1;  \n', true],
    ['SELECT 1; DELETE FROM t', false],
    ['SELECT 1; DELETE FROM t;', false],
    ['-- note\nSELECT 1; DELETE FROM t', false],
    ['DELETE FROM t', false],
  ])('isExplainable(%j) === %j', (sql, expected) => {
    expect(isExplainable(sql)).toBe(expected);
  });
});
