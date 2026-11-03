import { describe, expect, test } from 'bun:test';
import type { StackListResult } from '@kira/git-ipc';
import { compareNatural } from '@workbench/behaviour/match';
import { sleep } from '@workbench/testing/unit/async';
import { BridgeClient } from '../bridge/client.ts';
import { FakeTransport } from '../testing/fakeTransport.ts';
import { ChangeRequestState } from './changeRequests.ts';
import { StackState } from './stack.ts';

const REPO_A = '/repos/a';
const REPO_B = '/repos/b';

function emptyResult(): StackListResult {
  return { stacks: [], orphans: [] };
}

function oneStackResult(): StackListResult {
  return {
    stacks: [
      {
        base: 'main',
        baseTip: 'm1',
        needsRestack: true,
        branches: [
          {
            name: 'feat1',
            parent: 'main',
            depth: 0,
            tip: 'f1',
            parentTip: 'm1',
            recordedBase: 'm1',
            behind: 0,
            ahead: 1,
            state: 'upToDate',
            checkedOutIn: undefined,
            track: undefined,
            isHead: false,
          },
          {
            name: 'feat2',
            parent: 'feat1',
            depth: 1,
            tip: 'f2',
            parentTip: 'f1',
            recordedBase: 'f1',
            behind: 1,
            ahead: 1,
            state: 'needsRestack',
            checkedOutIn: undefined,
            track: undefined,
            isHead: true,
          },
        ],
      },
    ],
    orphans: [],
  };
}

describe('StackState — reload', () => {
  test('reloads on repo.changed refsChanged for the same repo', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const stack = new StackState(bridge);
    transport.onRequest = () => emptyResult();
    stack.setRepoId(REPO_A);
    await sleep();
    transport.calls.length = 0;

    transport.onRequest = () => oneStackResult();
    transport.emit('repo.changed', { repoId: REPO_A, kind: 'refsChanged' });
    await sleep();

    expect(transport.calls.filter((c) => c.method === 'stack.list').length).toBe(1);
    expect(stack.stacks.value.length).toBe(1);
    expect(stack.stacks.value[0]?.branches.length).toBe(2);
    stack.dispose();
  });

  test('ignores repo.changed for a different repo', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const stack = new StackState(bridge);
    transport.onRequest = () => emptyResult();
    stack.setRepoId(REPO_A);
    await sleep();
    transport.calls.length = 0;

    transport.emit('repo.changed', { repoId: REPO_B, kind: 'refsChanged' });
    await sleep();

    expect(transport.calls.filter((c) => c.method === 'stack.list').length).toBe(0);
    stack.dispose();
  });

  test('ignores worktreeChanged (only refsChanged reloads)', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const stack = new StackState(bridge);
    transport.onRequest = () => emptyResult();
    stack.setRepoId(REPO_A);
    await sleep();
    transport.calls.length = 0;

    transport.emit('repo.changed', { repoId: REPO_A, kind: 'worktreeChanged' });
    await sleep();

    expect(transport.calls.filter((c) => c.method === 'stack.list').length).toBe(0);
    stack.dispose();
  });

  // A repo switch that lands while a stack.list request is still in flight must not let the OLD
  // repo's stale reply overwrite the NEW repo's state — WorktreeState.reload's own guard.
  test('a stale reply from a since-switched-away repo is dropped', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const stack = new StackState(bridge);

    let resolveA: ((v: StackListResult) => void) | undefined;
    transport.onRequest = () =>
      new Promise<StackListResult>((resolve) => {
        resolveA = resolve;
      });
    stack.setRepoId(REPO_A);
    await sleep();

    transport.onRequest = () => emptyResult();
    stack.setRepoId(REPO_B);
    await sleep();

    // The REPO_A request (still in flight) finally resolves — after REPO_B is already current.
    resolveA?.(oneStackResult());
    await sleep();

    expect(stack.stacks.value.length).toBe(0); // REPO_B's own (empty) result must win.
    stack.dispose();
  });
});

describe('StackState — ChangeRequestState.ensureSnapshot (F13)', () => {
  test('called once per load with the union of every branch name', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const pr = new ChangeRequestState(bridge);
    pr.setRepoId(REPO_A);
    const stack = new StackState(bridge, pr);

    const prCalls: string[] = [];
    transport.onRequest = (method, params) => {
      if (method === 'stack.list') return oneStackResult();
      if (method === 'branch.resolveChangeRequest') {
        prCalls.push((params as { branch: string }).branch);
        return { kind: 'ok', changeRequests: [] };
      }
      throw new Error(`unscripted request ${method}`);
    };
    stack.setRepoId(REPO_A);
    await sleep();

    expect(prCalls.sort(compareNatural)).toEqual(['feat1', 'feat2']);
    stack.dispose();
    pr.dispose();
  });

  test('a second load does not re-request an already-cached branch', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const pr = new ChangeRequestState(bridge);
    pr.setRepoId(REPO_A);
    const stack = new StackState(bridge, pr);

    let resolveCalls = 0;
    transport.onRequest = (method, params) => {
      if (method === 'stack.list') return oneStackResult();
      if (method === 'branch.resolveChangeRequest') {
        resolveCalls++;
        return {
          kind: 'ok',
          changeRequests: [
            {
              number: 1,
              title: 't',
              url: 'u',
              state: 'open',
              headRef: (params as { branch: string }).branch,
              headSha: 's',
              baseRef: 'main',
              updatedAt: 0,
            },
          ],
        };
      }
      throw new Error(`unscripted request ${method}`);
    };
    stack.setRepoId(REPO_A);
    await sleep();
    expect(resolveCalls).toBe(2);

    await stack.reload();
    await sleep();
    expect(resolveCalls).toBe(2); // both branches already cached — no new requests.
    stack.dispose();
    pr.dispose();
  });
});

describe('StackState — runRestack/progress/restacking', () => {
  test('runRestack sets restacking during the call, accumulates progress, and reloads after', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const stack = new StackState(bridge);
    transport.onRequest = () => emptyResult();
    stack.setRepoId(REPO_A);
    await sleep();

    let resolveRestack: ((v: unknown) => void) | undefined;
    transport.onRequest = (method) => {
      if (method === 'stack.restack') {
        return new Promise((resolve) => {
          resolveRestack = resolve;
        });
      }
      return oneStackResult();
    };

    const promise = stack.runRestack('feat2');
    expect(stack.restacking.value).toBe(true);

    transport.emit('stack.progress', { repoId: REPO_A, branch: 'feat1', index: 1, total: 2 });
    transport.emit('stack.progress', { repoId: REPO_A, branch: 'feat2', index: 2, total: 2 });
    expect(stack.progress.value.length).toBe(2);

    resolveRestack?.({
      ok: true,
      restacked: ['feat1', 'feat2'],
      stoppedAt: undefined,
      remaining: [],
      undo: undefined,
      head: { kind: 'branch', name: 'feat2' },
      inProgress: null,
    });
    await promise;
    await sleep();

    expect(stack.restacking.value).toBe(false);
    expect(stack.stacks.value.length).toBe(1); // reloaded after the run.
    stack.dispose();
  });

  test('progress arriving after the restack reply is ignored', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const stack = new StackState(bridge);
    transport.onRequest = (method) =>
      method === 'stack.restack'
        ? {
            ok: true,
            restacked: [],
            stoppedAt: undefined,
            remaining: [],
            undo: undefined,
            head: { kind: 'branch', name: 'feat2' },
            inProgress: null,
          }
        : emptyResult();
    stack.setRepoId(REPO_A);
    await sleep();

    await stack.runRestack('feat2');
    transport.emit('stack.progress', { repoId: REPO_A, branch: 'feat2', index: 1, total: 1 });
    expect(stack.progress.value.length).toBe(0);
    stack.dispose();
  });

  test('progress from a different repo is ignored', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const stack = new StackState(bridge);
    transport.onRequest = () => emptyResult();
    stack.setRepoId(REPO_A);
    await sleep();

    transport.emit('stack.progress', { repoId: REPO_B, branch: 'x', index: 1, total: 1 });
    expect(stack.progress.value.length).toBe(0);
    stack.dispose();
  });

  test('cancelRestack forwards to stack.cancelRestack and returns its cancelled flag', async () => {
    const transport = new FakeTransport();
    const bridge = new BridgeClient(transport);
    const stack = new StackState(bridge);
    transport.onRequest = () => emptyResult();
    stack.setRepoId(REPO_A);
    await sleep();

    transport.onRequest = () => ({ cancelled: true });
    const result = await stack.cancelRestack();
    expect(result).toBe(true);
    expect(transport.calls.at(-1)?.method).toBe('stack.cancelRestack');
    stack.dispose();
  });
});
