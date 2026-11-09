import { queryOptions, useMutation, useQuery } from '@tanstack/vue-query';
import { tryOnScopeDispose, useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { queryClient } from '@workbench/state/queryClient';
import { computed } from 'vue';
import { control } from '../../../bridge/control';
import { onGitRepoEvent } from '../../../repo/git/gitCall';
import { useGitRepoHold } from '../../../repo/git/repoHolds';
import type { ReviewSync, ReviewWindowTarget } from '../wire';

/** How fresh the review's branch and base are against their remotes. Remote state is data Kira does
 *  not own, so the query re-reads on focus; a ref change of the window's repo re-reads it too. */
export function reviewSyncOptions(branchId: string) {
  return queryOptions({
    queryKey: ['adetask', 'reviewSync', branchId] as const,
    queryFn: () => control.adeTaskReviewSync({ branchId }),
    staleTime: 60_000,
    refetchOnWindowFocus: true,
  });
}

/** Sync facts plus the Fetch and Update actions. Either action re-reads the facts and the head
 *  badge's change request state; the file list and open diffs follow through the repo's ref events. */
export function useReviewSync(target: () => ReviewWindowTarget) {
  // The hold keeps the repo open for its ref events.
  useGitRepoHold(() => target().gitRepoId);
  const sync = useQuery(reviewSyncOptions(target().branchId));
  const refetchSoon = useDebounceFn(() => void sync.refetch(), TIMING.fileSettle);
  tryOnScopeDispose(
    onGitRepoEvent('repo.changed', target().gitRepoId, (p) => {
      if (p.kind === 'refsChanged') void refetchSoon();
    }),
  );

  // Not awaited: the action is done when the call returns, the re-reads follow.
  const settled = () => {
    void queryClient.invalidateQueries({ queryKey: reviewSyncOptions(target().branchId).queryKey });
    void queryClient.invalidateQueries({ queryKey: ['changeRequestReviewState'] });
  };
  const fetch = useMutation({
    mutationFn: async () => {
      const res = await control.adeTaskReviewFetch({ branchId: target().branchId });
      if (res.error) throw new Error(res.error.message);
    },
    onSettled: settled,
  });
  const update = useMutation({
    mutationFn: (side: 'branch' | 'base'): Promise<ReviewSync> =>
      control.adeTaskReviewUpdateLocal({ branchId: target().branchId, side }),
    onSettled: settled,
  });
  const error = computed(() => (fetch.error.value ?? update.error.value)?.message ?? '');
  function fetchNow(): void {
    update.reset();
    fetch.mutate();
  }
  function updateNow(side: 'branch' | 'base'): void {
    fetch.reset();
    update.mutate(side);
  }
  return {
    sync,
    fetching: fetch.isPending,
    updating: update.isPending,
    error,
    fetchNow,
    updateNow,
  };
}
