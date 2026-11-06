// rowsToInsert used to hard-code `"${c}"` for the column
// list regardless of dialect (a syntax error on MySQL/MariaDB/ClickHouse, where a double-quoted
// column name is a string literal, not an identifier) and staged a truncated cell's 64 KiB prefix
// as if it were the real value.
import { describe, expect, test } from 'bun:test';
import { rowsToInsert } from '../../../frontend/src/studio/views/shared/clipboardFormats';

describe('rowsToInsert', () => {
  test('postgres: double-quoted columns, single-quoted values', () => {
    const sql = rowsToInsert(
      'public.orders',
      [{ columns: ['id', 'name'], values: { id: '1', name: "O'Brien" } }],
      'postgres',
    );
    expect(sql).toBe("INSERT INTO public.orders (\"id\", \"name\")\nVALUES\n  ('1', 'O''Brien');");
  });

  test('mysql: backtick-quoted columns, not the double-quoted string literal that broke the statement', () => {
    const sql = rowsToInsert(
      'db.orders',
      [{ columns: ['id', 'name'], values: { id: '1', name: 'x' } }],
      'mysql',
    );
    expect(sql).toContain('INSERT INTO db.orders (`id`, `name`)');
  });

  test('mysql: a backslash in a value is doubled, not left to eat the closing quote', () => {
    const sql = rowsToInsert(
      'db.files',
      [{ columns: ['path'], values: { path: 'C:\\new' } }],
      'mysql',
    );
    expect(sql).toContain("VALUES\n  ('C:\\\\new');");
  });

  test('a truncated cell is never staged as the real value — NULL, with a visible note', () => {
    const sql = rowsToInsert(
      'public.docs',
      [
        {
          columns: ['id', 'body'],
          values: { id: '1', body: 'THE_STALE_PREFIX_A_CLIENT_MUST_NEVER_WRITE_BACK' },
          truncated: new Set(['body']),
        },
      ],
      'postgres',
    );
    expect(sql).toContain("('1', NULL)");
    expect(sql).toContain('body');
    expect(sql).toContain('truncated');
    expect(sql).not.toContain('THE_STALE_PREFIX_A_CLIENT_MUST_NEVER_WRITE_BACK');
  });

  test('no truncation, no note', () => {
    const sql = rowsToInsert('t', [{ columns: ['a'], values: { a: '1' } }], 'postgres');
    expect(sql).not.toContain('truncated');
  });
});
