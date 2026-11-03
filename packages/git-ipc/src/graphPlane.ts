/**
 * The graph data plane client: many credit-gated `graph.stream` streams over one `StreamChannel`
 * (the Go side is `internal/graphplane`). JSON text frames out (`open`, `credit`, `cancel`),
 * `chunk`/`end` frames in; a chunk frame carries its FlatBuffers blob out of band (`blobFrame.ts`).
 * Every other git call is a bound `GitService` method, not this channel.
 */
import { decodeStreamPayload } from './codec.ts';
import type { StreamChunkOf, StreamParamsOf } from './contract.ts';
import type { StreamChannel } from './streamChannel.ts';
import { RpcError, TransportError, type WireError } from './transport.ts';

/** Streams open with this much credit granted: one chunk can move while the previous one is
 *  processed, but a slow consumer never lets a 100k walk queue unbounded buffers. */
const INITIAL_STREAM_CREDIT = 2;

type InFrame =
  | { readonly t: 'chunk'; readonly id: number; readonly chunk: unknown }
  | { readonly t: 'end'; readonly id: number; readonly error?: WireError };

interface PendingStream {
  readonly onChunk: (chunk: StreamChunkOf<'graph.stream'>) => void | Promise<void>;
  readonly resolve: () => void;
  readonly reject: (error: unknown) => void;
  /** Chunks run strictly in order even when frames arrive while an `onChunk` is still awaited. */
  queue: Promise<void>;
  done: boolean;
}

export interface GraphPlane {
  /** Streams `graph.stream` rows of `windowKey`'s connection. Resolves when the stream ends, is
   *  aborted by `signal`, or the channel closes; rejects on a server error or a throwing
   *  `onChunk`. */
  stream(
    windowKey: string,
    params: StreamParamsOf<'graph.stream'>,
    onChunk: (chunk: StreamChunkOf<'graph.stream'>) => void | Promise<void>,
    signal?: AbortSignal,
  ): Promise<void>;
  dispose(): void;
}

export function createGraphPlane(channel: StreamChannel): GraphPlane {
  let nextId = 1;
  const pending = new Map<number, PendingStream>();

  function finish(id: number): void {
    const entry = pending.get(id);
    if (!entry) return;
    entry.done = true;
    pending.delete(id);
  }

  function settleAll(): void {
    for (const entry of pending.values()) {
      if (!entry.done) {
        entry.done = true;
        entry.resolve();
      }
    }
    pending.clear();
  }

  function handleFrame(frame: InFrame): void {
    const entry = pending.get(frame.id);
    if (!entry || entry.done) return;
    if (frame.t === 'chunk') {
      // The queued step never rejects: a rejected queue would skip every later `.then`, `end`
      // included, and wedge the stream.
      entry.queue = entry.queue.then(async () => {
        if (entry.done) return;
        try {
          const chunk = decodeStreamPayload('graph.stream', frame.chunk);
          await entry.onChunk(chunk as StreamChunkOf<'graph.stream'>);
          if (entry.done) return;
          channel.post({ t: 'credit', id: frame.id, n: 1 });
        } catch (error) {
          if (entry.done) return;
          finish(frame.id);
          channel.post({ t: 'cancel', id: frame.id });
          entry.reject(error);
        }
      });
      return;
    }
    entry.queue = entry.queue.then(() => {
      if (entry.done) return;
      finish(frame.id);
      if (frame.error) entry.reject(new RpcError(frame.error));
      else entry.resolve();
    });
  }

  const unsubscribe = channel.onMessage((raw) => handleFrame(raw as InFrame));
  const unsubscribeClose = channel.onClose(settleAll);

  return {
    stream(windowKey, params, onChunk, signal) {
      if (signal?.aborted) {
        return Promise.reject(
          new TransportError('cancelled', "stream 'graph.stream' was already cancelled"),
        );
      }
      const id = nextId++;
      return new Promise<void>((resolve, reject) => {
        const entry: PendingStream = {
          onChunk,
          resolve,
          reject,
          queue: Promise.resolve(),
          done: false,
        };
        pending.set(id, entry);
        signal?.addEventListener(
          'abort',
          () => {
            if (entry.done) return;
            finish(id);
            channel.post({ t: 'cancel', id });
            resolve();
          },
          { once: true },
        );
        channel.post({
          t: 'open',
          id,
          windowKey,
          params,
          credit: INITIAL_STREAM_CREDIT,
        });
      });
    },

    dispose(): void {
      unsubscribe();
      unsubscribeClose();
      settleAll();
      channel.close();
    },
  };
}
