/**
 * The native channel — a message channel over a Wails stream.
 * `internal/bridge/gitgraph.go` hands `*application.StreamConn` straight to `graphplane.Serve`
 * with no length prefix at all (a Wails stream is message-framed already), so every inbound
 * message here is one whole frame body. A Wails stream carries real `ArrayBuffer`s.
 *
 * The frame *body* shape is the one `blobFrame.ts` already owns: a `0x00` first byte is a
 * blob frame (`0x00 | uint32BE headerLen | headerJSON | blob…`), anything else is a plain JSON
 * frame. One implementation of that layout.
 *
 * `post` never encodes a blob — the client here never sends one, only `internal/graphplane` does.
 */
import {
  BLOB_FRAME_DISCRIMINANT,
  MalformedBlobFrameError,
  parseBlobFrameBody,
} from './blobFrame.ts';

// Re-exported so a caller can `instanceof`-check a close reason without importing blobFrame.ts
// directly.
export { MalformedBlobFrameError };

/**
 * The useful subset of `WebSocket`/Wails' own `WailsSocket` this file needs. Declared locally
 * rather than referencing the DOM lib's `WebSocket`/`MessageEvent`/`CloseEvent` types: this
 * package's `tsconfig.json` has no `"dom"` lib (it also runs under Bun/Node), so those names do
 * not exist here. The event-parameter types below are deliberately `any` — the real object a
 * caller passes (`@wailsio/runtime`'s `WailsSocket | WebSocket`) is declared against the DOM lib's
 * `Event`/`MessageEvent`/`CloseEvent`, and a structural interface narrower than those types would
 * fail TypeScript's contravariant parameter check for a plain property-typed callback (unlike a
 * method, which checks bivariantly) purely on library-type-name grounds, not on any real mismatch
 * at runtime — every `data`/`code`/`reason` reference below is defensive-cast at the point of use.
 */
export interface StreamSocketLike {
  binaryType: string;
  // Optional: a structural implementer that never exposes readyState (none does today) is treated
  // as CONNECTING until its own onopen fires — see createStreamChannel's open-ack gate (P67b §3.1).
  readonly readyState?: number;
  // biome-ignore lint/suspicious/noExplicitAny: cross-project structural type — see interface doc above.
  onopen: ((ev: any) => void) | null;
  // biome-ignore lint/suspicious/noExplicitAny: cross-project structural type — see interface doc above.
  onmessage: ((ev: any) => void) | null;
  // biome-ignore lint/suspicious/noExplicitAny: cross-project structural type — see interface doc above.
  onclose: ((ev: any) => void) | null;
  // biome-ignore lint/suspicious/noExplicitAny: cross-project structural type — see interface doc above.
  onerror: ((ev: any) => void) | null;
  // `ArrayBuffer`, not `ArrayBufferLike` — the latter includes `SharedArrayBuffer`, which the DOM
  // lib's own real `WebSocket.send` overload does not accept, breaking structural assignability
  // for that arm of `Stream()`'s own `WailsSocket | WebSocket` return type. `post()` below only
  // ever sends a string in practice (the client never encodes a blob — this file's own header
  // comment); the wider signature is kept for fidelity to what a real socket accepts, not because
  // anything here constructs an ArrayBuffer to send.
  send(data: string | ArrayBuffer | ArrayBufferView<ArrayBuffer>): void;
  close(): void;
}

export interface StreamChannel {
  post(message: unknown): void;
  onMessage(handler: (message: unknown) => void): () => void;
  close(): void;
  onClose(handler: (err?: Error) => void): () => void;
}

/** Thrown when an inbound frame's body cannot be decoded as JSON and is not a valid blob frame
 *  either */
export class StreamFrameDeliveryError extends Error {
  constructor(reason: string) {
    super(`streamChannel: frame delivery failed: ${reason}`);
    this.name = 'StreamFrameDeliveryError';
  }
}

function decodeFrame(data: ArrayBuffer): unknown {
  const bytes = new Uint8Array(data);
  if (bytes.byteLength > 0 && bytes[0] === BLOB_FRAME_DISCRIMINANT) {
    return parseBlobFrameBody(bytes);
  }
  return JSON.parse(new TextDecoder('utf-8').decode(bytes));
}

// A Wails/DOM socket's readyState for OPEN — checked defensively (P67b §3.1) so a socket that is
// already open by the time it reaches this function (nothing does this today) is not stranded
// waiting for an onopen that already fired.
const SOCKET_OPEN = 1;

export function createStreamChannel(socket: StreamSocketLike): StreamChannel {
  socket.binaryType = 'arraybuffer';

  let currentHandler: ((message: unknown) => void) | null = null;
  const closeHandlers = new Set<(err?: Error) => void>();

  function fireClose(err?: Error): void {
    for (const handler of [...closeHandlers]) handler(err);
  }

  // `Stream()` returns a socket in the CONNECTING state, and Wails' own
  // `send()` throws on a CONNECTING socket rather than queueing — `post` must stay synchronous
  // (graphPlane.ts calls it from inside a Promise executor and from the credit path) so this is a FIFO
  // queue, not a promise chain, preserving frame order relative to credit/cancel.
  let phase: 'connecting' | 'open' | 'closed' =
    socket.readyState === SOCKET_OPEN ? 'open' : 'connecting';
  const queued: string[] = [];

  socket.onopen = () => {
    if (phase !== 'connecting') return;
    phase = 'open';
    for (const frame of queued) socket.send(frame);
    queued.length = 0;
  };

  socket.onmessage = (ev: { data: ArrayBuffer }) => {
    // Decode and delivery share one try/catch: a synchronous throw from the handler must destroy
    // the connection the same way a malformed frame does, never escape uncaught and silently drop
    // the frame.
    try {
      const message = decodeFrame(ev.data);
      // No pending-frame queue: the graph plane
      // subscribes with onMessage before the socket can have delivered anything, and this channel
      // has exactly one subscriber for its whole life — there is no resubscribe-across-an-await
      // gap to lose a frame in.
      if (currentHandler) currentHandler(message);
    } catch (err) {
      socket.close();
      fireClose(err instanceof Error ? err : new StreamFrameDeliveryError(String(err)));
    }
  };
  socket.onclose = () => {
    phase = 'closed';
    queued.length = 0;
    fireClose();
  };
  socket.onerror = () => {
    // An `error` event on a Wails/DOM socket is always followed by `close` — teardown lives in
    // onclose alone, the same posture `port.ts`'s own `socket.onerror` already takes.
  };

  return {
    post(message): void {
      const frame = JSON.stringify(message);
      if (phase === 'open') socket.send(frame);
      else if (phase === 'connecting') queued.push(frame);
      // 'closed': dropped — the peer is gone; onclose has already fired. Settling in-flight
      // streams is the subscriber's job (graphPlane.ts does, on onClose).
    },

    onMessage(handler): () => void {
      currentHandler = handler;
      return () => {
        if (currentHandler === handler) currentHandler = null;
      };
    },

    close(): void {
      socket.close();
    },

    onClose(handler): () => void {
      closeHandlers.add(handler);
      return () => closeHandlers.delete(handler);
    },
  };
}
