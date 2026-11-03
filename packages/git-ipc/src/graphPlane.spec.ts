import { describe, expect, test } from 'bun:test';
import { deferred, sleep } from '@workbench/testing/unit/async';
import { encodeStreamPayload } from './codec.ts';
import type { PackedCommitChunk, StreamChunkOf } from './contract.ts';
import { createGraphPlane } from './graphPlane.ts';
import type { StreamChannel } from './streamChannel.ts';
import { RpcError, TransportError } from './transport.ts';

async function tick(times = 1): Promise<void> {
  for (let i = 0; i < times; i++) await sleep(0);
}

interface Pair {
  readonly client: StreamChannel;
  readonly server: StreamChannel;
  /** Closes both ends as a dropped socket does. */
  drop(): void;
}

/** In-memory pipe: posting on one end invokes the other end's handler after a structuredClone. */
function createChannelPair(): Pair {
  const handlers: Array<((message: unknown) => void) | undefined> = [undefined, undefined];
  const closers = new Set<() => void>();
  const end = (self: 0 | 1): StreamChannel => ({
    post(message) {
      handlers[1 - self]?.(structuredClone(message));
    },
    onMessage(handler) {
      handlers[self] = handler;
      return () => {
        if (handlers[self] === handler) handlers[self] = undefined;
      };
    },
    close() {},
    onClose(handler) {
      if (self === 0) closers.add(handler);
      return () => closers.delete(handler);
    },
  });
  return {
    client: end(0),
    server: end(1),
    drop: () => {
      for (const handler of [...closers]) handler();
    },
  };
}

type Emit = (chunk: StreamChunkOf<'graph.stream'>) => Promise<void>;

interface Peer {
  readonly opens: Array<Record<string, unknown>>;
  readonly credits: number[];
  readonly cancels: number[];
  end(id: number, error?: { code: string; message: string }): void;
}

/** A test-local plane server: credit-gated chunks per open, as the Go plane sends them. */
function createPeer(
  channel: StreamChannel,
  stream: (ctx: { id: number; signal: AbortSignal; emit: Emit }) => Promise<void>,
): Peer {
  const opens: Array<Record<string, unknown>> = [];
  const credits: number[] = [];
  const cancels: number[] = [];
  const gates = new Map<number, { n: number; wake?: () => void }>();
  const aborts = new Map<number, AbortController>();
  const end = (id: number, error?: { code: string; message: string }) =>
    channel.post({ t: 'end', id, ...(error ? { error } : {}) });

  channel.onMessage((raw) => {
    const frame = raw as { t: string; id: number; n?: number } & Record<string, unknown>;
    const id = frame.id;
    if (frame.t === 'open') {
      opens.push(frame);
      const controller = new AbortController();
      const gate: { n: number; wake?: () => void } = { n: Number(frame.credit ?? 0) };
      aborts.set(id, controller);
      gates.set(id, gate);
      const emit: Emit = async (chunk) => {
        while (gate.n === 0 && !controller.signal.aborted) {
          await new Promise<void>((resolve) => {
            gate.wake = resolve;
          });
        }
        if (controller.signal.aborted) return;
        gate.n--;
        channel.post({ t: 'chunk', id, chunk: encodeStreamPayload('graph.stream', chunk) });
      };
      void stream({ id, signal: controller.signal, emit }).then(() => {
        if (!controller.signal.aborted) end(id);
      });
    } else if (frame.t === 'credit') {
      credits.push(id);
      const gate = gates.get(id);
      if (gate) {
        gate.n += frame.n ?? 0;
        gate.wake?.();
      }
    } else if (frame.t === 'cancel') {
      cancels.push(id);
      aborts.get(id)?.abort();
      gates.get(id)?.wake?.();
    }
  });
  return { opens, credits, cancels, end };
}

function emptyPackedChunk(): PackedCommitChunk {
  return {
    from: 0,
    to: 0,
    shaWidthBytes: 20,
    shas: new ArrayBuffer(0),
    parentOffsets: new ArrayBuffer(4),
    parentShas: new ArrayBuffer(0),
    identityIds: new ArrayBuffer(0),
    times: new ArrayBuffer(0),
    subjectBytes: new ArrayBuffer(0),
    subjectOffsets: new ArrayBuffer(4),
    dictionaryBase: 0,
    dictionary: [],
    decorations: [],
  };
}

function chunkFor(seq: number): StreamChunkOf<'graph.stream'> {
  return {
    repoId: 'r1',
    seq,
    from: seq,
    to: seq + 1,
    source: 'git',
    remaining: 0,
    exhausted: false,
    commits: emptyPackedChunk(),
  };
}

describe('graph plane client', () => {
  test('open names the window and grants the initial credit', async () => {
    const pair = createChannelPair();
    const peer = createPeer(pair.server, async () => {});
    const plane = createGraphPlane(pair.client);

    await plane.stream('w1', { repoId: 'r1', resumeThroughRow: 5 }, () => {});

    expect(peer.opens).toEqual([
      {
        t: 'open',
        id: 1,
        windowKey: 'w1',
        params: { repoId: 'r1', resumeThroughRow: 5 },
        credit: 2,
      },
    ]);
    plane.dispose();
  });

  test('chunks arrive in order, one credit follows each, and the peer never outruns the grant', async () => {
    const pair = createChannelPair();
    let emitted = 0;
    const peer = createPeer(pair.server, async ({ emit }) => {
      for (let i = 0; i < 10; i++) {
        await emit(chunkFor(i));
        emitted++;
      }
    });
    const plane = createGraphPlane(pair.client);

    const received: number[] = [];
    const freeze = deferred<void>();
    let frozen = false;
    const done = plane.stream('w1', { repoId: 'r1' }, async (chunk) => {
      received.push(chunk.seq);
      if (!frozen) {
        frozen = true;
        await freeze.promise;
      }
    });

    await tick(3);
    expect(emitted).toBe(2);
    expect(received).toEqual([0]);
    expect(peer.credits).toEqual([]);

    freeze.resolve();
    await done;
    expect(received).toEqual([0, 1, 2, 3, 4, 5, 6, 7, 8, 9]);
    expect(peer.credits.length).toBe(10);
    plane.dispose();
  });

  test('abort mid-stream sends cancel and resolves', async () => {
    const pair = createChannelPair();
    const peer = createPeer(pair.server, async ({ signal, emit }) => {
      for (let i = 0; i < 5 && !signal.aborted; i++) await emit(chunkFor(i));
    });
    const plane = createGraphPlane(pair.client);
    const controller = new AbortController();
    const received: number[] = [];

    await plane.stream(
      'w1',
      { repoId: 'r1' },
      (chunk) => {
        received.push(chunk.seq);
        controller.abort();
      },
      controller.signal,
    );
    await tick(2);

    expect(received).toEqual([0]);
    expect(peer.cancels).toEqual([1]);
    plane.dispose();
  });

  test('a signal aborted beforehand rejects as cancelled without opening', async () => {
    const pair = createChannelPair();
    const peer = createPeer(pair.server, async () => {});
    const plane = createGraphPlane(pair.client);

    const rejected = plane.stream('w1', { repoId: 'r1' }, () => {}, AbortSignal.abort());

    await expect(rejected).rejects.toBeInstanceOf(TransportError);
    await expect(rejected).rejects.toHaveProperty('code', 'cancelled');
    expect(peer.opens).toEqual([]);
    plane.dispose();
  });

  test('a throwing onChunk cancels the stream and rejects with the thrown error', async () => {
    const pair = createChannelPair();
    const peer = createPeer(pair.server, async ({ signal, emit }) => {
      for (let i = 0; i < 5 && !signal.aborted; i++) await emit(chunkFor(i));
    });
    const plane = createGraphPlane(pair.client);
    const boom = new Error('bad chunk');

    await expect(
      plane.stream('w1', { repoId: 'r1' }, () => {
        throw boom;
      }),
    ).rejects.toBe(boom);
    expect(peer.cancels).toEqual([1]);
    plane.dispose();
  });

  test('an end frame with an error rejects with an RpcError', async () => {
    const pair = createChannelPair();
    const peer = createPeer(pair.server, () => new Promise<void>(() => {}));
    const plane = createGraphPlane(pair.client);

    const done = plane.stream('w1', { repoId: 'r1' }, () => {});
    peer.end(1, { code: 'E_CONFLICT', message: 'not open' });

    await expect(done).rejects.toBeInstanceOf(RpcError);
    await expect(done).rejects.toHaveProperty('code', 'E_CONFLICT');
    plane.dispose();
  });

  test('a closing channel resolves every pending stream', async () => {
    const pair = createChannelPair();
    createPeer(pair.server, async ({ emit }) => {
      await emit(chunkFor(0));
      await new Promise<void>(() => {});
    });
    const plane = createGraphPlane(pair.client);

    const first = plane.stream('w1', { repoId: 'r1' }, () => {});
    const second = plane.stream('w1', { repoId: 'r2' }, () => {});
    await tick(2);
    pair.drop();

    await first;
    await second;
    plane.dispose();
  });

  test('concurrent streams use distinct ids and finish independently', async () => {
    const pair = createChannelPair();
    const peer = createPeer(pair.server, async ({ emit }) => {
      for (let i = 0; i < 3; i++) await emit(chunkFor(i));
    });
    const plane = createGraphPlane(pair.client);
    const first: number[] = [];
    const second: number[] = [];

    await Promise.all([
      plane.stream('w1', { repoId: 'r1' }, (c) => void first.push(c.seq)),
      plane.stream('w1', { repoId: 'r2' }, (c) => void second.push(c.seq)),
    ]);

    expect(first).toEqual([0, 1, 2]);
    expect(second).toEqual([0, 1, 2]);
    expect(peer.opens.map((o) => o.id)).toEqual([1, 2]);
    plane.dispose();
  });
});
