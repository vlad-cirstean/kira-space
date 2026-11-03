/**
 * Window-level repo holds. The window's Go connection keeps one hold per repoId; every caller that
 * needs a repo open (the git transport, direct `gitCall` consumers) shares one refcounted entry here,
 * so the last release alone sends the bound `repo.close`.
 *
 * Opens and closes of one path run in order, so a re-hold right after a release cannot be undone by
 * the older close. A failed open drops its entry; the next hold retries.
 */
import type { ResultOf } from '@kira/git-ipc';
import { tryOnScopeDispose } from '@vueuse/core';
import { logError } from '@workbench/bridge/log';
import { watch } from 'vue';
import { callBound } from './boundGit';

export interface GitRepoHold {
  /** Bound repo.open's result; rejects when the open failed. */
  readonly ready: Promise<ResultOf<'repo.open'>>;
  /** Idempotent. The last release of a repo sends bound repo.close. */
  release(): void;
}

interface Entry {
  count: number;
  readonly ready: Promise<ResultOf<'repo.open'>>;
}

const inflight = new Set<() => void>();
const entries = new Map<string, Entry>();
const queues = new Map<string, Promise<unknown>>();

/** Runs `fn` after every earlier open/close of `path` settled. */
function enqueue<T>(path: string, fn: () => Promise<T>): Promise<T> {
  const run = (queues.get(path) ?? Promise.resolve()).then(fn);
  const tail = run.then(
    () => undefined,
    () => undefined,
  );
  queues.set(path, tail);
  void tail.then(() => {
    if (queues.get(path) === tail) queues.delete(path);
  });
  return run;
}

/** A hold scoped to the calling composable or component; follows `path` and releases on dispose.
 *  `ready` rethrows a failed open and takes a fresh hold, so the next call retries. */
export function useGitRepoHold(path: () => string): { ready: () => Promise<unknown> } {
  let hold: GitRepoHold | undefined;
  const swap = (next: string) => {
    hold?.release();
    hold = next === '' ? undefined : holdGitRepo(next);
  };
  watch(path, swap, { immediate: true });
  tryOnScopeDispose(() => {
    hold?.release();
    hold = undefined;
  });
  return {
    ready: async () => {
      const current = hold;
      if (!current) return;
      try {
        return await current.ready;
      } catch (err) {
        if (hold === current) swap(path());
        throw err;
      }
    },
  };
}

/** Holds `path` for the duration of `fn`. */
export async function withGitRepoHold<T>(path: string, fn: () => Promise<T>): Promise<T> {
  const hold = holdGitRepo(path);
  try {
    await hold.ready;
    return await fn();
  } finally {
    hold.release();
  }
}

export function holdGitRepo(path: string): GitRepoHold {
  let entry = entries.get(path);
  if (!entry) {
    const fresh: Entry = {
      count: 0,
      ready: enqueue(
        path,
        () =>
          callBound('repo.open', { path }, undefined, inflight) as Promise<ResultOf<'repo.open'>>,
      ),
    };
    fresh.ready.catch((err: unknown) => {
      if (entries.get(path) === fresh) entries.delete(path);
      logError('repoHolds', 'repo.open failed', err);
    });
    entries.set(path, fresh);
    entry = fresh;
  }
  const held = entry;
  held.count++;
  let released = false;
  return {
    ready: held.ready,
    release() {
      if (released) return;
      released = true;
      held.count--;
      if (held.count > 0) return;
      if (entries.get(path) === held) entries.delete(path);
      void enqueue(path, async () => {
        const opened = await held.ready;
        const repoId = opened.kind === 'ok' ? opened.repo.repoId : undefined;
        if (repoId) await callBound('repo.close', { repoId }, undefined, inflight);
      }).catch(() => {
        // Best effort: the window's close releases any hold this misses.
      });
    },
  };
}
