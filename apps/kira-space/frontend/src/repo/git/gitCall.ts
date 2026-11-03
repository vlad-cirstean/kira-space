/**
 * Direct calls to the bound `GitService` for callers outside `packages/git-ui`: no transport, no
 * lease. The caller holds the repo first (`repoHolds.ts`) and awaits the hold's `ready`; a missing
 * hold surfaces as `E_CONFLICT` from Go, not a hidden auto-open.
 */
import type { EventPayload, ParamsOf, ResultOf } from '@kira/git-ipc';
import { type BoundKey, callBound, onGitEvent } from './boundGit';
import { afterReviewWrite } from './reviewRepaint';

const inflight = new Set<() => void>();

/** Runs one bound git method. Errors stay `RpcError` (callers branch on `.code`); a review write
 *  that succeeds repaints every open review surface. */
export async function gitCall<K extends BoundKey>(
  method: K,
  params: ParamsOf<K>,
  signal?: AbortSignal,
): Promise<ResultOf<K>> {
  const result = (await callBound(method, params, signal, inflight)) as ResultOf<K>;
  afterReviewWrite(method, params);
  return result;
}

/** A Go-pushed git event of one repo; the window must hold the repo (`repoHolds.ts`). */
export function onGitRepoEvent<K extends 'repo.changed' | 'autoFetch.changed'>(
  event: K,
  gitRepoId: string,
  handler: (payload: EventPayload<K>) => void,
): () => void {
  return onGitEvent(event, (payload) => {
    if ((payload as { repoId: string }).repoId === gitRepoId) handler(payload as EventPayload<K>);
  });
}
