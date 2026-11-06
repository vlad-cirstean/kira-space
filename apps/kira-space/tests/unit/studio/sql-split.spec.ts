// shared/domain/sqlSplit.ts is a hand-written six-regime lexer that decides what SQL gets sent to
// the server (views/console/ConsoleView.vue's "Run statement"/"Run all"),
// and nothing in tests/ calls it directly. The only existing coverage is tests/e2e/console.spec.ts,
// which is Docker-gated (isDockerAvailable() -> test.skip) and has never run in this sandbox — and
// even on a box with containers, reproducing a dollar-quoted-body split means typing a multi-line
// PL/pgSQL function into CodeMirror and reading what the server rejects. Six direct expects over a
// pure function is the same fact in milliseconds instead of minutes, and the offset invariant
// (SqlStatement.start/.end index the *original* source, per the type's own doc comment) is not
// otherwise checked anywhere.
import { describe, expect, test } from 'bun:test';
import { splitSqlStatements, statementAtCursor } from '../../../../../packages/shared/domain/sqlSplit';

describe('splitSqlStatements — lexical regimes (P44 F43)', () => {
  test('1. a semicolon inside a single-quoted literal is not a boundary', () => {
    const stmts = splitSqlStatements(`SELECT 'a;b'; SELECT 2`);
    expect(stmts.map((s) => s.text)).toEqual([`SELECT 'a;b'`, 'SELECT 2']);
  });

  test('2. a doubled quote is an escape, not a close', () => {
    const stmts = splitSqlStatements(`SELECT 'it''s; still one'; SELECT 2`);
    expect(stmts.map((s) => s.text)).toEqual([`SELECT 'it''s; still one'`, 'SELECT 2']);
  });

  test('3a. MySQL/MariaDB/ClickHouse: a backslash escapes the next character', () => {
    const stmts = splitSqlStatements(`SELECT '\\'; still one'; SELECT 2`, {
      backslashEscapes: true,
    });
    expect(stmts).toHaveLength(2);
    expect(stmts[0]?.text).toBe(`SELECT '\\'; still one'`);
  });

  // P2 R2 (task #92): standard SQL has no backslash-escaping in a '...' literal — only a doubled
  // quote escapes. A literal ending in a real backslash immediately before its closing quote
  // (`'foo\'`) must not have that quote mistaken for an escaped character, or the scanner runs
  // on past the literal's true end and mis-splits everything after it.
  test('3b. Postgres/SQLite: a backslash is an ordinary character, not an escape', () => {
    const stmts = splitSqlStatements(`SELECT 'foo\\'; SELECT 2`, { backslashEscapes: false });
    expect(stmts.map((s) => s.text)).toEqual([`SELECT 'foo\\'`, 'SELECT 2']);
  });

  test('3c. splitSqlStatements defaults to backslashEscapes: true when no options are given', () => {
    const stmts = splitSqlStatements(`SELECT '\\'; still one'; SELECT 2`);
    expect(stmts).toHaveLength(2);
    expect(stmts[0]?.text).toBe(`SELECT '\\'; still one'`);
  });

  // A trailing backslash-escape as the very last character of source (with
  // backslashEscapes: true and no closing quote) steps scanSqlSpan's cursor two past that
  // backslash — one past source.length — before the loop notices it ran out of source. The
  // returned span's own end must still never exceed source.length.
  test('3d. an unterminated quote ending in a trailing backslash-escape never reports end past source.length', () => {
    const src = "'\\";
    const stmts = splitSqlStatements(src, { backslashEscapes: true });
    expect(stmts).toHaveLength(1);
    expect(stmts[0]?.end).toBeLessThanOrEqual(src.length);
  });

  test('4. -- runs to end of line', () => {
    const stmts = splitSqlStatements('SELECT 1; -- comment; not a boundary\nSELECT 2;');
    expect(stmts.map((s) => s.text)).toEqual(['SELECT 1', '-- comment; not a boundary\nSELECT 2']);
  });

  test('5. a block comment is skipped, including an unterminated one', () => {
    const terminated = splitSqlStatements('SELECT /* a; b */ 1; SELECT 2');
    expect(terminated.map((s) => s.text)).toEqual(['SELECT /* a; b */ 1', 'SELECT 2']);

    const unterminated = splitSqlStatements('SELECT 1; /* never closes; SELECT 2');
    expect(unterminated.map((s) => s.text)).toEqual(['SELECT 1', '/* never closes; SELECT 2']);
  });

  test('6. dollar-quoting protects a body with semicolons, and an unterminated tag swallows the rest', () => {
    const body = `CREATE FUNCTION f() RETURNS int AS $body$\nBEGIN\n  x := 1;\n  y := 2;\nEND;\n$body$ LANGUAGE plpgsql; SELECT 2`;
    const stmts = splitSqlStatements(body);
    expect(stmts).toHaveLength(2);
    expect(stmts[0]?.text).toContain('x := 1;');
    expect(stmts[0]?.text).toContain('y := 2;');
    expect(stmts[1]?.text).toBe('SELECT 2');

    const unterminated = splitSqlStatements('SELECT $$ never closes; SELECT 2');
    expect(unterminated).toHaveLength(1);
    expect(unterminated[0]?.text).toBe('SELECT $$ never closes; SELECT 2');
  });

  // `$` is a legal (if unusual) MySQL/MariaDB identifier character — an identifier containing two
  // of them used to read as an unterminated dollar-quote open tag regardless of dialect, swallowing
  // the rest of the document into one statement ("Run all" silently ran one statement instead of
  // two).
  test('6b. dollarQuoting: false treats $ as an ordinary character (MySQL identifiers)', () => {
    const stmts = splitSqlStatements('SELECT a$b$c FROM t; SELECT 2', { dollarQuoting: false });
    expect(stmts.map((s) => s.text)).toEqual(['SELECT a$b$c FROM t', 'SELECT 2']);
  });

  test('6c. splitSqlStatements defaults to dollarQuoting: true when no options are given', () => {
    const stmts = splitSqlStatements('SELECT $$ never closes; SELECT 2');
    expect(stmts).toHaveLength(1);
  });

  test('7. backtick- and double-quoted identifiers protect their own semicolons', () => {
    const backtick = splitSqlStatements('SELECT `a;b` FROM t; SELECT 2');
    expect(backtick.map((s) => s.text)).toEqual(['SELECT `a;b` FROM t', 'SELECT 2']);

    const doubled = splitSqlStatements('SELECT "a;b" FROM t; SELECT 2');
    expect(doubled.map((s) => s.text)).toEqual(['SELECT "a;b" FROM t', 'SELECT 2']);
  });

  test('8. empty statements are dropped', () => {
    expect(splitSqlStatements(';;  ;\n')).toEqual([]);
    expect(splitSqlStatements('')).toEqual([]);
  });

  test('9. start/end index the original source; text is trimmed', () => {
    const source = '  SELECT 1  ;  SELECT 2';
    const stmts = splitSqlStatements(source);
    expect(stmts[0]).toEqual({ text: 'SELECT 1', start: 0, end: 12 });
    const first = stmts[0];
    if (!first) throw new Error('expected a first statement');
    expect(source.slice(first.start, first.end)).toBe('  SELECT 1  ');
    expect(stmts[1]?.start).toBe(13);
    expect(stmts[1]?.end).toBe(source.length);
  });
});

describe('statementAtCursor (P44 F43)', () => {
  const source = 'SELECT 1; SELECT 2; SELECT 3';

  test('10a. a cursor inside a statement resolves to that statement', () => {
    expect(statementAtCursor(source, 0)?.text).toBe('SELECT 1');
    expect(statementAtCursor(source, 12)?.text).toBe('SELECT 2');
    expect(statementAtCursor(source, source.length)?.text).toBe('SELECT 3');
  });

  test('10b. a cursor past the end falls back to the last statement', () => {
    expect(statementAtCursor(source, source.length + 50)?.text).toBe('SELECT 3');
  });
});

// A caret right after a statement's own `;`, on the blank line between two
// statements, or in trailing whitespace at the end of the document, must resolve to the *preceding*
// statement — that is where the caret lands after typing `;` or pressing End. Boundary arithmetic
// over statementAtCursor's own range logic, not a restated function body.
describe('statementAtCursor: caret in whitespace belongs to the preceding statement (P108 Part 11 F3)', () => {
  const source = 'SELECT 1;\n\nDELETE FROM t;\n';
  const firstSemicolon = source.indexOf(';');
  const secondStatementStart = source.indexOf('DELETE');

  test('a caret right after the first `;` stays on the first statement', () => {
    expect(statementAtCursor(source, firstSemicolon + 1)?.text).toBe('SELECT 1');
  });

  test('a caret on the blank line between the two statements stays on the first statement', () => {
    expect(statementAtCursor(source, firstSemicolon + 2)?.text).toBe('SELECT 1');
  });

  test('a caret at the second statement’s own first character resolves to it', () => {
    expect(statementAtCursor(source, secondStatementStart)?.text).toBe('DELETE FROM t');
  });

  test('a caret in trailing whitespace at end of document stays on the last statement', () => {
    expect(statementAtCursor(source, source.length)?.text).toBe('DELETE FROM t');
  });
});

// P108 Part 11 F4: five dialect lexical forms the splitter used to miss entirely (each opt-in flag
// off by default so an untouched caller's split never changes) — one boundary case per form, per
// the finding's own "extend sql-split.spec.ts per form" fix text.
describe('splitSqlStatements — dialect lexical forms (P108 Part 11 F4)', () => {
  test('11. MySQL/ClickHouse: a `#` line comment hides a `;` until the newline', () => {
    const stmts = splitSqlStatements('SELECT 1; -- a\nSELECT 2 # a; b\nWHERE x; SELECT 3', {
      hashComments: true,
    });
    expect(stmts.map((s) => s.text)).toEqual([
      'SELECT 1',
      '-- a\nSELECT 2 # a; b\nWHERE x',
      'SELECT 3',
    ]);
  });

  test('11b. `#` is an ordinary character when hashComments is off', () => {
    const stmts = splitSqlStatements('SELECT 1 # 2; SELECT 3');
    expect(stmts.map((s) => s.text)).toEqual(['SELECT 1 # 2', 'SELECT 3']);
  });

  test('12. Mongo: a `//` line comment hides a `;` until the newline', () => {
    const stmts = splitSqlStatements('db.t.find({}); // a; b\ndb.t.find({})', {
      slashSlashComments: true,
    });
    expect(stmts.map((s) => s.text)).toEqual(['db.t.find({})', '// a; b\ndb.t.find({})']);
  });

  test('13. Postgres: a nested block comment only closes on its own matching `*/`', () => {
    const stmts = splitSqlStatements('SELECT /* a /* b; c */ d */ 1; SELECT 2', {
      nestedBlockComments: true,
    });
    expect(stmts.map((s) => s.text)).toEqual(['SELECT /* a /* b; c */ d */ 1', 'SELECT 2']);
  });

  test('13b. a non-nesting block comment closes on the first `*/` even with an inner `/*`', () => {
    const stmts = splitSqlStatements('SELECT /* a /* b */ c */ 1; SELECT 2');
    expect(stmts.map((s) => s.text)).toEqual(['SELECT /* a /* b */ c */ 1', 'SELECT 2']);
  });

  test('14. SQLite: a `[bracket identifier]` hides a `;` and doubles `]]` as an escape', () => {
    const stmts = splitSqlStatements('SELECT [a;b], [c ]] d] FROM t; SELECT 2', {
      bracketIdentifiers: true,
    });
    expect(stmts.map((s) => s.text)).toEqual(['SELECT [a;b], [c ]] d] FROM t', 'SELECT 2']);
  });

  test('14b. `[` is ordinary when bracketIdentifiers is off', () => {
    const stmts = splitSqlStatements('SELECT [a;b]');
    expect(stmts.map((s) => s.text)).toEqual(['SELECT [a', 'b]']);
  });

  test("15. Postgres: E'...' honours backslash escapes regardless of backslashEscapes", () => {
    const stmts = splitSqlStatements(`SELECT E'a\\'; still one'; SELECT 2`, {
      backslashEscapes: false,
      postgresEscapeStrings: true,
    });
    expect(stmts.map((s) => s.text)).toEqual([`SELECT E'a\\'; still one'`, 'SELECT 2']);
  });

  test("15b. a plain '...' still ignores backslashes when backslashEscapes is false", () => {
    const stmts = splitSqlStatements(`SELECT 'a\\'; still one'; SELECT 2`, {
      backslashEscapes: false,
      postgresEscapeStrings: true,
    });
    expect(stmts.map((s) => s.text)).toEqual([`SELECT 'a\\'`, `still one'; SELECT 2`]);
  });

  test("15c. a trailing 'E' that is part of a longer identifier is not an escape prefix", () => {
    // `TABLEE'...'` — the `E` is the tail of `TABLEE`, not a standalone escape-string prefix, so
    // postgresEscapeStrings must make no difference here: same (mis-)split as backslashEscapes:
    // false alone, not the E'...'-honours-backslashes behaviour case 15 exercises.
    const source = `SELECT TABLEE'x\\'y'; SELECT 2`;
    const withEscape = splitSqlStatements(source, {
      backslashEscapes: false,
      postgresEscapeStrings: true,
    });
    const withoutEscape = splitSqlStatements(source, { backslashEscapes: false });
    expect(withEscape.map((s) => s.text)).toEqual(withoutEscape.map((s) => s.text));
  });
});
