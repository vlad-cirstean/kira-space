import { describe, expect, test } from 'bun:test';
import { effectScope, nextTick, ref } from 'vue';
import { createQueryClient } from '../state/queryClient';
import { type DraftSource, useGuardedDraft } from './guardedDraft';

const tick = () => new Promise((r) => setTimeout(r, 0));

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function setup(over: Partial<Parameters<typeof useGuardedDraft<string>>[0]> = {}) {
  const source = ref<DraftSource<string> | undefined>({ value: 'a', hash: 'h1' });
  const writes: Array<{ value: string; baseHash: string }> = [];
  const scope = effectScope();
  const d = scope.run(() =>
    useGuardedDraft<string>({
      source,
      initial: '',
      write: async (value, baseHash) => {
        writes.push({ value, baseHash });
        return { hash: `h${writes.length + 1}` };
      },
      queryClient: createQueryClient({ remote: false }),
      ...over,
    }),
  );
  if (!d) throw new Error('no scope');
  return { d, source, writes, stop: () => scope.stop() };
}

describe('useGuardedDraft', () => {
  test('follows the source while clean, ignores a push while dirty, never calls adopt', async () => {
    let adopted = 0;
    const { d, source, stop } = setup({ adopt: () => void adopted++ });
    expect(d.draft.value).toBe('a');
    source.value = { value: 'b', hash: 'h2' };
    await nextTick();
    expect(d.draft.value).toBe('b');
    d.edit('typed');
    source.value = { value: 'c', hash: 'h3' };
    await nextTick();
    expect(d.draft.value).toBe('typed');
    expect(d.baseHash.value).toBe('h2');
    expect(adopted).toBe(0);
    stop();
  });

  test('an edit while the write runs keeps the draft dirty', async () => {
    const gate = deferred<{ hash: string }>();
    const { d, stop } = setup({ write: () => gate.promise });
    d.edit('one');
    const saved = d.save();
    await tick();
    expect(d.saving.value).toBe(true);
    expect(await d.save()).toBe(false);
    d.edit('two');
    gate.resolve({ hash: 'h9' });
    expect(await saved).toBe(true);
    expect(d.baseHash.value).toBe('h9');
    expect(d.dirty.value).toBe(true);
    stop();
  });

  test('overwrite takes the fresh hash, then saves on it; a conflict clears when it starts', async () => {
    const conflictErr = Object.assign(new Error('stale'), { code: 'E_CONFLICT' });
    let first = true;
    const written: string[] = [];
    const { d, stop } = setup({
      refetch: async () => ({ value: 'theirs', hash: 'fresh' }),
      write: async (_v, baseHash) => {
        written.push(baseHash);
        if (first) {
          first = false;
          throw conflictErr;
        }
        return { hash: 'after' };
      },
    });
    d.edit('mine');
    expect(await d.save()).toBe(false);
    expect(d.conflict.value).toBe(true);
    await d.overwrite();
    expect(written).toEqual(['h1', 'fresh']);
    expect(d.conflict.value).toBe(false);
    expect(d.dirty.value).toBe(false);
    expect(d.draft.value).toBe('mine');
    stop();
  });

  test('side flushes when only it is pending, and the file is not written', async () => {
    let pending = true;
    let flushed = 0;
    const { d, writes, stop } = setup({
      side: {
        pending: () => pending,
        flush: async () => {
          flushed++;
          pending = false;
        },
      },
    });
    expect(d.canSave.value).toBe(true);
    expect(await d.save()).toBe(true);
    expect(flushed).toBe(1);
    expect(writes).toHaveLength(0);
    expect(await d.save()).toBe(false);
    stop();
  });

  test('a failed file write skips the side flush', async () => {
    let flushed = 0;
    const { d, stop } = setup({
      write: async () => {
        throw new Error('disk');
      },
      side: { pending: () => true, flush: async () => void flushed++ },
    });
    d.edit('x');
    expect(await d.save()).toBe(false);
    expect(flushed).toBe(0);
    expect(d.saveError.value).toBe('disk');
    stop();
  });
});
