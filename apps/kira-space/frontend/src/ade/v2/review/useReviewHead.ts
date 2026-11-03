import { hostWords } from '@theme/i18n';
import { tryOnScopeDispose, useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { computed } from 'vue';
import { onGitRepoEvent } from '../../../repo/git/gitCall';
import type { ReviewWindowTarget } from '../wire';
import { headStale, useChangeRequestReview } from './useChangeRequestReview';

/** Whether the local branch is at the head the host reviews. Null when there is nothing to compare:
 *  no open change request, or no local branch. Refreshes on a ref change of the window's repo. */
export function useReviewHead(target: () => ReviewWindowTarget) {
  const { state } = useChangeRequestReview(target);
  const head = computed(() => {
    const d = state.data.value;
    if (d?.kind !== 'ok' || d.state !== 'open' || d.localHeadSha === '') return null;
    return {
      state: headStale(d) ? ('stale' as const) : ('current' as const),
      local: d.localHeadSha,
      host: d.headSha,
      short: hostWords(d.host).changeRequestShort,
    };
  });
  const refetchSoon = useDebounceFn(() => void state.refetch(), TIMING.fileSettle);
  // The repo hold comes from `useChangeRequestReview` in this scope.
  tryOnScopeDispose(
    onGitRepoEvent('repo.changed', target().gitRepoId, (p) => {
      if (p.kind === 'refsChanged') void refetchSoon();
    }),
  );
  return { head };
}
