export type { PageKind } from '../caps';

export type TypeClass = 'number' | 'text' | 'boolean' | 'temporal' | 'binary' | 'json' | 'other';

export interface ColumnDescriptor {
  name: string;
  /** The server's own type name, verbatim: 'numeric(20,6)', 'varchar(50)', 'longblob'. */
  dataType: string;
  typeClass: TypeClass;
  nullable: boolean;
  isPrimaryKey: boolean;
  /** A GENERATED/computed column (SQLite VIRTUAL/STORED, ClickHouse
   *  MATERIALIZED/ALIAS) — the server fills it in, an explicit value is refused. The renderer's
   *  own insert paths (add row, duplicate-as-insert, paste) skip it; `false` where an adapter
   *  never detects the concept (console results, or an adapter that hasn't wired detection yet). */
  generated: boolean;
}

/**
 * One column of one page. Three exactly-sized buffers and no per-row object:
 *   text of row i = utf8.decode(data.subarray(offsets[i], offsets[i + 1]))
 *   row i is NULL  = (nulls[i >> 3] & (1 << (i & 7))) !== 0
 * A NULL row has offsets[i] === offsets[i+1]; an empty string does too, which is why the
 * bitset is the only thing that distinguishes them (they must render differently).
 */
export interface TextColumnChunk {
  data: Uint8Array; // packed UTF-8, exactly-sized ArrayBuffer
  offsets: Uint32Array; // rowCount + 1 entries
  nulls: Uint8Array; // ceil(rowCount / 8) bytes
  /** Sorted row indices whose text was cut at MAX_CELL_BYTES. Usually empty. */
  truncated: Uint32Array;
  /** Sorted row indices whose text is base64 of non-UTF-8 bytes. Absent when none. */
  binary?: Uint32Array;
}

export interface PagePosition {
  /** Absolute row offset when the page came from an offset query; null for a keyset page. */
  offset: number | null;
  /** Informational and unread — a repo-wide grep (`grep -rn "position\.pageSize"
   *  src/ tests/`) finds no consumer. Every adapter writes it (some as the size requested, some as
   *  the size served — the two disagree by design where an adapter overshoots or clamps,
   *  e.g. redis/read.ts's SCAN-family round overshoot), but nothing downstream reads it back, so
   *  "requested or served?" has no observable answer. Not removed: it is on the wire
   *  and on every page every adapter builds, so deleting it would be an eleven-adapter
   *  change to delete something harmless. */
  pageSize: number;
  hasMore: boolean;
  nextToken: string | null;
  prevToken: string | null;
  strategy: 'keyset' | 'offset' | 'cursor' | 'offsetWindow' | 'batch';
  /** Console result stopped at the result cap; at least one more row existed. Absent == false (Go omits it). */
  truncated?: boolean;
}

// A page that is the whole result — no offset, no continuation, nothing more to fetch.
// Ten adapters' console/read paths wrote this six-field literal out by hand, its only variable
// being pageSize; the wire shape lives here, not under engine/adapters/,
// since callers span SQL and non-SQL adapters alike.
export function unpagedPosition(rowCount: number): PagePosition {
  return {
    offset: 0,
    pageSize: rowCount,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'offset',
  };
}

export interface TabularPage {
  kind: 'tabular';
  columns: ColumnDescriptor[];
  rowCount: number;
  chunks: TextColumnChunk[]; // index-aligned with `columns`
  position: PagePosition;
  truncatedCells: number;
  /** Measured, not estimated — this is what L2 budgets against (§2.2). */
  byteSize: number;
  fetchedAt: number; // epoch ms
}

/**
 * One document per row: `ids`/`bodies` are fixed semantic columns (not a caller-supplied
 * projection like TabularPage's `columns`) reusing the exact same TextColumnChunk codec —
 * `ids` holds each document's `_id` as EJSON text, `bodies` the full document as EJSON text.
 */
export interface DocumentPage {
  kind: 'document';
  position: PagePosition;
  ids: TextColumnChunk;
  bodies: TextColumnChunk;
  rowCount: number;
  byteSize: number;
  fetchedAt: number; // epoch ms
}

/**
 * One row per element of the key (hash field, list index, set/zset member, stream entry id) —
 * or a single row for a string. `fields`/`values` are fixed semantic columns reusing the exact
 * same TextColumnChunk codec as DocumentPage's `ids`/`bodies`. TTL/memory/type are whole-key
 * metadata, not per-row.
 *
 * Reuses this exact shape for a single s3 object (`redisType: 'object'`): `fields`/`values`
 * carry the object's metadata (ContentType, ContentLength, LastModified, ETag, StorageClass, ...)
 * plus, for an object at or under `SINGLE_ROW_MAX_BYTES`, a synthetic `Body` row for
 * its (possibly truncated) text content — a flat field/value listing is exactly what a hash-like
 * key already renders, and s3's own tree (bucket → prefix → object, '/'-delimited) already mirrors
 * redis's own namespace tree (db → namespace, ':'-delimited) closely enough that browsing one
 * object's contents needs no new page kind. `ttlMs` is always null for an s3 object (no such
 * concept there); `memoryBytes` carries the object's real `ContentLength` instead —
 * `KeyValueView.vue`'s existing size badge and its edit-size gate both read it from here.
 */
export interface KeyValuePage {
  kind: 'keyvalue';
  position: PagePosition;
  redisType: 'string' | 'hash' | 'list' | 'set' | 'zset' | 'stream' | 'object';
  ttlMs: number | null;
  memoryBytes: number | null;
  fields: TextColumnChunk;
  values: TextColumnChunk;
  rowCount: number;
  byteSize: number;
  fetchedAt: number; // epoch ms
}

/**
 * One row per message/record. `keys`/`headers`/`attrs`/`timestamps`/`bodies` are fixed semantic
 * columns reusing the exact same TextColumnChunk codec as DocumentPage's `ids`/`bodies` — `attrs`
 * is the one column whose meaning differs per engine (partition/offset JSON for Kafka, system
 * attributes JSON for SQS). `visibilityTimeoutSeconds` is SQS-only whole-page metadata
 * (null for Kafka), mirroring KeyValuePage's ttlMs/memoryBytes precedent.
 */
export interface StreamPage {
  kind: 'stream';
  position: PagePosition;
  keys: TextColumnChunk;
  headers: TextColumnChunk;
  attrs: TextColumnChunk;
  timestamps: TextColumnChunk;
  bodies: TextColumnChunk;
  rowCount: number;
  byteSize: number;
  fetchedAt: number; // epoch ms
  visibilityTimeoutSeconds: number | null;
  /** SQS redrive policy limit; null when none (and for Kafka). */
  maxReceiveCount: number | null;
}

export type Page = TabularPage | DocumentPage | KeyValuePage | StreamPage;

export const MAX_CELL_BYTES = 64 * 1024;
export const MAX_PAGE_SIZE = 10_000;
/** Per-value budget for a page that is one explicitly requested row (`singleRow`): a document
 *  body ("show all") or an s3 object's Body. Also the ceiling on an object body the
 *  app fetches, decodes and renders **at all**: a body that could only be shown truncated is a
 *  body whose remainder was transferred for nothing, now that Download hands over the whole file.
 *  Above this, nothing is fetched and no Body row exists. */
export const SINGLE_ROW_MAX_BYTES = MAX_CELL_BYTES * 64; // 4 MB

/** The ceiling on an object body the app will let the user edit and write back. Lower than
 *  the render ceiling because editing is a different cost class — a mutable CodeMirror buffer,
 *  a full re-encode and a PutObject of the result — and because a bad write is not undoable in
 *  an unversioned bucket. Sixteen cell budgets: far above a DB cell (a 200 KB JSON export is a
 *  normal thing to fix by hand), far below a document-sized page. */
export const OBJECT_BODY_EDIT_BYTES = MAX_CELL_BYTES * 16; // 1 MB

/** P33: AWS's hard limit for a single PutObject. Above it an upload needs multipart, which this
 *  phase does not implement (D12) — the adapter refuses with a message that says so. */
export const OBJECT_UPLOAD_MAX_BYTES = 5 * 1024 * 1024 * 1024; // 5 GiB

function bitsetBytes(rowCount: number): number {
  return Math.ceil(rowCount / 8);
}

function binarySearch(sorted: Uint32Array, target: number): number {
  let lo = 0;
  let hi = sorted.length - 1;
  while (lo <= hi) {
    const mid = (lo + hi) >>> 1;
    const v = sorted[mid];
    if (v === target) return mid;
    if (v < target) lo = mid + 1;
    else hi = mid - 1;
  }
  return -1;
}

/**
 * Cuts `bytes` at `maxBytes` on a UTF-8 code-point boundary, never mid-sequence — a split
 * surrogate makes the renderer's decoder emit U+FFFD and the cell looks corrupted.
 */
function truncateUtf8ToBoundary(
  bytes: Uint8Array<ArrayBuffer>,
  maxBytes: number,
): Uint8Array<ArrayBuffer> {
  let end = maxBytes;
  // A continuation byte (10xxxxxx) at `end` means the cut lands mid-sequence; back off to the
  // sequence's leading byte and drop the whole (necessarily incomplete) code point.
  while (end > 0 && (bytes[end] & 0xc0) === 0x80) end--;
  return bytes.subarray(0, end);
}

export function isNull(chunk: TextColumnChunk, row: number): boolean {
  return (chunk.nulls[row >> 3] & (1 << (row & 7))) !== 0;
}

export function cellText(chunk: TextColumnChunk, row: number, decoder: TextDecoder): string {
  return decoder.decode(chunk.data.subarray(chunk.offsets[row], chunk.offsets[row + 1]));
}

/** At most `maxBytes` of the cell, decoded non-fatally: a code point split at the cut becomes
 *  U+FFFD. For width measurement, which never needs the whole of a 64 KiB cell. */
export function cellPrefixText(
  chunk: TextColumnChunk,
  row: number,
  decoder: TextDecoder,
  maxBytes: number,
): string {
  const start = chunk.offsets[row];
  return decoder.decode(
    chunk.data.subarray(start, Math.min(chunk.offsets[row + 1], start + maxBytes)),
  );
}

// The raw UTF-8 byte length of row i's value, straight from the offsets this
// chunk already carries — for a caller that only wants a byte count (a "1.2 KB" badge), not the
// decoded text. Equal to `new TextEncoder().encode(cellText(chunk, row, decoder)).length` for any
// valid UTF-8 payload (decode-then-reencode round-trips exact byte length), without the decode and
// the redundant reencode a naive caller would otherwise do to get there.
export function cellByteLength(chunk: TextColumnChunk, row: number): number {
  return chunk.offsets[row + 1] - chunk.offsets[row];
}

export function isTruncated(chunk: TextColumnChunk, row: number): boolean {
  return binarySearch(chunk.truncated, row) >= 0;
}

export function isBinary(chunk: TextColumnChunk, row: number): boolean {
  return chunk.binary !== undefined && binarySearch(chunk.binary, row) >= 0;
}

export function chunkByteSize(chunk: TextColumnChunk): number {
  return (
    chunk.data.byteLength +
    chunk.offsets.byteLength +
    chunk.nulls.byteLength +
    chunk.truncated.byteLength +
    (chunk.binary?.byteLength ?? 0)
  );
}

// Per-column object overhead estimate: name + dataType at 2 bytes/char (UTF-16) + a fixed
// allowance for the descriptor's own object shape. A measurement, not a guess — L2's budget
// is only as honest as this number (§4a).
const COLUMN_ENVELOPE_BYTES = 64;

export function pageByteSize(page: TabularPage): number {
  let total = 0;
  for (let i = 0; i < page.chunks.length; i++) {
    const chunk = page.chunks[i];
    const column = page.columns[i];
    total += chunkByteSize(chunk) + (column.name.length + column.dataType.length) * 2;
    total += COLUMN_ENVELOPE_BYTES;
  }
  return total;
}

/** P5 C1/F8: every TextColumnChunk a page carries, regardless of kind — one page-kind-agnostic
 *  way to enumerate them, for the retention probe's own "distinct retained frame buffers" figure
 *  (main.ts's `__kiraRetention`) and for `frame.ts`'s per-page buffer copy (C7). */
export function pageChunks(page: Page): TextColumnChunk[] {
  switch (page.kind) {
    case 'tabular':
      return page.chunks;
    case 'document':
      return [page.ids, page.bodies];
    case 'keyvalue':
      return [page.fields, page.values];
    case 'stream':
      return [page.keys, page.headers, page.attrs, page.timestamps, page.bodies];
  }
}

/**
 * Accumulates one column's rows into growable scratch and only copies into exactly-sized
 * buffers at `finish()` (a view over an oversized buffer clones the slack).
 * `reverse()` is honoured entirely inside `finish()` by choosing which row order to copy in —
 * every row's byte range is already addressable via `rowStart`, so no extra buffer is needed.
 */
class ColumnScratch {
  private buffer = new Uint8Array(256);
  private used = 0;
  private readonly rowStart: number[] = [0];
  private readonly isNullRow: boolean[] = [];
  private readonly truncatedRows = new Set<number>();

  private grow(extra: number): void {
    if (this.used + extra <= this.buffer.length) return;
    let size = this.buffer.length * 2;
    while (size < this.used + extra) size *= 2;
    const next = new Uint8Array(size);
    next.set(this.buffer.subarray(0, this.used));
    this.buffer = next;
  }

  /** Returns whether this row's value was truncated. */
  appendValue(
    value: string | null,
    row: number,
    encoder: TextEncoder,
    maxBytes: number = MAX_CELL_BYTES,
  ): boolean {
    if (value === null) {
      this.isNullRow.push(true);
      this.rowStart.push(this.used);
      return false;
    }
    this.isNullRow.push(false);
    let bytes = encoder.encode(value);
    let truncated = false;
    if (bytes.length > maxBytes) {
      bytes = truncateUtf8ToBoundary(bytes, maxBytes);
      this.truncatedRows.add(row);
      truncated = true;
    }
    this.grow(bytes.length);
    this.buffer.set(bytes, this.used);
    this.used += bytes.length;
    this.rowStart.push(this.used);
    return truncated;
  }

  finish(rowCount: number, reversed: boolean): TextColumnChunk {
    const nulls = new Uint8Array(bitsetBytes(rowCount));
    const offsets = new Uint32Array(rowCount + 1);
    const data = new Uint8Array(this.used);
    const truncated: number[] = [];

    let cursor = 0;
    for (let newRow = 0; newRow < rowCount; newRow++) {
      const oldRow = reversed ? rowCount - 1 - newRow : newRow;
      const start = this.rowStart[oldRow];
      const end = this.rowStart[oldRow + 1];
      const len = end - start;
      if (len > 0) data.set(this.buffer.subarray(start, end), cursor);
      cursor += len;
      offsets[newRow + 1] = cursor;
      if (this.isNullRow[oldRow]) nulls[newRow >> 3] |= 1 << (newRow & 7);
      if (this.truncatedRows.has(oldRow)) truncated.push(newRow);
    }
    truncated.sort((a, b) => a - b);
    return { data, offsets, nulls, truncated: new Uint32Array(truncated) };
  }
}

export interface TabularPageBuilder {
  /** One row, one string-or-null per column, in `columns` order. */
  appendRow(values: readonly (string | null)[]): void;
  /** Reverses the accumulated rows before finishing — used by a keyset 'before' page (D7). */
  reverse(): void;
  finish(position: PagePosition): TabularPage;
}

export function createTabularPageBuilder(columns: ColumnDescriptor[]): TabularPageBuilder {
  const scratches = columns.map(() => new ColumnScratch());
  const encoder = new TextEncoder();
  let rowCount = 0;
  let reversed = false;
  let truncatedCells = 0;

  return {
    appendRow(values) {
      if (values.length !== columns.length) {
        throw new Error(`row has ${values.length} values, expected ${columns.length} columns`);
      }
      const row = rowCount;
      for (let i = 0; i < columns.length; i++) {
        if (scratches[i].appendValue(values[i] ?? null, row, encoder)) truncatedCells++;
      }
      rowCount++;
    },
    reverse() {
      reversed = true;
    },
    finish(position) {
      const chunks = scratches.map((s) => s.finish(rowCount, reversed));
      const page: TabularPage = {
        kind: 'tabular',
        columns,
        rowCount,
        chunks,
        position,
        truncatedCells,
        byteSize: 0,
        fetchedAt: Date.now(),
      };
      page.byteSize = pageByteSize(page);
      return page;
    },
  };
}

/**
 * DocumentPageBuilder/KeyValuePageBuilder/StreamPageBuilder each accumulated 2-5 fixed semantic
 * columns into their own named ColumnScratch instances, differing only in field count, per-field
 * maxBytes and (Stream) push()'s own row-object field names. `keys` stays a literal tuple (K
 * extends string, not string) so `finishColumns()` returns `Record<'ids' | 'bodies',
 * TextColumnChunk>`, not `Record<string, Chunk>` — no Page field's type widens, and each builder's
 * own `push(id, body)` / `push(field, value)` / `push(row)` public shape is unchanged below.
 */
function createColumnarBuilder<K extends string>(keys: readonly K[], maxBytes: (key: K) => number) {
  const scratches = new Map<K, ColumnScratch>(keys.map((k) => [k, new ColumnScratch()]));
  const encoder = new TextEncoder();
  let rowCount = 0;

  return {
    push(values: Record<K, string | null>): void {
      const row = rowCount;
      for (const k of keys) {
        // biome-ignore lint/style/noNonNullAssertion: scratches is built from the same `keys`
        scratches.get(k)!.appendValue(values[k], row, encoder, maxBytes(k));
      }
      rowCount++;
    },
    finishColumns(): { columns: Record<K, TextColumnChunk>; byteSize: number } {
      const columns = {} as Record<K, TextColumnChunk>;
      let byteSize = 0;
      for (const k of keys) {
        // biome-ignore lint/style/noNonNullAssertion: scratches is built from the same `keys`
        const chunk = scratches.get(k)!.finish(rowCount, false);
        columns[k] = chunk;
        byteSize += chunkByteSize(chunk);
      }
      return { columns, byteSize };
    },
    rowCount: () => rowCount,
  };
}

/** Mirrors `createTabularPageBuilder`, fixed to the two semantic columns `DocumentPage` uses. */
export interface DocumentPageBuilder {
  /** `id`/`body` are each pre-serialized EJSON text (caller owns EJSON.stringify). */
  push(id: string, body: string): void;
  finish(position: PagePosition): DocumentPage;
}

export function createDocumentPageBuilder(opts?: { singleRow?: boolean }): DocumentPageBuilder {
  const maxBytes = opts?.singleRow ? SINGLE_ROW_MAX_BYTES : MAX_CELL_BYTES;
  const columnar = createColumnarBuilder(['ids', 'bodies'] as const, () => maxBytes);

  return {
    push(id, body) {
      columnar.push({ ids: id, bodies: body });
    },
    finish(position) {
      const { columns, byteSize } = columnar.finishColumns();
      return {
        kind: 'document',
        position,
        ids: columns.ids,
        bodies: columns.bodies,
        rowCount: columnar.rowCount(),
        byteSize,
        fetchedAt: Date.now(),
      };
    },
  };
}

/** Mirrors `createDocumentPageBuilder`, fixed to the two semantic columns `KeyValuePage` uses. */
export interface KeyValuePageBuilder {
  push(field: string, value: string): void;
  finish(position: PagePosition): KeyValuePage;
}

export function createKeyValuePageBuilder(opts: {
  redisType: KeyValuePage['redisType'];
  ttlMs: number | null;
  memoryBytes: number | null;
  // Mirrors createDocumentPageBuilder's own singleRow — SINGLE_ROW_MAX_BYTES's "a
  // bigger budget when this is the one thing being fetched directly" reasoning applies just as
  // well to an s3 object's Body field as it does to a document; every redis type keeps the
  // plain MAX_CELL_BYTES default since none of them have a single dominant large-value field.
  singleRow?: boolean;
}): KeyValuePageBuilder {
  const valueMaxBytes = opts.singleRow ? SINGLE_ROW_MAX_BYTES : MAX_CELL_BYTES;
  const columnar = createColumnarBuilder(['fields', 'values'] as const, (k) =>
    k === 'values' ? valueMaxBytes : MAX_CELL_BYTES,
  );

  return {
    push(field, value) {
      columnar.push({ fields: field, values: value });
    },
    finish(position) {
      const { columns, byteSize } = columnar.finishColumns();
      return {
        kind: 'keyvalue',
        position,
        redisType: opts.redisType,
        ttlMs: opts.ttlMs,
        memoryBytes: opts.memoryBytes,
        fields: columns.fields,
        values: columns.values,
        rowCount: columnar.rowCount(),
        byteSize,
        fetchedAt: Date.now(),
      };
    },
  };
}

/** Mirrors `createKeyValuePageBuilder`, fixed to the five semantic columns `StreamPage` uses. */
export interface StreamPageBuilder {
  push(row: {
    key: string | null;
    headers: string;
    attrs: string;
    timestamp: string | null;
    body: string | null;
  }): void;
  finish(position: PagePosition): StreamPage;
}

export function createStreamPageBuilder(opts: {
  visibilityTimeoutSeconds: number | null;
  maxReceiveCount?: number | null;
}): StreamPageBuilder {
  const columnar = createColumnarBuilder(
    ['keys', 'headers', 'attrs', 'timestamps', 'bodies'] as const,
    () => MAX_CELL_BYTES,
  );

  return {
    push(row) {
      columnar.push({
        keys: row.key,
        headers: row.headers,
        attrs: row.attrs,
        timestamps: row.timestamp,
        bodies: row.body,
      });
    },
    finish(position) {
      const { columns, byteSize } = columnar.finishColumns();
      return {
        kind: 'stream',
        position,
        keys: columns.keys,
        headers: columns.headers,
        attrs: columns.attrs,
        timestamps: columns.timestamps,
        bodies: columns.bodies,
        rowCount: columnar.rowCount(),
        byteSize,
        fetchedAt: Date.now(),
        visibilityTimeoutSeconds: opts.visibilityTimeoutSeconds,
        maxReceiveCount: opts.maxReceiveCount ?? null,
      };
    },
  };
}

function assertChunkStructure(chunk: TextColumnChunk, rowCount: number, label: string): void {
  if (!(chunk.data instanceof Uint8Array)) throw new Error(`${label}.data is not a Uint8Array`);
  if (!(chunk.offsets instanceof Uint32Array)) {
    throw new Error(`${label}.offsets is not a Uint32Array`);
  }
  if (!(chunk.nulls instanceof Uint8Array)) throw new Error(`${label}.nulls is not a Uint8Array`);
  if (!(chunk.truncated instanceof Uint32Array)) {
    throw new Error(`${label}.truncated is not a Uint32Array`);
  }
  if (chunk.offsets.length !== rowCount + 1) {
    throw new Error(
      `${label}.offsets has ${chunk.offsets.length} entries, expected ${rowCount + 1}`,
    );
  }
  if (chunk.nulls.length !== bitsetBytes(rowCount)) {
    throw new Error(
      `${label}.nulls has ${chunk.nulls.length} bytes, expected ${bitsetBytes(rowCount)}`,
    );
  }
}

/** Throws when a page's typed arrays disagree with its own envelope fields. */
export function assertPageStructure(page: Page): void {
  if (page.kind === 'tabular') {
    if (page.chunks.length !== page.columns.length) {
      throw new Error(`page has ${page.chunks.length} chunks for ${page.columns.length} columns`);
    }
    for (const chunk of page.chunks) assertChunkStructure(chunk, page.rowCount, 'chunk');
    return;
  }
  if (page.kind === 'document') {
    assertChunkStructure(page.ids, page.rowCount, 'ids');
    assertChunkStructure(page.bodies, page.rowCount, 'bodies');
    return;
  }
  if (page.kind === 'keyvalue') {
    assertChunkStructure(page.fields, page.rowCount, 'fields');
    assertChunkStructure(page.values, page.rowCount, 'values');
    return;
  }
  assertChunkStructure(page.keys, page.rowCount, 'keys');
  assertChunkStructure(page.headers, page.rowCount, 'headers');
  assertChunkStructure(page.attrs, page.rowCount, 'attrs');
  assertChunkStructure(page.timestamps, page.rowCount, 'timestamps');
  assertChunkStructure(page.bodies, page.rowCount, 'bodies');
}
