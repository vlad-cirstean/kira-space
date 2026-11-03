// transport.ts routes every git method through bound calls (after the one-per-page-load window
// reset) and sends graph.stream on the gitgraph data plane. What needs a test is the cancellation
// and release arithmetic: an aborted call cancels its binding promise, a disposed transport cancels
// what is in flight and releases every repo it holds, and the reset is awaited once and retried
// after a failure.
import '@workbench/testing/unit/window';

import { afterEach, describe, expect, test } from 'bun:test';
import { getStream, resetCallFactory, setCallFactory } from '@workbench/testing/unit/wailsRuntime';
import { setActivePinia } from 'pinia';
import { pinia } from '../../frontend/src/state/pinia';

setActivePinia(pinia);

const { forgetGitWindowReset } = await import('../../frontend/src/repo/git/boundGit');
const { gitTransportFor, disposeGitTransport } = await import(
  '../../frontend/src/repo/git/transport'
);

interface BoundCall {
  name: string;
  arg: { windowKey: string; params?: unknown };
  cancelled: boolean;
  resolve(value: unknown): void;
  reject(err: unknown): void;
}

const calls: BoundCall[] = [];
// ResetWindow answers at once unless a test holds it.
let holdReset = false;

function recordBoundCalls(): void {
  calls.length = 0;
  setCallFactory((name, arg) => {
    let resolve!: (value: unknown) => void;
    let reject!: (err: unknown) => void;
    const promise = new Promise<unknown>((res, rej) => {
      resolve = res;
      reject = rej;
    }) as Promise<unknown> & { cancel(): void };
    const call: BoundCall = {
      name: String(name).split('.').pop() ?? '',
      arg: arg as BoundCall['arg'],
      cancelled: false,
      resolve,
      reject,
    };
    promise.cancel = () => {
      call.cancelled = true;
    };
    calls.push(call);
    if (call.name === 'ResetWindow' && !holdReset) resolve(null);
    return promise;
  });
}

afterEach(() => {
  holdReset = false;
  resetCallFactory();
});

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
const names = () => calls.map((c) => c.name).filter((n) => n !== 'ResetWindow');
const settle = (p: Promise<unknown>) =>
  p.then(
    () => undefined,
    (err: unknown) => err,
  );

describe('git transport bound routing', () => {
  test('the window reset is awaited once across concurrent calls and retried after a failure', async () => {
    holdReset = true;
    forgetGitWindowReset();
    recordBoundCalls();
    const transport = gitTransportFor('bound-reset');
    const first = settle(transport.request('app.init', {}));
    const second = settle(transport.request('app.init', {}));
    await tick();
    expect(calls.map((c) => c.name)).toEqual(['ResetWindow']);

    calls[0]?.reject(new Error('boom'));
    expect(await first).toBeDefined();
    expect(await second).toBeDefined();
    expect(names()).toEqual([]);

    holdReset = false;
    const third = transport.request('app.init', {});
    await tick();
    expect(calls.map((c) => c.name)).toEqual(['ResetWindow', 'ResetWindow', 'AppInit']);
    calls[2]?.resolve({});
    await third;

    disposeGitTransport('bound-reset');
  });

  test('an aborted bound call cancels its binding promise and rejects cancelled', async () => {
    recordBoundCalls();
    const transport = gitTransportFor('bound-abort');
    const abort = new AbortController();
    const outcome = settle(transport.request('app.init', {}, abort.signal));
    await tick();
    expect(names()).toEqual(['AppInit']);

    abort.abort();
    const err = await outcome;
    expect((err as Error).name).toBe('TransportError');
    expect((err as { code: string }).code).toBe('cancelled');
    expect(calls.find((c) => c.name === 'AppInit')?.cancelled).toBe(true);

    disposeGitTransport('bound-abort');
  });

  test('dispose cancels in-flight calls and releases each opened repo; graph.stream goes on the plane', async () => {
    recordBoundCalls();
    const transport = gitTransportFor('bound-dispose');

    const status = transport.request('status.get', { repoId: 'r1' });
    await tick();
    expect(names()).toEqual(['RepoOpen']);
    calls.find((c) => c.name === 'RepoOpen')?.resolve({ kind: 'ok', repo: { repoId: 'r1' } });
    await tick();
    expect(names()).toEqual(['RepoOpen', 'StatusGet']);
    calls.find((c) => c.name === 'StatusGet')?.resolve({});
    await status;

    // graph.refresh is a bound call like every other request.
    void transport.request('graph.refresh', { repoId: 'r1' }).catch(() => undefined);
    await tick();
    expect(names()).toEqual(['RepoOpen', 'StatusGet', 'GraphRefresh']);

    // graph.stream dials the plane and sends one open frame; no bound call.
    const socket = getStream('gitgraph');
    socket.__open();
    const streamed = transport.stream('graph.stream', { repoId: 'r1' } as never, () => {});
    await tick();
    const open = JSON.parse(String(socket.sent[0])) as { t: string; id: string; credit: number };
    expect(open).toMatchObject({ t: 'open', credit: 2, params: { repoId: 'r1' } });
    expect(names()).toHaveLength(3);
    socket.__message(new TextEncoder().encode(JSON.stringify({ t: 'end', id: open.id })).buffer);
    await streamed;

    const inflight = settle(transport.request('commit.detail', { repoId: 'r1', sha: 'abc' }));
    await tick();
    expect(names()).toEqual(['RepoOpen', 'StatusGet', 'GraphRefresh', 'CommitDetail']);

    disposeGitTransport('bound-dispose');
    const err = await inflight;
    expect((err as { code: string }).code).toBe('transport-closed');
    expect(calls.find((c) => c.name === 'CommitDetail')?.cancelled).toBe(true);
    const close = calls.find((c) => c.name === 'RepoClose');
    expect(close?.arg.params).toEqual({ repoId: 'r1' });
  });
});
