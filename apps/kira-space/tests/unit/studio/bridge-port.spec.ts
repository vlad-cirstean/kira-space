import { describe, expect, test } from 'bun:test';
import * as wire from '@shared/protocol/wire';
import type { FakeSocket } from '@workbench/testing/unit/fakeSocket';
import { getStream } from '@workbench/testing/unit/wailsRuntime';
import * as flatbuffers from 'flatbuffers';
import { encodeFrame, type FrameSpec } from '../../support/encodeFrame';

// port.ts calls Stream('engine') once at module scope, so every test in this file
// drives the one resulting connection through the same fake — matching how the real module
// uses the runtime, rather than one fresh socket per test. The dynamic import below is
// what triggers that module-scope Stream('engine') call *if* no earlier spec in this run already
// has (Bun's module registry caches port.ts itself for the whole test process) — either way,
// `getStream('engine')` after the import hands back whichever socket port.ts ended up
// holding, so this file's own assertions never depend on winning a load-order race against
// whatever other spec's import chain also reaches port.ts
// (see @workbench/testing/unit/wailsRuntime).
const { ready, request, onPortEvent } = await import('../../../frontend/src/studio/bridge/port');
const socket: FakeSocket = getStream('engine');

// request() gates its send on `ready` via a .then() registered synchronously but resolved on a
// later microtask; awaiting `ready` itself is not enough to guarantee that .then has *also* run,
// so tests flush an extra microtask tick before reading socket.sent.
async function flush(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

interface SentRequest {
  kind: 'req';
  id: number;
  op: string;
  payload: unknown;
}

// port.ts now always JSON.stringifies before calling send (P11 edit 3) — socket.sent holds that
// string, real Stream or this fake alike (fakeSocket.ts's own doc comment on `sent`).
function lastSent(): SentRequest {
  return JSON.parse(socket.sent.at(-1) as string) as SentRequest;
}

// FlatBuffers frames aren't JSON — every response/event this file feeds to socket.__message must
// be a real encoded ArrayBuffer, built through the same generated wire code port.ts's decodeFrame
// reads (encodeFrame is the test-support mirror of Go's own encode.go/frame.go, P11 step 15).
function frameBuffer(spec: FrameSpec): ArrayBuffer {
  const { bytes } = encodeFrame(spec);
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

// Flips the "KIF1" file identifier (bytes 4-8 of a finished buffer) so wire.Frame.bufferHasIdentifier
// fails — decodeFrame's one no-fallback throw (P11 D1/D5).
function corruptIdentifier(bytes: Uint8Array): ArrayBuffer {
  const copy = bytes.slice();
  copy.set([0, 0, 0, 0], 4);
  return copy.buffer as ArrayBuffer;
}

// A res frame whose envelope (kind/id/ok) decodes fine but whose own payload
// table fails to — hand-built, not through encodeFrame's own FramePayload union (which is typed
// precisely to prevent this mismatch): payloadType is tagged ReadResponse but no payload table is
// ever written, so decodeFrame's own `frame.payload(...)` returns null and decodePayload throws
// "frame: ReadResponse payload is missing" — while `frame.id()` is already known by then.
function resFrameWithUndecodablePayload(id: number): ArrayBuffer {
  const b = new flatbuffers.Builder(256);
  wire.Frame.startFrame(b);
  wire.Frame.addKind(b, wire.FrameKind.res);
  wire.Frame.addId(b, id);
  wire.Frame.addOk(b, true);
  wire.Frame.addPayloadType(b, wire.Payload.ReadResponse);
  const frameOff = wire.Frame.endFrame(b);
  wire.Frame.finishFrameBuffer(b, frameOff);
  const bytes = b.asUint8Array();
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

describe('apps/kira-space/frontend/src/studio/bridge/port.ts — the Stream transport (P57 D2/D3, P11)', () => {
  test('1. a request issued before the stream opens is not sent until it does', async () => {
    const p = request('ping');
    expect(socket.sent).toHaveLength(0); // CONNECTING: send() would throw on the real socket
    socket.__open();
    await ready;
    await flush();
    expect(socket.sent).toHaveLength(1);
    const sent = lastSent();
    expect(sent.op).toBe('ping');
    socket.__message(
      frameBuffer({
        kind: 'res',
        id: sent.id,
        ok: true,
        payload: { type: 'ping', enginePid: 4321, at: 999 },
      }),
    );
    await expect(p).resolves.toEqual({ pong: true, enginePid: 4321, at: 999 });
  });

  test('2. two concurrent requests resolve independently by id', async () => {
    const p1 = request('a');
    const p2 = request('b');
    await flush();
    const [reqA, reqB] = socket.sent.slice(-2).map((v) => JSON.parse(v as string) as SentRequest);
    // Answered out of order — resolution must key off `id`, not arrival order.
    socket.__message(
      frameBuffer({
        kind: 'res',
        id: reqB.id,
        ok: true,
        payload: { type: 'mutate', affectedRows: 222 },
      }),
    );
    socket.__message(
      frameBuffer({
        kind: 'res',
        id: reqA.id,
        ok: true,
        payload: { type: 'mutate', affectedRows: 111 },
      }),
    );
    await expect(p1).resolves.toEqual({ affectedRows: 111 });
    await expect(p2).resolves.toEqual({ affectedRows: 222 });
  });

  test('3. an error frame rejects with its code', async () => {
    const p = request('boom');
    await flush();
    const sent = lastSent();
    socket.__message(
      frameBuffer({
        kind: 'res',
        id: sent.id,
        ok: false,
        error: { message: 'PG is not connected', code: 'E_DISCONNECTED' },
      }),
    );
    await expect(p).rejects.toMatchObject({
      message: 'PG is not connected',
      code: 'E_DISCONNECTED',
    });
  });

  test('4. a request times out per its own timeoutMs, and timeoutMs: null never does', async () => {
    const p = request('slow', null, { timeoutMs: 10 });
    await expect(p).rejects.toThrow('engine request "slow" timed out');

    const never = request('no-timeout', null, { timeoutMs: null });
    const sentinel = Symbol('still pending');
    const race = await Promise.race([never, new Promise((r) => setTimeout(() => r(sentinel), 50))]);
    expect(race).toBe(sentinel);
    // Answer it so the test doesn't leave a dangling pending entry for later tests.
    await flush();
    const sent = lastSent();
    socket.__message(
      frameBuffer({ kind: 'res', id: sent.id, ok: true, payload: { type: 'empty' } }),
    );
    await never;
  });

  test('5. events fan out to every subscriber on a topic, and unsubscribe works', () => {
    const seenA: unknown[] = [];
    const seenB: unknown[] = [];
    const offA = onPortEvent('connection:state', (p) => seenA.push(p));
    const offB = onPortEvent('connection:state', (p) => seenB.push(p));
    socket.__message(
      frameBuffer({
        kind: 'evt',
        topic: 'connection:state',
        payload: { type: 'mutate', affectedRows: 1 },
      }),
    );
    expect(seenA).toEqual([{ affectedRows: 1 }]);
    expect(seenB).toEqual([{ affectedRows: 1 }]);
    offA();
    socket.__message(
      frameBuffer({
        kind: 'evt',
        topic: 'connection:state',
        payload: { type: 'mutate', affectedRows: 2 },
      }),
    );
    expect(seenA).toEqual([{ affectedRows: 1 }]);
    expect(seenB).toEqual([{ affectedRows: 1 }, { affectedRows: 2 }]);
    offB();
  });

  // A chunk buffer can no longer be malformed independently of the rest of the frame the way
  // base64 JSON allowed — the one throw decodeFrame has left with no fallback is a mismatched
  // "KIF1" file identifier, which can't be tied back to a specific pending request (the id
  // itself lives inside the very envelope that failed to parse). port.ts's onmessage wraps
  // `decodeFrame` in a try/catch so a synchronous throw during DOM event dispatch can't go
  // uncaught — the corrupt frame is dropped instead, and a later, valid frame for the same id
  // still settles the promise normally.
  test('8. a response frame with a corrupt file identifier is dropped, not left hanging forever', async () => {
    const p = request('read', null, { timeoutMs: null });
    await flush();
    const sent = lastSent();
    const good = encodeFrame({
      kind: 'res',
      id: sent.id,
      ok: true,
      payload: { type: 'mutate', affectedRows: 1 },
    });
    expect(() => socket.__message(corruptIdentifier(good.bytes))).not.toThrow();
    socket.__message(
      frameBuffer({
        kind: 'res',
        id: sent.id,
        ok: true,
        payload: { type: 'mutate', affectedRows: 7 },
      }),
    );
    await expect(p).resolves.toEqual({ affectedRows: 7 });
  });

  // The 'evt' branch has the same hazard, but there is no pending promise to reject for an event —
  // the only correct move is to drop the one corrupt event rather than let the throw propagate
  // (which would abort the fan-out to every *other* subscriber on the topic too).
  test('9. an event frame with a corrupt file identifier drops just that event', () => {
    const seen: unknown[] = [];
    const off = onPortEvent('bad-event', (p) => seen.push(p));
    const good = encodeFrame({
      kind: 'evt',
      topic: 'bad-event',
      payload: { type: 'mutate', affectedRows: 1 },
    });
    expect(() => socket.__message(corruptIdentifier(good.bytes))).not.toThrow();
    expect(seen).toEqual([]);
    socket.__message(
      frameBuffer({
        kind: 'evt',
        topic: 'bad-event',
        payload: { type: 'mutate', affectedRows: 2 },
      }),
    );
    expect(seen).toEqual([{ affectedRows: 2 }]);
    off();
  });

  // A data op carries no client-side timeout of its own (test 4's own `timeoutMs: null`) —
  // cancellation is the only escape hatch. Before this fix, decodeFrame threw on a payload-decode
  // failure the same as it does on a structurally corrupt frame (test 8), and port.ts's onmessage
  // drops any frame it can't decode, so this pending call would otherwise hang forever with
  // nothing left to reject it. The id is already known by the time the payload decode fails, so
  // decodeFrame itself now settles this as an ordinary {ok:false} response.
  test('10. a response frame whose own payload fails to decode rejects its pending call, not left hanging', async () => {
    const p = request('read', null, { timeoutMs: null });
    await flush();
    const sent = lastSent();
    socket.__message(resFrameWithUndecodablePayload(sent.id));
    await expect(p).rejects.toThrow(
      'frame: payload decode failed: frame: ReadResponse payload is missing',
    );
  });

  // Terminal: closing the stream is not reversible in this module, so this runs last.
  test('11. close rejects every pending request, and later requests reject immediately', async () => {
    const pending = request('during-close');
    await flush();
    socket.__close();
    await expect(pending).rejects.toThrow('engine stream closed before this request answered');
    await expect(request('after-close')).rejects.toThrow('engine stream is closed');
  });
});
