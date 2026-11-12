// The one earned unit test: blameLine.ts's controller (extracted from
// blameAnnotation.ts) holds two rules that have each already produced a real bug once (the file's
// own comments at the fix sites record both): the `line === lastLine` early return must precede
// `cancelPending` (otherwise a same-line cursor move cancels its own in-flight request and
// nothing restarts it), and an ABORTED request must never be cached as a miss (otherwise fast
// arrowing through lines accumulates permanently blame-less lines). `repo.changed` invalidation
// interacts with both. Drives the controller end to end with a plain fake cursor and the Wails
// call/event stubs the bound `GitService` calls go through — no Monaco import, per the controller's
// own `BlameCursorSource` narrowing.
import '@workbench/testing/unit/window';

import { afterEach, describe, expect, test } from 'bun:test';
import {
  emitWailsEvent,
  resetCallFactory,
  setCallFactory,
} from '@workbench/testing/unit/wailsRuntime';
import type { ResultOf, Transport } from '../../../../packages/git-ipc/src/index.ts';
import type { BlameLineControllerDeps } from '../../frontend/src/views/repo/blameLine';

const { createBlameLineController } = await import('../../frontend/src/views/repo/blameLine');

afterEach(resetCallFactory);

type BlameResult = ResultOf<'blame.line'>;
const UNCOMMITTED_SHA = '0'.repeat(40);

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

// Retries an assertion until it holds. Fixed sleeps raced the controller's 150 ms debounce on a
// loaded CI runner; waiting on the observable outcome has no timing margin to lose.
async function eventually(assertion: () => void, timeoutMs = 3000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    try {
      assertion();
      return;
    } catch (err) {
      if (Date.now() >= deadline) throw err;
      await sleep(5);
    }
  }
}

const callCount = (calls: readonly unknown[], n: number) => () => expect(calls).toHaveLength(n);

function fakeCursor() {
  let position: { lineNumber: number } | null = null;
  let listener: (() => void) | undefined;
  return {
    // Structurally satisfies `BlameCursorSource` (getPosition/onDidChangeCursorPosition) without
    // importing monaco-editor — the controller only ever reads `.lineNumber` and calls the
    // listener with no arguments of its own.
    source: {
      getPosition: () => position,
      onDidChangeCursorPosition: (cb: () => void) => {
        listener = cb;
        return { dispose: () => (listener = undefined) };
      },
    } as unknown as BlameLineControllerDeps['cursor'],
    moveTo(line: number): void {
      position = { lineNumber: line };
      listener?.();
    },
  };
}

interface PendingCall {
  readonly line: number;
  resolve(result: BlameResult): void;
  reject(err: unknown): void;
}

/** Answers the bound `RepoOpen`/`RepoClose` immediately and `BlameLine` per call, left to the test
 *  to resolve/reject. `repo.changed` is a real Wails event so the controller's own subscription can
 *  be driven directly. The transport is only the controller's host-local handle (`graph.revealCommit`). */
function fakeTransport(): {
  transport: Transport;
  calls: PendingCall[];
  emitRepoChanged: (repoId: string) => void;
} {
  const calls: PendingCall[] = [];
  setCallFactory((name, arg) => {
    const method = String(name).split('.').pop();
    let promise: Promise<unknown> & { cancel(): void };
    if (method === 'BlameLine') {
      const { line } = (arg as { params: { line: number } }).params;
      // No automatic abort -> reject wiring here, deliberately: the test drives
      // resolve()/reject() itself, including resolving an already-cancelled request
      // successfully (the real-world race `if (line === lastLine)` guards against on the
      // success path, not only via a transport that rejects on abort).
      promise = new Promise<BlameResult>((resolve, reject) => {
        calls.push({ line, resolve, reject });
      }) as typeof promise;
    } else if (method === 'RepoOpen') {
      const { path } = (arg as { params: { path: string } }).params;
      promise = Promise.resolve({ kind: 'ok', repo: { repoId: path } }) as typeof promise;
    } else if (method === 'ResetWindow') {
      // The window reset is memoized per module graph; whether it already ran depends on which
      // specs ran earlier, so the fake must answer it rather than rely on that order.
      promise = Promise.resolve({}) as typeof promise;
    } else if (method === 'RepoClose') {
      promise = Promise.resolve({}) as typeof promise;
    } else {
      throw new Error(`unexpected bound call ${String(name)}`);
    }
    promise.cancel = () => {};
    return promise;
  });
  return {
    transport: {} as Transport,
    calls,
    emitRepoChanged: (repoId: string) => {
      emitWailsEvent('kira:git:repo.changed', { repoId, kind: 'refsChanged' });
    },
  };
}

function result(sha: string): BlameResult {
  return { sha, author: 'Ada', authorTimeSeconds: 1_700_000_000, summary: 'a commit' };
}

describe('blameLine.ts createBlameLineController', () => {
  test('a same-line cursor move while a request is in flight: one call, still resolves', async () => {
    const { transport, calls } = fakeTransport();
    const cursor = fakeCursor();
    const controller = createBlameLineController({
      transport,
      gitRepoId: 'repo-a',
      path: 'a.ts',
      cursor: cursor.source,
    });

    cursor.moveTo(5);
    await eventually(callCount(calls, 1));
    expect(calls).toHaveLength(1);
    cursor.moveTo(5); // same line, request already in flight — must not cancel it (5a)

    calls[0]?.resolve(result('aaa'));
    await eventually(() =>
      expect(controller.state.value).toEqual({ kind: 'resolved', line: 5, ...result('aaa') }),
    );
    expect(calls).toHaveLength(1);
    controller.dispose();
  });

  test('a later line supersedes an earlier one: the earlier late response never becomes state', async () => {
    const { transport, calls } = fakeTransport();
    const cursor = fakeCursor();
    const controller = createBlameLineController({
      transport,
      gitRepoId: 'repo-b',
      path: 'a.ts',
      cursor: cursor.source,
    });

    cursor.moveTo(1);
    await eventually(callCount(calls, 1));
    expect(calls).toHaveLength(1);
    cursor.moveTo(2); // cancels line 1's in-flight request
    await eventually(callCount(calls, 2));
    expect(calls).toHaveLength(2);

    calls[1]?.resolve(result('bbb')); // line 2 resolves first
    await eventually(() =>
      expect(controller.state.value).toEqual({ kind: 'resolved', line: 2, ...result('bbb') }),
    );
    calls[0]?.resolve(result('aaa')); // line 1's late response arrives after
    await sleep(20); // absence check: the stale response must change nothing

    expect(controller.state.value).toEqual({ kind: 'resolved', line: 2, ...result('bbb') });
    controller.dispose();
  });

  test('an aborted request is not cached: revisiting the line issues a new request', async () => {
    const { transport, calls } = fakeTransport();
    const cursor = fakeCursor();
    const controller = createBlameLineController({
      transport,
      gitRepoId: 'repo-c',
      path: 'a.ts',
      cursor: cursor.source,
    });

    cursor.moveTo(3);
    await eventually(callCount(calls, 1));
    expect(calls).toHaveLength(1);
    cursor.moveTo(4); // aborts line 3's request (its AbortController.signal.aborted is now true)
    // The transport eventually reports the cancellation as a rejection — 5b: since the request
    // was aborted, this must NOT be cached as a resolved miss.
    calls[0]?.reject(new DOMException('aborted', 'AbortError'));
    await eventually(callCount(calls, 2));
    calls[1]?.resolve(result('ddd'));
    await eventually(() =>
      expect(controller.state.value).toEqual({ kind: 'resolved', line: 4, ...result('ddd') }),
    );

    cursor.moveTo(3); // revisit — a cached null would skip this silently
    await eventually(callCount(calls, 3));

    expect(calls).toHaveLength(3);
    expect(calls[2]?.line).toBe(3);
    calls[2]?.resolve(result('ccc'));
    await eventually(() =>
      expect(controller.state.value).toEqual({ kind: 'resolved', line: 3, ...result('ccc') }),
    );
    controller.dispose();
  });

  test('a genuine RPC failure is cached as a miss: revisiting issues none, state is none', async () => {
    const { transport, calls } = fakeTransport();
    const cursor = fakeCursor();
    const controller = createBlameLineController({
      transport,
      gitRepoId: 'repo-d',
      path: 'a.ts',
      cursor: cursor.source,
    });

    cursor.moveTo(6);
    await eventually(callCount(calls, 1));
    expect(calls).toHaveLength(1);
    calls[0]?.reject(new Error('boom')); // a real failure, not an abort
    await sleep(20); // state is 'none' before and after: only the cache effect below is observable
    expect(controller.state.value).toEqual({ kind: 'none' });

    cursor.moveTo(7);
    await eventually(callCount(calls, 2));
    cursor.moveTo(6); // revisit — cached miss, no new request
    await sleep(10);

    expect(calls).toHaveLength(2); // only line 7's own call, never a second line-6 call
    expect(controller.state.value).toEqual({ kind: 'none' });
    controller.dispose();
  });

  test('repo.changed for this repo clears and re-resolves; for another repo is a no-op', async () => {
    const { transport, calls, emitRepoChanged } = fakeTransport();
    const cursor = fakeCursor();
    const controller = createBlameLineController({
      transport,
      gitRepoId: 'repo-e',
      path: 'a.ts',
      cursor: cursor.source,
    });

    cursor.moveTo(9);
    await eventually(callCount(calls, 1));
    calls[0]?.resolve(result('eee1'));
    await eventually(() =>
      expect(controller.state.value).toEqual({ kind: 'resolved', line: 9, ...result('eee1') }),
    );

    emitRepoChanged('some-other-repo');
    await sleep(10);
    expect(calls).toHaveLength(1); // untouched — a different repo's event is a no-op

    emitRepoChanged('repo-e');
    await eventually(callCount(calls, 2));
    expect(calls).toHaveLength(2); // cache cleared, cursor's own line re-resolved
    calls[1]?.resolve(result('eee2'));
    await eventually(() =>
      expect(controller.state.value).toEqual({ kind: 'resolved', line: 9, ...result('eee2') }),
    );
    controller.dispose();
  });

  test('the all-zero sha resolves to uncommitted, never resolved', async () => {
    const { transport, calls } = fakeTransport();
    const cursor = fakeCursor();
    const controller = createBlameLineController({
      transport,
      gitRepoId: 'repo-f',
      path: 'a.ts',
      cursor: cursor.source,
    });

    cursor.moveTo(8);
    await eventually(callCount(calls, 1));
    calls[0]?.resolve(result(UNCOMMITTED_SHA));
    await eventually(() =>
      expect(controller.state.value).toEqual({ kind: 'uncommitted', line: 8 }),
    );
    controller.dispose();
  });
});
