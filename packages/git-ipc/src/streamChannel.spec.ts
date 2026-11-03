import { expect, test } from 'bun:test';
import * as path from 'node:path';
import { decodeStreamPayload } from './codec.ts';
import type { DecorationRef, StreamChunkOf } from './contract.ts';
import {
  createStreamChannel,
  MalformedBlobFrameError,
  type StreamSocketLike,
} from './streamChannel.ts';

// A minimal WebSocket-like double: this file's own contract is "MessageChannelLike over a
// WebSocket-like object" (§3.4), not any particular runtime's WebSocket — a plain in-memory stand-
// in is enough to drive onmessage/onclose the same way a real WailsSocket would.
class MockSocket implements StreamSocketLike {
  binaryType = 'arraybuffer';
  // 0 = CONNECTING, 1 = OPEN — matches WebSocket/WailsSocket's own readyState values (P67b §3.2).
  readyState = 0;
  onopen: (() => void) | null = null;
  // biome-ignore lint/suspicious/noExplicitAny: matches StreamSocketLike's own cross-project escape hatch.
  onmessage: ((ev: any) => void) | null = null;
  // biome-ignore lint/suspicious/noExplicitAny: matches StreamSocketLike's own cross-project escape hatch.
  onclose: ((ev: any) => void) | null = null;
  // biome-ignore lint/suspicious/noExplicitAny: matches StreamSocketLike's own cross-project escape hatch.
  onerror: ((ev: any) => void) | null = null;
  readonly sent: unknown[] = [];
  closed = false;

  send(data: string | ArrayBuffer | ArrayBufferView<ArrayBuffer>): void {
    this.sent.push(data);
  }

  // Deliberately does not fire onclose itself — a real socket's close() dispatches its close
  // event asynchronously, never synchronously from inside the caller that requested it, and a
  // test relying on the opposite would mask the malformed-frame path's own explicit fireClose.
  close(): void {
    this.closed = true;
  }

  /** Test-only: deliver one message as this socket's peer would. */
  deliver(data: ArrayBuffer): void {
    this.onmessage?.({ data });
  }

  /** Test-only: transition CONNECTING -> OPEN and fire onopen, as a real socket's open event would. */
  open(): void {
    this.readyState = 1;
    this.onopen?.();
  }
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

function jsonFrame(value: unknown): ArrayBuffer {
  return toArrayBuffer(new TextEncoder().encode(JSON.stringify(value)));
}

// buildBlobFrame constructs the same wire body streamChannel.ts parses —
// 0x00 | uint32BE headerLen | headerJSON | blob — with no outer length prefix, since a Wails
// stream is message-framed already (§3.4).
function buildBlobFrame(header: unknown, blob: Uint8Array): ArrayBuffer {
  const headerBytes = new TextEncoder().encode(JSON.stringify(header));
  const body = new Uint8Array(1 + 4 + headerBytes.byteLength + blob.byteLength);
  body[0] = 0x00;
  new DataView(body.buffer).setUint32(1, headerBytes.byteLength, false);
  body.set(headerBytes, 5);
  body.set(blob, 5 + headerBytes.byteLength);
  return toArrayBuffer(body);
}

test('decodes a plain JSON frame', () => {
  const socket = new MockSocket();
  const channel = createStreamChannel(socket);
  const received: unknown[] = [];
  channel.onMessage((msg) => received.push(msg));

  socket.deliver(jsonFrame({ hello: 'world' }));

  expect(received).toEqual([{ hello: 'world' }]);
});

test('substitutes a $blob marker nested inside a payload', () => {
  const socket = new MockSocket();
  const channel = createStreamChannel(socket);
  const received: unknown[] = [];
  channel.onMessage((msg) => received.push(msg));

  const blob = new TextEncoder().encode('the-blob-bytes');
  socket.deliver(
    buildBlobFrame({ t: 'chunk', envelope: { seq: 1 }, payload: { chunk: { $blob: true } } }, blob),
  );

  const msg = received[0] as { payload: { chunk: ArrayBuffer } };
  expect(new Uint8Array(msg.payload.chunk)).toEqual(blob);
});

test('closes and reports MalformedBlobFrameError when the header length exceeds the frame', () => {
  const socket = new MockSocket();
  const channel = createStreamChannel(socket);
  channel.onMessage(() => undefined);
  let closeErr: Error | undefined;
  channel.onClose((err) => {
    closeErr = err;
  });

  // discriminant + a header length far larger than any body actually present, no header/blob bytes.
  const body = new Uint8Array([0x00, 0xff, 0xff, 0xff, 0xff]);
  socket.deliver(toArrayBuffer(body));

  expect(closeErr).toBeInstanceOf(MalformedBlobFrameError);
  expect(socket.closed).toBe(true);
});

test('closes and reports MalformedBlobFrameError when the header carries no $blob marker', () => {
  const socket = new MockSocket();
  const channel = createStreamChannel(socket);
  channel.onMessage(() => undefined);
  let closeErr: Error | undefined;
  channel.onClose((err) => {
    closeErr = err;
  });

  socket.deliver(buildBlobFrame({ hello: 'world' }, new TextEncoder().encode('x')));

  expect(closeErr).toBeInstanceOf(MalformedBlobFrameError);
  expect(socket.closed).toBe(true);
});

test('closes and reports MalformedBlobFrameError when the header carries two $blob markers', () => {
  const socket = new MockSocket();
  const channel = createStreamChannel(socket);
  channel.onMessage(() => undefined);
  let closeErr: Error | undefined;
  channel.onClose((err) => {
    closeErr = err;
  });

  socket.deliver(
    buildBlobFrame({ a: { $blob: true }, b: { $blob: true } }, new TextEncoder().encode('x')),
  );

  expect(closeErr).toBeInstanceOf(MalformedBlobFrameError);
  expect(socket.closed).toBe(true);
});

test('post JSON-stringifies and sends, never encoding a blob', () => {
  const socket = new MockSocket();
  const channel = createStreamChannel(socket);
  socket.open();

  channel.post({ id: 1, method: 'graph.status' });

  expect(socket.sent).toEqual([JSON.stringify({ id: 1, method: 'graph.status' })]);
});

test('post before onopen queues, then flushes in order once onopen fires', () => {
  const socket = new MockSocket();
  const channel = createStreamChannel(socket);

  channel.post({ id: 1, method: 'app.init' });
  channel.post({ id: 2, method: 'graph.status' });
  expect(socket.sent).toEqual([]);

  socket.open();

  expect(socket.sent).toEqual([
    JSON.stringify({ id: 1, method: 'app.init' }),
    JSON.stringify({ id: 2, method: 'graph.status' }),
  ]);
});

test('post after onopen sends immediately', () => {
  const socket = new MockSocket();
  const channel = createStreamChannel(socket);
  socket.open();

  channel.post({ id: 1, method: 'graph.status' });

  expect(socket.sent).toEqual([JSON.stringify({ id: 1, method: 'graph.status' })]);
});

test('a socket already OPEN when wrapped sends immediately, with no onopen needed', () => {
  const socket = new MockSocket();
  socket.readyState = 1; // already open before createStreamChannel ever sees it
  const channel = createStreamChannel(socket);

  channel.post({ id: 1, method: 'graph.status' });

  expect(socket.sent).toEqual([JSON.stringify({ id: 1, method: 'graph.status' })]);
});

test('post after onclose sends nothing, does not throw, and drops the queue', () => {
  const socket = new MockSocket();
  const channel = createStreamChannel(socket);
  channel.onMessage(() => undefined);

  channel.post({ id: 1, method: 'queued.before.close' }); // still CONNECTING: queued, not sent
  socket.onclose?.(undefined);

  expect(() => channel.post({ id: 2, method: 'post.after.close' })).not.toThrow();
  expect(socket.sent).toEqual([]);

  socket.open();
  expect(socket.sent).toEqual([]); // the queue was dropped on close, not flushed by a late onopen
});

test('sets binaryType to arraybuffer so inbound frames are never delivered as a Blob', () => {
  const socket = new MockSocket();
  socket.binaryType = 'blob';
  createStreamChannel(socket);
  expect(socket.binaryType).toBe('arraybuffer');
});

// D16: the golden graph.stream chunk frame, captured from the native stream by
// flows/gitflow TestFixtures_CaptureGraphChunkFrame (KIRA_CONTRACT=write). Catches the Go
// encoder and the decode path disagreeing: a field in the wrong slot, a big-endian column.
interface GraphChunkFixture {
  readonly envelope: {
    readonly repoId: string;
    readonly seq: number;
    readonly from: number;
    readonly to: number;
    readonly source: 'git' | 'cache';
    readonly remaining: number;
    readonly exhausted: boolean;
  };
  readonly commits: {
    readonly from: number;
    readonly to: number;
    readonly shaWidthBytes: number;
    readonly shas: readonly string[];
    readonly parentOffsets: readonly number[];
    readonly parentShas: readonly string[];
    readonly identityIds: readonly number[];
    readonly times: readonly number[];
    readonly subjects: readonly string[];
    readonly subjectOffsets: readonly number[];
    readonly dictionaryBase: number;
    readonly dictionary: readonly string[];
    readonly decorations: ReadonlyArray<{
      readonly row: number;
      readonly refs: ReadonlyArray<{
        readonly kind: string;
        readonly name?: string;
        readonly isHead?: boolean;
      }>;
    }>;
  };
}

function expectedDecorationRef(r: {
  kind: string;
  name?: string;
  isHead?: boolean;
}): DecorationRef {
  switch (r.kind) {
    case 'branch':
      if (r.name === undefined) throw new Error('branch decoration fixture is missing a name');
      return { kind: 'branch', name: r.name, isHead: r.isHead ?? false };
    case 'remoteBranch':
      if (r.name === undefined) {
        throw new Error('remoteBranch decoration fixture is missing a name');
      }
      return { kind: 'remoteBranch', name: r.name };
    case 'tag':
      if (r.name === undefined) throw new Error('tag decoration fixture is missing a name');
      return { kind: 'tag', name: r.name };
    case 'head':
      return { kind: 'head' };
    case 'stash':
      if (r.name === undefined) throw new Error('stash decoration fixture is missing an index');
      return { kind: 'stash', index: Number(r.name) };
    default:
      throw new Error(`streamChannel.spec.ts: unrecognised decoration kind ${r.kind}`);
  }
}

test('D16: decodes the captured Go-encoded graph.stream chunk frame field for field', async () => {
  const fixtureDir = path.join(import.meta.dir, '..', 'testdata');
  const raw = new Uint8Array(
    await Bun.file(path.join(fixtureDir, 'graphChunkFrame.bin')).arrayBuffer(),
  );
  const expected = JSON.parse(
    await Bun.file(path.join(fixtureDir, 'graphChunkFrame.json')).text(),
  ) as GraphChunkFixture;

  const socket = new MockSocket();
  const channel = createStreamChannel(socket);
  const received = new Promise<unknown>((resolve) => channel.onMessage(resolve));
  // The Wails stream carries whole frames: the captured body goes in with no length prefix.
  socket.deliver(toArrayBuffer(raw));

  const frameBody = (await received) as {
    readonly t: string;
    readonly id: number;
    readonly chunk: unknown;
  };
  expect(frameBody.t).toBe('chunk');

  const chunk = decodeStreamPayload(
    'graph.stream',
    frameBody.chunk,
  ) as StreamChunkOf<'graph.stream'>;

  expect(chunk.repoId).toBe(expected.envelope.repoId);
  expect(chunk.seq).toBe(expected.envelope.seq);
  expect(chunk.from).toBe(expected.envelope.from);
  expect(chunk.to).toBe(expected.envelope.to);
  expect(chunk.source).toBe(expected.envelope.source);
  expect(chunk.remaining).toBe(expected.envelope.remaining);
  expect(chunk.exhausted).toBe(expected.envelope.exhausted);

  const commits = chunk.commits;
  expect(commits.from).toBe(expected.commits.from);
  expect(commits.to).toBe(expected.commits.to);
  expect(commits.shaWidthBytes).toBe(expected.commits.shaWidthBytes);

  const shaWidth = commits.shaWidthBytes;
  const shasBytes = new Uint8Array(commits.shas);
  const gotShas: string[] = [];
  for (let i = 0; i < shasBytes.length; i += shaWidth) {
    gotShas.push(Buffer.from(shasBytes.slice(i, i + shaWidth)).toString('hex'));
  }
  expect(gotShas).toEqual([...expected.commits.shas]);

  expect(Array.from(new Uint32Array(commits.parentOffsets))).toEqual([
    ...expected.commits.parentOffsets,
  ]);
  const parentShasBytes = new Uint8Array(commits.parentShas);
  const gotParentShas: string[] = [];
  for (let i = 0; i < parentShasBytes.length; i += shaWidth) {
    gotParentShas.push(Buffer.from(parentShasBytes.slice(i, i + shaWidth)).toString('hex'));
  }
  expect(gotParentShas).toEqual([...expected.commits.parentShas]);

  expect(Array.from(new Uint32Array(commits.identityIds))).toEqual([
    ...expected.commits.identityIds,
  ]);
  expect(Array.from(new Uint32Array(commits.times))).toEqual([...expected.commits.times]);
  expect(Array.from(new Uint32Array(commits.subjectOffsets))).toEqual([
    ...expected.commits.subjectOffsets,
  ]);

  const subjectBytes = new Uint8Array(commits.subjectBytes);
  const subjectOffsets = Array.from(new Uint32Array(commits.subjectOffsets));
  const gotSubjects: string[] = [];
  for (let i = 0; i < subjectOffsets.length - 1; i++) {
    gotSubjects.push(
      Buffer.from(subjectBytes.slice(subjectOffsets[i], subjectOffsets[i + 1])).toString('utf8'),
    );
  }
  expect(gotSubjects).toEqual([...expected.commits.subjects]);

  expect(commits.dictionaryBase).toBe(expected.commits.dictionaryBase);
  expect([...commits.dictionary]).toEqual([...expected.commits.dictionary]);

  const gotDecorations = commits.decorations.map(([row, refs]) => ({ row, refs: [...refs] }));
  const wantDecorations = expected.commits.decorations.map((d) => ({
    row: d.row,
    refs: d.refs.map(expectedDecorationRef),
  }));
  expect(gotDecorations).toEqual(wantDecorations);
});
