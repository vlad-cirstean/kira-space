import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import type { ReadResponse } from '@shared/protocol/dataOps';
import { decodeFrame } from '@shared/protocol/frame';
import { cellText, isNull, type Page, type TextColumnChunk } from '@shared/protocol/page';

// Frames written by the real Go encoder (adapterhost/golden_frames_test.go, regenerated with
// KIRA_IPC_FIXTURES=write). Pins what Go puts on the wire, not what a TS test encoder agrees with.
const decoder = new TextDecoder();

function golden(name: string): Page {
  const bytes = new Uint8Array(
    readFileSync(
      new URL(`../../../internal/adapterhost/testdata/frames/${name}.bin`, import.meta.url),
    ),
  );
  const frame = decodeFrame(bytes);
  if (frame.kind !== 'res' || !frame.ok) throw new Error(`${name}: not an ok res frame`);
  expect(frame.id).toBe(7);
  return (frame.payload as ReadResponse).page;
}

function source(name: string): string {
  const bytes = new Uint8Array(
    readFileSync(
      new URL(`../../../internal/adapterhost/testdata/frames/${name}.bin`, import.meta.url),
    ),
  );
  const frame = decodeFrame(bytes);
  return frame.kind === 'res' && frame.ok ? (frame.payload as ReadResponse).source : '';
}

function column(chunk: TextColumnChunk, rows: number): (string | null)[] {
  return Array.from({ length: rows }, (_, i) =>
    isNull(chunk, i) ? null : cellText(chunk, i, decoder),
  );
}

describe('Go-encoded frames decode to the logical page', () => {
  test('tabular keyset: null row, empty string, truncated cell, tokens, offset absent', () => {
    const page = golden('tabular-keyset');
    if (page.kind !== 'tabular') throw new Error('kind');
    expect(
      page.columns.map((c) => [c.name, c.typeClass, c.isPrimaryKey, c.nullable, c.generated]),
    ).toEqual([
      ['id', 'number', true, false, false],
      ['note', 'text', false, true, false],
      ['doc', 'json', false, true, true],
    ]);
    expect(column(page.chunks[0], 3)).toEqual(['1', '2', '3']);
    expect(column(page.chunks[1], 3)).toEqual(['hello', null, '']);
    const doc = column(page.chunks[2], 3);
    expect(doc.slice(0, 2)).toEqual(['{"a":1}', null]);
    expect(page.truncatedCells).toBe(1);
    expect(Array.from(page.chunks[2].truncated)).toEqual([2]);
    expect(page.position).toEqual({
      offset: null,
      pageSize: 100,
      hasMore: true,
      nextToken: 'next',
      prevToken: 'prev',
      strategy: 'keyset',
    });
    expect(page.fetchedAt).toBe(1_700_000_000_000);
  });

  test('tabular offset page, cache source, absent tokens decode as null', () => {
    const page = golden('tabular-offset-cache');
    expect(source('tabular-offset-cache')).toBe('cache');
    expect(page.position).toEqual({
      offset: 0,
      pageSize: 1,
      hasMore: false,
      nextToken: null,
      prevToken: null,
      strategy: 'offset',
    });
  });

  test('document with a cursor position', () => {
    const page = golden('document-cursor');
    if (page.kind !== 'document') throw new Error('kind');
    expect(column(page.ids, 2)).toEqual(['"a"', '"b"']);
    expect(column(page.bodies, 2)).toEqual(['{"k":1}', '{}']);
    expect(page.position).toMatchObject({
      offset: 20,
      strategy: 'cursor',
      nextToken: 'cur',
      prevToken: null,
    });
  });

  test('keyvalue: set ttl/memory, absent ttl/memory, zero is not absent', () => {
    const hash = golden('keyvalue-hash-ttl');
    if (hash.kind !== 'keyvalue') throw new Error('kind');
    expect([hash.redisType, hash.ttlMs, hash.memoryBytes]).toEqual(['hash', 5000, 88]);
    expect(column(hash.fields, 1)).toEqual(['f1']);

    const str = golden('keyvalue-string-nottl');
    if (str.kind !== 'keyvalue') throw new Error('kind');
    expect([str.redisType, str.ttlMs, str.memoryBytes]).toEqual(['string', null, null]);

    const obj = golden('keyvalue-object-zero');
    if (obj.kind !== 'keyvalue') throw new Error('kind');
    expect([obj.redisType, obj.ttlMs, obj.memoryBytes]).toEqual(['object', 0, 0]);
  });

  test('stream: null key/timestamp/body vs empty body, visibility timeout set and absent', () => {
    const page = golden('stream-visibility-null-row');
    if (page.kind !== 'stream') throw new Error('kind');
    expect(page.visibilityTimeoutSeconds).toBe(30);
    expect(column(page.keys, 2)).toEqual(['k0', null]);
    expect(column(page.timestamps, 2)).toEqual(['2024-01-01T00:00:00.000Z', null]);
    expect(column(page.bodies, 2)).toEqual(['body', null]);
    expect(page.position.strategy).toBe('batch');

    const bare = golden('stream-novisibility');
    if (bare.kind !== 'stream') throw new Error('kind');
    expect(bare.visibilityTimeoutSeconds).toBeNull();
    expect(column(bare.bodies, 1)).toEqual(['']);
    expect(bare.position).toMatchObject({ offset: 0, strategy: 'offsetWindow' });
  });
});
