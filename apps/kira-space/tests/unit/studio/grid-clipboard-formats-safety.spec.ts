//  - A cell is untrusted database content, but CSV/TSV copy handed it straight to a spreadsheet
//    — a value starting with `=`/`+`/`-`/`@` is evaluated as a formula on paste (CWE-1236).
//  - The TSV branch of parseDelimited kept the trailing empty row the CSV branch
//    already dropped, silently blanking or inserting an extra row on paste.
//  - rowsToTsv/columnsToTsv emitted tab/newline verbatim, so a cell containing
//    either corrupted the row layout on a copy→paste round trip.
//
// The formula-injection guard moved off TSV entirely.
// TSV is Plain Copy's own format, not an explicit spreadsheet-bound export — applying it there
// meant a plain range/row/column copy of `-5` wrote `'-5` to the clipboard, and pasting it back
// into this app's own grid staged the literal `'-5`, corrupting a same-app round trip to guard
// against a spreadsheet paste that was never happening. csvField (Copy as CSV, an explicit
// spreadsheet-bound format) and rowsToInsert (a literal handed to a SQL client) keep the guard.
import { describe, expect, test } from 'bun:test';
import {
  columnsToTsv,
  disambiguateNames,
  parseDelimited,
  rowsToCsv,
  rowsToJson,
  rowsToTsv,
} from '../../../frontend/src/studio/views/shared/clipboardFormats';

describe('formula-injection guard (finding 10)', () => {
  test('rowsToCsv prefixes a leading = + - @ with a single quote', () => {
    const rows = [
      { columns: ['a'], values: { a: '=IMPORTXML(A1)' } },
      { columns: ['a'], values: { a: '+1' } },
      { columns: ['a'], values: { a: '-1' } },
      { columns: ['a'], values: { a: '@SUM(A1)' } },
      { columns: ['a'], values: { a: 'plain text' } },
    ];
    const lines = rows.map((r) => rowsToCsv([r]));
    expect(lines[0]).toBe("'=IMPORTXML(A1)");
    expect(lines[1]).toBe("'+1");
    expect(lines[2]).toBe("'-1");
    expect(lines[3]).toBe("'@SUM(A1)");
    expect(lines[4]).toBe('plain text');
  });

  test('rowsToCsv still quotes the guarded value when it also needs CSV quoting', () => {
    const line = rowsToCsv([{ columns: ['a'], values: { a: '=A,B' } }]);
    expect(line).toBe('"\'=A,B"');
  });

  // F5 (P108 Part 10): TSV (Plain Copy) no longer guards — a same-app copy→paste round trip of
  // `-5`/`=1+1` must reproduce the exact value it copied, not silently mutate it into text.
  test('rowsToTsv does NOT prefix a formula-shaped value — TSV is Plain Copy, not a spreadsheet export', () => {
    const line = rowsToTsv([{ columns: ['a', 'b'], values: { a: '=1+1', b: 'ok' } }]);
    expect(line).toBe('=1+1\tok');
  });

  test('columnsToTsv does NOT prefix a formula-shaped value either', () => {
    const line = columnsToTsv([0], [0], () => ({ text: '=1+1', isNull: false }));
    expect(line).toBe('=1+1');
  });
});

describe('TSV escaping and round-trip (finding 7)', () => {
  test('rowsToTsv quotes a value containing an embedded tab or newline', () => {
    const line = rowsToTsv([{ columns: ['a', 'b'], values: { a: 'line one\nline two', b: 'x' } }]);
    expect(line).toBe('"line one\nline two"\tx');
  });

  test('copying a row with an embedded tab, then pasting, does not shift later columns', () => {
    const tsv = rowsToTsv([{ columns: ['a', 'b'], values: { a: 'has\ttab', b: 'second' } }]);
    const parsed = parseDelimited(tsv);
    expect(parsed).toEqual([['has\ttab', 'second']]);
  });

  test('copying a row with an embedded newline, then pasting, does not split it into two rows', () => {
    const tsv = rowsToTsv([
      { columns: ['a', 'b'], values: { a: 'line one\nline two', b: 'second' } },
    ]);
    const parsed = parseDelimited(tsv);
    expect(parsed).toEqual([['line one\nline two', 'second']]);
  });

  test('columnsToTsv round-trips an embedded newline the same way', () => {
    const tsv = columnsToTsv([0], [0, 1], (_r, c) =>
      c === 0 ? { text: 'line one\nline two', isNull: false } : { text: 'second', isNull: false },
    );
    expect(parseDelimited(tsv)).toEqual([['line one\nline two', 'second']]);
  });
});

describe('TSV trailing empty row (finding 6)', () => {
  test('a trailing newline (as every real clipboard TSV source adds) does not produce a bogus extra row', () => {
    expect(parseDelimited('a\tb\nc\td\n')).toEqual([
      ['a', 'b'],
      ['c', 'd'],
    ]);
  });

  test('a single trailing newline on one row is also dropped, not just multi-row input', () => {
    expect(parseDelimited('a\tb\n')).toEqual([['a', 'b']]);
  });

  test('a genuinely blank trailing row (two newlines) still round-trips as an empty row when it is not just the delimiter tail', () => {
    // Two consecutive newlines mean one real blank line in the middle, which must survive — only
    // the *very last* all-empty row (the delimiter's own trailing newline) is dropped.
    expect(parseDelimited('a\tb\n\nc\td\n')).toEqual([['a', 'b'], [''], ['c', 'd']]);
  });
});

// A single-cell copy used to be raw text while every other
// copy kind went through the (then formula-guarded) TSV quoting — same value, two different
// clipboard texts. And any clipboard text with no tab fell through to CSV regardless of its real
// source, so a single cell copied from this app itself split on commas/newlines on paste.
describe('single-cell copy/paste round trip (F5)', () => {
  test('text with no tab and no newline parses as one single value, never CSV-split', () => {
    expect(parseDelimited('Main St, Apt 4')).toEqual([['Main St, Apt 4']]);
    expect(parseDelimited('[1,2]')).toEqual([['[1,2]']]);
  });

  test('a single-cell copy (tsvField) and a one-cell range copy (columnsToTsv) produce the same text', () => {
    const value = 'Main St, Apt 4';
    const rangeCopy = columnsToTsv([0], [0], () => ({ text: value, isNull: false }));
    expect(rangeCopy).toBe(value);
  });

  test('a value containing a comma but no tab/quote/newline round-trips through copy and paste unchanged', () => {
    const rangeCopy = columnsToTsv([0], [0], () => ({ text: 'Main St, Apt 4', isNull: false }));
    expect(parseDelimited(rangeCopy)).toEqual([['Main St, Apt 4']]);
  });
});

// P108 Part 11 F7: console results routinely repeat a column name (self-joins, `SELECT 1 AS x, 2
// AS x`) — RowSnapshot keys `values` by name, so the later occurrence silently overwrote the
// earlier one's entry unless names are disambiguated first.
describe('disambiguateNames (F7)', () => {
  test('leaves a set of already-unique names untouched', () => {
    expect(disambiguateNames(['id', 'name', 'email'])).toEqual(['id', 'name', 'email']);
  });

  test('suffixes every occurrence of a repeated name past the first', () => {
    expect(disambiguateNames(['id', 'id'])).toEqual(['id', 'id_2']);
  });

  test('suffixes three occurrences with three distinct names', () => {
    expect(disambiguateNames(['x', 'x', 'x'])).toEqual(['x', 'x_2', 'x_3']);
  });

  test('skips a generated suffix that collides with a genuinely different real column', () => {
    // Two `id` columns plus a real `id_2` column: the second `id` can't become `id_2` (taken), so
    // it bumps past it.
    expect(disambiguateNames(['id', 'id', 'id_2'])).toEqual(['id', 'id_2', 'id_2_2']);
  });

  test('a self-join style repeat feeds rowsToTsv/rowsToCsv/rowsToJson without losing a column', () => {
    const names = disambiguateNames(['id', 'id']);
    const row = { columns: names, values: { id: '1', id_2: '2' } };
    expect(rowsToTsv([row])).toBe('1\t2');
    expect(rowsToCsv([row])).toBe('1,2');
    expect(JSON.parse(rowsToJson([row]))).toEqual([{ id: '1', id_2: '2' }]);
  });
});

// P168 Part 11 F14: a mid-field quote opened a quoted section and swallowed the rest of the
// clipboard into one cell; multi-line non-tabular text went through the CSV branch and spilled
// onto neighbouring rows.
describe('mid-field quotes and non-tabular text (F14)', () => {
  test('a quote inside an unquoted TSV field is literal', () => {
    expect(parseDelimited('12" ruler\tfoo\nbar\tbaz\n')).toEqual([
      ['12" ruler', 'foo'],
      ['bar', 'baz'],
    ]);
  });

  test('a quoted field still unwraps at field start', () => {
    expect(parseDelimited('"a\tb"\tc\n')).toEqual([['a\tb', 'c']]);
  });

  test('a multi-line JSON snippet pastes as one value', () => {
    const json = '{\n  "a": 1,\n  "b": "x, y"\n}';
    expect(parseDelimited(json)).toEqual([[json]]);
  });

  test('a rectangular multi-line column without tabs still pastes as rows', () => {
    expect(parseDelimited('a\nb\nc')).toEqual([['a'], ['b'], ['c']]);
  });
});
