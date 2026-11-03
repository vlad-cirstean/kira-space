// transport.ts routes the methods GitService carries through bound calls and everything else over
// the git stream. What needs a test is the cancellation and release arithmetic: an aborted call
// cancels its binding promise, a disposed transport cancels what is in flight and releases every
// repo it holds, and a method GitService does not carry still travels the stream.
import '@workbench/testing/unit/window';

import { afterEach, describe, expect, test } from 'bun:test';
import { getStream, resetCallFactory, setCallFactory } from '@workbench/testing/unit/wailsRuntime';
import { setActivePinia } from 'pinia';
import { pinia } from '../../frontend/src/state/pinia';

setActivePinia(pinia);

const { gitTransportFor, disposeGitTransport } = await import(
  '../../frontend/src/repo/git/transport'
);

interface BoundCall {
  name: string;
  arg: { windowKey: string; params?: unknown };
  cancelled: boolean;
  resolve(value: unknown): void;
}

const calls: BoundCall[] = [];

function recordBoundCalls(): void {
  calls.length = 0;
  setCallFactory((name, arg) => {
    let resolve!: (value: unknown) => void;
    const promise = new Promise<unknown>((r) => {
      resolve = r;
    }) as Promise<unknown> & { cancel(): void };
    const call: BoundCall = {
      name: String(name).split('.').pop() ?? '',
      arg: arg as BoundCall['arg'],
      cancelled: false,
      resolve,
    };
    promise.cancel = () => {
      call.cancelled = true;
    };
    calls.push(call);
    return promise;
  });
}

afterEach(resetCallFactory);

// The shared fake socket refires 'close' on every close() call, so disposing would recurse through
// the transport's own onClose; the real socket does not.
function disposeQuietly(socket: ReturnType<typeof getStream>, codeRepoId: string): void {
  const realClose = socket.close.bind(socket);
  socket.close = () => {};
  disposeGitTransport(codeRepoId);
  socket.close = realClose;
}

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

function answerStreamRequest(socket: ReturnType<typeof getStream>, from: number, result: unknown) {
  const frame = JSON.parse(String(socket.sent[from])) as { version: number; body: { id: number } };
  const reply = { version: frame.version, body: { t: 'res', id: frame.body.id, ok: true, result } };
  socket.__message(new TextEncoder().encode(JSON.stringify(reply)).buffer as ArrayBuffer);
}

describe('git transport bound routing', () => {
  test('an aborted bound call cancels its binding promise and rejects cancelled', async () => {
    recordBoundCalls();
    const transport = gitTransportFor('bound-abort');
    const abort = new AbortController();
    const pending = transport.request('app.init', {}, abort.signal);
    const outcome = pending.then(
      () => undefined,
      (err: unknown) => err,
    );
    await tick();
    expect(calls.map((c) => c.name)).toEqual(['AppInit']);

    abort.abort();
    const err = await outcome;
    expect((err as Error).name).toBe('TransportError');
    expect((err as { code: string }).code).toBe('cancelled');
    expect(calls[0]?.cancelled).toBe(true);

    disposeQuietly(getStream('git'), 'bound-abort');
  });

  test('dispose cancels in-flight calls and releases each opened repo, a stream method stays on the stream', async () => {
    recordBoundCalls();
    const socket = getStream('git');
    const transport = gitTransportFor('bound-dispose');
    socket.__open();
    const sentBefore = socket.sent.length;

    const status = transport.request('status.get', { repoId: 'r1' });
    await tick();
    // The repo opens bound first, then over the stream.
    expect(calls.map((c) => c.name)).toEqual(['RepoOpen']);
    calls[0]?.resolve({ kind: 'ok', repo: { repoId: 'r1' } });
    await tick();
    expect(JSON.parse(String(socket.sent[sentBefore])).body.method).toBe('repo.open');
    answerStreamRequest(socket, sentBefore, { kind: 'ok' });
    await tick();
    expect(calls.map((c) => c.name)).toEqual(['RepoOpen', 'StatusGet']);
    calls[1]?.resolve({});
    await status;

    // graph.refresh is not a GitService method: it goes over the stream, not a binding call.
    void transport.request('graph.refresh', { repoId: 'r1' }).catch(() => undefined);
    await tick();
    expect(JSON.parse(String(socket.sent.at(-1))).body.method).toBe('graph.refresh');
    expect(calls).toHaveLength(2);

    const inflight = transport.request('commit.detail', { repoId: 'r1', sha: 'abc' });
    const outcome = inflight.then(
      () => undefined,
      (err: unknown) => err,
    );
    await tick();
    expect(calls.map((c) => c.name)).toEqual(['RepoOpen', 'StatusGet', 'CommitDetail']);

    disposeQuietly(socket, 'bound-dispose');
    const err = await outcome;
    expect((err as { code: string }).code).toBe('transport-closed');
    expect(calls[2]?.cancelled).toBe(true);
    const close = calls.find((c) => c.name === 'RepoClose');
    expect(close?.arg.params).toEqual({ repoId: 'r1' });
  });
});
