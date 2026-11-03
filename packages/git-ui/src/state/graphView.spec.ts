import { describe, expect, spyOn, test } from 'bun:test';
import type { LayoutChunk } from '@kira/git-core';
import { buildPackedChunk } from '@kira/git-core/testing/packedChunk';
import type {
  EventKey,
  EventPayload,
  ParamsOf,
  RequestKey,
  ResultOf,
  StreamChunkOf,
  StreamKey,
  StreamParamsOf,
  Transport,
} from '@kira/git-ipc';
import { TransportError } from '@kira/git-ipc';
import { sleep } from '@workbench/testing/unit/async';
import { watch } from 'vue';
import { BridgeClient } from '../bridge/client.ts';
import { type LayoutClient, LayoutClientStaleError } from '../graph/layoutClient.ts';
import { GraphOrderState } from './graphOrder.ts';
import { GraphViewState } from './graphView.ts';

const REPO_A = '/repos/a';
const REPO_B = '/repos/b';

type GraphStreamChunk = StreamChunkOf<'graph.stream'>;

function chunkFor(
  repoId: string,
  sha: string,
  options: { readonly exhausted?: boolean } = {},
): GraphStreamChunk {
  const exhausted = options.exhausted ?? true;
  return {
    repoId,
    seq: 0,
    from: 0,
    to: 1,
    source: 'git',
    remaining: exhausted ? 0 : 1,
    exhausted,
    commits: buildPackedChunk([{ sha, subject: `${repoId} commit` }]),
  };
}

function fakeLayoutChunk(from: number, to: number): LayoutChunk {
  return {
    from,
    to,
    laneOf: new Uint32Array(Math.max(0, to - from)),
    colorOf: new Uint32Array(Math.max(0, to - from)),
    edges: new Uint32Array(0),
    edgeIndex: new Uint32Array(Math.max(0, to - from) + 1),
    patches: new Uint32Array(0),
    laneCount: 0,
    maxEdgeSpan: 0,
    transfer: [],
  };
}

/** F1's own regression fixture: a `LayoutClient` that never touches a real worker (bun test has
 *  no `import.meta.url`-relative module worker to load), settling every `submit()` immediately
 *  with an empty layout for whatever range it was asked to lay out. */
function fakeLayoutClient(): LayoutClient {
  return {
    submit: (input) => Promise.resolve(fakeLayoutChunk(input.from, input.to)),
    reset: () => {},
    dispose: () => {},
  };
}

interface StreamOpen {
  readonly repoId: string;
  readonly resumeThroughRow: number | undefined;
  push(chunk: GraphStreamChunk): void;
  end(): void;
}

/**
 * The scriptable `Transport`: `graph.stream` opens stay pending — recorded onto
 * `streamOpens` — until the test pushes a chunk and/or ends the stream, or the caller's own
 * `AbortSignal` fires (abort *resolves*, never rejects, like the graph plane's stream()).
 * `graph.loadMore` calls hand back a controller the test resolves on its own schedule, so a
 * repo switch can land while one is genuinely still in flight.
 * `graph.status` always answers "exhausted", which is all `#runLoad`'s post-resync call reads.
 */
class RaceTransport implements Transport {
  readonly streamOpens: StreamOpen[] = [];
  readonly loadMoreRequests: Array<{ resolve: () => void }> = [];
  #handlers = new Map<EventKey, Set<(payload: unknown) => void>>();

  request<K extends RequestKey>(
    method: K,
    _params: ParamsOf<K>,
    signal?: AbortSignal,
  ): Promise<ResultOf<K>> {
    if (method === 'graph.loadMore') {
      return new Promise<ResultOf<K>>((resolve, reject) => {
        if (signal?.aborted) {
          reject(new TransportError('cancelled', `request '${method}' was already cancelled`));
          return;
        }
        signal?.addEventListener(
          'abort',
          () => reject(new TransportError('cancelled', `request '${method}' was cancelled`)),
          { once: true },
        );
        this.loadMoreRequests.push({
          resolve: () => resolve({ started: true } as ResultOf<K>),
        });
      });
    }
    if (method === 'graph.status') {
      return Promise.resolve({ loaded: 1, remaining: 0, exhausted: true } as ResultOf<K>);
    }
    return Promise.reject(new Error(`RaceTransport: unscripted request '${method}'`));
  }

  on<K extends EventKey>(method: K, handler: (payload: EventPayload<K>) => void): () => void {
    let set = this.#handlers.get(method);
    if (!set) {
      set = new Set();
      this.#handlers.set(method, set);
    }
    const wrapped = handler as (payload: unknown) => void;
    set.add(wrapped);
    return () => set?.delete(wrapped);
  }

  stream<K extends StreamKey>(
    method: K,
    params: StreamParamsOf<K>,
    onChunk: (chunk: StreamChunkOf<K>) => void,
    signal?: AbortSignal,
  ): Promise<void> {
    if (method !== 'graph.stream') {
      return Promise.reject(new Error(`RaceTransport: unscripted stream '${method}'`));
    }
    const streamParams = params as StreamParamsOf<'graph.stream'>;
    return new Promise<void>((resolve) => {
      this.streamOpens.push({
        repoId: streamParams.repoId,
        resumeThroughRow: streamParams.resumeThroughRow,
        push: (chunk) => void (onChunk as (chunk: GraphStreamChunk) => void)(chunk),
        end: resolve,
      });
      signal?.addEventListener('abort', () => resolve(), { once: true });
    });
  }

  dispose(): void {}
}

describe('GraphViewState — F1 repo switch mid-load', () => {
  test('a repo switch during loadMore never reopens the OLD repo stream, and leaves the new repo usable', async () => {
    const transport = new RaceTransport();
    const bridge = new BridgeClient(transport);
    const graphView = new GraphViewState(bridge, fakeLayoutClient());

    // Open + settle repo A's initial stream, same as `handleRepoOpened`'s first ever call.
    const openA = graphView.openStream(REPO_A);
    await sleep();
    expect(transport.streamOpens).toHaveLength(1);
    transport.streamOpens[0]?.push(chunkFor(REPO_A, '1'.repeat(40)));
    transport.streamOpens[0]?.end();
    await openA;
    expect(graphView.loading.value).toBe('idle');

    // Start a `loadMore()` for repo A and let its `graph.loadMore` request reach the transport,
    // but never let it settle yet — this is the in-flight load the switch must land on top of.
    const loadMorePromise = graphView.loadMore();
    await sleep();
    expect(transport.loadMoreRequests).toHaveLength(1);
    expect(graphView.loading.value).toBe('loadingMore');

    // Switch repos mid-load — `App.vue`'s `handleRepoOpened` shape exactly: reset(), then
    // openStream(newRepoId), with no await of the still-in-flight loadMore in between.
    graphView.reset();
    const openB = graphView.openStream(REPO_B);
    await sleep();
    expect(graphView.loading.value).toBe('streaming'); // openStream(B) owns `loading` now
    expect(transport.streamOpens).toHaveLength(2);
    expect(transport.streamOpens[1]?.repoId).toBe(REPO_B);

    // NOW let repo A's stale loadMore reply land.
    transport.loadMoreRequests[0]?.resolve();
    await loadMorePromise;

    // F1: the stale reply's resync must not have reopened repo A's stream, and must not have
    // clobbered `loading` out from under repo B's own still-open stream.
    expect(transport.streamOpens).toHaveLength(2);
    expect(graphView.loading.value).toBe('streaming');

    // Repo B's stream lands and ends normally.
    transport.streamOpens[1]?.push(chunkFor(REPO_B, '2'.repeat(40)));
    transport.streamOpens[1]?.end();
    await openB;

    expect(graphView.loading.value).toBe('idle');
    expect(graphView.store.rowOfSha('2'.repeat(40))).toBe(0);
    // Repo A's row never lands in repo B's store.
    expect(graphView.store.rowOfSha('1'.repeat(40))).toBe(-1);

    graphView.dispose();
  });

  test('loadAll() releases its own #loadController identity on a mid-loop repo switch, without clobbering a later one', async () => {
    const transport = new RaceTransport();
    const bridge = new BridgeClient(transport);
    const graphView = new GraphViewState(bridge, fakeLayoutClient());

    const openA = graphView.openStream(REPO_A);
    await sleep();
    transport.streamOpens[0]?.push(chunkFor(REPO_A, '1'.repeat(40), { exhausted: false }));
    transport.streamOpens[0]?.end();
    await openA;

    const loadAllPromise = graphView.loadAll();
    await sleep();
    expect(transport.loadMoreRequests).toHaveLength(1);

    graphView.reset();
    const openB = graphView.openStream(REPO_B);
    await sleep();

    // Repo B starts its own load before repo A's stale loadAll() iteration ever settles.
    transport.streamOpens[1]?.push(chunkFor(REPO_B, '2'.repeat(40)));
    transport.streamOpens[1]?.end();
    await openB;
    const loadMoreB = graphView.loadMore();
    await sleep();
    expect(transport.loadMoreRequests).toHaveLength(2);

    // Now repo A's original loadMore (inside loadAll()'s loop) finally resolves. Its own abort
    // (from reset()) already rejects it as cancelled inside #runLoad, so loadAll()'s loop exits
    // on the aborted signal; its `finally` must not clear the `#loadController` repo B's own
    // loadMore() just installed.
    await loadAllPromise;

    // Resolve repo B's own loadMore and let its resync run.
    transport.loadMoreRequests[1]?.resolve();
    await sleep();
    expect(transport.streamOpens).toHaveLength(3); // B's post-load resync reopened B's stream
    expect(transport.streamOpens[2]?.repoId).toBe(REPO_B);
    transport.streamOpens[2]?.push(chunkFor(REPO_B, '3'.repeat(40)));
    transport.streamOpens[2]?.end();
    await loadMoreB;

    expect(graphView.loading.value).toBe('idle');
    // Only ever repo B's own streams reopened — repo A's stale loadAll() never did.
    expect(
      transport.streamOpens.every(
        (open) => open.repoId === REPO_B || open === transport.streamOpens[0],
      ),
    ).toBe(true);

    graphView.dispose();
  });
});

// P168 Part 18 F8: a server that always sends the same bad chunk used to loop re-opens forever.
describe('GraphViewState — corrupted chunk recovery is bounded', () => {
  function corruptChunk(): GraphStreamChunk {
    return {
      ...chunkFor(REPO_A, '1'.repeat(40)),
      commits: buildPackedChunk([{ sha: '1'.repeat(40), subject: 'bad' }], { dictionaryBase: 9 }),
    };
  }

  test('re-opens once, then stops and announces', async () => {
    const logged = spyOn(console, 'error').mockImplementation(() => {});
    const transport = new RaceTransport();
    const graphView = new GraphViewState(new BridgeClient(transport), fakeLayoutClient());
    const open = graphView.openStream(REPO_A);
    await sleep();
    transport.streamOpens[0]?.push(corruptChunk());
    await sleep();
    expect(transport.streamOpens).toHaveLength(2);
    transport.streamOpens[1]?.push(corruptChunk());
    await sleep();
    expect(transport.streamOpens).toHaveLength(2);
    expect(graphView.announcement.value).toContain('corrupted');
    transport.streamOpens[1]?.end();
    await open;
    logged.mockRestore();
  });
});

describe('GraphViewState — restart from row 0', () => {
  const sha = (n: number) => n.toString(16).padStart(2, '0').repeat(20);
  const chunk = (
    repoId: string,
    count: number,
    base: number,
    exhausted = true,
  ): GraphStreamChunk => ({
    ...chunkFor(repoId, sha(base), { exhausted }),
    to: count,
    commits: buildPackedChunk(
      Array.from({ length: count }, (_, i) => ({ sha: sha(base + i), subject: `c${base + i}` })),
    ),
  });

  test('the old history stays whole while the re-walk streams, then swaps in at once', async () => {
    const transport = new RaceTransport();
    const graphView = new GraphViewState(new BridgeClient(transport), fakeLayoutClient());
    const open = graphView.openStream(REPO_A);
    await sleep();
    transport.streamOpens[0]?.push(chunk(REPO_A, 10, 1));
    await sleep();
    expect(graphView.plan.value.length).toBe(10);

    const overruns: number[] = [];
    watch(
      graphView.generation,
      () => {
        if (graphView.plan.value.length > graphView.store.rowCount) {
          overruns.push(graphView.plan.value.length);
        }
      },
      { flush: 'sync' },
    );
    const generation = graphView.generation.value;
    transport.streamOpens[0]?.push(chunk(REPO_A, 3, 50, false));
    await sleep();
    expect(graphView.plan.value.length).toBe(10);
    expect(graphView.store.rowCount).toBe(10);
    expect(graphView.loadedRows.value).toBe(10);
    expect(graphView.generation.value).toBe(generation);

    transport.streamOpens[0]?.end();
    await open;
    expect(overruns).toEqual([]);
    expect(graphView.plan.value.length).toBe(3);
    expect(graphView.store.rowCount).toBe(3);
    expect(graphView.loadedRows.value).toBe(3);
    expect(graphView.store.shaAt(0)).toBe(sha(50));
    expect(graphView.generation.value).toBe(generation + 1);
    graphView.dispose();
  });

  test('rebuildOrder racing the staged swap still swaps', async () => {
    const pending: Array<{
      to: number;
      resolve: (chunk: LayoutChunk) => void;
      reject: (error: Error) => void;
    }> = [];
    const client: LayoutClient = {
      submit: (input) =>
        new Promise<LayoutChunk>((resolve, reject) => {
          pending.push({ to: input.to, resolve, reject });
        }),
      reset: () => {
        for (const request of pending.splice(0)) request.reject(new LayoutClientStaleError(0));
      },
      dispose: () => {},
    };
    const transport = new RaceTransport();
    const graphView = new GraphViewState(
      new BridgeClient(transport),
      client,
      new GraphOrderState(),
    );
    const open = graphView.openStream(REPO_A);
    await sleep();
    transport.streamOpens[0]?.push(chunk(REPO_A, 10, 1));
    await sleep();
    pending[0]?.resolve(fakeLayoutChunk(0, 10));
    await sleep();
    expect(graphView.plan.value.length).toBe(10);

    transport.streamOpens[0]?.push(chunk(REPO_A, 3, 50, false));
    transport.streamOpens[0]?.end();
    await sleep();
    expect(pending).toHaveLength(1);
    const racing = graphView.rebuildOrder(); // stales the swap's own submit
    await sleep();
    expect(pending).toHaveLength(1);
    pending[0]?.resolve(fakeLayoutChunk(0, 3));
    await racing;
    await open;
    expect(graphView.store.rowCount).toBe(3);
    expect(graphView.plan.value.length).toBe(3);
    graphView.dispose();
  });

  test('reset() during staging discards it and the new repo streams progressively', async () => {
    const transport = new RaceTransport();
    const graphView = new GraphViewState(new BridgeClient(transport), fakeLayoutClient());
    const open = graphView.openStream(REPO_A);
    await sleep();
    transport.streamOpens[0]?.push(chunk(REPO_A, 10, 1));
    await sleep();
    transport.streamOpens[0]?.push(chunk(REPO_A, 3, 50, false));
    await sleep();

    graphView.reset();
    expect(graphView.store.rowCount).toBe(0);
    const reopened = graphView.openStream(REPO_B);
    await sleep();
    transport.streamOpens[1]?.push(chunk(REPO_B, 2, 80, false));
    await sleep();
    expect(graphView.loadedRows.value).toBe(2);
    expect(graphView.plan.value.length).toBe(2);
    expect(graphView.store.shaAt(0)).toBe(sha(80));

    transport.streamOpens[1]?.end();
    await reopened;
    await open;
    graphView.dispose();
  });
});

describe('GraphViewState — plan and layout publish together', () => {
  test('overlapping relayouts: plan never moves before its layout lands, the stale one publishes nothing', async () => {
    const pending: Array<{
      to: number;
      resolve: (chunk: LayoutChunk) => void;
      reject: (error: Error) => void;
    }> = [];
    const client: LayoutClient = {
      submit: (input) =>
        new Promise<LayoutChunk>((resolve, reject) => {
          pending.push({ to: input.to, resolve, reject });
        }),
      // The real client rejects every in-flight submit as stale on reset.
      reset: () => {
        for (const request of pending.splice(0)) request.reject(new LayoutClientStaleError(0));
      },
      dispose: () => {},
    };
    const transport = new RaceTransport();
    const graphView = new GraphViewState(
      new BridgeClient(transport),
      client,
      new GraphOrderState(),
    );
    const sha = (n: number) => n.toString(16).padStart(2, '0').repeat(20);
    const seen: Array<{ published: number; current: number; layoutCurrent: boolean }> = [];
    graphView.onPlanPublished((published) =>
      seen.push({
        published: published.length,
        current: graphView.plan.value.length,
        layoutCurrent: graphView.layoutCurrent,
      }),
    );
    const open = graphView.openStream(REPO_A);
    await sleep();
    transport.streamOpens[0]?.push({
      ...chunkFor(REPO_A, sha(1)),
      to: 5,
      commits: buildPackedChunk(
        Array.from({ length: 5 }, (_, i) => ({ sha: sha(1 + i), subject: `c${i}` })),
      ),
    });
    await sleep();
    expect(pending).toHaveLength(1);
    expect(graphView.plan.value.length).toBe(0);
    expect(graphView.layoutCurrent).toBe(true);

    const second = graphView.rebuildOrder(); // stales the first submit
    await sleep();
    expect(pending).toHaveLength(1);
    expect(graphView.plan.value.length).toBe(0);
    expect(graphView.layoutCurrent).toBe(true);

    expect(seen).toEqual([]);

    pending[0]?.resolve(fakeLayoutChunk(0, 5));
    await second;
    expect(graphView.plan.value.length).toBe(5);
    expect(graphView.layoutCurrent).toBe(true);
    // The listener runs inside the publish, so it already sees the new plan and its layout.
    expect(seen).toEqual([{ published: 5, current: 5, layoutCurrent: true }]);

    transport.streamOpens[0]?.end();
    await open;
    graphView.dispose();
  });
});
