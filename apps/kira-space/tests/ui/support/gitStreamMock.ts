import { CONTRACT_VERSION } from '@kira/git-ipc';
import type { Page } from '@playwright/test';
import type { GraphStreamChunkFixture } from './graphStreamFixture';

// The bound GitService methods the transport routes off the stream, by the Wails call name the page
// POSTs to /wails/runtime. Answered in-browser from the same table as the stream requests, so one
// spec fixture serves both transports.
const BRIDGE_PKG = 'github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge';
const BOUND_GIT_WIRE: Readonly<Record<string, string>> = Object.fromEntries(
  Object.entries({
    AppInit: 'app.init',
    RepoOpen: 'repo.open',
    RepoClose: 'repo.close',
    StatusGet: 'status.get',
    RefsList: 'refs.list',
    CommitDetail: 'commit.detail',
    CommitFileDiff: 'commit.fileDiff',
    WorkingDetail: 'working.detail',
    FileRead: 'file.read',
    FileGoToTarget: 'file.goToTarget',
    BlameLine: 'blame.line',
    SearchRun: 'search.run',
    RepoSettingsGet: 'repoSettings.get',
    RepoSettingsSet: 'repoSettings.set',
    PreflightCheckout: 'preflight.checkout',
    PreflightRevert: 'preflight.revert',
    PreflightReset: 'preflight.reset',
    PreflightCherryPick: 'preflight.cherryPick',
    StashList: 'stash.list',
    StashShow: 'stash.show',
    PreflightStashPop: 'preflight.stashPop',
    PreflightStashBranch: 'preflight.stashBranch',
    GlobalStashList: 'globalStash.list',
    OpRun: 'op.run',
    UndoPeek: 'undo.peek',
    UndoRun: 'undo.run',
    RemotePullPreflight: 'remote.pullPreflight',
    RemotePushPreflight: 'remote.pushPreflight',
    RemoteRun: 'remote.run',
    RemoteCancel: 'remote.cancel',
    WorktreeList: 'worktree.list',
    PreflightWorktreeAdd: 'preflight.worktreeAdd',
    PreflightWorktreeRemove: 'preflight.worktreeRemove',
    StackList: 'stack.list',
    PreflightRestack: 'preflight.restack',
    StackRestack: 'stack.restack',
    StackCancelRestack: 'stack.cancelRestack',
    ReviewResolveBase: 'review.resolveBase',
    ReviewFiles: 'review.files',
    ReviewFileDiff: 'review.fileDiff',
    ReviewSnapshot: 'review.snapshot',
    ReviewMark: 'review.mark',
    ReviewMarkMany: 'review.markMany',
    ReviewCommentAdd: 'review.comment.add',
    ReviewCommentList: 'review.comment.list',
    ReviewCommentRemove: 'review.comment.remove',
    ReviewCommentSetState: 'review.comment.setState',
    ReviewCommentEdit: 'review.comment.edit',
    ReviewCommentClear: 'review.comment.clear',
    ReviewCommentExport: 'review.comment.export',
    CommitResolveChangeRequests: 'commit.resolveChangeRequests',
    BranchResolveChangeRequest: 'branch.resolveChangeRequest',
    BranchChangeRequestIndex: 'branch.changeRequestIndex',
    BranchSyncChangeRequests: 'branch.syncChangeRequests',
    GitHostInfo: 'gitHost.info',
    ChangeRequestBrowserUrl: 'changeRequest.browserUrl',
    ChangeRequestReviewState: 'changeRequest.reviewState',
    ChangeRequestSubmitReview: 'changeRequest.submitReview',
    ChangeRequestComment: 'changeRequest.comment',
    ChangeRequestLineComment: 'changeRequest.lineComment',
  }).map(([name, wire]) => [`${BRIDGE_PKG}.GitService.${name}`, wire]),
);

// GitHostService.Providers is also the runtime mock's `gitHostProviders`: a spec that sets a
// `gitHost.providers` result answers it here, any other falls through to that mock.
const GIT_HOST_PROVIDERS_CALL = `${BRIDGE_PKG}.GitHostService.Providers`;

interface InstallArgs extends GitStreamMockArgs {
  contractVersion: number;
  boundWire: Readonly<Record<string, string>>;
}

function installArgs(args: GitStreamMockArgs): InstallArgs {
  return {
    ...args,
    contractVersion: CONTRACT_VERSION,
    boundWire: { ...BOUND_GIT_WIRE, [GIT_HOST_PROVIDERS_CALL]: 'gitHost.providers' },
  };
}

// repo/git/transport.ts speaks the native git JSON-RPC protocol (@kira/git-ipc's rpc.ts) over a
// Wails Stream — an entirely different wire (and mocking mechanism, `window._wails.streamFactory`)
// than mockRuntime.ts's `control.*` Call endpoint or mockStream.ts's own FlatBuffers bulk-data
// protocol, and one this repo has no existing mock for (C10 never added a UI test for its own
// graph mount for the identical reason). installGitStreamMock below is a minimal, purpose-built
// stand-in — real enough to answer the three requests this shallow path actually needs
// (app.init, repo.list, refs.list) and silent (never crashing, just never resolving) for every
// other method, which is exactly what a real bootstrap() tolerates: review.session.load and
// review.resolveBase are awaited but never block the branch picker from rendering.
//
// P67b §3/§9: moved out of repo-workspace.spec.ts (its original, sole caller) so
// repo-graph-lifecycle.spec.ts can drive the same socket double for its two bug-regression cases.
// The socket's own `readyState`/`onopen` timing is the one thing both bugs hinge on:
//  - it starts CONNECTING (0) and flips to OPEN (1) only after a real macrotask (`setTimeout`),
//    the same "Stream() returns before the handshake completes" ordering the real WailsSocket has
//    (§3's own root cause) — an opened-synchronously double would make bug 2's regression pass
//    vacuously regardless of whether streamChannel.ts's open-ack gate is present.
//  - `send()` now THROWS on a CONNECTING socket, mirroring `@wailsio/runtime`'s own
//    `WailsSocket.send` (§3's root-cause quote) — without this, the pre-fix `post` (a bare
//    `socket.send(...)`) would silently succeed against this double even though the real socket
//    throws, which is the same vacuous-pass risk from the opposite direction.
// P76 §12.2: `extraResults` is optional and additive — merged into `resultByMethod` only for a
// caller that passes it, so `repo-graph-lifecycle.spec.ts`'s own bootstrap() (and every other
// existing caller) keeps hanging on every method beyond the three above exactly as before. Adding
// `repo.open`/`blame.line` globally would let bootstrap() proceed past a point it currently hangs
// at, a behavior change for specs that never asked for it (P74/P75 both recorded this mock's own
// missing graph.stream/commit.detail as a known gap — this stays additive for the same reason).
//
// P114 §3.1: two install paths share the browser-side body (`installInBrowser`) below.
// `installGitStreamMock` (`page.evaluate`) is the default — install per-spec, right before the
// click that first opens the git transport. `installGitStreamMockOnInit` (`page.addInitScript`) is
// for a launch whose *boot itself* opens the transport (a restored, active repo-file tab —
// `main.ts`'s post-hydrate fall-forward mounts it before the first click is possible), so the mock
// must exist before any page script runs. `page.addInitScript` JSON-serializes its args: an
// `undefined` value drops its key entirely, which then hangs that method forever in the mock
// (`method in resultByMethod` is false). `page.evaluate` preserves `undefined` as a real value, so
// existing per-spec callers can pass `'repo.open': undefined` safely — an init-script caller must
// use `'repo.open': null` (or any JSON-safe value) instead.
export interface GitStreamMockArgs {
  repoId: string;
  extraResults?: Record<string, unknown>;
  graphStreamChunks?: readonly GraphStreamChunkFixture[];
  /** Per-open chunk lists: open N answers list N, the last list repeats. Wins over
   *  `graphStreamChunks` when both are given. */
  graphStreamOpens?: readonly (readonly GraphStreamChunkFixture[])[];
  /** Chunks past this index of the held open are held until `gitStreamRelease`. */
  graphStreamHoldAfter?: number;
  /** Which open `graphStreamHoldAfter` applies to; the first by default. */
  graphStreamHoldOpen?: number;
}

function installInBrowser({
  repoId,
  extraResults,
  graphStreamChunks,
  graphStreamOpens,
  graphStreamHoldAfter,
  graphStreamHoldOpen,
  contractVersion,
  boundWire,
}: InstallArgs): void {
  const w =
    (window as unknown as { _wails?: { streamFactory?: (name: string) => unknown } })._wails ?? {};
  (window as unknown as { _wails: typeof w })._wails = w;
  const existingFactory = w.streamFactory;

  // P114 §3.1: every method name seen in a `t: 'req'` frame, answered or not — read back by
  // `gitStreamRequests` below. Reset per install so a fresh `relaunch()` starts a fresh log.
  (window as unknown as { __kiraGitRequests: string[] }).__kiraGitRequests = [];
  (window as unknown as { __kiraGitParams: unknown[] }).__kiraGitParams = [];
  (window as unknown as { __kiraGitWindowKeys: string[] }).__kiraGitWindowKeys = [];
  const delivered: number[] = [];
  (window as unknown as { __kiraGitDelivered: number[] }).__kiraGitDelivered = delivered;

  let streamOpenCount = 0;
  const seqCounts: Record<string, number> = {};
  let releaseHeld: (() => void) | undefined;
  (window as unknown as { __kiraGitRelease: () => void }).__kiraGitRelease = () => releaseHeld?.();
  let releaseOne: (() => void) | undefined;
  (window as unknown as { __kiraGitReleaseNext: () => void }).__kiraGitReleaseNext = () =>
    releaseOne?.();
  // Git events reach the page as `kira:git:<event>` Wails events (EmitTo), not stream frames.
  (
    window as unknown as {
      __kiraGitEmit: (method: string, payload: unknown) => Promise<void>;
    }
  ).__kiraGitEmit = (method, payload) => {
    (
      window as unknown as {
        _wails: { dispatchWailsEvent(e: { name: string; data: unknown }): void };
      }
    )._wails.dispatchWailsEvent({ name: `kira:git:${method}`, data: payload });
    return Promise.resolve();
  };

  const CONNECTING = 0;
  const OPEN = 1;
  const CLOSED = 3;

  interface MockSocket {
    binaryType: string;
    onopen: ((ev: unknown) => void) | null;
    onmessage: ((ev: { data: ArrayBuffer }) => void) | null;
    onclose: ((ev: unknown) => void) | null;
    onerror: ((ev: unknown) => void) | null;
    readyState: number;
    send(data: string): void;
    close(): void;
  }

  // A test's fake clock replaces the global timers; the transport double keeps the real ones.
  const nativeSetTimeout = window.setTimeout.bind(window);

  function deliver(socket: MockSocket, envelope: unknown): Promise<void> {
    const bytes = new TextEncoder().encode(JSON.stringify(envelope));
    return new Promise((resolve) => {
      nativeSetTimeout(() => {
        socket.onmessage?.({ data: bytes.buffer });
        resolve();
      }, 0);
    });
  }

  // `blobFrame.ts`'s own layout, assembled here rather than decoded — the real
  // client never sends a blob (that file's own doc comment), so no encoder exists there to
  // reuse. `id`/`version` are only known at this point (the request's own runtime fields),
  // which is why `graphStreamFixture.ts` hands over `meta`/`blob` separately instead of a
  // whole pre-built frame.
  function deliverGraphStreamChunk(
    socket: MockSocket,
    version: number,
    id: number,
    chunk: GraphStreamChunkFixture,
    open: number,
  ): void {
    const blobBytes = Uint8Array.from(atob(chunk.blob), (c) => c.charCodeAt(0));
    const header = {
      version,
      body: {
        t: 'chunk',
        id,
        seq: chunk.meta.seq,
        chunk: {
          repoId: chunk.meta.repoId,
          seq: chunk.meta.seq,
          from: chunk.meta.from,
          to: chunk.meta.to,
          source: chunk.meta.source,
          remaining: chunk.meta.remaining,
          exhausted: chunk.meta.exhausted,
          commits: { $fb: chunk.meta.commitsFb, d: { $blob: true } },
        },
      },
    };
    const headerBytes = new TextEncoder().encode(JSON.stringify(header));
    const out = new Uint8Array(1 + 4 + headerBytes.byteLength + blobBytes.byteLength);
    const view = new DataView(out.buffer);
    out[0] = 0x00;
    view.setUint32(1, headerBytes.byteLength, false);
    out.set(headerBytes, 5);
    out.set(blobBytes, 5 + headerBytes.byteLength);
    nativeSetTimeout(() => {
      delivered[open] = (delivered[open] ?? 0) + 1;
      socket.onmessage?.({ data: out.buffer });
    }, 0);
  }

  // Logs the request and answers it from the fixture table: undefined hangs it forever.
  // `{ __sequence: [a, b, ...] }`: the Nth request for a method answers entry N (the last repeats);
  // an entry `{ __error: { code, message } }` answers a wire failure.
  function answerRequest(
    method: string | undefined,
    params: unknown,
  ): { failure?: { code: string; message: string }; result?: unknown } | undefined {
    if (method !== undefined) {
      (window as unknown as { __kiraGitRequests: string[] }).__kiraGitRequests.push(method);
      (window as unknown as { __kiraGitParams: unknown[] }).__kiraGitParams.push({
        method,
        params,
      });
    }
    // @kira/git-ipc's rpc.ts frame union: {t:'res', id, ok:true, result}.
    const resultByMethod: Record<string, unknown> = {
      'app.init': {
        contractVersion,
        serverVersion: 'ui-test',
        git: { kind: 'ok', path: '/usr/bin/git', version: '2.40.0' },
      },
      'repo.list': { candidates: [], activeRepoId: repoId },
      'refs.list': {
        branches: [
          {
            refname: 'refs/heads/main',
            kind: 'branch',
            shortName: 'main',
            objectId: '0'.repeat(40),
            peeledObjectId: undefined,
            upstream: undefined,
            track: undefined,
            committerDate: 0,
            isHead: true,
            checkedOutIn: undefined,
            annotation: undefined,
          },
        ],
        remoteBranches: [],
        tags: [],
        head: { kind: 'branch', name: 'main' },
      },
      // Repo holds queue their next open behind a close, so an unanswered close would stall it.
      'repo.close': {},
      ...extraResults,
    };
    if (method === undefined || !(method in resultByMethod)) return undefined;
    let answer = resultByMethod[method] as { __sequence?: unknown[] } | undefined;
    if (answer && typeof answer === 'object' && Array.isArray(answer.__sequence)) {
      const n = seqCounts[method] ?? 0;
      seqCounts[method] = n + 1;
      answer = answer.__sequence[Math.min(n, answer.__sequence.length - 1)] as typeof answer;
    }
    const failure = (answer as { __error?: { code: string; message: string } } | undefined)
      ?.__error;
    return failure ? { failure } : { result: answer };
  }

  // The bound GitService calls: a /wails/runtime POST naming a GitService method is answered here,
  // everything else falls through to the page.route mock. The log keeps the bound call's params
  // without its windowKey, in order with the stream's requests.
  const nativeFetch = window.fetch.bind(window);
  window.fetch = async (input, init) => {
    const href = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    if (
      init?.method === 'POST' &&
      typeof init.body === 'string' &&
      new URL(href, window.location.href).pathname === '/wails/runtime'
    ) {
      const envelope = JSON.parse(init.body) as {
        object?: number;
        method?: number;
        args?: { methodName?: string; args?: unknown[] };
      };
      // Cancelling a bound call (an aborted git request) posts the runtime's cancel call; the
      // real backend accepts it, the page.route mock has no entry for it.
      if (
        envelope.object === 10 &&
        envelope.method === 0 &&
        envelope.args?.methodName === undefined
      ) {
        return new Response('null', {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      const call = envelope.args;
      const wire = call?.methodName === undefined ? undefined : boundWire[call.methodName];
      if (
        wire !== undefined &&
        !(wire === 'gitHost.providers' && !(wire in (extraResults ?? {})))
      ) {
        const arg = (call?.args?.[0] ?? {}) as { windowKey?: string; params?: unknown };
        (window as unknown as { __kiraGitWindowKeys: string[] }).__kiraGitWindowKeys.push(
          arg.windowKey ?? '',
        );
        const outcome = answerRequest(wire, arg.params ?? {});
        if (!outcome) {
          return new Promise<Response>((_, reject) =>
            init.signal?.addEventListener('abort', () =>
              reject(new DOMException('The operation was aborted.', 'AbortError')),
            ),
          );
        }
        await new Promise((resolve) => nativeSetTimeout(resolve, 0));
        const json = { 'Content-Type': 'application/json' };
        if (outcome.failure) {
          const body = { code: outcome.failure.code, message: outcome.failure.message };
          return new Response(
            JSON.stringify({ kind: 'RuntimeError', message: JSON.stringify(body), cause: body }),
            { status: 422, headers: json },
          );
        }
        return new Response(JSON.stringify(outcome.result ?? null), { status: 200, headers: json });
      }
    }
    return nativeFetch(input, init);
  };

  function createGitMockSocket(): MockSocket {
    const socket: MockSocket = {
      binaryType: 'arraybuffer',
      onopen: null,
      onmessage: null,
      onclose: null,
      onerror: null,
      readyState: CONNECTING,
      send(data: string) {
        // @wailsio/runtime's own WailsSocket.send throws on a CONNECTING socket (P67b §3's root
        // cause, quoted verbatim in the plan) — reproduced here so a channel that skips the
        // open-ack gate and sends synchronously actually fails the way the real bug did.
        if (socket.readyState === CONNECTING) {
          throw new DOMException('Still in CONNECTING state.', 'InvalidStateError');
        }
        // bridge/port.ts's own comment, describing the same WailsSocket: "throws on a
        // CONNECTING socket and silently drops on a closed one" — a request sent over a
        // transport whose lease/dispose tore down the underlying socket must hang
        // exactly as the real bug did, not (wrongly) still answer.
        if (socket.readyState === CLOSED) return;
        let envelope: {
          version: number;
          body?: { t?: string; id?: number; method?: string; params?: unknown };
        };
        try {
          envelope = JSON.parse(data);
        } catch {
          return;
        }
        const frame = envelope.body;
        if (frame?.id === undefined) return;
        // `graph.stream` opens as `t: 'open'`, not `t: 'req'` — a real unary call's frame
        // shape (rpc.ts's own `Frame` union). Only handled when a caller supplied
        // chunks; otherwise this falls through to the `!== 'req'` guard below and hangs,
        // same as every other unlisted method here.
        const openChunks = graphStreamOpens
          ? graphStreamOpens[Math.min(streamOpenCount, graphStreamOpens.length - 1)]
          : graphStreamChunks;
        if (frame.t === 'open' && frame.method === 'graph.stream' && openChunks) {
          const id = frame.id;
          const open = streamOpenCount;
          const hold = open === (graphStreamHoldOpen ?? 0) ? graphStreamHoldAfter : undefined;
          streamOpenCount++;
          const now = hold === undefined ? openChunks : openChunks.slice(0, hold);
          for (const chunk of now) {
            deliverGraphStreamChunk(socket, envelope.version, id, chunk, open);
          }
          const held = openChunks.slice(now.length);
          const finish = (): void => {
            for (const chunk of held.splice(0)) {
              deliverGraphStreamChunk(socket, envelope.version, id, chunk, open);
            }
            deliver(socket, { version: envelope.version, body: { t: 'end', id } });
          };
          releaseOne = (): void => {
            const chunk = held.shift();
            if (chunk) deliverGraphStreamChunk(socket, envelope.version, id, chunk, open);
            if (held.length === 0)
              deliver(socket, { version: envelope.version, body: { t: 'end', id } });
          };
          if (hold === undefined) finish();
          else releaseHeld = finish;
          return;
        }
        if (frame?.t !== 'req' || frame.id === undefined) return;
        const outcome = answerRequest(frame.method, frame.params);
        if (!outcome) return; // hang forever
        deliver(
          socket,
          outcome.failure
            ? {
                version: envelope.version,
                body: { t: 'res', id: frame.id, ok: false, error: outcome.failure },
              }
            : {
                version: envelope.version,
                body: { t: 'res', id: frame.id, ok: true, result: outcome.result },
              },
        );
      },
      close() {
        socket.readyState = CLOSED;
      },
    };
    // Genuinely asynchronous — a later macrotask, not the same tick `Stream()` returns in. Any
    // code that sends before this fires is sending on a CONNECTING socket, exactly the window
    // bug 2 lived in.
    nativeSetTimeout(() => {
      socket.readyState = OPEN;
      socket.onopen?.({});
    }, 0);
    return socket;
  }

  w.streamFactory = (name: string) =>
    name === 'git' ? createGitMockSocket() : existingFactory?.(name);
}

/**
 * Default install path: call right before the click that first opens the git transport
 * (`gitTransportFor` is lazy — see `repo-workspace.spec.ts`'s own comment at its first caller).
 * `extraResults`/`graphStreamChunks` are additive — see the module doc comment above.
 */
export async function installGitStreamMock(
  page: Page,
  gitRepoId: string,
  extraResults?: Record<string, unknown>,
  graphStreamChunks?: readonly GraphStreamChunkFixture[],
  graphStreamOpens?: readonly (readonly GraphStreamChunkFixture[])[],
  graphStreamHoldAfter?: number,
  graphStreamHoldOpen?: number,
): Promise<void> {
  await page.evaluate(
    installInBrowser,
    installArgs({
      repoId: gitRepoId,
      extraResults,
      graphStreamChunks,
      graphStreamOpens,
      graphStreamHoldAfter,
      graphStreamHoldOpen,
    }),
  );
}

/**
 * Boot-time install path, for a launch whose boot itself opens the git transport (a restored,
 * active repo-file tab) — see the module doc comment above for why `page.evaluate` is too late
 * there, and why `extraResults` values must be JSON-safe (`null`, not `undefined`).
 */
export async function installGitStreamMockOnInit(
  page: Page,
  args: GitStreamMockArgs,
): Promise<void> {
  await page.addInitScript(installInBrowser, installArgs(args));
}

/** The git-mock method request log — every method seen in a `t: 'req'` frame, in order, whether
 *  or not it was answered. Mirrors `mockStream.ts`'s own `ops()`. */
export async function gitStreamRequests(page: Page): Promise<string[]> {
  return page.evaluate(
    () => (window as unknown as { __kiraGitRequests?: string[] }).__kiraGitRequests ?? [],
  );
}

/** The window key of every bound GitService call so far, in order. */
export async function gitBoundWindowKeys(page: Page): Promise<string[]> {
  return page.evaluate(
    () => (window as unknown as { __kiraGitWindowKeys?: string[] }).__kiraGitWindowKeys ?? [],
  );
}

/** The params of every request sent for `method`, in order. */
export async function gitStreamParams(page: Page, method: string): Promise<unknown[]> {
  return page.evaluate(
    (m) =>
      (
        (window as unknown as { __kiraGitParams?: { method: string; params: unknown }[] })
          .__kiraGitParams ?? []
      )
        .filter((r) => r.method === m)
        .map((r) => r.params),
    method,
  );
}

/** Pushes a server event (`t: 'evt'`) to every open git socket, the way the daemon announces
 *  `repo.changed` or `autoFetch.changed`. */
export async function gitStreamEmit(page: Page, method: string, payload: unknown): Promise<void> {
  await page.evaluate(
    ({ method, payload }) =>
      (
        window as unknown as {
          __kiraGitEmit: (method: string, payload: unknown) => Promise<void>;
        }
      ).__kiraGitEmit(method, payload),
    { method, payload },
  );
}

/** How many chunks the `open`th `graph.stream` open has delivered so far. */
export async function gitStreamDelivered(page: Page, open: number): Promise<number> {
  return page.evaluate(
    (n) => (window as unknown as { __kiraGitDelivered?: number[] }).__kiraGitDelivered?.[n] ?? 0,
    open,
  );
}

/** Delivers the chunks `graphStreamHoldAfter` held back, then ends the held stream. */
export async function gitStreamRelease(page: Page): Promise<void> {
  await page.evaluate(() =>
    (window as unknown as { __kiraGitRelease: () => void }).__kiraGitRelease(),
  );
}

/** Delivers the next held chunk; the stream ends once the last one is out. */
export async function gitStreamReleaseNext(page: Page): Promise<void> {
  await page.evaluate(() =>
    (window as unknown as { __kiraGitReleaseNext: () => void }).__kiraGitReleaseNext(),
  );
}
