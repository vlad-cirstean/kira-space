// Generated SQL string literals (Filter by this value, FK
// navigation, Copy as INSERT) escaped only `'`, never consulting backslashEscapesFor even though
// that helper already exists and is already used for sql-split.ts/sqlLint.ts's own quote
// scanning. On MySQL/MariaDB/ClickHouse, a value containing a backslash either silently fails to
// match (`\n` read as a newline) or produces an unterminated literal (a value ending in `\`).
import { describe, expect, test } from 'bun:test';
import { quoteLiteral } from '../../../frontend/src/studio/views/shared/sqlIdent';

describe('quoteLiteral', () => {
  test('postgres/sqlite: a backslash is an ordinary character, only the quote is doubled', () => {
    expect(quoteLiteral('postgres', String.raw`C:\new`)).toBe(String.raw`'C:\new'`);
    expect(quoteLiteral('sqlite', 'back\\')).toBe("'back\\'");
  });

  test("mysql/clickhouse: a backslash is doubled first, so it can't eat the closing quote", () => {
    expect(quoteLiteral('mysql', String.raw`C:\new`)).toBe(String.raw`'C:\\new'`);
    expect(quoteLiteral('clickhouse', 'back\\')).toBe("'back\\\\'");
  });

  test('a single quote is still doubled on every dialect', () => {
    expect(quoteLiteral('postgres', "O'Brien")).toBe("'O''Brien'");
    expect(quoteLiteral('mysql', "O'Brien")).toBe("'O''Brien'");
  });

  test('undefined dialect (no SQL surface known) defaults to backslash-aware, matching backslashEscapesFor', () => {
    expect(quoteLiteral(undefined, String.raw`a\b`)).toBe(String.raw`'a\\b'`);
  });
});
