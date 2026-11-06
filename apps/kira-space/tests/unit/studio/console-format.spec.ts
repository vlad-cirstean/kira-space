// formatConsoleText's Mongo branch is a composed statement parser (mongoStatement.ts) +
// argument splitter + emitter (ejson.ts's beautifyShellText) with real boundary cases — nested
// constructor calls, an empty argument list, a trailing comma — the "parser or splitter with
// several interacting lexical rules" CLAUDE.md's own bar names explicitly. The SQL branch is a
// per-kind dialect lookup plus a library call, plumbing well below that bar, and is covered by
// tests/ui/console-format.spec.ts alone.
//
// Every case below now carries its own `failures: []` — formatConsoleText's return shape
// gained a per-statement `failures` array, and the cases at the bottom exercise it directly:
// one statement failing no longer takes its neighbours down with it, and the statement COUNT
// survives every case (what the caret-by-index mapping in ConsoleView.onFormat depends on).
import { describe, expect, test } from 'bun:test';
import {
  formatConsoleText,
  joinFormattedStatements,
} from '../../../frontend/src/studio/views/console/format';

describe('formatConsoleText — mongodb', () => {
  test('one argument', async () => {
    expect(await formatConsoleText('mongodb', 'db.c.find({a:1})')).toEqual({
      text: 'db.c.find({\n  "a": 1\n})',
      ok: true,
      failures: [],
    });
  });

  test('an aggregate pipeline — the case the feature exists for', async () => {
    const result = await formatConsoleText(
      'mongodb',
      'db.widgets.aggregate([{$match:{a:1}},{$group:{_id:"$a"}}])',
    );
    expect(result).toEqual({
      text:
        'db.widgets.aggregate([\n' +
        '  {\n' +
        '    "$match": {\n' +
        '      "a": 1\n' +
        '    }\n' +
        '  },\n' +
        '  {\n' +
        '    "$group": {\n' +
        '      "_id": "$a"\n' +
        '    }\n' +
        '  }\n' +
        '])',
      ok: true,
      failures: [],
    });
  });

  test('two arguments — one per line at indent 2', async () => {
    expect(await formatConsoleText('mongodb', 'db.c.updateOne({a:1},{$set:{b:2}})')).toEqual({
      text:
        'db.c.updateOne(\n' +
        '  {\n' +
        '    "a": 1\n' +
        '  },\n' +
        '  {\n' +
        '    "$set": {\n' +
        '      "b": 2\n' +
        '    }\n' +
        '  }\n' +
        ')',
      ok: true,
      failures: [],
    });
  });

  test('no arguments — byte-identical, reported as an ordinary success with no failures', async () => {
    expect(await formatConsoleText('mongodb', 'db.c.countDocuments()')).toEqual({
      text: 'db.c.countDocuments()',
      ok: true,
      failures: [],
    });
  });

  test('a nested constructor argument is carried whole, never re-parsed', async () => {
    const result = await formatConsoleText(
      'mongodb',
      'db.c.find({_id: ObjectId("507f1f77bcf86cd799439011")})',
    );
    expect(result).toEqual({
      text: 'db.c.find({\n  "_id": ObjectId("507f1f77bcf86cd799439011")\n})',
      ok: true,
      failures: [],
    });
  });

  test('a trailing comma is dropped, not treated as an empty argument', async () => {
    expect(await formatConsoleText('mongodb', 'db.c.find({a:1,})')).toEqual({
      text: 'db.c.find({\n  "a": 1\n})',
      ok: true,
      failures: [],
    });
  });

  test('two ;-separated statements are rejoined with a blank line between them', async () => {
    const result = await formatConsoleText('mongodb', 'db.a.find({x:1});db.b.find({y:2})');
    expect(result).toEqual({
      text: 'db.a.find({\n  "x": 1\n});\n\ndb.b.find({\n  "y": 2\n})',
      ok: true,
      failures: [],
    });
  });

  test('an unsupported method fails with the linter own wording, text unchanged', async () => {
    const input = 'db.c.frobnicate({a:1})';
    expect(await formatConsoleText('mongodb', input)).toEqual({
      text: input,
      ok: false,
      reason: {
        key: 'studio.console.format.mongoMethod.error',
        params: { method: 'db.c.frobnicate()' },
      },
      failures: [
        {
          index: 0,
          reason: {
            key: 'studio.console.format.mongoMethod.error',
            params: { method: 'db.c.frobnicate()' },
          },
        },
      ],
    });
  });

  test('an unbalanced brace inside the argument fails, text unchanged', async () => {
    const input = 'db.c.find({a:1)';
    const result = await formatConsoleText('mongodb', input);
    expect(result.ok).toBe(false);
    expect(result.text).toBe(input);
    expect(result.failures).toEqual([
      { index: 0, reason: expect.objectContaining({ key: expect.any(String) }) },
    ]);
  });

  test('D13: one unparseable statement among three formats the other two, verbatim in place', async () => {
    const result = await formatConsoleText(
      'mongodb',
      'db.a.find({x:1});db.c.frobnicate({y:2});db.b.find({z:3})',
    );
    expect(result.ok).toBe(true);
    expect(result.failures).toEqual([
      {
        index: 1,
        reason: {
          key: 'studio.console.format.mongoMethod.error',
          params: { method: 'db.c.frobnicate()' },
        },
      },
    ]);
    expect(result.text).toBe(
      'db.a.find({\n  "x": 1\n});\n\ndb.c.frobnicate({y:2});\n\ndb.b.find({\n  "z": 3\n})',
    );
  });

  test('D13: every statement failing is still ok:false with the original text, statement count preserved', async () => {
    const input = 'db.c.frobnicate({x:1});db.d.frobnicate({y:2})';
    const result = await formatConsoleText('mongodb', input);
    expect(result.ok).toBe(false);
    expect(result.text).toBe(input);
    expect(result.failures).toHaveLength(2);
    expect(result.failures.map((f) => f.index)).toEqual([0, 1]);
  });
});

// splitSqlStatements' pushIfNonEmpty slices up to, not through, a statement's own ';'
// (sql-split.ts), so a plain join(';\n\n') emitted N-1 semicolons for N statements — silently
// deleting the document's LAST one on every press, a regression against the whole-document
// formatDialect call. None of the cases above happen to end their input in ';', which is exactly
// why the regression went unnoticed; these are the ones that would have caught it.
describe("formatConsoleText — D12 preserves the document's own trailing terminator", () => {
  test('a document ending in ";" still does', async () => {
    const result = await formatConsoleText('sqlite', 'select 1;');
    expect(result.ok).toBe(true);
    expect(result.text.endsWith(';')).toBe(true);
  });

  test('a document NOT ending in ";" still does not', async () => {
    const result = await formatConsoleText('sqlite', 'select 1');
    expect(result.ok).toBe(true);
    expect(result.text.endsWith(';')).toBe(false);
  });

  test('a multi-statement document keeps every internal ";" and its own trailing one', async () => {
    const result = await formatConsoleText('sqlite', 'select 1; select 2;');
    expect(result.ok).toBe(true);
    // Two statements means exactly two ';' — one between them, one at the end — never one fewer.
    expect(result.text.match(/;/g)).toHaveLength(2);
    expect(result.text.endsWith(';')).toBe(true);
  });

  test('the mongo branch (which also goes through splitSqlStatements, F20) keeps a trailing ";" too', async () => {
    const result = await formatConsoleText('mongodb', 'db.c.find({a:1});');
    expect(result.ok).toBe(true);
    expect(result.text).toBe('db.c.find({\n  "a": 1\n});');
  });

  test('a document where every statement fails is returned byte-identical, terminator included', async () => {
    // D12's own note: the all-failed branch already returns `text` untouched (the original
    // string, terminator and all) — correct by construction, not by this fix. Pinned here anyway
    // so a future refactor of that branch can't silently drop it.
    const input = 'db.c.frobnicate({x:1});';
    const result = await formatConsoleText('mongodb', input);
    expect(result.ok).toBe(false);
    expect(result.text).toBe(input);
  });
});

// `stmt.text` never carries its own ';' — when a formatted statement's own last
// line holds a line comment, a plain join(';\n\n') put the ';' inside that comment, and the
// splitter then saw one statement instead of two on the next Run all/Format press. Splitter rules
// (what counts as "the last line's own comment") interacting with the rejoin — the bar this
// codebase's unit tests apply.
describe('joinFormattedStatements — F9', () => {
  test('no trailing comment: an ordinary join, unchanged', () => {
    expect(joinFormattedStatements(['SELECT 1', 'SELECT 2'], false)).toBe('SELECT 1;\n\nSELECT 2');
  });

  test('a "--" comment on the statement\'s own last line moves the ";" onto its own line', () => {
    expect(joinFormattedStatements(['SELECT 1 -- first', 'SELECT 2'], false)).toBe(
      'SELECT 1 -- first\n;\n\nSELECT 2',
    );
  });

  test('a "#" comment only triggers the same fix when hashComments is true (MySQL/ClickHouse)', () => {
    expect(joinFormattedStatements(['SELECT 1 # first', 'SELECT 2'], false)).toBe(
      'SELECT 1 # first;\n\nSELECT 2', // sqlite/postgres: '#' is not a comment, plain join is correct
    );
    expect(joinFormattedStatements(['SELECT 1 # first', 'SELECT 2'], true)).toBe(
      'SELECT 1 # first\n;\n\nSELECT 2',
    );
  });

  test('a comment on a non-last line needs no fix — only the last line reaches the separator', () => {
    expect(
      joinFormattedStatements(['SELECT\n  1 -- comment on line 1\nFROM t', 'SELECT 2'], false),
    ).toBe('SELECT\n  1 -- comment on line 1\nFROM t;\n\nSELECT 2');
  });

  test('the last statement never gets a trailing separator or the extra newline', () => {
    expect(joinFormattedStatements(['SELECT 1', 'SELECT 2 -- last'], false)).toBe(
      'SELECT 1;\n\nSELECT 2 -- last',
    );
  });
});

describe('formatConsoleText — F9 end to end, real sql-formatter output', () => {
  test('SQLite: a "--" comment right before the original ";" no longer merges the next statement in', async () => {
    const result = await formatConsoleText('sqlite', 'SELECT 1 -- first\n;\nSELECT 2;');
    expect(result.ok).toBe(true);
    // The bug: sql-formatter keeps the comment on SELECT 1's own last line — a plain join put the
    // rejoin's ';' right after "first" on that same line, so the whole document re-split as ONE
    // statement. Fixed: the ';' lands on its own line, right after the comment, never inside it.
    expect(result.text).toContain('-- first\n;');
    expect(result.text).not.toMatch(/-- first;/);
  });

  test('MySQL: the same fix applies to a "#" comment, MySQL\'s own line-comment syntax', async () => {
    const result = await formatConsoleText('mysql', 'SELECT 1 # first\n;\nSELECT 2;');
    expect(result.ok).toBe(true);
    expect(result.text).toContain('# first\n;');
    expect(result.text).not.toMatch(/# first;/);
  });

  test('Mongo: a chained call after the closing paren is refused, not silently dropped', async () => {
    const input = 'db.c.find({}).limit(5)';
    const result = await formatConsoleText('mongodb', input);
    expect(result.ok).toBe(false);
    expect(result.text).toBe(input); // verbatim — .limit(5) is never lost
    expect(result.failures).toEqual([
      { index: 0, reason: { key: 'studio.console.format.trailing.error' } },
    ]);
  });
});
