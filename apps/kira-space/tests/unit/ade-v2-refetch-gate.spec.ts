import { describe, expect, test } from 'bun:test';
import { QueryClient, QueryObserver } from '@tanstack/vue-query';
import { createRefetchGate } from '../../frontend/src/ade/v2/refetchGate';

// Single flight with a dirty flag: a burst of pushes during one slow fetch costs exactly one more fetch.
const key = ['adetask', 'board'] as const;

interface Harness {
  gate: ReturnType<typeof createRefetchGate>;
  queryClient: QueryClient;
  fetches: () => number;
  /** Finishes the oldest pending fetch; `fail` rejects it. */
  finish: (fail?: boolean) => void;
}

function harness(): Harness {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const pending: { resolve: () => void; reject: () => void }[] = [];
  let count = 0;
  queryClient.setQueryData(key, 0);
  const observer = new QueryObserver(queryClient, {
    queryKey: key,
    staleTime: Number.POSITIVE_INFINITY,
    queryFn: () => {
      count++;
      return new Promise<number>((resolve, reject) => {
        pending.push({ resolve: () => resolve(count), reject: () => reject(new Error('down')) });
      });
    },
  });
  observer.subscribe(() => {});
  return {
    queryClient,
    gate: createRefetchGate(queryClient, key),
    fetches: () => count,
    finish: (fail = false) => {
      const next = pending.shift();
      if (fail) next?.reject();
      else next?.resolve();
    },
  };
}

const tick = () => new Promise((r) => setTimeout(r, 0));

describe('createRefetchGate', () => {
  test('five signals during one slow fetch give exactly two fetches', async () => {
    const h = harness();
    for (let i = 0; i < 5; i++) h.gate.signal();
    await tick();
    expect(h.fetches()).toBe(1);
    h.finish();
    await tick();
    expect(h.fetches()).toBe(2);
    h.finish();
    await tick();
    expect(h.fetches()).toBe(2);
  });

  test('a signal after the fetch settled with nothing dirty gives one fetch', async () => {
    const h = harness();
    h.gate.signal();
    await tick();
    h.finish();
    await tick();
    expect(h.fetches()).toBe(1);
    h.gate.signal();
    await tick();
    expect(h.fetches()).toBe(2);
    h.finish();
    await tick();
    expect(h.fetches()).toBe(2);
  });

  test('a failed fetch still honors the dirty flag', async () => {
    const h = harness();
    h.gate.signal();
    h.gate.signal();
    await tick();
    h.finish(true);
    await tick();
    expect(h.fetches()).toBe(2);
    h.finish();
    await tick();
  });

  test('a signal during a fetch the gate did not start waits for it, then fetches again', async () => {
    const h = harness();
    void h.queryClient.invalidateQueries({ queryKey: key, exact: true });
    await tick();
    expect(h.fetches()).toBe(1);
    h.gate.signal();
    await tick();
    expect(h.fetches()).toBe(1);
    h.finish();
    await tick();
    expect(h.fetches()).toBe(2);
    h.finish();
    await tick();
    expect(h.fetches()).toBe(2);
  });
});
