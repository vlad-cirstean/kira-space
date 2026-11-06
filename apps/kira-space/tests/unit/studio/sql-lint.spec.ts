// sql-lint.ts's lintSql shares sql-split.ts's quote-scanning lexer, including the
// same dialect-conditional backslash-escaping fix — a standard-SQL (Postgres/SQLite) literal ending
// in a real backslash right before its closing quote must not have that quote mistaken for an
// escaped character, or the scanner runs past the literal's true end and misreports it as
// unterminated (and any parenthesis inside the "swallowed" tail as unbalanced).
import { describe, expect, test } from 'bun:test';
import { lintSql } from '../../../../../packages/shared/domain/sqlLint';

describe('lintSql — backslash escaping is dialect-conditional (P2 R2, task #92)', () => {
  test('MySQL/MariaDB/ClickHouse: backslashEscapes true treats \\ as an escape inside a literal', () => {
    // The quote right after the backslash is escaped, so the literal actually runs to the *next*
    // quote — genuinely unterminated here since there is no second closing quote.
    const issues = lintSql(`SELECT '\\'`, { backslashEscapes: true });
    expect(issues).toHaveLength(1);
    expect(issues[0]?.message).toBe('Unterminated string literal.');
  });

  test('Postgres/SQLite: backslashEscapes false treats \\ as an ordinary character', () => {
    const issues = lintSql(`SELECT 'foo\\'; SELECT (1`, { backslashEscapes: false });
    // The literal 'foo\' closes normally at the real quote; the only real defect is the
    // unbalanced '(' in the second statement.
    expect(issues).toHaveLength(1);
    expect(issues[0]?.message).toBe('Unbalanced parenthesis.');
  });

  test('defaults to backslashEscapes: true when no options are given', () => {
    const issues = lintSql(`SELECT '\\'`);
    expect(issues).toHaveLength(1);
    expect(issues[0]?.message).toBe('Unterminated string literal.');
  });
});

// F10/P21 round 1: mirrors sql-split.ts's own dollarQuoting fix — `$` is a legal MySQL/MariaDB
// identifier character, so an identifier containing two of them must not be misread as an
// unterminated Postgres dollar-quote open tag on a non-Postgres dialect.
describe('lintSql — dollar-quoting is dialect-conditional (F10/P21 round 1)', () => {
  test('dollarQuoting: false treats $ as an ordinary character', () => {
    const issues = lintSql('SELECT a$b$c FROM t', { dollarQuoting: false });
    expect(issues).toHaveLength(0);
  });

  test('defaults to dollarQuoting: true when no options are given (a real Postgres tag)', () => {
    const issues = lintSql('SELECT $$ never closes');
    expect(issues).toHaveLength(1);
    expect(issues[0]?.message).toBe('Unterminated dollar-quoted string.');
  });
});
