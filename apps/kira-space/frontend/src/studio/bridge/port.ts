import { decodeFrame } from '@shared/protocol/frame';
import type { PortEvent, PortRequest, PortResponse } from '@shared/protocol/port';
// tsconfig.web.json maps this specifier onto @wailsio/runtime's real types (D8) and resolves it
// cleanly; tests/unit/tsconfig.json has no such mapping (a "paths" entry there breaks Bun's own
// mock.module interception for every file that transitively imports this one, not just the
// declaring project — see @workbench/testing/unit/wailsRuntime), so this import is unresolvable
// under that project, and TypeScript forbids an ambient `declare module` for a path-like
// specifier, leaving no clean per-project fix. The directive below deliberately stays the
// suppress-if-present kind rather than the require-an-error kind, which would itself fail as
// "unused" under the project where this import already resolves fine.
// biome-ignore lint/suspicious/noTsIgnore: an "unused directive" kind fails where this resolves fine (see comment above)
// @ts-ignore
import { Stream } from '/wails/runtime.js';

const DEFAULT_TIMEOUT_MS = 30_000;

interface PendingRequest {
  resolve: (payload: unknown) => void;
  reject: (err: Error & { code?: string }) => void;
  timer: ReturnType<typeof setTimeout> | null;
}

let nextId = 1;
const pending = new Map<number, PendingRequest>();
const eventListeners = new Map<string, Set<(payload: unknown) => void>>();

// One named stream, matching bridge/stream.go's StreamName. Wails supersedes an
// older page generation's session itself (stream.ts's WailsSocket._closed + the poll loop it
// aborts), which is what retires src/main/index.ts's own `generation` counter — nothing here
// re-implements it.
const socket = Stream('engine');
// P11 F4: WailsSocket wraps the payload in a Blob if binaryType === 'blob'. It defaults to
// 'arraybuffer', but this is set explicitly anyway — one line that removes a silent-Blob failure
// mode across three socket implementations (WailsSocket, native WebSocket, the test mock).
socket.binaryType = 'arraybuffer';

let closed = false;

function rejectAllPending(message: string): void {
  for (const [id, req] of pending) {
    if (req.timer) clearTimeout(req.timer);
    req.reject(new Error(message));
    pending.delete(id);
  }
}

let resolveReady!: () => void;
let rejectReady!: (err: Error) => void;
export const ready = new Promise<void>((resolve, reject) => {
  resolveReady = resolve;
  rejectReady = reject;
});
// The stream is opened at module scope and `ready` may reject before any caller attaches a
// handler; without this, a failed open is an unhandled rejection in the console rather than the
// error `initEngineState`'s own catch is about to report.
void ready.catch(() => {
  // Reported by initEngineState's own catch; this only marks the rejection as handled.
});

function handleMessage(message: PortResponse | PortEvent): void {
  if (message.kind === 'evt') {
    for (const cb of eventListeners.get(message.topic) ?? []) cb(message.payload);
    return;
  }
  const req = pending.get(message.id);
  if (!req) return;
  pending.delete(message.id);
  if (req.timer) clearTimeout(req.timer);
  if (message.ok) {
    req.resolve(message.payload);
  } else {
    const err: Error & { code?: string } = new Error(message.error.message);
    err.code = message.error.code;
    req.reject(err);
  }
}

socket.onopen = () => resolveReady();

// decodeFrame still throws on a structurally corrupt or truncated frame (a mismatched "KIF1"
// file identifier, a missing envelope field such as an event's topic) — genuinely reachable, not
// just a defensive check, and with no id to reject a specific pending request with either, so it
// is dropped here, the same move dataframe.go's own probe-decode makes on an unparseable frame.
// This callback runs synchronously from DOM event dispatch, which swallows a listener's throw into
// an uncaught-error report rather than routing it anywhere a promise could see it, so the
// try/catch here is what keeps one of those genuinely-unparseable frames from leaving some pending
// request hanging forever.
//
// A `res` frame's own *payload* failing to decode is different — its id is
// already known by the time that happens, so decodeFrame itself now resolves that case as an
// ordinary `{ok:false}` response (handleMessage below rejects the matching pending call with it)
// rather than throwing; nothing here needs to catch that case specially any more.
socket.onmessage = (ev: MessageEvent<unknown>) => {
  try {
    handleMessage(decodeFrame(new Uint8Array(ev.data as ArrayBuffer)));
  } catch {
    // Dropped.
  }
};

socket.onclose = () => {
  closed = true;
  rejectReady(new Error('engine stream closed'));
  rejectAllPending('engine stream closed before this request answered');
};
socket.onerror = () => {
  // `error` is always followed by `close`, so the teardown lives in onclose alone rather than
  // being duplicated and having to be made idempotent.
};

export function onPortEvent(topic: string, cb: (payload: unknown) => void): () => void {
  let set = eventListeners.get(topic);
  if (!set) {
    set = new Set();
    eventListeners.set(topic, set);
  }
  set.add(cb);
  return () => set?.delete(cb);
}

/**
 * `timeoutMs: null` means no client-side timeout (D25) — cancellation is the only escape hatch
 * for a data op, matching §5.1's rule that a stop button, never a timeout, is what ends a
 * still-running server query.
 */
export function request(
  op: string,
  payload: unknown = null,
  opts?: { timeoutMs?: number | null },
): Promise<unknown> {
  if (closed) return Promise.reject(new Error('engine stream is closed'));
  const id = nextId++;
  const timeoutMs = opts?.timeoutMs === undefined ? DEFAULT_TIMEOUT_MS : opts.timeoutMs;
  return new Promise((resolve, reject) => {
    const timer =
      timeoutMs === null
        ? null
        : setTimeout(() => {
            pending.delete(id);
            reject(new Error(`engine request "${op}" timed out`));
          }, timeoutMs);
    pending.set(id, { resolve, reject, timer });
    const req: PortRequest = { kind: 'req', id, op, payload };
    // send() throws on a CONNECTING socket and silently drops on a closed one, so the
    // send is gated on the open ack rather than fired synchronously. The timer starts now, not
    // after the ack — a request issued during a slow open must still time out on the caller's
    // schedule, which is the behaviour today's null-port rejection approximates.
    ready.then(
      () => {
        if (!closed) socket.send(JSON.stringify(req));
      },
      (err) => {
        pending.delete(id);
        if (timer) clearTimeout(timer);
        reject(err as Error);
      },
    );
  });
}
