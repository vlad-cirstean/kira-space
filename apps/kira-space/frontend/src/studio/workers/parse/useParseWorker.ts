import { tryOnScopeDispose } from '@vueuse/core';
import type { RunOptions } from './client';
import { parseClient } from './instance';
import type { JobInput, JobKind, JobOutput } from './protocol';

/**
 * Component-scoped access to the shared parse worker. Jobs started through it abort when the
 * scope disposes; `runLatest` also aborts the previous job under the same key.
 */
export function useParseWorker() {
  const scope = new AbortController();
  const latest = new Map<string, AbortController>();
  tryOnScopeDispose(() => scope.abort());

  function run<K extends JobKind>(
    kind: K,
    input: JobInput<K>,
    opts?: RunOptions,
  ): Promise<JobOutput<K>> {
    const signal = opts?.signal ? AbortSignal.any([scope.signal, opts.signal]) : scope.signal;
    return parseClient.run(kind, input, { signal });
  }

  function runLatest<K extends JobKind>(
    key: string,
    kind: K,
    input: JobInput<K>,
  ): Promise<JobOutput<K>> {
    latest.get(key)?.abort();
    const ctrl = new AbortController();
    latest.set(key, ctrl);
    return run(kind, input, { signal: ctrl.signal }).finally(() => {
      if (latest.get(key) === ctrl) latest.delete(key);
    });
  }

  return { run, runLatest };
}
