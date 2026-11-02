import { execFileSync } from 'node:child_process';
import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import type { Page, Route } from '@playwright/test';

/**
 * The `/wails/` mock core both apps' own `tests/ui/support/mockRuntime.ts` implemented twice,
 * byte-identical bar each app's own bound-call table (`FQN_SUFFIX_BY_IPC_KEY`), wildcard defaults,
 * and two genuinely app-specific behaviors kept out of this file rather than forced in: Kira
 * Studio's `refresh:true` → `refresh:false` fixture fallback and its inferred `windowsEnsure` boot
 * mode (both exposed as hooks below, `resolveSnapshotFallback`/`resolveMissingBody`). See
 * `@workbench/testing/ui/server`'s own doc comment for why this tier hoists per-app path resolution
 * out to a parameter rather than computing it from `__dirname` here.
 */

/** One control-channel snapshot — the shape both apps' own `tests/ui/support/types.ts`
 *  `ControlSnapshot` already declare structurally identically; each app keeps its own doc comment
 *  on its own copy rather than importing this one, since neither wants a fixture-authoring type to
 *  come from a test-infra package. */
export interface ControlSnapshot<T = unknown> {
  channel: string;
  args?: unknown;
  response?: T;
  error?: { code: string; message: string; details?: unknown };
  /** Holds this call's reply until the spec calls the handle's own `release()`
   *  for this channel — a deterministic alternative to a fixed delay for staging an ordering race
   *  (e.g. another action completing while this call is still in flight), one call this mock
   *  intercepts at a time. Never set by a committed `tests/ipc/**` capture. */
  hold?: boolean;
}

export interface ControlLogEntry {
  channel: string;
  args: unknown;
}

export interface ControlMockHandle {
  /** Every Call this mock actually answered, in order. */
  log(): ControlLogEntry[];
  /** Releases one call on `channel` that was intercepted with `hold: true` and is
   *  waiting to be answered — lets its `route.fulfill` proceed. Safe to call before the held
   *  request has even reached this mock yet: the release is then banked and consumed by the
   *  next hold on that channel instead of being lost, so a spec's own action-then-release ordering
   *  never has to race the network round trip itself. */
  release(channel: string): void;
  /** From now on answers every call on `channel` with `response`, whatever its snapshots say; a spec
   *  uses it to change what a polled channel returns mid-test. */
  respond(channel: string, response: unknown): void;
}

interface CallRequestBody {
  object: number;
  method: number;
  args?: {
    'call-id': string;
    methodName?: string;
    args?: unknown[];
  };
}

/** `go list` resolves the on-disk path for whatever version go.mod pins, rather than
 *  hand-writing a GOPATH-shaped path that would silently go stale on a version bump. `cwd` is each
 *  app's own root (`resolve(__dirname, '../../../')` from its own
 *  `tests/ui/support/mockRuntime.ts`) — passed in rather than computed here for the same reason
 *  `server.ts`'s `distDir` is. */
export function resolveWailsRuntimeJsPath(cwd: string): string {
  const wailsModuleDir = execFileSync(
    'go',
    ['list', '-m', '-f', '{{.Dir}}', 'github.com/wailsapp/wails/v3'],
    { cwd, encoding: 'utf8' },
  ).trim();
  return resolve(wailsModuleDir, 'internal/assetserver/bundledassets/runtime.js');
}

/** `FQN_SUFFIX_BY_IPC_KEY`'s IPC-key -> legacy-channel-string, bridge-package-qualified FQN pair,
 *  both directions — each app's own `mockRuntime.ts` calls this once with its own service table. */
export function buildChannelMaps(
  ipc: Readonly<Record<string, string>>,
  fqnSuffixByIpcKey: Readonly<Record<string, string>>,
  bridgePkg: string,
): {
  channelToFqn: Readonly<Record<string, string>>;
  fqnToChannel: Readonly<Record<string, string>>;
} {
  const channelToFqn: Readonly<Record<string, string>> = Object.freeze(
    Object.fromEntries(
      Object.entries(fqnSuffixByIpcKey).map(([key, suffix]) => [
        ipc[key],
        `${bridgePkg}.${suffix}`,
      ]),
    ),
  );
  const fqnToChannel: Readonly<Record<string, string>> = Object.freeze(
    Object.fromEntries(Object.entries(channelToFqn).map(([channel, fqn]) => [fqn, channel])),
  );
  return { channelToFqn, fqnToChannel };
}

export interface CanonicalOptions {
  /** A key excluded outright regardless of value — a per-window/per-tab/per-terminal id generated
   *  at runtime, never reproducible from a committed fixture. */
  excludeKeys: readonly string[];
  /** Called for a key that survived `excludeKeys` and is not `undefined` — return true to drop it
   *  too. Kira Studio's own `refresh:false` normalisation (a live call always sends it explicitly;
   *  an older capture may predate the field and omit it) is the one user of this today. */
  dropKey?: (key: string, value: unknown) => boolean;
}

// Structured clone preserves a key whose value is `undefined`; the wire format here is JSON, which
// drops it outright — both are normalised the same way before comparing, so a fixture recorded from
// either transport matches.
export function canonical(value: unknown, options: CanonicalOptions): string {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    const out: Record<string, unknown> = {};
    for (const key of Object.keys(value as Record<string, unknown>).sort((a, b) =>
      a < b ? -1 : a > b ? 1 : 0,
    )) {
      if (options.excludeKeys.includes(key)) continue;
      const v = (value as Record<string, unknown>)[key];
      if (v === undefined) continue;
      if (options.dropKey?.(key, v)) continue;
      out[key] = v;
    }
    return JSON.stringify(out);
  }
  return JSON.stringify(value);
}

/** The exact shape each app's own `internal/bridge` transport writes for a bound method's error —
 *  `.message` is `ipcerr.Error`'s own JSON encoding, `.cause` is that same `{code, message}` as a
 *  real object. `control.ts`'s `unwrap` reads `.cause` first, so a fixture miss surfaces as a
 *  diagnosable `E_FIXTURE_MISS`, not a raw network failure. */
export function runtimeErrorBody(code: string, message: string, details?: unknown): string {
  const body: Record<string, unknown> = { code, message };
  if (details !== undefined) body.details = details;
  return JSON.stringify({ kind: 'RuntimeError', message: JSON.stringify(body), cause: body });
}

const runtimeJsCache = new Map<string, Buffer>();

async function serveWailsRuntimeJs(route: Route, runtimeJsPath: string): Promise<void> {
  let body = runtimeJsCache.get(runtimeJsPath);
  if (!body) {
    body = await readFile(runtimeJsPath);
    runtimeJsCache.set(runtimeJsPath, body);
  }
  await route.fulfill({ status: 200, contentType: 'text/javascript; charset=utf-8', body });
}

export interface InstallControlMocksConfig {
  fqnToChannel: Readonly<Record<string, string>>;
  wildcardDefaults: Readonly<Record<string, string>>;
  runtimeJsPath: string;
  canonicalOptions: CanonicalOptions;
  /** Called with the live call's own args when the direct (channel, canonical-args) lookup finds
   *  no snapshot — a hook for app-specific fallback matching (Kira Studio's `refresh:true` retried
   *  as `refresh:false`). Receives `findSnap` so it can re-run the same lookup against rewritten
   *  args. Undefined (the default) falls straight through to `resolveMissingBody`/the wildcard
   *  table below. */
  resolveSnapshotFallback?: (
    channel: string,
    callArgs: unknown,
    findSnap: (args: unknown) => ControlSnapshot | undefined,
  ) => ControlSnapshot | undefined;
  /** Called when no snapshot (direct or fallback) answers a channel, before `wildcardDefaults` is
   *  consulted — a hook for a fully custom miss response (Kira Studio's inferred `windowsEnsure`
   *  boot mode, read off whatever `tabsList` snapshot the spec did provide). Returns the JSON
   *  response body to answer with, or undefined to fall through. */
  resolveMissingBody?: (
    channel: string,
    byChannel: ReadonlyMap<string, ControlSnapshot[]>,
  ) => string | undefined;
}

/**
 * Replaces the control channel's answers at the network layer — `page.route` intercepts every
 * request under `/wails/`: the real runtime bundle itself (served for real, off disk) and the one
 * RPC endpoint bound calls POST to. The mocked HTTP response is exactly what `unwrap`/`trust` in
 * each app's own `bridge/rpc.ts` are written to consume, so a frontend spec still exercises that
 * code for real.
 */
export async function installControlMocks(
  page: Page,
  snapshots: readonly ControlSnapshot[],
  config: InstallControlMocksConfig,
): Promise<ControlMockHandle> {
  const log: ControlLogEntry[] = [];
  const byChannel = new Map<string, ControlSnapshot[]>();
  for (const snap of snapshots) {
    const list = byChannel.get(snap.channel) ?? [];
    list.push(snap);
    byChannel.set(snap.channel, list);
  }
  // Two or more snapshots can share one (channel, args) key on purpose.
  const byKey = new Map<string, Map<string, ControlSnapshot[]>>();
  for (const [channel, list] of byChannel) {
    const grouped = new Map<string, ControlSnapshot[]>();
    for (const snap of list) {
      const key = canonical(snap.args, config.canonicalOptions);
      const group = grouped.get(key) ?? [];
      group.push(snap);
      grouped.set(key, group);
    }
    byKey.set(channel, grouped);
  }
  const cursors = new Map<string, number>();
  const responders = new Map<string, unknown>();

  // P108 Part 12 F4: a rendezvous per channel, order-independent — whichever of "a hold arrived"
  // or "release() was called" happens second resolves the wait; the other case banks its own
  // event (a waiting resolver, or a spent credit) for the next one to consume.
  const heldResolvers = new Map<string, Array<() => void>>();
  const releaseCredits = new Map<string, number>();

  function release(channel: string): void {
    const waiting = heldResolvers.get(channel);
    const resolve = waiting?.shift();
    if (resolve) {
      resolve();
      return;
    }
    releaseCredits.set(channel, (releaseCredits.get(channel) ?? 0) + 1);
  }

  function waitForRelease(channel: string): Promise<void> {
    const credits = releaseCredits.get(channel) ?? 0;
    if (credits > 0) {
      releaseCredits.set(channel, credits - 1);
      return Promise.resolve();
    }
    return new Promise((resolve) => {
      const waiting = heldResolvers.get(channel) ?? [];
      waiting.push(resolve);
      heldResolvers.set(channel, waiting);
    });
  }

  // Split out of the route handler below (the hold/release addition pushed it one point past
  // biome's cognitive-complexity cap) — the three-way "no snapshot at all" fallback (app-specific
  // miss hook, then the wildcard table, then a diagnosable fixture-miss error) is a self-contained
  // decision, not entangled with the request-routing above it or the hold/error/ok response-writing
  // below it.
  async function fulfillMissingSnapshot(
    route: Route,
    channel: string,
    callArgs: unknown,
  ): Promise<void> {
    const missingBody = config.resolveMissingBody?.(channel, byChannel);
    if (missingBody !== undefined) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: missingBody });
      return;
    }
    const wildcard = config.wildcardDefaults[channel];
    if (wildcard !== undefined) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: wildcard });
      return;
    }
    await route.fulfill({
      status: 422,
      contentType: 'application/json',
      body: runtimeErrorBody(
        'E_FIXTURE_MISS',
        `no fixture snapshot for ${channel} args ${JSON.stringify(callArgs)}`,
      ),
    });
  }

  await page.route('**/wails/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());

    if (request.method() === 'GET' && url.pathname === '/wails/runtime.js') {
      await serveWailsRuntimeJs(route, config.runtimeJsPath);
      return;
    }

    // The runtime bundle's own last line (runtime.js's `loadOptionalScript`) HEADs this once per
    // page load, unconditionally — a 200 with a non-JavaScript content type is what a real Wails
    // backend's own custom-asset-not-configured path looks like at the HTTP level, avoiding a
    // spurious devtools "Failed to load resource" line a bare 404 would otherwise leave in every
    // spec's console-errors collection.
    if (url.pathname === '/wails/custom.js') {
      await route.fulfill({ status: 200, contentType: 'text/plain', body: '' });
      return;
    }

    if (url.pathname !== '/wails/runtime' || request.method() !== 'POST') {
      await route.fulfill({
        status: 501,
        contentType: 'text/plain',
        body: `unmocked ${request.method()} ${url.pathname}`,
      });
      return;
    }

    const body = JSON.parse(request.postData() ?? '{}') as CallRequestBody;
    const methodName = body.args?.methodName;
    const channel = methodName ? config.fqnToChannel[methodName] : undefined;
    const callArgs = body.args?.args?.[0];

    if (!channel) {
      await route.fulfill({
        status: 422,
        contentType: 'application/json',
        body: runtimeErrorBody('E_FIXTURE_MISS', `no CHANNEL_TO_FQN entry for ${methodName}`),
      });
      return;
    }
    log.push({ channel, args: callArgs });

    if (responders.has(channel)) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(responders.get(channel) ?? null),
      });
      return;
    }

    const grouped = byKey.get(channel);
    const list = byChannel.get(channel) ?? [];
    // A channel called with the same args every time (connectionsList, connectionsStates) has
    // exactly one snapshot and answers regardless of the exact args it was called with.
    function findSnap(args: unknown): ControlSnapshot | undefined {
      if (!grouped) return undefined;
      const key = canonical(args, config.canonicalOptions);
      const group = grouped.get(key);
      if (!group) return undefined;
      const at = cursors.get(`${channel}:${key}`) ?? 0;
      cursors.set(`${channel}:${key}`, at + 1);
      return group[Math.min(at, group.length - 1)];
    }
    const snap =
      list.length === 1
        ? list[0]
        : (findSnap(callArgs) ?? config.resolveSnapshotFallback?.(channel, callArgs, findSnap));
    if (!snap) {
      await fulfillMissingSnapshot(route, channel, callArgs);
      return;
    }
    if (snap.hold) await waitForRelease(channel);
    if (snap.error) {
      await route.fulfill({
        status: 422,
        contentType: 'application/json',
        body: runtimeErrorBody(snap.error.code, snap.error.message, snap.error.details),
      });
      return;
    }
    const responseBody = snap.response === undefined ? 'null' : JSON.stringify(snap.response);
    await route.fulfill({ status: 200, contentType: 'application/json', body: responseBody });
  });

  return {
    log: () => log,
    release,
    respond: (channel, response) => void responders.set(channel, response),
  };
}

/**
 * Delivers a pushed Wails event into a page that has real runtime.js loaded — the piece
 * `installControlMocks` above deliberately does not cover (it intercepts only the `Call` RPC
 * endpoint). The bundle exposes `window._wails.dispatchWailsEvent({name, data})` for exactly this
 * shape: `name` is the same channel string each app's own `bridge/rpc.ts`'s `on(...)` subscribes
 * `Events.On` to, `data` is whatever that channel's own `cb(ev.data)` expects.
 */
export async function emitWailsEvent(page: Page, name: string, data: unknown): Promise<void> {
  await page.evaluate(
    ({ name, data }) => {
      (
        window as unknown as {
          _wails: { dispatchWailsEvent(e: { name: string; data: unknown }): void };
        }
      )._wails.dispatchWailsEvent({ name, data });
    },
    { name, data },
  );
}
