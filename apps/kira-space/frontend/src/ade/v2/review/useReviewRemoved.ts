import type { ReviewRemovedEntry } from '@kira/git-ipc';
import { useMutation, useQuery } from '@tanstack/vue-query';
import { tryOnScopeDispose, useDebounceFn } from '@vueuse/core';
import { TIMING } from '@workbench/behaviour/timing';
import { queryClient } from '@workbench/state/queryClient';
import { computed, watch } from 'vue';
import { gitCall, onGitRepoEvent } from '../../../repo/git/gitCall';
import { useGitRepoHold } from '../../../repo/git/repoHolds';
import { onReviewRepaint } from '../../../repo/git/reviewRepaint';
import type { ReviewWindowTarget } from '../wire';
import { useReviewWorking } from './useReviewWorking';

const reviewRemovedKey = (gitRepoId: string, branch: string, base: string) =>
  ['reviewRemoved', gitRepoId, branch, base] as const;

/** Reviewed lines that left the branch since they were reviewed, committed and uncommitted. The
 *  list call captures pending removals, so any change that could remove reviewed lines refetches:
 *  a ref change, a review mark, and a changed uncommitted list. */
export function useReviewRemoved(target: () => ReviewWindowTarget) {
  const key = computed(() => reviewRemovedKey(target().gitRepoId, target().branch, target().base));
  const hold = useGitRepoHold(() => target().gitRepoId);
  const removed = useQuery({
    queryKey: key,
    queryFn: async (): Promise<readonly ReviewRemovedEntry[]> => {
      const tg = target();
      await hold.ready();
      const res = await gitCall('review.removed.list', {
        repoId: tg.gitRepoId,
        branch: tg.branch,
        base: tg.base,
      });
      return res.entries;
    },
    refetchOnWindowFocus: true,
  });

  const invalidate = (): void => {
    void queryClient.invalidateQueries({ queryKey: key.value });
  };
  const invalidateSoon = useDebounceFn(invalidate, TIMING.fileSettle);
  const stops = [
    onGitRepoEvent('repo.changed', target().gitRepoId, (p) => {
      if (p.kind === 'refsChanged') void invalidateSoon();
    }),
    onReviewRepaint((event) => {
      if (
        event.kind === 'mark' &&
        event.repoId === target().gitRepoId &&
        event.branch === target().branch
      ) {
        void invalidateSoon();
      }
    }),
  ];
  tryOnScopeDispose(() => {
    for (const stop of stops) stop();
  });
  const working = useReviewWorking(() => target().branchId);
  // Compare data, not dataUpdatedAt: identical refetches keep their structural-shared reference.
  watch(
    () => working.data.value,
    () => void invalidateSoon(),
  );

  const dismiss = useMutation({
    mutationFn: async (ids: readonly number[]) => {
      const tg = target();
      await hold.ready();
      return gitCall('review.removed.dismiss', {
        repoId: tg.gitRepoId,
        branch: tg.branch,
        ids,
      });
    },
    onSuccess: (_res, ids) => {
      queryClient.setQueryData<readonly ReviewRemovedEntry[]>(key.value, (old) =>
        old?.filter((e) => !ids.includes(e.id)),
      );
      invalidate();
    },
  });
  return { removed, dismiss };
}
