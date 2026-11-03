// repoHolds.ts refcounts one window-level hold per repo: the last release alone closes it, a failed
// open is retried by the next hold, and a release before the open answers closes only after it.
import '@workbench/testing/unit/window';

import { afterEach, describe, expect, test } from 'bun:test';
import { resetCallFactory, setCallFactory } from '@workbench/testing/unit/wailsRuntime';

const { holdGitRepo } = await import('../../frontend/src/repo/git/repoHolds');

interface BoundCall {
  name: string;
  arg: { windowKey: string; params?: { path?: string; repoId?: string } };
  resolve(value: unknown): void;
  reject(err: unknown): void;
}

const calls: BoundCall[] = [];

function recordBoundCalls(): void {
  calls.length = 0;
  setCallFactory((name, arg) => {
    // The window reset every bound call waits for.
    if (String(name).endsWith('.ResetWindow') || !/GitService\./.test(String(name))) {
      return Object.assign(Promise.resolve(null), { cancel() {} });
    }
    let resolve!: (value: unknown) => void;
    let reject!: (err: unknown) => void;
    const promise = new Promise<unknown>((res, rej) => {
      resolve = res;
      reject = rej;
    }) as Promise<unknown> & { cancel(): void };
    promise.cancel = () => {};
    calls.push({
      name: String(name).split('.').pop() ?? '',
      arg: arg as BoundCall['arg'],
      resolve,
      reject,
    });
    return promise;
  });
}

afterEach(resetCallFactory);

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
const names = () => calls.map((c) => c.name);
const opened = (repoId: string) => ({ kind: 'ok', repo: { repoId } });

describe('window repo holds', () => {
  test('two holds open once; only the last release closes', async () => {
    recordBoundCalls();
    const a = holdGitRepo('/r/one');
    const b = holdGitRepo('/r/one');
    await tick();
    expect(names()).toEqual(['RepoOpen']);
    calls[0]?.resolve(opened('/r/one'));
    await Promise.all([a.ready, b.ready]);

    a.release();
    a.release();
    await tick();
    expect(names()).toEqual(['RepoOpen']);

    b.release();
    await tick();
    expect(names()).toEqual(['RepoOpen', 'RepoClose']);
    expect(calls[1]?.arg.params).toEqual({ repoId: '/r/one' });
  });

  test('a failed open is not cached; the next hold retries', async () => {
    recordBoundCalls();
    const first = holdGitRepo('/r/two');
    await tick();
    calls[0]?.reject(new Error('boom'));
    await expect(first.ready).rejects.toBeDefined();
    first.release();
    await tick();
    expect(names()).toEqual(['RepoOpen']);

    const second = holdGitRepo('/r/two');
    await tick();
    expect(names()).toEqual(['RepoOpen', 'RepoOpen']);
    calls[1]?.resolve(opened('/r/two'));
    await second.ready;
    second.release();
    await tick();
    expect(names()).toEqual(['RepoOpen', 'RepoOpen', 'RepoClose']);
  });

  test('a release before the open answers closes after it, never before', async () => {
    recordBoundCalls();
    const hold = holdGitRepo('/r/three');
    await tick();
    hold.release();
    await tick();
    expect(names()).toEqual(['RepoOpen']);

    calls[0]?.resolve(opened('/r/three'));
    await tick();
    expect(names()).toEqual(['RepoOpen', 'RepoClose']);
  });

  test('a hold taken right after the last release reopens after the close', async () => {
    recordBoundCalls();
    const first = holdGitRepo('/r/four');
    await tick();
    calls[0]?.resolve(opened('/r/four'));
    await first.ready;
    first.release();
    const second = holdGitRepo('/r/four');
    await tick();
    expect(names()).toEqual(['RepoOpen', 'RepoClose']);
    calls[1]?.resolve({});
    await tick();
    expect(names()).toEqual(['RepoOpen', 'RepoClose', 'RepoOpen']);
    calls[2]?.resolve(opened('/r/four'));
    await second.ready;
    second.release();
  });
});
