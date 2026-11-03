import { describe, expect, test } from 'bun:test';
import {
  type CellRow,
  findFilePaths,
  MAX_MATCHES_PER_LINE,
  toCellRange,
} from '../../../../packages/workbench/src/terminal/filePaths';

// The parser has interacting rules (token breaks, trailing punctuation, position suffix, URL
// exclusion, qualification), so each case pins one interaction.
function found(text: string) {
  return findFilePaths(text).map((m) => [m.path, m.line, m.col, text.slice(m.start, m.end)]);
}

describe('findFilePaths', () => {
  test.each([
    ['see src/a.go now', [['src/a.go', null, null, 'src/a.go']]],
    [
      './a.ts ../b/c.ts ~/d.json /abs/e.md',
      [
        ['./a.ts', null, null, './a.ts'],
        ['../b/c.ts', null, null, '../b/c.ts'],
        ['~/d.json', null, null, '~/d.json'],
        ['/abs/e.md', null, null, '/abs/e.md'],
      ],
    ],
    ['src/a.go:12', [['src/a.go', 12, null, 'src/a.go:12']]],
    ['src/a.go:12:5', [['src/a.go', 12, 5, 'src/a.go:12:5']]],
    ['src/a.go:12:5: error', [['src/a.go', 12, 5, 'src/a.go:12:5']]],
    ['wrote a.go.', [['a.go', null, null, 'a.go']]],
    ['a.go:0', [['a.go', null, null, 'a.go']]],
    ['a.go:12345678', [['a.go', null, null, 'a.go']]],
    ['a.go:3:0', [['a.go', null, null, 'a.go']]],
    ['("src/a.go:2")', [['src/a.go', 2, null, 'src/a.go:2']]],
    [
      '[a/b.go], `c/d.go`',
      [
        ['a/b.go', null, null, 'a/b.go'],
        ['c/d.go', null, null, 'c/d.go'],
      ],
    ],
    ['Makefile README bin/Makefile', [['bin/Makefile', null, null, 'bin/Makefile']]],
    ['-flag/x.go --out=a/b.go', []],
    ['.. ...', []],
    ['12:30:45 note', []],
  ])('%s', (text, want) => {
    expect(found(text)).toEqual(want);
  });

  test('web links are not paths', () => {
    expect(found('https://example.com/x.go')).toEqual([]);
    expect(found('(https://example.com/a,b/c.go) src/d.go')).toEqual([
      ['src/d.go', null, null, 'src/d.go'],
    ]);
    expect(found('https://example.com/a,b/c.go')).toEqual([]);
  });

  test('caps matches per line', () => {
    const text = Array.from({ length: MAX_MATCHES_PER_LINE + 1 }, (_, i) => `d/f${i}.go`).join(' ');
    const got = findFilePaths(text);
    expect(got).toHaveLength(MAX_MATCHES_PER_LINE);
    expect(got.at(-1)?.path).toBe(`d/f${MAX_MATCHES_PER_LINE - 1}.go`);
  });

  test('indices stay string indices past wide chars', () => {
    const text = '日本語 src/a.go:2';
    const [m] = findFilePaths(text);
    expect(text.slice(m?.start, m?.end)).toBe('src/a.go:2');
  });
});

function row(text: string): CellRow {
  const chars: number[] = [];
  const widths: number[] = [];
  for (const ch of text) {
    const wide = (ch.codePointAt(0) ?? 0) > 0x2e80;
    chars.push(ch.length);
    widths.push(wide ? 2 : 1);
    if (wide) {
      chars.push(0);
      widths.push(0);
    }
  }
  return { chars, widths };
}

describe('toCellRange', () => {
  test('a wide char before the path shifts cells but not string indices', () => {
    const text = '日本 a/b.go';
    const [m] = findFilePaths(text);
    expect(toCellRange([row(text)], m?.start ?? 0, m?.end ?? 0)).toEqual({
      startRow: 0,
      startCol: 5,
      endRow: 0,
      endCol: 10,
    });
  });

  test('a path wrapped across two rows spans both', () => {
    const rows = [row('xx src/long'), row('er/file.go yy')];
    const text = 'xx src/long' + 'er/file.go yy';
    const [m] = findFilePaths(text);
    expect(m?.path).toBe('src/longer/file.go');
    expect(toCellRange(rows, m?.start ?? 0, m?.end ?? 0)).toEqual({
      startRow: 0,
      startCol: 3,
      endRow: 1,
      endCol: 9,
    });
  });

  test('out of range gives null', () => {
    expect(toCellRange([row('ab')], 0, 5)).toBeNull();
    expect(toCellRange([row('ab')], 1, 1)).toBeNull();
  });
});
