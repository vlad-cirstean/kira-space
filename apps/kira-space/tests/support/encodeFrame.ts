import type {
  ColumnDescriptor,
  DocumentPage,
  KeyValuePage,
  PagePosition,
  StreamPage,
  TabularPage,
  TextColumnChunk,
  TypeClass,
  Page as WirePage,
} from '@shared/protocol/page';
import * as wire from '@shared/protocol/wire';
import * as flatbuffers from 'flatbuffers';

// The test-support mirror of internal/page/encode.go + internal/adapterhost/frame.go: shared
// by tests/ui/support/mockStream.ts (which needs a template frame with id 0 and forceDefaults, so
// mockStreamBrowser.js can patch a real id into it per request) and tests/unit/bridge-port.spec.ts
// (which knows the real id up front and can encode it directly).

export interface EncodedFrame {
  bytes: Uint8Array;
  /** Absolute byte offset of the frame's `id` field within `bytes`. Only meaningful when the frame
   *  was built with `forceDefaults: true` — otherwise an `id` of 0 has no vtable slot to find. */
  idOffset: number;
}

/** One member per `wire.Payload` value (Payload.NONE excepted — nothing in this app ever sends
 *  it). Mirrors adapterhost/frame.go's `encodePayload` type switch, one level up: a plain literal
 *  instead of a Go `any`/TS `unknown` payload value. */
export type FramePayload =
  | { type: 'read'; page: WirePage; source: 'cache' | 'server' }
  | {
      type: 'count';
      value: number;
      exact: boolean;
      at: number;
      stale: boolean;
      source: 'cache' | 'server';
    }
  | { type: 'preview'; statements: string[] }
  | { type: 'mutate'; affectedRows: number }
  | { type: 'execute'; pages: WirePage[] }
  | { type: 'objectDownload'; bytes: number }
  | { type: 'ping'; enginePid: number; at: number }
  | {
      type: 'cacheStats';
      l2Bytes: number;
      l2BudgetBytes: number;
      l2Entries: number;
      l2Hits: number;
      l2Misses: number;
      l3Entries: number;
    }
  | { type: 'empty' };

export type FrameSpec =
  | { kind: 'res'; id: number; ok: true; payload: FramePayload; forceDefaults?: boolean }
  | {
      kind: 'res';
      id: number;
      ok: false;
      error: { message: string; code?: string };
      forceDefaults?: boolean;
    }
  | { kind: 'evt'; topic: string; payload: FramePayload; forceDefaults?: boolean };

function encodeTypeClass(t: TypeClass): wire.TypeClass {
  switch (t) {
    case 'number':
      return wire.TypeClass.number;
    case 'text':
      return wire.TypeClass.text;
    case 'boolean':
      return wire.TypeClass.boolean;
    case 'temporal':
      return wire.TypeClass.temporal;
    case 'binary':
      return wire.TypeClass.binary;
    case 'json':
      return wire.TypeClass.json;
    case 'other':
      return wire.TypeClass.other;
  }
}

function encodeStrategy(s: PagePosition['strategy']): wire.Strategy {
  switch (s) {
    case 'keyset':
      return wire.Strategy.keyset;
    case 'offset':
      return wire.Strategy.offset;
    case 'cursor':
      return wire.Strategy.cursor;
    case 'offsetWindow':
      return wire.Strategy.offsetWindow;
    case 'batch':
      return wire.Strategy.batch;
  }
}

// wire.fbs suffixes string_/set_ to dodge a reserved-word collision in generated TypeScript — the
// mirror of Go's encode.go encodeRedisType.
function encodeRedisType(t: KeyValuePage['redisType']): wire.RedisType {
  switch (t) {
    case 'string':
      return wire.RedisType.string_;
    case 'hash':
      return wire.RedisType.hash;
    case 'list':
      return wire.RedisType.list;
    case 'set':
      return wire.RedisType.set_;
    case 'zset':
      return wire.RedisType.zset;
    case 'stream':
      return wire.RedisType.stream;
    case 'object':
      return wire.RedisType.object;
  }
}

function encodeSource(s: 'cache' | 'server'): wire.Source {
  return s === 'cache' ? wire.Source.cache : wire.Source.server;
}

function encodeChunk(b: flatbuffers.Builder, c: TextColumnChunk): flatbuffers.Offset {
  const dataOff = wire.Chunk.createDataVector(b, c.data);
  const offsetsOff = wire.Chunk.createOffsetsVector(b, c.offsets);
  const nullsOff = wire.Chunk.createNullsVector(b, c.nulls);
  const truncatedOff = wire.Chunk.createTruncatedVector(b, c.truncated);
  const binaryOff = c.binary ? wire.Chunk.createBinaryVector(b, c.binary) : 0;
  // Not createChunk: under forceDefaults an absent `binary` would be written as a null offset.
  wire.Chunk.startChunk(b);
  wire.Chunk.addData(b, dataOff);
  wire.Chunk.addOffsets(b, offsetsOff);
  wire.Chunk.addNulls(b, nullsOff);
  wire.Chunk.addTruncated(b, truncatedOff);
  if (binaryOff) wire.Chunk.addBinary(b, binaryOff);
  return wire.Chunk.endChunk(b);
}

function encodeColumnDescriptor(b: flatbuffers.Builder, c: ColumnDescriptor): flatbuffers.Offset {
  const nameOff = b.createString(c.name);
  const dataTypeOff = b.createString(c.dataType);
  return wire.ColumnDescriptor.createColumnDescriptor(
    b,
    nameOff,
    dataTypeOff,
    encodeTypeClass(c.typeClass),
    c.nullable,
    c.isPrimaryKey,
    c.generated,
  );
}

function encodePagePosition(b: flatbuffers.Builder, p: PagePosition): flatbuffers.Offset {
  const nextTokenOff = p.nextToken !== null ? b.createString(p.nextToken) : 0;
  const prevTokenOff = p.prevToken !== null ? b.createString(p.prevToken) : 0;
  return wire.PagePosition.createPagePosition(
    b,
    p.offset,
    p.pageSize,
    p.hasMore,
    nextTokenOff,
    prevTokenOff,
    encodeStrategy(p.strategy),
    p.truncated ?? false,
  );
}

function encodeTabularPage(b: flatbuffers.Builder, p: TabularPage): flatbuffers.Offset {
  const columnOffs = p.columns.map((c) => encodeColumnDescriptor(b, c));
  const columnsOff = wire.TabularPage.createColumnsVector(b, columnOffs);
  const chunkOffs = p.chunks.map((c) => encodeChunk(b, c));
  const chunksOff = wire.TabularPage.createChunksVector(b, chunkOffs);
  const positionOff = encodePagePosition(b, p.position);

  wire.TabularPage.startTabularPage(b);
  wire.TabularPage.addColumns(b, columnsOff);
  wire.TabularPage.addRowCount(b, p.rowCount);
  wire.TabularPage.addChunks(b, chunksOff);
  wire.TabularPage.addPosition(b, positionOff);
  wire.TabularPage.addTruncatedCells(b, p.truncatedCells);
  wire.TabularPage.addByteSize(b, p.byteSize);
  wire.TabularPage.addFetchedAt(b, p.fetchedAt);
  return wire.TabularPage.endTabularPage(b);
}

function encodeDocumentPage(b: flatbuffers.Builder, p: DocumentPage): flatbuffers.Offset {
  const idsOff = encodeChunk(b, p.ids);
  const bodiesOff = encodeChunk(b, p.bodies);
  const positionOff = encodePagePosition(b, p.position);

  wire.DocumentPage.startDocumentPage(b);
  wire.DocumentPage.addPosition(b, positionOff);
  wire.DocumentPage.addIds(b, idsOff);
  wire.DocumentPage.addBodies(b, bodiesOff);
  wire.DocumentPage.addRowCount(b, p.rowCount);
  wire.DocumentPage.addByteSize(b, p.byteSize);
  wire.DocumentPage.addFetchedAt(b, p.fetchedAt);
  return wire.DocumentPage.endDocumentPage(b);
}

function encodeKeyValuePage(b: flatbuffers.Builder, p: KeyValuePage): flatbuffers.Offset {
  const fieldsOff = encodeChunk(b, p.fields);
  const valuesOff = encodeChunk(b, p.values);
  const positionOff = encodePagePosition(b, p.position);

  wire.KeyValuePage.startKeyValuePage(b);
  wire.KeyValuePage.addPosition(b, positionOff);
  wire.KeyValuePage.addRedisType(b, encodeRedisType(p.redisType));
  if (p.ttlMs !== null) wire.KeyValuePage.addTtlMs(b, p.ttlMs);
  if (p.memoryBytes !== null) wire.KeyValuePage.addMemoryBytes(b, p.memoryBytes);
  wire.KeyValuePage.addFields(b, fieldsOff);
  wire.KeyValuePage.addValues(b, valuesOff);
  wire.KeyValuePage.addRowCount(b, p.rowCount);
  wire.KeyValuePage.addByteSize(b, p.byteSize);
  wire.KeyValuePage.addFetchedAt(b, p.fetchedAt);
  return wire.KeyValuePage.endKeyValuePage(b);
}

function encodeStreamPage(b: flatbuffers.Builder, p: StreamPage): flatbuffers.Offset {
  const keysOff = encodeChunk(b, p.keys);
  const headersOff = encodeChunk(b, p.headers);
  const attrsOff = encodeChunk(b, p.attrs);
  const timestampsOff = encodeChunk(b, p.timestamps);
  const bodiesOff = encodeChunk(b, p.bodies);
  const positionOff = encodePagePosition(b, p.position);

  wire.StreamPage.startStreamPage(b);
  wire.StreamPage.addPosition(b, positionOff);
  wire.StreamPage.addKeys(b, keysOff);
  wire.StreamPage.addHeaders(b, headersOff);
  wire.StreamPage.addAttrs(b, attrsOff);
  wire.StreamPage.addTimestamps(b, timestampsOff);
  wire.StreamPage.addBodies(b, bodiesOff);
  wire.StreamPage.addRowCount(b, p.rowCount);
  wire.StreamPage.addByteSize(b, p.byteSize);
  wire.StreamPage.addFetchedAt(b, p.fetchedAt);
  if (p.visibilityTimeoutSeconds !== null) {
    wire.StreamPage.addVisibilityTimeoutSeconds(b, p.visibilityTimeoutSeconds);
  }
  if (p.maxReceiveCount !== null) wire.StreamPage.addMaxReceiveCount(b, p.maxReceiveCount);
  return wire.StreamPage.endStreamPage(b);
}

function encodePage(b: flatbuffers.Builder, p: WirePage): flatbuffers.Offset {
  if (p.kind === 'tabular') {
    return wire.Page.createPage(b, wire.PageBody.TabularPage, encodeTabularPage(b, p));
  }
  if (p.kind === 'document') {
    return wire.Page.createPage(b, wire.PageBody.DocumentPage, encodeDocumentPage(b, p));
  }
  if (p.kind === 'keyvalue') {
    return wire.Page.createPage(b, wire.PageBody.KeyValuePage, encodeKeyValuePage(b, p));
  }
  return wire.Page.createPage(b, wire.PageBody.StreamPage, encodeStreamPage(b, p));
}

function encodePayload(
  b: flatbuffers.Builder,
  payload: FramePayload,
): [flatbuffers.Offset, wire.Payload] {
  switch (payload.type) {
    case 'read': {
      const pageOff = encodePage(b, payload.page);
      return [
        wire.ReadResponse.createReadResponse(b, pageOff, encodeSource(payload.source)),
        wire.Payload.ReadResponse,
      ];
    }
    case 'count':
      return [
        wire.CountResponse.createCountResponse(
          b,
          payload.value,
          payload.exact,
          payload.at,
          payload.stale,
          encodeSource(payload.source),
        ),
        wire.Payload.CountResponse,
      ];
    case 'preview': {
      const offs = payload.statements.map((s) => b.createString(s));
      const vec = wire.PreviewResponse.createStatementsVector(b, offs);
      return [wire.PreviewResponse.createPreviewResponse(b, vec), wire.Payload.PreviewResponse];
    }
    case 'mutate':
      return [
        wire.MutateResponse.createMutateResponse(b, payload.affectedRows),
        wire.Payload.MutateResponse,
      ];
    case 'execute': {
      const offs = payload.pages.map((p) => encodePage(b, p));
      const vec = wire.ExecuteResponse.createPagesVector(b, offs);
      return [wire.ExecuteResponse.createExecuteResponse(b, vec), wire.Payload.ExecuteResponse];
    }
    case 'objectDownload':
      return [
        wire.ObjectDownloadResponse.createObjectDownloadResponse(b, payload.bytes),
        wire.Payload.ObjectDownloadResponse,
      ];
    case 'ping':
      return [
        wire.PingPayload.createPingPayload(b, true, payload.enginePid, payload.at),
        wire.Payload.PingPayload,
      ];
    case 'cacheStats':
      return [
        wire.CacheStats.createCacheStats(
          b,
          payload.l2Bytes,
          payload.l2BudgetBytes,
          payload.l2Entries,
          payload.l2Hits,
          payload.l2Misses,
          payload.l3Entries,
        ),
        wire.Payload.CacheStats,
      ];
    case 'empty': {
      wire.EmptyResponse.startEmptyResponse(b);
      return [wire.EmptyResponse.endEmptyResponse(b), wire.Payload.EmptyResponse];
    }
  }
}

function finish(b: flatbuffers.Builder, frameOff: flatbuffers.Offset): EncodedFrame {
  wire.Frame.finishFrameBuffer(b, frameOff);
  const bytes = b.asUint8Array();
  const bb = new flatbuffers.ByteBuffer(bytes);
  const frame = wire.Frame.getRootAsFrame(bb);
  // Vtable slot 6 is Frame.id (frame.ts's own `id()`); with forceDefaults this slot exists even
  // when id is still the placeholder 0, so this is the byte position mockStreamBrowser.js later
  // patches a real request id into.
  const idOffset = frame.bb_pos + bb.__offset(frame.bb_pos, 6);
  return { bytes, idOffset };
}

export function encodeFrame(spec: FrameSpec): EncodedFrame {
  const b = new flatbuffers.Builder(1024);
  if (spec.forceDefaults) b.forceDefaults(true);

  if (spec.kind === 'evt') {
    const [payloadOff, payloadType] = encodePayload(b, spec.payload);
    const topicOff = b.createString(spec.topic);
    wire.Frame.startFrame(b);
    wire.Frame.addKind(b, wire.FrameKind.evt);
    wire.Frame.addTopic(b, topicOff);
    wire.Frame.addPayloadType(b, payloadType);
    wire.Frame.addPayload(b, payloadOff);
    return finish(b, wire.Frame.endFrame(b));
  }

  if (spec.ok) {
    const [payloadOff, payloadType] = encodePayload(b, spec.payload);
    wire.Frame.startFrame(b);
    wire.Frame.addKind(b, wire.FrameKind.res);
    wire.Frame.addId(b, spec.id);
    wire.Frame.addOk(b, true);
    wire.Frame.addPayloadType(b, payloadType);
    wire.Frame.addPayload(b, payloadOff);
    return finish(b, wire.Frame.endFrame(b));
  }

  const messageOff = b.createString(spec.error.message);
  const codeOff = spec.error.code !== undefined ? b.createString(spec.error.code) : 0;
  wire.Error.startError(b);
  wire.Error.addMessage(b, messageOff);
  if (spec.error.code !== undefined) wire.Error.addCode(b, codeOff);
  const errOff = wire.Error.endError(b);

  wire.Frame.startFrame(b);
  wire.Frame.addKind(b, wire.FrameKind.res);
  wire.Frame.addId(b, spec.id);
  wire.Frame.addOk(b, false);
  wire.Frame.addError(b, errOff);
  return finish(b, wire.Frame.endFrame(b));
}
