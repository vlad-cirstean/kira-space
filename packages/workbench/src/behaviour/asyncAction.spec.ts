import { describe, expect, test } from 'bun:test';
import { effectScope } from 'vue';
import { createQueryClient } from '../state/queryClient';
import { useAsyncAction } from './asyncAction';

function deferred() {
  let resolve!: () => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const tick = () => new Promise((r) => setTimeout(r, 0));

function setup(queue: Array<ReturnType<typeof deferred>>) {
  const scope = effectScope();
  const action = scope.run(() =>
    useAsyncAction(() => (queue.shift() as ReturnType<typeof deferred>).promise, {
      queryClient: createQueryClient({ remote: false }),
    }),
  );
  if (!action) throw new Error('no scope');
  return { action, stop: () => scope.stop() };
}

describe('useAsyncAction overlapping runs', () => {
  test('pending holds until the newest run settles; a late older failure is dropped', async () => {
    const a = deferred();
    const b = deferred();
    const { action, stop } = setup([a, b]);
    const first = action.run();
    await tick();
    const second = action.run();
    await tick();
    b.resolve();
    expect(await second).toBe(true);
    expect(action.pending.value).toBe(false);
    a.reject(new Error('late'));
    expect(await first).toBe(false);
    await tick();
    expect(action.error.value).toBeNull();
    stop();
  });

  test('an older run failing first leaves error null while the newer still runs', async () => {
    const a = deferred();
    const b = deferred();
    const { action, stop } = setup([a, b]);
    const first = action.run();
    await tick();
    const second = action.run();
    await tick();
    a.reject(new Error('older'));
    expect(await first).toBe(false);
    await tick();
    expect(action.error.value).toBeNull();
    expect(action.pending.value).toBe(true);
    b.resolve();
    await second;
    stop();
  });
});
