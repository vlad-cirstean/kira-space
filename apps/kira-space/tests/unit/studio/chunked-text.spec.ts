import { describe, expect, test } from 'bun:test';
import { pumpChunks, textChunks } from '../../../frontend/src/studio/editor/chunkedText';

const chunks = (t: string, chunkChars: number, lineAligned: boolean): string[] => [
  ...textChunks(t, { chunkChars, lineAligned }),
];

describe('textChunks', () => {
  test('line-aligned chunks end after a newline and concatenate to the input', () => {
    const t = 'aaa\nbbbb\ncc\ndddd\nee\nf';
    const out = chunks(t, 5, true);
    expect(out.join('')).toBe(t);
    for (const c of out.slice(0, -1)) expect(c.endsWith('\n')).toBe(true);
  });

  test('no newline within reach cuts at the nominal size', () => {
    const t = 'x'.repeat(25);
    expect(chunks(t, 10, true).map((c) => c.length)).toEqual([10, 10, 5]);
  });

  test('never splits a surrogate pair', () => {
    const t = '😀'.repeat(10);
    for (const c of chunks(t, 3, false)) expect(c.length % 2).toBe(0);
    expect(chunks(t, 3, false).join('')).toBe(t);
  });

  test('never splits \\r\\n when not line-aligned', () => {
    const t = 'abc\r\ndef\r\nghi';
    const out = chunks(t, 4, false);
    expect(out.join('')).toBe(t);
    for (const c of out) expect(c.endsWith('\r') && !c.endsWith('\r\n')).toBe(false);
  });
});

describe('pumpChunks', () => {
  test('applies every chunk and paces between them', async () => {
    const seen: string[] = [];
    let paced = 0;
    const done = await pumpChunks(['a', 'b', 'c'], (c) => seen.push(c), {
      pace: async () => {
        paced++;
      },
    });
    expect(done).toBe(true);
    expect(seen).toEqual(['a', 'b', 'c']);
    expect(paced).toBe(2);
  });

  test('abort stops further chunks and resolves false', async () => {
    const ctrl = new AbortController();
    const seen: string[] = [];
    const done = await pumpChunks(['a', 'b', 'c'], (c) => seen.push(c), {
      signal: ctrl.signal,
      pace: async () => ctrl.abort(),
    });
    expect(done).toBe(false);
    expect(seen).toEqual(['a']);
  });
});
