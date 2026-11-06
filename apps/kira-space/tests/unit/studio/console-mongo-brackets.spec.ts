// lintMongoBrackets (views/console/lint.ts) had zero coverage before this
// refactor pass, direct or indirect — the "before you touch it" bar CLAUDE.md's own adapter-
// conformance exemption doesn't cover, so this pins its behaviour first, ahead of the
// extraction of skipLineComment/scanStringLiteral out of it.
import { describe, expect, test } from 'bun:test';
import {
  consoleLintSource,
  lintMongoBrackets,
} from '../../../frontend/src/studio/views/console/lint';

describe('lintMongoBrackets', () => {
  test('Unterminated string literal.', () => {
    expect(lintMongoBrackets("'unterminated")).toEqual([
      { from: 0, to: 13, severity: 'error', message: 'Unterminated string literal.' },
    ]);
  });

  test('a bracket character inside a closed string is not a real bracket', () => {
    expect(lintMongoBrackets('"(unmatched"')).toEqual([]);
  });

  test('a bracket character inside a // comment is not a real bracket', () => {
    expect(lintMongoBrackets('// (\n)')).toEqual([
      { from: 5, to: 6, severity: 'error', message: 'Unmatched ).' },
    ]);
  });

  test('mismatched pair', () => {
    expect(lintMongoBrackets('(]')).toEqual([
      { from: 1, to: 2, severity: 'error', message: 'Unmatched ].' },
      { from: 0, to: 1, severity: 'error', message: 'Unbalanced (.' },
    ]);
  });

  test('unclosed at EOF', () => {
    expect(lintMongoBrackets('{[')).toEqual([
      { from: 0, to: 1, severity: 'error', message: 'Unbalanced {.' },
      { from: 1, to: 2, severity: 'error', message: 'Unbalanced [.' },
    ]);
  });
});

describe('consoleLintSource mongodb', () => {
  const lint = consoleLintSource('mongodb') as (t: string) => unknown[];
  test('leading comments and quotes inside block comments are not errors', () => {
    expect(lint('// users\ndb.users.find({})')).toEqual([]);
    expect(lint("/* don't */ db.a.find({})")).toEqual([]);
  });
  test('every statement is linted, offsets stay absolute', () => {
    expect(lint('db.a.find({});\ndb.b.nope({})')).toEqual([
      expect.objectContaining({ from: 20, to: 24, message: expect.stringContaining('nope') }),
    ]);
    expect(lint('db.a.find({}); db.b.find({x: })')).toHaveLength(1);
  });
});
