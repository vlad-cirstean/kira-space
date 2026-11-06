import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { Page } from '@playwright/test';
import { DATA_OP, PORT_EVENT } from '@shared/protocol/dataOps';
import {
  createDocumentPageBuilder,
  createKeyValuePageBuilder,
  createStreamPageBuilder,
  createTabularPageBuilder,
  pageByteSize,
  type Page as WirePage,
} from '@shared/protocol/page';
import type { LogicalPage, LogicalPortResponse, PortSnapshot } from '../../../ipc/support/types';
import { type EncodedFrame, encodeFrame, type FramePayload } from '../../../support/encodeFrame';

export interface SeenPortRequest {
  op: string;
  payload: unknown;
  /** The request's own `opId` (the id the renderer would pass to `opsCancel`), when it has one. */
  opId?: string;
}

/** `PortSnapshot` plus a Studio-local gate (Part 5's type stays unchanged): the reply is held
 *  until the renderer sends `opsCancel`, so a spec proves the view recovered because of the
 *  cancel, not because a canned reply timed out. */
export type StudioPortSnapshot = PortSnapshot & { untilCancel?: true };

/** Pushes one engine `cache:stats` event to the open stream, as the engine does on a change. */
export async function pushCacheStats(
  page: Page,
  stats: {
    l2Bytes: number;
    l2BudgetBytes: number;
    l2Entries: number;
    l2Hits: number;
    l2Misses: number;
    l3Entries: number;
  },
): Promise<void> {
  const { bytes } = encodeFrame({
    kind: 'evt',
    topic: PORT_EVENT.cacheStats,
    payload: { type: 'cacheStats', ...stats },
  });
  const base64 = toBase64(bytes);
  await page.evaluate(
    (b) => (globalThis as unknown as { __kiraPushFrame(b: string): void }).__kiraPushFrame(b),
    base64,
  );
}

export interface MockStreamHandle {
  /** Every `PortRequest` the UI actually issued, in order — ported from
   *  `tests/ipc/support/mockPort.ts`'s `MockPortHandle.ops()` (P50 D7). */
  ops(): Promise<SeenPortRequest[]>;
}

interface EncodedFrameJSON {
  base64: string;
  idOffset: number;
}

/** What the injected browser script needs per snapshot: the request-matching key
 *  (`op`/`payload`) plus an already-encoded frame, so the browser has nothing left to build
 *  (page construction and wire encoding go through the real `@shared/protocol/page` builders
 *  and the generated FlatBuffers code). */
interface PreparedPortSnapshot {
  op: string;
  payload: unknown;
  /** Always a `res` frame — `ok:true` wrapping the logical response, or `ok:false` wrapping
   *  `error` — built with `id: 0` and `forceDefaults: true` so `idOffset` names a real byte
   *  position to patch a request's actual id into (mockStreamBrowser.js has no FlatBuffers
   *  knowledge of its own). */
  frame: EncodedFrameJSON;
  delayMs?: number;
  untilCancel?: true;
}

function toBase64(bytes: Uint8Array): string {
  return Buffer.from(bytes.buffer, bytes.byteOffset, bytes.byteLength).toString('base64');
}

function toJSON({ bytes, idOffset }: EncodedFrame): EncodedFrameJSON {
  return { base64: toBase64(bytes), idOffset };
}

function buildPage(logical: LogicalPage): WirePage {
  if (logical.kind === 'tabular') {
    const builder = createTabularPageBuilder(logical.columns);
    for (const row of logical.rows) builder.appendRow(row);
    const page = builder.finish(logical.position);
    if (logical.truncatedRows) {
      logical.truncatedRows.forEach((rows, i) => {
        if (rows) page.chunks[i].truncated = new Uint32Array(rows);
      });
      page.byteSize = pageByteSize(page);
    }
    return page;
  }
  if (logical.kind === 'document') {
    const builder = createDocumentPageBuilder();
    for (let i = 0; i < logical.ids.length; i++) {
      builder.push(logical.ids[i] ?? '', logical.bodies[i] ?? '');
    }
    return builder.finish(logical.position);
  }
  if (logical.kind === 'keyvalue') {
    const builder = createKeyValuePageBuilder({
      redisType: logical.redisType,
      ttlMs: logical.ttlMs,
      memoryBytes: logical.memoryBytes,
    });
    for (let i = 0; i < logical.fields.length; i++) {
      builder.push(logical.fields[i] ?? '', logical.values[i] ?? '');
    }
    return builder.finish(logical.position);
  }
  const builder = createStreamPageBuilder({
    visibilityTimeoutSeconds: logical.visibilityTimeoutSeconds,
  });
  for (let i = 0; i < logical.keys.length; i++) {
    builder.push({
      key: logical.keys[i],
      headers: logical.headers[i] ?? '',
      attrs: logical.attrs[i] ?? '',
      timestamp: logical.timestamps[i],
      // No `?? ''`, unlike headers/attrs above: body is `string | null` (a Kafka tombstone is a
      // real null, distinct from empty string), same as timestamp just above — coalescing it here
      // silently turned every mocked tombstone into an empty body.
      body: logical.bodies[i],
    });
  }
  return builder.finish(logical.position);
}

function buildFramePayload(response: LogicalPortResponse): FramePayload {
  if (response.kind === 'read') {
    return { type: 'read', page: buildPage(response.page), source: response.source };
  }
  if (response.kind === 'count') {
    return {
      type: 'count',
      value: response.value,
      exact: response.exact,
      at: Date.now(),
      stale: response.stale,
      source: response.source,
    };
  }
  if (response.kind === 'mutate') return { type: 'mutate', affectedRows: response.affectedRows };
  if (response.kind === 'preview') return { type: 'preview', statements: response.statements };
  if (response.kind === 'execute') {
    return { type: 'execute', pages: response.pages.map(buildPage) };
  }
  return { type: 'empty' };
}

function prepareSnapshot(snap: StudioPortSnapshot): PreparedPortSnapshot {
  const frame = snap.error
    ? encodeFrame({ kind: 'res', id: 0, ok: false, error: snap.error, forceDefaults: true })
    : encodeFrame({
        kind: 'res',
        id: 0,
        ok: true,
        payload: snap.response ? buildFramePayload(snap.response) : { type: 'empty' },
        forceDefaults: true,
      });
  return {
    op: snap.op,
    payload: snap.payload,
    frame: toJSON(frame),
    delayMs: snap.delayMs,
    untilCancel: snap.untilCancel,
  };
}

// Answers workbench/state/engine.ts's initEngineState ping — no spec's fixture array carries a
// 'ping' entry (types.ts's own doc comment: PortSnapshot.op is "a value from
// shared/protocol/dataOps.ts's DATA_OP map", and ping isn't one), so this is built once here
// rather than duplicated per spec.
const PING_FRAME = toJSON(
  encodeFrame({
    kind: 'res',
    id: 0,
    ok: true,
    payload: { type: 'ping', enginePid: 1, at: 0 },
    forceDefaults: true,
  }),
);

// One E_FIXTURE_MISS frame per DATA_OP value, so a miss still names its op — mirrors the plain
// object mockStreamBrowser.js used to build inline before FlatBuffers replaced the JSON wire
// format: that file has no encoding knowledge left, so every frame it can send must
// already exist.
const MISS_FRAMES: Record<string, EncodedFrameJSON> = Object.fromEntries(
  Object.values(DATA_OP).map((op) => [
    op,
    toJSON(
      encodeFrame({
        kind: 'res',
        id: 0,
        ok: false,
        error: { message: `no fixture snapshot for ${op}`, code: 'E_FIXTURE_MISS' },
        forceDefaults: true,
      }),
    ),
  ]),
);

// Read once, not per call: `installMockStream` runs at least once per test. `bun run format`
// (biome) reformats mockStreamBrowser.js like any other tracked file, including appending a
// trailing `;` after its top-level expression statement — stripped here so wrapping it in a call
// (`(${BROWSER_SCRIPT})(...)`, below) stays valid regardless of which way the formatter last
// touched it.
const BROWSER_SCRIPT = readFileSync(resolve(__dirname, 'mockStreamBrowser.js'), 'utf8')
  .trim()
  .replace(/;$/, '');

/**
 * Replaces the bulk-data transport from the renderer side by installing
 * `window._wails.streamFactory` — `@wailsio/runtime`'s `stream.js` calls this synchronously
 * inside `Stream(name)` if it exists, in preference to opening a real connection
 * (`stream.js`: *"Server builds install a factory returning a real WebSocket… both objects present
 * the same interface, so nothing above this line cares which it is"*). `bridge/port.ts` calls
 * `Stream('engine')` at its own module scope, so this must be installed via `page.addInitScript` —
 * before any page script runs, on every navigation — never `page.evaluate`, which would race the
 * app's own module graph.
 *
 * Every snapshot's response is pre-built here, through the real `@shared/protocol/page` builders
 * and the generated FlatBuffers encoders (`encodeFrame`), and shipped to the browser as a
 * `{base64, idOffset}` template — `mockStreamBrowser.js` decodes it, clones it per request and
 * patches in the request's real id at `idOffset`, and has no page-construction or FlatBuffers
 * knowledge of its own. The socket shim itself still lives in `mockStreamBrowser.js`, a plain,
 * uncompiled JS file injected as a **string**, not `page.addInitScript(fn, arg)` — see that file's
 * own doc comment for why a typed function does not survive the round trip here (a `keepNames`
 * artefact common to every esbuild-based TS loader this repo's tooling runs under).
 */
export async function installMockStream(
  page: Page,
  snapshots: readonly StudioPortSnapshot[],
): Promise<MockStreamHandle> {
  const init = {
    snapshots: snapshots.map(prepareSnapshot),
    ping: PING_FRAME,
    miss: MISS_FRAMES,
  };
  await page.addInitScript(`(${BROWSER_SCRIPT})(${JSON.stringify(init)});`);

  return {
    ops: () =>
      page.evaluate(
        () => (globalThis as unknown as { __kiraStreamSeen: SeenPortRequest[] }).__kiraStreamSeen,
      ),
  };
}
